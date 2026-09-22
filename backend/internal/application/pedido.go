package application

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mornix/elerp/internal/domain/aplicacion"
	"github.com/mornix/elerp/internal/domain/cuenta"
	"github.com/mornix/elerp/internal/domain/pedido"
)

/* PEDIDOS PARA LLEVAR — ciclo de vida.
 *
 * CONFIRMAR ES EL PUNTO SIN RETORNO. Ahí se revisa el stock, se valida la
 * dirección contra las zonas, se calcula el costo de envío y se fija la promesa
 * de entrega. Antes de confirmar, un pedido es una intención; después, es trabajo
 * en cocina o en el estante, mercancía comprometida y una promesa a un cliente.
 * Por eso todo lo que puede fallar se comprueba ANTES y junto: aceptar y después
 * descubrir que no hay stock o que la dirección está fuera de zona es el caso
 * peor, porque ya se cobró y ya se cocinó.
 *
 * Y por eso los pedidos que entran por API nacen `nuevo`: que una persona mire
 * es el default seguro. La confirmación automática existe, pero se enciende a
 * propósito y por canal.
 */

var (
	ErrPedidosNoDisponible = errors.New("el módulo de pedidos no está disponible")
	ErrPedidoNoExiste      = errors.New("el pedido no existe")
	ErrTransicion          = errors.New("el pedido no puede pasar a ese estado desde donde está")
	ErrPedidoSinItems      = errors.New("el pedido no tiene renglones")
	ErrPedidoSinDestino    = errors.New("el pedido necesita una dirección de entrega")
	ErrFueraDeZona         = errors.New("la dirección está fuera de las zonas de reparto")
	ErrPedidoMinimo        = errors.New("el pedido no alcanza el mínimo de la zona")
	ErrRepartidorNoExiste  = errors.New("el repartidor no existe o no está disponible")
	ErrSinTracking         = errors.New("el pedido todavía no tiene número de envío")
)

// ConPedidos cablea el módulo. Sin él, ElERP se comporta como si no existiera.
func (s *Service) ConPedidos(p pedido.Repository, c pedido.CanalRepository,
	z pedido.ZonaRepository, r pedido.RepartidorRepository) *Service {
	s.pedidos = p
	s.canalesPedido = c
	s.zonasPedido = z
	s.repartidores = r
	return s
}

/* HAY ALGO QUE PRODUCIR.
 *
 * Lo decide el PRODUCTO, no el pedido: un renglón con receta se produce —hoy un
 * plato, mañana una pieza formulada— y uno sin receta sale del estante. Por eso
 * no hay «modo restaurante» ni «modo retail»: hay pedidos con renglones que se
 * producen y pedidos sin ellos, y el ciclo es el mismo.
 *
 * Se exige además que la empresa tenga el módulo de producción activo, porque
 * mandar una comanda a un tablero que nadie mira es peor que no mandarla: el
 * pedido quedaría esperando a una cocina que no existe.
 */
func (s *Service) hayQueProducir(empresaID string, items []pedido.Item) bool {
	if !s.ModuloActivo(empresaID, aplicacion.ModRestaurante) {
		return false
	}
	for _, it := range items {
		if p, ok := s.productos.BySKU(empresaID, it.SKU); ok && p.EsPlato {
			return true
		}
	}
	return false
}

// EntradaPedido son los datos con los que nace un pedido, vengan del mostrador o
// de un canal conectado.
type EntradaPedido struct {
	EmpresaID         string
	SedeID            string
	Origen            string
	CanalID           string
	ReferenciaExterna string
	ClienteID         string
	ClienteNombre     string
	Destino           pedido.Destino
	Items             []pedido.Item
	FormaPago         string
	Total             float64
	ProgramadoPara    string
	Actor             string
	OrigenEvento      string
}

