package application

import (
	"errors"
	"strings"

	"github.com/mornix/elerp/internal/domain/fiscal"
)

// MAESTRO DE CONCEPTOS ISLR (nota de la contadora, 15:53). Ver
// domain/fiscal/concepto.go para el porqué del modelo.
//
// Lo que resuelve acá: que quien registra una retención ELIJA el concepto y la
// tarifa venga con él, en vez de saberse de memoria la tabla del reglamento.

var (
	ErrConceptoNoExiste    = errors.New("el concepto de ISLR no existe")
	ErrConceptoInvalido    = errors.New("el concepto de ISLR está incompleto o mal formado")
	ErrConceptosNoCargados = errors.New("el maestro de conceptos de ISLR no está disponible")
	// ErrConceptoDuplicado: ya hay una fila activa para ese par (código, sujeto).
	// El par es la clave con la que se resuelve la tarifa; duplicarlo no da error en
	// ningún lado, solo hace que gane una de las dos sin decir cuál.
	ErrConceptoDuplicado = errors.New("ya existe un concepto activo con ese código para ese tipo de sujeto")
)

// ConConceptosISLR cablea el maestro de conceptos.
func (s *Service) ConConceptosISLR(r fiscal.ConceptoISLRRepo) *Service {
	s.conceptosISLR = r
	return s
}

// ConceptosISLR devuelve el maestro de la empresa, SEMBRÁNDOLO la primera vez.
//
// La siembra es perezosa y no en el alta de la empresa por la misma razón que el
// maestro de impuestos: cuando esto se construyó ya había empresas creadas, y
// una migración por ocho filas es más frágil que sembrar al primer uso.
func (s *Service) ConceptosISLR(empresaID string) []fiscal.ConceptoISLR {
	if s.conceptosISLR == nil {
		return []fiscal.ConceptoISLR{}
	}
	out := s.conceptosISLR.List(empresaID)
	if len(out) == 0 {
		for _, c := range fiscal.ConceptosPorDefecto(empresaID) {
			s.conceptosISLR.Create(c)
		}
		out = s.conceptosISLR.List(empresaID)
	}
	return out
}

