package mongo

import (
	"github.com/mornix/elerp/internal/domain/sello"
	"regexp"

	gomongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/mornix/elerp/internal/domain/cliente"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

func (st *Store) attachFiscal(db *gomongo.Database) {
	st.Clientes = &ClienteRepo{coll[cliente.Cliente]{db.Collection("clientes")}}
	st.Documentos = &DocumentoRepo{coll[fiscal.Documento]{db.Collection("documentos")}}
	st.Numerador = &NumeradorRepo{db.Collection("contadores")}
	st.CuentasCobro = &CuentaCobroRepo{coll[fiscal.CuentaCobro]{db.Collection("cuentascobro")}}
	st.MetodosPago = &MetodoPagoRepo{coll[fiscal.MetodoPago]{db.Collection("metodospago")}}
	st.Dispositivos = &DispositivoFiscalRepo{coll[fiscal.DispositivoFiscal]{db.Collection("dispositivosfiscales")}}
	st.CierresZ = &CierreZRepo{coll[fiscal.CierreZ]{db.Collection("cierresz")}}
	st.Retenciones = &RetencionRepo{coll[fiscal.Retencion]{db.Collection("retenciones")}}
}

// --- Retenciones (IVA e ISLR, append-only) ---

// RetencionRepo persiste comprobantes de retención con filtro empresaid
// obligatorio. Append-only: no expone Update ni Delete.
type RetencionRepo struct{ c coll[fiscal.Retencion] }

func (r *RetencionRepo) List(empresaID string) []fiscal.Retencion {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *RetencionRepo) ByID(empresaID, id string) (fiscal.Retencion, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *RetencionRepo) ByDocumento(empresaID, tipo, impuesto, docID string) (fiscal.Retencion, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "tipo": tipo, "impuesto": impuesto, "documentoid": docID})
}
func (r *RetencionRepo) Append(x fiscal.Retencion) fiscal.Retencion {
	if x.ID == "" {
		x.ID = newID("ret_")
	}
	r.c.insert(x)
	return x
}

// --- Cierres Z (append-only) ---

type CierreZRepo struct{ c coll[fiscal.CierreZ] }

func (r *CierreZRepo) List(empresaID string) []fiscal.CierreZ {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *CierreZRepo) Append(z fiscal.CierreZ) fiscal.CierreZ {
	if z.ID == "" {
		z.ID = newID("z_")
	}
	r.c.insert(z)
	return z
}

// --- Clientes ---

type ClienteRepo struct{ c coll[cliente.Cliente] }

func (r *ClienteRepo) List(empresaID string) []cliente.Cliente {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *ClienteRepo) ByID(empresaID, id string) (cliente.Cliente, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *ClienteRepo) ByDocumento(empresaID, tipoDoc, documento string) (cliente.Cliente, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "tipodocumento": tipoDoc, "documento": documento})
}
func (r *ClienteRepo) Create(c cliente.Cliente) cliente.Cliente {
	if c.ID == "" {
		c.ID = newID("cli_")
	}
	r.c.insert(c)
	return c
}
func (r *ClienteRepo) Update(c cliente.Cliente) (cliente.Cliente, bool) {
	if _, ok := r.ByID(c.EmpresaID, c.ID); !ok {
		return cliente.Cliente{}, false
	}
	r.c.replace(c.ID, c)
	return c, true
}

// --- Documentos (append-only) ---

type DocumentoRepo struct{ c coll[fiscal.Documento] }

func (r *DocumentoRepo) Append(d fiscal.Documento) fiscal.Documento {
	if d.ID == "" {
		d.ID = newID("doc_")
	}
	prev := ""
	if todos := r.c.all(map[string]any{"empresaid": d.EmpresaID}); len(todos) > 0 {
		prev = todos[len(todos)-1].Hash
	}
	d.PrevHash = prev
	d.Hash = sello.Encadenar(prev, d.Contenido())
	r.c.insert(d)
	return d
}
func (r *DocumentoRepo) List(empresaID string) []fiscal.Documento {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *DocumentoRepo) ByID(empresaID, id string) (fiscal.Documento, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}

// --- Numerador (folios atómicos con $inc) ---

type NumeradorRepo struct{ c *gomongo.Collection }

func (r *NumeradorRepo) Siguiente(empresaID, sedeID, serie string) int {
	ctx, cancel := opctx()
	defer cancel()
	key := empresaID + "|" + sedeID + "|" + serie
	var out struct {
		Seq int `bson:"seq"`
	}
	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)
	err := r.c.FindOneAndUpdate(ctx, map[string]any{"id": key}, map[string]any{"$inc": map[string]any{"seq": 1}}, opts).Decode(&out)
	if err != nil {
		return 0
	}
	return out.Seq
}