// CrearPedido da de alta un pedido.
//
// Nace CONFIRMADO si lo arma el mostrador —una persona ya lo revisó al armarlo—
// y NUEVO si entra por API. Los pedidos de canal con confirmación automática
// pasan solos, pero solo si superan las mismas validaciones: automático no
// significa sin revisar, significa revisado por la máquina.
func (s *Service) CrearPedido(in EntradaPedido) (pedido.Pedido, error) {
	if s.pedidos == nil {
		return pedido.Pedido{}, ErrPedidosNoDisponible
	}
	empresaID := in.EmpresaIDOrEmpty()
	if len(in.Items) == 0 {
		return pedido.Pedido{}, ErrPedidoSinItems
	}
	if strings.TrimSpace(in.Destino.Direccion) == "" {
		return pedido.Pedido{}, ErrPedidoSinDestino
	}
	if !pedido.OrigenValido(in.Origen) {
		in.Origen = pedido.OrigenManual
	}
	// DEDUPLICACIÓN: un canal que reintenta no puede crear dos pedidos. Es la
	// razón por la que la referencia externa existe.
	if in.CanalID != "" && in.ReferenciaExterna != "" {
		if ya, existe := s.pedidos.ByReferencia(empresaID, in.CanalID, in.ReferenciaExterna); existe {
			return ya, nil
		}
	}

	p := pedido.Pedido{
		EmpresaID: empresaID, SedeID: in.SedeID,
		Estado: pedido.EstadoNuevo, Origen: in.Origen,
		CanalID: in.CanalID, ReferenciaExterna: in.ReferenciaExterna,
		ClienteID: in.ClienteID, ClienteNombre: in.ClienteNombre,
		Destino: in.Destino, Items: in.Items,
		FormaPago: in.FormaPago, Total: in.Total,
		ProgramadoPara: in.ProgramadoPara,
		TokenPublico:   tokenSeguimiento(),
		Creado:         ahora(),
	}
	p.Normalizar()
	if p.FormaPago == "" {
		p.FormaPago = pedido.PagoContraEntrega
	}
	// El correlativo visible del local. No es fiscal: la factura lleva el suyo.
	if s.numerador != nil {
		p.Numero = s.numerador.Siguiente(empresaID, in.SedeID, "PED")
	}

	var canal pedido.Canal
	if in.CanalID != "" && s.canalesPedido != nil {
		canal, _ = s.canalesPedido.ByID(empresaID, in.CanalID)
		p.CanalNombre = canal.Nombre
		// La ventana de aceptación se fija al nacer: vencida, el canal da el pedido
		// por perdido y avisa al cliente, así que hay que verla corriendo en la
		// bandeja y no descubrirla después.
		if canal.MinutosAceptacion > 0 {
			p.VenceAceptacion = time.Now().Add(time.Duration(canal.MinutosAceptacion) * time.Minute).Format(time.RFC3339)
		}
	}
	p.Bitacora = []pedido.Evento{{
		Cuando: ahora(), Estado: pedido.EstadoNuevo,
		Actor: in.Actor, Origen: in.OrigenEvento,
	}}
	out := s.pedidos.Append(p)
	s.audit.Append(evento(empresaID, in.Actor, in.OrigenEvento, "pedido.creado",
		fmt.Sprintf("#%d", out.Numero), out.Origen))

	// El mostrador nace confirmado; el canal, solo si lo pidió explícitamente.
	if in.Origen == pedido.OrigenManual || canal.ConfirmacionAutomatica {
		if conf, err := s.ConfirmarPedido(empresaID, out.ID, in.Actor, in.OrigenEvento); err == nil {
			return conf, nil
		} else if in.Origen == pedido.OrigenManual {
			// Un pedido de mostrador que no pasa las validaciones se devuelve con el
			// motivo: el cajero tiene al cliente enfrente y puede arreglarlo.
			return out, err
		}
		// Uno automático que falla se QUEDA en la bandeja, que es justo lo que la
		// confirmación automática promete: solo se detienen los que fallan.
	}
	return s.recargarPedido(empresaID, out.ID), nil
}

// EmpresaIDOrEmpty existe para no arrastrar el campo por toda la firma.
func (in EntradaPedido) EmpresaIDOrEmpty() string { return in.EmpresaID }

