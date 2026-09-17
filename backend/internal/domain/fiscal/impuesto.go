package fiscal

import (
	"sort"
	"strings"
)

// MAESTRO DE IMPUESTOS: las alícuotas de IVA que la empresa puede aplicar.
//
// POR QUÉ EXISTE: hasta acá el IVA era una constante (16%) y un producto solo
// podía ser «gravado» o «exento». En Venezuela eso no alcanza — conviven tres
// tasas a la vez:
//
//   - la GENERAL (16%),
//   - una REDUCIDA (8%) para ciertos alimentos y servicios,
//   - y la ADICIONAL de bienes suntuarios, que NO reemplaza a la general: se
//     SUMA. Un artículo de lujo paga 16% + 15% = 31%, y el SENIAT exige que las
//     dos porciones se declaren POR SEPARADO en el libro de ventas. Por eso la
//     adicional se modela como un recargo y no como una tasa de 31%: si se
//     guardara el total, la declaración ya no se podría descomponer.
//
// Y CON VIGENCIA: la alícuota cambia por providencia. El cálculo de un documento
// tiene que usar la tasa que regía EL DÍA del hecho imponible, nunca la de hoy
// (ADR de reglas como dato versionado + Art. 177). De ahí VigenteDesde/Hasta.

// Tipos de alícuota. El tipo manda sobre el código: dice CÓMO participa en el
// cálculo, no cómo se llama.
const (
	// TipoGeneral es la tasa ordinaria (16%).
	TipoGeneral = "general"
	// TipoReducida es la tasa rebajada de ciertos bienes y servicios (8%).
	TipoReducida = "reducida"
	// TipoAdicional es el RECARGO de bienes suntuarios: se aplica ADEMÁS de la
	// general sobre la misma base. La contadora lo llama «alícuota adicional».
	TipoAdicional = "adicional"
	// TipoExento no causa impuesto, pero su base SÍ se declara (aparte).
	TipoExento = "exento"
)

// TipoAlicuotaValido acota el tipo a los cuatro que sabe calcular el motor.
func TipoAlicuotaValido(t string) bool {
	switch t {
	case TipoGeneral, TipoReducida, TipoAdicional, TipoExento:
		return true
	}
	return false
}

// Códigos de las alícuotas que se siembran por defecto. Un producto apunta a un
// código, no a un porcentaje: así cambiar la tasa por providencia no obliga a
// tocar el catálogo entero.
const (
	CodGeneral   = "general"
	CodReducida  = "reducida"
	CodSuntuario = "suntuario" // general + adicional
	CodExento    = "exento"
)

// Alicuota es una tasa del maestro, vigente en un rango de fechas.
type Alicuota struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	// Codigo es la referencia estable que guarda el producto ("general",
	// "reducida", "suntuario"…). No cambia aunque cambie el porcentaje.
	Codigo string `json:"codigo" bson:"codigo"`
	Nombre string `json:"nombre" bson:"nombre"` // "General 16%"
	Tipo   string `json:"tipo" bson:"tipo"`
	// Porcentaje en FRACCIÓN (0.16 = 16%). Para el tipo adicional es SOLO el
	// recargo (0.15), no el total: la general se suma aparte.
	Porcentaje float64 `json:"porcentaje" bson:"porcentaje"`
	// Adicional es el recargo suntuario que acompaña a esta alícuota, cuando la
	// clasificación lleva las dos (el caso del lujo: 16% + 15%). 0 = sin recargo.
	Adicional float64 `json:"adicional" bson:"adicional"`
	// VigenteDesde y VigenteHasta acotan cuándo rige, en AAAA-MM-DD. Hasta vacío
	// = sigue vigente. Dos filas del mismo código con rangos distintos son la
	// MISMA alícuota antes y después de una providencia.
	VigenteDesde string `json:"vigenteDesde" bson:"vigentedesde"`
	VigenteHasta string `json:"vigenteHasta" bson:"vigentehasta"`
	// Activa permite retirar una clasificación sin borrar el histórico.
	Activa bool `json:"activa" bson:"activa"`
}

