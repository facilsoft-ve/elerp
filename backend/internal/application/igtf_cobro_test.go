package application_test

import (
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

// Un abono en DIVISA de una venta a crédito causa IGTF sobre su equivalente en Bs y
// entra al reporte de IGTF (antes se evadía: solo se calculaba al emitir). Regresión
// del Alto de auditoría de Tesorería.
func TestRegistrarCobro_EnDivisaCausaIGTF(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, 40, ""); err != nil {
		t.Fatalf("cargar tasa: %v", err)
	}
	cred := aCredito(t, svc, 2) // venta a crédito: saldo por cobrar

	_, totalAntes := svc.ReporteIGTF(empDemo)

	// Abono en US$ 1 (= 40 Bs). IGTF = 3% de 40 = 1,20.
	cob, err := svc.RegistrarCobro(empDemo, sede1, actorA, origenTst, application.CobroEntrada{
		DocumentoID: cred.ID, Monto: 1, Moneda: "USD", Metodo: fiscal.PagoEfectivoUSD,
	})
	if err != nil {
		t.Fatalf("cobro en divisa: %v", err)
	}
	if !casi(cob.IGTF, 1.2) {
		t.Errorf("el cobro en divisa debía causar IGTF de 1,20, se obtuvo %v", cob.IGTF)
	}
	_, totalDespues := svc.ReporteIGTF(empDemo)
	if !casi(totalDespues-totalAntes, 1.2) {
		t.Errorf("el reporte de IGTF debía subir 1,20 con el abono en divisa: %v → %v", totalAntes, totalDespues)
	}
}

// Un abono en bolívares NO causa IGTF.
func TestRegistrarCobro_EnBsSinIGTF(t *testing.T) {
	svc, _ := nuevoServicio(t)
	cred := aCredito(t, svc, 2)
	cob, err := svc.RegistrarCobro(empDemo, sede1, actorA, origenTst, application.CobroEntrada{
		DocumentoID: cred.ID, Monto: 50, Moneda: "VES", Metodo: fiscal.PagoPagoMovil,
	})
	if err != nil {
		t.Fatalf("cobro en Bs: %v", err)
	}
	if cob.IGTF != 0 {
		t.Errorf("un cobro en bolívares no debe causar IGTF, se obtuvo %v", cob.IGTF)
	}
}
