package inventario

import "time"

// PLAN DE CONTEO: qué almacén toca contar, y cada cuánto.
//
// POR QUÉ EXISTE. El conteo físico ya funciona, pero se lanza cuando alguien se
// acuerda — y nadie se acuerda del almacén que no da problemas, que es justo donde
// la diferencia lleva meses creciendo sin que nadie la mire.
//
// LO QUE NO ES: un cron. Un conteo no se puede ejecutar solo, porque contar es ir
// al estante y mirar. Lo que el plan automatiza es SABER QUÉ TOCA y tener la hoja
// lista, no hacerlo por su cuenta. Un sistema que dijera «conteo realizado» sin que
// nadie hubiera contado nada sería exactamente lo contrario de lo que se busca.
type PlanConteo struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	SedeID    string `json:"sedeId" bson:"sedeid"`
	// AlmacenID acota el plan a un depósito; vacío = el principal de la sede.
	AlmacenID string `json:"almacenId,omitempty" bson:"almacenid,omitempty"`
	// UbicacionID acota a una casilla. Es lo que permite el conteo CÍCLICO de
	// verdad: en vez de parar el almacén entero un día, se cuenta un pasillo cada
	// semana y en un mes está todo revisado sin cerrar nunca.
	UbicacionID string `json:"ubicacionId,omitempty" bson:"ubicacionid,omitempty"`
	Nombre      string `json:"nombre" bson:"nombre"`
	// Rubro acota a una categoría. Un almacén mixto no se cuenta igual: lo que se
	// mueve rápido se revisa cada semana y lo demás cada trimestre.
	Rubro string `json:"rubro,omitempty" bson:"rubro,omitempty"`
	// CadaDias es la periodicidad. Cero significa «a demanda»: el plan existe para
	// tener la hoja preparada, pero no vence nunca ni aparece como pendiente.
	CadaDias int `json:"cadaDias" bson:"cadadias"`
	// UltimoConteo es la fecha (AAAA-MM-DD) del último conteo APLICADO con este
	// plan. Vacío = nunca se contó, y entonces toca ya: un plan recién creado sobre
	// un almacén que nadie ha revisado es precisamente el caso urgente.
	UltimoConteo string `json:"ultimoConteo,omitempty" bson:"ultimoconteo,omitempty"`
	Activo       bool   `json:"activo" bson:"activo"`
	Creado       string `json:"creado" bson:"creado"`
}

// DiasDesdeElUltimo dice cuántos días pasaron desde el último conteo aplicado.
// Devuelve -1 cuando nunca se contó, que quien llama traduce como «toca ya».
func (p PlanConteo) DiasDesdeElUltimo(hoy string) int {
	if p.UltimoConteo == "" {
		return -1
	}
	u, err1 := time.Parse("2006-01-02", p.UltimoConteo)
	h, err2 := time.Parse("2006-01-02", hoy)
	if err1 != nil || err2 != nil {
		return -1
	}
	return int(h.Sub(u).Hours() / 24)
}

// Vencido dice si el plan toca hoy.
//
// Un plan SIN periodicidad nunca vence: existe para tener la hoja a mano cuando
// alguien decida contar. Marcarlo como pendiente lo dejaría siempre en rojo y
// enseñaría a ignorar la lista entera — que es lo que mata a los avisos.
func (p PlanConteo) Vencido(hoy string) bool {
	if !p.Activo || p.CadaDias <= 0 {
		return false
	}
	d := p.DiasDesdeElUltimo(hoy)
	return d < 0 || d >= p.CadaDias
}

// PlanConteoRepo persiste los planes, aislado por empresaID. Editable, sin borrado
// duro: desactivar conserva cuándo se contó por última vez.
type PlanConteoRepo interface {
	List(empresaID string) []PlanConteo
	ByID(empresaID, id string) (PlanConteo, bool)
	Create(p PlanConteo) PlanConteo
	Update(p PlanConteo) (PlanConteo, bool)
}