// ConfirmarPedido es el punto sin retorno.
func (s *Service) ConfirmarPedido(empresaID, pedidoID, actor, origen string) (pedido.Pedido, error) {
	p, ok := s.pedidoDe(empresaID, pedidoID)
	if !ok {
		return pedido.Pedido{}, ErrPedidoNoExiste
	}
	if !pedido.PuedePasarA(p.Estado, pedido.EstadoConfirmado) {
		return pedido.Pedido{}, ErrTransicion
	}
	// ZONA: se resuelve antes que nada. Fuera de zona es la razón más común de
	// rechazo, y detectarla acá evita el caso peor — aceptado, cobrado y cocinado
	// y después nadie puede llevarlo.
	zona, err := s.zonaDe(empresaID, p)
	if err != nil {
		return pedido.Pedido{}, err
	}
	if zona.ID != "" {
		if zona.PedidoMinimo > 0 && p.Total < zona.PedidoMinimo {
			return pedido.Pedido{}, fmt.Errorf("%w: %s pide un mínimo de %.2f", ErrPedidoMinimo, zona.Nombre, zona.PedidoMinimo)
		}
		p.Destino.ZonaID, p.Destino.ZonaNombre = zona.ID, zona.Nombre
		p.CostoEnvio = zona.CostoEnvio
		if zona.MinutosPromesa > 0 {
			p.PromesaEntrega = time.Now().Add(time.Duration(zona.MinutosPromesa) * time.Minute).Format(time.RFC3339)
		}
	}
	// QUIÉN LO LLEVA: el canal manda sobre la zona. Un canal que reparte con su
	// flota no deja decisión que tomar.
	if s.canalesPedido != nil && p.CanalID != "" {
		if c, ok := s.canalesPedido.ByID(empresaID, p.CanalID); ok {
			p.ModoEnvio = c.ModoEnvioDe(zona)
		}
	}
	if p.ModoEnvio == "" {
		p.ModoEnvio = pedido.EnvioPropio
		if zona.ModoEnvio != "" {
			p.ModoEnvio = zona.ModoEnvio
		}
	}
	p.VenceAceptacion = ""

	p = s.marcar(p, pedido.EstadoConfirmado, actor, origen, "")
	s.pedidos.Update(p)
	s.audit.Append(evento(empresaID, actor, origen, "pedido.confirmado", fmt.Sprintf("#%d", p.Numero), p.ModoEnvio))

	// Un pedido PROGRAMADO no entra a preparación al confirmarse: entra a su hora.
	// Mandarlo ahora a cocina significaría comida fría a las ocho para las diez.
	if p.ProgramadoPara != "" && p.ProgramadoPara > ahora() {
		return p, nil
	}
	return s.enviarAPreparacion(p, actor, origen), nil
}

// enviarAPreparacion pone el pedido en preparación y, si hay algo que producir,
// manda la comanda.
//
// Cuando hay producción se abre una cuenta SIN MESA: los renglones llegan al
// tablero de siempre, con los mismos estados y el mismo gesto del cocinero.
// Cuando no la hay no se abre nada —una cuenta vacía en un local sin salón es un
// fantasma en una pantalla que nadie mira— y el pedido queda esperando a que
// quien lo arma lo marque listo. El estado es el mismo en los dos casos.
func (s *Service) enviarAPreparacion(p pedido.Pedido, actor, origen string) pedido.Pedido {
	if s.hayQueProducir(p.EmpresaID, p.Items) && s.cuentasMesa != nil {
		cta, err := s.AbrirCuenta(AperturaCuenta{
			EmpresaID: p.EmpresaID, SedeID: p.SedeID,
			MesoneroNombre: "Delivery", Actor: actor, Origen: origen,
		})
		if err == nil {
			p.CuentaID = cta.ID
			// La comanda tiene que decir de DÓNDE viene y con qué números. Cuando
			// el courier de una app llega al mostrador preguntando por un número,
			// ese número es el de la app, no el nuestro — así que van los dos.
			cta.PedidoID = p.ID
			cta.PedidoNumero = p.Numero
			cta.PedidoReferencia = p.ReferenciaExterna
			cta.PedidoOrigen = p.CanalNombre
			if cta.PedidoOrigen == "" {
				cta.PedidoOrigen = etiquetaOrigen(p.Origen)
			}
			cta.MesaNombre = etiquetaComanda(p, cta.PedidoOrigen)
			s.cuentasMesa.Update(cta)
			items := make([]ItemInput, 0, len(p.Items))
			for _, it := range p.Items {
				items = append(items, ItemInput{
					SKU: it.SKU, Nombre: it.Nombre, Cantidad: it.Cantidad,
					PrecioUnitario: it.PrecioUnitario, Nota: it.Nota,
				})
			}
			// rolActor vacío: la asignación de mesas no aplica a un pedido de
			// delivery, que no tiene mesa que asignar.
			if _, err := s.AgregarItems(p.EmpresaID, cta.ID, actor, "", origen, items); err == nil {
				s.EnviarACocina(p.EmpresaID, cta.ID, actor, origen)
			}
		}
	}
	p = s.marcar(p, pedido.EstadoEnPreparacion, actor, origen, "")
	s.pedidos.Update(p)
	return p
}

