package application_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mornix/elerp/internal/adapter/inmem"
	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

/* Maestro de impuestos.
 *
 * Lo que de verdad se prueba acá no es el CRUD: es que cambiar una tasa NO
 * reescriba el pasado. Un documento de hace seis meses tiene que poder
 * explicarse con la tasa que regía ese día — es el Art. 177 y es lo primero que
 * mira el SENIAT. */

func servicioImpuestos(t *testing.T) (*application.Service, *inmem.Store) {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConAlicuotas(st.Alicuotas)
	return svc, st
}

func hoyDia() string { return time.Now().UTC().Format("2006-01-02") }

// alicuotaPorCodigo busca en el maestro la fila vigente de un código.
func alicuotaPorCodigo(t *testing.T, svc *application.Service, codigo string) fiscal.Alicuota {
	t.Helper()
	a, ok := fiscal.VigenteEn(svc.AlicuotasDeEmpresa(empDemo), codigo, hoyDia())
	if !ok {
		t.Fatalf("no hay alícuota vigente para %q", codigo)
	}
	return a
}

// El maestro se siembra solo: una empresa que nunca lo configuró no puede
// quedarse sin poder clasificar sus productos.
func TestAlicuotas_SeSiembranLasVenezolanas(t *testing.T) {
	svc, _ := servicioImpuestos(t)
	todas := svc.AlicuotasDeEmpresa(empDemo)
	if len(todas) != 4 {
		t.Fatalf("se esperaban 4 alícuotas por defecto, hay %d", len(todas))
	}
	general := alicuotaPorCodigo(t, svc, fiscal.CodGeneral)
	if general.Porcentaje != 0.16 || general.Adicional != 0 {
		t.Errorf("la general debe ser 16 %% sin recargo: %+v", general)
	}
	reducida := alicuotaPorCodigo(t, svc, fiscal.CodReducida)
	if reducida.Porcentaje != 0.08 {
		t.Errorf("la reducida debe ser 8 %%: %v", reducida.Porcentaje)
	}
	// La suntuaria NO es una tasa de 31 %: es la general MÁS un recargo de 15 %.
	// Guardarla como 31 haría imposible llenar el libro de ventas, que declara
	// las dos porciones en columnas separadas.
	sunt := alicuotaPorCodigo(t, svc, fiscal.CodSuntuario)
	if sunt.Porcentaje != 0.16 || sunt.Adicional != 0.15 {
		t.Errorf("la suntuaria debe ser 16 %% + 15 %% adicional, es %v + %v", sunt.Porcentaje, sunt.Adicional)
	}
	exento := alicuotaPorCodigo(t, svc, fiscal.CodExento)
	if exento.Grava() || exento.Porcentaje != 0 {
		t.Errorf("la exenta no causa impuesto: %+v", exento)
	}
}

// Sembrar dos veces no duplica: la siembra corre en cada lectura de la pantalla
// de configuración y tiene que ser idempotente.
func TestAlicuotas_LaSiembraEsIdempotente(t *testing.T) {
	svc, _ := servicioImpuestos(t)
	svc.AlicuotasDeEmpresa(empDemo)
	if n := len(svc.AlicuotasDeEmpresa(empDemo)); n != 4 {
		t.Fatalf("la segunda lectura duplicó el maestro: %d filas", n)
	}
}

