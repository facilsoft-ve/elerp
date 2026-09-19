package application

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mornix/elerp/internal/domain/compra"
)

// Errores de negocio del submódulo Solicitudes de presupuesto (RFQ).
var (
	ErrSolicitudNoExiste     = errors.New("solicitud de presupuesto no existe")
	ErrSolicitudVacia        = errors.New("la solicitud no tiene líneas")
	ErrSolicitudSinProv      = errors.New("la solicitud no indica proveedores a los que pedir presupuesto")
	ErrSolicitudEstado       = errors.New("la solicitud no está en un estado que permita esta acción")
	ErrProvNoInvitado        = errors.New("el proveedor no fue incluido en esta solicitud")
	ErrSolicitudSinPrecios   = errors.New("el proveedor no cotizó ningún precio: no hay nada que convertir en orden")
	ErrSolicitudNoDisponible = errors.New("el registro de solicitudes de presupuesto no está disponible")
	// ErrModalidadInvalida: la modalidad no es una de las admitidas.
	ErrModalidadInvalida = errors.New("la modalidad debe ser adjudicación directa, licitación o lista de precios")
	// ErrLicitacionUnProveedor: una licitación con un solo invitado no es una
	// licitación. Si de verdad es uno solo, la modalidad es adjudicación directa.
	ErrLicitacionUnProveedor = errors.New("una licitación necesita al menos dos proveedores a quienes pedir presupuesto")
	// ErrDirectaVariosProveedores: si se le pide a varios hubo concurso, no
	// adjudicación directa. El documento no puede decir lo contrario de lo que hace.
	ErrDirectaVariosProveedores = errors.New("una adjudicación directa es a un solo proveedor: con varios, la modalidad es licitación")
	// ErrSinListaDeProveedor: la modalidad es lista de precios pero ese proveedor no
	// tiene tarifa de compra activa.
	ErrSinListaDeProveedor = errors.New("ese proveedor no tiene una lista de precios de compra activa")
	// ErrSKUSinTarifa: la tarifa del proveedor no cubre un producto de la solicitud.
	ErrSKUSinTarifa = errors.New("la lista de precios del proveedor no tiene tarifa para este producto")
)

// serieSolicitud es la correlativa de las solicitudes de presupuesto (no es fiscal).
const serieSolicitud = "SOL"

// LineaSolicitudEntrada es un renglón al crear/editar una solicitud: solo qué
// producto y cuánto se pide cotizar (el precio lo pone cada proveedor).
type LineaSolicitudEntrada struct {
	SKU      string
	Cantidad float64
}

// EntradaSolicitud son los datos para crear/editar una solicitud de presupuesto.
type EntradaSolicitud struct {
	SedeID string
	Notas  string
	// Modalidad: cómo se elige al proveedor (ver compra.Modalidad*). Vacío ⇒
	// adjudicación directa.
	Modalidad   string
	Lineas      []LineaSolicitudEntrada
	Proveedores []string // ids de proveedores a los que se pide presupuesto
}

// modalidadResuelta decide qué modalidad se guarda. Si quien crea la solicitud la
// DECLARA, esa manda (y se valida contra los hechos). Si no dice nada, se DEDUCE de
// a cuántos se les pide: a varios es un concurso, a uno es directa.
//
// Deducir en vez de exigir es lo que mantiene compatible el API y las solicitudes
// que ya existían: nadie tiene que empezar a mandar un campo nuevo, y el documento
// igual queda diciendo lo que de verdad pasó.
func modalidadResuelta(declarada string, n int) string {
	if declarada != "" {
		return declarada
	}
	if n >= 2 {
		return compra.ModalidadLicitacion
	}
	return compra.ModalidadDirecta
}

