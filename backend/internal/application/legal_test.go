package application_test

import (
	"strings"
	"testing"

	"github.com/mornix/elerp/internal/adapter/inmem"
	"github.com/mornix/elerp/internal/domain/legal"
)

func TestLegal_AceptacionQuitaPendientesYGuardaPrueba(t *testing.T) {
	svc, _ := nuevoServicio(t)
	svc.ConLegal(inmem.NewLegalRepo())

	// Al inicio, los dos documentos están pendientes.
	pend := svc.LegalPendiente("usr_x")
	if len(pend) != 2 {
		t.Fatalf("deben quedar 2 documentos pendientes al inicio, hay %d", len(pend))
	}

	// El hash es estable y no vacío.
	vs := svc.VersionesLegalesVigentes()
	for _, v := range vs {
		if !strings.HasPrefix(v.Hash, "sha256:") {
			t.Fatalf("el hash de %s debe ser sha256, es %q", v.Documento, v.Hash)
		}
	}

	// Aceptar Términos → queda 1 pendiente (Privacidad).
	if _, err := svc.AceptarLegal("usr_x", "emp_demo", "dueno", legal.DocTerminos, "1.2.3.4", "TestUA", ""); err != nil {
		t.Fatalf("aceptar términos: %v", err)
	}
	if len(svc.LegalPendiente("usr_x")) != 1 {
		t.Fatal("tras aceptar Términos debe quedar 1 pendiente")
	}

	// Aceptar Privacidad → sin pendientes.
	if _, err := svc.AceptarLegal("usr_x", "emp_demo", "dueno", legal.DocPrivacidad, "1.2.3.4", "TestUA", ""); err != nil {
		t.Fatalf("aceptar privacidad: %v", err)
	}
	if n := len(svc.LegalPendiente("usr_x")); n != 0 {
		t.Fatalf("tras aceptar ambos no debe quedar pendiente, hay %d", n)
	}

	// El historial (evidencia) tiene las dos aceptaciones con hash e IP.
	hist := svc.AceptacionesDe("usr_x")
	if len(hist) != 2 {
		t.Fatalf("el historial debe tener 2 aceptaciones, tiene %d", len(hist))
	}
	if hist[0].Hash == "" || hist[0].Origen != "1.2.3.4" {
		t.Fatalf("la prueba debe guardar hash e IP: %+v", hist[0])
	}

	// Documento desconocido → error.
	if _, err := svc.AceptarLegal("usr_x", "", "", "otro", "1.2.3.4", "UA", ""); err == nil {
		t.Fatal("aceptar un documento desconocido debe fallar")
	}
}
