package application

import (
	"errors"
	"sort"
	"strings"

	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/inventario"
)

// CUENTA DE INVENTARIO POR RUBRO.
//
// POR QUÉ EXISTE. Todo el inventario se acumulaba en una sola cuenta de activo. A
// una bodega le sobra; a una ferretería que vende tornillos y también maquinaria,
// no: quiere verlos separados en el balance, y con una sola cuenta su contador lo
// estima a mano en cada cierre.
//
// LA REGLA QUE NO SE PUEDE ROMPER. Si un rubro tiene cuenta propia, TODOS sus
// asientos van a ella: la entrada de compra, el costo de la venta, la merma, el
// sobrante, la devolución al proveedor, la reversa de una nota de crédito y el
// costo en destino. En cuanto uno solo se quedara en la cuenta general, esa cuenta
// crecería para siempre y la general se iría a negativo — con el balance cuadrando
// en los dos casos, porque el total sigue siendo el mismo. Sería peor que no tener
// la función: un descuadre invisible dentro de un balance correcto.
//
// Por eso la cuenta la resuelve UN SOLO SITIO —cuentaInventarioDe— y los asientos
// que mueven varios productos a la vez reparten con lineasDeInventario, que agrupa
// por cuenta y garantiza que la suma no cambia.
var (
	// ErrRubroNoExiste: el rubro no está o no es de esta empresa.
	ErrRubroNoExiste = errors.New("el rubro no existe")
	// La cuenta declarada se valida contra el plan AL DECLARARLA (con el
	// ErrCuentaNoExiste que ya existe en contabilidad.go) y no al asentar: un
	// asiento contra una cuenta inexistente no se guarda —solo se registra el
	// fallo—, así que el error aparecería como una AUSENCIA de asientos que nadie
	// relacionaría con haber tocado un rubro.
	//
	// ErrCuentaNoEsActivo: el inventario es un activo. Acumularlo en una cuenta de
	// otro tipo descuadra el balance por clasificación, que es un error que los
	// totales no delatan.
	ErrCuentaNoEsActivo = errors.New("la cuenta de inventario tiene que ser de tipo activo")
)

// ActualizarCuentaRubro asigna (o quita, con cuenta vacía) la cuenta de inventario
// de un rubro.
func (s *Service) ActualizarCuentaRubro(empresaID, rubroID, cuenta, actor, origen string) (inventario.Rubro, error) {
	var r inventario.Rubro
	encontrado := false
	for _, x := range s.Rubros(empresaID) {
		if x.ID == rubroID {
			r, encontrado = x, true
			break
		}
	}
	if !encontrado {
		return inventario.Rubro{}, ErrRubroNoExiste
	}
	cuenta = strings.TrimSpace(cuenta)
	if cuenta != "" {
		c, ok := s.cuentaContable(empresaID, cuenta)
		if !ok {
			return inventario.Rubro{}, ErrCuentaNoExiste
		}
		if c.Tipo != contabilidad.TipoActivo {
			return inventario.Rubro{}, ErrCuentaNoEsActivo
		}
	}
	anterior := r.CuentaInventario
	if anterior == "" {
		anterior = contabilidad.CtaInventario
	}
	destinoCuenta := cuenta
	if destinoCuenta == "" {
		destinoCuenta = contabilidad.CtaInventario
	}
	// El valor que ya tiene este rubro en el almacén, ANTES de mover nada: es lo que
	// hay que reclasificar.
	valor := s.valorDelRubro(empresaID, r.Nombre)

	r.CuentaInventario = cuenta
	out, ok := s.rubros.Update(r)
	if !ok {
		return inventario.Rubro{}, ErrRubroNoExiste
	}
	s.reclasificarInventarioDeRubro(empresaID, actor, out.Nombre, anterior, destinoCuenta, valor)

	destino := cuenta
	if destino == "" {
		destino = contabilidad.CtaInventario + " (la general)"
	}
	s.audit.Append(evento(empresaID, actor, origen, "inventario.rubro.cuenta", out.Nombre, destino))
	return out, nil
}

// valorDelRubro es lo que vale hoy, en toda la empresa, el inventario de un rubro.
func (s *Service) valorDelRubro(empresaID, rubro string) float64 {
	if s.productos == nil || s.movimientos == nil {
		return 0
	}
	total := 0.0
	for _, p := range s.productos.List(empresaID) {
		if p.Rubro != rubro || p.EsCombo || p.EsPlato {
			continue
		}
		cant, avg := fold(s.movimientos.List(empresaID, inventario.FiltroMovimiento{ProductoID: p.ID}))
		total += cant * avg
	}
	return round2(total)
}