// LA prueba de este archivo: cambiar la tasa NO pisa la fila vieja. Si lo
// hiciera, un documento de ayer quedaría explicado por una tasa que ayer no
// existía.
func TestActualizarAlicuota_CambiarLaTasaAbreVigenciaNueva(t *testing.T) {
	svc, _ := servicioImpuestos(t)
	general := alicuotaPorCodigo(t, svc, fiscal.CodGeneral)
	manana := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")

	nuevo := 0.18
	out, err := svc.ActualizarAlicuota(empDemo, actorA, origenTst, general.ID,
		application.CambiosAlicuota{Porcentaje: &nuevo, VigenteDesde: manana})
	if err != nil {
		t.Fatalf("actualizar: %v", err)
	}
	if out.ID == general.ID {
		t.Fatal("un cambio de tasa tiene que crear una fila NUEVA, no pisar la vigente")
	}
	if out.Porcentaje != 0.18 || out.VigenteDesde != manana {
		t.Errorf("la fila nueva quedó mal: %+v", out)
	}

	// La vieja sigue ahí, cerrada el día ANTES: sin hueco ni solape, que en una
	// declaración mensual se nota.
	todas := svc.AlicuotasDeEmpresa(empDemo)
	var vieja fiscal.Alicuota
	for _, a := range todas {
		if a.ID == general.ID {
			vieja = a
		}
	}
	if vieja.ID == "" {
		t.Fatal("la alícuota anterior desapareció del maestro")
	}
	if vieja.Porcentaje != 0.16 {
		t.Errorf("la tasa vieja cambió: %v", vieja.Porcentaje)
	}
	if vieja.VigenteHasta != hoyDia() {
		t.Errorf("la vieja debe cerrar el día antes de la nueva (%s), cerró %q", hoyDia(), vieja.VigenteHasta)
	}
	// Y un documento de HOY sigue viendo el 16 %.
	if hoy := alicuotaPorCodigo(t, svc, fiscal.CodGeneral); hoy.Porcentaje != 0.16 {
		t.Errorf("hoy todavía rige el 16 %%, resolvió %v", hoy.Porcentaje)
	}
}

// Corregir el NOMBRE no puede partir el histórico: no cambia ningún cálculo.
func TestActualizarAlicuota_RenombrarNoAbreVigencia(t *testing.T) {
	svc, _ := servicioImpuestos(t)
	general := alicuotaPorCodigo(t, svc, fiscal.CodGeneral)
	antes := len(svc.AlicuotasDeEmpresa(empDemo))

	nombre := "IVA general"
	out, err := svc.ActualizarAlicuota(empDemo, actorA, origenTst, general.ID,
		application.CambiosAlicuota{Nombre: &nombre})
	if err != nil {
		t.Fatalf("renombrar: %v", err)
	}
	if out.ID != general.ID {
		t.Error("renombrar debe editar la MISMA fila")
	}
	if n := len(svc.AlicuotasDeEmpresa(empDemo)); n != antes {
		t.Errorf("renombrar no debe agregar filas: %d → %d", antes, n)
	}
}

// Quien se equivocó al cargar la tasa hace un rato tiene que poder corregirla
// hoy mismo. Es seguro porque cada documento SELLA su alícuota al emitir: lo ya
// facturado no depende de esta tabla.
func TestActualizarAlicuota_MismoDiaCorrigeEnElSitio(t *testing.T) {
	svc, _ := servicioImpuestos(t)
	general := alicuotaPorCodigo(t, svc, fiscal.CodGeneral)
	antes := len(svc.AlicuotasDeEmpresa(empDemo))

	nuevo := 0.165
	out, err := svc.ActualizarAlicuota(empDemo, actorA, origenTst, general.ID,
		application.CambiosAlicuota{Porcentaje: &nuevo, VigenteDesde: general.VigenteDesde})
	if err != nil {
		t.Fatalf("corregir: %v", err)
	}
	if out.ID != general.ID {
		t.Error("una corrección del mismo día edita la fila, no abre otra")
	}
	if n := len(svc.AlicuotasDeEmpresa(empDemo)); n != antes {
		t.Errorf("no debe agregar filas: %d → %d", antes, n)
	}
	if out.Porcentaje != 0.165 {
		t.Errorf("la corrección no se aplicó: %v", out.Porcentaje)
	}
}

// Abrir una tasa ANTES de la que ya rige reescribiría el pasado.
func TestActualizarAlicuota_NoSePuedeRetroceder(t *testing.T) {
	svc, _ := servicioImpuestos(t)
	general := alicuotaPorCodigo(t, svc, fiscal.CodGeneral)
	ayer := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")

	nuevo := 0.2
	_, err := svc.ActualizarAlicuota(empDemo, actorA, origenTst, general.ID,
		application.CambiosAlicuota{Porcentaje: &nuevo, VigenteDesde: ayer})
	if !errors.Is(err, application.ErrVigenciaAnterior) {
		t.Fatalf("se esperaba ErrVigenciaAnterior, se obtuvo: %v", err)
	}
}

