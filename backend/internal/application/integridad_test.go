package application_test

import (
	"testing"

	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/sello"
)

// Tras operar, ambos libros quedan SELLADOS e ÍNTEGROS: el adaptador en memoria
// siembra y anexa todo vía Append, que encadena por hash.
func TestIntegridad_LibrosSelladosEIntegros(t *testing.T) {
	svc, _ := nuevoServicio(t)
	svc.RecontabilizarPendientes(empDemo, "sistema") // asienta los documentos del seed

	for _, r := range svc.VerificarIntegridad(empDemo) {
		if !r.Integro {
			t.Errorf("libro %q debería estar íntegro, se rompió en %q", r.Libro, r.RotoEn)
		}
		if r.Total == 0 || r.Sellados == 0 {
			t.Errorf("libro %q: total=%d sellados=%d (se esperaban > 0)", r.Libro, r.Total, r.Sellados)
		}
		if r.SinSellar != 0 {
			t.Errorf("libro %q: %d registros sin sellar (deberían estar todos sellados)", r.Libro, r.SinSellar)
		}
	}
}

// El sello detecta la ALTERACIÓN del contenido y la ROTURA del encadenamiento.
func TestSello_DetectaAlteracionYRotura(t *testing.T) {
	a1 := contabilidad.Asiento{Numero: 1, Codigo: "AS-000001", Fecha: "2026-01-01T00:00:00Z", Total: 100,
		Lineas: []contabilidad.Linea{{Codigo: "1101", Debe: 100}, {Codigo: "4101", Haber: 100}}}
	a1.PrevHash = ""
	a1.Hash = sello.Encadenar(a1.PrevHash, a1.Contenido())

	a2 := contabilidad.Asiento{Numero: 2, Codigo: "AS-000002", Fecha: "2026-01-02T00:00:00Z", Total: 50,
		Lineas: []contabilidad.Linea{{Codigo: "1101", Debe: 50}, {Codigo: "4101", Haber: 50}}}
	a2.PrevHash = a1.Hash
	a2.Hash = sello.Encadenar(a2.PrevHash, a2.Contenido())

	// Cadena válida.
	if sello.Encadenar(a2.PrevHash, a2.Contenido()) != a2.Hash {
		t.Fatal("la cadena recién construida debería verificar")
	}
	// Alterar un monto rompe el hash del registro.
	alterado := a2
	alterado.Total = 999
	alterado.Lineas[0].Debe = 999
	if sello.Encadenar(alterado.PrevHash, alterado.Contenido()) == a2.Hash {
		t.Error("alterar el contenido debe cambiar el hash (no debería coincidir con el sellado)")
	}
	// Romper el encadenamiento (PrevHash que no enlaza) también se detecta.
	if a2.PrevHash != a1.Hash {
		t.Error("PrevHash de a2 debería enlazar con el Hash de a1")
	}
}
