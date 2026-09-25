package application

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/mornix/elerp/internal/domain/inventario"
)

// PLANES DE CONTEO. Ver domain/inventario/plan_conteo.go para el porqué.
//
// LO QUE APORTA SOBRE EL CONTEO SUELTO: saber QUÉ TOCA sin que nadie se acuerde, y
// salir al almacén con la hoja ya hecha. Contar sigue siendo ir al estante y mirar
// — eso no lo automatiza nadie.
var (
	// ErrPlanNoExiste: el plan no está o no es de esta empresa.
	ErrPlanNoExiste = errors.New("el plan de conteo no existe")
	// ErrPlanSinNombre: el nombre es por donde se le reconoce en la lista.
	ErrPlanSinNombre = errors.New("el plan necesita un nombre")
	// ErrPlanPeriodicidad: una periodicidad negativa no describe nada. Cero sí es
	// válido y significa «a demanda».
	ErrPlanPeriodicidad = errors.New("la periodicidad no puede ser negativa")
)

// ConPlanesDeConteo cablea el maestro. Sin él el conteo sigue lanzándose a mano,
// que es como funcionaba antes.
func (s *Service) ConPlanesDeConteo(r inventario.PlanConteoRepo) *Service {
	s.planesConteo = r
	return s
}

// PlanConteoView es un plan con lo que hace falta para decidir si toca.
type PlanConteoView struct {
	inventario.PlanConteo
	AlmacenNombre string `json:"almacenNombre,omitempty"`
	Ubicacion     string `json:"ubicacion,omitempty"`
	// DiasDesde es -1 cuando nunca se contó.
	DiasDesde int  `json:"diasDesde"`
	Vencido   bool `json:"vencido"`
	// Productos es cuántos entrarían en la hoja hoy. Un plan cuyo alcance se quedó
	// sin productos —se movió el rubro, se vació el pasillo— sigue venciendo cada
	// mes y mandando a contar un sitio donde no hay nada.
	Productos int `json:"productos"`
}

// PlanesDeConteo lista los planes de una sede, los vencidos primero.
func (s *Service) PlanesDeConteo(empresaID, sedeID string) []PlanConteoView {
	if s.planesConteo == nil {
		return []PlanConteoView{}
	}
	hoy := time.Now().UTC().Format("2006-01-02")
	out := []PlanConteoView{}
	for _, p := range s.planesConteo.List(empresaID) {
		if sedeID != "" && p.SedeID != sedeID {
			continue
		}
		v := PlanConteoView{
			PlanConteo: p,
			DiasDesde:  p.DiasDesdeElUltimo(hoy),
			Vencido:    p.Vencido(hoy),
			Productos:  len(s.alcanceDelPlan(empresaID, p)),
		}
		if p.AlmacenID != "" && s.almacenes != nil {
			if a, ok := s.almacenes.ByID(empresaID, p.AlmacenID); ok {
				v.AlmacenNombre = a.Nombre
			}
		}
		if p.UbicacionID != "" && s.ubicaciones != nil {
			if u, ok := s.ubicaciones.ByID(empresaID, p.UbicacionID); ok {
				v.Ubicacion = u.Codigo
			}
		}
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Vencido != out[j].Vencido {
			return out[i].Vencido // lo que toca, arriba
		}
		return out[i].Nombre < out[j].Nombre
	})
	return out
}

// CrearPlanDeConteo da de alta un plan.
func (s *Service) CrearPlanDeConteo(empresaID, actor, origen string, p inventario.PlanConteo) (inventario.PlanConteo, error) {
	if s.planesConteo == nil {
		return inventario.PlanConteo{}, errors.New("los planes de conteo no están disponibles")
	}
	p.Nombre = strings.TrimSpace(p.Nombre)
	if p.Nombre == "" {
		return inventario.PlanConteo{}, ErrPlanSinNombre
	}
	if p.CadaDias < 0 {
		return inventario.PlanConteo{}, ErrPlanPeriodicidad
	}
	p.EmpresaID = empresaID
	p.Activo = true
	p.Creado = ahora()
	out := s.planesConteo.Create(p)
	s.audit.Append(evento(empresaID, actor, origen, "inventario.plan_conteo.crear", out.Nombre,
		periodicidadLegible(out.CadaDias)))
	return out, nil
}

// ActualizarPlanDeConteo edita el alcance o la periodicidad.
func (s *Service) ActualizarPlanDeConteo(empresaID, id, actor, origen string, cambios inventario.PlanConteo) (inventario.PlanConteo, error) {
	if s.planesConteo == nil {
		return inventario.PlanConteo{}, errors.New("los planes de conteo no están disponibles")
	}
	p, ok := s.planesConteo.ByID(empresaID, id)
	if !ok {
		return inventario.PlanConteo{}, ErrPlanNoExiste
	}
	if n := strings.TrimSpace(cambios.Nombre); n != "" {
		p.Nombre = n
	}
	if cambios.CadaDias < 0 {
		return inventario.PlanConteo{}, ErrPlanPeriodicidad
	}
	p.CadaDias, p.AlmacenID, p.UbicacionID, p.Rubro = cambios.CadaDias, cambios.AlmacenID, cambios.UbicacionID, cambios.Rubro
	p.Activo = cambios.Activo
	// UltimoConteo NO se edita: es un hecho, no una preferencia. Poder retocarlo
	// permitiría aplazar un conteo vencido cambiando una fecha, que es justo lo que
	// el plan existe para evitar.
	out, ok := s.planesConteo.Update(p)
	if !ok {
		return inventario.PlanConteo{}, ErrPlanNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "inventario.plan_conteo.editar", out.Nombre,
		periodicidadLegible(out.CadaDias)))
	return out, nil
}

