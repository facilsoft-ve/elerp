package application

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/mornix/elerp/internal/domain/cotizacion"
	"github.com/mornix/elerp/internal/domain/cuenta"
	"github.com/mornix/elerp/internal/domain/mesa"
)

// PREFACTURA DE UNA MESA
//
// Cuando el comensal pide la cuenta, el mesonero la PREFACTURA: la cuenta de mesa se
// convierte en una (o varias) cotizaciones CONFIRMADAS, rotuladas con la mesa. El cajero
// las encuentra en Ventas › Confirmadas —el cajero no conoce números de cotización,
// conoce mesas— y al cobrar emite la factura por la MISMA ruta que el mostrador
// (FacturarCotizacion → EmitirFactura: inventario, IGTF, vuelto y asiento contable).
//
// La solicitud es INCREMENTAL: el mesonero puede pedir factura de una parte de la mesa
// (los renglones de quien se va), y más tarde de otra. Cada solicitud se lleva SUS
// renglones —quedan marcados con su PrefacturaID— y los que nadie pidió siguen
// disponibles para la siguiente. La mesa se cierra sola cuando no queda consumo sin
// pedir Y todas las solicitudes están facturadas: así el cajero nunca tiene que
// decidir si cierra la mesa o no.
//
// Cada solicitud puede llevar además el CLIENTE ya identificado (el mesonero le tomó
// los datos en la mesa) o venir sin él, para que el cajero los pida al cobrar.
//
// Dividir la cuenta tiene DOS naturalezas distintas y por eso se resuelven distinto:
//
//   - PARTES IGUALES: es un reparto del COBRO, no del documento. Se emite UNA prefactura
//     con los renglones enteros y el cajero registra un pago por comensal. Así la factura
//     sale limpia ("3 × Spaghetti", no "1,5 ×"), el inventario se descuenta una sola vez y
//     el IGTF se calcula bien sobre la parte pagada en divisas.
//   - POR PRODUCTOS: cada comensal se lleva SUS renglones, así que sí son documentos
//     distintos: una prefactura por parte y, después, una factura por parte (es el caso
//     del que necesita su propia factura con su RIF).

var (
	ErrCuentaSinItems         = errors.New("la cuenta no tiene renglones para prefacturar")
	ErrCuentaYaPrefacturada   = errors.New("la cuenta ya tiene prefactura: anulala para volver a armarla")
	ErrNadaPorFacturar        = errors.New("no queda consumo sin facturar en esta mesa")
	ErrItemYaFacturado        = errors.New("ese renglón ya entró en una solicitud de facturación")
	ErrDivisionInvalida       = errors.New("la división de la cuenta no es válida")
	ErrParteSinItems          = errors.New("cada parte de la cuenta necesita al menos un renglón")
	ErrPrefacturaNoDisponible = errors.New("las prefacturas no están disponibles")
)

// Modos de división.
const (
	// DivisionUnica: una sola prefactura. Si `Comensales` > 1, se deja anotado para que
	// el cajero reparta el cobro en esa cantidad de pagos.
	DivisionUnica = "unica"
	// DivisionPorItems: una prefactura por parte, con los renglones que se le asignaron.
	DivisionPorItems = "por_items"
)

// DivisionCuenta describe cómo repartir la cuenta al prefacturar.
type DivisionCuenta struct {
	Modo string
	// Comensales es entre cuántos se reparte el cobro en el modo único (informativo
	// para el cajero: no divide el documento).
	Comensales int
	// Items asigna cada renglón a un número de parte (1..N) en el modo por productos.
	// Clave: id del renglón de la cuenta.
	Items map[string]int
	// Nombres rotula cada parte ("Ana", "Luis"). Opcional.
	Nombres map[int]string
	// Seleccion limita ESTA solicitud a esos renglones (ids de la cuenta). Vacía =
	// entra todo lo que todavía no se pidió. Es lo que permite segmentar la mesa:
	// "esto lo paga él" ahora, el resto después.
	Seleccion []string
	// ClienteID identifica a quien paga ESTA solicitud cuando el mesonero ya tomó
	// los datos en la mesa. Vacío = los pone el cajero al cobrar.
	ClienteID string
}