func TestCrearAlicuota_ValidaLoQueRompeElCalculo(t *testing.T) {
	svc, _ := servicioImpuestos(t)
	base := fiscal.Alicuota{Codigo: "especial", Nombre: "Especial", Tipo: fiscal.TipoGeneral, Porcentaje: 0.1}

	sinCodigo := base
	sinCodigo.Codigo = "  "
	if _, err := svc.CrearAlicuota(empDemo, actorA, origenTst, sinCodigo); !errors.Is(err, application.ErrAlicuotaCodigo) {
		t.Errorf("sin código debe fallar: %v", err)
	}
	malTipo := base
	malTipo.Tipo = "inventado"
	if _, err := svc.CrearAlicuota(empDemo, actorA, origenTst, malTipo); !errors.Is(err, application.ErrAlicuotaTipo) {
		t.Errorf("tipo inválido debe fallar: %v", err)
	}
	// 16 en vez de 0,16 multiplicaría el impuesto por cien: se rechaza en el
	// servidor, no solo en la pantalla.
	enPorciento := base
	enPorciento.Porcentaje = 16
	if _, err := svc.CrearAlicuota(empDemo, actorA, origenTst, enPorciento); !errors.Is(err, application.ErrAlicuotaPorcentaje) {
		t.Errorf("una tasa en porcentaje (16) debe rechazarse: %v", err)
	}
}

// Dos filas vigentes con el mismo código dejarían al motor eligiendo por
// desempate en vez de por configuración.
func TestCrearAlicuota_CodigoVigenteDuplicado(t *testing.T) {
	svc, _ := servicioImpuestos(t)
	svc.AlicuotasDeEmpresa(empDemo) // siembra
	_, err := svc.CrearAlicuota(empDemo, actorA, origenTst, fiscal.Alicuota{
		Codigo: fiscal.CodGeneral, Nombre: "Otra general", Tipo: fiscal.TipoGeneral, Porcentaje: 0.2,
	})
	if !errors.Is(err, application.ErrAlicuotaCodigoEnUso) {
		t.Fatalf("se esperaba ErrAlicuotaCodigoEnUso, se obtuvo: %v", err)
	}
}