// Actual lee el último folio entregado de la clave (0 si el contador no existe).
// Consulta pura: no toca el documento.
func (r *NumeradorRepo) Actual(empresaID, sedeID, serie string) int {
	ctx, cancel := opctx()
	defer cancel()
	key := empresaID + "|" + sedeID + "|" + serie
	var out struct {
		Seq int `bson:"seq"`
	}
	if err := r.c.FindOne(ctx, map[string]any{"id": key}).Decode(&out); err != nil {
		return 0
	}
	return out.Seq
}

// Fijar establece el último folio SOLO hacia adelante. Rechaza el retroceso: si
// el contador ya existe con un seq MAYOR que `ultimo`, devuelve ErrFolioRetrocede
// (los folios usados no se reasignan). Si no existe (continuar desde un sistema
// previo sin haber emitido aún) o su seq es <= ultimo, hace upsert al nuevo valor.
//
// Se comprueba el estado actual antes del upsert en vez de confiar en un índice
// único de `id`: la colección `contadores` no lo declara, así que un upsert
// condicional que no case el filtro insertaría un duplicado en vez de fallar. El
// upsert final es un $set simple con filtro por id, que es correcto pase lo que
// pase con los índices.
func (r *NumeradorRepo) Fijar(empresaID, sedeID, serie string, ultimo int) error {
	if r.Actual(empresaID, sedeID, serie) > ultimo {
		return fiscal.ErrFolioRetrocede
	}
	ctx, cancel := opctx()
	defer cancel()
	key := empresaID + "|" + sedeID + "|" + serie
	update := map[string]any{
		"$set":         map[string]any{"seq": ultimo},
		"$setOnInsert": map[string]any{"id": key},
	}
	opts := options.Update().SetUpsert(true)
	_, err := r.c.UpdateOne(ctx, map[string]any{"id": key}, update, opts)
	return err
}

// Series devuelve las claves de la empresa con su último folio entregado. El
// empresaID se escapa con QuoteMeta antes de armar el regex (defensa: el '|' del
// separador y cualquier metacarácter no deben alterar el patrón de anclaje).
func (r *NumeradorRepo) Series(empresaID string) map[string]int {
	ctx, cancel := opctx()
	defer cancel()
	pattern := "^" + regexp.QuoteMeta(empresaID+"|")
	out := map[string]int{}
	cur, err := r.c.Find(ctx, map[string]any{"id": map[string]any{"$regex": pattern}})
	if err != nil {
		return out
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var d struct {
			ID  string `bson:"id"`
			Seq int    `bson:"seq"`
		}
		if cur.Decode(&d) == nil {
			out[d.ID] = d.Seq
		}
	}
	return out
}

// --- Cuentas de cobro ---

type CuentaCobroRepo struct{ c coll[fiscal.CuentaCobro] }

func (r *CuentaCobroRepo) List(empresaID string) []fiscal.CuentaCobro {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *CuentaCobroRepo) ByID(empresaID, id string) (fiscal.CuentaCobro, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *CuentaCobroRepo) Create(c fiscal.CuentaCobro) fiscal.CuentaCobro {
	if c.ID == "" {
		c.ID = newID("cta_")
	}
	r.c.insert(c)
	return c
}

// --- Métodos de pago ---

type MetodoPagoRepo struct{ c coll[fiscal.MetodoPago] }

func (r *MetodoPagoRepo) List(empresaID string) []fiscal.MetodoPago {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *MetodoPagoRepo) ByID(empresaID, id string) (fiscal.MetodoPago, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *MetodoPagoRepo) Create(m fiscal.MetodoPago) fiscal.MetodoPago {
	if m.ID == "" {
		m.ID = newID("mpago_")
	}
	r.c.insert(m)
	return m
}
func (r *MetodoPagoRepo) Update(m fiscal.MetodoPago) (fiscal.MetodoPago, bool) {
	if _, ok := r.ByID(m.EmpresaID, m.ID); !ok {
		return fiscal.MetodoPago{}, false
	}
	r.c.replace(m.ID, m)
	return m, true
}
func (r *MetodoPagoRepo) Delete(empresaID, id string) bool {
	return r.c.del(map[string]any{"empresaid": empresaID, "id": id})
}

// --- Dispositivos fiscales ---

type DispositivoFiscalRepo struct {
	c coll[fiscal.DispositivoFiscal]
}

func (r *DispositivoFiscalRepo) List(empresaID string) []fiscal.DispositivoFiscal {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *DispositivoFiscalRepo) ByID(empresaID, id string) (fiscal.DispositivoFiscal, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *DispositivoFiscalRepo) Create(d fiscal.DispositivoFiscal) fiscal.DispositivoFiscal {
	if d.ID == "" {
		d.ID = newID("disp_")
	}
	r.c.insert(d)
	return d
}
func (r *DispositivoFiscalRepo) Update(d fiscal.DispositivoFiscal) (fiscal.DispositivoFiscal, bool) {
	if _, ok := r.ByID(d.EmpresaID, d.ID); !ok {
		return fiscal.DispositivoFiscal{}, false
	}
	r.c.replace(d.ID, d)
	return d, true
}