// MarcarListo deja el pedido listo para despachar y EMITE EL TRACKING.
//
// Es la MISMA operación venga de donde venga: la dispara quien arma el pedido en
// una tienda, o el cierre del último renglón en cocina. No hay dos caminos, hay
// dos disparadores del mismo.
//
// El tracking se emite acá y no al confirmar porque es la llave del envío: la
// etiqueta impresa, el enlace del cliente y la referencia frente a la app.
// Emitirlo al confirmar obligaría a anularlo cada vez que un pedido se cancela
// en preparación, que es cuando más se cancelan.
func (s *Service) MarcarListo(empresaID, pedidoID, actor, origen string) (pedido.Pedido, error) {
	p, ok := s.pedidoDe(empresaID, pedidoID)
	if !ok {
		return pedido.Pedido{}, ErrPedidoNoExiste
	}
	if !pedido.PuedePasarA(p.Estado, pedido.EstadoListo) {
		return pedido.Pedido{}, ErrTransicion
	}
	if p.Tracking == "" {
		p.Tracking = s.nuevoTracking(empresaID, p.SedeID)
	}
	p = s.marcar(p, pedido.EstadoListo, actor, origen, "")
	s.pedidos.Update(p)
	s.audit.Append(evento(empresaID, actor, origen, "pedido.listo", p.Tracking, ""))
	return p, nil
}

// avisarProduccionLista marca listo el pedido cuya producción terminó.
//
// Se llama desde el tablero de cocina y no al revés porque quien sabe que el
// último renglón salió es la cuenta. Si la cuenta no es de un pedido, o todavía
// falta algo, no hace nada: es un aviso, no una obligación.
func (s *Service) avisarProduccionLista(empresaID string, c cuenta.Cuenta, actor, origen string) {
	if s.pedidos == nil {
		return
	}
	// Todos los renglones vivos tienen que estar listos (o servidos). Un renglón
	// cancelado no cuenta: cancelarlo es justamente decir que no se va a hacer.
	for _, it := range c.Items {
		if it.Estado == cuenta.ItemCancelado {
			continue
		}
		if it.Estado != cuenta.ItemListo && it.Estado != cuenta.ItemServido {
			return
		}
	}
	for _, p := range s.pedidos.List(empresaID, c.SedeID) {
		if p.CuentaID != c.ID || p.Estado != pedido.EstadoEnPreparacion {
			continue
		}
		s.MarcarListo(empresaID, p.ID, actor, origen)
		return
	}
}

// etiquetaOrigen nombra el origen cuando no vino por un canal con nombre.
func etiquetaOrigen(o string) string {
	switch o {
	case pedido.OrigenManual:
		return "Mostrador"
	case pedido.OrigenEcommerce:
		return "Tienda web"
	case pedido.OrigenAppCommerce:
		return "App de pedidos"
	}
	return "Delivery"
}

