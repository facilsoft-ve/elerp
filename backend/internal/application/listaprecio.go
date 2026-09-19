package application

import (
	"errors"
	"strings"

	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/listaprecio"
)

var (
	ErrListaPrecioNoExiste = errors.New("lista de precio no existe")
	ErrTipoListaInvalido   = errors.New("tipo de lista inválido (venta o compra)")
	ErrListaNoDisponible   = errors.New("el maestro de listas de precio no está disponible")
)

// ConListasPrecio cablea el maestro de listas de precio. Se configura aparte de
// New (como ConAlmacenes) para no romper las firmas de los constructores ya
// cableados en cmd/api; sin él, el servicio funciona igual y las listas quedan
// vacías.
func (s *Service) ConListasPrecio(r listaprecio.Repository) *Service {
	s.listasPrecio = r
	return s
}

// EntradaListaPrecio son los datos para crear/editar una lista de precio.
type EntradaListaPrecio struct {
	Nombre string
	Tipo   string
	// ProveedorID ata una lista de COMPRA a su proveedor. Vacío = lista sin dueño
	// (sigue siendo válida; simplemente nadie la propone en una orden).
	ProveedorID string
	Activa      bool
	Moneda      string
	Items       []listaprecio.ItemLista
}

// normalizarItemsLista limpia los ítems: descarta los que no traen SKU, recorta
// el SKU y valida que ningún precio sea negativo. Deduplica por SKU (el último
// gana), para que editar la misma fila dos veces no genere renglones duplicados.
func normalizarItemsLista(in []listaprecio.ItemLista) ([]listaprecio.ItemLista, error) {
	orden := make([]string, 0, len(in))
	porSKU := make(map[string]listaprecio.ItemLista, len(in))
	for _, it := range in {
		sku := strings.TrimSpace(it.SKU)
		if sku == "" {
			continue
		}
		if it.Precio < 0 {
			return nil, errors.New("los precios de la lista no pueden ser negativos")
		}
		if _, ya := porSKU[sku]; !ya {
			orden = append(orden, sku)
		}
		porSKU[sku] = listaprecio.ItemLista{SKU: sku, Precio: round2(it.Precio)}
	}
	items := make([]listaprecio.ItemLista, 0, len(orden))
	for _, sku := range orden {
		items = append(items, porSKU[sku])
	}
	return items, nil
}

// ListasPrecio lista las listas de precio de la empresa, opcionalmente filtradas
// por tipo ("venta"|"compra"; vacío = todas).
func (s *Service) ListasPrecio(empresaID, tipo string) []listaprecio.ListaPrecio {
	out := []listaprecio.ListaPrecio{}
	if s.listasPrecio == nil {
		return out
	}
	for _, l := range s.listasPrecio.List(empresaID) {
		if tipo == "" || l.Tipo == tipo {
			out = append(out, l)
		}
	}
	return out
}

// CrearListaPrecio da de alta una lista de precio validando nombre, tipo y precios.
func (s *Service) CrearListaPrecio(empresaID, actor, origen string, in EntradaListaPrecio) (listaprecio.ListaPrecio, error) {
	if s.listasPrecio == nil {
		return listaprecio.ListaPrecio{}, ErrListaNoDisponible
	}
	nombre := strings.TrimSpace(in.Nombre)
	if nombre == "" {
		return listaprecio.ListaPrecio{}, errors.New("nombre requerido")
	}
	if !listaprecio.TipoValido(in.Tipo) {
		return listaprecio.ListaPrecio{}, ErrTipoListaInvalido
	}
	items, err := normalizarItemsLista(in.Items)
	if err != nil {
		return listaprecio.ListaPrecio{}, err
	}
	moneda := strings.ToUpper(strings.TrimSpace(in.Moneda))
	if moneda == "" {
		moneda = empresa.MonedaVES
	}
	provID, err := s.proveedorDeLista(empresaID, in.Tipo, in.ProveedorID)
	if err != nil {
		return listaprecio.ListaPrecio{}, err
	}
	l := listaprecio.ListaPrecio{
		EmpresaID: empresaID, Nombre: nombre, Tipo: in.Tipo, ProveedorID: provID,
		Activa: in.Activa, Moneda: moneda, Items: items,
	}
	out := s.listasPrecio.Create(l)
	s.audit.Append(evento(empresaID, actor, origen, "ventas.listaprecio.crear", out.ID, out.Nombre))
	return out, nil
}

