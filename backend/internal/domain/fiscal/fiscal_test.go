package fiscal

import (
	"math"
	"testing"
)

// TestTipoDispositivoValido cubre la validación del tipo de dispositivo fiscal,
// incluida la balanza (venta por peso). Es sensible a mayúsculas y al vacío.
func TestTipoDispositivoValido(t *testing.T) {
	casos := []struct {
		tipo   string
		quiere bool
	}{
		{DispositivoImpresoraFiscal, true},
		{DispositivoBalanza, true},
		{DispositivoOtro, true},
		{"", false},
		{"scanner", false},
		{"impresora", false},
		{"IMPRESORA_FISCAL", false}, // sensible a mayúsculas
		{" balanza", false},         // sin trim: el espacio invalida
	}
	for _, c := range casos {
		t.Run("tipo_"+c.tipo, func(t *testing.T) {
			if got := TipoDispositivoValido(c.tipo); got != c.quiere {
				t.Errorf("TipoDispositivoValido(%q) = %v; se esperaba %v", c.tipo, got, c.quiere)
			}
		})
	}
}

// TestTiposDispositivo_Valores fija el valor canónico de cada tipo de
// dispositivo: se persisten y el agente fiscal local los interpreta.
func TestTiposDispositivo_Valores(t *testing.T) {
	casos := map[string]string{
		"impresora_fiscal": DispositivoImpresoraFiscal,
		"balanza":          DispositivoBalanza,
		"otro":             DispositivoOtro,
	}
	for esperado, got := range casos {
		if got != esperado {
			t.Errorf("tipo de dispositivo = %q; se esperaba %q", got, esperado)
		}
	}
}

// TestTiposDocumento_Valores fija los valores canónicos de los tipos de
// documento fiscal. Son strings legalmente significativos y persistidos.
func TestTiposDocumento_Valores(t *testing.T) {
	casos := map[string]string{
		"factura":      TipoFactura,
		"nota_credito": TipoNotaCredito,
		"nota_debito":  TipoNotaDebito,
		"anulacion":    TipoAnulacion,
	}
	for esperado, got := range casos {
		if got != esperado {
			t.Errorf("tipo de documento = %q; se esperaba %q", got, esperado)
		}
	}
}

// TestTiposDocumento_Distintos garantiza que los cuatro tipos sean distintos:
// dos tipos colapsados romperían la clasificación de los libros fiscales.
func TestTiposDocumento_Distintos(t *testing.T) {
	tipos := []string{TipoFactura, TipoNotaCredito, TipoNotaDebito, TipoAnulacion}
	visto := map[string]bool{}
	for _, tp := range tipos {
		if visto[tp] {
			t.Errorf("tipo de documento duplicado: %q", tp)
		}
		visto[tp] = true
	}
}

// TestMetodosPago_Valores fija los métodos de pago canónicos usados por el POS.
func TestMetodosPago_Valores(t *testing.T) {
	casos := map[string]string{
		"efectivo_bs":   PagoEfectivoBs,
		"efectivo_usd":  PagoEfectivoUSD,
		"pago_movil":    PagoPagoMovil,
		"zelle":         PagoZelle,
		"tarjeta":       PagoTarjeta,
		"transferencia": PagoTransfer,
	}
	for esperado, got := range casos {
		if got != esperado {
			t.Errorf("método de pago = %q; se esperaba %q", got, esperado)
		}
	}
}

// TestVueltoMetodos_Valores fija los medios por los que se entrega el vuelto.
func TestVueltoMetodos_Valores(t *testing.T) {
	if VueltoEfectivo != "efectivo" {
		t.Errorf("VueltoEfectivo = %q; se esperaba %q", VueltoEfectivo, "efectivo")
	}
	if VueltoPagoMovil != "pago_movil" {
		t.Errorf("VueltoPagoMovil = %q; se esperaba %q", VueltoPagoMovil, "pago_movil")
	}
}

// TestAlicuotas_Valores fija las alícuotas base: IVA 16% e IGTF 3%. Aunque un
// motor de reglas las parametrice a futuro, estos son los valores por defecto.
func TestAlicuotas_Valores(t *testing.T) {
	if math.Abs(AlicuotaIVA-0.16) >= 0.005 {
		t.Errorf("AlicuotaIVA = %v; se esperaba 0.16", AlicuotaIVA)
	}
	if math.Abs(AlicuotaIGTF-0.03) >= 0.005 {
		t.Errorf("AlicuotaIGTF = %v; se esperaba 0.03", AlicuotaIGTF)
	}
}

