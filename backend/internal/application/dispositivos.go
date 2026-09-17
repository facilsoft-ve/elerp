package application

import (
	"errors"
	"sort"
	"strings"

	"github.com/mornix/elerp/internal/domain/dispositivo"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

// ErrDispositivoNoExiste indica que el dispositivo no pertenece a la empresa.
var ErrDispositivoNoExiste = errors.New("dispositivo fiscal no existe")

// DispositivosFiscales lista los dispositivos fiscales configurados de la
// empresa, ordenados por nombre.
func (s *Service) DispositivosFiscales(empresaID string) []fiscal.DispositivoFiscal {
	out := s.dispositivos.List(empresaID)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Nombre < out[j].Nombre })
	return out
}

// serieDispositivoDuplicada indica si otra ficha de la empresa (distinta de
// excluirID) ya usa esa serie. Serie vacía nunca colisiona.
func (s *Service) serieDispositivoDuplicada(empresaID, serie, excluirID string) bool {
	serie = strings.TrimSpace(serie)
	if serie == "" {
		return false
	}
	for _, d := range s.dispositivos.List(empresaID) {
		if d.ID != excluirID && strings.EqualFold(strings.TrimSpace(d.Serie), serie) {
			return true
		}
	}
	return false
}

// CrearDispositivoFiscal registra un dispositivo fiscal de la empresa.
func (s *Service) CrearDispositivoFiscal(empresaID, actor, origen string, d fiscal.DispositivoFiscal) (fiscal.DispositivoFiscal, error) {
	d.Nombre = strings.TrimSpace(d.Nombre)
	if d.Nombre == "" {
		return fiscal.DispositivoFiscal{}, errors.New("nombre requerido")
	}
	if d.Tipo == "" {
		d.Tipo = fiscal.DispositivoImpresoraFiscal
	}
	if !fiscal.TipoDispositivoValido(d.Tipo) {
		return fiscal.DispositivoFiscal{}, errors.New("tipo inválido")
	}
	if s.serieDispositivoDuplicada(empresaID, d.Serie, "") {
		return fiscal.DispositivoFiscal{}, errors.New("ya existe un dispositivo con esa serie")
	}
	d.EmpresaID = empresaID
	d.Serie = strings.TrimSpace(d.Serie)
	// Puerto/Protocolo son datos de conexión (los usa la balanza); se normalizan
	// pero no se validan: el agente fiscal local es quien sabe qué acepta.
	d.Puerto = strings.TrimSpace(d.Puerto)
	d.Protocolo = strings.TrimSpace(d.Protocolo)
	d.Marca = strings.TrimSpace(d.Marca)
	d.Modelo = strings.TrimSpace(d.Modelo)
	d = normalizarDesdeCatalogo(d)
	d.Activo = true
	d.Creado = ahora()
	out := s.dispositivos.Create(d)
	s.audit.Append(evento(empresaID, actor, origen, "config.dispositivo.crear", out.ID, out.Nombre))
	return out, nil
}

// CambiosDispositivoFiscal describe una edición parcial de un dispositivo. Los
// punteros nil dejan el campo intacto (semántica PATCH).
type CambiosDispositivoFiscal struct {
	Nombre    *string
	SedeID    *string
	Tipo      *string
	Marca     *string
	Modelo    *string
	Serie     *string
	Puerto    *string
	Protocolo *string
	Activo    *bool
}

