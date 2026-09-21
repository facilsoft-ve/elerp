package application

import (
	"sort"
	"strings"
	"time"

	"github.com/mornix/elerp/internal/domain/inventario"
)

// EL RASTRO DE UN LOTE: «¿a quién le vendí el lote X?».
//
// Es la consulta que justifica toda la trazabilidad. Guardar el lote en cada
// movimiento y no poder preguntarlo después es tener el dato y no la función: en
// una alerta sanitaria hay horas para avisar a quien compró, y revisar el ledger
// a mano no es una respuesta.
//
// Se deriva del mismo ledger, sin tablas nuevas: cada movimiento del lote apunta
// con RefTipo/RefID al hecho que lo originó —una compra, una venta, una
// transferencia— y de ahí se recupera el documento y su tercero.

// PasoDeLote es un movimiento del lote con el documento y el tercero detrás.
type PasoDeLote struct {
	Fecha    string  `json:"fecha"`
	Tipo     string  `json:"tipo"`
	SedeID   string  `json:"sedeId"`
	Cantidad float64 `json:"cantidad"`
	// Saldo es el que queda tras este paso: convierte una lista de movimientos en
	// algo que se puede leer de arriba abajo.
	Saldo float64 `json:"saldo"`
	// Documento es el número legible del hecho (factura, orden, transferencia) y
	// Tercero a quién se le vendió o a quién se le compró — lo que hay que saber
	// para poder avisar.
	Documento string `json:"documento,omitempty"`
	Tercero   string `json:"tercero,omitempty"`
	Motivo    string `json:"motivo,omitempty"`
	RefTipo   string `json:"refTipo,omitempty"`
	RefID     string `json:"refId,omitempty"`
}

// RastroDeLote es la historia completa de un lote de un producto.
type RastroDeLote struct {
	SKU         string       `json:"sku"`
	Nombre      string       `json:"nombre"`
	Lote        string       `json:"lote"`
	Vencimiento string       `json:"vencimiento,omitempty"`
	Vencido     bool         `json:"vencido,omitempty"`
	Recibido    float64      `json:"recibido"`
	Salido      float64      `json:"salido"`
	EnStock     float64      `json:"enStock"`
	Pasos       []PasoDeLote `json:"pasos"`
	// Clientes son los terceros a los que salió mercancía de este lote, sin
	// repetir. Es la respuesta corta a «¿a quién hay que avisar?».
	Clientes []string `json:"clientes"`
}

