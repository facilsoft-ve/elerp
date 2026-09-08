package application_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

// Cerrar un período genera el asiento de cierre: salda las nominales contra Resultados
// acumulados (3102), cuadra, y lleva RefTipo "cierre" (para que el P&G lo excluya).
func TestCerrarPeriodo_GeneraAsientoDeCierre(t *testing.T) {
	svc, _ := nuevoServicio(t)
	svc.RecontabilizarPendientes(empDemo, "sistema")

	mesActual := time.Now().UTC().Format("2006-01")
	// Primer mes con ventas anterior al mes en curso.
	anio, mes := 0, 0
	for _, a := range svc.LibroDiario(empDemo) {
		if a.RefTipo != "documento" || len(a.Fecha) < 7 || a.Fecha[:7] >= mesActual {
			continue
		}
		var y, m int
		if _, err := fmt.Sscanf(a.Fecha[:7], "%d-%d", &y, &m); err == nil {
			anio, mes = y, m
			break
		}
	}
	if anio == 0 {
		t.Skip("el seed no tiene ventas en un mes anterior al actual; nada que cerrar")
	}
	if _, err := svc.CerrarPeriodo(empDemo, actorA, origenTst, anio, mes); err != nil {
		t.Fatalf("cerrar período %04d-%02d: %v", anio, mes, err)
	}
	// Debe existir un asiento de cierre que cuadre y salde contra Resultados acumulados.
	var cierre contabilidad.Asiento
	for _, a := range svc.LibroDiario(empDemo) {
		if a.RefTipo == "cierre" {
			cierre = a
			break
		}
	}
	if cierre.ID == "" {
		t.Fatal("no se generó el asiento de cierre")
	}
	if !cierre.Cuadra() {
		t.Error("el asiento de cierre no cuadra")
	}
	toca3102 := false
	for _, l := range cierre.Lineas {
		if l.Codigo == contabilidad.CtaResultadosAcumulados {
			toca3102 = true
		}
	}
	if !toca3102 {
		t.Error("el asiento de cierre debe saldar contra Resultados acumulados (3102)")
	}
}

// M6: el asiento derivado se fecha con la fecha del HECHO (la del documento), no con
// la de registro. Se comprueba que cada asiento de venta hereda la fecha de su factura.
func TestAsientoVenta_HeredaLaFechaDelDocumento(t *testing.T) {
	svc, _ := nuevoServicio(t)
	svc.RecontabilizarPendientes(empDemo, "sistema")

	porDoc := map[string]string{} // docID -> fecha (YYYY-MM-DD) de la factura
	for _, d := range svc.Documentos(empDemo) {
		if d.Tipo == fiscal.TipoFactura && len(d.Fecha) >= 10 {
			porDoc[d.ID] = d.Fecha[:10]
		}
	}
	revisados := 0
	for _, a := range svc.LibroDiario(empDemo) {
		if a.RefTipo != "documento" {
			continue
		}
		if fdoc, ok := porDoc[a.RefID]; ok {
			if len(a.Fecha) < 10 || a.Fecha[:10] != fdoc {
				t.Errorf("asiento %s: fecha %q no coincide con la del documento %q", a.Codigo, a.Fecha, fdoc)
			}
			revisados++
		}
	}
	if revisados == 0 {
		t.Fatal("no se revisó ningún asiento de venta (¿seed sin facturas?)")
	}
}

// El Estado de Resultados acotado por período solo suma los asientos del rango: un
// rango futuro da todo en cero, mientras que el acumulado tiene ingresos del seed.
func TestResultados_AcotadoPorPeriodo(t *testing.T) {
	svc, _ := nuevoServicio(t)
	svc.RecontabilizarPendientes(empDemo, "sistema") // asienta los documentos del seed

	acum := svc.Resultados(empDemo, "", "")
	if acum.IngresoTotal <= 0 {
		t.Fatalf("el acumulado debería tener ingresos del seed, se obtuvo %v", acum.IngresoTotal)
	}
	futuro := svc.Resultados(empDemo, "2099-01-01", "2099-12-31")
	if futuro.IngresoTotal != 0 || futuro.CostoDeVentas != 0 || futuro.UtilidadNeta != 0 {
		t.Errorf("un período sin asientos debe dar P&G en cero, se obtuvo %+v", futuro)
	}
}
