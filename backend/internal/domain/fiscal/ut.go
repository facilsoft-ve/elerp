package fiscal

import "sort"

// UNIDAD TRIBUTARIA: el valor en bolívares de la UT, con su vigencia.
//
// POR QUÉ EXISTE. El reglamento de ISLR no expresa los sustraendos ni los montos
// mínimos en bolívares: los expresa en UNIDADES TRIBUTARIAS, y el SENIAT ajusta
// el valor de la UT por providencia. Guardar el sustraendo en bolívares en la
// tabla de conceptos parece más simple y es una trampa: el número envejece con
// cada providencia, nadie se entera —porque no falla nada— y a partir de ese día
// se retiene de más o de menos en cada factura.
//
// Guardar la cantidad en UT y multiplicarla por el valor vigente convierte una
// actualización de veinte filas en una sola. Es el mismo principio que las
// alícuotas: la regla es DATO versionado, no código.
//
// CON VIGENCIA, y por la misma razón que la alícuota: registrar hoy una factura
// de agosto tiene que usar la UT que regía en agosto, no la de hoy (Art. 177).
// Sin esto, capturar facturas atrasadas después de una providencia produce
// retenciones distintas a las del comprobante que ya se emitió.
type UnidadTributaria struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	// Valor es cuántos bolívares vale una UT.
	Valor float64 `json:"valor" bson:"valor"`
	// VigenteDesde es el día en que empieza a regir (AAAA-MM-DD). No hay
	// VigenteHasta a propósito: una UT rige hasta que la sustituye la siguiente, y
	// un rango cerrado permitiría configurar huecos en los que no regiría ninguna.
	VigenteDesde string `json:"vigenteDesde" bson:"vigentedesde"`
	// Fuente documenta de dónde salió el valor (la Gaceta, la providencia). No
	// participa del cálculo; sirve para poder defenderlo.
	Fuente string `json:"fuente" bson:"fuente"`
	Actor  string `json:"actor" bson:"actor"`
	Creada string `json:"creada" bson:"creada"` // UTC RFC3339
}

// UTVigenteEn devuelve la UT que regía en una fecha (AAAA-MM-DD): la de
// VigenteDesde más reciente que no sea posterior a ella.
//
// Devuelve false cuando no hay ninguna —tabla vacía, o la fecha es anterior a la
// primera cargada—. Quien llama NO debe tratar eso como «la UT vale cero»: un
// sustraendo de cero retiene de más, en silencio.
func UTVigenteEn(uts []UnidadTributaria, fecha string) (UnidadTributaria, bool) {
	f := fechaCorta(fecha)
	if f == "" {
		return UnidadTributaria{}, false
	}
	candidatas := []UnidadTributaria{}
	for _, u := range uts {
		if u.Valor > 0 && u.VigenteDesde != "" && u.VigenteDesde <= f {
			candidatas = append(candidatas, u)
		}
	}
	if len(candidatas) == 0 {
		return UnidadTributaria{}, false
	}
	sort.SliceStable(candidatas, func(i, j int) bool {
		return candidatas[i].VigenteDesde > candidatas[j].VigenteDesde
	})
	return candidatas[0], true
}

// UnidadTributariaRepo persiste el histórico de la UT, aislado por empresa.
//
// Es de SOLO ANEXADO (sin Update ni Delete): el valor de una UT pasada no se
// corrige, se carga la siguiente. Si se pudiera editar, un comprobante emitido
// el año pasado dejaría de poder explicarse.
type UnidadTributariaRepo interface {
	List(empresaID string) []UnidadTributaria
	Create(u UnidadTributaria) UnidadTributaria
}