// reclasificarInventarioDeRubro mueve el saldo que ya existe de una cuenta a otra.
//
// SIN ESTO LA FUNCIÓN NACERÍA ROTA. Asignarle una cuenta a un rubro cambia dónde se
// valora TODO su inventario —incluido el que ya estaba—, pero los asientos hechos
// hasta ese momento siguen donde se hicieron. Una empresa que active esto teniendo
// mercancía se encontraría, desde el primer día, con que el informe de valoración
// le dice que su inventario está descuadrado; y tendría razón, porque lo estaría.
//
// El asiento es una RECLASIFICACIÓN: el mismo importe cambia de cuenta dentro del
// activo. Ni el patrimonio ni el resultado se mueven, y por eso no pasa por el
// estado de resultados: no ha ocurrido ningún hecho económico, solo se ha decidido
// contarlo en otro renglón.
func (s *Service) reclasificarInventarioDeRubro(empresaID, actor, rubro, desde, hasta string, valor float64) {
	if s.asientos == nil || desde == hasta || valor <= 0.004 {
		return
	}
	s.asentar(empresaID, actor, "", "Reclasificación de inventario · rubro "+rubro,
		"reclasificacion_rubro", rubro, []contabilidad.Linea{
			{Codigo: hasta, Debe: round2(valor)},
			{Codigo: desde, Haber: round2(valor)},
		})
}

// cuentaContable busca una cuenta por código EN EL PLAN COMPLETO, sembrándolo si
// aún no existe.
//
// Tiene que ser así y no leyendo la lista cruda: el plan se siembra perezosamente,
// así que una empresa que todavía no ha registrado ningún asiento lo tiene vacío.
// Validar contra la lista cruda hacía que configurar el rubro antes del primer
// movimiento respondiera «esa cuenta no existe» de cuentas que sí están en su plan
// — y quien lo viera no tendría forma de adivinar que el problema era el momento.
func (s *Service) cuentaContable(empresaID, codigo string) (contabilidad.Cuenta, bool) {
	if s.cuentas == nil {
		return contabilidad.Cuenta{}, false
	}
	for _, c := range s.PlanDeCuentas(empresaID) {
		if c.Codigo == codigo {
			return c, true
		}
	}
	return contabilidad.Cuenta{}, false
}

// cuentaExisteEnPlan es la comprobación de SOLO LECTURA que usa el camino de los
// asientos. Separada a propósito de cuentaContable: aquélla siembra —escribe—, y
// el reparto de un asiento se ejecuta en cada venta y cada recepción. Sembrar
// desde ahí convertiría una lectura en una escritura sin que nadie lo pidiera.
func (s *Service) cuentaExisteEnPlan(empresaID, codigo string) bool {
	if s.cuentas == nil {
		return false
	}
	for _, c := range s.cuentas.List(empresaID) {
		if c.Codigo == codigo {
			return true
		}
	}
	return false
}

// cuentaInventarioDe resuelve en qué cuenta se acumula un producto.
//
// ES EL ÚNICO CAMINO. Espeja almacenParaEscritura y ubicacionParaEscritura por el
// mismo motivo: si cada asiento eligiera por su cuenta, el primero que se olvidara
// dejaría la cuenta del rubro sin cerrar, y eso no falla — solo hace que dos
// cuentas del balance dejen de significar lo que dicen.
//
// Cae a la cuenta general en TODOS los casos dudosos (sin rubro, rubro borrado,
// cuenta que ya no existe en el plan) porque el asiento tiene que salir: uno
// descuadrado no se guarda, así que un rubro mal configurado no puede tener como
// consecuencia que desaparezcan asientos.
func (s *Service) cuentaInventarioDe(empresaID, productoID string) string {
	if s.rubros == nil || productoID == "" {
		return contabilidad.CtaInventario
	}
	p, ok := s.productos.ByID(empresaID, productoID)
	if !ok || strings.TrimSpace(p.Rubro) == "" {
		return contabilidad.CtaInventario
	}
	for _, r := range s.Rubros(empresaID) {
		if r.Nombre != p.Rubro || r.CuentaInventario == "" {
			continue
		}
		if !s.cuentaExisteEnPlan(empresaID, r.CuentaInventario) {
			// La cuenta se validó al declararla, pero un plan editado después puede
			// haberla dejado sin respaldo. Mejor la general que ningún asiento.
			return contabilidad.CtaInventario
		}
		return r.CuentaInventario
	}
	return contabilidad.CtaInventario
}

// montoPorProducto acumula importes por producto para repartirlos después.
type montoPorProducto map[string]float64

