package application

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/inventario"
	"github.com/mornix/elerp/internal/domain/tesoreria"
)

// Errores de negocio de Contabilidad.
var (
	ErrAsientoDescuadrado = errors.New("el asiento no cuadra: la suma del debe tiene que ser igual a la del haber")
	ErrAsientoNoExiste    = errors.New("ese asiento no existe")
	ErrAsientoYaRevertido = errors.New("ese asiento ya tiene su contrario")
	ErrPeriodoCerrado     = errors.New("el período contable ya está cerrado")
	ErrCierreInvalido     = errors.New("cierre de período inválido")
	// Asiento manual.
	ErrDescripcionAsiento   = errors.New("el asiento necesita una descripción")
	ErrAsientoMinLineas     = errors.New("un asiento manual necesita al menos dos líneas")
	ErrLineaAsientoInvalida = errors.New("cada línea lleva un monto en el debe O en el haber (no ambos, no negativo)")
	ErrCuentaDesactivada    = errors.New("no se puede asentar en una cuenta desactivada")
	// Subcuentas (jerarquía).
	ErrCuentaPadreNoExiste = errors.New("la cuenta padre no existe")
	ErrCuentaPadreInvalida = errors.New("una cuenta no puede ser su propia madre")
	ErrCuentaConHijas      = errors.New("no se puede asentar en una cuenta con subcuentas: usa una de sus subcuentas")
)

// tieneHijas indica si una cuenta tiene subcuentas (es decir, NO es hoja).
func (s *Service) tieneHijas(empresaID, codigo string) bool {
	for _, c := range s.cuentas.List(empresaID) {
		if c.CodigoPadre == codigo {
			return true
		}
	}
	return false
}

// PlanBase es el plan de cuentas mínimo con el que arranca una empresa. Son los
// códigos del prototipo más los que exige la operación venezolana (ventas
// exentas e IGTF). Un plan por giro se construye encima de este.
func PlanBase() []contabilidad.Cuenta {
	def := []struct {
		codigo, nombre, tipo string
	}{
		{contabilidad.CtaCajaBancos, "Caja y bancos", contabilidad.TipoActivo},
		{contabilidad.CtaCuentasPorCobrar, "Cuentas por cobrar", contabilidad.TipoActivo},
		{contabilidad.CtaIVACreditoFiscal, "IVA crédito fiscal", contabilidad.TipoActivo},
		{contabilidad.CtaRetencionIVAaFavor, "Retenciones de IVA a favor", contabilidad.TipoActivo},
		{contabilidad.CtaRetencionISLRaFavor, "Retenciones de ISLR a favor / anticipo", contabilidad.TipoActivo},
		{contabilidad.CtaInventario, "Inventario de mercancía", contabilidad.TipoActivo},
		{contabilidad.CtaCuentasPorPagar, "Cuentas por pagar", contabilidad.TipoPasivo},
		{contabilidad.CtaIVADebito, "IVA débito fiscal", contabilidad.TipoPasivo},
		{contabilidad.CtaIGTFPorPagar, "IGTF por pagar", contabilidad.TipoPasivo},
		{contabilidad.CtaIVARetenidoPorEnterar, "IVA retenido por enterar", contabilidad.TipoPasivo},
		{contabilidad.CtaISLRRetenidoPorEnterar, "ISLR retenido por enterar", contabilidad.TipoPasivo},
		{contabilidad.CtaCapital, "Capital social", contabilidad.TipoPatrimonio},
		{contabilidad.CtaResultadosAcumulados, "Resultados acumulados", contabilidad.TipoPatrimonio},
		{contabilidad.CtaVentas, "Ventas", contabilidad.TipoIngreso},
		{contabilidad.CtaVentasExentas, "Ventas exentas de IVA", contabilidad.TipoIngreso},
		{contabilidad.CtaCostoDeVentas, "Costo de ventas", contabilidad.TipoCosto},
		{contabilidad.CtaGastosOperativos, "Gastos operativos", contabilidad.TipoGasto},
		{contabilidad.CtaDiferenciaEnCompras, "Diferencia en compras", contabilidad.TipoGasto},
	}
	out := make([]contabilidad.Cuenta, 0, len(def))
	for _, d := range def {
		out = append(out, contabilidad.Cuenta{
			Codigo: d.codigo, Nombre: d.nombre, Tipo: d.tipo,
			Deudora: contabilidad.Naturaleza(d.tipo),
		})
	}
	return out
}

// PlanPorGiro devuelve las cuentas SUGERIDAS extra según el giro del negocio (se
// suman al PlanBase). A diferencia de las base, estas NO las usa el motor de
// asientos: son plantilla, así que el usuario puede renombrarlas o desactivarlas.
// Un giro desconocido devuelve solo las comunes a todo comercio.
func PlanPorGiro(giro string) []contabilidad.Cuenta {
	c := func(codigo, nombre, tipo string) contabilidad.Cuenta {
		return contabilidad.Cuenta{Codigo: codigo, Nombre: nombre, Tipo: tipo, Deudora: contabilidad.Naturaleza(tipo)}
	}
	// Comunes a cualquier comercio.
	comun := []contabilidad.Cuenta{
		c("4103", "Descuentos y devoluciones en ventas", contabilidad.TipoIngreso),
		c("5203", "Mermas y faltantes de inventario", contabilidad.TipoGasto),
		c("5204", "Servicios básicos (luz, agua, aseo)", contabilidad.TipoGasto),
		c("5205", "Alquiler del local", contabilidad.TipoGasto),
		c("5206", "Sueldos y salarios", contabilidad.TipoGasto),
	}
	var esp []contabilidad.Cuenta
	switch giro {
	case "bodega", "abasto", "supermercado":
		esp = []contabilidad.Cuenta{
			c("4104", "Ventas de víveres", contabilidad.TipoIngreso),
			c("5207", "Fletes y transporte de mercancía", contabilidad.TipoGasto),
		}
	case "farmacia":
		esp = []contabilidad.Cuenta{
			c("4104", "Ventas de medicamentos", contabilidad.TipoIngreso),
			c("4105", "Ventas de misceláneos", contabilidad.TipoIngreso),
			c("5207", "Bajas por vencimiento", contabilidad.TipoGasto),
		}
	case "ferreteria":
		esp = []contabilidad.Cuenta{
			c("4104", "Ventas de materiales", contabilidad.TipoIngreso),
			c("4106", "Servicios (corte, mezcla, alquiler)", contabilidad.TipoIngreso),
			c("5207", "Fletes de mercancía", contabilidad.TipoGasto),
		}
	case "servicios":
		esp = []contabilidad.Cuenta{
			c("4107", "Ingresos por servicios", contabilidad.TipoIngreso),
			c("5208", "Honorarios profesionales", contabilidad.TipoGasto),
			c("5209", "Materiales y suministros", contabilidad.TipoGasto),
		}
	}
	return append(comun, esp...)
}

