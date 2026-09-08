// Package facturacion es el dominio de suscripciones/billing de la consola: un catálogo
// de PLANES (comerciales, propios de Mornix) y la SUSCRIPCIÓN de cada organización a un
// plan. Vive en la base de datos de la consola (no en el core). Al asignar un plan, sus
// LÍMITES se aplican al core (vía /internal/orgs/:id/plan); el cobro es local (manual) o,
// opcionalmente, vía checkout de Hubmy (packages → Stripe).
package facturacion

// Intervalos de cobro de un plan.
const (
	IntervaloMensual = "mensual"
	IntervaloAnual   = "anual"
	IntervaloUnico   = "unico"
)

// Estados de una suscripción.
const (
	EstadoTrial         = "trial"          // periodo de prueba, sin cobrar
	EstadoActiva        = "activa"          // pagada (manual o Hubmy) y vigente
	EstadoPendientePago = "pendiente_pago"  // checkout generado, aún sin pagar
	EstadoVencida       = "vencida"         // pasó la fecha de próximo cobro sin pagar
	EstadoCancelada     = "cancelada"       // dada de baja
)

// Limites replica los límites de capacidad que entiende el core (organizacion.Limites).
// 0 = sin límite. La consola los aplica al core al asignar el plan.
type Limites struct {
	FacturasMes int `json:"facturasMes" bson:"facturasmes"`
	Usuarios    int `json:"usuarios" bson:"usuarios"`
	Sucursales  int `json:"sucursales" bson:"sucursales"`
}

// Plan es una entrada del catálogo comercial (tier). El precio va en centavos de la
// moneda (como Hubmy). HubmyPackageID enlaza opcionalmente el plan con un package de
// Hubmy para generar checkouts reales.
type Plan struct {
	ID             string  `json:"id" bson:"id"`
	Nombre         string  `json:"nombre" bson:"nombre"`
	Descripcion    string  `json:"descripcion" bson:"descripcion"`
	PrecioCents    int     `json:"precioCents" bson:"preciocents"`
	Moneda         string  `json:"moneda" bson:"moneda"` // ISO 4217 (USD, VES…)
	Intervalo      string  `json:"intervalo" bson:"intervalo"`
	Limites        Limites `json:"limites" bson:"limites"`
	HubmyPackageID string  `json:"hubmyPackageId" bson:"hubmypackageid"`
	Activo         bool    `json:"activo" bson:"activo"`
	Creado         string  `json:"creado" bson:"creado"`
}

// Suscripcion es el vínculo de una organización con un plan (una vigente por org).
type Suscripcion struct {
	OrgID        string `json:"orgId" bson:"orgid"`
	PlanID       string `json:"planId" bson:"planid"`
	Estado       string `json:"estado" bson:"estado"`
	Inicio       string `json:"inicio" bson:"inicio"`             // RFC3339
	ProximoCobro string `json:"proximoCobro" bson:"proximocobro"` // RFC3339 ("" si único/sin fecha)
	// Datos de la integración con Hubmy (opcionales).
	HubmyUserID string `json:"hubmyUserId" bson:"hubmyuserid"` // usuario Hubmy que paga (dueño del tenant)
	HubmySaleID string `json:"hubmySaleId" bson:"hubmysaleid"`
	CheckoutURL string `json:"checkoutUrl" bson:"checkouturl"`
	Actualizada string `json:"actualizada" bson:"actualizada"`
}

// PlanRepo persiste el catálogo de planes.
type PlanRepo interface {
	List() []Plan
	ByID(id string) (Plan, bool)
	Create(p Plan) Plan
	Update(p Plan) (Plan, bool)
}

// SuscripcionRepo persiste las suscripciones (clave lógica: OrgID).
type SuscripcionRepo interface {
	List() []Suscripcion
	ByOrg(orgID string) (Suscripcion, bool)
	Upsert(s Suscripcion) Suscripcion
}

// MRRCents devuelve el aporte mensual (en centavos) de un plan según su intervalo.
// Anual se prorratea a 1/12; el pago único no aporta MRR.
func (p Plan) MRRCents() int {
	switch p.Intervalo {
	case IntervaloMensual:
		return p.PrecioCents
	case IntervaloAnual:
		return p.PrecioCents / 12
	default:
		return 0
	}
}