// etiquetaComanda es cómo se identifica la cuenta en el tablero de cocina, donde
// una cuenta de mesa muestra el número de mesa. Un pedido para llevar no tiene
// mesa, así que muestra de dónde viene y sus dos números: el interno primero
// —que es el que el local canta— y el de la app entre paréntesis, que es por el
// que va a preguntar el courier.
func etiquetaComanda(p pedido.Pedido, origen string) string {
	etq := fmt.Sprintf("%s · #%d", origen, p.Numero)
	if p.ReferenciaExterna != "" {
		etq += " (" + p.ReferenciaExterna + ")"
	}
	return etq
}

// nuevoTracking arma el número de envío con la forma DLV-AAMM-NNNN. Lleva el
// período adentro para que se lea de un vistazo de cuándo es: un número suelto
// de siete cifras no le dice nada a quien atiende el teléfono.
func (s *Service) nuevoTracking(empresaID, sedeID string) string {
	n := 0
	if s.numerador != nil {
		n = s.numerador.Siguiente(empresaID, sedeID, "DLV")
	}
	t := time.Now()
	return fmt.Sprintf("DLV-%02d%02d-%04d", t.Year()%100, int(t.Month()), n)
}

// AsignarRepartidor pone el pedido a nombre de alguien de la flota propia.
func (s *Service) AsignarRepartidor(empresaID, pedidoID, repartidorID, actor, origen string) (pedido.Pedido, error) {
	p, ok := s.pedidoDe(empresaID, pedidoID)
	if !ok {
		return pedido.Pedido{}, ErrPedidoNoExiste
	}
	if !pedido.PuedePasarA(p.Estado, pedido.EstadoAsignado) {
		return pedido.Pedido{}, ErrTransicion
	}
	if s.repartidores == nil {
		return pedido.Pedido{}, ErrPedidosNoDisponible
	}
	r, ok := s.repartidores.ByID(empresaID, repartidorID)
	if !ok || !r.Activo {
		return pedido.Pedido{}, ErrRepartidorNoExiste
	}
	p.RepartidorID, p.RepartidorNombre = r.ID, r.Nombre
	p.ModoEnvio = pedido.EnvioPropio
	p = s.marcar(p, pedido.EstadoAsignado, actor, origen, "")
	s.pedidos.Update(p)
	s.audit.Append(evento(empresaID, actor, origen, "pedido.asignado", p.Tracking, r.Nombre))
	return p, nil
}

// MarcarEnRuta registra que el pedido salió del local.
func (s *Service) MarcarEnRuta(empresaID, pedidoID, actor, origen string) (pedido.Pedido, error) {
	p, ok := s.pedidoDe(empresaID, pedidoID)
	if !ok {
		return pedido.Pedido{}, ErrPedidoNoExiste
	}
	if !pedido.PuedePasarA(p.Estado, pedido.EstadoEnRuta) {
		return pedido.Pedido{}, ErrTransicion
	}
	if p.Tracking == "" {
		return pedido.Pedido{}, ErrSinTracking
	}
	p = s.marcar(p, pedido.EstadoEnRuta, actor, origen, "")
	s.pedidos.Update(p)
	return p, nil
}

// CierreEntrega son los datos con que se cierra una entrega.
type CierreEntrega struct {
	Prueba string
	// CobradoBs es lo que el repartidor recibió en la puerta. Se guarda aunque el
	// pedido venga pagado por el canal: la diferencia entre lo que debía cobrar y
	// lo que trajo es lo que se liquida al volver.
	CobradoBs float64
	Motivo    string
}

// MarcarEntregado cierra el pedido.
func (s *Service) MarcarEntregado(empresaID, pedidoID, actor, origen string, in CierreEntrega) (pedido.Pedido, error) {
	p, ok := s.pedidoDe(empresaID, pedidoID)
	if !ok {
		return pedido.Pedido{}, ErrPedidoNoExiste
	}
	if !pedido.PuedePasarA(p.Estado, pedido.EstadoEntregado) {
		return pedido.Pedido{}, ErrTransicion
	}
	p.PruebaEntrega = strings.TrimSpace(in.Prueba)
	p.Cerrado = ahora()
	p = s.marcar(p, pedido.EstadoEntregado, actor, origen, "")
	s.pedidos.Update(p)
	s.audit.Append(evento(empresaID, actor, origen, "pedido.entregado", p.Tracking, p.PruebaEntrega))
	return p, nil
}