// GuardarConceptoISLR da de alta o edita un concepto. A diferencia de las
// alícuotas, acá NO hace falta versionar por fecha: lo retenido ya quedó copiado
// en su comprobante (porcentaje y sustraendo incluidos), que es append-only, así
// que cambiar la tabla no altera nada de lo emitido.
func (s *Service) GuardarConceptoISLR(empresaID, actor, origen string, c fiscal.ConceptoISLR) (fiscal.ConceptoISLR, error) {
	if s.conceptosISLR == nil {
		return fiscal.ConceptoISLR{}, ErrConceptosNoCargados
	}
	c.EmpresaID = empresaID
	c.Codigo = strings.TrimSpace(strings.ToLower(c.Codigo))
	c.Nombre = strings.TrimSpace(c.Nombre)
	if c.Codigo == "" || c.Nombre == "" {
		return fiscal.ConceptoISLR{}, ErrConceptoInvalido
	}
	if !fiscal.SujetoValido(c.Sujeto) {
		return fiscal.ConceptoISLR{}, ErrConceptoInvalido
	}
	// Una tarifa fuera de (0, 100] no es una tarifa: en 0 no retiene nada y se ve
	// configurada, y por encima de 100 se quedaría con más de lo facturado.
	if c.Porcentaje <= 0 || c.Porcentaje > 100 {
		return fiscal.ConceptoISLR{}, ErrConceptoInvalido
	}
	if c.SustraendoUT < 0 || c.BaseMinimaUT < 0 || c.DesdeAcumuladoUT < 0 {
		return fiscal.ConceptoISLR{}, ErrConceptoInvalido
	}
	// La porción gravable es un porcentaje: 0 se lee como «sobre todo el pago», pero
	// por encima de 100 gravaría más de lo que se paga.
	if c.PorcentajeBase < 0 || c.PorcentajeBase > 100 {
		return fiscal.ConceptoISLR{}, ErrConceptoInvalido
	}
	// Un concepto con sustraendo o mínimo en UT no se puede calcular sin saber
	// cuánto vale la UT. Se avisa al GUARDARLO —cuando quien lo configura tiene el
	// contexto— y no al primer documento que lo use, semanas después.
	if c.Activo && c.RequiereUT() {
		if _, ok := s.UTVigenteEn(empresaID, ""); !ok {
			return fiscal.ConceptoISLR{}, ErrUTNoCargada
		}
	}
	maestro := s.ConceptosISLR(empresaID) // asegura la siembra antes de tocar la tabla
	// La CLAVE de la tabla es la terna (código, sujeto, tramo). Un mismo código y
	// sujeto tiene VARIAS filas cuando la tarifa es una escala —la Tarifa 2 de los
	// no domiciliados—, una por tramo; lo que no puede haber es dos tramos que
	// arranquen en el mismo acumulado, porque entonces cuál gana lo decidiría el
	// orden de lectura. Y eso no da ningún error: solo una tarifa distinta a la
	// esperada. Se rechaza al guardar, que es el único momento en que se puede.
	for _, otro := range maestro {
		if otro.ID == c.ID || !otro.Activo || !c.Activo {
			continue
		}
		if strings.EqualFold(otro.Codigo, c.Codigo) && otro.Sujeto == c.Sujeto &&
			casiIgualUT(otro.DesdeAcumuladoUT, c.DesdeAcumuladoUT) {
			return fiscal.ConceptoISLR{}, ErrConceptoDuplicado
		}
	}
	if c.ID != "" {
		if _, ok := s.conceptosISLR.ByID(empresaID, c.ID); !ok {
			return fiscal.ConceptoISLR{}, ErrConceptoNoExiste
		}
		out, _ := s.conceptosISLR.Update(c)
		s.audit.Append(evento(empresaID, actor, origen, "config.concepto_islr.actualizar", out.Codigo, out.Nombre))
		return out, nil
	}
	out := s.conceptosISLR.Create(c)
	s.audit.Append(evento(empresaID, actor, origen, "config.concepto_islr.crear", out.Codigo, out.Nombre))
	return out, nil
}

// casiIgualUT compara dos pisos de tramo. Se comparan con tolerancia porque son
// cifras con decimales que viajan por JSON: 2000.01 tecleado dos veces tiene que
// chocar consigo mismo, y un error de coma flotante no puede colar un duplicado.
func casiIgualUT(a, b float64) bool {
	d := a - b
	return d < 0.005 && d > -0.005
}

// ErrConceptoISLRDesconocido: la ficha de producto trae un código que el maestro
// no tiene. Se rechaza en vez de guardarse, por lo mismo que la alícuota: un
// producto clasificado con un concepto inexistente no retendría nada, y esa
// ausencia se ve igual que «no es un servicio».
var ErrConceptoISLRDesconocido = errors.New("ese concepto de ISLR no existe en el maestro")

// validarConceptoISLRProducto comprueba que el código del producto exista en el
// maestro, y de paso lo SIEMBRA si la empresa todavía no lo tiene.
//
// Solo valida el CÓDIGO, no el sujeto: el producto declara QUÉ se paga
// (honorarios, flete) y el proveedor declara A QUIÉN (natural o jurídica). Es la
// división que hace que la tarifa se pueda resolver sola al comprar; pedirle el
// sujeto a la ficha del producto sería preguntarle algo que no puede saber.
//
// Código vacío es válido y es el caso de toda mercancía: no sujeto a retención.
func (s *Service) validarConceptoISLRProducto(empresaID, codigo string) (string, error) {
	cod := strings.ToLower(strings.TrimSpace(codigo))
	if cod == "" || s.conceptosISLR == nil {
		return cod, nil
	}
	if !fiscal.ExisteConcepto(s.ConceptosISLR(empresaID), cod) {
		return "", ErrConceptoISLRDesconocido
	}
	return cod, nil
}

