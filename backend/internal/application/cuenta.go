package application

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mornix/elerp/internal/domain/cuenta"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/mesa"
)

var (
	ErrCuentasNoDisponible = errors.New("las cuentas de mesa no están disponibles")
	ErrCuentaMesaNoExiste  = errors.New("la cuenta no existe")
	ErrCuentaCerrada       = errors.New("la cuenta ya está cerrada")
	ErrItemCuentaNoExiste  = errors.New("ese renglón no existe en la cuenta")
	ErrSinPendientes       = errors.New("no hay renglones nuevos para enviar a cocina")
	ErrItemSinCantidad     = errors.New("cada renglón necesita una cantidad mayor que cero")
)

// ConCuentas cablea el repositorio de cuentas de mesa (módulo Restaurante).
func (s *Service) ConCuentas(r cuenta.Repository) *Service {
	s.cuentasMesa = r
	return s
}

// CuentasAbiertas devuelve las cuentas abiertas de una sede (tablero de mesas),
// ordenadas por nombre de mesa.
func (s *Service) CuentasAbiertas(empresaID, sedeID string) []cuenta.Cuenta {
	if s.cuentasMesa == nil {
		return []cuenta.Cuenta{}
	}
	out := s.cuentasMesa.Abiertas(empresaID, sedeID)
	sort.SliceStable(out, func(i, j int) bool { return out[i].MesaNombre < out[j].MesaNombre })
	return out
}

// Cuenta devuelve una cuenta por id.
func (s *Service) Cuenta(empresaID, id string) (cuenta.Cuenta, bool) {
	if s.cuentasMesa == nil {
		return cuenta.Cuenta{}, false
	}
	return s.cuentasMesa.ByID(empresaID, id)
}

// syncMesaEstado fija el estado operativo de una mesa (ocupada/libre) según su
// cuenta. No falla si el maestro de mesas no está cableado.
func (s *Service) syncMesaEstado(empresaID, mesaID, estado string) {
	if s.mesas == nil || mesaID == "" {
		return
	}
	m, ok := s.mesas.ByID(empresaID, mesaID)
	if !ok || m.Estado == estado {
		return
	}
	m.Estado = estado
	s.mesas.Update(m)
}

// AperturaCuenta son los datos para abrir la cuenta de una mesa. Es un struct y no una
// lista de parámetros porque ya son ocho y el rol del actor se sumó después (lo necesita
// la regla de asignación de mesas).
type AperturaCuenta struct {
	EmpresaID string
	SedeID    string
	MesaID    string
	// MesoneroID/MesoneroNombre es quien queda como responsable de la cuenta.
	MesoneroID     string
	MesoneroNombre string
	// RolActor decide si aplica la asignación de mesas: solo limita al rol mesonero.
	RolActor   string
	Actor      string
	Origen     string
	Comensales int
}

// AbrirCuenta abre (o devuelve la ya abierta) la cuenta de una mesa. Marca la mesa
// como ocupada.
//
// Si la mesa está asignada a OTRO mesonero: con la configuración flexible (por defecto)
// se permite y queda en la bitácora; con la estricta se rechaza con
// ErrMesaDeOtroMesonero.
func (s *Service) AbrirCuenta(in AperturaCuenta) (cuenta.Cuenta, error) {
	empresaID, sedeID, mesaID := in.EmpresaID, in.SedeID, in.MesaID
	mesoneroID, mesoneroNombre := in.MesoneroID, in.MesoneroNombre
	actor, origen, comensales := in.Actor, in.Origen, in.Comensales
	if s.cuentasMesa == nil {
		return cuenta.Cuenta{}, ErrCuentasNoDisponible
	}
	nombre := mesaID
	if s.mesas != nil {
		m, ok := s.mesas.ByID(empresaID, mesaID)
		if !ok {
			return cuenta.Cuenta{}, ErrMesaNoExiste
		}
		nombre = m.Nombre
		if sedeID == "" {
			sedeID = m.SedeID
		}
		// La asignación se comprueba ANTES de devolver una cuenta ya abierta: si no,
		// en modo estricto se le entregaría al mesonero la cuenta de otro con solo
		// tocar la mesa, que es exactamente lo que el candado debe impedir.
		ajena, duenos, err := s.verificarMesaDelMesonero(empresaID, sedeID, mesoneroID, in.RolActor, m)
		if err != nil {
			return cuenta.Cuenta{}, err
		}
		if ajena {
			s.audit.Append(evento(empresaID, actor, origen, "restaurante.mesa.ajena", mesaID,
				"mesa "+nombre+" asignada a "+nombresDe(duenos)))
		}
	}
	if ex, ok := s.cuentasMesa.AbiertaDeMesa(empresaID, mesaID); ok {
		return ex, nil
	}
	if comensales < 0 {
		comensales = 0
	}
	c := cuenta.Cuenta{
		EmpresaID: empresaID, SedeID: sedeID, MesaID: mesaID, MesaNombre: nombre,
		Estado: cuenta.EstadoAbierta, MesoneroID: mesoneroID, MesoneroNombre: mesoneroNombre,
		Comensales: comensales, Items: []cuenta.Item{}, Abierta: ahora(),
	}
	out := s.cuentasMesa.Create(c)
	s.syncMesaEstado(empresaID, mesaID, mesa.EstadoOcupada)
	s.audit.Append(evento(empresaID, actor, origen, "restaurante.cuenta.abrir", out.ID, "mesa "+nombre))
	return out, nil
}