// validarModalidadConProveedores comprueba que una modalidad DECLARADA cuadre con a
// cuántos se les está pidiendo. Es lo que impide que el documento mienta: una
// «adjudicación directa» con cuatro invitados fue un concurso, y una «licitación»
// con uno no lo fue. Sobre una modalidad deducida no aplica — se dedujo de los
// hechos, así que no puede contradecirlos.
func validarModalidadConProveedores(declarada string, n int) error {
	switch declarada {
	case compra.ModalidadLicitacion:
		if n < 2 {
			return ErrLicitacionUnProveedor
		}
	case compra.ModalidadDirecta, compra.ModalidadListaPrecios:
		if n > 1 {
			return ErrDirectaVariosProveedores
		}
	}
	return nil
}

// RespuestaLineaEntrada es el precio que un proveedor cotiza para un SKU.
type RespuestaLineaEntrada struct {
	SKU            string
	PrecioUnitario float64
}

// ConSolicitudesCompra cablea el repositorio de solicitudes de presupuesto. Se
// configura aparte de New (como ConListasPrecio / ConNotasCompra) para no romper
// las firmas de los constructores ya cableados en cmd/api; sin él, el servicio
// funciona igual y las solicitudes quedan vacías.
func (s *Service) ConSolicitudesCompra(r compra.SolicitudCompraRepo) *Service {
	s.solicitudes = r
	return s
}

// SolicitudesCompra lista las solicitudes de presupuesto de la empresa.
func (s *Service) SolicitudesCompra(empresaID string) []compra.SolicitudCompra {
	if s.solicitudes == nil {
		return []compra.SolicitudCompra{}
	}
	return s.solicitudes.List(empresaID)
}

// SolicitudCompra devuelve una solicitud de presupuesto por id.
func (s *Service) SolicitudCompra(empresaID, id string) (compra.SolicitudCompra, bool) {
	if s.solicitudes == nil {
		return compra.SolicitudCompra{}, false
	}
	return s.solicitudes.ByID(empresaID, id)
}

// armarLineasSolicitud construye las líneas desde el catálogo (enriqueciendo
// nombre/productoID) validando que cada SKU exista y que la cantidad sea > 0.
func (s *Service) armarLineasSolicitud(empresaID string, in []LineaSolicitudEntrada) ([]compra.LineaSolicitud, error) {
	lineas := make([]compra.LineaSolicitud, 0, len(in))
	vistos := map[string]bool{}
	for _, l := range in {
		sku := strings.TrimSpace(l.SKU)
		if sku == "" || vistos[sku] {
			continue
		}
		if l.Cantidad <= 0 {
			return nil, fmt.Errorf("la cantidad de %s debe ser mayor a 0", sku)
		}
		p, ok := s.productos.BySKU(empresaID, sku)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrProductoNoExiste, sku)
		}
		// Un combo no se cotiza ni se compra: se piden presupuestos de sus componentes.
		if p.EsCombo {
			return nil, fmt.Errorf("%w: %s", ErrComboEnCompra, sku)
		}
		vistos[sku] = true
		lineas = append(lineas, compra.LineaSolicitud{
			ProductoID: p.ID, SKU: p.SKU, Nombre: p.Nombre, Cantidad: l.Cantidad,
		})
	}
	return lineas, nil
}

// armarProveedoresSolicitud construye la lista de proveedores invitados desde sus
// ids, validando que cada uno exista y deduplicando. Todos arrancan pendientes.
func (s *Service) armarProveedoresSolicitud(empresaID string, ids []string) ([]compra.ProveedorCotiza, error) {
	out := make([]compra.ProveedorCotiza, 0, len(ids))
	vistos := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || vistos[id] {
			continue
		}
		prov, ok := s.provs.ByID(empresaID, id)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrProveedorNoExiste, id)
		}
		vistos[id] = true
		out = append(out, compra.ProveedorCotiza{
			ProveedorID: prov.ID, ProveedorNombre: prov.Nombre,
			Estado: compra.CotizaPendiente, Respondida: false, Lineas: []compra.RespuestaLinea{},
		})
	}
	return out, nil
}

