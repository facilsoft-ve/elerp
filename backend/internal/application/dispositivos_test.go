package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

// crearBalanza da de alta una balanza de mostrador y devuelve la ficha creada.
func crearBalanza(t *testing.T, svc *application.Service, nombre, serie string) fiscal.DispositivoFiscal {
	t.Helper()
	d, err := svc.CrearDispositivoFiscal(empDemo, actorA, origenTst, fiscal.DispositivoFiscal{
		Nombre: nombre, SedeID: sede1, Tipo: fiscal.DispositivoBalanza,
		Puerto: "COM4", Protocolo: "prt1", Serie: serie,
	})
	if err != nil {
		t.Fatalf("crear balanza: %v", err)
	}
	return d
}

func TestCrearDispositivo_NombreObligatorio(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if _, err := svc.CrearDispositivoFiscal(empDemo, actorA, origenTst, fiscal.DispositivoFiscal{Nombre: "  "}); err == nil {
		t.Fatal("un dispositivo sin nombre debe rechazarse")
	}
}

func TestCrearDispositivo_TipoInvalido(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if _, err := svc.CrearDispositivoFiscal(empDemo, actorA, origenTst, fiscal.DispositivoFiscal{
		Nombre: "Raro", Tipo: "teletransportador",
	}); err == nil {
		t.Fatal("un tipo de dispositivo desconocido debe rechazarse")
	}
}

func TestCrearDispositivo_TipoPorDefectoImpresora(t *testing.T) {
	svc, _ := nuevoServicio(t)
	d, err := svc.CrearDispositivoFiscal(empDemo, actorA, origenTst, fiscal.DispositivoFiscal{Nombre: "Sin tipo"})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if d.Tipo != fiscal.DispositivoImpresoraFiscal {
		t.Errorf("sin tipo debe asumirse impresora fiscal, se obtuvo %q", d.Tipo)
	}
	if !d.Activo {
		t.Error("un dispositivo nuevo nace activo")
	}
}

func TestCrearDispositivo_SerieDuplicada(t *testing.T) {
	svc, _ := nuevoServicio(t)
	crearBalanza(t, svc, "Balanza A", "SERIE-XYZ")
	// Otra ficha con la misma serie (aunque difiera en mayúsculas) choca.
	if _, err := svc.CrearDispositivoFiscal(empDemo, actorA, origenTst, fiscal.DispositivoFiscal{
		Nombre: "Balanza B", Tipo: fiscal.DispositivoBalanza, Serie: "serie-xyz",
	}); err == nil {
		t.Fatal("una serie ya usada por otro dispositivo debe rechazarse")
	}
}

func TestActualizarDispositivo_NoExiste(t *testing.T) {
	svc, _ := nuevoServicio(t)
	nombre := "x"
	if _, err := svc.ActualizarDispositivoFiscal(empDemo, actorA, origenTst, "disp_fantasma", application.CambiosDispositivoFiscal{
		Nombre: &nombre,
	}); !errors.Is(err, application.ErrDispositivoNoExiste) {
		t.Fatalf("editar un dispositivo inexistente debe dar ErrDispositivoNoExiste, se obtuvo: %v", err)
	}
}

func TestActualizarDispositivo_SerieDuplicadaEntreFichas(t *testing.T) {
	svc, _ := nuevoServicio(t)
	crearBalanza(t, svc, "Balanza A", "SERIE-A")
	b := crearBalanza(t, svc, "Balanza B", "SERIE-B")
	serieA := "SERIE-A"
	// Reasignarle a B la serie de A choca; su propia serie no.
	if _, err := svc.ActualizarDispositivoFiscal(empDemo, actorA, origenTst, b.ID, application.CambiosDispositivoFiscal{
		Serie: &serieA,
	}); err == nil {
		t.Fatal("mover a otra ficha una serie ya usada debe rechazarse")
	}
	serieB := "SERIE-B"
	if _, err := svc.ActualizarDispositivoFiscal(empDemo, actorA, origenTst, b.ID, application.CambiosDispositivoFiscal{
		Serie: &serieB,
	}); err != nil {
		t.Fatalf("conservar la propia serie no debe chocar: %v", err)
	}
}

