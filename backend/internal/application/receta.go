package application

import (
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/inventario"
)

// consumoInventario es una salida/entrada de inventario resuelta: qué SKU y cuánto.
// Es el resultado de EXPANDIR una línea fiscal a lo que realmente mueve el stock:
// el propio producto, o —si es un plato con receta— sus insumos.
type consumoInventario struct {
	SKU        string
	ProductoID string
	Cantidad   float64
}

// recetaSnapshot devuelve el snapshot de insumos de un plato (escandallo) para
// grabarlo en la línea fiscal al emitir. Nil para un producto normal. Resuelve el
// nombre/ID de cada insumo del catálogo actual (memoria del documento).
func (s *Service) recetaSnapshot(empresaID string, p inventario.Producto) []fiscal.InsumoLinea {
	if !p.EsPlato || len(p.Receta) == 0 {
		return nil
	}
	out := make([]fiscal.InsumoLinea, 0, len(p.Receta))
	for _, comp := range p.Receta {
		nombre, pid := comp.SKU, ""
		if ins, ok := s.productos.BySKU(empresaID, comp.SKU); ok {
			nombre, pid = ins.Nombre, ins.ID
		}
		out = append(out, fiscal.InsumoLinea{SKU: comp.SKU, ProductoID: pid, Nombre: nombre, CantidadUnitaria: comp.Cantidad})
	}
	return out
}

// consumosDeLinea expande una línea fiscal a lo que mueve el inventario por
// `cantidad` unidades del renglón: si la línea trae receta snapshot (plato), son
// sus insumos (cantidadUnitaria × cantidad); si no, el propio SKU (× cantidad).
// Se usa idéntico al emitir (salida), anular y en la nota de crédito (reingreso),
// para que el stock siempre cuadre.
func consumosDeLinea(l fiscal.Linea, cantidad float64) []consumoInventario {
	if len(l.Insumos) > 0 {
		out := make([]consumoInventario, 0, len(l.Insumos))
		for _, in := range l.Insumos {
			out = append(out, consumoInventario{SKU: in.SKU, ProductoID: in.ProductoID, Cantidad: in.CantidadUnitaria * cantidad})
		}
		return out
	}
	return []consumoInventario{{SKU: l.SKU, ProductoID: l.ProductoID, Cantidad: cantidad}}
}
