package application

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mornix/elerp/internal/domain/empresa"
)

// Configuración del RANGO AUTORIZADO del Número de Control (SENIAT). La providencia
// autoriza un prefijo + un rango [desde, hasta]; el correlativo vivo lo lleva el
// Numerador (serie "CTRL"), así que fijar el inicio = poner el contador en desde-1
// (forward-only: no se puede reiniciar por debajo de lo ya emitido, esos números no
// se reutilizan). Ver comprobantes-seniat.md.

var (
	ErrNumeroControlPrefijo = errors.New("el prefijo del número de control debe ser de 1 a 2 dígitos")
	ErrNumeroControlRango   = errors.New("el rango del número de control es inválido: 'hasta' debe ser mayor o igual que 'desde'")
	ErrNumeroControlUsado   = errors.New("no puedes iniciar el rango antes del último número de control ya emitido")
)

// NumeroControlView es el estado del rango del número de control para la UI.
type NumeroControlView struct {
	Prefijo   string `json:"prefijo"`
	Desde     int    `json:"desde"`
	Hasta     int    `json:"hasta"`     // 0 = sin tope declarado
	Actual    int    `json:"actual"`    // último correlativo entregado
	Proximo   int    `json:"proximo"`   // siguiente a emitir
	Restantes int    `json:"restantes"` // Hasta - Actual (si hay tope); -1 = sin tope
	Ejemplo   string `json:"ejemplo"`   // cómo se verá el próximo (prefijo-8dígitos)
}

func soloDigitos(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// EstadoNumeroControl devuelve el rango configurado y el correlativo vivo.
func (s *Service) EstadoNumeroControl(empresaID string) NumeroControlView {
	emp, _ := s.empresas.ByID(empresaID)
	prefijo := emp.NumeroControlPrefijo
	if prefijo == "" {
		prefijo = "00"
	}
	actual := 0
	if s.numerador != nil {
		actual = s.numerador.Actual(empresaID, "", serieControl)
	}
	v := NumeroControlView{
		Prefijo: prefijo, Desde: emp.NumeroControlDesde, Hasta: emp.NumeroControlHasta,
		Actual: actual, Proximo: actual + 1, Restantes: -1,
		Ejemplo: fmt.Sprintf("%s-%08d", prefijo, actual+1),
	}
	if emp.NumeroControlHasta > 0 {
		if v.Restantes = emp.NumeroControlHasta - actual; v.Restantes < 0 {
			v.Restantes = 0
		}
	}
	return v
}

// ConfigurarNumeroControl fija el prefijo y el rango [desde, hasta]. Si `desde` > 0,
// posiciona el correlativo en desde-1 (solo hacia adelante). Audita el cambio.
func (s *Service) ConfigurarNumeroControl(empresaID, actor, origen, prefijo string, desde, hasta int) (empresa.Empresa, error) {
	prefijo = strings.TrimSpace(prefijo)
	if prefijo == "" {
		prefijo = "00"
	}
	if len(prefijo) > 2 || !soloDigitos(prefijo) {
		return empresa.Empresa{}, ErrNumeroControlPrefijo
	}
	if desde < 0 || hasta < 0 || (hasta > 0 && desde > 0 && hasta < desde) {
		return empresa.Empresa{}, ErrNumeroControlRango
	}
	emp, ok := s.empresas.ByID(empresaID)
	if !ok {
		return empresa.Empresa{}, ErrEmpresaNoExiste
	}
	// Posicionar el correlativo al inicio del rango (forward-only).
	if desde > 0 && s.numerador != nil {
		if err := s.numerador.Fijar(empresaID, "", serieControl, desde-1); err != nil {
			return empresa.Empresa{}, ErrNumeroControlUsado
		}
	}
	emp.NumeroControlPrefijo = prefijo
	emp.NumeroControlDesde = desde
	emp.NumeroControlHasta = hasta
	out, ok := s.empresas.Update(emp)
	if !ok {
		return empresa.Empresa{}, ErrEmpresaNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "config.numerocontrol", empresaID, fmt.Sprintf("%s · %d–%d", prefijo, desde, hasta)))
	return out, nil
}
