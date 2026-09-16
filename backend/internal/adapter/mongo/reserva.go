package mongo

import (
	"sort"

	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/reserva"
)

func (st *Store) attachReservas(db *gomongo.Database) {
	st.Reservas = &ReservaRepo{coll[reserva.Reserva]{db.Collection("reservas")}}
}

// ReservaRepo persiste las reservaciones del salón con filtro empresaid obligatorio.
type ReservaRepo struct {
	c coll[reserva.Reserva]
}

// porHora deja la agenda como se lee un día de salón: por fecha y hora.
func porHora(xs []reserva.Reserva) []reserva.Reserva {
	sort.SliceStable(xs, func(i, j int) bool {
		if xs[i].Fecha != xs[j].Fecha {
			return xs[i].Fecha < xs[j].Fecha
		}
		return xs[i].Hora < xs[j].Hora
	})
	return xs
}

func (r *ReservaRepo) DelDia(empresaID, sedeID, fecha string) []reserva.Reserva {
	return porHora(r.c.all(map[string]any{"empresaid": empresaID, "sedeid": sedeID, "fecha": fecha}))
}

func (r *ReservaRepo) Desde(empresaID, sedeID, fecha string) []reserva.Reserva {
	// La fecha se guarda como YYYY-MM-DD, que ordena igual como texto que como día:
	// por eso $gte sobre el string es correcto y no hace falta convertir.
	return porHora(r.c.all(map[string]any{
		"empresaid": empresaID, "sedeid": sedeID,
		"fecha": map[string]any{"$gte": fecha},
	}))
}

func (r *ReservaRepo) ByID(empresaID, id string) (reserva.Reserva, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}

func (r *ReservaRepo) Create(x reserva.Reserva) reserva.Reserva {
	if x.ID == "" {
		x.ID = newID("res_")
	}
	r.c.insert(x)
	return x
}

func (r *ReservaRepo) Update(x reserva.Reserva) (reserva.Reserva, bool) {
	if _, ok := r.ByID(x.EmpresaID, x.ID); !ok {
		return reserva.Reserva{}, false
	}
	r.c.replace(x.ID, x)
	return x, true
}
