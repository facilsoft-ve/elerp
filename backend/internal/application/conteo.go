package application

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mornix/elerp/internal/domain/inventario"
)

// CONTEO FÍSICO (cíclico).
//
// POR QUÉ EXISTE. Cuadrar un almacén producto por producto obliga a hacer una
// operación por renglón, y en la práctica eso significa que no se cuadra: nadie
// hace ochenta ajustes a mano. El conteo recibe la hoja entera y emite los ajustes
// que hagan falta, con un motivo común que los une.
//
// SE CUENTA UNA CASILLA, NO UN PRODUCTO. Quien cuenta está delante de un estante y
// ve lo que hay EN ESE ESTANTE. Si el conteo se aplicara al producto entero, contar
// un pasillo daría por contado lo que está en los demás y los pondría en cero.
//
// ES UNA LISTA DE DIFERENCIAS, NO UN INVENTARIO COMPLETO. Lo que no se declara no
// se toca: una hoja parcial es el caso normal —se cuenta un pasillo un martes y
// otro el jueves— y suponer cero en lo no declarado convertiría cada conteo parcial
// en una merma masiva.
var (
	// ErrConteoVacio: sin líneas no hay nada que cuadrar.
	ErrConteoVacio = errors.New("el conteo no tiene líneas")
	// ErrConteoNegativo: no se puede contar una cantidad negativa.
	ErrConteoNegativo = errors.New("la cantidad contada no puede ser negativa")
)

// LineaConteo es lo que alguien contó en una casilla.
type LineaConteo struct {
	SKU string
	// UbicacionID es la casilla contada. Vacío = el almacén sin más detalle, que es
	// lo correcto en un almacén que no está dividido.
	UbicacionID string
	// Contado es lo que hay de verdad en el anaquel, no la diferencia.
	Contado float64
	Lote    string
}

// ResultadoConteoLinea es lo que pasó con una línea.
type ResultadoConteoLinea struct {
	SKU         string  `json:"sku"`
	Nombre      string  `json:"nombre"`
	UbicacionID string  `json:"ubicacionId,omitempty"`
	Ubicacion   string  `json:"ubicacion"`
	Sistema     float64 `json:"sistema"`
	Contado     float64 `json:"contado"`
	Diferencia  float64 `json:"diferencia"`
	Ajustado    bool    `json:"ajustado"`
	Error       string  `json:"error,omitempty"`
}

// ResultadoConteo resume la jornada.
type ResultadoConteo struct {
	Fecha      string                 `json:"fecha"`
	SedeID     string                 `json:"sedeId"`
	AlmacenID  string                 `json:"almacenId,omitempty"`
	Motivo     string                 `json:"motivo"`
	Lineas     []ResultadoConteoLinea `json:"lineas"`
	ConAjuste  int                    `json:"conAjuste"`
	SinCambio  int                    `json:"sinCambio"`
	ConError   int                    `json:"conError"`
	ValorNeto  float64                `json:"valorNeto"`
	Diferencia float64                `json:"diferenciaUnidades"`
}

// PrevisualizarConteo dice qué pasaría, SIN tocar nada.
//
// Existe porque un conteo mal tecleado es una merma masiva que ya no se puede
// deshacer —el ledger es de solo anexado—, y el momento de descubrir que alguien
// puso 5 donde había 500 es antes de aplicarlo, no en el balance del mes.
func (s *Service) PrevisualizarConteo(empresaID, sedeID, almacenID string, lineas []LineaConteo) (ResultadoConteo, error) {
	return s.conteo(empresaID, sedeID, almacenID, "previsualización", lineas, "", "", false)
}

// AplicarConteo emite los ajustes de un conteo físico.
//
// Cada línea se ajusta por separado y un fallo no detiene las demás: si una sola
// línea se cae —un SKU que ya no existe—, abortar el conteo entero obligaría a
// recontar el almacén. Lo que se cayó se devuelve marcado, para rehacerlo solo.
func (s *Service) AplicarConteo(empresaID, sedeID, almacenID, motivo string, lineas []LineaConteo, actor, origen string) (ResultadoConteo, error) {
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		return ResultadoConteo{}, ErrMotivoRequerido
	}
	return s.conteo(empresaID, sedeID, almacenID, motivo, lineas, actor, origen, true)
}

