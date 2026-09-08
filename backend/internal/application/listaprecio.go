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
// New (como ConLeadsDemo) para no romper las firmas de los constructores ya
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
	Activa bool
	Moneda string
	Items  []listaprecio.ItemLista
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
	l := listaprecio.ListaPrecio{
		EmpresaID: empresaID, Nombre: nombre, Tipo: in.Tipo,
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
	cur.Items = items
	// El Tipo se conserva de `cur`: no se muta desde la edición.

	out, ok := s.listasPrecio.Update(cur)
	if !ok {
		return listaprecio.ListaPrecio{}, ErrListaPrecioNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "ventas.listaprecio.actualizar", out.ID, out.Nombre))
	return out, nil
}
