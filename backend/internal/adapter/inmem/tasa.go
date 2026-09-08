package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/tasa"
)

// TasaRepo es el histórico de tasas en memoria. SOLO-ANEXADO: no hay Update ni
// Delete, igual que en el puerto del dominio.
type TasaRepo struct {
	mu    sync.Mutex
	items []tasa.Tasa
}

// NewTasaRepo construye el repositorio.
func NewTasaRepo() *TasaRepo { return &TasaRepo{} }

// Append anexa un registro y le asigna id.
func (r *TasaRepo) Append(t tasa.Tasa) tasa.Tasa {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t.ID == "" {
		t.ID = nextID("tasa_")
	}
	if t.Estado == "" {
		t.Estado = tasa.EstadoVigente
	}
	r.items = append(r.items, t)
	return t
}

// UltimaVigente devuelve la tasa vigente más reciente del ámbito.
func (r *TasaRepo) UltimaVigente(empresaID string) (tasa.Tasa, bool) {
	return r.UltimaVigenteDeFuentes(empresaID)
}

// UltimaVigenteDeFuentes filtra además por fuente (vacío = cualquiera). Es del
// dólar: acota a la moneda "USD" (retrocompat: el vacío cuenta como "USD").
func (r *TasaRepo) UltimaVigenteDeFuentes(empresaID string, fuentes ...string) (tasa.Tasa, bool) {
	return r.UltimaVigenteDeMonedaFuentes(empresaID, tasa.MonedaUSD, fuentes...)
}

// UltimaVigenteDeMonedaFuentes filtra por moneda (Moneda=="" se trata como
// "USD") y opcionalmente por fuente (vacío = cualquiera).
func (r *TasaRepo) UltimaVigenteDeMonedaFuentes(empresaID, moneda string, fuentes ...string) (tasa.Tasa, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	quiere := tasa.NormalizarMoneda(moneda)
	permitida := func(f string) bool {
		if len(fuentes) == 0 {
			return true
		}
		for _, x := range fuentes {
			if x == f {
				return true
			}
		}
		return false
	}
	// Se recorre de atrás hacia adelante: el histórico está en orden de anexado.
	for i := len(r.items) - 1; i >= 0; i-- {
		t := r.items[i]
		if t.EmpresaID == empresaID && t.Vigente() &&
			tasa.NormalizarMoneda(t.Moneda) == quiere && permitida(t.Fuente) {
			return t, true
		}
	}
	return tasa.Tasa{}, false
}

// Historial devuelve el histórico del ámbito, más reciente primero, incluyendo
// las rechazadas: la cuarentena tiene que ser visible.
func (r *TasaRepo) Historial(empresaID string, limite int) []tasa.Tasa {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []tasa.Tasa{}
	for i := len(r.items) - 1; i >= 0 && (limite <= 0 || len(out) < limite); i-- {
		if r.items[i].EmpresaID == empresaID {
			out = append(out, r.items[i])
		}
	}
	return out
}

// ByID busca un registro dentro del ámbito (aislamiento de tenant).
func (r *TasaRepo) ByID(empresaID, id string) (tasa.Tasa, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.items {
		if t.EmpresaID == empresaID && t.ID == id {
			return t, true
		}
	}
	return tasa.Tasa{}, false
}