// PrefacturarCuenta convierte la cuenta de una mesa en prefacturas confirmadas.
func (s *Service) PrefacturarCuenta(empresaID, cuentaID, actor, origen string, div DivisionCuenta) (cuenta.Cuenta, []cotizacion.Cotizacion, error) {
	if s.cotizaciones == nil {
		return cuenta.Cuenta{}, nil, ErrPrefacturaNoDisponible
	}
	c, err := s.cuentaAbierta(empresaID, cuentaID)
	if err != nil {
		return cuenta.Cuenta{}, nil, err
	}
	// Disponibles = lo vivo que todavía no se llevó ninguna solicitud. Los cancelados
	// no se cobran y los ya pedidos son de otra solicitud.
	disponibles := c.ItemsSinFacturar()
	if len(c.Items) == 0 {
		return cuenta.Cuenta{}, nil, ErrCuentaSinItems
	}
	if len(disponibles) == 0 {
		return cuenta.Cuenta{}, nil, ErrNadaPorFacturar
	}
	vivos, err := seleccionarItems(disponibles, div.Seleccion)
	if err != nil {
		return cuenta.Cuenta{}, nil, err
	}

	grupos, err := agruparPorParte(vivos, div)
	if err != nil {
		return cuenta.Cuenta{}, nil, err
	}

	creadas := make([]cotizacion.Cotizacion, 0, len(grupos))
	ids := make([]string, 0, len(grupos))
	for _, g := range grupos {
		lineas := make([]LineaEntrada, 0, len(g.items))
		for _, it := range g.items {
			// El precio ya está en Bs y es autoritativo (lo resolvió la comandera con
			// la tasa del momento, igual que el POS): se pasa tal cual.
			lineas = append(lineas, LineaEntrada{
				SKU: it.SKU, Cantidad: it.Cantidad, PrecioUnitario: it.PrecioUnitario,
			})
		}
		cot, err := s.CrearCotizacion(empresaID, c.SedeID, actor, origen, EntradaCotizacion{
			Lineas: lineas, CondicionesPago: "Contado",
			Notas:        g.nota(c.MesaNombre, len(grupos), div),
			CuentaMesaID: c.ID, MesaNombre: c.MesaNombre,
			// Si el mesonero ya tomó los datos en la mesa, la solicitud llega
			// identificada y el cajero no pregunta nada.
			ClienteID: strings.TrimSpace(div.ClienteID),
		})
		if err != nil {
			return cuenta.Cuenta{}, nil, err
		}
		// Confirmada = prefactura: ya no se edita y el cajero la puede cobrar.
		conf, err := s.ConfirmarCotizacion(empresaID, cot.ID, actor, origen)
		if err != nil {
			return cuenta.Cuenta{}, nil, err
		}
		creadas = append(creadas, conf)
		ids = append(ids, conf.ID)
		// Los renglones de esta parte quedan marcados: ya no entran en otra solicitud.
		for _, it := range g.items {
			for i := range c.Items {
				if c.Items[i].ID == it.ID {
					c.Items[i].PrefacturaID = conf.ID
					break
				}
			}
		}
	}

	// Se ACUMULAN: una mesa puede pedir factura varias veces (uno se va antes).
	c.Prefacturas = append(c.Prefacturas, ids...)
	if c.PrefacturadaEn == "" {
		c.PrefacturadaEn = ahora()
	}
	actualizada, ok := s.cuentasMesa.Update(c)
	if !ok {
		return cuenta.Cuenta{}, nil, ErrCuentaMesaNoExiste
	}
	// La mesa pasa a «por cobrar»: el tablero lo muestra y la caja sabe que va a cobrar.
	s.syncMesaEstado(empresaID, c.MesaID, mesa.EstadoPorCobrar)
	s.audit.Append(evento(empresaID, actor, origen, "restaurante.cuenta.prefacturar", c.ID,
		fmt.Sprintf("mesa %s · %d prefactura(s)", c.MesaNombre, len(creadas))))
	return actualizada, creadas, nil
}