// EntregaFallida registra que no se pudo entregar. El motivo es obligatorio:
// «no se pudo» sin razón no sirve para decidir si se reintenta, se devuelve o
// se cobra la merma.
func (s *Service) EntregaFallida(empresaID, pedidoID, actor, origen, motivo string) (pedido.Pedido, error) {
	if strings.TrimSpace(motivo) == "" {
		return pedido.Pedido{}, ErrMotivoRequerido
	}
	p, ok := s.pedidoDe(empresaID, pedidoID)
	if !ok {
		return pedido.Pedido{}, ErrPedidoNoExiste
	}
	if !pedido.PuedePasarA(p.Estado, pedido.EstadoEntregaFallida) {
		return pedido.Pedido{}, ErrTransicion
	}
	p = s.marcar(p, pedido.EstadoEntregaFallida, actor, origen, motivo)
	s.pedidos.Update(p)
	s.audit.Append(evento(empresaID, actor, origen, "pedido.entrega_fallida", p.Tracking, motivo))
	return p, nil
}

// RechazarPedido niega un pedido. El motivo es obligatorio y se le avisa al
// canal: un pedido que desaparece sin explicación deja al cliente esperando.
func (s *Service) RechazarPedido(empresaID, pedidoID, actor, origen, motivo string) (pedido.Pedido, error) {
	if strings.TrimSpace(motivo) == "" {
		return pedido.Pedido{}, ErrMotivoRequerido
	}
	return s.cerrarCon(empresaID, pedidoID, pedido.EstadoRechazado, actor, origen, motivo)
}

// CancelarPedido lo cancela antes de la entrega.
func (s *Service) CancelarPedido(empresaID, pedidoID, actor, origen, motivo string) (pedido.Pedido, error) {
	if strings.TrimSpace(motivo) == "" {
		return pedido.Pedido{}, ErrMotivoRequerido
	}
	return s.cerrarCon(empresaID, pedidoID, pedido.EstadoCancelado, actor, origen, motivo)
}

// DevolverAlLocal cierra una entrega fallida cuya mercancía volvió.
func (s *Service) DevolverAlLocal(empresaID, pedidoID, actor, origen, motivo string) (pedido.Pedido, error) {
	return s.cerrarCon(empresaID, pedidoID, pedido.EstadoDevuelto, actor, origen, motivo)
}

func (s *Service) cerrarCon(empresaID, pedidoID, estado, actor, origen, motivo string) (pedido.Pedido, error) {
	p, ok := s.pedidoDe(empresaID, pedidoID)
	if !ok {
		return pedido.Pedido{}, ErrPedidoNoExiste
	}
	if !pedido.PuedePasarA(p.Estado, estado) {
		return pedido.Pedido{}, ErrTransicion
	}
	p.MotivoCierre = strings.TrimSpace(motivo)
	p.Cerrado = ahora()
	p = s.marcar(p, estado, actor, origen, motivo)
	s.pedidos.Update(p)
	s.audit.Append(evento(empresaID, actor, origen, "pedido."+estado, fmt.Sprintf("#%d", p.Numero), motivo))
	return p, nil
}

// marcar cambia el estado y deja la huella en la bitácora. El estado del pedido
// es uno solo, pero CÓMO llegó ahí es lo que se necesita cuando alguien reclama.
func (s *Service) marcar(p pedido.Pedido, estado, actor, origen, motivo string) pedido.Pedido {
	p.Estado = estado
	p.Bitacora = append(p.Bitacora, pedido.Evento{
		Cuando: ahora(), Estado: estado, Actor: actor, Origen: origen, Motivo: motivo,
	})
	return p
}

/* --- Consultas ------------------------------------------------------------ */

func (s *Service) pedidoDe(empresaID, id string) (pedido.Pedido, bool) {
	if s.pedidos == nil {
		return pedido.Pedido{}, false
	}
	return s.pedidos.ByID(empresaID, id)
}