// CrearSolicitud arma una solicitud de presupuesto en borrador con folio de la
// serie "SOL". Valida que haya al menos una línea; los proveedores son opcionales
// al crear (se pueden agregar antes de enviar).
func (s *Service) CrearSolicitud(empresaID, sedeID, actor, origen string, in EntradaSolicitud) (compra.SolicitudCompra, error) {
	if s.solicitudes == nil {
		return compra.SolicitudCompra{}, ErrSolicitudNoDisponible
	}
	lineas, err := s.armarLineasSolicitud(empresaID, in.Lineas)
	if err != nil {
		return compra.SolicitudCompra{}, err
	}
	if len(lineas) == 0 {
		return compra.SolicitudCompra{}, ErrSolicitudVacia
	}
	if !compra.ModalidadValida(in.Modalidad) {
		return compra.SolicitudCompra{}, ErrModalidadInvalida
	}
	provs, err := s.armarProveedoresSolicitud(empresaID, in.Proveedores)
	if err != nil {
		return compra.SolicitudCompra{}, err
	}
	// En borrador los proveedores son opcionales, así que la coherencia con la
	// modalidad solo se exige si ya hay alguno; el control duro es al enviar.
	if len(provs) > 0 {
		if err := validarModalidadConProveedores(in.Modalidad, len(provs)); err != nil {
			return compra.SolicitudCompra{}, err
		}
	}
	modalidad := modalidadResuelta(in.Modalidad, len(provs))
	sede := in.SedeID
	if sede == "" {
		sede = sedeID
	}
	sol := compra.SolicitudCompra{
		EmpresaID: empresaID, SedeID: sede,
		Serie: serieSolicitud, Estado: compra.SolBorrador,
		Modalidad: modalidad,
		Fecha:     ahora(), Notas: strings.TrimSpace(in.Notas),
		Lineas: lineas, Proveedores: provs,
		Actor: actor, Creada: ahora(), Actualizada: ahora(),
	}
	sol.Numero = s.numerador.Siguiente(empresaID, sede, serieSolicitud)
	sol.NumeroCompleto = fmt.Sprintf("%s-%06d", serieSolicitud, sol.Numero)

	out := s.solicitudes.Create(sol)
	s.audit.Append(evento(empresaID, actor, origen, "compras.solicitud.crear", out.NumeroCompleto, ""))
	return out, nil
}

// ActualizarSolicitud edita una solicitud en BORRADOR: líneas, proveedores y
// notas. Una vez enviada ya no se editan sus líneas ni proveedores (queda
// registrada). Los proveedores que ya existían conservan su respuesta si siguen
// invitados; los nuevos entran pendientes.
func (s *Service) ActualizarSolicitud(empresaID, id, actor, origen string, in EntradaSolicitud) (compra.SolicitudCompra, error) {
	if s.solicitudes == nil {
		return compra.SolicitudCompra{}, ErrSolicitudNoDisponible
	}
	sol, ok := s.solicitudes.ByID(empresaID, id)
	if !ok {
		return compra.SolicitudCompra{}, ErrSolicitudNoExiste
	}
	if sol.Estado != compra.SolBorrador {
		return compra.SolicitudCompra{}, ErrSolicitudEstado
	}
	lineas, err := s.armarLineasSolicitud(empresaID, in.Lineas)
	if err != nil {
		return compra.SolicitudCompra{}, err
	}
	if len(lineas) == 0 {
		return compra.SolicitudCompra{}, ErrSolicitudVacia
	}
	nuevos, err := s.armarProveedoresSolicitud(empresaID, in.Proveedores)
	if err != nil {
		return compra.SolicitudCompra{}, err
	}
	// Conserva la respuesta de un proveedor que siga invitado (no se pierde por editar).
	previos := map[string]compra.ProveedorCotiza{}
	for _, p := range sol.Proveedores {
		previos[p.ProveedorID] = p
	}
	for i, p := range nuevos {
		if ya, existe := previos[p.ProveedorID]; existe {
			nuevos[i] = ya
		}
	}
	if !compra.ModalidadValida(in.Modalidad) {
		return compra.SolicitudCompra{}, ErrModalidadInvalida
	}
	if len(nuevos) > 0 {
		if err := validarModalidadConProveedores(in.Modalidad, len(nuevos)); err != nil {
			return compra.SolicitudCompra{}, err
		}
	}
	sol.Modalidad = modalidadResuelta(in.Modalidad, len(nuevos))
	sol.Lineas = lineas
	sol.Proveedores = nuevos
	sol.Notas = strings.TrimSpace(in.Notas)
	sol.Actualizada = ahora()

	out, ok := s.solicitudes.Update(sol)
	if !ok {
		return compra.SolicitudCompra{}, ErrSolicitudNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "compras.solicitud.actualizar", out.NumeroCompleto, ""))
	return out, nil
}

