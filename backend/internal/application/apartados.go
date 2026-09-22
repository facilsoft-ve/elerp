package application

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/inventario"
)

// APARTADOS: mercancía comprometida que todavía no ha salido.
// Ver domain/inventario/apartado.go para el porqué.
//
// DÓNDE MUERDE, y son DOS SITIOS con dos efectos distintos. La diferencia importa:
//
//  1. En bucketsDe, que es el único camino por el que sale mercancía. Ahí lo
//     apartado deja de surtir, así que cualquier salida PREFIERE lo libre sin que
//     tenga que saber que los apartados existen. Es reparto, no prohibición.
//
//  2. En la validación PREVIA a emitir una venta. Ahí es donde se dice que no, y
//     tiene que ser ahí: el anexado tolera el descubierto a propósito —una venta
//     ya ocurrió y negarla no devuelve la mercancía al anaquel—, de modo que una
//     guarda puesta en el reparto no habría negado ninguna venta, solo anotado el
//     faltante después de emitir la factura.
//
// Y HAY UNA SALIDA QUE NO SE BLOQUEA, a propósito: el AJUSTE. Si se rompió una
// caja, se rompió, esté apartada o no; negar la merma solo conseguiría que el
// inventario siguiera contando mercancía que ya no existe.
var (
	// ErrApartadoNoExiste: el apartado no está o no es de esta empresa.
	ErrApartadoNoExiste = errors.New("el apartado no existe")
	// ErrApartadoVacio: un apartado sin líneas no compromete nada.
	ErrApartadoVacio = errors.New("el apartado no tiene líneas")
	// ErrApartadoSinDisponible: no queda libre lo suficiente. Es LA guarda: sin
	// ella, dos apartados comprometerían la misma unidad y el segundo cliente se
	// quedaría sin nada con las dos ventas hechas.
	ErrApartadoSinDisponible = errors.New("no hay disponible suficiente: parte ya está apartada")
	// ErrApartadoCerrado: ya se despachó o se liberó; no admite más cambios.
	ErrApartadoCerrado = errors.New("el apartado ya está cerrado")
)

// ConApartados cablea el repositorio. Sin él nada aparta y el disponible es la
// existencia, que es como funcionaba antes.
func (s *Service) ConApartados(r inventario.ApartadoRepo) *Service {
	s.apartados = r
	return s
}

// Apartados lista los apartados de una empresa, los abiertos primero.
func (s *Service) Apartados(empresaID, sedeID string) []inventario.Apartado {
	if s.apartados == nil {
		return []inventario.Apartado{}
	}
	out := []inventario.Apartado{}
	for _, a := range s.apartados.List(empresaID) {
		if sedeID != "" && a.SedeID != sedeID {
			continue
		}
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].Estado == inventario.ApartadoAbierto) != (out[j].Estado == inventario.ApartadoAbierto) {
			return out[i].Estado == inventario.ApartadoAbierto
		}
		return out[i].Creado > out[j].Creado
	})
	return out
}

// apartadoPorCasilla proyecta cuánto hay comprometido de un producto, desglosado
// por la casilla de la que se apartó.
//
// Solo cuentan los ABIERTOS: un apartado despachado ya salió por el ledger, y
// restarlo otra vez lo contaría dos veces — el disponible bajaría solo, sin que
// nada hubiera pasado.
//
// `exceptoID` deja fuera un apartado concreto. Lo usa la validación al AMPLIAR un
// apartado existente: sin esta salvedad, lo que ya tiene comprometido contaría
// contra sí mismo y no podría ni mantenerse igual.
func (s *Service) apartadoPorCasilla(empresaID, sedeID, productoID, exceptoID string) map[claveCasilla]float64 {
	out := map[claveCasilla]float64{}
	if s.apartados == nil {
		return out
	}
	for _, a := range s.apartados.List(empresaID) {
		if a.Estado != inventario.ApartadoAbierto || a.ID == exceptoID {
			continue
		}
		if sedeID != "" && a.SedeID != sedeID {
			continue
		}
		for _, l := range a.Lineas {
			if l.ProductoID != productoID {
				continue
			}
			out[claveCasilla{Lote: l.Lote, Almacen: a.AlmacenID, Ubicacion: l.UbicacionID}] += l.Cantidad
		}
	}
	return out
}