func (s *Service) recargarPedido(empresaID, id string) pedido.Pedido {
	p, _ := s.pedidoDe(empresaID, id)
	return p
}

// Pedidos lista los de una sede, los abiertos primero y por antigüedad: lo que
// lleva más tiempo esperando es lo que hay que atender.
func (s *Service) Pedidos(empresaID, sedeID string) []pedido.Pedido {
	if s.pedidos == nil {
		return []pedido.Pedido{}
	}
	out := s.pedidos.List(empresaID, sedeID)
	sort.SliceStable(out, func(i, j int) bool {
		fi, fj := pedido.Final(out[i].Estado), pedido.Final(out[j].Estado)
		if fi != fj {
			return !fi
		}
		return out[i].Creado < out[j].Creado
	})
	return out
}

// PedidoPorToken resuelve la página pública de seguimiento.
func (s *Service) PedidoPorToken(token string) (pedido.Pedido, bool) {
	if s.pedidos == nil || token == "" {
		return pedido.Pedido{}, false
	}
	return s.pedidos.ByToken(token)
}

// zonaDe resuelve en qué zona cae la dirección del pedido.
//
// Se evalúan de la MÁS CHICA a la más grande, así una zona cercana y barata gana
// sobre una lejana que también la contiene. Sin coordenadas no se puede decidir,
// y entonces NO se bloquea: un local que todavía no dibujó sus zonas tiene que
// poder despachar igual, y muchas direcciones venezolanas se dictan por
// referencia y no por punto en el mapa.
func (s *Service) zonaDe(empresaID string, p pedido.Pedido) (pedido.Zona, error) {
	if s.zonasPedido == nil {
		return pedido.Zona{}, nil
	}
	zonas := s.zonasPedido.List(empresaID, p.SedeID)
	if len(zonas) == 0 {
		return pedido.Zona{}, nil
	}
	if p.Destino.Lat == 0 && p.Destino.Lon == 0 {
		return pedido.Zona{}, nil
	}
	sede, ok := s.sedeDe(empresaID, p.SedeID)
	if !ok || (sede.Lat == 0 && sede.Lon == 0) {
		return pedido.Zona{}, nil
	}
	metros := distanciaM(sede.Lat, sede.Lon, p.Destino.Lat, p.Destino.Lon)
	sort.SliceStable(zonas, func(i, j int) bool { return zonas[i].RadioM < zonas[j].RadioM })
	for _, z := range zonas {
		if z.Cubre(metros) {
			return z, nil
		}
	}
	return pedido.Zona{}, fmt.Errorf("%w (%.0f m del local)", ErrFueraDeZona, metros)
}

func (s *Service) sedeDe(empresaID, sedeID string) (sedeCoord, bool) {
	if s.sedes == nil {
		return sedeCoord{}, false
	}
	for _, sd := range s.sedes.List(empresaID) {
		if sd.ID == sedeID {
			return sedeCoord{Lat: sd.Lat, Lon: sd.Lon}, true
		}
	}
	return sedeCoord{}, false
}

type sedeCoord struct{ Lat, Lon float64 }

// tokenSeguimiento genera el identificador de la página pública. Es aleatorio y
// NO el número de tracking: el tracking se imprime en la etiqueta y se dicta por
// teléfono, así que si fuera la llave de la página, cualquiera que oiga un
// número entraría a ver la dirección de otro cliente.
func tokenSeguimiento() string {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
}

// VencerAceptaciones cierra los pedidos que el canal ya dio por perdidos. Corre
// desde el lazo de fondo: la ventana vence sola, sin que nadie toque nada.
func (s *Service) VencerAceptaciones(empresaID string) int {
	if s.pedidos == nil {
		return 0
	}
	n := 0
	hoy := ahora()
	for _, p := range s.pedidos.List(empresaID, "") {
		if !p.VencioAceptacion(hoy) {
			continue
		}
		if _, err := s.cerrarCon(empresaID, p.ID, pedido.EstadoRechazado, "sistema", "vencimiento",
			"venció la ventana de aceptación del canal"); err == nil {
			n++
		}
	}
	return n
}