// EnviarSolicitud registra el envío de la solicitud a los proveedores: pasa de
// borrador a enviada y a partir de ahí queda a la espera de respuestas. Exige al
// menos un proveedor al que pedir presupuesto.
func (s *Service) EnviarSolicitud(empresaID, id, actor, origen string) (compra.SolicitudCompra, error) {
	if s.solicitudes == nil {
		return compra.SolicitudCompra{}, ErrSolicitudNoDisponible
	}
	sol, ok := s.solicitudes.ByID(empresaID, id)
	if !ok {
		return compra.SolicitudCompra{}, ErrSolicitudNoExiste
	}
	if sol.Estado != compra.SolBorrador {
		return compra.SolicitudCompra{}, ErrSolicitudEstado
	}
	if len(sol.Proveedores) == 0 {
		return compra.SolicitudCompra{}, ErrSolicitudSinProv
	}
	// Control duro: acá ya no hay excusa de «todavía la estoy armando». Lo que el
	// documento declare tiene que cuadrar con a cuántos se les pidió.
	if err := validarModalidadConProveedores(sol.Modalidad, len(sol.Proveedores)); err != nil {
		return compra.SolicitudCompra{}, err
	}
	// En modalidad lista de precios no se le pide presupuesto a nadie: se compra a la
	// tarifa ya pactada. Sin tarifa activa no hay de dónde sacar los costos, y
	// enviarla sería prometer una conversión que después falla.
	if sol.Modalidad == compra.ModalidadListaPrecios {
		if _, ok := s.ListaDeCompraDe(empresaID, sol.Proveedores[0].ProveedorID); !ok {
			return compra.SolicitudCompra{}, ErrSinListaDeProveedor
		}
	}
	sol.Estado = compra.SolEnviada
	sol.Actualizada = ahora()
	out, ok := s.solicitudes.Update(sol)
	if !ok {
		return compra.SolicitudCompra{}, ErrSolicitudNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "compras.solicitud.enviar", out.NumeroCompleto, out.Estado))
	return out, nil
}