// CancelarPrefacturasCuenta anula las prefacturas de una cuenta y la devuelve a servicio.
// Existe porque agregar un postre después de pedir la cuenta es normal: sin esto habría
// que cerrar la mesa y volver a montar el pedido.
func (s *Service) CancelarPrefacturasCuenta(empresaID, cuentaID, actor, origen string) (cuenta.Cuenta, error) {
	c, err := s.cuentaAbierta(empresaID, cuentaID)
	if err != nil {
		return cuenta.Cuenta{}, err
	}
	if len(c.Prefacturas) == 0 {
		return c, nil
	}
	// Se anulan las solicitudes PENDIENTES y sus renglones vuelven a estar disponibles.
	// Las ya facturadas se quedan: su documento fiscal existe y no se deshace por acá
	// (para eso está la nota de crédito). Así una mesa que pagó una parte y sigue
	// comiendo puede volver a servicio sin tocar lo ya cobrado.
	quedan := make([]string, 0, len(c.Prefacturas))
	anuladas := make(map[string]bool, len(c.Prefacturas))
	for _, id := range c.Prefacturas {
		cot, ok := s.cotizaciones.ByID(empresaID, id)
		if !ok {
			continue
		}
		if cot.Estado == cotizacion.EstadoFacturada {
			quedan = append(quedan, id)
			continue
		}
		if _, err := s.CancelarCotizacion(empresaID, id, actor, origen, "la mesa volvió a servicio"); err != nil {
			return cuenta.Cuenta{}, err
		}
		anuladas[id] = true
	}
	if len(anuladas) == 0 {
		return c, nil
	}
	for i := range c.Items {
		if anuladas[c.Items[i].PrefacturaID] {
			c.Items[i].PrefacturaID = ""
		}
	}
	c.Prefacturas = quedan
	if len(quedan) == 0 {
		c.PrefacturadaEn = ""
	}
	actualizada, ok := s.cuentasMesa.Update(c)
	if !ok {
		return cuenta.Cuenta{}, ErrCuentaMesaNoExiste
	}
	s.syncMesaEstado(empresaID, c.MesaID, mesa.EstadoOcupada)
	s.audit.Append(evento(empresaID, actor, origen, "restaurante.cuenta.prefactura.anular", c.ID, "mesa "+c.MesaNombre))
	return actualizada, nil
}

// cerrarCuentaSiPrefacturasFacturadas cierra la cuenta y libera la mesa cuando TODAS sus
// prefacturas quedaron facturadas. Lo llama FacturarCotizacion: es lo que completa el
// circuito mesonero → caja sin que nadie tenga que cerrar la mesa a mano.
func (s *Service) cerrarCuentaSiPrefacturasFacturadas(empresaID, cuentaMesaID, documentoID, actor, origen string) {
	if cuentaMesaID == "" || s.cuentasMesa == nil {
		return
	}
	c, ok := s.cuentasMesa.ByID(empresaID, cuentaMesaID)
	if !ok || c.Estado != cuenta.EstadoAbierta {
		return
	}
	// Queda consumo que nadie pidió: la mesa sigue viva aunque lo emitido ya se cobró.
	if c.TieneSinFacturar() {
		return
	}
	for _, id := range c.Prefacturas {
		cot, ok := s.cotizaciones.ByID(empresaID, id)
		if !ok || cot.Estado != cotizacion.EstadoFacturada {
			return // todavía falta cobrar alguna parte
		}
	}
	c.Estado = cuenta.EstadoCerrada
	c.Cerrada = ahora()
	if c.DocumentoID == "" {
		c.DocumentoID = documentoID
	}
	if _, ok := s.cuentasMesa.Update(c); !ok {
		return
	}
	s.syncMesaEstado(empresaID, c.MesaID, mesa.EstadoLibre)
	s.audit.Append(evento(empresaID, actor, origen, "restaurante.cuenta.cerrar", c.ID,
		"mesa "+c.MesaNombre+" cobrada"))
}

// --- Reparto de renglones ---

