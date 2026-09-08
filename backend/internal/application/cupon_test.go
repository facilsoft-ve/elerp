package application_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mornix/elerp/internal/adapter/inmem"
	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/cupon"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

// servicioConCupones arma el servicio del seed demo con el maestro de cupones
// cableado (nuevoServicio no lo cablea, igual que cmd/api lo hace aparte).
func servicioConCupones(t *testing.T) (*application.Service, *inmem.Store) {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConCupones(st.Cupones)
	return svc, st
}

func fechaCupon(d int) string { return time.Now().UTC().AddDate(0, 0, d).Format("2006-01-02") }

func TestCrearCupon_ValidaCodigoTipoYValor(t *testing.T) {
	svc, _ := servicioConCupones(t)

	if _, err := svc.CrearCupon(empDemo, actorA, origenTst, application.EntradaCupon{Codigo: " ", Tipo: cupon.TipoPorcentaje, Valor: 10}); err == nil {
		t.Errorf("un código vacío debía fallar")
	}
	if _, err := svc.CrearCupon(empDemo, actorA, origenTst, application.EntradaCupon{Codigo: "X", Tipo: "otro", Valor: 10}); !errors.Is(err, application.ErrTipoCuponInvalido) {
		t.Errorf("un tipo inválido debía dar ErrTipoCuponInvalido, se obtuvo %v", err)
	}
	if _, err := svc.CrearCupon(empDemo, actorA, origenTst, application.EntradaCupon{Codigo: "X", Tipo: cupon.TipoPorcentaje, Valor: 0}); err == nil {
		t.Errorf("un valor 0 debía fallar")
	}
	if _, err := svc.CrearCupon(empDemo, actorA, origenTst, application.EntradaCupon{Codigo: "X", Tipo: cupon.TipoPorcentaje, Valor: 120}); err == nil {
		t.Errorf("un cupón porcentual >100 debía fallar")
	}
	// Código único por empresa (case-insensitive: se normaliza a MAYÚSCULAS).
	if _, err := svc.CrearCupon(empDemo, actorA, origenTst, application.EntradaCupon{Codigo: "dup", Tipo: cupon.TipoMonto, Valor: 5, Activo: true}); err != nil {
		t.Fatalf("crear DUP: %v", err)
	}
	if _, err := svc.CrearCupon(empDemo, actorA, origenTst, application.EntradaCupon{Codigo: "DUP", Tipo: cupon.TipoMonto, Valor: 7}); !errors.Is(err, application.ErrCuponCodigoDup) {
		t.Errorf("un código repetido debía dar ErrCuponCodigoDup, se obtuvo %v", err)
	}
}

func TestValidarCupon_VigenteAplica(t *testing.T) {
	svc, _ := servicioConCupones(t)
	if _, err := svc.CrearCupon(empDemo, actorA, origenTst, application.EntradaCupon{
		Codigo: "OK10", Tipo: cupon.TipoPorcentaje, Valor: 10, Activo: true,
		Desde: fechaCupon(-1), Hasta: fechaCupon(10),
	}); err != nil {
		t.Fatalf("crear: %v", err)
	}
	res, err := svc.ValidarCupon(empDemo, "ok10", 1000) // case-insensitive
	if err != nil {
		t.Fatalf("un cupón vigente debía aplicar, se obtuvo %v", err)
	}
	if res.DescuentoBs != 100 { // 10% de 1000
		t.Errorf("descuento esperado 100, se obtuvo %v", res.DescuentoBs)
	}
}

