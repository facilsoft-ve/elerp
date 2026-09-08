package application_test

import (
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

// Un abono en DIVISA de una venta a crédito causa IGTF y entra al reporte SENIAT; un
// abono en bolívares no. Regresión del Alto de auditoría de Tesorería.
func TestRegistrarCobro_IGTFEnDivisa(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, 40, ""); err != nil {
		t.Fatalf("cargar tasa: %v", err)
	}
	cred := aCredito(t, svc, 2) // venta a crédito, saldo 232 (200 + IVA 32)
	_, totalAntes := svc.ReporteIGTF(empDemo)

	// Abono de 1 US$ (= 40 Bs) → IGTF = 40 × 3% = 1,20.
	cob, err := svc.RegistrarCobro(empDemo, sede1, actorA, origenTst, application.CobroEntrada{
		DocumentoID: cred.ID, Monto: 1, Moneda: "USD", Metodo: fiscal.PagoEfectivoUSD,
	})
	if err != nil {
		t.Fatalf("cobro en divisa: %v", err)
	}
	if !casi(cob.IGTF, 1.20) {
		t.Errorf("el IGTF del abono en divisa debía ser 1,20, se obtuvo %v", cob.IGTF)
	}
	if _, totalDespues := svc.ReporteIGTF(empDemo); !casi(totalDespues-totalAntes, 1.20) {
		t.Errorf("el reporte IGTF debía crecer 1,20 con el abono en divisa, creció %v", totalDespues-totalAntes)
	}

	// Un abono en bolívares no causa IGTF.
	cobBs, err := svc.RegistrarCobro(empDemo, sede1, actorA, origenTst, application.CobroEntrada{
		DocumentoID: cred.ID, Monto: 50, Moneda: "VES", Metodo: fiscal.PagoPagoMovil,
	})
	if err != nil {
		t.Fatalf("cobro en Bs: %v", err)
	}
	if cobBs.IGTF != 0 {
		t.Errorf("un abono en bolívares no debe causar IGTF, se obtuvo %v", cobBs.IGTF)
	}
}