// RigeEn indica si esta fila estaba vigente en una fecha (AAAA-MM-DD). Las
// fechas se comparan como texto, que para AAAA-MM-DD ordena igual que como día.
func (a Alicuota) RigeEn(fecha string) bool {
	f := fechaCorta(fecha)
	if f == "" {
		return false
	}
	if a.VigenteDesde != "" && f < a.VigenteDesde {
		return false
	}
	if a.VigenteHasta != "" && f > a.VigenteHasta {
		return false
	}
	return true
}

// Grava indica si esta alícuota causa impuesto.
func (a Alicuota) Grava() bool { return a.Tipo != TipoExento }

// fechaCorta recorta un RFC3339 (o un AAAA-MM-DD) a su día.
func fechaCorta(fecha string) string {
	f := strings.TrimSpace(fecha)
	if len(f) < 10 {
		return ""
	}
	return f[:10]
}

// VigenteEn elige, de un juego de alícuotas, la del código que regía en esa
// fecha. Devuelve false si no hay ninguna: quien llama decide si eso es un error
// o si cae a la tasa del sistema.
//
// Ante varias que rijan a la vez (configuración solapada, que no debería pasar)
// gana la de VigenteDesde MÁS RECIENTE: es la que se cargó después.
func VigenteEn(alicuotas []Alicuota, codigo, fecha string) (Alicuota, bool) {
	cod := strings.TrimSpace(strings.ToLower(codigo))
	candidatas := []Alicuota{}
	for _, a := range alicuotas {
		if strings.ToLower(a.Codigo) == cod && a.RigeEn(fecha) {
			candidatas = append(candidatas, a)
		}
	}
	if len(candidatas) == 0 {
		return Alicuota{}, false
	}
	sort.SliceStable(candidatas, func(i, j int) bool {
		return candidatas[i].VigenteDesde > candidatas[j].VigenteDesde
	})
	return candidatas[0], true
}

// AlicuotasPorDefecto es el juego que se siembra para una empresa nueva: las
// tres tasas venezolanas más la clasificación exenta. Son DATO, no código —
// editables y con vigencia—, que es lo que permite absorber una providencia sin
// redesplegar (principio 1).
func AlicuotasPorDefecto(empresaID, desde string) []Alicuota {
	return []Alicuota{
		{EmpresaID: empresaID, Codigo: CodGeneral, Nombre: "General (16%)", Tipo: TipoGeneral,
			Porcentaje: 0.16, VigenteDesde: desde, Activa: true},
		{EmpresaID: empresaID, Codigo: CodReducida, Nombre: "Reducida (8%)", Tipo: TipoReducida,
			Porcentaje: 0.08, VigenteDesde: desde, Activa: true},
		// El lujo NO es una tasa de 31%: es la general más un recargo de 15%. Se
		// guardan separadas porque el libro de ventas las declara en columnas
		// distintas.
		{EmpresaID: empresaID, Codigo: CodSuntuario, Nombre: "Suntuaria (16% + 15% adicional)", Tipo: TipoGeneral,
			Porcentaje: 0.16, Adicional: 0.15, VigenteDesde: desde, Activa: true},
		{EmpresaID: empresaID, Codigo: CodExento, Nombre: "Exento", Tipo: TipoExento,
			Porcentaje: 0, VigenteDesde: desde, Activa: true},
	}
}

// AlicuotaRepo persiste el maestro de impuestos, aislado por empresa. Editable
// (no es un ledger), pero las filas viejas NO se tocan al cambiar una tasa: se
// cierra la vigente y se abre otra, o el histórico dejaría de cuadrar.
type AlicuotaRepo interface {
	List(empresaID string) []Alicuota
	ByID(empresaID, id string) (Alicuota, bool)
	Create(a Alicuota) Alicuota
	Update(a Alicuota) (Alicuota, bool)
}