func periodicidadLegible(dias int) string {
	if dias <= 0 {
		return "a demanda"
	}
	return "cada " + itoa(dias) + " día(s)"
}

// alcanceDelPlan devuelve los productos que entran en la hoja: los que tienen
// existencia dentro del ámbito del plan.
//
// SE CUENTA LO QUE EL SISTEMA CREE QUE HAY, no el catálogo entero. Una hoja con los
// ochocientos productos del catálogo no se usa; una con los cuarenta que deberían
// estar en ese pasillo, sí. Lo que aparezca de más —mercancía en un sitio donde el
// sistema no la tenía— se anota igual: la hoja admite añadir.
func (s *Service) alcanceDelPlan(empresaID string, p inventario.PlanConteo) []LineaHojaConteo {
	out := []LineaHojaConteo{}
	if s.productos == nil || s.movimientos == nil {
		return out
	}
	alm := p.AlmacenID
	if alm == "" && s.almacenes != nil {
		alm = s.almacenParaEscritura(empresaID, p.SedeID, "")
	}
	rubro := strings.TrimSpace(p.Rubro)
	for _, prod := range s.productos.List(empresaID) {
		// Se cuenta lo que se puede contar, que es la pregunta de SeStockea. Acá
		// decía «combo o plato» y dejaba fuera de la hoja el PLATO QUE SE FABRICA
		// PARA STOCK: ese está en la vitrina, tiene existencia y se cuenta como
		// cualquier cosa — quedaba invisible para el recuento y su ajuste nunca
		// llegaba. También se cuela un servicio, que no se cuenta nunca.
		if !prod.SeStockea() {
			continue
		}
		if rubro != "" && prod.Rubro != rubro {
			continue
		}
		total := 0.0
		for _, b := range s.bucketsDe(empresaID, p.SedeID, alm, prod.ID) {
			if p.UbicacionID != "" && b.UbicacionID != p.UbicacionID {
				continue
			}
			total += b.Cantidad
		}
		if total <= 0.0001 {
			continue
		}
		out = append(out, LineaHojaConteo{
			SKU: prod.SKU, Nombre: prod.Nombre, UnidadBase: prod.UnidadBase,
			Sistema: round2(total),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].SKU < out[j].SKU })
	return out
}

// LineaHojaConteo es un renglón de la hoja que se lleva al almacén.
type LineaHojaConteo struct {
	SKU        string  `json:"sku"`
	Nombre     string  `json:"nombre"`
	UnidadBase string  `json:"unidadBase,omitempty"`
	Sistema    float64 `json:"sistema"`
}

// HojaDeConteo es lo que se imprime o se lleva en el teléfono.
type HojaDeConteo struct {
	PlanID    string            `json:"planId"`
	Nombre    string            `json:"nombre"`
	SedeID    string            `json:"sedeId"`
	AlmacenID string            `json:"almacenId,omitempty"`
	Ubicacion string            `json:"ubicacion,omitempty"`
	Fecha     string            `json:"fecha"`
	Lineas    []LineaHojaConteo `json:"lineas"`
}

// HojaDeConteoDe arma la hoja de un plan.
func (s *Service) HojaDeConteoDe(empresaID, planID string) (HojaDeConteo, error) {
	if s.planesConteo == nil {
		return HojaDeConteo{}, errors.New("los planes de conteo no están disponibles")
	}
	p, ok := s.planesConteo.ByID(empresaID, planID)
	if !ok {
		return HojaDeConteo{}, ErrPlanNoExiste
	}
	h := HojaDeConteo{
		PlanID: p.ID, Nombre: p.Nombre, SedeID: p.SedeID, AlmacenID: p.AlmacenID,
		Fecha: ahora(), Lineas: s.alcanceDelPlan(empresaID, p),
	}
	if p.UbicacionID != "" && s.ubicaciones != nil {
		if u, ok := s.ubicaciones.ByID(empresaID, p.UbicacionID); ok {
			h.Ubicacion = u.Codigo
		}
	}
	return h, nil
}

// AplicarConteoDePlan aplica un conteo y SELLA el plan con la fecha de hoy.
//
// El sello va DESPUÉS de aplicar y solo si algo se aplicó: marcar el plan como
// contado cuando la hoja falló entera dejaría el almacén sin revisar y sin aviso
// hasta el siguiente vencimiento.
func (s *Service) AplicarConteoDePlan(empresaID, planID, motivo string, lineas []LineaConteo, actor, origen string) (ResultadoConteo, error) {
	if s.planesConteo == nil {
		return ResultadoConteo{}, errors.New("los planes de conteo no están disponibles")
	}
	p, ok := s.planesConteo.ByID(empresaID, planID)
	if !ok {
		return ResultadoConteo{}, ErrPlanNoExiste
	}
	if strings.TrimSpace(motivo) == "" {
		motivo = p.Nombre
	}
	// Las líneas heredan la ubicación del plan cuando no declaran una: quien cuenta
	// el pasillo B no tiene por qué repetir «pasillo B» en cada renglón.
	if p.UbicacionID != "" {
		for i := range lineas {
			if lineas[i].UbicacionID == "" {
				lineas[i].UbicacionID = p.UbicacionID
			}
		}
	}
	res, err := s.AplicarConteo(empresaID, p.SedeID, p.AlmacenID, motivo, lineas, actor, origen)
	if err != nil {
		return res, err
	}
	if res.ConAjuste+res.SinCambio > 0 {
		p.UltimoConteo = time.Now().UTC().Format("2006-01-02")
		s.planesConteo.Update(p)
	}
	return res, nil
}
