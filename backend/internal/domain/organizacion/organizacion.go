// Package organizacion es el nivel superior de la jerarquía de cuentas:
// Organización → Empresa → Sede. Una organización agrupa varias empresas
// (RIF distintos) bajo un mismo cliente/plan comercial.
package organizacion

// Limites son los topes de CAPACIDAD del plan comercial (SAD: "límites por
// configuración, nunca sobre obligaciones legales"). Un tope en 0 significa
// ILIMITADO — así una organización sin límites fijados (valor cero del struct)
// no restringe nada, y las organizaciones ya creadas no necesitan migración.
type Limites struct {
	FacturasMes int      `json:"facturasMes" bson:"facturasmes"` // documentos fiscales / mes
	Usuarios    int      `json:"usuarios" bson:"usuarios"`       // miembros de la organización
	Sucursales  int      `json:"sucursales" bson:"sucursales"`   // sedes
	Modulos     []string `json:"modulos" bson:"modulos"`         // módulos habilitados por el plan ([] ⇒ sin restricción por plan)
}

// Organizacion es el titular comercial (a lo que Super Admin le asigna un plan).
type Organizacion struct {
	ID      string  `json:"id" bson:"id"`
	Nombre  string  `json:"nombre" bson:"nombre"`
	Estado  string  `json:"estado" bson:"estado"` // activa | suspendida
	Plan    string  `json:"plan" bson:"plan"`     // id del plan comercial
	Limites Limites `json:"limites" bson:"limites"`
	Creada  string  `json:"creada" bson:"creada"`
}

// Repository es el puerto de persistencia de organizaciones.
type Repository interface {
	List() []Organizacion
	ByID(id string) (Organizacion, bool)
	Create(o Organizacion) Organizacion
	Update(o Organizacion) (Organizacion, bool)
}
