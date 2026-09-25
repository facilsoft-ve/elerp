package application

import "log"

/* LOS PERÍODOS CERRADOS DE LA DEMOSTRACIÓN.
 *
 * Un mes cerrado es una función del producto —y de las que un contador mira
 * primero—, así que la demo tiene que enseñarla funcionando. Pero un cierre no se
 * puede sembrar como se siembra un cliente: rechaza todo asiento con fecha anterior,
 * de modo que si estuviera puesto ANTES de que el arranque recontabilice, el libro
 * no se podría construir y el tenant quedaría vacío.
 *
 * Por eso se cierran acá, al final del arranque y no en el seed: primero se
 * reconstruye el libro entero, y después se le pone la barrera encima. El orden es
 * el mismo que en la vida real —se cierra un mes cuando ya está contabilizado— y es
 * lo que permite regenerar un tenant demo cuantas veces haga falta.
 */

// periodoDemo es un mes que un tenant de DEMOSTRACIÓN tiene cerrado.
type periodoDemo struct {
	empresaID string
	anio, mes int
}

// periodosDemo son los cierres que la demostración enseña.
//
// La bodega lleva dos meses seguidos, que es lo que hace visible la regla de que se
// cierra en orden y sin saltos. El restaurante lleva agosto: su inventario inicial
// es de julio, así que el cierre queda por encima de datos que ya existen, que es
// justo el caso que hay que poder mostrar sin que nada se rompa.
var periodosDemo = []periodoDemo{
	{"emp_demo", 2026, 5},
	{"emp_demo", 2026, 6},
	{"emp_demo_rest", 2026, 8},
}

// AsegurarPeriodosDemo cierra los meses de demostración que falten. Idempotente: un
// mes ya cerrado se salta, y cerrar es de solo-anexado (§7.3).
//
// Solo actúa sobre los tenants de la lista, que son de demostración. Ninguna empresa
// real se cierra sola: eso lo decide quien lleva la contabilidad.
func (s *Service) AsegurarPeriodosDemo() int {
	if s.periodos == nil || s.empresas == nil {
		return 0
	}
	n := 0
	for _, p := range periodosDemo {
		if _, existe := s.empresas.ByID(p.empresaID); !existe {
			continue // esta instancia no tiene ese tenant demo
		}
		yaCerrado := false
		for _, cerrado := range s.periodos.List(p.empresaID) {
			if cerrado.Anio == p.anio && cerrado.Mes == p.mes {
				yaCerrado = true
				break
			}
		}
		if yaCerrado {
			continue
		}
		if _, err := s.CerrarPeriodo(p.empresaID, "sistema", "arranque", p.anio, p.mes); err != nil {
			// No es fatal: la demo funciona igual sin el cierre, y el motivo más
			// probable es benigno (el mes todavía no terminó).
			log.Printf("Demo: no se pudo cerrar %d-%02d de %s: %v", p.anio, p.mes, p.empresaID, err)
			continue
		}
		n++
	}
	return n
}
