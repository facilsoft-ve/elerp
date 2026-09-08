package application_test

import (
	"encoding/json"
	"testing"
)

func TestRestaurarDatos_RoundTrip(t *testing.T) {
	svc, _ := nuevoServicio(t)

	// Exportar el tenant demo y re-parsear como el respaldo (map de colecciones).
	b, err := json.Marshal(svc.ExportarTenant(empDemo))
	if err != nil {
		t.Fatalf("marshal export: %v", err)
	}
	var datos map[string]json.RawMessage
	if err := json.Unmarshal(b, &datos); err != nil {
		t.Fatalf("unmarshal export: %v", err)
	}

	prodOrig := len(svc.Productos(empDemo))
	docsOrig := len(svc.Documentos(empDemo))
	if prodOrig == 0 || docsOrig == 0 {
		t.Fatal("el seed demo debe tener productos y documentos para probar el restore")
	}

	// Restaurar en una EMPRESA NUEVA (id fresco).
	const nuevo = "emp_restore_test"
	counts := svc.RestaurarDatos(nuevo, datos)

	if counts["productos"] != prodOrig {
		t.Fatalf("se debieron restaurar %d productos, se contaron %d", prodOrig, counts["productos"])
	}
	if got := len(svc.Productos(nuevo)); got != prodOrig {
		t.Fatalf("la empresa restaurada debe tener %d productos, tiene %d", prodOrig, got)
	}
	if got := len(svc.Documentos(nuevo)); got != docsOrig {
		t.Fatalf("la empresa restaurada debe tener %d documentos, tiene %d", docsOrig, got)
	}

	// Aislamiento: la empresa ORIGINAL queda intacta (no se duplicó ni se movió nada).
	if got := len(svc.Productos(empDemo)); got != prodOrig {
		t.Fatalf("la empresa original debe seguir con %d productos, tiene %d", prodOrig, got)
	}

	// Integridad: el libro de la empresa restaurada verifica (los asientos traen su hash).
	for _, r := range svc.VerificarIntegridad(nuevo) {
		if !r.Integro {
			t.Fatalf("el libro %s de la empresa restaurada debe verificar (roto en %s)", r.Libro, r.RotoEn)
		}
	}
}
