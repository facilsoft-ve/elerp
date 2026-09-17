// Package dispositivo contiene el CATÁLOGO PRECARGADO del hardware que se usa en
// el mercado venezolano: impresoras fiscales homologadas por el SENIAT, balanzas
// de mostrador y comanderas térmicas.
//
// POR QUÉ EXISTE: sin esto, configurar un dispositivo es teclear "marca" y
// "modelo" a mano en dos campos libres. El resultado real es un maestro sucio
// ("HKA80", "hka 80", "The Factory HKA80", "hka-80" son la misma máquina) y, peor,
// un dato del que el agente fiscal local no puede deducir NADA: ni el protocolo,
// ni el ancho del papel, ni el puerto por defecto. Con el catálogo, elegir el
// modelo PRECONFIGURA la conexión y deja el maestro normalizado.
//
// NO ES UN DRIVER. Acá no se habla con el hardware —eso es del agente fiscal
// local (SAD §9.3), un binario aparte—. Esto es la ficha: qué existe en el
// mercado, cómo se llama y con qué valores suele conectarse.
//
// ES DATO, NO CÓDIGO (principio 1, «cumplimiento como configuración»): la lista
// de homologados del SENIAT cambia con cada providencia, así que esto se edita y
// se versiona (ver Revisado) sin tocar la lógica. Un modelo que no esté en la
// lista NO bloquea nada: la ficha admite marca/modelo libres.
package dispositivo

import "strings"

// Tipos de dispositivo del catálogo. Los dos primeros coinciden con los tipos de
// fiscal.DispositivoFiscal; "comandera" es la impresora térmica NO fiscal del
// módulo Restaurante (cocina.Impresora), que no es un dispositivo fiscal pero se
// elige igual y merece el mismo catálogo.
const (
	TipoImpresoraFiscal = "impresora_fiscal"
	TipoBalanza         = "balanza"
	TipoComandera       = "comandera"
)

// Tecnologías de impresión (solo informativo, para que quien elige reconozca su
// máquina: la matricial es la ruidosa de papel continuo, la térmica la de rollo).
const (
	TecTermica   = "termica"
	TecMatricial = "matricial"
)

// Revisado es la fecha (UTC, AAAA-MM-DD) de la última revisión de esta lista
// contra el mercado. La interfaz la muestra para que nadie asuma que está al día:
// la homologación del SENIAT es un trámite vivo y esta lista es una AYUDA, no la
// fuente legal. La fuente legal es la providencia vigente.
const Revisado = "2026-09-16"

// Modelo es una ficha del catálogo. Los campos de conexión son SUGERENCIAS que
// precargan el formulario; quien configura siempre los puede cambiar.
type Modelo struct {
	Marca  string `json:"marca"`
	Modelo string `json:"modelo"`
	Tipo   string `json:"tipo"`
	// Tecnologia: termica | matricial. Vacío en balanzas.
	Tecnologia string `json:"tecnologia,omitempty"`
	// Protocolo sugerido (balanzas): cómo entrega el peso. Coincide con los ids
	// que ya maneja la ficha de balanza (prt1, dialog06, pedido, toledo, …).
	Protocolo string `json:"protocolo,omitempty"`
	// Puerto sugerido (balanzas e impresoras locales): COM3, /dev/ttyUSB0, …
	Puerto string `json:"puerto,omitempty"`
	// AnchoMM sugerido (comanderas): 58 u 80 mm de papel.
	AnchoMM int `json:"anchoMm,omitempty"`
	// Conexion sugerida (comanderas): local | red.
	Conexion string `json:"conexion,omitempty"`
	// Nota es la pista que ayuda a reconocer el equipo o a no equivocarse.
	Nota string `json:"nota,omitempty"`
}