// claveCasilla identifica una casilla del ledger: lote + almacén + ubicación.
type claveCasilla struct{ Lote, Almacen, Ubicacion string }

// Apartado devuelve cuánto hay comprometido de un producto en una sede.
func (s *Service) Apartado(empresaID, sedeID, productoID string) float64 {
	total := 0.0
	for _, c := range s.apartadoPorCasilla(empresaID, sedeID, productoID, "") {
		total += c
	}
	return round2(total)
}

// CrearApartado compromete mercancía.
//
// Valida TODAS las líneas antes de crear nada: un apartado a medias comprometería
// parte del pedido y dejaría a quien lo pidió creyendo que tiene el pedido entero.
func (s *Service) CrearApartado(empresaID, actor, origen string, a inventario.Apartado) (inventario.Apartado, error) {
	if s.apartados == nil {
		return inventario.Apartado{}, errors.New("los apartados no están disponibles")
	}
	limpias := []inventario.LineaApartado{}
	for _, l := range a.Lineas {
		if l.Cantidad <= 0.0001 {
			continue
		}
		p, ok := s.productos.BySKU(empresaID, l.SKU)
		if !ok {
			return inventario.Apartado{}, fmt.Errorf("%w: %s", ErrProductoNoExiste, l.SKU)
		}
		if p.EsCombo {
			return inventario.Apartado{}, fmt.Errorf("%w: %s", ErrComboNoStockeable, l.SKU)
		}
		l.ProductoID = p.ID
		// La guarda: lo que se aparta tiene que estar LIBRE. Se mira el disponible,
		// no la existencia — si se mirara la existencia, dos apartados podrían
		// comprometer la misma unidad y nada fallaría hasta el despacho.
		if libre := s.DisponibleDe(empresaID, a.SedeID, p.SKU); l.Cantidad > libre+0.0001 {
			return inventario.Apartado{}, fmt.Errorf("%w: %s (libre %.2f, pedido %.2f)",
				ErrApartadoSinDisponible, p.SKU, libre, l.Cantidad)
		}
		limpias = append(limpias, l)
	}
	if len(limpias) == 0 {
		return inventario.Apartado{}, ErrApartadoVacio
	}
	a.Lineas = limpias
	a.EmpresaID, a.Actor = empresaID, actor
	a.AlmacenID = s.almacenParaEscritura(empresaID, a.SedeID, a.AlmacenID)
	a.Estado = inventario.ApartadoAbierto
	a.Motivo = strings.TrimSpace(a.Motivo)
	a.Creado = ahora()
	out := s.apartados.Create(a)
	s.audit.Append(evento(empresaID, actor, origen, "inventario.apartado.crear", out.ID, out.Motivo))
	return out, nil
}

// LiberarApartado deja de comprometer la mercancía. No borra el documento: se
// conserva para poder explicar por qué estuvo apartada.
func (s *Service) LiberarApartado(empresaID, id, actor, origen string) (inventario.Apartado, error) {
	if s.apartados == nil {
		return inventario.Apartado{}, errors.New("los apartados no están disponibles")
	}
	a, ok := s.apartados.ByID(empresaID, id)
	if !ok {
		return inventario.Apartado{}, ErrApartadoNoExiste
	}
	if a.Estado != inventario.ApartadoAbierto {
		return inventario.Apartado{}, ErrApartadoCerrado
	}
	a.Estado, a.Cerrado = inventario.ApartadoLiberado, ahora()
	out, _ := s.apartados.Update(a)
	s.audit.Append(evento(empresaID, actor, origen, "inventario.apartado.liberar", out.ID, out.Motivo))
	return out, nil
}