func TestDesactivarDispositivo_SoftDisable(t *testing.T) {
	svc, _ := nuevoServicio(t)
	b := crearBalanza(t, svc, "Balanza mostrador", "SERIE-DES")
	if err := svc.DesactivarDispositivoFiscal(empDemo, actorA, origenTst, b.ID); err != nil {
		t.Fatalf("desactivar: %v", err)
	}
	// No se borra: sigue existiendo, pero inactivo.
	var vista *fiscal.DispositivoFiscal
	for _, d := range svc.DispositivosFiscales(empDemo) {
		if d.ID == b.ID {
			dd := d
			vista = &dd
		}
	}
	if vista == nil {
		t.Fatal("desactivar no debe borrar el dispositivo")
	}
	if vista.Activo {
		t.Error("tras desactivar, el dispositivo debe quedar inactivo")
	}
}

func TestDesactivarDispositivo_NoExiste(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if err := svc.DesactivarDispositivoFiscal(empDemo, actorA, origenTst, "disp_fantasma"); !errors.Is(err, application.ErrDispositivoNoExiste) {
		t.Fatalf("desactivar un dispositivo inexistente debe dar ErrDispositivoNoExiste, se obtuvo: %v", err)
	}
}

func TestDispositivosFiscales_OrdenadosPorNombre(t *testing.T) {
	svc, _ := nuevoServicio(t)
	crearBalanza(t, svc, "Zeta", "S-Z")
	crearBalanza(t, svc, "Alfa", "S-A")
	list := svc.DispositivosFiscales(empDemo)
	for i := 1; i < len(list); i++ {
		if list[i-1].Nombre > list[i].Nombre {
			t.Fatalf("los dispositivos deben venir ordenados por nombre, %q antes de %q", list[i-1].Nombre, list[i].Nombre)
		}
	}
}

// TestAsignarDispositivoACaja comprueba las reglas que viven en application: a una
// caja solo se le asigna un dispositivo que EXISTA en el tenant y que sea una
// IMPRESORA fiscal (una balanza no imprime documentos, así que no es asignable).
func TestAsignarDispositivoACaja(t *testing.T) {
	svc, _ := nuevoServicio(t)
	// Un id inexistente se rechaza con ErrDispositivoNoExiste.
	fantasma := "disp_fantasma"
	if _, err := svc.ActualizarCaja(empDemo, actorA, origenTst, caja1, application.CambiosCaja{
		DispositivoFiscalID: &fantasma,
	}); !errors.Is(err, application.ErrDispositivoNoExiste) {
		t.Fatalf("asignar a la caja un dispositivo inexistente debe dar ErrDispositivoNoExiste, se obtuvo: %v", err)
	}
	// Una BALANZA existe pero NO es impresora fiscal: se rechaza en Actualizar…
	bal := crearBalanza(t, svc, "Balanza mostrador", "S-BAL")
	if _, err := svc.ActualizarCaja(empDemo, actorA, origenTst, caja1, application.CambiosCaja{
		DispositivoFiscalID: &bal.ID,
	}); !errors.Is(err, application.ErrDispositivoNoEsImpresora) {
		t.Fatalf("asignar una balanza a la caja debe dar ErrDispositivoNoEsImpresora, se obtuvo: %v", err)
	}
	// …y también en Crear.
	if _, err := svc.CrearCaja(empDemo, sede1, actorA, origenTst, "Caja con balanza", bal.ID); !errors.Is(err, application.ErrDispositivoNoEsImpresora) {
		t.Fatalf("CrearCaja con una balanza debe dar ErrDispositivoNoEsImpresora, se obtuvo: %v", err)
	}
	// Una IMPRESORA fiscal sí es asignable (Crear y Actualizar).
	imp, err := svc.CrearDispositivoFiscal(empDemo, actorA, origenTst, fiscal.DispositivoFiscal{
		Nombre: "Impresora caja", SedeID: sede1, Tipo: fiscal.DispositivoImpresoraFiscal, Serie: "S-IMP",
	})
	if err != nil {
		t.Fatalf("crear impresora: %v", err)
	}
	if _, err := svc.ActualizarCaja(empDemo, actorA, origenTst, caja1, application.CambiosCaja{
		DispositivoFiscalID: &imp.ID,
	}); err != nil {
		t.Fatalf("asignar una impresora fiscal a la caja debe funcionar: %v", err)
	}
	if _, err := svc.CrearCaja(empDemo, sede1, actorA, origenTst, "Caja con impresora", imp.ID); err != nil {
		t.Fatalf("CrearCaja con una impresora fiscal debe funcionar: %v", err)
	}
}

/* --- Catálogo precargado -------------------------------------------------- */

