package application_test

import (
	"testing"

	"github.com/mornix/elerp/internal/adapter/inmem"
	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/plantilla"
)

func servicioConPlantillas(t *testing.T) (*application.Service, *inmem.Store) {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConPlantillas(st.Plantillas)
	return svc, st
}

func TestPlantillas_SeedDemo(t *testing.T) {
	svc, _ := servicioConPlantillas(t)
	ps := svc.Plantillas(empDemo)
	if len(ps) != 6 {
		t.Fatalf("la empresa demo debía traer 6 formatos preestablecidos, trajo %d", len(ps))
	}
	predetPorTipo := map[string]int{}
	for _, p := range ps {
		if p.Predeterminada {
			predetPorTipo[p.Tipo]++
		}
		if p.AnchoMM <= 0 || p.AltoMM <= 0 || len(p.Bloques) == 0 {
			t.Errorf("el formato demo %q vino sin dimensiones o sin bloques", p.Nombre)
		}
	}
	for _, tipo := range []string{plantilla.TipoFactura, plantilla.TipoCotizacion, plantilla.TipoNotaEntrega} {
		if predetPorTipo[tipo] != 1 {
			t.Errorf("el tipo %q debía tener exactamente 1 formato predeterminado, tiene %d", tipo, predetPorTipo[tipo])
		}
	}
}

func TestPorDefecto_TicketApilaSinTotalesEnNota(t *testing.T) {
	// El ticket usa el ancho del rollo; la nota de entrega no lleva totales.
	fac := plantilla.PorDefecto(plantilla.TipoFactura, plantilla.PapelTicket80)
	if fac.AnchoMM != 80 {
		t.Errorf("ticket 80: ancho %v; quería 80", fac.AnchoMM)
	}
	hayTotales := false
	for _, b := range fac.Bloques {
		if b.Tipo == plantilla.BloqueTotales {
			hayTotales = true
		}
	}
	if !hayTotales {
		t.Errorf("la factura ticket debía incluir el bloque de totales")
	}
	nota := plantilla.PorDefecto(plantilla.TipoNotaEntrega, plantilla.PapelCarta)
	for _, b := range nota.Bloques {
		if b.Tipo == plantilla.BloqueTotales {
			t.Errorf("una nota de entrega NO debía llevar bloque de totales")
		}
	}
}

func TestCrearPlantilla_PrimeraDeTipoEsPredeterminada(t *testing.T) {
	svc, _ := servicioConPlantillas(t)
	// El presupuesto es un tipo que el seed NO trae ⇒ el primero debe quedar
	// predeterminado, y arrancar con los bloques del preset (lienzo no vacío).
	p1, err := svc.CrearPlantilla(empDemo, actorA, origenTst, plantilla.Plantilla{
		Nombre: "Presupuesto A", Tipo: plantilla.TipoPresupuesto, Papel: plantilla.PapelCarta,
	})
	if err != nil {
		t.Fatalf("crear formato: %v", err)
	}
	if !p1.Predeterminada {
		t.Errorf("el primer formato de un tipo nuevo debía ser predeterminado")
	}
	if p1.AnchoMM != 215.9 {
		t.Errorf("carta debía fijar ancho 215.9, se obtuvo %v", p1.AnchoMM)
	}
	if len(p1.Bloques) == 0 {
		t.Errorf("un alta sin bloques debía autocompletarse con el preset del tamaño")
	}
	// Un segundo del mismo tipo NO debe robar el predeterminado.
	p2, _ := svc.CrearPlantilla(empDemo, actorA, origenTst, plantilla.Plantilla{
		Nombre: "Presupuesto B", Tipo: plantilla.TipoPresupuesto, Papel: plantilla.PapelA4,
	})
	if p2.Predeterminada {
		t.Errorf("el segundo formato de un tipo no debía nacer predeterminado")
	}
}