// RegistrarRespuesta carga los precios que respondió un proveedor invitado a la
// solicitud. Solo se admite si la solicitud ya se envió (enviada o respondida) y
// el proveedor fue incluido. Calcula el total del proveedor (precio × cantidad de
// cada SKU cotizado) para poder comparar, y mueve la solicitud a "respondida".
func (s *Service) RegistrarRespuesta(empresaID, id, actor, origen, proveedorID string, lineas []RespuestaLineaEntrada) (compra.SolicitudCompra, error) {
	if s.solicitudes == nil {
		return compra.SolicitudCompra{}, ErrSolicitudNoDisponible
	}
	sol, ok := s.solicitudes.ByID(empresaID, id)
	if !ok {
		return compra.SolicitudCompra{}, ErrSolicitudNoExiste
	}
	if sol.Estado != compra.SolEnviada && sol.Estado != compra.SolRespondida {
		return compra.SolicitudCompra{}, ErrSolicitudEstado
	}
	idx := -1
	for i, p := range sol.Proveedores {
		if p.ProveedorID == proveedorID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return compra.SolicitudCompra{}, ErrProvNoInvitado
	}
	// Cantidad pedida por SKU (para valorizar la respuesta). Solo se aceptan
	// precios de SKU que están en la solicitud; los demás se ignoran.
	cantPorSKU := map[string]float64{}
	for _, l := range sol.Lineas {
		cantPorSKU[l.SKU] = l.Cantidad
	}
	resp := make([]compra.RespuestaLinea, 0, len(lineas))
	var total float64
	vistos := map[string]bool{}
	for _, l := range lineas {
		sku := strings.TrimSpace(l.SKU)
		cant, ok := cantPorSKU[sku]
		if !ok || vistos[sku] || l.PrecioUnitario < 0 {
			continue
		}
		vistos[sku] = true
		precio := round2(l.PrecioUnitario)
		resp = append(resp, compra.RespuestaLinea{SKU: sku, PrecioUnitario: precio})
		total += precio * cant
	}
	sol.Proveedores[idx].Lineas = resp
	sol.Proveedores[idx].Total = round2(total)
	sol.Proveedores[idx].Respondida = len(resp) > 0
	sol.Proveedores[idx].Estado = compra.CotizaPendiente
	if len(resp) > 0 {
		sol.Proveedores[idx].Estado = compra.CotizaRespondida
		sol.Proveedores[idx].RespondidaEn = ahora()
	}
	// La solicitud queda "respondida" en cuanto algún proveedor tiene respuesta.
	if sol.Estado == compra.SolEnviada && sol.Proveedores[idx].Respondida {
		sol.Estado = compra.SolRespondida
	}
	sol.Actualizada = ahora()
	out, ok := s.solicitudes.Update(sol)
	if !ok {
		return compra.SolicitudCompra{}, ErrSolicitudNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "compras.solicitud.respuesta", out.NumeroCompleto, sol.Proveedores[idx].ProveedorNombre))
	return out, nil
}

// CerrarSolicitud cierra la negociación a mano (sin convertir a orden): pasa a
// "cerrada". Se admite desde borrador, enviada o respondida.
func (s *Service) CerrarSolicitud(empresaID, id, actor, origen string) (compra.SolicitudCompra, error) {
	if s.solicitudes == nil {
		return compra.SolicitudCompra{}, ErrSolicitudNoDisponible
	}
	sol, ok := s.solicitudes.ByID(empresaID, id)
	if !ok {
		return compra.SolicitudCompra{}, ErrSolicitudNoExiste
	}
	switch sol.Estado {
	case compra.SolBorrador, compra.SolEnviada, compra.SolRespondida:
		// permitido
	default:
		return compra.SolicitudCompra{}, ErrSolicitudEstado
	}
	sol.Estado = compra.SolCerrada
	sol.Actualizada = ahora()
	out, ok := s.solicitudes.Update(sol)
	if !ok {
		return compra.SolicitudCompra{}, ErrSolicitudNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "compras.solicitud.cerrar", out.NumeroCompleto, out.Estado))
	return out, nil
}

