package application_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/promocion"
)

// servicioConPromociones arma el servicio del seed demo con el maestro de
// promociones cableado (nuevoServicio no lo cablea, igual que cmd/api aparte).
func servicioConPromociones(t *testing.T) *application.Service {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConPromociones(st.Promociones)
	return svc
}

func fechaPromo(d int) string { return time.Now().UTC().AddDate(0, 0, d).Format("2006-01-02") }

func TestCrearPromocion_ValidaNombreTipoYContenido(t *testing.T) {
	svc := servicioConPromociones(t)

	if _, err := svc.CrearPromocion(empDemo, actorA, origenTst, application.EntradaPromocion{Nombre: " ", Tipo: promocion.TipoTexto, Titulo: "X"}); err == nil {
		t.Errorf("un nombre vacío debía fallar")
	}
	if _, err := svc.CrearPromocion(empDemo, actorA, origenTst, application.EntradaPromocion{Nombre: "N", Tipo: "otro", Titulo: "X"}); !errors.Is(err, application.ErrTipoPromocionInvalido) {
		t.Errorf("un tipo inválido debía dar ErrTipoPromocionInvalido, se obtuvo %v", err)
	}
	if _, err := svc.CrearPromocion(empDemo, actorA, origenTst, application.EntradaPromocion{Nombre: "N", Tipo: promocion.TipoImagen}); err == nil {
		t.Errorf("una promoción de imagen sin imagen debía fallar")
	}
	if _, err := svc.CrearPromocion(empDemo, actorA, origenTst, application.EntradaPromocion{Nombre: "N", Tipo: promocion.TipoTexto}); err == nil {
		t.Errorf("una promoción de texto sin título debía fallar")
	}
	if _, err := svc.CrearPromocion(empDemo, actorA, origenTst, application.EntradaPromocion{
		Nombre: "N", Tipo: promocion.TipoTexto, Titulo: "X", Desde: fechaPromo(5), Hasta: fechaPromo(1),
	}); err == nil {
		t.Errorf("hasta anterior a desde debía fallar")
	}
}

func TestPromocionesActivas_SoloActivasYVigentes(t *testing.T) {
	svc := servicioConPromociones(t)
	// Tenant propio del test (sin las promociones del seed demo) para contar exacto.
	const emp = "emp_promo_test"

	// Vigente y activa (debe salir).
	if _, err := svc.CrearPromocion(emp, actorA, origenTst, application.EntradaPromocion{
		Nombre: "Vigente", Tipo: promocion.TipoTexto, Titulo: "Hola", Activa: true,
		Desde: fechaPromo(-1), Hasta: fechaPromo(10), Orden: 2,
	}); err != nil {
		t.Fatalf("crear vigente: %v", err)
	}
	// Vigente sin límites, orden menor (debe salir primero).
	if _, err := svc.CrearPromocion(emp, actorA, origenTst, application.EntradaPromocion{
		Nombre: "Primera", Tipo: promocion.TipoImagen, Imagen: "/api/archivos/x.png", Activa: true, Orden: 1,
	}); err != nil {
		t.Fatalf("crear primera: %v", err)
	}
	// Inactiva (no debe salir).
	if _, err := svc.CrearPromocion(emp, actorA, origenTst, application.EntradaPromocion{
		Nombre: "Off", Tipo: promocion.TipoTexto, Titulo: "Oculta", Activa: false,
	}); err != nil {
		t.Fatalf("crear off: %v", err)
	}
	// Vencida (activa pero fuera de vigencia; no debe salir).
	if _, err := svc.CrearPromocion(emp, actorA, origenTst, application.EntradaPromocion{
		Nombre: "Vencida", Tipo: promocion.TipoTexto, Titulo: "Vieja", Activa: true,
		Desde: fechaPromo(-30), Hasta: fechaPromo(-1),
	}); err != nil {
		t.Fatalf("crear vencida: %v", err)
	}

	act := svc.PromocionesActivas(emp)
	if len(act) != 2 {
		t.Fatalf("esperaba 2 promociones activas y vigentes, se obtuvieron %d", len(act))
	}
	if act[0].Nombre != "Primera" {
		t.Errorf("esperaba orden por Orden ascendente (Primera antes que Vigente), se obtuvo %q primero", act[0].Nombre)
	}
}