// ActualizarDispositivoFiscal edita un dispositivo existente de la empresa.
func (s *Service) ActualizarDispositivoFiscal(empresaID, actor, origen, id string, cambios CambiosDispositivoFiscal) (fiscal.DispositivoFiscal, error) {
	d, ok := s.dispositivos.ByID(empresaID, id)
	if !ok {
		return fiscal.DispositivoFiscal{}, ErrDispositivoNoExiste
	}
	if cambios.Nombre != nil {
		n := strings.TrimSpace(*cambios.Nombre)
		if n == "" {
			return fiscal.DispositivoFiscal{}, errors.New("nombre requerido")
		}
		d.Nombre = n
	}
	if cambios.SedeID != nil {
		d.SedeID = *cambios.SedeID
	}
	if cambios.Tipo != nil {
		if !fiscal.TipoDispositivoValido(*cambios.Tipo) {
			return fiscal.DispositivoFiscal{}, errors.New("tipo inválido")
		}
		d.Tipo = *cambios.Tipo
	}
	if cambios.Marca != nil {
		d.Marca = strings.TrimSpace(*cambios.Marca)
	}
	if cambios.Modelo != nil {
		d.Modelo = strings.TrimSpace(*cambios.Modelo)
	}
	if cambios.Serie != nil {
		if s.serieDispositivoDuplicada(empresaID, *cambios.Serie, id) {
			return fiscal.DispositivoFiscal{}, errors.New("ya existe un dispositivo con esa serie")
		}
		d.Serie = strings.TrimSpace(*cambios.Serie)
	}
	if cambios.Puerto != nil {
		d.Puerto = strings.TrimSpace(*cambios.Puerto)
	}
	if cambios.Protocolo != nil {
		d.Protocolo = strings.TrimSpace(*cambios.Protocolo)
	}
	if cambios.Activo != nil {
		d.Activo = *cambios.Activo
	}
	d = normalizarDesdeCatalogo(d)
	out, ok := s.dispositivos.Update(d)
	if !ok {
		return fiscal.DispositivoFiscal{}, ErrDispositivoNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "config.dispositivo.actualizar", out.ID, out.Nombre))
	return out, nil
}

// DesactivarDispositivoFiscal desactiva (soft-disable) un dispositivo de la
// empresa. No borra: pone Activo=false para que la operación sea reversible (se
// reactiva con el toggle Activo). Así no se pierde el histórico ni referencias.
func (s *Service) DesactivarDispositivoFiscal(empresaID, actor, origen, id string) error {
	d, ok := s.dispositivos.ByID(empresaID, id)
	if !ok {
		return ErrDispositivoNoExiste
	}
	d.Activo = false
	if _, ok := s.dispositivos.Update(d); !ok {
		return ErrDispositivoNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "config.dispositivo.desactivar", id, ""))
	return nil
}

// CatalogoDispositivosView es el catálogo precargado tal como lo consume la
// interfaz: la lista de modelos conocidos más la fecha en que se revisó, para
// que la pantalla pueda decir con honestidad de cuándo es (la homologación del
// SENIAT es un trámite vivo; esta lista ayuda a elegir, no certifica nada).
type CatalogoDispositivosView struct {
	Revisado string               `json:"revisado"`
	Modelos  []dispositivo.Modelo `json:"modelos"`
}

// CatalogoDispositivos devuelve el catálogo precargado de impresoras fiscales,
// balanzas y comanderas del mercado venezolano. No depende del tenant: es dato
// de referencia igual para todas las empresas.
func (s *Service) CatalogoDispositivos() CatalogoDispositivosView {
	return CatalogoDispositivosView{Revisado: dispositivo.Revisado, Modelos: dispositivo.Catalogo()}
}

// normalizarDesdeCatalogo deja la ficha con la GRAFÍA CANÓNICA del catálogo y le
// rellena los datos de conexión que quedaron en blanco. Es lo que evita que el
// maestro termine con "HKA80", "hka 80" y "The Factory HKA 80" como si fueran
// tres máquinas distintas, y lo que hace que elegir un modelo conocido baste
// para dejar la balanza configurada. Si el modelo NO está catalogado no toca
// nada: marca y modelo son campos libres a propósito.
func normalizarDesdeCatalogo(d fiscal.DispositivoFiscal) fiscal.DispositivoFiscal {
	m, ok := dispositivo.Buscar(d.Tipo, d.Marca, d.Modelo)
	if !ok {
		return d
	}
	d.Marca, d.Modelo = m.Marca, m.Modelo
	if d.Puerto == "" {
		d.Puerto = m.Puerto
	}
	if d.Protocolo == "" {
		d.Protocolo = m.Protocolo
	}
	return d
}
