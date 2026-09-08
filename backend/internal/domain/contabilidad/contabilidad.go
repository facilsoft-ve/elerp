// Package contabilidad modela el plan de cuentas y el libro diario.
//
// Dos decisiones de arquitectura que este paquete hace cumplir (principios 2 y 4
// del CLAUDE.md), y que son la razón de que exista así y no como un módulo de
// captura manual:
//
//  1. **Los asientos son DERIVADOS de las operaciones**, no se teclean. Una
//     factura emitida genera su asiento en el mismo caso de uso que la emite; un
//     cobro, el suyo. Así la contabilidad no se «cuadra» a fin de mes contra el
//     inventario o el fiscal: nace cuadrada porque sale del mismo hecho.
//  2. **El libro diario es de SOLO-ANEXADO.** Un asiento no se edita ni se borra:
//     se corrige con su ASIENTO CONTRARIO, que referencia al original. Es lo que
//     permite demostrar ante el SENIAT que no hubo alteración.
//
// El asiento, además, tiene que CUADRAR (suma del debe = suma del haber). Un
// asiento descuadrado no se guarda: no es un dato imperfecto, es un dato falso.
package contabilidad

import "fmt"

// Tipos de cuenta del plan.
const (
	TipoActivo     = "activo"
	TipoPasivo     = "pasivo"
	TipoPatrimonio = "patrimonio"
	TipoIngreso    = "ingreso"
	TipoCosto      = "costo"
	TipoGasto      = "gasto"
)

// Códigos del plan de cuentas base. Son los del prototipo (pantalla Plan de
// cuentas), más los que exige la operación venezolana: ventas exentas e IGTF por
// pagar, que en una bodega no son un caso raro sino el día a día.
const (
	CtaCajaBancos       = "1101"
	CtaCuentasPorCobrar = "1102"
	// CtaIVACreditoFiscal es el IVA soportado en las compras (activo deudor): el
	// crédito fiscal que se reconoce con la factura del proveedor y se compensa
	// contra el débito fiscal de las ventas en la declaración de IVA.
	CtaIVACreditoFiscal = "1103"
	// CtaRetencionIVAaFavor es el IVA que un cliente agente de retención te retuvo
	// sobre tus ventas (activo deudor): un crédito a favor que se compensa contra
	// el IVA a pagar en la declaración.
	CtaRetencionIVAaFavor = "1104"
	// CtaRetencionISLRaFavor es el ISLR que un cliente agente de retención te retuvo
	// sobre tus ventas (activo deudor): un anticipo de ISLR a favor que se compensa
	// contra el impuesto sobre la renta a pagar en la declaración.
	CtaRetencionISLRaFavor = "1105"
	CtaInventario          = "1201"
	CtaCuentasPorPagar     = "2101"
	CtaIVADebito           = "2201"
	CtaIGTFPorPagar        = "2202"
	// CtaIVARetenidoPorEnterar es el IVA que TÚ le retuviste al proveedor y aún no
	// has enterado al SENIAT (pasivo acreedor): dinero que no es tuyo, lo debes.
	CtaIVARetenidoPorEnterar = "2203"
	// CtaISLRRetenidoPorEnterar es el ISLR que TÚ le retuviste al proveedor y aún no
	// has enterado al SENIAT (pasivo acreedor): dinero que no es tuyo, lo debes.
	CtaISLRRetenidoPorEnterar = "2204"
	CtaCapital                = "3101"
	// CtaResultadosAcumulados recibe la utilidad o pérdida del ejercicio al CERRAR un
	// período: el asiento de cierre salda las cuentas nominales (ingresos, costos,
	// gastos) contra esta cuenta de patrimonio.
	CtaResultadosAcumulados = "3102"
	CtaVentas               = "4101"
	CtaVentasExentas        = "4102"
	CtaCostoDeVentas        = "5101"
	CtaGastosOperativos     = "5201"
	// CtaDiferenciaEnCompras recoge la diferencia entre la base facturada por el
	// proveedor y el costo NETO efectivamente recibido (que ya entró al Kardex y
	// asentó CxP en la recepción). Al Debe cuando la factura supera lo recibido (un
	// mayor costo asumido), al Haber cuando es menor (una recuperación). El
	// inventario NO se re-valúa: el Kardex mantiene el costo recibido y esta cuenta
	// absorbe la variación, dejándola explícita y auditable en resultados.
	CtaDiferenciaEnCompras = "5202"
)

// Cuenta es una cuenta del plan.
type Cuenta struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"` // tenant
	Codigo    string `json:"codigo" bson:"codigo"`
	Nombre    string `json:"nombre" bson:"nombre"`
	Tipo      string `json:"tipo" bson:"tipo"`
	// Deudora indica la naturaleza del saldo: las cuentas deudoras (activo, costo,
	// gasto) aumentan por el debe; las acreedoras, por el haber. Sin esto el
	// balance de comprobación no puede presentar saldos con signo correcto.
	Deudora bool `json:"deudora" bson:"deudora"`
	// Desactivada oculta la cuenta para nuevas selecciones sin borrarla (nunca se
	// elimina una cuenta: los asientos ya emitidos la referencian por código). Valor
	// cero = activa, así las cuentas ya sembradas no necesitan migración.
	Desactivada bool `json:"desactivada" bson:"desactivada"`
	// CodigoPadre enlaza una SUBCUENTA con su cuenta madre (jerarquía por código).
	// Vacío = cuenta de primer nivel. Una subcuenta hereda el TIPO del padre, y solo
	// las cuentas HOJA (sin subcuentas) reciben asientos; el padre agrega sus saldos.
	CodigoPadre string `json:"codigoPadre" bson:"codigopadre"`
}