// planEsperado es el plan que DEBERÍA existir para una empresa: el base más la
// plantilla de su giro.
func (s *Service) planEsperado(empresaID string) []contabilidad.Cuenta {
	esperado := PlanBase()
	if s.empresas != nil {
		if emp, ok := s.empresas.ByID(empresaID); ok {
			esperado = append(esperado, PlanPorGiro(emp.Giro)...)
		}
	}
	return esperado
}

// PlanDeCuentas devuelve el plan de la empresa, sembrándolo si falta: base +
// plantilla por giro. Una empresa sin plan no puede registrar nada, y no tiene
// sentido pedirle a la dueña que lo cree a mano para empezar.
func (s *Service) PlanDeCuentas(empresaID string) []contabilidad.Cuenta {
	if s.cuentas == nil {
		return []contabilidad.Cuenta{}
	}
	actual := s.cuentas.List(empresaID)
	esperado := s.planEsperado(empresaID)
	if len(actual) >= len(esperado) {
		return actual
	}
	// Se siembra cuenta por cuenta comprobando el CÓDIGO (idempotente): con el guard
	// anterior («si la lista está vacía»), dos llamadas casi simultáneas —el
	// recontabilizado del arranque y la primera petición— sembraban el plan dos
	// veces y el balance mostraba cada cuenta repetida. Una cuenta propia del
	// usuario con el mismo código que una de plantilla tiene prioridad (no se pisa).
	for _, cta := range esperado {
		if _, existe := s.cuentas.ByCodigo(empresaID, cta.Codigo); existe {
			continue
		}
		cta.EmpresaID = empresaID
		s.cuentas.Create(cta)
	}
	return s.cuentas.List(empresaID)
}

// --- Edición del plan de cuentas (maestro editable) ---
//
// El plan es un MAESTRO editable (a diferencia del libro diario, solo-anexado).
// Reglas que protegen la integridad: el CÓDIGO es inmutable (los asientos lo
// referencian); nunca se BORRA una cuenta, se DESACTIVA; renombrar es seguro
// porque cada asiento ya guardó el nombre vigente al emitirse; y las cuentas BASE
// (las del PlanBase que usa el motor de asientos derivados) no se desactivan.

var (
	ErrCuentaExiste       = errors.New("ya existe una cuenta con ese código")
	ErrCuentaNoExiste     = errors.New("la cuenta no existe")
	ErrCuentaBase         = errors.New("es una cuenta base del sistema: no se puede desactivar")
	ErrTipoCuentaInvalido = errors.New("tipo de cuenta inválido (activo|pasivo|patrimonio|ingreso|costo|gasto)")
	ErrDatosCuenta        = errors.New("el código y el nombre son obligatorios")
)

func tipoCuentaValido(t string) bool {
	switch t {
	case contabilidad.TipoActivo, contabilidad.TipoPasivo, contabilidad.TipoPatrimonio,
		contabilidad.TipoIngreso, contabilidad.TipoCosto, contabilidad.TipoGasto:
		return true
	}
	return false
}

// EsCuentaBase indica si un código pertenece al plan base del sistema (las cuentas
// que el motor usa por código para los asientos derivados).
func EsCuentaBase(codigo string) bool {
	for _, c := range PlanBase() {
		if c.Codigo == codigo {
			return true
		}
	}
	return false
}

// CrearCuenta agrega una cuenta al plan. Código único y tipo válido; la naturaleza
// (deudora/acreedora) se deriva del tipo.
func (s *Service) CrearCuenta(empresaID, codigo, nombre, tipo, codigoPadre, actor, origen string) (contabilidad.Cuenta, error) {
	codigo, nombre, codigoPadre = strings.TrimSpace(codigo), strings.TrimSpace(nombre), strings.TrimSpace(codigoPadre)
	if codigo == "" || nombre == "" {
		return contabilidad.Cuenta{}, ErrDatosCuenta
	}
	s.PlanDeCuentas(empresaID) // asegura el plan base sembrado antes de comparar el código
	if _, existe := s.cuentas.ByCodigo(empresaID, codigo); existe {
		return contabilidad.Cuenta{}, ErrCuentaExiste
	}
	// Subcuenta: hereda el tipo del padre (una subcuenta de un gasto es un gasto).
	if codigoPadre != "" {
		if codigoPadre == codigo {
			return contabilidad.Cuenta{}, ErrCuentaPadreInvalida
		}
		padre, ok := s.cuentas.ByCodigo(empresaID, codigoPadre)
		if !ok {
			return contabilidad.Cuenta{}, ErrCuentaPadreNoExiste
		}
		tipo = padre.Tipo
	} else if !tipoCuentaValido(tipo) {
		return contabilidad.Cuenta{}, ErrTipoCuentaInvalido
	}
	c := s.cuentas.Create(contabilidad.Cuenta{
		EmpresaID: empresaID, Codigo: codigo, Nombre: nombre, Tipo: tipo,
		Deudora: contabilidad.Naturaleza(tipo), CodigoPadre: codigoPadre,
	})
	s.audit.Append(evento(empresaID, actor, origen, "contabilidad.cuenta.crear", codigo, nombre))
	return c, nil
}

// RenombrarCuenta cambia solo el NOMBRE (el código es inmutable). Seguro: los
// asientos ya emitidos conservan el nombre que tenían.
func (s *Service) RenombrarCuenta(empresaID, codigo, nombre, actor, origen string) (contabilidad.Cuenta, error) {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return contabilidad.Cuenta{}, ErrDatosCuenta
	}
	s.PlanDeCuentas(empresaID) // asegura el plan base sembrado (una empresa nueva aún no lo materializó)
	c, ok := s.cuentas.ByCodigo(empresaID, codigo)
	if !ok {
		return contabilidad.Cuenta{}, ErrCuentaNoExiste
	}
	c.Nombre = nombre
	out, ok := s.cuentas.Update(c)
	if !ok {
		return contabilidad.Cuenta{}, ErrCuentaNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "contabilidad.cuenta.renombrar", codigo, nombre))
	return out, nil
}