// ConvertirEnOrden crea una ORDEN DE COMPRA a partir de la solicitud usando las
// líneas y los precios que cotizó el proveedor elegido. Reusa CrearOrdenCompra
// (no reimplementa la OC): la orden nace en borrador con su propio folio "OC" y su
// flujo de recepción/asiento intacto. La solicitud queda "cerrada", enlazada a la
// orden generada. Exige que el proveedor haya respondido con al menos un precio.
func (s *Service) ConvertirEnOrden(empresaID, id, actor, origen, proveedorID string) (compra.SolicitudCompra, compra.OrdenCompra, error) {
	if s.solicitudes == nil {
		return compra.SolicitudCompra{}, compra.OrdenCompra{}, ErrSolicitudNoDisponible
	}
	sol, ok := s.solicitudes.ByID(empresaID, id)
	if !ok {
		return compra.SolicitudCompra{}, compra.OrdenCompra{}, ErrSolicitudNoExiste
	}
	if sol.Estado != compra.SolEnviada && sol.Estado != compra.SolRespondida {
		return compra.SolicitudCompra{}, compra.OrdenCompra{}, ErrSolicitudEstado
	}
	var cotiza *compra.ProveedorCotiza
	for i := range sol.Proveedores {
		if sol.Proveedores[i].ProveedorID == proveedorID {
			cotiza = &sol.Proveedores[i]
			break
		}
	}
	if cotiza == nil {
		return compra.SolicitudCompra{}, compra.OrdenCompra{}, ErrProvNoInvitado
	}
	// De dónde sale el precio depende de la MODALIDAD.
	//
	//   - Lista de precios: de la tarifa ya pactada con el proveedor. No se le pidió
	//     presupuesto, así que exigir una respuesta no tendría sentido; lo que se
	//     exige es que la tarifa cubra lo que se pide, y se dice qué falta.
	//   - Directa y licitación: de lo que el proveedor cotizó.
	precioPorSKU := map[string]float64{}
	if sol.Modalidad == compra.ModalidadListaPrecios {
		if _, hay := s.ListaDeCompraDe(empresaID, proveedorID); !hay {
			return compra.SolicitudCompra{}, compra.OrdenCompra{}, ErrSinListaDeProveedor
		}
		for _, l := range sol.Lineas {
			precio, ok := s.CostoPactadoCon(empresaID, proveedorID, l.SKU)
			if !ok {
				return compra.SolicitudCompra{}, compra.OrdenCompra{},
					fmt.Errorf("%w: %s", ErrSKUSinTarifa, l.SKU)
			}
			precioPorSKU[l.SKU] = precio
		}
	} else {
		if !cotiza.Respondida || len(cotiza.Lineas) == 0 {
			return compra.SolicitudCompra{}, compra.OrdenCompra{}, ErrSolicitudSinPrecios
		}
		// Solo se llevan a la orden las líneas que el proveedor coticó (con precio).
		// Las cantidades salen siempre de la solicitud.
		for _, l := range cotiza.Lineas {
			precioPorSKU[l.SKU] = l.PrecioUnitario
		}
	}
	entrada := EntradaOC{
		ProveedorID: proveedorID, SedeID: sol.SedeID,
		Notas: fmt.Sprintf("Generada desde la solicitud de presupuesto %s", sol.NumeroCompleto),
	}
	for _, l := range sol.Lineas {
		precio, ok := precioPorSKU[l.SKU]
		if !ok {
			continue
		}
		entrada.Lineas = append(entrada.Lineas, LineaOCEntrada{
			SKU: l.SKU, Cantidad: l.Cantidad, CostoUnitario: precio,
		})
	}
	if len(entrada.Lineas) == 0 {
		return compra.SolicitudCompra{}, compra.OrdenCompra{}, ErrSolicitudSinPrecios
	}
	oc, err := s.CrearOrdenCompra(empresaID, sol.SedeID, actor, origen, entrada)
	if err != nil {
		return compra.SolicitudCompra{}, compra.OrdenCompra{}, err
	}
	sol.Estado = compra.SolCerrada
	sol.ProveedorElegidoID = proveedorID
	sol.OrdenGeneradaID = oc.ID
	sol.Actualizada = ahora()
	out, ok := s.solicitudes.Update(sol)
	if !ok {
		return compra.SolicitudCompra{}, compra.OrdenCompra{}, ErrSolicitudNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "compras.solicitud.convertir", out.NumeroCompleto, oc.NumeroCompleto))
	return out, oc, nil
}
