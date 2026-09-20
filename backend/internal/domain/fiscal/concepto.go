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

// Tipos de sujeto: la tarifa de ISLR depende de a quién se le retiene. Son las
// CUATRO combinaciones del reglamento —naturaleza (natural o jurídica) por
// residencia (residente/domiciliado o no)— y la razón por la que un mismo
// concepto tiene varias tarifas.
//
// Las dos no domiciliadas no son un caso exótico: a un proveedor del exterior se
// le retiene, y a tarifas bastante más altas. Sin ellas, un pago al exterior se
// clasificaría como domiciliado —la única opción disponible— y retendría de menos.
const (
	// SujetoNaturalResidente: persona natural residente en el país.
	SujetoNaturalResidente = "natural_residente"
	// SujetoNaturalNoResidente: persona natural no residente.
	SujetoNaturalNoResidente = "natural_no_residente"
	// SujetoJuridicaDomiciliada: persona jurídica domiciliada en el país.
	SujetoJuridicaDomiciliada = "juridica_domiciliada"
	// SujetoJuridicaNoDomiciliada: persona jurídica no domiciliada.
	SujetoJuridicaNoDomiciliada = "juridica_no_domiciliada"
)

// SujetoValido acota el tipo de sujeto.
func SujetoValido(s string) bool {
	switch s {
	case SujetoNaturalResidente, SujetoNaturalNoResidente,
		SujetoJuridicaDomiciliada, SujetoJuridicaNoDomiciliada:
		return true
	}
	return false
}

// SujetoNombre escribe el tipo de sujeto como se lee en un comprobante.
func SujetoNombre(s string) string {
	switch s {
	case SujetoNaturalResidente:
		return "Persona natural residente"
	case SujetoNaturalNoResidente:
		return "Persona natural no residente"
	case SujetoJuridicaDomiciliada:
		return "Persona jurídica domiciliada"
	case SujetoJuridicaNoDomiciliada:
		return "Persona jurídica no domiciliada"
	}
	return s
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
	// SustraendoUT es el sustraendo de la tabla del reglamento, EN UNIDADES
	// TRIBUTARIAS (83,33 para personas naturales residentes). Aplica a personas
	// naturales; en jurídicas es 0.
	//
	// En UT y no en bolívares porque así lo expresa el reglamento y así cambia: el
	// SENIAT ajusta el valor de la UT, no los sustraendos. Guardado en bolívares,
	// el número envejece con cada providencia SIN QUE FALLE NADA, y a partir de ese
	// día se retiene mal en cada factura. Ver SustraendoEn.
	SustraendoUT float64 `json:"sustraendoUt" bson:"sustraendout"`
	// BaseMinimaUT es el monto a partir del cual se retiene, también en UT. Por
	// debajo no hay retención — sin esto se emitirían comprobantes por montos
	// irrisorios que después hay que anular a mano.
	BaseMinimaUT float64 `json:"baseMinimaUt" bson:"baseminimaut"`
	Activo       bool    `json:"activo" bson:"activo"`
}

// SustraendoEn convierte el sustraendo a bolívares con el valor de la UT de una
// fecha. La fórmula publicada es SUSTRAENDO = UT × unidades × porcentaje, no una
// simple multiplicación: el sustraendo depende de la tarifa del concepto.
func (c ConceptoISLR) SustraendoEn(valorUT float64) float64 {
	return c.SustraendoUT * valorUT * c.Porcentaje / 100
}

// BaseMinimaEn convierte el mínimo a bolívares con el valor de la UT de una fecha.
func (c ConceptoISLR) BaseMinimaEn(valorUT float64) float64 {
	return c.BaseMinimaUT * valorUT
}

// RequiereUT indica si este concepto necesita el valor de la UT para calcularse.
// Un concepto sin sustraendo ni mínimo (el caso de las jurídicas) se resuelve
// solo con el porcentaje y no depende de la UT.
func (c ConceptoISLR) RequiereUT() bool {
	return c.SustraendoUT > 0 || c.BaseMinimaUT > 0
}

