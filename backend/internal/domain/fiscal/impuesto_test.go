package fiscal

import "testing"

/* El maestro de impuestos es DATO con vigencia, y de ahí salen las dos cosas
 * que estas pruebas cuidan:
 *
 *   1. que un documento viejo se recalcule con la tasa de SU día y no con la de
 *      hoy (Art. 177: si el IVA sube por providencia, la nota de crédito de una
 *      factura de marzo sigue siendo del 16% de marzo);
 *   2. que la suntuaria siga siendo 16% + 15% y no un 31% colapsado — el libro
 *      de ventas las declara en columnas separadas y un total no se puede
 *      descomponer. */

func maestro() []Alicuota {
	return []Alicuota{
		{Codigo: CodGeneral, Nombre: "General 16%", Tipo: TipoGeneral, Porcentaje: 0.16,
			VigenteDesde: "2020-01-01", VigenteHasta: "2026-05-31", Activa: true},
		// Una providencia sube la general al 18% desde junio.
		{Codigo: CodGeneral, Nombre: "General 18%", Tipo: TipoGeneral, Porcentaje: 0.18,
			VigenteDesde: "2026-06-01", Activa: true},
		{Codigo: CodReducida, Nombre: "Reducida 8%", Tipo: TipoReducida, Porcentaje: 0.08,
			VigenteDesde: "2020-01-01", Activa: true},
		{Codigo: CodSuntuario, Nombre: "Suntuaria", Tipo: TipoGeneral, Porcentaje: 0.16, Adicional: 0.15,
			VigenteDesde: "2020-01-01", Activa: true},
		{Codigo: CodExento, Nombre: "Exento", Tipo: TipoExento, VigenteDesde: "2020-01-01", Activa: true},
	}
}

// LA prueba de la vigencia: una factura de marzo se recalcula con el 16% de
// marzo aunque hoy rija el 18%.
func TestVigenteEn_EligeLaTasaDelDiaDelHechoImponible(t *testing.T) {
	a, ok := VigenteEn(maestro(), CodGeneral, "2026-03-15T10:00:00Z")
	if !ok {
		t.Fatal("no encontró la general de marzo")
	}
	if a.Porcentaje != 0.16 {
		t.Errorf("en marzo regía el 16%%, devolvió %v", a.Porcentaje)
	}
	b, _ := VigenteEn(maestro(), CodGeneral, "2026-07-01T10:00:00Z")
	if b.Porcentaje != 0.18 {
		t.Errorf("en julio rige el 18%%, devolvió %v", b.Porcentaje)
	}
}

// El día del cambio pertenece a la tasa NUEVA: VigenteDesde es inclusivo, y el
// anterior cerró el 31 de mayo.
func TestVigenteEn_LimitesInclusivos(t *testing.T) {
	a, _ := VigenteEn(maestro(), CodGeneral, "2026-05-31")
	if a.Porcentaje != 0.16 {
		t.Errorf("el 31 de mayo todavía rige el 16%%, dio %v", a.Porcentaje)
	}
	b, _ := VigenteEn(maestro(), CodGeneral, "2026-06-01")
	if b.Porcentaje != 0.18 {
		t.Errorf("el 1 de junio ya rige el 18%%, dio %v", b.Porcentaje)
	}
}

func TestVigenteEn_FueraDeTodoRangoNoEncuentra(t *testing.T) {
	if _, ok := VigenteEn(maestro(), CodGeneral, "2019-12-31"); ok {
		t.Error("antes de la primera vigencia no debería haber alícuota")
	}
	if _, ok := VigenteEn(maestro(), "no-existe", "2026-03-15"); ok {
		t.Error("un código desconocido no puede resolver")
	}
	if _, ok := VigenteEn(maestro(), CodGeneral, ""); ok {
		t.Error("sin fecha no se puede decidir qué regía")
	}
}

