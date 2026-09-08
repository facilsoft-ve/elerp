package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/plantilla"
)

func (st *Store) attachPlantillas(db *gomongo.Database) {
	st.Plantillas = &PlantillaRepo{coll[plantilla.Plantilla]{db.Collection("plantillas")}}
}

// PlantillaRepo persiste formatos de documento con filtro empresaid obligatorio.
type PlantillaRepo struct {
	c coll[plantilla.Plantilla]
}

func (r *PlantillaRepo) List(empresaID string) []plantilla.Plantilla {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *PlantillaRepo) ByID(empresaID, id string) (plantilla.Plantilla, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *PlantillaRepo) Create(p plantilla.Plantilla) plantilla.Plantilla {
	if p.ID == "" {
		p.ID = newID("fmt_")
	}
	r.c.insert(p)
	return p
}
func (r *PlantillaRepo) Update(p plantilla.Plantilla) (plantilla.Plantilla, bool) {
	if _, ok := r.ByID(p.EmpresaID, p.ID); !ok {
		return plantilla.Plantilla{}, false
	}
	r.c.replace(p.ID, p)
	return p, true
}
func (r *PlantillaRepo) Delete(empresaID, id string) bool {
	return r.c.del(map[string]any{"empresaid": empresaID, "id": id})
}