// desgloseDeDocumento reparte el costo de un documento entre los productos que lo
// movieron, leyéndolo DEL PROPIO LEDGER.
//
// POR QUÉ ASÍ y no pasando el desglose desde quien llama: el total ya se calculó
// recorriendo esos mismos movimientos, así que sacar de ahí el reparto hace que las
// dos cifras no puedan discrepar. Pasarlo por parámetro habría exigido que seis
// sitios lo llenaran bien, y el primero que se olvidara habría dejado un asiento
// descuadrado — que en este sistema no se guarda: el fallo aparecería como una
// ausencia de asientos, no como un error.
//
// El reparto se hace PROPORCIONAL al total recibido y el residuo del redondeo va
// al producto de mayor importe, de modo que la suma es exactamente `total` hasta
// el céntimo. Devuelve nil cuando no puede repartir con seguridad —sin movimientos,
// o con una diferencia que no se explica por redondeo—, y entonces el asiento sale
// con la cuenta general, exactamente como antes.
func (s *Service) desgloseDeDocumento(empresaID, docID string, total float64) montoPorProducto {
	if total <= 0.004 {
		return nil
	}
	crudo := montoPorProducto{}
	suma := 0.0
	for _, m := range s.movimientos.List(empresaID, inventario.FiltroMovimiento{}) {
		if m.RefTipo != "documento" || m.RefID != docID || m.Tipo == inventario.MovRevaluacion {
			continue
		}
		c := m.Cantidad
		if c < 0 {
			c = -c
		}
		importe := c * m.CostoUnitario
		if importe <= 0 {
			continue
		}
		crudo[m.ProductoID] += importe
		suma += importe
	}
	if len(crudo) == 0 || suma <= 0.004 {
		return nil
	}
	// Una diferencia grande significa que el total no salió de estos movimientos;
	// repartirlo igualmente inventaría un desglose. Mejor la cuenta general.
	if dif := suma - total; dif > 0.05 || dif < -0.05 {
		return nil
	}

	out := montoPorProducto{}
	mayor, mayorMonto, acumulado := "", 0.0, 0.0
	for prodID, importe := range crudo {
		parte := round2(total * importe / suma)
		out[prodID] = parte
		acumulado += parte
		if importe > mayorMonto {
			mayor, mayorMonto = prodID, importe
		}
	}
	// El residuo del redondeo al de mayor importe: repartir en porcentajes nunca
	// suma exacto, y un céntimo suelto descuadra el asiento entero.
	if resto := round2(total - acumulado); resto != 0 && mayor != "" {
		out[mayor] = round2(out[mayor] + resto)
	}
	return out
}

// lineasInventarioDeDocumento es el atajo que usan los asientos de venta y de
// reversa: desglosa por rubro si puede y, si no, deja la línea única de siempre.
func (s *Service) lineasInventarioDeDocumento(empresaID, docID string, total float64, alDebe bool) []contabilidad.Linea {
	if montos := s.desgloseDeDocumento(empresaID, docID, total); montos != nil {
		return s.lineasDeInventario(empresaID, montos, alDebe)
	}
	if alDebe {
		return []contabilidad.Linea{{Codigo: contabilidad.CtaInventario, Debe: round2(total)}}
	}
	return []contabilidad.Linea{{Codigo: contabilidad.CtaInventario, Haber: round2(total)}}
}

// lineasDeInventario convierte importes por producto en líneas contables agrupadas
// por cuenta.
//
// `alDebe` decide el lado: el inventario sube (compra, sobrante, reingreso) o baja
// (venta, merma, devolución).
//
// LA SUMA NO CAMBIA. Es lo que hace seguro este cambio: un asiento que antes tenía
// una línea de 500 en la cuenta general ahora puede tener dos de 300 y 200, pero el
// total es el mismo y la contrapartida no se toca. Sin cuentas por rubro devuelve
// exactamente una línea, idéntica a la de antes.
func (s *Service) lineasDeInventario(empresaID string, montos montoPorProducto, alDebe bool) []contabilidad.Linea {
	porCuenta := map[string]float64{}
	for prodID, monto := range montos {
		porCuenta[s.cuentaInventarioDe(empresaID, prodID)] += monto
	}
	codigos := make([]string, 0, len(porCuenta))
	for c := range porCuenta {
		codigos = append(codigos, c)
	}
	// Orden fijo: dos asientos con las mismas cuentas tienen que salir iguales, y el
	// recorrido de un mapa no garantiza ningún orden.
	sort.Strings(codigos)

	out := make([]contabilidad.Linea, 0, len(codigos))
	for _, c := range codigos {
		monto := round2(porCuenta[c])
		if monto <= 0.004 {
			continue
		}
		if alDebe {
			out = append(out, contabilidad.Linea{Codigo: c, Debe: monto})
		} else {
			out = append(out, contabilidad.Linea{Codigo: c, Haber: monto})
		}
	}
	return out
}