// FijarCuentaActiva activa/desactiva una cuenta. Las cuentas BASE no se pueden
// desactivar (el motor de asientos las necesita).
func (s *Service) FijarCuentaActiva(empresaID, codigo string, activa bool, actor, origen string) (contabilidad.Cuenta, error) {
	if !activa && EsCuentaBase(codigo) {
		return contabilidad.Cuenta{}, ErrCuentaBase
	}
	s.PlanDeCuentas(empresaID) // asegura el plan base sembrado
	c, ok := s.cuentas.ByCodigo(empresaID, codigo)
	if !ok {
		return contabilidad.Cuenta{}, ErrCuentaNoExiste
	}
	c.Desactivada = !activa
	out, ok := s.cuentas.Update(c)
	if !ok {
		return contabilidad.Cuenta{}, ErrCuentaNoExiste
	}
	accion := "contabilidad.cuenta.reactivar"
	if !activa {
		accion = "contabilidad.cuenta.desactivar"
	}
	s.audit.Append(evento(empresaID, actor, origen, accion, codigo, c.Nombre))
	return out, nil
}

// --- Asiento manual ---
//
// Único punto de captura MANUAL en el libro (el resto se deriva de operaciones).
// Se somete a las MISMAS reglas append-only: tiene que CUADRAR (lo verifica
// asentar, que rechaza el descuadrado), respeta el período cerrado, y se corrige
// con su contrario (RevertirAsiento), nunca se edita. Habilita, además, usar las
// subcuentas/plantillas del plan para registros que el motor no deriva (ajustes,
// depreciación, provisiones, reclasificaciones).

// LineaAsientoManual es un renglón capturado a mano: una cuenta con su monto en el
// debe O en el haber.
type LineaAsientoManual struct {
	Codigo string
	Debe   float64
	Haber  float64
}

// RegistrarAsientoManual valida y anexa un asiento tecleado a mano. Rechaza:
// descripción vacía, menos de dos líneas, líneas con ambos/ningún lado o negativas,
// cuentas inexistentes o desactivadas, y —vía asentar— el descuadre y el período
// cerrado.
func (s *Service) RegistrarAsientoManual(empresaID, actor, origen, fecha, descripcion string, lineas []LineaAsientoManual) (contabilidad.Asiento, error) {
	descripcion = strings.TrimSpace(descripcion)
	if descripcion == "" {
		return contabilidad.Asiento{}, ErrDescripcionAsiento
	}
	// Índice del plan: valida que cada código exista y esté activo.
	activa := map[string]bool{}
	for _, c := range s.PlanDeCuentas(empresaID) {
		activa[c.Codigo] = !c.Desactivada
	}
	lin := make([]contabilidad.Linea, 0, len(lineas))
	for _, l := range lineas {
		if l.Debe < 0 || l.Haber < 0 || (l.Debe > 0.004) == (l.Haber > 0.004) {
			return contabilidad.Asiento{}, ErrLineaAsientoInvalida
		}
		act, existe := activa[l.Codigo]
		if !existe {
			return contabilidad.Asiento{}, fmt.Errorf("%w: %s", ErrCuentaNoExiste, l.Codigo)
		}
		if !act {
			return contabilidad.Asiento{}, fmt.Errorf("%w: %s", ErrCuentaDesactivada, l.Codigo)
		}
		if s.tieneHijas(empresaID, l.Codigo) {
			return contabilidad.Asiento{}, fmt.Errorf("%w: %s", ErrCuentaConHijas, l.Codigo)
		}
		lin = append(lin, contabilidad.Linea{Codigo: l.Codigo, Debe: round2(l.Debe), Haber: round2(l.Haber)})
	}
	if len(lin) < 2 {
		return contabilidad.Asiento{}, ErrAsientoMinLineas
	}
	// Fecha solo-día → mediodía UTC, para ordenar junto a los asientos derivados
	// (RFC3339) sin quedar siempre al inicio del día.
	if len(fecha) == 10 {
		fecha += "T12:00:00Z"
	}
	a, err := s.asentar(empresaID, actor, fecha, descripcion, "manual", "", lin)
	if err != nil {
		return contabilidad.Asiento{}, err
	}
	s.audit.Append(evento(empresaID, actor, origen, "contabilidad.asiento.manual", a.Codigo, descripcion))
	return a, nil
}

// nombreDeCuenta resuelve el nombre para escribirlo en la línea del asiento: el
// asiento se guarda autocontenido, para que leerlo no dependa de que el plan siga
// igual dentro de cinco años.
func (s *Service) nombreDeCuenta(empresaID, codigo string) string {
	for _, c := range s.PlanDeCuentas(empresaID) {
		if c.Codigo == codigo {
			return c.Nombre
		}
	}
	return codigo
}