func TestValidarCupon_MontoFijoTopadoAlSubtotal(t *testing.T) {
	svc, _ := servicioConCupones(t)
	if _, err := svc.CrearCupon(empDemo, actorA, origenTst, application.EntradaCupon{
		Codigo: "FIJO50", Tipo: cupon.TipoMonto, Valor: 50, Activo: true,
	}); err != nil {
		t.Fatalf("crear: %v", err)
	}
	res, err := svc.ValidarCupon(empDemo, "FIJO50", 200)
	if err != nil {
		t.Fatalf("aplicar: %v", err)
	}
	if res.DescuentoBs != 50 {
		t.Errorf("descuento esperado 50, se obtuvo %v", res.DescuentoBs)
	}
	// Un subtotal menor al monto fijo no puede dejar el total en negativo.
	res, err = svc.ValidarCupon(empDemo, "FIJO50", 30)
	if err != nil {
		t.Fatalf("aplicar: %v", err)
	}
	if res.DescuentoBs != 30 {
		t.Errorf("el descuento debía toparse al subtotal (30), se obtuvo %v", res.DescuentoBs)
	}
}

func TestEmitirFactura_ConsumeElCuponYAplicaElTope(t *testing.T) {
	// FIX: UsosMax no se aplicaba porque UsosActuales nunca subía (ValidarCupon solo
	// lee el contador). Al emitir con un cupón el uso se CONSUME; un cupón de un solo
	// uso queda agotado tras la venta.
	svc, _ := servicioConCupones(t)
	abrirTurno(t, svc, actorA)
	if _, err := svc.CrearCupon(empDemo, actorA, origenTst, application.EntradaCupon{
		Codigo: "UNO", Tipo: cupon.TipoMonto, Valor: 10, Activo: true, UsosMax: 1,
	}); err != nil {
		t.Fatalf("crear cupón: %v", err)
	}
	// Antes de emitir, el cupón aplica.
	if _, err := svc.ValidarCupon(empDemo, "UNO", 1000); err != nil {
		t.Fatalf("el cupón debía aplicar la primera vez: %v", err)
	}
	// Emitir con el cupón: el consumo sube UsosActuales (aquí solo importa el
	// consumo; el descuento ya lo aplicó el front sobre el precio de línea).
	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas:      []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
		Pagos:       []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 116, Moneda: "VES"}},
		CuponCodigo: "UNO",
	}); err != nil {
		t.Fatalf("emitir con cupón: %v", err)
	}
	// Consumido el único uso, el segundo intento se rechaza por agotado.
	if _, err := svc.ValidarCupon(empDemo, "UNO", 1000); !errors.Is(err, application.ErrCuponUsosAgotados) {
		t.Errorf("tras consumir el único uso, el cupón debe rechazarse por agotado, se obtuvo: %v", err)
	}
}

func TestFacturarCotizacion_ConsumeElCupon(t *testing.T) {
	// El cupón aplicado a una cotización queda GUARDADO en ella y se consume al
	// facturarla (misma vía fiscal que el POS). Antes no se consumía por Ventas.
	svc, _ := servicioConCupones(t)
	if _, err := svc.CrearCupon(empDemo, actorA, origenTst, application.EntradaCupon{
		Codigo: "COT1", Tipo: cupon.TipoMonto, Valor: 10, Activo: true, UsosMax: 1,
	}); err != nil {
		t.Fatalf("crear cupón: %v", err)
	}
	c, err := svc.CrearCotizacion(empDemo, sede1, actorA, origenTst, application.EntradaCotizacion{
		Lineas:      []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
		CuponCodigo: "COT1",
	})
	if err != nil {
		t.Fatalf("crear cotización: %v", err)
	}
	if _, err := svc.ConfirmarCotizacion(empDemo, c.ID, actorA, origenTst); err != nil {
		t.Fatalf("confirmar: %v", err)
	}
	// Total = 100 + IVA 16 = 116; se cobra todo en Bs.
	if _, _, err := svc.FacturarCotizacion(empDemo, c.ID, actorA, origenTst, application.EntradaFacturacion{
		Pagos: []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 116, Moneda: "VES"}},
	}); err != nil {
		t.Fatalf("facturar cotización: %v", err)
	}
	// Consumido el único uso, un segundo intento se rechaza por agotado.
	if _, err := svc.ValidarCupon(empDemo, "COT1", 1000); !errors.Is(err, application.ErrCuponUsosAgotados) {
		t.Errorf("tras facturar, el cupón de un solo uso debe quedar agotado, se obtuvo: %v", err)
	}
}

