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
	if c.Sustraendo < 0 || c.BaseMinima < 0 {
		return fiscal.ConceptoISLR{}, ErrConceptoInvalido
	}
	s.ConceptosISLR(empresaID) // asegura la siembra antes de tocar la tabla
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

// SugerenciaRetencionISLR es lo que la pantalla precarga al registrar una
// retención de ISLR: de dónde salió la tarifa y cuánto da.
type SugerenciaRetencionISLR struct {
	Codigo     string  `json:"codigo"`
	Nombre     string  `json:"nombre"`
	Sujeto     string  `json:"sujeto"`
	Porcentaje float64 `json:"porcentaje"`
	Sustraendo float64 `json:"sustraendo"`
	Base       float64 `json:"base"`
	// Monto es lo que se retendría. 0 con Retiene=false significa que la base no
	// llega al mínimo del concepto — que NO es lo mismo que «no aplica».
	Monto   float64 `json:"monto"`
	Retiene bool    `json:"retiene"`
}

// SugerirRetencionISLR resuelve la tarifa de un concepto para un sujeto y
// calcula cuánto se retendría sobre una base. Es lo que hace que el usuario
// elija en vez de teclear.
func (s *Service) SugerirRetencionISLR(empresaID, codigo, sujeto string, base float64) (SugerenciaRetencionISLR, error) {
	c, ok := fiscal.ConceptoPara(s.ConceptosISLR(empresaID), codigo, sujeto)
	if !ok {
		return SugerenciaRetencionISLR{}, ErrConceptoNoExiste
	}
	monto := round2(c.Retener(base))
	return SugerenciaRetencionISLR{
		Codigo: c.Codigo, Nombre: c.Nombre, Sujeto: c.Sujeto,
		Porcentaje: c.Porcentaje, Sustraendo: c.Sustraendo,
		Base: base, Monto: monto, Retiene: monto > 0,
	}, nil
}