// ItemInput es un renglón a agregar a la cuenta (el front ya resolvió el precio en
// Bs, como el POS).
type ItemInput struct {
	SKU            string  `json:"sku"`
	Nombre         string  `json:"nombre"`
	Cantidad       float64 `json:"cantidad"`
	PrecioUnitario float64 `json:"precioUnitario"`
	Exento         bool    `json:"exento"`
	Nota           string  `json:"nota"`
}

// AgregarItems agrega renglones (estado "pendiente") a una cuenta abierta.
//
// `rolActor` aplica la misma regla de asignación que AbrirCuenta: sumarle renglones a la
// mesa de otro mesonero es tomarla igual que abrirla, así que no tendría sentido cuidar
// una puerta y dejar la otra abierta.
func (s *Service) AgregarItems(empresaID, cuentaID, actor, rolActor, origen string, items []ItemInput) (cuenta.Cuenta, error) {
	c, err := s.cuentaAbierta(empresaID, cuentaID)
	if err != nil {
		return cuenta.Cuenta{}, err
	}
	if s.mesas != nil {
		if m, ok := s.mesas.ByID(empresaID, c.MesaID); ok {
			ajena, duenos, err := s.verificarMesaDelMesonero(empresaID, c.SedeID, actor, rolActor, m)
			if err != nil {
				return cuenta.Cuenta{}, err
			}
			if ajena {
				s.audit.Append(evento(empresaID, actor, origen, "restaurante.mesa.ajena", c.MesaID,
					"mesa "+m.Nombre+" asignada a "+nombresDe(duenos)))
			}
		}
	}
	now := ahora()
	base := time.Now().UnixNano()
	agregados := 0
	for i, in := range items {
		if in.Cantidad <= 0 {
			return cuenta.Cuenta{}, ErrItemSinCantidad
		}
		c.Items = append(c.Items, cuenta.Item{
			ID: fmt.Sprintf("it_%d_%d", base, i), SKU: in.SKU, Nombre: strings.TrimSpace(in.Nombre),
			Cantidad: in.Cantidad, PrecioUnitario: in.PrecioUnitario, Exento: in.Exento,
			Nota: cuenta.NormalizarNota(in.Nota), Estado: cuenta.ItemPendiente, Ronda: 0, Agregada: now,
		})
		agregados++
	}
	if agregados == 0 {
		return c, nil
	}
	out, ok := s.cuentasMesa.Update(c)
	if !ok {
		return cuenta.Cuenta{}, ErrCuentaMesaNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "restaurante.cuenta.agregar", out.ID, fmt.Sprintf("%d renglón(es)", agregados)))
	return out, nil
}

// EnviarACocina agrupa los renglones pendientes en una nueva RONDA y los marca
// "en_cocina". Devuelve la cuenta y los renglones de esa ronda (la comanda a
// imprimir). Falla si no hay pendientes.
func (s *Service) EnviarACocina(empresaID, cuentaID, actor, origen string) (cuenta.Cuenta, []cuenta.Item, error) {
	c, err := s.cuentaAbierta(empresaID, cuentaID)
	if err != nil {
		return cuenta.Cuenta{}, nil, err
	}
	if !c.TienePendientes() {
		return cuenta.Cuenta{}, nil, ErrSinPendientes
	}
	ronda := c.UltimaRonda + 1
	enviado := ahora()
	comanda := []cuenta.Item{}
	for i := range c.Items {
		if c.Items[i].Estado == cuenta.ItemPendiente {
			c.Items[i].Estado = cuenta.ItemEnCocina
			c.Items[i].Ronda = ronda
			c.Items[i].EnviadoEn = enviado
			comanda = append(comanda, c.Items[i])
		}
	}
	c.UltimaRonda = ronda
	out, ok := s.cuentasMesa.Update(c)
	if !ok {
		return cuenta.Cuenta{}, nil, ErrCuentaMesaNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "restaurante.cuenta.enviar_cocina", out.ID, fmt.Sprintf("ronda %d · %d renglón(es)", ronda, len(comanda))))
	return out, comanda, nil
}