func (s *Service) conteo(empresaID, sedeID, almacenID, motivo string, lineas []LineaConteo, actor, origen string, aplicar bool) (ResultadoConteo, error) {
	res := ResultadoConteo{
		Fecha: ahora(), SedeID: sedeID, AlmacenID: almacenID, Motivo: motivo,
		Lineas: []ResultadoConteoLinea{},
	}
	if len(lineas) == 0 {
		return res, ErrConteoVacio
	}
	// Se valida TODO antes de emitir el primer ajuste. El ledger es de solo anexado:
	// aplicar media hoja y fallar dejaría un almacén medio cuadrado, que es peor que
	// uno descuadrado — porque parece correcto.
	for _, l := range lineas {
		if l.Contado < 0 {
			return res, fmt.Errorf("%w: %s", ErrConteoNegativo, l.SKU)
		}
		if _, ok := s.productos.BySKU(empresaID, l.SKU); !ok {
			return res, fmt.Errorf("%w: %s", ErrProductoNoExiste, l.SKU)
		}
	}

	for _, l := range lineas {
		p, _ := s.productos.BySKU(empresaID, l.SKU)
		fila := ResultadoConteoLinea{
			SKU: p.SKU, Nombre: p.Nombre, UbicacionID: l.UbicacionID,
			Ubicacion: "SIN UBICAR", Contado: round2(l.Contado),
		}
		if l.UbicacionID != "" && s.ubicaciones != nil {
			if u, ok := s.ubicaciones.ByID(empresaID, l.UbicacionID); ok {
				fila.Ubicacion = u.Codigo
			} else {
				fila.Ubicacion = l.UbicacionID
			}
		}
		fila.Sistema = round2(s.existenciaEnCasilla(empresaID, sedeID, almacenID, l.UbicacionID, p.ID, l.Lote))
		fila.Diferencia = round2(fila.Contado - fila.Sistema)

		if fila.Diferencia > -0.0001 && fila.Diferencia < 0.0001 {
			res.SinCambio++
			res.Lineas = append(res.Lineas, fila)
			continue
		}
		res.Diferencia += fila.Diferencia
		_, avg := fold(s.movimientos.List(empresaID, inventario.FiltroMovimiento{SedeID: sedeID, ProductoID: p.ID}))
		res.ValorNeto += fila.Diferencia * avg

		if !aplicar {
			res.ConAjuste++
			res.Lineas = append(res.Lineas, fila)
			continue
		}
		if _, err := s.AjustarEnUbicacion(empresaID, sedeID, almacenID, l.UbicacionID, p.SKU,
			"conteo físico · "+motivo, fila.Diferencia, l.Lote, "", actor, origen); err != nil {
			fila.Error = err.Error()
			res.ConError++
		} else {
			fila.Ajustado = true
			res.ConAjuste++
		}
		res.Lineas = append(res.Lineas, fila)
	}
	res.ValorNeto = round2(res.ValorNeto)
	res.Diferencia = round2(res.Diferencia)
	sort.SliceStable(res.Lineas, func(i, j int) bool {
		if res.Lineas[i].Ubicacion != res.Lineas[j].Ubicacion {
			return res.Lineas[i].Ubicacion < res.Lineas[j].Ubicacion
		}
		return res.Lineas[i].SKU < res.Lineas[j].SKU
	})
	if aplicar {
		s.audit.Append(evento(empresaID, actor, origen, "inventario.conteo.aplicar", motivo,
			fmt.Sprintf("%d ajustadas, %d sin cambio, %d con error", res.ConAjuste, res.SinCambio, res.ConError)))
	}
	return res, nil
}

// existenciaEnCasilla proyecta lo que el sistema cree que hay en una casilla
// concreta: el almacén, su ubicación y, si se declara, un lote.
//
// Es lo que se compara con lo contado, y tiene que mirar la MISMA casilla que
// después se ajusta. Comparar contra el total del producto y ajustar una casilla
// haría que cada conteo parcial arrastrara a esa casilla la diferencia de todas
// las demás.
/* CONTRA QUÉ SE COMPARA LO CONTADO.
 *
 * SIN UBICACIÓN INDICADA SE COMPARA CONTRA TODO EL ALMACÉN, no contra la casilla
 * «sin ubicar». Es la diferencia entre «conté el pasillo B» y «conté este producto,
 * sin entrar en qué estante» — y la hoja de conteo ofrece lo segundo por defecto,
 * que es como cuenta cualquiera que no lleva ubicaciones al día.
 *
 * Filtrando exacto, en cuanto el almacén tenía estantes el sistema respondía CERO a
 * todo: quien contaba 140 y tenía 140 recibía «faltan 140 por sumar», y aplicarlo
 * DUPLICABA la existencia. No fallaba nada —la cuenta de una casilla vacía es cero,
 * y eso es cierto—, solo que respondía a una pregunta que nadie había hecho.
 *
 * No lo trajo la demostración con sus estantes sembrados: eso solo lo hizo visible.
 * Cualquier empresa que hubiera dividido su almacén y contara tenía lo mismo.
 */
func (s *Service) existenciaEnCasilla(empresaID, sedeID, almacenID, ubicacionID, productoID, lote string) float64 {
	alm := s.almacenParaEscritura(empresaID, sedeID, almacenID)
	total := 0.0
	for _, b := range s.bucketsDe(empresaID, sedeID, alm, productoID) {
		if ubicacionID != "" && b.UbicacionID != ubicacionID {
			continue
		}
		if lote != "" && b.Lote != lote {
			continue
		}
		total += b.Cantidad
	}
	return total
}
