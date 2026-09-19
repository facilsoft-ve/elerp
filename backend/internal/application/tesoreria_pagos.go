package application

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mornix/elerp/internal/domain/compra"
	"github.com/mornix/elerp/internal/domain/tesoreria"
)

// Errores de negocio de los pagos a proveedor.
var (
	ErrPagoInvalido    = errors.New("el monto del pago debe ser mayor que cero")
	ErrProveedorNoPago = errors.New("ese proveedor no existe")
	ErrPagoExcede      = errors.New("el pago es mayor que el saldo pendiente del proveedor")
	ErrPagoNoExiste    = errors.New("ese pago no existe")
	ErrPagoYaReversado = errors.New("ese pago ya fue reversado")
)

// EntradaPagoProveedor son los datos de un pago hecho a un proveedor. El pago se
// registra directamente en bolívares (MontoBs); la imputación a una orden es
// opcional (vacía = pago a cuenta de la deuda global del proveedor).
type EntradaPagoProveedor struct {
	ProveedorID   string
	OrdenCompraID string
	MontoBs       float64
	Metodo        string
	Referencia    string
	// ExcepcionMotivo autoriza pagar una compra cuya factura no cuadra con lo
	// recibido (control en tres vías). Se exige solo cuando hace falta, y lo que se
	// pide es el MOTIVO: es lo que después se puede leer en la auditoría.
	ExcepcionMotivo string
}

// ErrPagoBloqueadoPorControl: la factura de esa compra no cuadra con lo recibido y
// la empresa exige declarar por qué se paga igual.
var ErrPagoBloqueadoPorControl = errors.New("la factura de esa compra no cuadra con lo recibido: indica el motivo para pagarla igual")

// facturasEnExcepcionDe devuelve las facturas del proveedor que el control en tres
// vías marcó como NO conformes. Solo se miran las evaluadas: las registradas antes
// de que el control existiera no son excepciones, son facturas sin veredicto.
func (s *Service) facturasEnExcepcionDe(empresaID, proveedorID, ordenID string) []compra.FacturaCompra {
	var out []compra.FacturaCompra
	if s.facturasCompra == nil {
		return out
	}
	for _, f := range s.facturasCompra.List(empresaID) {
		if !f.ControlEvaluado || f.ControlConforme {
			continue
		}
		if f.ProveedorID != proveedorID {
			continue
		}
		// Imputado a una orden: solo importa ESA. A cuenta del proveedor: importa
		// cualquiera suya, o el bloqueo se esquivaría pagando «a cuenta».
		if ordenID != "" && f.OrdenCompraID != ordenID {
			continue
		}
		out = append(out, f)
	}
	return out
}

// verificarControlTresVias frena el pago de una compra cuya factura no cuadra con
// lo recibido, salvo que quien paga DECLARE por qué. Con política "avisar" la
// excepción queda marcada en la factura pero no frena nada.
func (s *Service) verificarControlTresVias(empresaID string, in EntradaPagoProveedor) error {
	if s.politicaControlCompras(empresaID) != ControlBloquear {
		return nil
	}
	if strings.TrimSpace(in.ExcepcionMotivo) != "" {
		return nil // alguien lo autoriza y dice por qué; queda en el pago y en la auditoría
	}
	enExcepcion := s.facturasEnExcepcionDe(empresaID, in.ProveedorID, strings.TrimSpace(in.OrdenCompraID))
	if len(enExcepcion) == 0 {
		return nil
	}
	f := enExcepcion[0]
	return fmt.Errorf("%w (factura %s: facturado %.2f contra recibido %.2f, tolerancia %.2f)",
		ErrPagoBloqueadoPorControl, f.NumeroFactura, f.BaseImponible+f.BaseExenta, f.BaseRecibida, f.ControlTolerancia)
}