// TestErrFolioRetrocede verifica que el error centinela de numeración exista y
// lleve mensaje: la regla "los folios solo avanzan" se apoya en él.
func TestErrFolioRetrocede(t *testing.T) {
	if ErrFolioRetrocede == nil {
		t.Fatal("ErrFolioRetrocede no debería ser nil")
	}
	if ErrFolioRetrocede.Error() == "" {
		t.Error("ErrFolioRetrocede debería tener un mensaje no vacío")
	}
}

// anuladoDerivado replica la regla de dominio (§13.2): un documento está
// "anulado" si EXISTE una reversa total (una anulación) que lo referencia. El
// estado no se guarda; se deriva del ledger de documentos. Una nota de crédito
// (corrección parcial) NO anula el documento. La proyección real vive en
// application; aquí se caracteriza la regla usando los tipos y constantes del
// dominio para blindarla contra renombres.
func anuladoDerivado(docs []Documento, origID string) bool {
	for _, d := range docs {
		if d.Tipo == TipoAnulacion && d.RefDocumentoID == origID {
			return true
		}
	}
	return false
}

// TestAnuladoDerivado cubre la derivación del estado "anulado".
func TestAnuladoDerivado(t *testing.T) {
	factura := Documento{ID: "F1", Tipo: TipoFactura}
	casos := []struct {
		nombre string
		docs   []Documento
		quiere bool
	}{
		{
			nombre: "sin_reversa",
			docs:   []Documento{factura},
			quiere: false,
		},
		{
			nombre: "con_anulacion_que_referencia",
			docs:   []Documento{factura, {ID: "A1", Tipo: TipoAnulacion, RefDocumentoID: "F1"}},
			quiere: true,
		},
		{
			nombre: "anulacion_de_otro_documento",
			docs:   []Documento{factura, {ID: "A1", Tipo: TipoAnulacion, RefDocumentoID: "F9"}},
			quiere: false,
		},
		{
			// Una nota de crédito referencia la factura pero NO la anula: es una
			// corrección parcial, no una reversa total.
			nombre: "nota_credito_no_anula",
			docs:   []Documento{factura, {ID: "NC1", Tipo: TipoNotaCredito, RefDocumentoID: "F1"}},
			quiere: false,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := anuladoDerivado(c.docs, "F1"); got != c.quiere {
				t.Errorf("anuladoDerivado(F1) = %v; se esperaba %v", got, c.quiere)
			}
		})
	}
}

// sumaVueltoBs suma en bolívares las partes del vuelto (cobro/vuelto mixto). El
// resumen retrocompatible Documento.Vuelto debe igualar esta suma cuando el
// vuelto se reparte en varias partes.
func sumaVueltoBs(partes []VueltoParte) float64 {
	var total float64
	for _, p := range partes {
		total += p.MontoBs
	}
	return total
}

// TestVueltoPartes_SumaBs caracteriza el reparto del vuelto: una sola parte, y
// un vuelto MIXTO (US$20 en efectivo + el resto en Bs por pago móvil). El total
// en Bs es la suma de las partes, y cada parte usa un medio válido.
func TestVueltoPartes_SumaBs(t *testing.T) {
	casos := []struct {
		nombre  string
		partes  []VueltoParte
		totalBs float64
	}{
		{
			nombre:  "sin_vuelto",
			partes:  nil,
			totalBs: 0,
		},
		{
			nombre:  "una_parte_efectivo_usd",
			partes:  []VueltoParte{{Moneda: "USD", Metodo: VueltoEfectivo, Monto: 23, MontoBs: 828}},
			totalBs: 828,
		},
		{
			// US$3 debía darse: US$20 no aplica aquí; ejemplo mixto US$20 efectivo
			// + resto Bs por pago móvil, cada parte con su MontoBs.
			nombre: "mixto_usd_efectivo_mas_bs_pagomovil",
			partes: []VueltoParte{
				{Moneda: "USD", Metodo: VueltoEfectivo, Monto: 20, MontoBs: 720},
				{Moneda: "VES", Metodo: VueltoPagoMovil, Monto: 108, MontoBs: 108},
			},
			totalBs: 828,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := sumaVueltoBs(c.partes); math.Abs(got-c.totalBs) >= 0.005 {
				t.Errorf("sumaVueltoBs = %v; se esperaba %v", got, c.totalBs)
			}
			for i, p := range c.partes {
				if p.Metodo != VueltoEfectivo && p.Metodo != VueltoPagoMovil {
					t.Errorf("parte %d: método de vuelto inválido %q", i, p.Metodo)
				}
			}
		})
	}
}