// RastroDeLote reconstruye la historia de un lote: qué entró, qué salió, a quién
// y qué queda. Sin sede se barre la empresa entera, que es lo que hace falta en
// una alerta: el lote no respeta los límites de una sucursal.
func (s *Service) RastroDeLote(empresaID, sku, lote, sedeID string) RastroDeLote {
	p, ok := s.productos.BySKU(empresaID, sku)
	out := RastroDeLote{SKU: sku, Lote: lote, Pasos: []PasoDeLote{}, Clientes: []string{}}
	if !ok {
		return out
	}
	out.Nombre = p.Nombre

	movs := []inventario.Movimiento{}
	for _, m := range s.movimientos.List(empresaID, inventario.FiltroMovimiento{SedeID: sedeID, ProductoID: p.ID}) {
		if m.Lote == lote && m.Tipo != inventario.MovRevaluacion {
			movs = append(movs, m)
		}
	}
	sort.SliceStable(movs, func(i, j int) bool { return movs[i].Fecha < movs[j].Fecha })

	vistos := map[string]bool{}
	saldo := 0.0
	for _, m := range movs {
		saldo = round2(saldo + m.Cantidad)
		// Los totales cuentan lo que entró y salió DE LA EMPRESA, no los movimientos
		// internos. Una transferencia entre sedes aparece dos veces —salida en una,
		// entrada en otra— y sumarlas haría que «recibido» dijera más de lo que se
		// compró: la pregunta que se le hace a este número es cuánto entró y a cuánto
		// salió, no cuántas veces se movió de estante.
		//
		// Filtrando POR SEDE sí se cuentan: ahí una transferencia es una entrada o una
		// salida de verdad para esa sucursal.
		interno := m.Tipo == inventario.MovTransferencia && sedeID == ""
		if !interno {
			if m.Cantidad > 0 {
				out.Recibido = round2(out.Recibido + m.Cantidad)
			} else {
				out.Salido = round2(out.Salido - m.Cantidad)
			}
		}
		doc, tercero := s.documentoYTerceroDe(empresaID, m.RefTipo, m.RefID)
		out.Pasos = append(out.Pasos, PasoDeLote{
			Fecha: m.Fecha, Tipo: m.Tipo, SedeID: m.SedeID, Cantidad: m.Cantidad, Saldo: saldo,
			Documento: doc, Tercero: tercero, Motivo: m.Motivo, RefTipo: m.RefTipo, RefID: m.RefID,
		})
		// Solo las SALIDAS forman la lista de a quién avisar: a quien nos vendió el
		// lote no hay que avisarle de nada, y una sede propia no es un destinatario
		// —mover el lote de estante no es haberlo entregado a nadie—.
		if m.Cantidad < 0 && m.Tipo != inventario.MovTransferencia && tercero != "" && !vistos[tercero] {
			vistos[tercero] = true
			out.Clientes = append(out.Clientes, tercero)
		}
		if m.Vencimiento != "" && out.Vencimiento == "" {
			out.Vencimiento = m.Vencimiento
		}
	}
	out.EnStock = saldo
	if out.Vencimiento != "" {
		out.Vencido = out.Vencimiento < time.Now().UTC().Format("2006-01-02")
	}
	return out
}

// documentoYTerceroDe recupera el número legible y el tercero del hecho que
// originó un movimiento.
//
// Devuelve vacíos cuando no se puede resolver —un ajuste no tiene tercero, un
// documento borrado tampoco— en vez de inventar una etiqueta: en una alerta
// sanitaria, un nombre equivocado es peor que un hueco.
func (s *Service) documentoYTerceroDe(empresaID, refTipo, refID string) (string, string) {
	if refID == "" {
		return "", ""
	}
	switch refTipo {
	case "documento": // venta
		if d, ok := s.documentos.ByID(empresaID, refID); ok {
			return d.NumeroCompleto, strings.TrimSpace(d.ClienteNombre)
		}
	case "compra": // recepción de una orden
		if o, ok := s.ordenesCompra.ByID(empresaID, refID); ok {
			return o.NumeroCompleto, strings.TrimSpace(o.ProveedorNombre)
		}
	case refNotaCompra: // devolución al proveedor
		for _, n := range s.NotasCompra(empresaID) {
			if n.ID == refID {
				return n.NumeroCompleto, strings.TrimSpace(n.ProveedorNombre)
			}
		}
	case "transferencia":
		if t, ok := s.transferencias.ByID(empresaID, refID); ok {
			return "Transferencia", t.OrigenSedeID + " → " + t.DestinoSedeID
		}
	}
	return "", ""
}

// LotesDeProducto lista los lotes que ALGUNA VEZ existieron de un producto, no
// solo los que tienen saldo. Es lo que necesita el buscador del rastro: un lote
// agotado es justo el que hay que poder consultar tras una alerta.
func (s *Service) LotesDeProducto(empresaID, sku, sedeID string) []string {
	p, ok := s.productos.BySKU(empresaID, sku)
	if !ok {
		return []string{}
	}
	vistos := map[string]bool{}
	out := []string{}
	for _, m := range s.movimientos.List(empresaID, inventario.FiltroMovimiento{SedeID: sedeID, ProductoID: p.ID}) {
		if m.Lote == "" || vistos[m.Lote] {
			continue
		}
		vistos[m.Lote] = true
		out = append(out, m.Lote)
	}
	sort.Strings(out)
	return out
}