// DespacharApartado emite la salida de lo comprometido y cierra el apartado.
//
// Es el segundo paso de una entrega. La salida sale POR EL CAMINO DE SIEMPRE: el
// ajuste, que reparte por FEFO y anexa al ledger. No hay una ruta especial para
// los apartados, y eso importa — una salida que no pasara por ahí se saltaría el
// reparto por lote y por ubicación, y el saldo cuadraría con el sitio equivocado.
func (s *Service) DespacharApartado(empresaID, id, actor, origen string) (inventario.Apartado, error) {
	if s.apartados == nil {
		return inventario.Apartado{}, errors.New("los apartados no están disponibles")
	}
	a, ok := s.apartados.ByID(empresaID, id)
	if !ok {
		return inventario.Apartado{}, ErrApartadoNoExiste
	}
	if a.Estado != inventario.ApartadoAbierto {
		return inventario.Apartado{}, ErrApartadoCerrado
	}
	// Se marca despachado ANTES de emitir las salidas, no después. Es lo que deja
	// de contarlo contra el disponible durante su propio despacho; hacerlo al final
	// haría que cada línea compitiera con la reserva que la respalda.
	a.Estado, a.Cerrado = inventario.ApartadoDespachado, ahora()
	if _, ok := s.apartados.Update(a); !ok {
		return inventario.Apartado{}, ErrApartadoNoExiste
	}
	motivo := a.Motivo
	if motivo == "" {
		motivo = "despacho de apartado"
	}
	for _, l := range a.Lineas {
		if _, err := s.ajustar(empresaID, a.SedeID, a.AlmacenID, l.UbicacionID, l.SKU,
			motivo, -l.Cantidad, l.Lote, "", actor, origen); err != nil {
			// Se revierte el estado: dejarlo despachado con las salidas a medias
			// descontaría del inventario una mercancía que sigue en el anaquel.
			a.Estado, a.Cerrado = inventario.ApartadoAbierto, ""
			s.apartados.Update(a)
			return inventario.Apartado{}, err
		}
	}
	s.audit.Append(evento(empresaID, actor, origen, "inventario.apartado.despachar", a.ID, motivo))
	return a, nil
}

// validarVentaContraApartados niega una venta que se comería mercancía apartada.
//
// VA ANTES DE NUMERAR, junto a la validación de lotes vendibles, y no en el
// anexado. El anexado tolera el descubierto a propósito —una venta ya ocurrió y
// negarla no devuelve la mercancía al anaquel—, así que una guarda puesta ahí no
// habría negado nada: habría dejado pasar la venta y anotado el faltante. Aquí sí
// se puede decir que no, porque todavía no se ha emitido el documento.
//
// Se agregan los consumos por producto antes de comprobar: dos líneas del mismo
// SKU tienen que mirarse juntas, o cada una pasaría por su cuenta y entre las dos
// se llevarían lo apartado.
func (s *Service) validarVentaContraApartados(empresaID, sedeID string, lineas []fiscal.Linea) error {
	if s.apartados == nil {
		return nil
	}
	porProducto := map[string]float64{}
	for _, l := range lineas {
		for _, cs := range consumosDeLinea(l, l.Cantidad) {
			porProducto[cs.ProductoID] += cs.Cantidad
		}
	}
	for prodID, cant := range porProducto {
		comprometido := s.Apartado(empresaID, sedeID, prodID)
		if comprometido <= 0.0001 {
			continue // nada apartado de este producto: la venta va como siempre
		}
		p, _ := s.productos.ByID(empresaID, prodID)
		existencia, _ := fold(s.movimientos.List(empresaID, inventario.FiltroMovimiento{SedeID: sedeID, ProductoID: prodID}))
		if libre := round2(existencia - comprometido); cant > libre+0.0001 {
			return fmt.Errorf("%w: %s (libre %.2f, se piden %.2f; %.2f están apartadas)",
				ErrApartadoSinDisponible, p.SKU, libre, cant, comprometido)
		}
	}
	return nil
}

// DisponibleDe es la existencia MENOS lo apartado: lo que de verdad se puede
// vender hoy. Es la cifra que debería mirar quien promete una entrega.
func (s *Service) DisponibleDe(empresaID, sedeID, sku string) float64 {
	p, ok := s.productos.BySKU(empresaID, sku)
	if !ok {
		return 0
	}
	cant, _ := fold(s.movimientos.List(empresaID, inventario.FiltroMovimiento{SedeID: sedeID, ProductoID: p.ID}))
	return round2(cant - s.Apartado(empresaID, sedeID, p.ID))
}
