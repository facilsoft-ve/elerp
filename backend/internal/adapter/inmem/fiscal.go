package inmem

import (
	"github.com/mornix/elerp/internal/domain/sello"
	"strings"
	"sync"

	"github.com/mornix/elerp/internal/domain/cliente"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

// --- Clientes ---

type ClienteRepo struct {
	mu    sync.Mutex
	items []cliente.Cliente
}

func NewClienteRepo() *ClienteRepo { return &ClienteRepo{} }

func (r *ClienteRepo) List(empresaID string) []cliente.Cliente {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []cliente.Cliente{}
	for _, c := range r.items {
		if c.EmpresaID == empresaID {
			out = append(out, c)
		}
	}
	return out
}

func (r *ClienteRepo) ByID(empresaID, id string) (cliente.Cliente, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.items {
		if c.EmpresaID == empresaID && c.ID == id {
			return c, true
		}
	}
	return cliente.Cliente{}, false
}

func (r *ClienteRepo) ByDocumento(empresaID, tipoDoc, documento string) (cliente.Cliente, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.items {
		if c.EmpresaID == empresaID && c.TipoDocumento == tipoDoc && c.Documento == documento {
			return c, true
		}
	}
	return cliente.Cliente{}, false
}

func (r *ClienteRepo) Create(c cliente.Cliente) cliente.Cliente {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.ID == "" {
		c.ID = nextID("cli_")
	}
	r.items = append(r.items, c)
	return c
}

func (r *ClienteRepo) Update(c cliente.Cliente) (cliente.Cliente, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == c.EmpresaID && cur.ID == c.ID {
			r.items[i] = c
			return c, true
		}
	}
	return cliente.Cliente{}, false
}

// --- Documentos (append-only) ---

type DocumentoRepo struct {
	mu    sync.Mutex
	items []fiscal.Documento
}

func NewDocumentoRepo() *DocumentoRepo { return &DocumentoRepo{} }

func (r *DocumentoRepo) Append(d fiscal.Documento) fiscal.Documento {
	r.mu.Lock()
	defer r.mu.Unlock()
	if d.ID == "" {
		d.ID = nextID("doc_")
	}
	prev := ""
	for i := len(r.items) - 1; i >= 0; i-- {
		if r.items[i].EmpresaID == d.EmpresaID {
			prev = r.items[i].Hash
			break
		}
	}
	d.PrevHash = prev
	d.Hash = sello.Encadenar(prev, d.Contenido())
	r.items = append(r.items, d)
	return d
}

func (r *DocumentoRepo) List(empresaID string) []fiscal.Documento {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []fiscal.Documento{}
	for _, d := range r.items {
		if d.EmpresaID == empresaID {
			out = append(out, d)
		}
	}
	return out
}

func (r *DocumentoRepo) ByID(empresaID, id string) (fiscal.Documento, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range r.items {
		if d.EmpresaID == empresaID && d.ID == id {
			return d, true
		}
	}
	return fiscal.Documento{}, false
}

// --- Numerador (folios serializados) ---

type NumeradorRepo struct {
	mu   sync.Mutex
	seqs map[string]int
}

func NewNumeradorRepo() *NumeradorRepo { return &NumeradorRepo{seqs: map[string]int{}} }

func numeradorKey(empresaID, sedeID, serie string) string {
	return empresaID + "|" + sedeID + "|" + serie
}

func (r *NumeradorRepo) Siguiente(empresaID, sedeID, serie string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := numeradorKey(empresaID, sedeID, serie)
	r.seqs[key]++
	return r.seqs[key]
}

// Actual devuelve el último folio entregado (0 si la clave no existe). No muta.
func (r *NumeradorRepo) Actual(empresaID, sedeID, serie string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.seqs[numeradorKey(empresaID, sedeID, serie)]
}

// Fijar establece el último folio SOLO hacia adelante: rechaza cualquier valor
// por debajo del actual (forward-only), para que un folio ya usado nunca se
// reasigne.
func (r *NumeradorRepo) Fijar(empresaID, sedeID, serie string, ultimo int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := numeradorKey(empresaID, sedeID, serie)
	if ultimo < r.seqs[key] {
		return fiscal.ErrFolioRetrocede
	}
	r.seqs[key] = ultimo
	return nil
}

// Series devuelve las claves de la empresa (prefijo "empresaID|") con su último
// folio entregado.
func (r *NumeradorRepo) Series(empresaID string) map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	prefix := empresaID + "|"
	out := map[string]int{}
	for k, v := range r.seqs {
		if strings.HasPrefix(k, prefix) {
			out[k] = v
		}
	}
	return out
}

// Estado devuelve una copia del mapa de secuencias (clave
// "empresaID|sedeID|serie" → último folio entregado). Lo usa el Snapshot para
// que el adaptador Mongo siembre la colección de contadores ya adelantada más
// allá de los folios de la demo, y no reinicie en 1 al emitir en vivo.
func (r *NumeradorRepo) Estado() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]int, len(r.seqs))
	for k, v := range r.seqs {
		out[k] = v
	}
	return out
}

// --- Cierres Z (append-only) ---

type CierreZRepo struct {
	mu    sync.Mutex
	items []fiscal.CierreZ
}

func NewCierreZRepo() *CierreZRepo { return &CierreZRepo{} }

func (r *CierreZRepo) List(empresaID string) []fiscal.CierreZ {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []fiscal.CierreZ{}
	for _, z := range r.items {
		if z.EmpresaID == empresaID {
			out = append(out, z)
		}
	}
	return out
}

