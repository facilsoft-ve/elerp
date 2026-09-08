package application

import (
	"errors"
	"sort"

	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

// CuentasCobro lista las cuentas de cobro de la empresa.
func (s *Service) CuentasCobro(empresaID string) []fiscal.CuentaCobro {
	return s.cuentasCobro.List(empresaID)
}

// CrearCuentaCobro registra una cuenta donde la empresa recibe pagos.
func (s *Service) CrearCuentaCobro(empresaID, actor, origen string, c fiscal.CuentaCobro) (fiscal.CuentaCobro, error) {
	c.EmpresaID = empresaID
	out := s.cuentasCobro.Create(c)
	s.audit.Append(evento(empresaID, actor, origen, "tesoreria.cuenta.crear", out.ID, out.Tipo))
	return out, nil
}

// ErrMetodoPagoNoExiste indica que el método no pertenece a la empresa.
var ErrMetodoPagoNoExiste = errors.New("método de pago no existe")

// MetodosPago lista los métodos de pago configurados de la empresa, ordenados
// por Orden.
func (s *Service) MetodosPago(empresaID string) []fiscal.MetodoPago {
	out := s.metodosPago.List(empresaID)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Orden < out[j].Orden })
	return out
}

// CrearMetodoPago registra un método de pago configurable de la empresa.
func (s *Service) CrearMetodoPago(empresaID, actor, origen string, m fiscal.MetodoPago) (fiscal.MetodoPago, error) {
	if m.Moneda != "" && !empresa.MonedaValida(m.Moneda) {
		return fiscal.MetodoPago{}, errors.New("moneda inválida")
	}
	m.EmpresaID = empresaID
	m.Activo = true
	out := s.metodosPago.Create(m)
	s.audit.Append(evento(empresaID, actor, origen, "config.metodopago.crear", out.ID, out.Nombre))
	return out, nil
}

// CambiosMetodoPago describe una edición parcial de un método de pago. Los
// punteros nil dejan el campo intacto (semántica PATCH).
type CambiosMetodoPago struct {
	Nombre        *string
	Moneda        *string
	CuentaCobroID *string
	EnCaja        *bool
	EnVentas      *bool
	Activo        *bool
	Orden         *int
}

// ActualizarMetodoPago edita un método de pago existente de la empresa.
func (s *Service) ActualizarMetodoPago(empresaID, actor, origen, id string, cambios CambiosMetodoPago) (fiscal.MetodoPago, error) {
	m, ok := s.metodosPago.ByID(empresaID, id)
	if !ok {
		return fiscal.MetodoPago{}, ErrMetodoPagoNoExiste
	}
	if cambios.Nombre != nil {
		m.Nombre = *cambios.Nombre
	}
	if cambios.Moneda != nil {
		if *cambios.Moneda != "" && !empresa.MonedaValida(*cambios.Moneda) {
			return fiscal.MetodoPago{}, errors.New("moneda inválida")
		}
		m.Moneda = *cambios.Moneda
	}
	if cambios.CuentaCobroID != nil {
		m.CuentaCobroID = *cambios.CuentaCobroID
	}
	if cambios.EnCaja != nil {
		m.EnCaja = *cambios.EnCaja
	}
	if cambios.EnVentas != nil {
		m.EnVentas = *cambios.EnVentas
	}
	if cambios.Activo != nil {
		m.Activo = *cambios.Activo
	}
	if cambios.Orden != nil {
		m.Orden = *cambios.Orden
	}
	out, ok := s.metodosPago.Update(m)
	if !ok {
		return fiscal.MetodoPago{}, ErrMetodoPagoNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "config.metodopago.actualizar", out.ID, out.Nombre))
	return out, nil
}

// EliminarMetodoPago desactiva (soft-disable) un método de pago de la empresa.
// No borra: pone Activo=false para que la operación sea reversible (se reactiva
// con el toggle Activo). Así no se pierde el histórico ni las referencias.
func (s *Service) EliminarMetodoPago(empresaID, actor, origen, id string) error {
	m, ok := s.metodosPago.ByID(empresaID, id)
	if !ok {
		return ErrMetodoPagoNoExiste
	}
	m.Activo = false
	if _, ok := s.metodosPago.Update(m); !ok {
		return ErrMetodoPagoNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "config.metodopago.desactivar", id, ""))
	return nil
}