// CancelarItem quita un renglón: si aún estaba pendiente se elimina; si ya fue a
// cocina, se marca cancelado (queda la traza, no aporta al total).
func (s *Service) CancelarItem(empresaID, cuentaID, itemID, actor, origen string) (cuenta.Cuenta, error) {
	c, err := s.cuentaAbierta(empresaID, cuentaID)
	if err != nil {
		return cuenta.Cuenta{}, err
	}
	idx := -1
	for i := range c.Items {
		if c.Items[i].ID == itemID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return cuenta.Cuenta{}, ErrItemCuentaNoExiste
	}
	if c.Items[idx].Estado == cuenta.ItemPendiente {
		c.Items = append(c.Items[:idx], c.Items[idx+1:]...)
	} else {
		c.Items[idx].Estado = cuenta.ItemCancelado
	}
	out, ok := s.cuentasMesa.Update(c)
	if !ok {
		return cuenta.Cuenta{}, ErrCuentaMesaNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "restaurante.cuenta.cancelar_item", out.ID, itemID))
	return out, nil
}

// MarcarItem cambia el estado de cocina de un renglón (listo/servido). Usado por la
// cocina/el mesonero.
func (s *Service) MarcarItem(empresaID, cuentaID, itemID, estado, actor, origen string) (cuenta.Cuenta, error) {
	if !cuenta.EstadoItemValido(estado) {
		return cuenta.Cuenta{}, errors.New("estado de renglón inválido")
	}
	c, err := s.cuentaAbierta(empresaID, cuentaID)
	if err != nil {
		return cuenta.Cuenta{}, err
	}
	found := false
	for i := range c.Items {
		if c.Items[i].ID == itemID {
			c.Items[i].Estado = estado
			found = true
			break
		}
	}
	if !found {
		return cuenta.Cuenta{}, ErrItemCuentaNoExiste
	}
	out, ok := s.cuentasMesa.Update(c)
	if !ok {
		return cuenta.Cuenta{}, ErrCuentaMesaNoExiste
	}
	return out, nil
}

// CerrarCuenta cierra la cuenta (cierre OPERATIVO) y libera la mesa. La emisión de
// la factura fiscal desde la cuenta se integra con el flujo de cobro en la fase
// siguiente; aquí solo se cierra la mesa.
func (s *Service) CerrarCuenta(empresaID, cuentaID, actor, origen string) (cuenta.Cuenta, error) {
	c, err := s.cuentaAbierta(empresaID, cuentaID)
	if err != nil {
		return cuenta.Cuenta{}, err
	}
	c.Estado = cuenta.EstadoCerrada
	c.Cerrada = ahora()
	out, ok := s.cuentasMesa.Update(c)
	if !ok {
		return cuenta.Cuenta{}, ErrCuentaMesaNoExiste
	}
	s.syncMesaEstado(empresaID, c.MesaID, mesa.EstadoLibre)
	s.audit.Append(evento(empresaID, actor, origen, "restaurante.cuenta.cerrar", out.ID, "mesa "+c.MesaNombre))
	return out, nil
}

// cuentaAbierta busca una cuenta y valida que esté abierta.
func (s *Service) cuentaAbierta(empresaID, cuentaID string) (cuenta.Cuenta, error) {
	if s.cuentasMesa == nil {
		return cuenta.Cuenta{}, ErrCuentasNoDisponible
	}
	c, ok := s.cuentasMesa.ByID(empresaID, cuentaID)
	if !ok {
		return cuenta.Cuenta{}, ErrCuentaMesaNoExiste
	}
	if c.Estado != cuenta.EstadoAbierta {
		return cuenta.Cuenta{}, ErrCuentaCerrada
	}
	return c, nil
}

// CobroCuentaEntrada es la instrucción de cobro simple de una cuenta: un pago
// único en bolívares con el método elegido. El cobro mixto multimoneda / propina /
// división de cuenta se integra con el flujo de cobro del POS en fases siguientes.
type CobroCuentaEntrada struct {
	Metodo        string `json:"metodo"`        // método del pago (Bs): efectivo_bs, pago_movil…
	CuentaCobroID string `json:"cuentaCobroId"` // destino del pago (métodos no-efectivo)
	ClienteID     string `json:"clienteId"`     // opcional; vacío ⇒ consumidor final
	SinCaja       bool   `json:"sinCaja"`       // true ⇒ forma libre (sin exigir caja abierta)
}