// seleccionarItems reduce los renglones disponibles a los que pide la solicitud.
// Sin selección entra todo lo disponible (el caso normal: "la cuenta, por favor").
// Un id que no está disponible es un error explícito y no un renglón que se ignora
// en silencio: significa que la comandera y el servidor no ven lo mismo.
func seleccionarItems(disponibles []cuenta.Item, seleccion []string) ([]cuenta.Item, error) {
	if len(seleccion) == 0 {
		return disponibles, nil
	}
	porID := make(map[string]cuenta.Item, len(disponibles))
	for _, it := range disponibles {
		porID[it.ID] = it
	}
	vistos := make(map[string]bool, len(seleccion))
	out := make([]cuenta.Item, 0, len(seleccion))
	for _, id := range seleccion {
		id = strings.TrimSpace(id)
		if id == "" || vistos[id] {
			continue
		}
		it, ok := porID[id]
		if !ok {
			return nil, ErrItemYaFacturado
		}
		vistos[id] = true
		out = append(out, it)
	}
	if len(out) == 0 {
		return nil, ErrCuentaSinItems
	}
	return out, nil
}

type parteCuenta struct {
	numero int
	nombre string
	items  []cuenta.Item
}

// nota es lo que el cajero lee en la prefactura. Nombra la mesa siempre —es como la
// busca— y, en el modo único con varios comensales, cuánto toca por persona.
func (g parteCuenta) nota(mesaNombre string, total int, div DivisionCuenta) string {
	base := "Mesa " + mesaNombre
	if total > 1 {
		etiqueta := g.nombre
		if etiqueta == "" {
			etiqueta = fmt.Sprintf("parte %d de %d", g.numero, total)
		}
		return base + " · " + etiqueta
	}
	if div.Comensales > 1 {
		suma := 0.0
		for _, it := range g.items {
			suma += it.Cantidad * it.PrecioUnitario
		}
		// Referencia para el cajero: el total definitivo (con IVA) lo calcula la
		// cotización; acá se deja el reparto sobre el consumo para orientar el cobro.
		porPersona := math.Round(suma/float64(div.Comensales)*100) / 100
		return fmt.Sprintf("%s · pagan %d personas (≈ Bs %s cada una sobre el consumo)", base, div.Comensales, bolivares(porPersona))
	}
	return base
}

// agruparPorParte reparte los renglones según el modo de división.
func agruparPorParte(items []cuenta.Item, div DivisionCuenta) ([]parteCuenta, error) {
	modo := strings.TrimSpace(div.Modo)
	if modo == "" {
		modo = DivisionUnica
	}
	switch modo {
	case DivisionUnica:
		if div.Comensales < 0 {
			return nil, ErrDivisionInvalida
		}
		return []parteCuenta{{numero: 1, items: items}}, nil

	case DivisionPorItems:
		if len(div.Items) == 0 {
			return nil, ErrDivisionInvalida
		}
		porParte := map[int][]cuenta.Item{}
		for _, it := range items {
			parte, ok := div.Items[it.ID]
			if !ok || parte < 1 {
				// Un renglón sin asignar dejaría plata sin cobrar: es un error, no un
				// caso a resolver por omisión.
				return nil, fmt.Errorf("%w: el renglón %s no está asignado a ninguna parte", ErrDivisionInvalida, it.Nombre)
			}
			porParte[parte] = append(porParte[parte], it)
		}
		numeros := make([]int, 0, len(porParte))
		for n := range porParte {
			numeros = append(numeros, n)
		}
		sort.Ints(numeros)
		out := make([]parteCuenta, 0, len(numeros))
		for i, n := range numeros {
			if len(porParte[n]) == 0 {
				return nil, ErrParteSinItems
			}
			out = append(out, parteCuenta{numero: i + 1, nombre: div.Nombres[n], items: porParte[n]})
		}
		return out, nil
	}
	return nil, ErrDivisionInvalida
}

// bolivares escribe un monto como se lee en Venezuela: miles con punto y decimales con
// coma. Esta nota la lee el cajero en pantalla, y "17100.00" no es un monto en Bs.
func bolivares(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	ent := int64(v)
	dec := int64(math.Round((v - float64(ent)) * 100))
	if dec == 100 { // el redondeo empujó al siguiente entero
		ent++
		dec = 0
	}
	s := strconv.FormatInt(ent, 10)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	out := fmt.Sprintf("%s,%02d", b.String(), dec)
	if neg {
		return "-" + out
	}
	return out
}