/* --- Maestros del módulo -------------------------------------------------- */

// PedidoDe expone un pedido por id.
func (s *Service) PedidoDe(empresaID, id string) (pedido.Pedido, bool) {
	return s.pedidoDe(empresaID, id)
}

// CanalesPedido lista los canales conectados.
func (s *Service) CanalesPedido(empresaID string) []pedido.Canal {
	if s.canalesPedido == nil {
		return []pedido.Canal{}
	}
	return s.canalesPedido.List(empresaID)
}

// GuardarCanalPedido crea o actualiza un canal.
func (s *Service) GuardarCanalPedido(empresaID, actor, origen string, c pedido.Canal) (pedido.Canal, error) {
	if s.canalesPedido == nil {
		return pedido.Canal{}, ErrPedidosNoDisponible
	}
	c.EmpresaID = empresaID
	c.Normalizar()
	if c.Nombre == "" {
		return pedido.Canal{}, errors.New("el canal necesita un nombre")
	}
	if c.Creado == "" {
		c.Creado = ahora()
	}
	c.Actualizado = ahora()
	out := s.canalesPedido.Upsert(c)
	s.audit.Append(evento(empresaID, actor, origen, "pedido.canal.guardar", out.Nombre,
		fmt.Sprintf("auto=%v envio_del_canal=%v", out.ConfirmacionAutomatica, out.EnvioPropioDelCanal)))
	return out, nil
}

// ZonasPedido lista las zonas de reparto de una sede.
func (s *Service) ZonasPedido(empresaID, sedeID string) []pedido.Zona {
	if s.zonasPedido == nil {
		return []pedido.Zona{}
	}
	z := s.zonasPedido.List(empresaID, sedeID)
	// De la más chica a la más grande: es el orden en que se evalúan, así que
	// mostrarlas al revés haría pensar que gana la que no gana.
	sort.SliceStable(z, func(i, j int) bool { return z[i].RadioM < z[j].RadioM })
	return z
}

// GuardarZonaPedido crea o actualiza una zona.
func (s *Service) GuardarZonaPedido(empresaID, actor, origen string, z pedido.Zona) (pedido.Zona, error) {
	if s.zonasPedido == nil {
		return pedido.Zona{}, ErrPedidosNoDisponible
	}
	z.EmpresaID = empresaID
	z.Nombre = strings.TrimSpace(z.Nombre)
	if z.Nombre == "" {
		return pedido.Zona{}, errors.New("la zona necesita un nombre")
	}
	if z.RadioM <= 0 {
		return pedido.Zona{}, errors.New("la zona necesita un radio en metros")
	}
	out := s.zonasPedido.Upsert(z)
	s.audit.Append(evento(empresaID, actor, origen, "pedido.zona.guardar", out.Nombre,
		fmt.Sprintf("%d m · envío %.2f", out.RadioM, out.CostoEnvio)))
	return out, nil
}

// Repartidores lista la flota de una sede.
func (s *Service) Repartidores(empresaID, sedeID string) []pedido.Repartidor {
	if s.repartidores == nil {
		return []pedido.Repartidor{}
	}
	return s.repartidores.List(empresaID, sedeID)
}

// GuardarRepartidor crea o actualiza un repartidor.
func (s *Service) GuardarRepartidor(empresaID, actor, origen string, r pedido.Repartidor) (pedido.Repartidor, error) {
	if s.repartidores == nil {
		return pedido.Repartidor{}, ErrPedidosNoDisponible
	}
	r.EmpresaID = empresaID
	r.Nombre = strings.TrimSpace(r.Nombre)
	if r.Nombre == "" {
		return pedido.Repartidor{}, errors.New("el repartidor necesita un nombre")
	}
	if r.Creado == "" {
		r.Creado = ahora()
	}
	out := s.repartidores.Upsert(r)
	s.audit.Append(evento(empresaID, actor, origen, "pedido.repartidor.guardar", out.Nombre, out.Codigo))
	return out, nil
}