// catalogo es la lista precargada. Orden: por tipo, y dentro de cada tipo por
// presencia real en el mercado venezolano (lo más común primero), NO alfabético:
// quien configura reconoce su máquina más rápido si lo frecuente está arriba.
var catalogo = []Modelo{
	// --- Impresoras fiscales (homologadas SENIAT) -------------------------
	// The Factory HKA es el fabricante dominante en Venezuela; la HKA80 es la
	// máquina más extendida del país.
	{Marca: "The Factory HKA", Modelo: "HKA80", Tipo: TipoImpresoraFiscal, Tecnologia: TecTermica, Nota: "La más extendida en Venezuela. Térmica, corte automático."},
	{Marca: "The Factory HKA", Modelo: "HKA112", Tipo: TipoImpresoraFiscal, Tecnologia: TecMatricial, Nota: "Matricial de impacto: copia física, para facturación de mayor volumen."},
	{Marca: "The Factory HKA", Modelo: "SRP-350", Tipo: TipoImpresoraFiscal, Tecnologia: TecTermica},
	{Marca: "The Factory HKA", Modelo: "SRP-270", Tipo: TipoImpresoraFiscal, Tecnologia: TecMatricial},
	{Marca: "The Factory HKA", Modelo: "SRP-812", Tipo: TipoImpresoraFiscal, Tecnologia: TecTermica},
	{Marca: "The Factory HKA", Modelo: "PP-80", Tipo: TipoImpresoraFiscal, Tecnologia: TecTermica},

	{Marca: "Aclas", Modelo: "PP9-PLUS", Tipo: TipoImpresoraFiscal, Tecnologia: TecTermica, Nota: "Térmica de entrada, común en pequeños y medianos comercios."},
	{Marca: "Aclas", Modelo: "PP9", Tipo: TipoImpresoraFiscal, Tecnologia: TecTermica},
	{Marca: "Aclas", Modelo: "CR2050", Tipo: TipoImpresoraFiscal, Tecnologia: TecTermica, Nota: "Registradora fiscal (no impresora): opera sola, sin PC."},

	{Marca: "Bematech", Modelo: "MP-4200 TH FI", Tipo: TipoImpresoraFiscal, Tecnologia: TecTermica},
	{Marca: "Bematech", Modelo: "MP-4000 TH FI", Tipo: TipoImpresoraFiscal, Tecnologia: TecTermica},
	{Marca: "Bematech", Modelo: "MP-2100 TH FI", Tipo: TipoImpresoraFiscal, Tecnologia: TecTermica},
	{Marca: "Bematech", Modelo: "MP-20 MI", Tipo: TipoImpresoraFiscal, Tecnologia: TecMatricial},

	{Marca: "Epson", Modelo: "TM-T88V Fiscal", Tipo: TipoImpresoraFiscal, Tecnologia: TecTermica},
	{Marca: "Epson", Modelo: "PF-300", Tipo: TipoImpresoraFiscal, Tecnologia: TecTermica},
	{Marca: "Epson", Modelo: "PF-220", Tipo: TipoImpresoraFiscal, Tecnologia: TecMatricial},

	{Marca: "Dascom", Modelo: "DT-230 FE", Tipo: TipoImpresoraFiscal, Tecnologia: TecTermica},
	{Marca: "Dascom", Modelo: "Tally 1125", Tipo: TipoImpresoraFiscal, Tecnologia: TecMatricial},

	{Marca: "Bixolon", Modelo: "SRP-350", Tipo: TipoImpresoraFiscal, Tecnologia: TecTermica},
	{Marca: "Bixolon", Modelo: "SRP-270", Tipo: TipoImpresoraFiscal, Tecnologia: TecMatricial},

	{Marca: "Vmax", Modelo: "V1000", Tipo: TipoImpresoraFiscal, Tecnologia: TecTermica},
	{Marca: "Vmax", Modelo: "801", Tipo: TipoImpresoraFiscal, Tecnologia: TecTermica},

	{Marca: "Hasar", Modelo: "SMH/P 715", Tipo: TipoImpresoraFiscal, Tecnologia: TecMatricial},

	// --- Balanzas de mostrador --------------------------------------------
	// El protocolo es lo que de verdad importa: dice cómo la balanza entrega el
	// peso. El puerto por defecto es el típico en Windows (COM) porque el equipo
	// del mostrador casi siempre lo es; en Linux se cambia a /dev/ttyUSB0.
	{Marca: "Aclas", Modelo: "OS2X", Tipo: TipoBalanza, Protocolo: "aclas", Puerto: "COM3"},
	{Marca: "Aclas", Modelo: "LS2", Tipo: TipoBalanza, Protocolo: "aclas", Puerto: "COM3"},
	{Marca: "Torrey", Modelo: "PCR-40T", Tipo: TipoBalanza, Protocolo: "toledo", Puerto: "COM3"},
	{Marca: "Torrey", Modelo: "L-EQ", Tipo: TipoBalanza, Protocolo: "toledo", Puerto: "COM3"},
	{Marca: "Mettler Toledo", Modelo: "8217", Tipo: TipoBalanza, Protocolo: "toledo", Puerto: "COM3", Nota: "El protocolo 8217 de Toledo es el más difundido; muchas balanzas lo emulan."},
	{Marca: "Excell", Modelo: "ESW", Tipo: TipoBalanza, Protocolo: "prt1", Puerto: "COM3"},
	{Marca: "Rhino", Modelo: "BAPRE-40", Tipo: TipoBalanza, Protocolo: "prt1", Puerto: "COM3"},
	{Marca: "Systel", Modelo: "Croma", Tipo: TipoBalanza, Protocolo: "systel", Puerto: "COM3"},
	{Marca: "Kretz", Modelo: "Report", Tipo: TipoBalanza, Protocolo: "kretz", Puerto: "COM3"},
	{Marca: "Ohaus", Modelo: "Serie RC", Tipo: TipoBalanza, Protocolo: "pedido", Puerto: "COM3", Nota: "Entrega el peso solo cuando se le pide (ENQ)."},

	// --- Comanderas (térmicas ESC/POS, NO fiscales) ------------------------
	// Son impresoras de cocina/barra: no llevan homologación porque no emiten
	// documento fiscal. Casi todas hablan ESC/POS; lo que cambia es el ancho del
	// papel y si están en la red (RAW 9100) o colgadas del equipo.
	{Marca: "Epson", Modelo: "TM-T20III", Tipo: TipoComandera, Tecnologia: TecTermica, AnchoMM: 80, Conexion: "red", Nota: "El caballo de batalla: robusta y fácil de conseguir."},
	{Marca: "Epson", Modelo: "TM-T20II", Tipo: TipoComandera, Tecnologia: TecTermica, AnchoMM: 80, Conexion: "red"},
	{Marca: "Epson", Modelo: "TM-T88VI", Tipo: TipoComandera, Tecnologia: TecTermica, AnchoMM: 80, Conexion: "red"},
	{Marca: "Epson", Modelo: "TM-T88V", Tipo: TipoComandera, Tecnologia: TecTermica, AnchoMM: 80, Conexion: "red"},
	{Marca: "Epson", Modelo: "TM-U220", Tipo: TipoComandera, Tecnologia: TecMatricial, AnchoMM: 76, Conexion: "red", Nota: "Matricial: aguanta el calor y el vapor de la cocina mejor que la térmica."},
	{Marca: "Bixolon", Modelo: "SRP-330III", Tipo: TipoComandera, Tecnologia: TecTermica, AnchoMM: 80, Conexion: "red"},
	{Marca: "Bixolon", Modelo: "SRP-350III", Tipo: TipoComandera, Tecnologia: TecTermica, AnchoMM: 80, Conexion: "red"},
	{Marca: "Star Micronics", Modelo: "TSP143III", Tipo: TipoComandera, Tecnologia: TecTermica, AnchoMM: 80, Conexion: "red"},
	{Marca: "Star Micronics", Modelo: "TSP100", Tipo: TipoComandera, Tecnologia: TecTermica, AnchoMM: 80, Conexion: "local"},
	{Marca: "Xprinter", Modelo: "XP-80C", Tipo: TipoComandera, Tecnologia: TecTermica, AnchoMM: 80, Conexion: "red", Nota: "Económica y muy común en el mercado local."},
	{Marca: "Xprinter", Modelo: "XP-N160II", Tipo: TipoComandera, Tecnologia: TecTermica, AnchoMM: 80, Conexion: "red"},
	{Marca: "Xprinter", Modelo: "XP-58IIH", Tipo: TipoComandera, Tecnologia: TecTermica, AnchoMM: 58, Conexion: "local"},
	{Marca: "3nStar", Modelo: "RPT-006", Tipo: TipoComandera, Tecnologia: TecTermica, AnchoMM: 80, Conexion: "red"},
	{Marca: "3nStar", Modelo: "RPT-008", Tipo: TipoComandera, Tecnologia: TecTermica, AnchoMM: 80, Conexion: "red"},
	{Marca: "EC Line", Modelo: "EC-PM-80360", Tipo: TipoComandera, Tecnologia: TecTermica, AnchoMM: 80, Conexion: "red"},
	{Marca: "Zjiang", Modelo: "ZJ-8330", Tipo: TipoComandera, Tecnologia: TecTermica, AnchoMM: 80, Conexion: "red"},
	{Marca: "Genérica", Modelo: "POS-80 (ESC/POS)", Tipo: TipoComandera, Tecnologia: TecTermica, AnchoMM: 80, Conexion: "red", Nota: "Para las clonas sin marca: casi todas responden ESC/POS por el 9100."},
	{Marca: "Genérica", Modelo: "POS-58 (ESC/POS)", Tipo: TipoComandera, Tecnologia: TecTermica, AnchoMM: 58, Conexion: "local"},
}