// Lo que hace útil al catálogo no es la lista en sí: es que elegir un modelo
// conocido deje la ficha NORMALIZADA. Si esto no pasa, el maestro se ensucia
// igual que si no existiera el catálogo.
func TestCrearDispositivo_NormalizaGrafiaDelCatalogo(t *testing.T) {
	svc, _ := nuevoServicio(t)
	d, err := svc.CrearDispositivoFiscal(empDemo, actorA, origenTst, fiscal.DispositivoFiscal{
		Nombre: "Caja 1", Tipo: fiscal.DispositivoImpresoraFiscal,
		Marca: "  the factory hka  ", Modelo: "hka80",
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if d.Marca != "The Factory HKA" || d.Modelo != "HKA80" {
		t.Errorf("no normalizó a la grafía del catálogo: %q / %q", d.Marca, d.Modelo)
	}
}

// Elegir una balanza catalogada tiene que bastar: el protocolo y el puerto los
// pone el catálogo, que es justo el dato que nadie en el mostrador se sabe.
func TestCrearDispositivo_BalanzaTomaProtocoloDelCatalogo(t *testing.T) {
	svc, _ := nuevoServicio(t)
	d, err := svc.CrearDispositivoFiscal(empDemo, actorA, origenTst, fiscal.DispositivoFiscal{
		Nombre: "Balanza charcutería", Tipo: fiscal.DispositivoBalanza,
		Marca: "Torrey", Modelo: "PCR-40T",
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if d.Protocolo != "toledo" {
		t.Errorf("protocolo sugerido no se aplicó: %q", d.Protocolo)
	}
	if d.Puerto == "" {
		t.Error("puerto sugerido no se aplicó")
	}
}

// El catálogo SUGIERE, no manda: lo que la persona escribió a mano gana siempre.
func TestCrearDispositivo_CatalogoNoPisaLoQueSeEscribio(t *testing.T) {
	svc, _ := nuevoServicio(t)
	d, err := svc.CrearDispositivoFiscal(empDemo, actorA, origenTst, fiscal.DispositivoFiscal{
		Nombre: "Balanza 2", Tipo: fiscal.DispositivoBalanza,
		Marca: "Torrey", Modelo: "PCR-40T", Protocolo: "prt1", Puerto: "/dev/ttyUSB0",
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if d.Protocolo != "prt1" || d.Puerto != "/dev/ttyUSB0" {
		t.Errorf("el catálogo pisó lo elegido a mano: %q / %q", d.Protocolo, d.Puerto)
	}
}

// Un modelo que no está en la lista NO puede bloquear ni ser reescrito: el
// catálogo es una ayuda, y el mercado siempre va por delante de la lista.
func TestCrearDispositivo_ModeloFueraDelCatalogoSeRespeta(t *testing.T) {
	svc, _ := nuevoServicio(t)
	d, err := svc.CrearDispositivoFiscal(empDemo, actorA, origenTst, fiscal.DispositivoFiscal{
		Nombre: "Nueva", Tipo: fiscal.DispositivoImpresoraFiscal,
		Marca: "Marca Nueva 2027", Modelo: "XZ-1",
	})
	if err != nil {
		t.Fatalf("un modelo fuera del catálogo debe aceptarse: %v", err)
	}
	if d.Marca != "Marca Nueva 2027" || d.Modelo != "XZ-1" {
		t.Errorf("alteró un modelo libre: %q / %q", d.Marca, d.Modelo)
	}
}

// Editar también normaliza: si no, el maestro se ensucia por la puerta de atrás.
func TestActualizarDispositivo_NormalizaGrafiaDelCatalogo(t *testing.T) {
	svc, _ := nuevoServicio(t)
	d := crearBalanza(t, svc, "Balanza deli", "BZ-9")
	marca, modelo := "aclas", "os2x"
	out, err := svc.ActualizarDispositivoFiscal(empDemo, actorA, origenTst, d.ID,
		application.CambiosDispositivoFiscal{Marca: &marca, Modelo: &modelo})
	if err != nil {
		t.Fatalf("actualizar: %v", err)
	}
	if out.Marca != "Aclas" || out.Modelo != "OS2X" {
		t.Errorf("no normalizó al editar: %q / %q", out.Marca, out.Modelo)
	}
}

func TestCatalogoDispositivos_TraeLosTresTiposYSuFecha(t *testing.T) {
	svc, _ := nuevoServicio(t)
	cat := svc.CatalogoDispositivos()
	if cat.Revisado == "" {
		t.Error("el catálogo debe declarar cuándo se revisó")
	}
	tipos := map[string]int{}
	for _, m := range cat.Modelos {
		tipos[m.Tipo]++
	}
	for _, tipo := range []string{"impresora_fiscal", "balanza", "comandera"} {
		if tipos[tipo] == 0 {
			t.Errorf("el catálogo no trae ningún %q", tipo)
		}
	}
}
