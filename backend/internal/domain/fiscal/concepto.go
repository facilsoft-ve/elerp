package fiscal

import (
	"sort"
	"strings"
)

// MAESTRO DE CONCEPTOS ISLR: la tabla de conceptos retenibles con su tarifa y su
// sustraendo.
//
// POR QUÉ EXISTE (nota de la contadora, 15:53): «es necesario cargar tabla de
// conceptos y % de retención de impuesto sobre la renta que sean configurables
// en los productos tipo servicios».
//
// Hoy el concepto, el porcentaje y el sustraendo se TECLEAN en cada comprobante.
// Eso tiene dos problemas que no se ven hasta que duelen:
//
//   - quien registra la retención tiene que saberse de memoria la tarifa del
//     reglamento, y un dedo de más se convierte en un impuesto mal enterado;
//   - el mismo concepto termina escrito de cinco maneras («honorarios»,
//     «Honorarios Profesionales», «hon. prof.»), y después no hay forma de
//     declarar ni de auditar por concepto.
//
// Con el maestro, el concepto se ELIGE y la tarifa viene con él. El comprobante
// sigue guardando porcentaje y sustraendo COPIADOS (es append-only y
// autocontenido): cambiar la tabla mañana no puede alterar lo ya retenido.

// Tipos de sujeto: la tarifa de ISLR depende de a quién se le retiene. Es la
// distinción del reglamento y la razón por la que un mismo concepto tiene dos
// tarifas.
const (
	// SujetoNaturalResidente: persona natural residente.
	SujetoNaturalResidente = "natural_residente"
	// SujetoJuridicaDomiciliada: persona jurídica domiciliada.
	SujetoJuridicaDomiciliada = "juridica_domiciliada"
)

// SujetoValido acota el tipo de sujeto.
func SujetoValido(s string) bool {
	return s == SujetoNaturalResidente || s == SujetoJuridicaDomiciliada
}

// ConceptoISLR es una fila del maestro: un concepto retenible, para un tipo de
// sujeto, con su tarifa y su sustraendo.
type ConceptoISLR struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	// Codigo es la referencia estable del concepto ("honorarios",
	// "arrendamiento_inmuebles"…). Lo guarda el producto de servicio.
	Codigo string `json:"codigo" bson:"codigo"`
	Nombre string `json:"nombre" bson:"nombre"`
	Sujeto string `json:"sujeto" bson:"sujeto"`
	// Porcentaje es la tarifa en PORCENTAJE (3 = 3%), igual que Retencion.Porcentaje
	// — se guarda en la misma unidad a propósito, para que no haya que convertir
	// entre el maestro y el comprobante y equivocarse por un factor de 100.
	Porcentaje float64 `json:"porcentaje" bson:"porcentaje"`
	// Sustraendo se RESTA al impuesto calculado (tabla del reglamento). Aplica a
	// personas naturales; en jurídicas es 0.
	Sustraendo float64 `json:"sustraendo" bson:"sustraendo"`
	// BaseMinima es el monto a partir del cual se retiene. Por debajo no hay
	// retención — sin esto se emitirían comprobantes por montos irrisorios que
	// después hay que anular a mano.
	BaseMinima float64 `json:"baseMinima" bson:"baseminima"`
	Activo     bool    `json:"activo" bson:"activo"`
}

// Retener calcula el impuesto a retener sobre una base con este concepto.
// Devuelve 0 cuando la base no llega al mínimo o cuando el sustraendo se come el
// resultado: una retención negativa no existe, y redondearla hacia arriba sería
// cobrarle de más al proveedor.
func (c ConceptoISLR) Retener(base float64) float64 {
	if base < c.BaseMinima {
		return 0
	}
	monto := base*c.Porcentaje/100 - c.Sustraendo
	if monto < 0 {
		return 0
	}
	return monto
}