// TipoValido indica si el tipo es uno de los del catálogo.
func TipoValido(t string) bool {
	return t == TipoImpresoraFiscal || t == TipoBalanza || t == TipoComandera
}

// Catalogo devuelve una COPIA de todo el catálogo. Copia y no el slice interno
// para que nadie lo mute por accidente desde otra capa.
func Catalogo() []Modelo {
	out := make([]Modelo, len(catalogo))
	copy(out, catalogo)
	return out
}

// De devuelve los modelos de un tipo, en el orden del catálogo (lo más común
// primero). Tipo desconocido ⇒ lista vacía.
func De(tipo string) []Modelo {
	out := []Modelo{}
	for _, m := range catalogo {
		if m.Tipo == tipo {
			out = append(out, m)
		}
	}
	return out
}

// Marcas devuelve las marcas distintas de un tipo, sin repetir y en el orden en
// que aparecen en el catálogo.
func Marcas(tipo string) []string {
	out := []string{}
	visto := map[string]bool{}
	for _, m := range catalogo {
		if m.Tipo != tipo || visto[m.Marca] {
			continue
		}
		visto[m.Marca] = true
		out = append(out, m.Marca)
	}
	return out
}

// Buscar localiza una ficha por tipo + marca + modelo, sin distinguir mayúsculas
// ni espacios sobrantes. Es lo que permite, dado un dispositivo ya guardado,
// recuperar sus valores sugeridos. Devuelve false si no está en el catálogo (un
// modelo libre es perfectamente válido).
func Buscar(tipo, marca, modelo string) (Modelo, bool) {
	for _, m := range catalogo {
		if m.Tipo == tipo && igual(m.Marca, marca) && igual(m.Modelo, modelo) {
			return m, true
		}
	}
	return Modelo{}, false
}

func igual(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
