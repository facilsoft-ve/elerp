package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/cupon"
	"github.com/mornix/elerp/internal/domain/promocion"
)

func TestActualizarCupon_RecalculaYConservaUsos(t *testing.T) {
	svc, _ := servicioConCupones(t)
	c, err := svc.CrearCupon(empDemo, actorA, origenTst, application.EntradaCupon{
		Codigo: "EDIT10", Tipo: cupon.TipoPorcentaje, Valor: 10, Activo: true,
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	// Aparece en el listado.
	visto := false
	for _, x := range svc.Cupones(empDemo) {
		if x.ID == c.ID {
			visto = true
		}
	}
	if !visto {
		t.Fatal("el cupón creado debe aparecer en el listado")
	}
	// Editar el valor.
	upd, err := svc.ActualizarCupon(empDemo, c.ID, actorA, origenTst, application.EntradaCupon{
		Codigo: "EDIT10", Tipo: cupon.TipoPorcentaje, Valor: 25, Activo: true,
	})
	if err != nil {
		t.Fatalf("actualizar: %v", err)
	}
	if upd.Valor != 25 {
		t.Errorf("el valor editado debe ser 25, se obtuvo %v", upd.Valor)
	}
	if upd.UsosActuales != c.UsosActuales {
		t.Errorf("editar un cupón no debe reescribir sus usos: %d → %d", c.UsosActuales, upd.UsosActuales)
	}
}

func TestActualizarCupon_NoExiste(t *testing.T) {
	svc, _ := servicioConCupones(t)
	if _, err := svc.ActualizarCupon(empDemo, "cup_fantasma", actorA, origenTst, application.EntradaCupon{
		Codigo: "X", Tipo: cupon.TipoMonto, Valor: 5,
	}); !errors.Is(err, application.ErrCuponNoExiste) {
		t.Fatalf("editar un cupón inexistente debe dar ErrCuponNoExiste, se obtuvo: %v", err)
	}
}

func TestActualizarPromocion_EditaYLista(t *testing.T) {
	svc := servicioConPromociones(t)
	const emp = "emp_promo_edit"
	p, err := svc.CrearPromocion(emp, actorA, origenTst, application.EntradaPromocion{
		Nombre: "Banner", Tipo: promocion.TipoTexto, Titulo: "Hola", Activa: true,
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if len(svc.Promociones(emp)) != 1 {
		t.Fatalf("la promoción creada debe listarse, se obtuvo %d", len(svc.Promociones(emp)))
	}
	upd, err := svc.ActualizarPromocion(emp, p.ID, actorA, origenTst, application.EntradaPromocion{
		Nombre: "Banner v2", Tipo: promocion.TipoTexto, Titulo: "Hola de nuevo", Activa: false,
	})
	if err != nil {
		t.Fatalf("actualizar: %v", err)
	}
	if upd.Nombre != "Banner v2" || upd.Titulo != "Hola de nuevo" || upd.Activa {
		t.Errorf("la edición de la promoción no se aplicó: %+v", upd)
	}
}
