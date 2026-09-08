package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

func TestCrearCuentaCobro(t *testing.T) {
	svc, _ := nuevoServicio(t)
	antes := len(svc.CuentasCobro(empDemo))
	cc, err := svc.CrearCuentaCobro(empDemo, actorA, origenTst, fiscal.CuentaCobro{
		Tipo: "pago_movil", Moneda: "VES", Titular: "Comercio Demo", Datos: "0414-1234567",
	})
	if err != nil {
		t.Fatalf("crear cuenta de cobro: %v", err)
	}
	if cc.EmpresaID != empDemo || cc.ID == "" {
		t.Errorf("la cuenta debe quedar sellada al tenant y con id, se obtuvo %+v", cc)
	}
	if len(svc.CuentasCobro(empDemo)) != antes+1 {
		t.Error("la nueva cuenta de cobro debe aparecer en el listado")
	}
}

func TestCrearMetodoPago_ValidaMoneda(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if _, err := svc.CrearMetodoPago(empDemo, actorA, origenTst, fiscal.MetodoPago{
		Nombre: "Raro", Tipo: fiscal.PagoEfectivoBs, Moneda: "XXX",
	}); err == nil {
		t.Fatal("una moneda inválida debe rechazarse al crear el método de pago")
	}
}

func TestMetodoPago_CicloDeVidaYOrden(t *testing.T) {
	svc, _ := nuevoServicio(t)
	m, err := svc.CrearMetodoPago(empDemo, actorA, origenTst, fiscal.MetodoPago{
		Nombre: "Zelle", Tipo: fiscal.PagoZelle, Moneda: "USD", EnVentas: true, Orden: 5,
	})
	if err != nil {
		t.Fatalf("crear método: %v", err)
	}
	if !m.Activo {
		t.Error("un método de pago nace activo")
	}

	// PATCH parcial: solo el nombre y el orden; el resto queda intacto.
	nuevoNombre := "Zelle USD"
	nuevoOrden := 1
	upd, err := svc.ActualizarMetodoPago(empDemo, actorA, origenTst, m.ID, application.CambiosMetodoPago{
		Nombre: &nuevoNombre, Orden: &nuevoOrden,
	})
	if err != nil {
		t.Fatalf("actualizar método: %v", err)
	}
	if upd.Nombre != "Zelle USD" || upd.Orden != 1 {
		t.Errorf("el PATCH no se aplicó: %+v", upd)
	}
	if upd.Moneda != "USD" || upd.Tipo != fiscal.PagoZelle {
		t.Errorf("los campos no incluidos en el PATCH no deben cambiar: %+v", upd)
	}

	// PATCH con moneda inválida se rechaza.
	monedaMala := "ABC"
	if _, err := svc.ActualizarMetodoPago(empDemo, actorA, origenTst, m.ID, application.CambiosMetodoPago{Moneda: &monedaMala}); err == nil {
		t.Error("actualizar con una moneda inválida debe rechazarse")
	}

	// Eliminar es soft-disable: sigue existiendo pero inactivo.
	if err := svc.EliminarMetodoPago(empDemo, actorA, origenTst, m.ID); err != nil {
		t.Fatalf("eliminar método: %v", err)
	}
	for _, mp := range svc.MetodosPago(empDemo) {
		if mp.ID == m.ID && mp.Activo {
			t.Error("tras eliminar, el método debe quedar inactivo (no borrado)")
		}
	}
	// La lista debe venir ordenada por Orden.
	list := svc.MetodosPago(empDemo)
	for i := 1; i < len(list); i++ {
		if list[i-1].Orden > list[i].Orden {
			t.Fatalf("los métodos de pago deben venir ordenados por Orden: %d antes de %d", list[i-1].Orden, list[i].Orden)
		}
	}
}

func TestMetodoPago_ActualizarInexistente(t *testing.T) {
	svc, _ := nuevoServicio(t)
	n := "x"
	if _, err := svc.ActualizarMetodoPago(empDemo, actorA, origenTst, "mp_fantasma", application.CambiosMetodoPago{Nombre: &n}); !errors.Is(err, application.ErrMetodoPagoNoExiste) {
		t.Fatalf("editar un método inexistente debe dar ErrMetodoPagoNoExiste, se obtuvo: %v", err)
	}
	if err := svc.EliminarMetodoPago(empDemo, actorA, origenTst, "mp_fantasma"); !errors.Is(err, application.ErrMetodoPagoNoExiste) {
		t.Fatalf("eliminar un método inexistente debe dar ErrMetodoPagoNoExiste, se obtuvo: %v", err)
	}
}