// totalCuentaBs calcula subtotal, IVA y total (Bs) de los renglones no cancelados,
// con la MISMA lógica que EmitirFactura (base gravada vs. exenta según el catálogo,
// alícuota de la empresa, redondeo a 2). Sin IGTF: el cobro simple es en Bs.
func (s *Service) totalCuentaBs(empresaID string, items []cuenta.Item) (subtotal, iva, total float64) {
	var baseImp, baseEx float64
	for _, it := range items {
		if it.Estado == cuenta.ItemCancelado {
			continue
		}
		exento := it.Exento
		if p, ok := s.productos.BySKU(empresaID, it.SKU); ok {
			exento = p.ExentoIVA // el motor usa el flag del catálogo, no el del renglón
		}
		linea := it.PrecioUnitario * it.Cantidad
		if exento {
			baseEx += linea
		} else {
			baseImp += linea
		}
	}
	baseImp = round2(baseImp)
	baseEx = round2(baseEx)
	subtotal = round2(baseImp + baseEx)
	iva = round2(baseImp * s.alicuotaIVA(empresaID))
	total = round2(subtotal + iva)
	return
}

// PreviewCobroCuenta devuelve el subtotal/IVA/total (Bs) que se facturaría al
// cerrar la cuenta, para que la interfaz muestre el monto antes de cobrar.
func (s *Service) PreviewCobroCuenta(empresaID, cuentaID string) (subtotal, iva, total float64, err error) {
	c, e := s.cuentaAbierta(empresaID, cuentaID)
	if e != nil {
		return 0, 0, 0, e
	}
	sub, ivaC, tot := s.totalCuentaBs(empresaID, c.Items)
	return sub, ivaC, tot, nil
}

// CobrarCuenta emite la FACTURA fiscal de la cuenta (reusando EmitirFactura) con un
// pago único en Bs que cubre el total, cierra la cuenta y libera la mesa. Devuelve
// la cuenta cerrada y el documento emitido.
func (s *Service) CobrarCuenta(empresaID, sedeID, cuentaID, actor, origen string, in CobroCuentaEntrada) (cuenta.Cuenta, fiscal.Documento, error) {
	c, err := s.cuentaAbierta(empresaID, cuentaID)
	if err != nil {
		return cuenta.Cuenta{}, fiscal.Documento{}, err
	}
	var lineas []LineaEntrada
	var items []cuenta.Item
	for _, it := range c.Items {
		if it.Estado == cuenta.ItemCancelado {
			continue
		}
		lineas = append(lineas, LineaEntrada{SKU: it.SKU, Cantidad: it.Cantidad, PrecioUnitario: it.PrecioUnitario})
		items = append(items, it)
	}
	if len(lineas) == 0 {
		return cuenta.Cuenta{}, fiscal.Documento{}, errors.New("la cuenta no tiene renglones para facturar")
	}
	_, _, total := s.totalCuentaBs(empresaID, items)
	metodo := strings.TrimSpace(in.Metodo)
	if metodo == "" {
		metodo = fiscal.PagoEfectivoBs
	}
	ent := EmitirEntrada{
		ClienteID: in.ClienteID, Moneda: "VES", SinCaja: in.SinCaja, Lineas: lineas,
		Pagos: []PagoEntrada{{Metodo: metodo, CuentaID: in.CuentaCobroID, Monto: total, Moneda: "VES"}},
	}
	emp, _ := s.empresas.ByID(empresaID)
	doc, err := s.EmitirFactura(empresaID, sedeID, emp.Modalidad, actor, origen, ent)
	if err != nil {
		return cuenta.Cuenta{}, fiscal.Documento{}, err
	}
	c.Estado = cuenta.EstadoCerrada
	c.Cerrada = ahora()
	c.DocumentoID = doc.ID
	out, ok := s.cuentasMesa.Update(c)
	if !ok {
		return cuenta.Cuenta{}, fiscal.Documento{}, ErrCuentaMesaNoExiste
	}
	s.syncMesaEstado(empresaID, c.MesaID, mesa.EstadoLibre)
	s.audit.Append(evento(empresaID, actor, origen, "restaurante.cuenta.cobrar", out.ID, doc.NumeroCompleto))
	return out, doc, nil
}