func (r *CierreZRepo) Append(z fiscal.CierreZ) fiscal.CierreZ {
	r.mu.Lock()
	defer r.mu.Unlock()
	if z.ID == "" {
		z.ID = nextID("z_")
	}
	r.items = append(r.items, z)
	return z
}

// --- Cuentas de cobro ---

type CuentaCobroRepo struct {
	mu    sync.Mutex
	items []fiscal.CuentaCobro
}

func NewCuentaCobroRepo() *CuentaCobroRepo { return &CuentaCobroRepo{} }

func (r *CuentaCobroRepo) List(empresaID string) []fiscal.CuentaCobro {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []fiscal.CuentaCobro{}
	for _, c := range r.items {
		if c.EmpresaID == empresaID {
			out = append(out, c)
		}
	}
	return out
}

func (r *CuentaCobroRepo) ByID(empresaID, id string) (fiscal.CuentaCobro, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.items {
		if c.EmpresaID == empresaID && c.ID == id {
			return c, true
		}
	}
	return fiscal.CuentaCobro{}, false
}

func (r *CuentaCobroRepo) Create(c fiscal.CuentaCobro) fiscal.CuentaCobro {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.ID == "" {
		c.ID = nextID("cta_")
	}
	r.items = append(r.items, c)
	return c
}

// --- Métodos de pago ---

type MetodoPagoRepo struct {
	mu    sync.Mutex
	items []fiscal.MetodoPago
}

func NewMetodoPagoRepo() *MetodoPagoRepo { return &MetodoPagoRepo{} }

func (r *MetodoPagoRepo) List(empresaID string) []fiscal.MetodoPago {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []fiscal.MetodoPago{}
	for _, m := range r.items {
		if m.EmpresaID == empresaID {
			out = append(out, m)
		}
	}
	return out
}

func (r *MetodoPagoRepo) ByID(empresaID, id string) (fiscal.MetodoPago, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range r.items {
		if m.EmpresaID == empresaID && m.ID == id {
			return m, true
		}
	}
	return fiscal.MetodoPago{}, false
}

func (r *MetodoPagoRepo) Create(m fiscal.MetodoPago) fiscal.MetodoPago {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m.ID == "" {
		m.ID = nextID("mpago_")
	}
	r.items = append(r.items, m)
	return m
}

func (r *MetodoPagoRepo) Update(m fiscal.MetodoPago) (fiscal.MetodoPago, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == m.EmpresaID && cur.ID == m.ID {
			r.items[i] = m
			return m, true
		}
	}
	return fiscal.MetodoPago{}, false
}

func (r *MetodoPagoRepo) Delete(empresaID, id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == empresaID && cur.ID == id {
			r.items = append(r.items[:i], r.items[i+1:]...)
			return true
		}
	}
	return false
}

// --- Dispositivos fiscales ---

type DispositivoFiscalRepo struct {
	mu    sync.Mutex
	items []fiscal.DispositivoFiscal
}

func NewDispositivoFiscalRepo() *DispositivoFiscalRepo { return &DispositivoFiscalRepo{} }

func (r *DispositivoFiscalRepo) List(empresaID string) []fiscal.DispositivoFiscal {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []fiscal.DispositivoFiscal{}
	for _, d := range r.items {
		if d.EmpresaID == empresaID {
			out = append(out, d)
		}
	}
	return out
}

func (r *DispositivoFiscalRepo) ByID(empresaID, id string) (fiscal.DispositivoFiscal, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range r.items {
		if d.EmpresaID == empresaID && d.ID == id {
			return d, true
		}
	}
	return fiscal.DispositivoFiscal{}, false
}

func (r *DispositivoFiscalRepo) Create(d fiscal.DispositivoFiscal) fiscal.DispositivoFiscal {
	r.mu.Lock()
	defer r.mu.Unlock()
	if d.ID == "" {
		d.ID = nextID("disp_")
	}
	r.items = append(r.items, d)
	return d
}

func (r *DispositivoFiscalRepo) Update(d fiscal.DispositivoFiscal) (fiscal.DispositivoFiscal, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == d.EmpresaID && cur.ID == d.ID {
			r.items[i] = d
			return d, true
		}
	}
	return fiscal.DispositivoFiscal{}, false
}

// --- Retenciones (IVA e ISLR, append-only) ---

// RetencionRepo guarda los comprobantes de retención en memoria, aislados por
// empresaID. Append-only: no expone Update ni Delete.
type RetencionRepo struct {
	mu    sync.Mutex
	items []fiscal.Retencion
}

// NewRetencionRepo construye el repositorio.
func NewRetencionRepo() *RetencionRepo { return &RetencionRepo{} }

func (r *RetencionRepo) List(empresaID string) []fiscal.Retencion {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []fiscal.Retencion{}
	for _, x := range r.items {
		if x.EmpresaID == empresaID {
			out = append(out, x)
		}
	}
	return out
}

func (r *RetencionRepo) ByID(empresaID, id string) (fiscal.Retencion, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, x := range r.items {
		if x.EmpresaID == empresaID && x.ID == id {
			return x, true
		}
	}
	return fiscal.Retencion{}, false
}

func (r *RetencionRepo) ByDocumento(empresaID, tipo, impuesto, docID string) (fiscal.Retencion, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, x := range r.items {
		if x.EmpresaID == empresaID && x.Tipo == tipo && x.Impuesto == impuesto && x.DocumentoID == docID {
			return x, true
		}
	}
	return fiscal.Retencion{}, false
}

func (r *RetencionRepo) Append(x fiscal.Retencion) fiscal.Retencion {
	r.mu.Lock()
	defer r.mu.Unlock()
	if x.ID == "" {
		x.ID = nextID("ret_")
	}
	r.items = append(r.items, x)
	return x
}