func TestCrearPlantilla_PapelCustomExigeDimensiones(t *testing.T) {
	svc, _ := servicioConPlantillas(t)
	_, err := svc.CrearPlantilla(empDemo, actorA, origenTst, plantilla.Plantilla{
		Nombre: "A medida", Tipo: plantilla.TipoRecibo, Papel: plantilla.PapelPersonalizado,
	})
	if err == nil {
		t.Fatalf("un papel a medida sin dimensiones debía rechazarse")
	}
	ok, err := svc.CrearPlantilla(empDemo, actorA, origenTst, plantilla.Plantilla{
		Nombre: "A medida", Tipo: plantilla.TipoRecibo, Papel: plantilla.PapelPersonalizado,
		AnchoMM: 100, AltoMM: 150,
	})
	if err != nil {
		t.Fatalf("papel a medida con dimensiones: %v", err)
	}
	if ok.AnchoMM != 100 || ok.AltoMM != 150 {
		t.Errorf("dimensiones a medida no conservadas: %v×%v", ok.AnchoMM, ok.AltoMM)
	}
}

func TestAsignarYResolverPlantillaPorSede(t *testing.T) {
	svc, _ := servicioConPlantillas(t)
	// Segundo formato de factura, para asignarlo a una sede concreta.
	alt, err := svc.CrearPlantilla(empDemo, actorA, origenTst, plantilla.Plantilla{
		Nombre: "Factura ticket", Tipo: plantilla.TipoFactura, Papel: plantilla.PapelTicket80,
	})
	if err != nil {
		t.Fatalf("crear formato alterno: %v", err)
	}
	// Sin asignación, ambas sedes resuelven al predeterminado (el del seed).
	base, ok := svc.ResolverPlantilla(empDemo, plantilla.TipoFactura, sede1)
	if !ok || !base.Predeterminada {
		t.Fatalf("sin asignación debía resolver al predeterminado (ok=%v)", ok)
	}
	// Asignar el ticket a la sede 2.
	if err := svc.AsignarPlantillaSede(empDemo, plantilla.TipoFactura, sede2, alt.ID, actorA, origenTst); err != nil {
		t.Fatalf("asignar: %v", err)
	}
	got, ok := svc.ResolverPlantilla(empDemo, plantilla.TipoFactura, sede2)
	if !ok || got.ID != alt.ID {
		t.Errorf("la sede 2 debía resolver al ticket asignado (%s), resolvió %s", alt.ID, got.ID)
	}
	// La sede 1 sigue en el predeterminado.
	got1, _ := svc.ResolverPlantilla(empDemo, plantilla.TipoFactura, sede1)
	if got1.ID != base.ID {
		t.Errorf("la sede 1 no debía cambiar de formato")
	}
	// Reasignar la sede 2 a "" la devuelve al predeterminado.
	if err := svc.AsignarPlantillaSede(empDemo, plantilla.TipoFactura, sede2, "", actorA, origenTst); err != nil {
		t.Fatalf("desasignar: %v", err)
	}
	got2, _ := svc.ResolverPlantilla(empDemo, plantilla.TipoFactura, sede2)
	if got2.ID != base.ID {
		t.Errorf("al quitar la asignación, la sede 2 debía volver al predeterminado")
	}
}

func TestEliminarPlantilla_ReasignaPredeterminado(t *testing.T) {
	svc, _ := servicioConPlantillas(t)
	// La factura predeterminada del seed (carta).
	var predet plantilla.Plantilla
	for _, p := range svc.Plantillas(empDemo) {
		if p.Tipo == plantilla.TipoFactura && p.Predeterminada {
			predet = p
		}
	}
	if predet.ID == "" {
		t.Fatalf("el seed debía traer una factura predeterminada")
	}
	if err := svc.EliminarPlantilla(empDemo, predet.ID, actorA, origenTst); err != nil {
		t.Fatalf("eliminar: %v", err)
	}
	// Al borrar el predeterminado, otra factura hereda la marca y el tipo sigue
	// resolviendo (quedan la media carta y el ticket).
	got, ok := svc.ResolverPlantilla(empDemo, plantilla.TipoFactura, sede1)
	if !ok || got.ID == predet.ID {
		t.Errorf("tras borrar el predeterminado, otra factura debía resolver (ok=%v)", ok)
	}
	if !got.Predeterminada {
		t.Errorf("la factura que quedó debía heredar la marca de predeterminada")
	}
}