// SugerenciaRetencionISLR es lo que la pantalla precarga al registrar una
// retención de ISLR: de dónde salió la tarifa y cuánto da.
type SugerenciaRetencionISLR struct {
	Codigo     string  `json:"codigo"`
	Nombre     string  `json:"nombre"`
	Sujeto     string  `json:"sujeto"`
	Porcentaje float64 `json:"porcentaje"`
	// Sustraendo y BaseMinima van en BOLÍVARES, ya convertidos con la UT de la
	// fecha. El maestro los guarda en unidades tributarias; la pantalla muestra
	// bolívares porque es lo que se compara con la factura que tiene delante.
	Sustraendo float64 `json:"sustraendo"`
	BaseMinima float64 `json:"baseMinima"`
	// ValorUT es la UT con la que se resolvió, para poder explicar el número.
	ValorUT float64 `json:"valorUt"`
	Base    float64 `json:"base"`
	// Monto es lo que se retendría. 0 con Retiene=false significa que la base no
	// llega al mínimo del concepto — que NO es lo mismo que «no aplica».
	Monto   float64 `json:"monto"`
	Retiene bool    `json:"retiene"`
}

// SugerirRetencionISLR resuelve la tarifa de un concepto para un sujeto y
// calcula cuánto se retendría sobre una base. Es lo que hace que el usuario
// elija en vez de teclear.
//
// `fecha` es la del hecho (AAAA-MM-DD); vacía = hoy. Importa porque de ella sale
// el valor de la UT: registrar en octubre una factura de agosto tiene que usar la
// UT de agosto, no la de hoy.
func (s *Service) SugerirRetencionISLR(empresaID, codigo, sujeto, fecha, terceroID string, base float64) (SugerenciaRetencionISLR, error) {
	// El tramo de la escala depende del acumulado del ejercicio CON este pago
	// dentro. Sin tercero el acumulado es 0 y sale el primer tramo, que es lo
	// correcto para una consulta suelta: no hay a quién acumularle.
	valorUTPrevio := s.ValorUTEn(empresaID, fecha)
	primero, _ := fiscal.ConceptoPara(s.ConceptosISLR(empresaID), codigo, sujeto, 0)
	acumulado := s.AcumuladoISLRUT(empresaID, terceroID, codigo, fecha, "") +
		baseEnUT(primero.BaseGravable(base), valorUTPrevio)

	c, ok := fiscal.ConceptoPara(s.ConceptosISLR(empresaID), codigo, sujeto, acumulado)
	if !ok {
		return SugerenciaRetencionISLR{}, ErrConceptoNoExiste
	}
	// Sin UT, un concepto que la requiere se calcularía con sustraendo y mínimo en
	// cero: retendría de más y nadie lo notaría. Se dice que falta.
	valorUT := valorUTPrevio
	if c.RequiereUT() && valorUT <= 0 {
		return SugerenciaRetencionISLR{}, ErrUTNoCargada
	}
	monto := round2(c.Retener(base, valorUT))
	return SugerenciaRetencionISLR{
		Codigo: c.Codigo, Nombre: c.Nombre, Sujeto: c.Sujeto,
		Porcentaje: c.Porcentaje,
		Sustraendo: round2(c.SustraendoEn(valorUT)), BaseMinima: round2(c.BaseMinimaEn(valorUT)),
		ValorUT: valorUT,
		// La base que se informa es la GRAVABLE, no el pago: es la que multiplicada
		// por la tarifa da el monto. Devolver el pago dejaría una cifra que no
		// explica el número de al lado.
		Base: round2(c.BaseGravable(base)), Monto: monto, Retiene: monto > 0,
	}, nil
}