// La suntuaria es la general MÁS un recargo. Si alguien la colapsa en 31%, el
// libro de ventas ya no se puede llenar.
func TestSuntuaria_EsGeneralMasRecargo(t *testing.T) {
	a, ok := VigenteEn(maestro(), CodSuntuario, "2026-03-15")
	if !ok {
		t.Fatal("falta la suntuaria")
	}
	if a.Porcentaje != 0.16 {
		t.Errorf("la porción general debe ser 16%%, es %v", a.Porcentaje)
	}
	if a.Adicional != 0.15 {
		t.Errorf("el recargo adicional debe ser 15%%, es %v", a.Adicional)
	}
	if a.Porcentaje+a.Adicional != 0.31 {
		t.Errorf("juntas deben dar 31%%, dan %v", a.Porcentaje+a.Adicional)
	}
}

func TestGrava(t *testing.T) {
	if (Alicuota{Tipo: TipoExento}).Grava() {
		t.Error("lo exento no causa impuesto")
	}
	for _, tipo := range []string{TipoGeneral, TipoReducida, TipoAdicional} {
		if !(Alicuota{Tipo: tipo}).Grava() {
			t.Errorf("%q sí causa impuesto", tipo)
		}
	}
}

func TestTipoAlicuotaValido(t *testing.T) {
	for _, tipo := range []string{TipoGeneral, TipoReducida, TipoAdicional, TipoExento} {
		if !TipoAlicuotaValido(tipo) {
			t.Errorf("%q debería ser válido", tipo)
		}
	}
	for _, tipo := range []string{"", "iva", "GENERAL", "lujo"} {
		if TipoAlicuotaValido(tipo) {
			t.Errorf("%q no debería ser válido", tipo)
		}
	}
}

// Configuración solapada (que no debería pasar): gana la que se cargó después.
// Es determinista a propósito — sin regla, el cálculo dependería del orden en
// que la base devolvió las filas.
func TestVigenteEn_SolapadasGanaLaMasReciente(t *testing.T) {
	solapadas := []Alicuota{
		{Codigo: CodGeneral, Porcentaje: 0.16, Tipo: TipoGeneral, VigenteDesde: "2020-01-01", Activa: true},
		{Codigo: CodGeneral, Porcentaje: 0.18, Tipo: TipoGeneral, VigenteDesde: "2026-01-01", Activa: true},
	}
	a, _ := VigenteEn(solapadas, CodGeneral, "2026-06-01")
	if a.Porcentaje != 0.18 {
		t.Errorf("debería ganar la de vigencia más reciente (18%%), dio %v", a.Porcentaje)
	}
}

func TestAlicuotasPorDefecto_TraeLasTresTasasVenezolanas(t *testing.T) {
	def := AlicuotasPorDefecto("emp_x", "2026-01-01")
	porCodigo := map[string]Alicuota{}
	for _, a := range def {
		if a.EmpresaID != "emp_x" {
			t.Errorf("%s: empresa mal sembrada", a.Codigo)
		}
		if !a.Activa || a.VigenteDesde != "2026-01-01" {
			t.Errorf("%s: debe nacer activa y vigente desde la fecha dada", a.Codigo)
		}
		porCodigo[a.Codigo] = a
	}
	if porCodigo[CodGeneral].Porcentaje != 0.16 {
		t.Errorf("general = %v, se esperaba 0,16", porCodigo[CodGeneral].Porcentaje)
	}
	if porCodigo[CodReducida].Porcentaje != 0.08 {
		t.Errorf("reducida = %v, se esperaba 0,08", porCodigo[CodReducida].Porcentaje)
	}
	if porCodigo[CodSuntuario].Adicional != 0.15 {
		t.Errorf("suntuaria: recargo = %v, se esperaba 0,15", porCodigo[CodSuntuario].Adicional)
	}
	if porCodigo[CodExento].Grava() {
		t.Error("la clasificación exenta no puede gravar")
	}
}
