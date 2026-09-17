package dispositivo

import "testing"

// El catálogo es DATO, y el dato se degrada solo: alguien agrega un modelo con
// el tipo mal escrito, o repite una entrada, y la pantalla lo muestra roto sin
// que nada falle. Estas pruebas son la reja de esa lista.

func TestCatalogoEntradasBienFormadas(t *testing.T) {
	visto := map[string]bool{}
	for _, m := range Catalogo() {
		if m.Marca == "" || m.Modelo == "" {
			t.Errorf("entrada sin marca o modelo: %+v", m)
		}
		if !TipoValido(m.Tipo) {
			t.Errorf("%s %s: tipo inválido %q", m.Marca, m.Modelo, m.Tipo)
		}
		clave := m.Tipo + "|" + m.Marca + "|" + m.Modelo
		if visto[clave] {
			t.Errorf("entrada duplicada: %s", clave)
		}
		visto[clave] = true
	}
}

// Una comandera sin ancho de papel no sirve para precargar nada, que es justo
// para lo que existe la ficha; y el ancho solo puede ser uno de los dos rollos
// que existen en el mercado.
func TestComanderasTraenAnchoYConexionUtiles(t *testing.T) {
	for _, m := range De(TipoComandera) {
		if m.AnchoMM != 58 && m.AnchoMM != 80 && m.AnchoMM != 76 {
			t.Errorf("%s %s: ancho %d mm no es un rollo real", m.Marca, m.Modelo, m.AnchoMM)
		}
		if m.Conexion != "local" && m.Conexion != "red" {
			t.Errorf("%s %s: conexión sugerida inválida %q", m.Marca, m.Modelo, m.Conexion)
		}
	}
}

// El protocolo es LO ÚNICO que de verdad hace falta para hablarle a una balanza:
// una ficha de balanza sin protocolo sugerido no ahorra ningún trabajo.
func TestBalanzasTraenProtocolo(t *testing.T) {
	balanzas := De(TipoBalanza)
	if len(balanzas) == 0 {
		t.Fatal("el catálogo no trae balanzas")
	}
	for _, m := range balanzas {
		if m.Protocolo == "" {
			t.Errorf("%s %s: sin protocolo sugerido", m.Marca, m.Modelo)
		}
	}
}

func TestDeFiltraPorTipoYTipoDesconocidoNoRompe(t *testing.T) {
	fiscales := De(TipoImpresoraFiscal)
	if len(fiscales) == 0 {
		t.Fatal("el catálogo no trae impresoras fiscales")
	}
	for _, m := range fiscales {
		if m.Tipo != TipoImpresoraFiscal {
			t.Fatalf("De() devolvió un %q", m.Tipo)
		}
	}
	if got := De("teletransportador"); len(got) != 0 {
		t.Errorf("tipo desconocido debería dar lista vacía, dio %d", len(got))
	}
}

func TestMarcasNoRepiteYRespetaElOrden(t *testing.T) {
	marcas := Marcas(TipoImpresoraFiscal)
	if len(marcas) == 0 {
		t.Fatal("sin marcas de impresora fiscal")
	}
	if marcas[0] != "The Factory HKA" {
		t.Errorf("la marca dominante en Venezuela debería ir primero, fue %q", marcas[0])
	}
	visto := map[string]bool{}
	for _, m := range marcas {
		if visto[m] {
			t.Errorf("marca repetida: %s", m)
		}
		visto[m] = true
	}
}

// Buscar es lo que normaliza el maestro: quien teclea "hka80" en minúsculas o
// con espacios de más tiene que caer en la MISMA ficha.
func TestBuscarIgnoraMayusculasYEspacios(t *testing.T) {
	m, ok := Buscar(TipoImpresoraFiscal, "  the factory hka ", "HKA80")
	if !ok {
		t.Fatal("no encontró la HKA80 escrita distinto")
	}
	if m.Marca != "The Factory HKA" || m.Modelo != "HKA80" {
		t.Errorf("no devolvió la grafía canónica: %q / %q", m.Marca, m.Modelo)
	}
}

// El tipo forma parte de la identidad: hay marcas (Epson, Bixolon, Aclas) que
// están en más de un tipo, y una comandera Epson no es una impresora fiscal.
func TestBuscarNoCruzaTipos(t *testing.T) {
	if _, ok := Buscar(TipoComandera, "The Factory HKA", "HKA80"); ok {
		t.Error("una impresora fiscal no debería aparecer como comandera")
	}
	if _, ok := Buscar(TipoImpresoraFiscal, "Xprinter", "XP-80C"); ok {
		t.Error("una comandera no debería aparecer como impresora fiscal")
	}
}

func TestBuscarModeloDesconocidoNoEncuentra(t *testing.T) {
	if _, ok := Buscar(TipoImpresoraFiscal, "Marca Que No Existe", "ZZ-1"); ok {
		t.Error("encontró un modelo que no está en el catálogo")
	}
}

// El catálogo se entrega por copia: mutarlo desde afuera no puede envenenar la
// lista para el resto del proceso.
func TestCatalogoSeEntregaPorCopia(t *testing.T) {
	c := Catalogo()
	original := c[0].Marca
	c[0].Marca = "MUTADO"
	if Catalogo()[0].Marca != original {
		t.Error("mutar la copia alteró el catálogo interno")
	}
}
