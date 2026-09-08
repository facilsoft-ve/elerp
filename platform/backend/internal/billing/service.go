// Package billing es la capa de aplicación de suscripciones: gestiona el catálogo de
// planes, asigna planes a organizaciones (aplicando sus LÍMITES al core vía /internal),
// y lleva el ciclo de vida de la suscripción (cobro manual o checkout de Hubmy opt-in).
package billing

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/mornix/elerp-platform/internal/core"
	"github.com/mornix/elerp-platform/internal/facturacion"
	"github.com/mornix/elerp-platform/internal/hubmy"
)

var (
	ErrPlanNoExiste = errors.New("el plan no existe")
	ErrSinSuscripcion = errors.New("la organización no tiene suscripción")
	ErrCore           = errors.New("el core rechazó la actualización del plan")
)

// Service orquesta planes + suscripciones. `hub` puede ser nil/no-configurado: en ese
// caso el billing es 100% local (cobro manual).
type Service struct {
	planes facturacion.PlanRepo
	subs   facturacion.SuscripcionRepo
	core   *core.Client
	hub    *hubmy.Client
}

func New(planes facturacion.PlanRepo, subs facturacion.SuscripcionRepo, core *core.Client, hub *hubmy.Client) *Service {
	return &Service{planes: planes, subs: subs, core: core, hub: hub}
}

// HubmyDisponible indica si hay checkout de Hubmy configurado.
func (s *Service) HubmyDisponible() bool { return s.hub != nil && s.hub.Configurado() }

/* --- Catálogo de planes ---------------------------------------------------- */

func (s *Service) Planes() []facturacion.Plan { return s.planes.List() }

func (s *Service) CrearPlan(p facturacion.Plan) facturacion.Plan {
	p.Activo = true
	return s.planes.Create(p)
}

func (s *Service) EditarPlan(id string, cambios facturacion.Plan) (facturacion.Plan, bool) {
	p, ok := s.planes.ByID(id)
	if !ok {
		return facturacion.Plan{}, false
	}
	cambios.ID = p.ID
	cambios.Creado = p.Creado
	return s.planes.Update(cambios)
}

func (s *Service) ArchivarPlan(id string) (facturacion.Plan, bool) {
	p, ok := s.planes.ByID(id)
	if !ok {
		return facturacion.Plan{}, false
	}
	p.Activo = false
	return s.planes.Update(p)
}

/* --- Suscripción por organización ------------------------------------------ */

// SuscripcionView es la suscripción con el plan expandido, para la UI.
type SuscripcionView struct {
	Suscripcion *facturacion.Suscripcion `json:"suscripcion"`
	Plan        *facturacion.Plan        `json:"plan"`
}

// Suscripcion devuelve la suscripción de una org (con su plan), o vacía si no tiene.
func (s *Service) Suscripcion(orgID string) SuscripcionView {
	sub, ok := s.subs.ByOrg(orgID)
	if !ok {
		return SuscripcionView{}
	}
	view := SuscripcionView{Suscripcion: &sub}
	if p, ok := s.planes.ByID(sub.PlanID); ok {
		view.Plan = &p
	}
	return view
}

// AsignarPlan asigna un plan a una organización: aplica sus límites al core y registra la
// suscripción. Si Hubmy está configurado, el plan tiene package y se pasa un hubmyUserID,
// genera un checkout (estado pendiente_pago); si no, la suscripción queda activa (manual).
func (s *Service) AsignarPlan(ctx context.Context, orgID, planID, hubmyUserID, actor string) (SuscripcionView, error) {
	plan, ok := s.planes.ByID(planID)
	if !ok {
		return SuscripcionView{}, ErrPlanNoExiste
	}

	// 1) Aplicar los LÍMITES del plan al core (fuente de verdad del enforcement).
	body, _ := json.Marshal(map[string]any{
		"plan": plan.Nombre,
		"limites": map[string]any{
			"facturasMes": plan.Limites.FacturasMes,
			"usuarios":    plan.Limites.Usuarios,
			"sucursales":  plan.Limites.Sucursales,
			"modulos":     []string{},
		},
	})
	resp, err := s.core.Forward(ctx, http.MethodPatch, "/internal/orgs/"+orgID+"/plan", actor, body, "application/json")
	if err != nil {
		return SuscripcionView{}, err
	}
	if resp.Status >= 400 {
		return SuscripcionView{}, ErrCore
	}

	// 2) Registrar/actualizar la suscripción.
	sub, _ := s.subs.ByOrg(orgID)
	sub.OrgID = orgID
	sub.PlanID = planID
	if hubmyUserID != "" {
		sub.HubmyUserID = hubmyUserID
	}
	sub.Inicio = ahora()
	sub.ProximoCobro = proximoCobro(plan.Intervalo)
	sub.HubmySaleID = ""
	sub.CheckoutURL = ""
	sub.Actualizada = ahora()

	// 3) Checkout de Hubmy (opt-in). Si algo falla, la suscripción queda local y se avisa.
	if s.HubmyDisponible() && plan.HubmyPackageID != "" && sub.HubmyUserID != "" {
		venta, err := s.hub.CrearVenta(ctx, plan.HubmyPackageID, sub.HubmyUserID)
		if err == nil {
			sub.HubmySaleID = venta.ID
			sub.CheckoutURL = venta.CheckoutURL
			sub.Estado = facturacion.EstadoPendientePago
		} else {
			// No se pudo generar el checkout: se deja como pendiente de pago sin URL.
			sub.Estado = facturacion.EstadoPendientePago
		}
	} else {
		// Sin Hubmy: cobro manual, se marca activa (el operador confirma el pago aparte).
		sub.Estado = facturacion.EstadoActiva
	}

	saved := s.subs.Upsert(sub)
	return s.Suscripcion(saved.OrgID), nil
}