// Naturaleza deduce si una cuenta es deudora a partir de su tipo.
func Naturaleza(tipo string) bool {
	switch tipo {
	case TipoActivo, TipoCosto, TipoGasto:
		return true
	}
	return false
}

// Linea es un renglón del asiento: una cuenta con su monto al debe o al haber.
type Linea struct {
	Codigo string  `json:"codigo" bson:"codigo"`
	Nombre string  `json:"nombre" bson:"nombre"`
	Debe   float64 `json:"debe" bson:"debe"`
	Haber  float64 `json:"haber" bson:"haber"`
}

// Asiento es una entrada inmutable del libro diario.
type Asiento struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	// Numero es correlativo por empresa (AS-000001…): un libro diario con saltos
	// sin explicación es un libro que no se puede defender.
	Numero      int    `json:"numero" bson:"numero"`
	Codigo      string `json:"codigo" bson:"codigo"` // AS-000001
	Fecha       string `json:"fecha" bson:"fecha"`   // UTC RFC3339
	Descripcion string `json:"descripcion" bson:"descripcion"`
	// RefTipo y RefID amarran el asiento al hecho que lo originó (documento
	// fiscal, cobro, movimiento). Es lo que permite ir del asiento a la operación
	// y demostrar que no se inventó.
	RefTipo string  `json:"refTipo" bson:"reftipo"`
	RefID   string  `json:"refId" bson:"refid"`
	Lineas  []Linea `json:"lineas" bson:"lineas"`
	Total   float64 `json:"total" bson:"total"` // suma del debe (= suma del haber)
	// Contrario marca el asiento que revierte a otro (RefAsientoID).
	Contrario    bool   `json:"contrario" bson:"contrario"`
	RefAsientoID string `json:"refAsientoId" bson:"refasientoid"`
	Actor        string `json:"actor" bson:"actor"`
	// Sello de integridad (encadenamiento por hash). PrevHash es el Hash del asiento
	// anterior de la empresa; Hash sella este asiento. Vacío = aún no sellado
	// (registros anteriores a la activación del sello). Lo asigna el repositorio al
	// anexar. No entran en Contenido() (son el sello, no el dato sellado).
	PrevHash string `json:"prevHash" bson:"prevhash"`
	Hash     string `json:"hash" bson:"hash"`
}

// Contenido devuelve la representación canónica e inmutable del asiento para el
// sello de integridad (excluye PrevHash/Hash). Cualquier cambio en un campo
// material o en las líneas cambia el contenido y, por tanto, el hash.
func (a Asiento) Contenido() string {
	s := fmt.Sprintf("%d|%s|%s|%s|%s|%s|%.2f|%t|%s|%s",
		a.Numero, a.Codigo, a.Fecha, a.Descripcion, a.RefTipo, a.RefID, a.Total, a.Contrario, a.RefAsientoID, a.Actor)
	for _, l := range a.Lineas {
		s += fmt.Sprintf("|%s:%.2f:%.2f", l.Codigo, l.Debe, l.Haber)
	}
	return s
}

// Cuadra indica si el asiento está balanceado con tolerancia de medio céntimo.
func (a Asiento) Cuadra() bool {
	debe, haber := 0.0, 0.0
	for _, l := range a.Lineas {
		debe += l.Debe
		haber += l.Haber
	}
	dif := debe - haber
	return dif < 0.005 && dif > -0.005
}

// CuentaRepo es el puerto del plan de cuentas.
type CuentaRepo interface {
	List(empresaID string) []Cuenta
	ByCodigo(empresaID, codigo string) (Cuenta, bool)
	Create(c Cuenta) Cuenta
	// Update reemplaza una cuenta existente (por id, con guardia de empresa). Es un
	// MAESTRO editable (renombrar, activar/desactivar); no confundir con el libro
	// diario, que es solo-anexado.
	Update(c Cuenta) (Cuenta, bool)
}

// AsientoRepo es el puerto del libro diario. SOLO-ANEXADO: no expone Update ni
// Delete a propósito, igual que el ledger de inventario y el fiscal.
type AsientoRepo interface {
	Append(a Asiento) Asiento
	List(empresaID string) []Asiento
	ByID(empresaID, id string) (Asiento, bool)
	// PorRef devuelve los asientos originados por un hecho concreto.
	PorRef(empresaID, refTipo, refID string) []Asiento
	// SiguienteNumero da el correlativo del libro por empresa.
	SiguienteNumero(empresaID string) int
}
