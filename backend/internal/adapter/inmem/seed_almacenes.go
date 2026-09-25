package inmem

import (
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

// appendMov siembra un movimiento resolviendo su almacén si no trae uno. Respeta el
// que venga puesto: sembrar en un almacén concreto sigue siendo posible.
func (s *Store) appendMov(m inventario.Movimiento) inventario.Movimiento {
	if m.AlmacenID == "" {
		m.AlmacenID = s.almacenDeSede(m.EmpresaID, m.SedeID)
	}
	return s.Movimientos.Append(m)
}