// RegistrarPagoProveedor anexa un pago contra la deuda de un proveedor y deriva su
// asiento. Espejo de RegistrarCobro del lado del pasivo.
//
// Reglas: el proveedor tiene que existir; el monto positivo; y el pago no puede
// exceder el saldo NETO pendiente del proveedor (lo adeudado por sus órdenes
// recibidas menos lo ya pagado). Pagar de más no es un pago, es un anticipo, y ese
// es otro concepto.
func (s *Service) RegistrarPagoProveedor(empresaID, actor, origen string, in EntradaPagoProveedor) (tesoreria.PagoProveedor, error) {
	if s.pagosProveedor == nil {
		return tesoreria.PagoProveedor{}, errors.New("Tesorería no está configurada")
	}
	if in.MontoBs <= 0 {
		return tesoreria.PagoProveedor{}, ErrPagoInvalido
	}
	prov, ok := s.provs.ByID(empresaID, in.ProveedorID)
	if !ok {
		return tesoreria.PagoProveedor{}, ErrProveedorNoPago
	}
	// El saldo pendiente sale de Cuentas por pagar (adeudado − pagos previos): así el
	// tope se mueve solo a medida que se paga, sin un contador que sincronizar.
	saldo := s.saldoPorPagarProveedor(empresaID, in.ProveedorID)
	if saldo <= 0.004 {
		return tesoreria.PagoProveedor{}, fmt.Errorf("%w: no se le adeuda nada", ErrPagoExcede)
	}
	// Control en tres vías: no se paga lo que no llegó, salvo declaración expresa.
	if err := s.verificarControlTresVias(empresaID, in); err != nil {
		return tesoreria.PagoProveedor{}, err
	}
	montoBs := round2(in.MontoBs)
	if montoBs > saldo+0.005 {
		return tesoreria.PagoProveedor{}, fmt.Errorf("%w: el saldo es %.2f Bs", ErrPagoExcede, saldo)
	}

	p := s.pagosProveedor.Append(tesoreria.PagoProveedor{
		EmpresaID: empresaID, ProveedorID: prov.ID, ProveedorNombre: prov.Nombre,
		OrdenCompraID: strings.TrimSpace(in.OrdenCompraID),
		Monto:         montoBs, MontoBs: montoBs, Moneda: "VES",
		Metodo: in.Metodo, Referencia: strings.TrimSpace(in.Referencia),
		ExcepcionMotivo: strings.TrimSpace(in.ExcepcionMotivo),
		Actor:           actor, Fecha: ahora(),
	})
	// Asiento derivado: baja la cuenta por pagar y sale el dinero.
	s.asentarPagoProveedor(empresaID, actor, p)
	s.audit.Append(evento(empresaID, actor, origen, "tesoreria.pago_proveedor.registrar", prov.Nombre,
		fmt.Sprintf("%.2f Bs", montoBs)))
	return p, nil
}

// ReversarPagoProveedor anula un pago mal registrado anexando su reverso. No se
// borra nada: el histórico tiene que poder explicar el saldo actual.
func (s *Service) ReversarPagoProveedor(empresaID, actor, origen, id, motivo string) (tesoreria.PagoProveedor, error) {
	if s.pagosProveedor == nil {
		return tesoreria.PagoProveedor{}, ErrPagoNoExiste
	}
	orig, ok := s.pagosProveedor.ByID(empresaID, id)
	if !ok || orig.Reverso {
		return tesoreria.PagoProveedor{}, ErrPagoNoExiste
	}
	for _, p := range s.pagosProveedor.List(empresaID) {
		if p.Reverso && p.RefPagoID == id {
			return tesoreria.PagoProveedor{}, ErrPagoYaReversado
		}
	}
	if strings.TrimSpace(motivo) == "" {
		return tesoreria.PagoProveedor{}, errors.New("reversar un pago exige un motivo")
	}
	rev := s.pagosProveedor.Append(tesoreria.PagoProveedor{
		EmpresaID: empresaID, ProveedorID: orig.ProveedorID, ProveedorNombre: orig.ProveedorNombre,
		OrdenCompraID: orig.OrdenCompraID,
		Monto:         orig.Monto, MontoBs: orig.MontoBs, Moneda: orig.Moneda,
		Metodo:  orig.Metodo,
		Reverso: true, RefPagoID: orig.ID, Motivo: strings.TrimSpace(motivo),
		Actor: actor, Fecha: ahora(),
	})
	s.asentarPagoProveedor(empresaID, actor, rev)
	s.audit.Append(evento(empresaID, actor, origen, "tesoreria.pago_proveedor.reversar", orig.ProveedorNombre, motivo))
	return rev, nil
}

// PagosProveedor devuelve el histórico de pagos a proveedor (más reciente primero).
func (s *Service) PagosProveedor(empresaID string) []tesoreria.PagoProveedor {
	if s.pagosProveedor == nil {
		return []tesoreria.PagoProveedor{}
	}
	out := s.pagosProveedor.List(empresaID)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Fecha > out[j-1].Fecha; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
