package application

import (
	"strings"

	"github.com/mornix/elerp/internal/domain/fiscal"
)

// ACUMULADO DEL EJERCICIO PARA LA TARIFA 2 DEL ISLR.
//
// POR QUÉ EXISTE. A los sujetos NO domiciliados no se les retiene un porcentaje
// fijo sino una ESCALA —15 %, 22 %, 34 %—, y el tramo lo decide cuánto se le lleva
// pagado en el año por ese concepto, no lo que dice la factura de hoy. Sin
// acumular, cada factura se mira aislada, todas caen en el primer tramo y se
// retiene de menos durante todo el ejercicio: un error que crece con el volumen y
// que no da ninguna señal.
//
// EN UNIDADES TRIBUTARIAS, no en bolívares. Cada comprobante guarda su base ya
// convertida con la UT de su día (fiscal.Retencion.BaseUT) y acá solo se suman. Si
// se acumularan bolívares y se convirtieran al final, el acumulado se movería cada
// vez que el SENIAT publica una UT nueva —los pagos viejos valdrían menos UT— y el
// tramo de diciembre dependería de una providencia de noviembre.

// ejercicioDe devuelve el año fiscal de una fecha AAAA-MM-DD.
//
// Se usa el AÑO CALENDARIO. Un ejercicio que no coincida con el año calendario
// —lo permite la ley para algunas empresas— no está soportado, y es mejor que
// falte a que acumule mal en silencio. Fecha vacía o ilegible devuelve "", que no
// casa con ningún comprobante: no acumula nada en vez de acumularlo todo.
func ejercicioDe(fecha string) string {
	f := strings.TrimSpace(fecha)
	if len(f) < 4 {
		return ""
	}
	return f[:4]
}

// AcumuladoISLRUT suma, en unidades tributarias, las bases ya retenidas a un
// tercero por un concepto dentro del ejercicio de una fecha.
//
// Cuenta solo las retenciones EMITIDAS —las que hicimos nosotros al proveedor—:
// las recibidas son de nuestras ventas y no dicen nada de lo que le llevamos
// pagado a él. `excluirDocumentoID` deja fuera un documento concreto, para poder
// recalcular una factura sin que su propia retención se sume dos veces.
//
// Los comprobantes anteriores a que esto existiera no llevan BaseUT ni
// ConceptoCodigo y por tanto no suman. No se intenta reconstruirlos: habría que
// adivinar con qué UT se emitieron, y un acumulado inventado decide un tramo
// equivocado con la misma seguridad que uno correcto.
func (s *Service) AcumuladoISLRUT(empresaID, terceroID, conceptoCodigo, fecha, excluirDocumentoID string) float64 {
	if s.retenciones == nil || terceroID == "" || conceptoCodigo == "" {
		return 0
	}
	// Fecha vacía = hoy, igual que en UTVigenteEn. Devolver 0 sería peor que
	// inútil: el tramo saldría el primero y la pantalla mostraría un número
	// distinto al que guarda el servidor, sin que ninguno de los dos fallara.
	anio := ejercicioDe(fecha)
	if anio == "" {
		anio = ejercicioDe(ahora())
	}
	cod := strings.ToLower(strings.TrimSpace(conceptoCodigo))
	total := 0.0
	for _, r := range s.retenciones.List(empresaID) {
		if r.Tipo != fiscal.RetencionEmitida || r.Impuesto != fiscal.ImpuestoISLR {
			continue
		}
		if r.TerceroID != terceroID || strings.ToLower(r.ConceptoCodigo) != cod {
			continue
		}
		if ejercicioDe(r.Fecha) != anio {
			continue
		}
		if excluirDocumentoID != "" && r.DocumentoID == excluirDocumentoID {
			continue
		}
		total += r.BaseUT
	}
	return total
}

// AcumuladosISLRUTDe devuelve el acumulado del ejercicio de un tercero para CADA
// concepto del maestro, indexado por código. Es lo que necesita la pantalla de
// órdenes para proyectar el tramo correcto mientras se arma el pedido, sin
// resolver un cálculo que es del servidor.
func (s *Service) AcumuladosISLRUTDe(empresaID, terceroID, fecha string) map[string]float64 {
	out := map[string]float64{}
	if terceroID == "" {
		return out
	}
	for _, c := range s.ConceptosISLR(empresaID) {
		cod := strings.ToLower(strings.TrimSpace(c.Codigo))
		if _, visto := out[cod]; visto {
			continue
		}
		out[cod] = s.AcumuladoISLRUT(empresaID, terceroID, cod, fecha, "")
	}
	return out
}

// baseEnUT convierte una base gravable a unidades tributarias. Devuelve 0 cuando
// no hay UT: sin ella la cifra no significa nada y sumarla al acumulado lo
// ensuciaría para siempre.
func baseEnUT(base, valorUT float64) float64 {
	if valorUT <= 0 {
		return 0
	}
	return base / valorUT
}
