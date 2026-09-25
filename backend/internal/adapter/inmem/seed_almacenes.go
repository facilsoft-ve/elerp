package inmem

import (
	"strconv"

	"github.com/mornix/elerp/internal/domain/almacen"
	"github.com/mornix/elerp/internal/domain/inventario"
)

/* EL ALMACÉN DE LO QUE SIEMBRA EL SEED.
 *
 * El seed escribe movimientos DIRECTO al repositorio, saltándose la capa de
 * aplicación —donde `almacenParaEscritura` decide dónde va cada uno—. Resultado:
 * todo el inventario de demostración nacía con AlmacenID vacío.
 *
 * Eso no rompía nada: un movimiento sin almacén se atribuye al principal de su
 * sede, así que las cuentas cuadraban. Lo que dejaba es una DEMOSTRACIÓN QUE NO
 * DEMUESTRA: el desglose por almacén tenía una sola fila y las ubicaciones salían
 * todas «SIN UBICAR», que es justo lo que alguien evaluando el producto abre para
 * ver si sirve.
 *
 * appendMov es el equivalente de almacenParaEscritura para el seed: un solo sitio
 * decide, y los once puntos que siembran movimientos lo usan. Cualquier Append que
 * se añada después queda cubierto sin acordarse de nada.
 */

// almacenDeSede devuelve el id del almacén principal de la sede, creándolo si aún
// no existe. Idempotente: el backfill del arranque (main.go) encuentra el almacén
// ya hecho y no duplica.
func (s *Store) almacenDeSede(empresaID, sedeID string) string {
	if empresaID == "" || sedeID == "" {
		return ""
	}
	for _, a := range s.Almacenes.List(empresaID) {
		if a.SedeID == sedeID && a.Activo {
			return a.ID
		}
	}
	a := s.Almacenes.Create(almacen.Almacen{
		EmpresaID: empresaID, SedeID: sedeID,
		Nombre: "Almacén Principal", Tipo: "principal",
		Principal: true, Activo: true,
	})
	return a.ID
}

/* IDENTIDAD DE LO QUE SIEMBRA EL SEED.
 *
 * `nextID` cuenta con un contador global del PROCESO, que arranca en cero cada vez
 * que el servidor se levanta. Para datos que viven en memoria da igual; para el
 * seed no, porque sus filas se insertan en una base PERSISTENTE a lo largo de
 * muchos arranques: el que siembra hoy los insumos de cocina y el que mañana añade
 * los de repostería recorren la misma secuencia y emiten los mismos `mov_180`.
 *
 * Así llegaron a producción seis ids compartidos por dos movimientos distintos cada
 * uno. No rompe ninguna suma —el fold recorre la lista, no el mapa— pero envenena
 * todo lo que indexa por id: el asiento referencia por id, la recontabilización
 * marca por id lo ya asentado, el rastro de lotes enlaza por id. Asentar uno da por
 * asentado al otro. Y los guards de «esto ya está sembrado» preguntan por id, así
 * que una colisión hace que se salte una fila legítima.
 *
 * El id sembrado lleva ahora LA EMPRESA y un contador propio de ella. Dos empresas
 * no pueden chocar, y dos arranques con el mismo seed producen los mismos ids: los
 * guards por id vuelven a significar lo que dicen.
 *
 * Los ids ya emitidos NO cambian —hay asientos que los referencian—: esto evita la
 * próxima colisión, no repara la que ya está en la base.
 */
func (s *Store) idDeMovimiento(empresaID string) string {
	s.muSeedMov.Lock()
	defer s.muSeedMov.Unlock()
	if s.seedMovSeq == nil {
		s.seedMovSeq = map[string]int{}
	}
	s.seedMovSeq[empresaID]++
	return "mov_" + empresaID + "_" + strconv.Itoa(s.seedMovSeq[empresaID])
}

// appendMov siembra un movimiento resolviendo su almacén y su id si no traen uno.
// Respeta los que vengan puestos: sembrar en un almacén concreto, o con un id
// elegido a mano, sigue siendo posible.
func (s *Store) appendMov(m inventario.Movimiento) inventario.Movimiento {
	if m.AlmacenID == "" {
		m.AlmacenID = s.almacenDeSede(m.EmpresaID, m.SedeID)
	}
	if m.ID == "" && m.EmpresaID != "" {
		m.ID = s.idDeMovimiento(m.EmpresaID)
	}
	return s.Movimientos.Append(m)
}