// ConceptosPorDefecto son los conceptos más usados del reglamento venezolano,
// para que una empresa no arranque con la tabla vacía. Son DATO editable: cada
// quien ajusta su tabla, y la tarifa real la define el reglamento vigente.
//
// OJO: esta lista es un PUNTO DE PARTIDA, no una fuente legal. La contadora
// tiene que revisarla contra el reglamento antes de usarla en producción.
func ConceptosPorDefecto(empresaID string) []ConceptoISLR {
	return []ConceptoISLR{
		{EmpresaID: empresaID, Codigo: "honorarios", Nombre: "Honorarios profesionales",
			Sujeto: SujetoNaturalResidente, Porcentaje: 3, Activo: true},
		{EmpresaID: empresaID, Codigo: "honorarios", Nombre: "Honorarios profesionales",
			Sujeto: SujetoJuridicaDomiciliada, Porcentaje: 5, Activo: true},
		{EmpresaID: empresaID, Codigo: "servicios", Nombre: "Servicios en general",
			Sujeto: SujetoNaturalResidente, Porcentaje: 3, Activo: true},
		{EmpresaID: empresaID, Codigo: "servicios", Nombre: "Servicios en general",
			Sujeto: SujetoJuridicaDomiciliada, Porcentaje: 2, Activo: true},
		{EmpresaID: empresaID, Codigo: "arrendamiento_inmuebles", Nombre: "Arrendamiento de inmuebles",
			Sujeto: SujetoNaturalResidente, Porcentaje: 3, Activo: true},
		{EmpresaID: empresaID, Codigo: "arrendamiento_inmuebles", Nombre: "Arrendamiento de inmuebles",
			Sujeto: SujetoJuridicaDomiciliada, Porcentaje: 5, Activo: true},
		{EmpresaID: empresaID, Codigo: "fletes", Nombre: "Fletes y transporte",
			Sujeto: SujetoJuridicaDomiciliada, Porcentaje: 3, Activo: true},
		{EmpresaID: empresaID, Codigo: "publicidad", Nombre: "Publicidad y propaganda",
			Sujeto: SujetoJuridicaDomiciliada, Porcentaje: 5, Activo: true},
	}
}

// ConceptoPara busca en el maestro el concepto activo de un código para un tipo
// de sujeto. Es la consulta que resuelve «cuánto le retengo a ESTE proveedor por
// ESTE servicio».
func ConceptoPara(conceptos []ConceptoISLR, codigo, sujeto string) (ConceptoISLR, bool) {
	cod := strings.TrimSpace(strings.ToLower(codigo))
	for _, c := range conceptos {
		if strings.ToLower(c.Codigo) == cod && c.Sujeto == sujeto && c.Activo {
			return c, true
		}
	}
	return ConceptoISLR{}, false
}

// NombreDeConcepto devuelve el nombre legible de un código, sin importar el
// sujeto. Sirve para NOMBRAR un concepto que el maestro conoce pero del que no
// tiene tarifa para cierto sujeto: «Fletes y transporte» dice mucho más que
// «fletes» cuando hay que explicar por qué no se retuvo.
func NombreDeConcepto(conceptos []ConceptoISLR, codigo string) string {
	cod := strings.TrimSpace(strings.ToLower(codigo))
	for _, c := range conceptos {
		if strings.ToLower(c.Codigo) == cod {
			return c.Nombre
		}
	}
	return ""
}

// ExisteConcepto dice si el maestro conoce ese código para ALGÚN sujeto. Es lo
// que valida la ficha de producto: el producto clasifica el CONCEPTO (qué se
// paga) y el sujeto lo pone el proveedor (a quién se le paga), así que exigir
// acá un sujeto sería pedirle a la ficha algo que no sabe.
func ExisteConcepto(conceptos []ConceptoISLR, codigo string) bool {
	cod := strings.TrimSpace(strings.ToLower(codigo))
	for _, c := range conceptos {
		if strings.ToLower(c.Codigo) == cod && c.Activo {
			return true
		}
	}
	return false
}

// CodigosDeConcepto devuelve los códigos distintos del maestro, ordenados. Es lo
// que ofrece el selector de la ficha de producto.
func CodigosDeConcepto(conceptos []ConceptoISLR) []string {
	visto := map[string]bool{}
	out := []string{}
	for _, c := range conceptos {
		if c.Activo && !visto[c.Codigo] {
			visto[c.Codigo] = true
			out = append(out, c.Codigo)
		}
	}
	sort.Strings(out)
	return out
}

// ConceptoISLRRepo persiste el maestro de conceptos, aislado por empresa.
// Editable (es configuración, no un ledger): lo retenido ya quedó copiado en su
// comprobante y no depende de esta tabla.
type ConceptoISLRRepo interface {
	List(empresaID string) []ConceptoISLR
	ByID(empresaID, id string) (ConceptoISLR, bool)
	Create(c ConceptoISLR) ConceptoISLR
	Update(c ConceptoISLR) (ConceptoISLR, bool)
}