// La exenta no puede llevar porcentaje: sería una contradicción que después
// nadie sabría leer.
func TestCrearAlicuota_ExentaNoLlevaPorcentaje(t *testing.T) {
	svc, _ := servicioImpuestos(t)
	out, err := svc.CrearAlicuota(empDemo, actorA, origenTst, fiscal.Alicuota{
		Codigo: "exonerado", Nombre: "Exonerado", Tipo: fiscal.TipoExento, Porcentaje: 0.16, Adicional: 0.1,
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if out.Porcentaje != 0 || out.Adicional != 0 {
		t.Errorf("la exenta debe quedar en cero: %+v", out)
	}
}

// La vista marca cuál rige HOY, para que la pantalla no tenga que repetir la
// comparación de fechas (y equivocarse).
func TestAlicuotasVista_MarcaLaVigente(t *testing.T) {
	svc, _ := servicioImpuestos(t)
	general := alicuotaPorCodigo(t, svc, fiscal.CodGeneral)
	manana := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	nuevo := 0.18
	if _, err := svc.ActualizarAlicuota(empDemo, actorA, origenTst, general.ID,
		application.CambiosAlicuota{Porcentaje: &nuevo, VigenteDesde: manana}); err != nil {
		t.Fatalf("actualizar: %v", err)
	}
	vigentes := 0
	for _, v := range svc.AlicuotasVista(empDemo) {
		if v.Codigo == fiscal.CodGeneral && v.Vigente {
			vigentes++
			if v.Porcentaje != 0.16 {
				t.Errorf("la vigente de hoy es el 16 %%, marcó %v", v.Porcentaje)
			}
		}
	}
	if vigentes != 1 {
		t.Errorf("debe haber exactamente una general vigente, hay %d", vigentes)
	}
}

// El aislamiento por tenant es la última línea.
func TestAlicuotas_AisladasPorEmpresa(t *testing.T) {
	svc, _ := servicioImpuestos(t)
	general := alicuotaPorCodigo(t, svc, fiscal.CodGeneral)
	nombre := "Ajena"
	if _, err := svc.ActualizarAlicuota(empSalon, actorA, origenTst, general.ID,
		application.CambiosAlicuota{Nombre: &nombre}); !errors.Is(err, application.ErrAlicuotaNoExiste) {
		t.Errorf("no se puede editar la alícuota de otra empresa: %v", err)
	}
}

/* --- La coherencia entre el catálogo y el motor ---------------------------- */

// EL bug que destapó el smoke: un tenant que clasifica un producto SIN haber
// abierto nunca la pantalla de configuración facturaba a la tasa de respaldo
// aunque su ficha dijera «suntuario». Clasificar tiene que sembrar el maestro.
func TestClasificarProducto_SiembraElMaestroYFacturaBien(t *testing.T) {
	svc, _ := servicioImpuestos(t)
	// Nadie pasó por Configuración: el maestro está vacío a propósito.
	sku := "HAR-001"
	cod := fiscal.CodSuntuario
	if _, err := svc.ActualizarProducto(empDemo, actorA, origenTst, sku,
		application.CambiosProducto{AlicuotaCodigo: &cod}); err != nil {
		t.Fatalf("clasificar: %v", err)
	}
	// Y ahora el motor tiene que poder resolverla.
	a, ok := fiscal.VigenteEn(svc.AlicuotasDeEmpresa(empDemo), fiscal.CodSuntuario, hoyDia())
	if !ok {
		t.Fatal("clasificar un producto debe dejar el maestro sembrado")
	}
	if a.Porcentaje != 0.16 || a.Adicional != 0.15 {
		t.Errorf("la suntuaria quedó mal sembrada: %+v", a)
	}
}

// Un código que el maestro no conoce se rechaza en vez de aceptarse y facturar
// a la tasa de respaldo: el error silencioso es peor que el ruidoso.
func TestClasificarProducto_CodigoDesconocidoSeRechaza(t *testing.T) {
	svc, _ := servicioImpuestos(t)
	cod := "inventada"
	_, err := svc.ActualizarProducto(empDemo, actorA, origenTst, "HAR-001",
		application.CambiosProducto{AlicuotaCodigo: &cod})
	if !errors.Is(err, application.ErrAlicuotaDesconocida) {
		t.Fatalf("se esperaba ErrAlicuotaDesconocida, se obtuvo: %v", err)
	}
}

// Código vacío sigue siendo válido: es «como siempre», y los catálogos ya
// cargados no se migran.
func TestClasificarProducto_VacioSigueSiendoValido(t *testing.T) {
	svc, _ := servicioImpuestos(t)
	cod := ""
	if _, err := svc.ActualizarProducto(empDemo, actorA, origenTst, "HAR-001",
		application.CambiosProducto{AlicuotaCodigo: &cod}); err != nil {
		t.Fatalf("un código vacío debe aceptarse: %v", err)
	}
}

// Clasificar como exento mantiene en sincronía el booleano heredado: medio
// sistema (POS, cotizaciones, comandera) todavía lee `ExentoIVA`, y si quedara
// desfasado un producto exento se cobraría con IVA en el mostrador.
func TestClasificarProducto_SincronizaElBooleanoHeredado(t *testing.T) {
	svc, _ := servicioImpuestos(t)
	exento := fiscal.CodExento
	p, err := svc.ActualizarProducto(empDemo, actorA, origenTst, "HAR-001",
		application.CambiosProducto{AlicuotaCodigo: &exento})
	if err != nil {
		t.Fatalf("clasificar: %v", err)
	}
	if !p.ExentoIVA {
		t.Error("clasificado exento, el booleano heredado debe quedar en true")
	}
	general := fiscal.CodGeneral
	p, err = svc.ActualizarProducto(empDemo, actorA, origenTst, "HAR-001",
		application.CambiosProducto{AlicuotaCodigo: &general})
	if err != nil {
		t.Fatalf("clasificar: %v", err)
	}
	if p.ExentoIVA {
		t.Error("clasificado general, el booleano heredado debe quedar en false")
	}
}