func TestValidarCupon_ErroresDeNegocio(t *testing.T) {
	svc, st := servicioConCupones(t)

	// No existe.
	if _, err := svc.ValidarCupon(empDemo, "NOPE", 1000); !errors.Is(err, application.ErrCuponNoExiste) {
		t.Errorf("un código inexistente debía dar ErrCuponNoExiste, se obtuvo %v", err)
	}

	// Vencido (Hasta en el pasado).
	if _, err := svc.CrearCupon(empDemo, actorA, origenTst, application.EntradaCupon{
		Codigo: "VENCIDO", Tipo: cupon.TipoPorcentaje, Valor: 10, Activo: true,
		Desde: fechaCupon(-30), Hasta: fechaCupon(-1),
	}); err != nil {
		t.Fatalf("crear VENCIDO: %v", err)
	}
	if _, err := svc.ValidarCupon(empDemo, "VENCIDO", 1000); !errors.Is(err, application.ErrCuponFueraVigencia) {
		t.Errorf("un cupón vencido debía dar ErrCuponFueraVigencia, se obtuvo %v", err)
	}

	// Aún no vigente (Desde en el futuro).
	if _, err := svc.CrearCupon(empDemo, actorA, origenTst, application.EntradaCupon{
		Codigo: "FUTURO", Tipo: cupon.TipoPorcentaje, Valor: 10, Activo: true, Desde: fechaCupon(5),
	}); err != nil {
		t.Fatalf("crear FUTURO: %v", err)
	}
	if _, err := svc.ValidarCupon(empDemo, "FUTURO", 1000); !errors.Is(err, application.ErrCuponFueraVigencia) {
		t.Errorf("un cupón aún no vigente debía dar ErrCuponFueraVigencia, se obtuvo %v", err)
	}

	// Inactivo.
	if _, err := svc.CrearCupon(empDemo, actorA, origenTst, application.EntradaCupon{
		Codigo: "OFF", Tipo: cupon.TipoMonto, Valor: 5, Activo: false,
	}); err != nil {
		t.Fatalf("crear OFF: %v", err)
	}
	if _, err := svc.ValidarCupon(empDemo, "OFF", 1000); !errors.Is(err, application.ErrCuponInactivo) {
		t.Errorf("un cupón inactivo debía dar ErrCuponInactivo, se obtuvo %v", err)
	}

	// Monto mínimo no alcanzado.
	if _, err := svc.CrearCupon(empDemo, actorA, origenTst, application.EntradaCupon{
		Codigo: "MIN500", Tipo: cupon.TipoMonto, Valor: 50, MontoMinimo: 500, Activo: true,
	}); err != nil {
		t.Fatalf("crear MIN500: %v", err)
	}
	if _, err := svc.ValidarCupon(empDemo, "MIN500", 400); !errors.Is(err, application.ErrCuponMontoMinimo) {
		t.Errorf("por debajo del mínimo debía dar ErrCuponMontoMinimo, se obtuvo %v", err)
	}

	// Usos agotados: se siembra directo por el repo con UsosActuales == UsosMax
	// (el servicio no expone incremento de usos; ver cupon.go).
	st.Cupones.Create(cupon.Cupon{
		EmpresaID: empDemo, Codigo: "AGOTADO", Tipo: cupon.TipoMonto, Valor: 10,
		Activo: true, UsosMax: 3, UsosActuales: 3,
	})
	if _, err := svc.ValidarCupon(empDemo, "AGOTADO", 1000); !errors.Is(err, application.ErrCuponUsosAgotados) {
		t.Errorf("un cupón sin usos debía dar ErrCuponUsosAgotados, se obtuvo %v", err)
	}
}