// MarcarPagada confirma manualmente el pago (o renueva) de una suscripción.
func (s *Service) MarcarPagada(orgID string) (SuscripcionView, error) {
	sub, ok := s.subs.ByOrg(orgID)
	if !ok {
		return SuscripcionView{}, ErrSinSuscripcion
	}
	plan, _ := s.planes.ByID(sub.PlanID)
	sub.Estado = facturacion.EstadoActiva
	sub.Inicio = ahora()
	sub.ProximoCobro = proximoCobro(plan.Intervalo)
	sub.Actualizada = ahora()
	s.subs.Upsert(sub)
	return s.Suscripcion(orgID), nil
}

// Cancelar da de baja la suscripción (no toca los límites ya aplicados al core).
func (s *Service) Cancelar(orgID string) (SuscripcionView, error) {
	sub, ok := s.subs.ByOrg(orgID)
	if !ok {
		return SuscripcionView{}, ErrSinSuscripcion
	}
	sub.Estado = facturacion.EstadoCancelada
	sub.Actualizada = ahora()
	s.subs.Upsert(sub)
	return s.Suscripcion(orgID), nil
}

// Sincronizar consulta el estado del checkout de Hubmy y actualiza la suscripción.
func (s *Service) Sincronizar(ctx context.Context, orgID string) (SuscripcionView, error) {
	sub, ok := s.subs.ByOrg(orgID)
	if !ok {
		return SuscripcionView{}, ErrSinSuscripcion
	}
	if !s.HubmyDisponible() || sub.HubmySaleID == "" {
		return s.Suscripcion(orgID), nil
	}
	venta, err := s.hub.Venta(ctx, sub.HubmySaleID)
	if err != nil {
		return SuscripcionView{}, err
	}
	switch venta.Status {
	case "paid":
		plan, _ := s.planes.ByID(sub.PlanID)
		sub.Estado = facturacion.EstadoActiva
		if sub.Inicio == "" {
			sub.Inicio = ahora()
		}
		sub.ProximoCobro = proximoCobro(plan.Intervalo)
	case "refunded", "disputed":
		sub.Estado = facturacion.EstadoCancelada
	}
	sub.Actualizada = ahora()
	s.subs.Upsert(sub)
	return s.Suscripcion(orgID), nil
}

/* --- Resumen / MRR --------------------------------------------------------- */

// ResumenFacturacion consolida el estado comercial de la cartera.
type ResumenFacturacion struct {
	Suscripciones int            `json:"suscripciones"`
	Activas       int            `json:"activas"`
	MRRCents      int            `json:"mrrCents"`
	Moneda        string         `json:"moneda"`
	PorEstado     map[string]int `json:"porEstado"`
	PorPlan       map[string]int `json:"porPlan"` // nombre del plan → cantidad
	HubmyDisponible bool         `json:"hubmyDisponible"`
}

// Resumen calcula MRR y conteos. El MRR suma el aporte mensual de las suscripciones
// vigentes (activa/trial). La moneda se toma del primer plan con precio (mezclar monedas
// es responsabilidad del operador; acá se informa la predominante).
func (s *Service) Resumen() ResumenFacturacion {
	res := ResumenFacturacion{PorEstado: map[string]int{}, PorPlan: map[string]int{}, HubmyDisponible: s.HubmyDisponible()}
	planCache := map[string]facturacion.Plan{}
	planDe := func(id string) (facturacion.Plan, bool) {
		if p, ok := planCache[id]; ok {
			return p, true
		}
		p, ok := s.planes.ByID(id)
		if ok {
			planCache[id] = p
		}
		return p, ok
	}
	for _, sub := range s.subs.List() {
		res.Suscripciones++
		res.PorEstado[sub.Estado]++
		if p, ok := planDe(sub.PlanID); ok {
			res.PorPlan[p.Nombre]++
			if sub.Estado == facturacion.EstadoActiva || sub.Estado == facturacion.EstadoTrial {
				res.Activas++
				res.MRRCents += p.MRRCents()
				if res.Moneda == "" {
					res.Moneda = p.Moneda
				}
			}
		}
	}
	return res
}

/* --- Helpers de tiempo ----------------------------------------------------- */

func ahora() string { return time.Now().UTC().Format(time.RFC3339) }

func proximoCobro(intervalo string) string {
	now := time.Now().UTC()
	switch intervalo {
	case facturacion.IntervaloMensual:
		return now.AddDate(0, 1, 0).Format(time.RFC3339)
	case facturacion.IntervaloAnual:
		return now.AddDate(1, 0, 0).Format(time.RFC3339)
	default:
		return "" // pago único: sin próximo cobro
	}
}