// Retener calcula el impuesto a retener sobre una base con este concepto, usando
// el valor de la UT vigente para la fecha del hecho.
//
// Devuelve 0 cuando la base no llega al mínimo o cuando el sustraendo se come el
// resultado: una retención negativa no existe, y redondearla hacia arriba sería
// cobrarle de más al proveedor.
//
// OJO con valorUT en cero: si el concepto REQUIERE la UT, llamar acá con 0 anula
// el sustraendo y el mínimo, y el resultado es una retención DE MÁS que no falla
// en ningún lado. Quien llama tiene que comprobar RequiereUT antes y decir que
// falta la UT, no dejar que el cálculo siga con un cero.
func (c ConceptoISLR) Retener(base, valorUT float64) float64 {
	if base < c.BaseMinimaEn(valorUT) {
		return 0
	}
	monto := base*c.Porcentaje/100 - c.SustraendoEn(valorUT)
	if monto < 0 {
		return 0
	}
	return monto
}

// sustraendoPNRE son las unidades tributarias del sustraendo de las personas
// NATURALES RESIDENTES, y a la vez el mínimo a partir del cual se les retiene.
// Es el 83,33 de la tabla del reglamento, el mismo valor que usa nuestra
// localización de Odoo.
const sustraendoPNRE = 83.33

// ConceptosPorDefecto son los conceptos más usados del reglamento venezolano,
// para que una empresa no arranque con la tabla vacía. Son DATO editable: cada
// quien ajusta su tabla, y la tarifa real la define el reglamento vigente.
//
// OJO: esta lista es un PUNTO DE PARTIDA, no una fuente legal. La contadora
// tiene que revisarla contra el reglamento antes de usarla en producción, y
// CARGAR EL VALOR DE LA UT: sin él, los conceptos con sustraendo no se pueden
// calcular y la orden lo dice en vez de retener de más.
//
// NO se siembran las tarifas de los sujetos NO domiciliados. No es un olvido: no
// tenerlas cargadas se ve —el concepto aparece sin tarifa para ese sujeto y la
// pantalla lo explica—, mientras que sembrarlas con un número inventado se vería
// exactamente igual que tenerlas bien. Entre un hueco visible y un dato falso
// invisible, el hueco.
func ConceptosPorDefecto(empresaID string) []ConceptoISLR {
	// A las personas naturales residentes se les retiene con sustraendo y con un
	// mínimo; a las jurídicas domiciliadas, sobre el total desde el primer bolívar.
	pnre := func(codigo, nombre string, pct float64) ConceptoISLR {
		return ConceptoISLR{
			EmpresaID: empresaID, Codigo: codigo, Nombre: nombre,
			Sujeto: SujetoNaturalResidente, Porcentaje: pct,
			SustraendoUT: sustraendoPNRE, BaseMinimaUT: sustraendoPNRE, Activo: true,
		}
	}
	pjdo := func(codigo, nombre string, pct float64) ConceptoISLR {
		return ConceptoISLR{
			EmpresaID: empresaID, Codigo: codigo, Nombre: nombre,
			Sujeto: SujetoJuridicaDomiciliada, Porcentaje: pct, Activo: true,
		}
	}
	return []ConceptoISLR{
		pnre("honorarios", "Honorarios profesionales", 3),
		pjdo("honorarios", "Honorarios profesionales", 5),
		pnre("servicios", "Servicios en general", 3),
		pjdo("servicios", "Servicios en general", 2),
		pnre("arrendamiento_inmuebles", "Arrendamiento de inmuebles", 3),
		pjdo("arrendamiento_inmuebles", "Arrendamiento de inmuebles", 5),
		pjdo("fletes", "Fletes y transporte", 3),
		pjdo("publicidad", "Publicidad y propaganda", 5),
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