// LibroDiario devuelve los asientos de la empresa, del más reciente al más viejo.
func (s *Service) LibroDiario(empresaID string) []contabilidad.Asiento {
	if s.asientos == nil {
		return []contabilidad.Asiento{}
	}
	out := s.asientos.List(empresaID)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Numero > out[j-1].Numero; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// asentar valida y anexa un asiento. Es el único camino al libro diario: si no
// cuadra, no entra.
// `fecha` es la fecha del HECHO que origina el asiento (YYYY-MM-DD o RFC3339); vacío
// ⇒ hoy. Un libro diario SENIAT se fecha al hecho, no al registro.
func (s *Service) asentar(empresaID, actor, fecha, descripcion, refTipo, refID string, lineas []contabilidad.Linea) (contabilidad.Asiento, error) {
	return s.asentarContrarioDe(empresaID, actor, fecha, descripcion, refTipo, refID, "", lineas)
}

// asentarContrarioDe es el asentado completo: `contrarioDe` amarra el asiento al
// que revierte, y queda dentro del propio registro (no se edita el original).
func (s *Service) asentarContrarioDe(empresaID, actor, fecha, descripcion, refTipo, refID, contrarioDe string, lineas []contabilidad.Linea) (contabilidad.Asiento, error) {
	if s.asientos == nil {
		return contabilidad.Asiento{}, nil // sin contabilidad cableada, no se asienta
	}
	// Se descartan las líneas en cero: un asiento no lleva renglones vacíos.
	limpias := make([]contabilidad.Linea, 0, len(lineas))
	total := 0.0
	for _, l := range lineas {
		if l.Debe <= 0.004 && l.Haber <= 0.004 {
			continue
		}
		l.Nombre = s.nombreDeCuenta(empresaID, l.Codigo)
		limpias = append(limpias, l)
		total += l.Debe
	}
	if len(limpias) == 0 {
		return contabilidad.Asiento{}, nil
	}
	if fecha == "" {
		fecha = ahora()
	}
	numero := s.asientos.SiguienteNumero(empresaID)
	a := contabilidad.Asiento{
		EmpresaID: empresaID, Numero: numero, Codigo: fmt.Sprintf("AS-%06d", numero),
		Fecha: fecha, Descripcion: descripcion, RefTipo: refTipo, RefID: refID,
		Lineas: limpias, Total: round2(total), Actor: actor,
		Contrario: contrarioDe != "", RefAsientoID: contrarioDe,
	}
	if !a.Cuadra() {
		// No se guarda un asiento descuadrado: no es un dato imperfecto, es falso.
		// Los llamadores derivados ignoran el error (la operación no se aborta), así
		// que se DEJA TRAZA para no perderlo en silencio (contexto: auditoría M3).
		log.Printf("contabilidad: asiento NO registrado (descuadrado) empresa=%s ref=%s/%s desc=%q total=%.2f",
			empresaID, refTipo, refID, descripcion, round2(total))
		return contabilidad.Asiento{}, ErrAsientoDescuadrado
	}
	// Barrera de período cerrado (§7.3): si la fecha del asiento cae dentro (o
	// antes) de un mes ya cerrado, no se anexa. La comparación es de strings
	// RFC3339 UTC, que ordena cronológicamente. Como los asientos derivados de hoy
	// llevan Fecha=ahora() y solo se cierran meses estrictamente anteriores al
	// actual, esto nunca bloquea la operación corriente; protege asientos manuales
	// o reversas con fecha antigua.
	if s.periodos != nil {
		for _, p := range s.periodos.List(empresaID) {
			if a.Fecha <= p.FechaCierre {
				log.Printf("contabilidad: asiento NO registrado (período cerrado %s) empresa=%s ref=%s/%s desc=%q",
					p.FechaCierre, empresaID, refTipo, refID, descripcion)
				return contabilidad.Asiento{}, ErrPeriodoCerrado
			}
		}
	}
	return s.asientos.Append(a), nil
}

/* --- Asientos derivados de las operaciones ---------------------------------
 *
 * Se llaman desde el caso de uso que produce el hecho (emitir, anular, cobrar).
 * Por eso la contabilidad no se reconcilia contra el fiscal: sale de él.
 */

// asentarVenta registra la factura y el costo de lo vendido.
func (s *Service) asentarVenta(empresaID, actor string, doc fiscal.Documento, costo float64) {
	if s.asientos == nil {
		return
	}
	// Al debe entra lo que la empresa recibe: caja/bancos por lo cobrado y
	// cuentas por cobrar por el saldo. El vuelto no se asienta: nunca fue ingreso.
	cobrado := doc.Cobrado
	if cobrado > doc.Total {
		cobrado = doc.Total
	}
	porCobrar := round2(doc.Total - cobrado)
	lineas := []contabilidad.Linea{
		{Codigo: contabilidad.CtaCajaBancos, Debe: round2(cobrado)},
		{Codigo: contabilidad.CtaCuentasPorCobrar, Debe: porCobrar},
		// Al haber, el ingreso separado por condición de IVA y los impuestos que la
		// empresa solo retiene: no son suyos, los debe.
		{Codigo: contabilidad.CtaVentas, Haber: doc.BaseImponible},
		{Codigo: contabilidad.CtaVentasExentas, Haber: doc.BaseExenta},
		{Codigo: contabilidad.CtaIVADebito, Haber: doc.IVA},
		{Codigo: contabilidad.CtaIGTFPorPagar, Haber: doc.IGTF},
	}
	s.asentar(empresaID, actor, doc.Fecha, "Factura "+doc.NumeroCompleto+" emitida", "documento", doc.ID, lineas)

	// Costo de lo vendido: sale del inventario al costo promedio del ledger, no de
	// un porcentaje estimado.
	if costo > 0.004 {
		s.asentar(empresaID, actor, doc.Fecha, "Costo de ventas de "+doc.NumeroCompleto, "documento", doc.ID, []contabilidad.Linea{
			{Codigo: contabilidad.CtaCostoDeVentas, Debe: round2(costo)},
			{Codigo: contabilidad.CtaInventario, Haber: round2(costo)},
		})
	}
}

// asentarNotaDebito registra el asiento de una nota de débito de cliente: un
// cargo adicional que INCREMENTA lo que el cliente debe. Debe Cuentas por cobrar
// (1102) / Haber ingresos (4101/4102) + IVA débito (2201). Es el mismo patrón
// que asentarVenta a crédito (sin cobrar), pero sobre un concepto nuevo: no mueve
// inventario (es un ajuste de valor, no mercancía) ni IGTF. Cuadra por
// construcción: el total al debe iguala base + IVA al haber.
func (s *Service) asentarNotaDebito(empresaID, actor string, doc fiscal.Documento) {
	if s.asientos == nil {
		return
	}
	s.asentar(empresaID, actor, doc.Fecha, "Nota de débito "+doc.NumeroCompleto+" emitida", "documento", doc.ID, []contabilidad.Linea{
		{Codigo: contabilidad.CtaCuentasPorCobrar, Debe: round2(doc.Total)},
		{Codigo: contabilidad.CtaVentas, Haber: doc.BaseImponible},
		{Codigo: contabilidad.CtaVentasExentas, Haber: doc.BaseExenta},
		{Codigo: contabilidad.CtaIVADebito, Haber: doc.IVA},
	})
}

// asentarReversaFiscal registra el asiento contrario de una anulación o nota de
// crédito. El asiento original queda intacto.
func (s *Service) asentarReversaFiscal(empresaID, actor string, rev fiscal.Documento, orig fiscal.Documento, costo float64) {
	if s.asientos == nil {
		return
	}
	cobrado := orig.Cobrado
	if cobrado > orig.Total {
		cobrado = orig.Total
	}
	porCobrar := round2(orig.Total - cobrado)
	// Mismas cuentas, invertidas: lo que entró sale.
	s.asentar(empresaID, actor, rev.Fecha, "Reversa de "+orig.NumeroCompleto+" ("+rev.NumeroCompleto+")", "documento", rev.ID, []contabilidad.Linea{
		{Codigo: contabilidad.CtaVentas, Debe: orig.BaseImponible},
		{Codigo: contabilidad.CtaVentasExentas, Debe: orig.BaseExenta},
		{Codigo: contabilidad.CtaIVADebito, Debe: orig.IVA},
		{Codigo: contabilidad.CtaIGTFPorPagar, Debe: orig.IGTF},
		{Codigo: contabilidad.CtaCajaBancos, Haber: round2(cobrado)},
		{Codigo: contabilidad.CtaCuentasPorCobrar, Haber: porCobrar},
	})
	if costo > 0.004 {
		s.asentar(empresaID, actor, rev.Fecha, "Reingreso de inventario por "+rev.NumeroCompleto, "documento", rev.ID, []contabilidad.Linea{
			{Codigo: contabilidad.CtaInventario, Debe: round2(costo)},
			{Codigo: contabilidad.CtaCostoDeVentas, Haber: round2(costo)},
		})
	}
}

// asentarCobro registra el cobro de una venta a crédito: el dinero entra y la
// cuenta por cobrar baja. Un reverso de cobro asienta lo contrario.
func (s *Service) asentarCobro(empresaID, actor string, c tesoreria.Cobro) {
	if s.asientos == nil {
		return
	}
	// El cliente entrega el abono (baja CxC) MÁS el IGTF si pagó en divisa (pasivo a
	// enterar). El dinero que entra a caja/banco es la suma. Las líneas en cero se
	// descartan en asentar, así que sin IGTF el asiento queda como antes.
	entra := round2(c.MontoBs + c.IGTF)
	desc := "Cobro de " + c.DocumentoNum
	lineas := []contabilidad.Linea{
		{Codigo: contabilidad.CtaCajaBancos, Debe: entra},
		{Codigo: contabilidad.CtaCuentasPorCobrar, Haber: c.MontoBs},
		{Codigo: contabilidad.CtaIGTFPorPagar, Haber: c.IGTF},
	}
	if c.Reverso {
		desc = "Reverso del cobro de " + c.DocumentoNum
		lineas = []contabilidad.Linea{
			{Codigo: contabilidad.CtaCuentasPorCobrar, Debe: c.MontoBs},
			{Codigo: contabilidad.CtaIGTFPorPagar, Debe: c.IGTF},
			{Codigo: contabilidad.CtaCajaBancos, Haber: entra},
		}
	}
	s.asentar(empresaID, actor, c.Fecha, desc, "cobro", c.ID, lineas)
}

// asentarPagoProveedor registra el pago a un proveedor: baja la cuenta por pagar
// y sale el dinero. Es el espejo de asentarCobro del lado del pasivo. Un reverso
// de pago asienta lo contrario.
func (s *Service) asentarPagoProveedor(empresaID, actor string, p tesoreria.PagoProveedor) {
	if s.asientos == nil {
		return
	}
	desc := "Pago a proveedor " + p.ProveedorNombre
	lineas := []contabilidad.Linea{
		{Codigo: contabilidad.CtaCuentasPorPagar, Debe: p.MontoBs},
		{Codigo: contabilidad.CtaCajaBancos, Haber: p.MontoBs},
	}
	if p.Reverso {
		desc = "Reverso del pago a proveedor " + p.ProveedorNombre
		lineas = []contabilidad.Linea{
			{Codigo: contabilidad.CtaCajaBancos, Debe: p.MontoBs},
			{Codigo: contabilidad.CtaCuentasPorPagar, Haber: p.MontoBs},
		}
	}
	s.asentar(empresaID, actor, p.Fecha, desc, "pago_proveedor", p.ID, lineas)
}

// RevertirAsiento genera el asiento CONTRARIO de uno existente. Es la única forma
// de corregir el libro: el original nunca se toca.
func (s *Service) RevertirAsiento(empresaID, actor, origen, id, motivo string) (contabilidad.Asiento, error) {
	if s.asientos == nil {
		return contabilidad.Asiento{}, ErrAsientoNoExiste
	}
	orig, ok := s.asientos.ByID(empresaID, id)
	if !ok {
		return contabilidad.Asiento{}, ErrAsientoNoExiste
	}
	if orig.Contrario {
		return contabilidad.Asiento{}, errors.New("un asiento contrario no se revierte: revierte el original")
	}
	for _, a := range s.asientos.List(empresaID) {
		if a.Contrario && a.RefAsientoID == id {
			return contabilidad.Asiento{}, ErrAsientoYaRevertido
		}
	}
	invertidas := make([]contabilidad.Linea, 0, len(orig.Lineas))
	for _, l := range orig.Lineas {
		invertidas = append(invertidas, contabilidad.Linea{Codigo: l.Codigo, Debe: l.Haber, Haber: l.Debe})
	}
	desc := "Contrario de " + orig.Codigo
	if motivo != "" {
		desc += " — " + motivo
	}
	a, err := s.asentarContrarioDe(empresaID, actor, "", desc, orig.RefTipo, orig.RefID, orig.ID, invertidas)
	if err != nil {
		return contabilidad.Asiento{}, err
	}
	s.audit.Append(evento(empresaID, actor, origen, "contabilidad.asiento.revertir", orig.Codigo, motivo))
	return a, nil
}

/* --- Cierre de período contable (§7.3) -------------------------------------
 *
 * Cerrar un mes lo marca como CERRADO DEFINITIVAMENTE: no se reabre. A partir de
 * ahí ningún asiento con fecha dentro (o antes) de ese mes puede anexarse — lo
 * hace cumplir la barrera en asentarContrarioDe.
 *
 * Regla que preserva la operación: solo se cierra un mes ESTRICTAMENTE anterior
 * al mes en curso. Nunca el mes actual ni uno futuro. Así los asientos derivados
 * de hoy (Fecha=ahora()) jamás caen en un período cerrado. El cierre es en orden
 * ascendente: no se salta hacia atrás de lo ya cerrado.
 */

// PeriodosCerrados devuelve los cierres de la empresa (solo lectura).
func (s *Service) PeriodosCerrados(empresaID string) []contabilidad.Periodo {
	if s.periodos == nil {
		return []contabilidad.Periodo{}
	}
	return s.periodos.List(empresaID)
}

// CerrarPeriodo cierra el mes (anio, mes) de una empresa. Valida que el mes sea
// válido, estrictamente anterior al mes actual, que no esté ya cerrado y que sea
// posterior al último cerrado (orden ascendente, sin reabrir ni retroceder).
// asentarCierrePeriodo genera el asiento de cierre del período: salda las cuentas
// nominales (ingresos, costos, gastos) del rango contra Resultados acumulados
// (3102), llevando la utilidad/pérdida a patrimonio. Se fecha al cierre del período.
// RefTipo "cierre" para que el P&G lo excluya y no muestre el mes cerrado en cero.
func (s *Service) asentarCierrePeriodo(empresaID, actor, desde, hasta, fechaAsiento, etiqueta string) {
	bal := s.balanceEntre(empresaID, desde, hasta, true) // operativo (sin cierres)
	lineas := []contabilidad.Linea{}
	resultado := 0.0 // + utilidad, - pérdida
	for _, c := range bal.Cuentas {
		switch c.Tipo {
		case contabilidad.TipoIngreso:
			if c.Saldo > 0.004 { // saldo acreedor: se salda al Debe
				lineas = append(lineas, contabilidad.Linea{Codigo: c.Codigo, Debe: c.Saldo})
				resultado += c.Saldo
			}
		case contabilidad.TipoCosto, contabilidad.TipoGasto:
			if c.Saldo > 0.004 { // saldo deudor: se salda al Haber
				lineas = append(lineas, contabilidad.Linea{Codigo: c.Codigo, Haber: c.Saldo})
				resultado -= c.Saldo
			}
		}
	}
	if len(lineas) == 0 {
		return // no hubo actividad nominal en el período: nada que cerrar
	}
	if r := round2(resultado); r > 0.004 {
		lineas = append(lineas, contabilidad.Linea{Codigo: contabilidad.CtaResultadosAcumulados, Haber: r})
	} else if r < -0.004 {
		lineas = append(lineas, contabilidad.Linea{Codigo: contabilidad.CtaResultadosAcumulados, Debe: -r})
	}
	s.asentar(empresaID, actor, fechaAsiento, "Cierre de período "+etiqueta, "cierre", etiqueta, lineas)
}

func (s *Service) CerrarPeriodo(empresaID, actor, origen string, anio, mes int) (contabilidad.Periodo, error) {
	if s.periodos == nil {
		return contabilidad.Periodo{}, ErrCierreInvalido
	}
	if mes < 1 || mes > 12 || anio < 2000 {
		return contabilidad.Periodo{}, fmt.Errorf("%w: mes o año fuera de rango", ErrCierreInvalido)
	}

	// (anio, mes) tiene que ser estrictamente anterior al mes en curso (UTC).
	ahora := time.Now().UTC()
	mesActual := int(ahora.Year())*12 + int(ahora.Month()) - 1
	mesPedido := anio*12 + mes - 1
	if mesPedido >= mesActual {
		return contabilidad.Periodo{}, fmt.Errorf("%w: solo se puede cerrar un mes ya terminado, nunca el mes en curso ni uno futuro", ErrCierreInvalido)
	}

	// Ni duplicado ni retroceso: debe ser posterior a todos los ya cerrados.
	for _, p := range s.periodos.List(empresaID) {
		cerrado := p.Anio*12 + p.Mes - 1
		if mesPedido == cerrado {
			return contabilidad.Periodo{}, ErrPeriodoCerrado
		}
		if mesPedido < cerrado {
			return contabilidad.Periodo{}, fmt.Errorf("%w: los cierres van en orden; ya hay un mes posterior cerrado", ErrCierreInvalido)
		}
	}

	// FechaCierre = último instante del mes: el primer día del mes siguiente menos
	// un segundo. time.Date normaliza mes=13 → enero del año siguiente.
	finDeMes := time.Date(anio, time.Month(mes)+1, 1, 0, 0, 0, 0, time.UTC).Add(-time.Second)
	// Asiento de cierre ANTES de registrar el marcador: salda las nominales del mes
	// contra Resultados acumulados, fechado al fin de mes. Debe postearse antes de que
	// exista el período cerrado, o la barrera (fecha <= FechaCierre) lo rechazaría.
	etiqueta := fmt.Sprintf("%04d-%02d", anio, mes)
	s.asentarCierrePeriodo(empresaID, actor,
		fmt.Sprintf("%04d-%02d-01", anio, mes), finDeMes.Format("2006-01-02"),
		finDeMes.Format(time.RFC3339), etiqueta)
	p := contabilidad.Periodo{
		EmpresaID:   empresaID,
		Anio:        anio,
		Mes:         mes,
		FechaCierre: finDeMes.Format(time.RFC3339),
		CerradoPor:  actor,
		CerradoEl:   ahora.Format(time.RFC3339),
	}
	p = s.periodos.Append(p)
	if s.audit != nil {
		s.audit.Append(evento(empresaID, actor, origen, "contabilidad.periodo.cerrar", p.ID, fmt.Sprintf("%04d-%02d", anio, mes)))
	}
	return p, nil
}

/* --- Proyecciones: balance de comprobación y estado de resultados ---------- */

// SaldoCuenta es el acumulado de una cuenta en el libro.
type SaldoCuentaContable struct {
	Codigo  string  `json:"codigo"`
	Nombre  string  `json:"nombre"`
	Tipo    string  `json:"tipo"`
	Deudora bool    `json:"deudora"`
	Debe    float64 `json:"debe"`
	Haber   float64 `json:"haber"`
	// Saldo con el signo de la naturaleza de la cuenta.
	Saldo float64 `json:"saldo"`
}

// BalanceComprobacion es el balance de comprobación: la prueba de que el libro
// cuadra. Si los totales no son iguales, hay un problema y se dice.
type BalanceComprobacion struct {
	Cuentas    []SaldoCuentaContable `json:"cuentas"`
	TotalDebe  float64               `json:"totalDebe"`
	TotalHaber float64               `json:"totalHaber"`
	Cuadra     bool                  `json:"cuadra"`
	Asientos   int                   `json:"asientos"`
}

// Balance proyecta el balance de comprobación (acumulado desde el inicio) del libro
// diario. Es un balance de SALDOS: cumulativo por naturaleza.
func (s *Service) Balance(empresaID string) BalanceComprobacion {
	return s.balanceEntre(empresaID, "", "", false)
}

// balanceEntre es Balance acotado por rango de fechas (YYYY-MM-DD; vacío = sin límite
// por ese lado). `omitirCierre` excluye los asientos de cierre de período (RefTipo
// "cierre"): el Estado de Resultados los omite para mostrar la actividad operativa;
// el balance de comprobación público los incluye (es el balance de saldos completo).
func (s *Service) balanceEntre(empresaID, desde, hasta string, omitirCierre bool) BalanceComprobacion {
	res := BalanceComprobacion{Cuentas: []SaldoCuentaContable{}}
	if s.asientos == nil {
		return res
	}
	porCodigo := map[string]*SaldoCuentaContable{}
	orden := []string{}
	for _, c := range s.PlanDeCuentas(empresaID) {
		porCodigo[c.Codigo] = &SaldoCuentaContable{
			Codigo: c.Codigo, Nombre: c.Nombre, Tipo: c.Tipo, Deudora: c.Deudora,
		}
		orden = append(orden, c.Codigo)
	}
	for _, a := range s.asientos.List(empresaID) {
		if omitirCierre && a.RefTipo == "cierre" {
			continue
		}
		fecha := a.Fecha
		if len(fecha) >= 10 {
			fecha = fecha[:10]
		}
		if (desde != "" && fecha < desde) || (hasta != "" && fecha > hasta) {
			continue
		}
		res.Asientos++
		for _, l := range a.Lineas {
			c, ok := porCodigo[l.Codigo]
			if !ok {
				// Una cuenta que ya no está en el plan igual se presenta: el libro no
				// se puede esconder porque alguien borró la cuenta.
				porCodigo[l.Codigo] = &SaldoCuentaContable{Codigo: l.Codigo, Nombre: l.Nombre, Deudora: true}
				orden = append(orden, l.Codigo)
				c = porCodigo[l.Codigo]
			}
			c.Debe += l.Debe
			c.Haber += l.Haber
			res.TotalDebe += l.Debe
			res.TotalHaber += l.Haber
		}
	}
	for _, cod := range orden {
		c := porCodigo[cod]
		c.Debe = round2(c.Debe)
		c.Haber = round2(c.Haber)
		if c.Deudora {
			c.Saldo = round2(c.Debe - c.Haber)
		} else {
			c.Saldo = round2(c.Haber - c.Debe)
		}
		res.Cuentas = append(res.Cuentas, *c)
	}
	res.TotalDebe = round2(res.TotalDebe)
	res.TotalHaber = round2(res.TotalHaber)
	dif := res.TotalDebe - res.TotalHaber
	res.Cuadra = dif < 0.005 && dif > -0.005
	return res
}

// EstadoResultados es el P&G del período, derivado del libro.
type EstadoResultados struct {
	Ventas        float64 `json:"ventas"`
	VentasExentas float64 `json:"ventasExentas"`
	IngresoTotal  float64 `json:"ingresoTotal"`
	CostoDeVentas float64 `json:"costoDeVentas"`
	UtilidadBruta float64 `json:"utilidadBruta"`
	Gastos        float64 `json:"gastos"`
	UtilidadNeta  float64 `json:"utilidadNeta"`
	// MargenBruto en porcentaje sobre el ingreso; 0 si no hubo ventas.
	MargenBruto float64 `json:"margenBruto"`
}

// Resultados proyecta el estado de resultados (P&G) del período [desde, hasta]
// (YYYY-MM-DD; vacío = acumulado desde el inicio). Las cuentas nominales (ingresos,
// costo, gastos) se suman solo de los asientos del rango.
func (s *Service) Resultados(empresaID, desde, hasta string) EstadoResultados {
	var r EstadoResultados
	// El P&G refleja la actividad operativa: se excluyen los asientos de cierre (que
	// saldan las nominales contra patrimonio), o un mes cerrado saldría en cero.
	for _, c := range s.balanceEntre(empresaID, desde, hasta, true).Cuentas {
		switch {
		case c.Codigo == contabilidad.CtaVentas:
			r.Ventas = c.Saldo
		case c.Codigo == contabilidad.CtaVentasExentas:
			r.VentasExentas = c.Saldo
		case c.Codigo == contabilidad.CtaCostoDeVentas:
			r.CostoDeVentas = c.Saldo
		case c.Tipo == contabilidad.TipoGasto:
			r.Gastos += c.Saldo
		}
	}
	r.IngresoTotal = round2(r.Ventas + r.VentasExentas)
	r.UtilidadBruta = round2(r.IngresoTotal - r.CostoDeVentas)
	r.Gastos = round2(r.Gastos)
	r.UtilidadNeta = round2(r.UtilidadBruta - r.Gastos)
	if r.IngresoTotal > 0 {
		r.MargenBruto = round2(r.UtilidadBruta / r.IngresoTotal * 100)
	}
	return r
}

/* --- Recontabilizado (backfill) --------------------------------------------
 *
 * Los documentos que existían ANTES de que hubiera contabilidad no tienen
 * asiento: los del seed de demostración, y los de cualquier empresa que ya
 * estuviera operando. Esto los asienta una sola vez.
 *
 * Es idempotente por construcción: si el documento ya tiene asiento, se salta. No
 * puede duplicar el libro por correrse dos veces.
 */

// asentarMovimientoInventario asienta un movimiento del ledger que no viene de un
// documento fiscal: la entrada inicial de mercancía y los ajustes.
//
// Sin esto el inventario del balance sale NEGATIVO: las ventas descargan la cuenta
// 1201 pero nada la había cargado. Las compras a proveedores llegarán con el
// módulo de Compras (y cargarán 1201 contra cuentas por pagar); la entrada inicial
// del demo se asienta contra Capital social, que es lo que económicamente es: un
// aporte de mercancía del dueño.
func (s *Service) asentarMovimientoInventario(empresaID, actor string, m inventario.Movimiento) {
	if s.asientos == nil || m.CostoUnitario <= 0 {
		return
	}
	monto := m.Cantidad * m.CostoUnitario
	switch {
	case m.Tipo == inventario.MovEntrada && monto > 0.004:
		s.asentar(empresaID, actor, m.Fecha, "Entrada de inventario — "+m.Motivo, "movimiento", m.ID, []contabilidad.Linea{
			{Codigo: contabilidad.CtaInventario, Debe: round2(monto)},
			{Codigo: contabilidad.CtaCapital, Haber: round2(monto)},
		})
	case m.Tipo == inventario.MovAjuste && monto < -0.004:
		// Merma: la mercancía que falta es un costo del período.
		s.asentar(empresaID, actor, m.Fecha, "Ajuste de inventario (merma) — "+m.Motivo, "movimiento", m.ID, []contabilidad.Linea{
			{Codigo: contabilidad.CtaCostoDeVentas, Debe: round2(-monto)},
			{Codigo: contabilidad.CtaInventario, Haber: round2(-monto)},
		})
	case m.Tipo == inventario.MovAjuste && monto > 0.004:
		// Sobrante encontrado en un conteo: entra al inventario y rebaja el costo.
		s.asentar(empresaID, actor, m.Fecha, "Ajuste de inventario (sobrante) — "+m.Motivo, "movimiento", m.ID, []contabilidad.Linea{
			{Codigo: contabilidad.CtaInventario, Debe: round2(monto)},
			{Codigo: contabilidad.CtaCostoDeVentas, Haber: round2(monto)},
		})
	}
	// Las transferencias entre sedes NO se asientan: la mercancía sigue siendo de
	// la misma empresa, así que el patrimonio no cambia.
}

// RecontabilizarPendientes asienta los documentos fiscales que aún no tienen
// asiento, del más viejo al más nuevo para que la numeración del libro siga el
// orden de los hechos. Devuelve cuántos asentó.
func (s *Service) RecontabilizarPendientes(empresaID, actor string) int {
	if s.asientos == nil {
		return 0
	}
	docs := s.documentos.List(empresaID)
	// Más viejo primero: un libro numerado al revés de los hechos es ilegible.
	for i := 1; i < len(docs); i++ {
		for j := i; j > 0 && docs[j].Fecha < docs[j-1].Fecha; j-- {
			docs[j], docs[j-1] = docs[j-1], docs[j]
		}
	}
	porID := map[string]fiscal.Documento{}
	for _, d := range docs {
		porID[d.ID] = d
	}

	// Costo real registrado en el ledger para cada documento: se usa el que quedó
	// en los movimientos, no una estimación.
	costoDe := func(docID string) float64 {
		total := 0.0
		for _, m := range s.movimientos.List(empresaID, inventario.FiltroMovimiento{}) {
			if m.RefTipo != "documento" || m.RefID != docID {
				continue
			}
			c := m.Cantidad
			if c < 0 {
				c = -c
			}
			total += c * m.CostoUnitario
		}
		return round2(total)
	}

	n := 0
	// Movimientos de inventario que no vienen de un documento (entrada inicial,
	// ajustes): del más viejo al más nuevo, igual que los documentos.
	movs := s.movimientos.List(empresaID, inventario.FiltroMovimiento{})
	for i := 1; i < len(movs); i++ {
		for j := i; j > 0 && movs[j].Fecha < movs[j-1].Fecha; j-- {
			movs[j], movs[j-1] = movs[j-1], movs[j]
		}
	}
	for _, m := range movs {
		// Se saltan los movimientos que YA tienen su propia vía de asentado en vivo:
		//   · "documento" (ventas/reversas) → asentarVenta/asentarReversaFiscal
		//   · transferencias → no generan asiento (mismo patrimonio, otra sede)
		//   · "compra" (recepción) → asentarCompra (Debe 1201 / Haber 2101) al recibir
		// Su ref NO es ("movimiento", m.ID), así que el guard de idempotencia de abajo
		// no los detecta; sin este salto, cada reinicio los RE-asentaría como entrada
		// (Debe 1201 / Haber 3101), duplicando inventario y patrimonio. Solo se
		// backfillea lo que nace sin asiento propio: entradas iniciales y ajustes.
		if m.RefTipo == "documento" || m.RefTipo == "compra" || m.Tipo == inventario.MovTransferencia {
			continue
		}
		if len(s.asientos.PorRef(empresaID, "movimiento", m.ID)) > 0 {
			continue
		}
		s.asentarMovimientoInventario(empresaID, actor, m)
		n++
	}

	// Notas de crédito/débito de PROVEEDOR que aún no tienen asiento (p. ej. las del
	// seed de demostración): reciben el suyo una sola vez, idempotente por RefID.
	if s.notasCompra != nil {
		for _, nc := range s.notasCompra.List(empresaID) {
			if len(s.asientos.PorRef(empresaID, "nota_compra", nc.ID)) > 0 {
				continue
			}
			s.asentarNotaCompra(empresaID, actor, nc)
			n++
		}
	}

	// Comprobantes de retención (IVA o ISLR) que aún no tienen asiento (p. ej. los
	// del seed): reciben el suyo una sola vez, idempotente por RefID.
	if s.retenciones != nil {
		for _, r := range s.retenciones.List(empresaID) {
			if len(s.asientos.PorRef(empresaID, "retencion_iva", r.ID)) > 0 {
				continue
			}
			s.asentarRetencion(empresaID, actor, r)
			n++
		}
	}

	for _, d := range docs {
		if len(s.asientos.PorRef(empresaID, "documento", d.ID)) > 0 {
			continue // ya está asentado
		}
		switch d.Tipo {
		case fiscal.TipoFactura:
			s.asentarVenta(empresaID, actor, d, costoDe(d.ID))
			n++
		case fiscal.TipoNotaDebito:
			// Cargo adicional: mismo asiento que al emitirla (Debe CxC / Haber
			// ingresos + IVA débito). No mueve inventario.
			s.asentarNotaDebito(empresaID, actor, d)
			n++
		case fiscal.TipoAnulacion, fiscal.TipoNotaCredito:
			orig, ok := porID[d.RefDocumentoID]
			if !ok {
				continue
			}
			// Una nota de crédito parcial revierte solo su parte: se usa el propio
			// documento de la nota como origen de los montos, en negativo.
			base := orig
			if d.Tipo == fiscal.TipoNotaCredito {
				base = fiscal.Documento{
					NumeroCompleto: orig.NumeroCompleto,
					BaseImponible:  -d.BaseImponible, BaseExenta: -d.BaseExenta,
					IVA: -d.IVA, IGTF: -d.IGTF, Total: -d.Total, Cobrado: -d.Total,
				}
			}
			s.asentarReversaFiscal(empresaID, actor, d, base, costoDe(d.ID))
			n++
		}
	}
	return n
}