// ActualizarListaPrecio edita una lista existente del tenant (maestro editable,
// no ledger): nombre, actividad, moneda e ítems. El tipo NO se cambia (una lista
// de venta no se convierte en una de compra: son maestros de módulos distintos).
func (s *Service) ActualizarListaPrecio(empresaID, id, actor, origen string, in EntradaListaPrecio) (listaprecio.ListaPrecio, error) {
	if s.listasPrecio == nil {
		return listaprecio.ListaPrecio{}, ErrListaNoDisponible
	}
	cur, ok := s.listasPrecio.ByID(empresaID, id)
	if !ok {
		return listaprecio.ListaPrecio{}, ErrListaPrecioNoExiste
	}
	nombre := strings.TrimSpace(in.Nombre)
	if nombre == "" {
		return listaprecio.ListaPrecio{}, errors.New("nombre requerido")
	}
	items, err := normalizarItemsLista(in.Items)
	if err != nil {
		return listaprecio.ListaPrecio{}, err
	}
	cur.Nombre = nombre
	cur.Activa = in.Activa
	if m := strings.ToUpper(strings.TrimSpace(in.Moneda)); m != "" {
		cur.Moneda = m
	}
	provID, err := s.proveedorDeLista(empresaID, cur.Tipo, in.ProveedorID)
	if err != nil {
		return listaprecio.ListaPrecio{}, err
	}
	cur.ProveedorID = provID
	cur.Items = items
	// El Tipo se conserva de `cur`: no se muta desde la edición.

	out, ok := s.listasPrecio.Update(cur)
	if !ok {
		return listaprecio.ListaPrecio{}, ErrListaPrecioNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "ventas.listaprecio.actualizar", out.ID, out.Nombre))
	return out, nil
}

/* --- Listas de COMPRA atadas al proveedor ---------------------------------- */

// ErrListaProveedorSoloCompra: una lista de VENTA no tiene proveedor. El precio
// al cliente no depende de a quién se le compró.
var ErrListaProveedorSoloCompra = errors.New("solo una lista de compra puede tener proveedor")

// proveedorDeLista valida el proveedor declarado para una lista. Vacío se acepta
// (lista sin dueño); en una lista de venta, declararlo es un error, no un dato que
// se ignora en silencio.
func (s *Service) proveedorDeLista(empresaID, tipo, provID string) (string, error) {
	provID = strings.TrimSpace(provID)
	if provID == "" {
		return "", nil
	}
	if tipo != listaprecio.TipoCompra {
		return "", ErrListaProveedorSoloCompra
	}
	if s.provs == nil {
		return "", ErrProveedorNoExiste
	}
	if _, ok := s.provs.ByID(empresaID, provID); !ok {
		return "", ErrProveedorNoExiste
	}
	return provID, nil
}

// ListaDeCompraDe devuelve la lista de precios de COMPRA activa de un proveedor.
// Si tiene más de una activa gana la PRIMERA del maestro: son tarifas negociadas,
// no promociones que compitan entre sí, y tener dos vigentes es un error de datos
// que el usuario debe resolver — no algo que el sistema deba adivinar.
func (s *Service) ListaDeCompraDe(empresaID, proveedorID string) (listaprecio.ListaPrecio, bool) {
	if s.listasPrecio == nil || strings.TrimSpace(proveedorID) == "" {
		return listaprecio.ListaPrecio{}, false
	}
	for _, l := range s.listasPrecio.List(empresaID) {
		if l.Tipo == listaprecio.TipoCompra && l.Activa && l.ProveedorID == proveedorID {
			return l, true
		}
	}
	return listaprecio.ListaPrecio{}, false
}

// CostoPactadoCon devuelve el costo que la tarifa del proveedor fija para un SKU.
// El booleano distingue «no hay tarifa» de «la tarifa dice 0»: un precio de cero
// puede ser legítimo (una muestra, un obsequio) y no debe confundirse con la
// ausencia de dato, que es lo que deja el costo a criterio de quien compra.
func (s *Service) CostoPactadoCon(empresaID, proveedorID, sku string) (float64, bool) {
	lista, ok := s.ListaDeCompraDe(empresaID, proveedorID)
	if !ok {
		return 0, false
	}
	sku = strings.TrimSpace(sku)
	for _, it := range lista.Items {
		if it.SKU == sku {
			return it.Precio, true
		}
	}
	return 0, false
}
