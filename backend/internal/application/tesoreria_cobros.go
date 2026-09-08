package application

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/tasa"
	"github.com/mornix/elerp/internal/domain/tesoreria"
)

// Errores de negocio de Tesorería.
var (
	ErrCobroSinSaldo    = errors.New("esa factura ya está cobrada por completo")
	ErrCobroExcedido    = errors.New("el cobro es mayor que el saldo pendiente de la factura")
	ErrCobroInvalido    = errors.New("el monto del cobro debe ser mayor que cero")
	ErrDocSinCredito    = errors.New("esa factura no quedó a crédito: no tiene saldo por cobrar")
	ErrCobroNoExiste    = errors.New("ese cobro no existe")
	ErrCobroYaReversado = errors.New("ese cobro ya fue reversado")
)

/* --- Cuentas por cobrar (proyección) ---------------------------------------
 *
 * El saldo de una factura NO se guarda: se deriva de «total − abonos iniciales −
 * cobros posteriores». Y el estado «vencido» se deriva de la fecha acordada, no
 * de una marca que alguien tenga que acordarse de poner.
 */

// CuentaPorCobrar es una factura a crédito con su saldo derivado.
type CuentaPorCobrar struct {
	DocumentoID      string  `json:"documentoId"`
	NumeroCompleto   string  `json:"numeroCompleto"`
	Fecha            string  `json:"fecha"`
	VenceEl          string  `json:"venceEl"`
	ClienteID        string  `json:"clienteId"`
	ClienteNombre    string  `json:"clienteNombre"`
	ClienteDocumento string  `json:"clienteDocumento"`
	Total            float64 `json:"total"`
	Cobrado          float64 `json:"cobrado"`
	Saldo            float64 `json:"saldo"`
	// DiasVencido > 0 significa que la fecha acordada ya pasó.
	DiasVencido int  `json:"diasVencido"`
	Vencida     bool `json:"vencida"`
}

// ResumenPorCobrar son los totales que alimentan los KPIs del Inicio y de
// Tesorería, para que la interfaz no los recalcule a su manera.
type ResumenPorCobrar struct {
	Cuentas          []CuentaPorCobrar `json:"cuentas"`
	Total            float64           `json:"total"`
	TotalVencido     float64           `json:"totalVencido"`
	ClientesConSaldo int               `json:"clientesConSaldo"`
	CobradoDelMes    float64           `json:"cobradoDelMes"`
}

// CuentasPorCobrar proyecta las facturas a crédito con saldo pendiente.
func (s *Service) CuentasPorCobrar(empresaID string) ResumenPorCobrar {
	res := ResumenPorCobrar{Cuentas: []CuentaPorCobrar{}}
	if s.cobros == nil {
		return res
	}
	// Documentos anulados no se cobran: su reversa los deja sin efecto.
	anulados := map[string]bool{}
	for _, d := range s.documentos.List(empresaID) {
		if d.Tipo == fiscal.TipoAnulacion && d.RefDocumentoID != "" {
			anulados[d.RefDocumentoID] = true
		}
	}
	// Notas de crédito rebajan lo que el cliente debe.
	notas := map[string]float64{}
	for _, d := range s.documentos.List(empresaID) {
		if d.Tipo == fiscal.TipoNotaCredito && d.RefDocumentoID != "" {
			notas[d.RefDocumentoID] += -d.Total // la nota se guarda en negativo
		}
	}

	cobradoPorDoc := map[string]float64{}
	for _, c := range s.cobros.List(empresaID) {
		signo := 1.0
		if c.Reverso {
			signo = -1
		}
		cobradoPorDoc[c.DocumentoID] += signo * c.MontoBs
		if esDelMesActual(c.Fecha) {
			res.CobradoDelMes += signo * c.MontoBs
		}
	}
	// Retenciones RECIBIDAS (IVA o ISLR): el cliente agente de retención paga neto de
	// lo retenido, así que ese impuesto nunca entrará como cobro. Cuenta como aplicado
	// a la factura (baja su saldo por cobrar), igual que un cobro o una nota de crédito.
	retenidoPorDoc := s.retencionesRecibidasPorDoc(empresaID)

	clientes := map[string]bool{}
	hoy := hoyVE()
	for _, d := range s.documentos.List(empresaID) {
		if d.Tipo != fiscal.TipoFactura || !d.Credito || anulados[d.ID] {
			continue
		}
		total := round2(d.Total - notas[d.ID])
		cobrado := round2(d.Cobrado + cobradoPorDoc[d.ID] + retenidoPorDoc[d.ID])
		saldo := round2(total - cobrado)
		if saldo <= 0.004 {
			continue
		}
		cta := CuentaPorCobrar{
			DocumentoID: d.ID, NumeroCompleto: d.NumeroCompleto, Fecha: d.Fecha, VenceEl: d.VenceEl,
			ClienteID: d.ClienteID, ClienteNombre: d.ClienteNombre, ClienteDocumento: d.ClienteDocumento,
			Total: total, Cobrado: cobrado, Saldo: saldo,
		}
		if d.VenceEl != "" && d.VenceEl < hoy {
			cta.Vencida = true
			cta.DiasVencido = diasEntre(d.VenceEl, hoy)
			res.TotalVencido += saldo
		}
		res.Total += saldo
		// Se agrupa por cliente identificado, y si no lo hay (documentos sembrados o
		// importados sin id) por su nombre: contar «1 cliente» porque el id venía
		// vacío daría un KPI falso.
		if d.ClienteID != "" {
			clientes[d.ClienteID] = true
		} else if d.ClienteNombre != "" {
			clientes["nombre:"+d.ClienteNombre] = true
		}
		res.Cuentas = append(res.Cuentas, cta)
	}
	res.Total = round2(res.Total)
	res.TotalVencido = round2(res.TotalVencido)
	res.CobradoDelMes = round2(res.CobradoDelMes)
	res.ClientesConSaldo = len(clientes)
	// Más vencidas primero: es el orden en que hay que llamar a los clientes.
	for i := 1; i < len(res.Cuentas); i++ {
		for j := i; j > 0 && res.Cuentas[j].DiasVencido > res.Cuentas[j-1].DiasVencido; j-- {
			res.Cuentas[j], res.Cuentas[j-1] = res.Cuentas[j-1], res.Cuentas[j]
		}
	}
	return res
}

// SaldoPorCobrar devuelve el saldo pendiente de una factura concreta.
func (s *Service) SaldoPorCobrar(empresaID, documentoID string) (float64, bool) {
	for _, c := range s.CuentasPorCobrar(empresaID).Cuentas {
		if c.DocumentoID == documentoID {
			return c.Saldo, true
		}
	}
	return 0, false
}

/* --- Registrar un cobro ---------------------------------------------------- */

// CobroEntrada son los datos de un cobro recibido.
type CobroEntrada struct {
	DocumentoID string
	Monto       float64
	Moneda      string
	Metodo      string
	CuentaID    string
	Referencia  string
}

// RegistrarCobro anexa un cobro contra una factura a crédito.
//
// Reglas: la factura tiene que existir, ser a crédito y tener saldo; el cobro no
// puede exceder el saldo (cobrar de más no es un cobro, es un anticipo, y ese es
// otro documento); y un cobro en divisas necesita la tasa del servidor para
// expresarse en bolívares.
func (s *Service) RegistrarCobro(empresaID, sedeID, actor, origen string, in CobroEntrada) (tesoreria.Cobro, error) {
	if s.cobros == nil {
		return tesoreria.Cobro{}, errors.New("Tesorería no está configurada")
	}
	if in.Monto <= 0 {
		return tesoreria.Cobro{}, ErrCobroInvalido
	}
	doc, ok := s.documentos.ByID(empresaID, in.DocumentoID)
	if !ok || doc.Tipo != fiscal.TipoFactura {
		return tesoreria.Cobro{}, ErrDocumentoNoExiste
	}
	if !doc.Credito {
		return tesoreria.Cobro{}, ErrDocSinCredito
	}
	saldo, hay := s.SaldoPorCobrar(empresaID, in.DocumentoID)
	if !hay || saldo <= 0.004 {
		return tesoreria.Cobro{}, ErrCobroSinSaldo
	}

	// Moneda y equivalente en bolívares con la tasa del servidor (nunca del cliente).
	moneda := strings.ToUpper(strings.TrimSpace(in.Moneda))
	if moneda == "" {
		moneda = "VES"
	}
	// Conversión con la tasa de LA divisa del cobro (USD, EUR, …), no con la del
	// dólar: antes se usaba s.TasaVigente (alias del USD) para cualquier divisa, así
	// que un cobro en EUR se valorizaba a la tasa del USD (MontoBs y tasa histórica
	// erróneos, Art. 177). Es el mismo criterio por-moneda que usa EmitirFactura.
	montoBs := in.Monto
	tasaValor := 1.0 // VES: no convierte
	tasaFuente := ""
	if moneda != "VES" {
		tp, ok := s.TasaVigenteDeMoneda(empresaID, moneda)
		if !ok {
			if tasa.NormalizarMoneda(moneda) == empresa.MonedaUSD {
				return tesoreria.Cobro{}, ErrSinTasa
			}
			return tesoreria.Cobro{}, fmt.Errorf("%w: %s", ErrSinTasaDivisa, moneda)
		}
		tasaValor = tp.Valor
		tasaFuente = tp.Fuente
		montoBs = round2(in.Monto * tasaValor)
	}
	if montoBs > saldo+0.005 {
		return tesoreria.Cobro{}, fmt.Errorf("%w: el saldo es %.2f Bs", ErrCobroExcedido, saldo)
	}
	// IGTF: un abono pagado en DIVISA causa el impuesto sobre su equivalente en Bs
	// (el abono reduce la CxC; el IGTF es un cargo adicional que se declara al SENIAT).
	// En bolívares no aplica.
	igtf := 0.0
	if moneda != "VES" {
		igtf = round2(montoBs * s.alicuotaIGTF(empresaID))
	}

	c := s.cobros.Append(tesoreria.Cobro{
		EmpresaID: empresaID, SedeID: sedeID,
		DocumentoID: doc.ID, DocumentoNum: doc.NumeroCompleto,
		ClienteID: doc.ClienteID, ClienteNombre: doc.ClienteNombre,
		Monto: round2(in.Monto), Moneda: moneda, MontoBs: montoBs, IGTF: igtf,
		TasaCambio: tasaValor, TasaFuente: tasaFuente,
		Metodo: in.Metodo, CuentaID: in.CuentaID, Referencia: strings.TrimSpace(in.Referencia),
		Actor: actor, Fecha: ahora(),
	})
	// Asiento derivado: entra el dinero y baja la cuenta por cobrar.
	s.asentarCobro(empresaID, actor, c)
	s.audit.Append(evento(empresaID, actor, origen, "tesoreria.cobro", doc.NumeroCompleto,
		fmt.Sprintf("%.2f %s", in.Monto, moneda)))
	return c, nil
}

// ReversarCobro anula un cobro mal registrado anexando su reverso. No se borra
// nada: el histórico tiene que poder explicar el saldo actual.
func (s *Service) ReversarCobro(empresaID, actor, origen, cobroID, motivo string) (tesoreria.Cobro, error) {
	if s.cobros == nil {
		return tesoreria.Cobro{}, ErrCobroNoExiste
	}
	orig, ok := s.cobros.ByID(empresaID, cobroID)
	if !ok || orig.Reverso {
		return tesoreria.Cobro{}, ErrCobroNoExiste
	}
	for _, c := range s.cobros.PorDocumento(empresaID, orig.DocumentoID) {
		if c.Reverso && c.RefCobroID == cobroID {
			return tesoreria.Cobro{}, ErrCobroYaReversado
		}
	}
	if strings.TrimSpace(motivo) == "" {
		return tesoreria.Cobro{}, errors.New("reversar un cobro exige un motivo")
	}
	rev := s.cobros.Append(tesoreria.Cobro{
		EmpresaID: empresaID, SedeID: orig.SedeID,
		DocumentoID: orig.DocumentoID, DocumentoNum: orig.DocumentoNum,
		ClienteID: orig.ClienteID, ClienteNombre: orig.ClienteNombre,
		Monto: orig.Monto, Moneda: orig.Moneda, MontoBs: orig.MontoBs, IGTF: orig.IGTF,
		TasaCambio: orig.TasaCambio, TasaFuente: orig.TasaFuente,
		Metodo: orig.Metodo, CuentaID: orig.CuentaID,
		Reverso: true, RefCobroID: orig.ID, Motivo: strings.TrimSpace(motivo),
		Actor: actor, Fecha: ahora(),
	})
	s.asentarCobro(empresaID, actor, rev)
	s.audit.Append(evento(empresaID, actor, origen, "tesoreria.cobro.reversar", orig.DocumentoNum, motivo))
	return rev, nil
}

// Cobros devuelve el histórico de cobros de la empresa (más reciente primero).
func (s *Service) Cobros(empresaID string) []tesoreria.Cobro {
	if s.cobros == nil {
		return []tesoreria.Cobro{}
	}
	out := s.cobros.List(empresaID)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Fecha > out[j-1].Fecha; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

/* --- Saldos de cuentas (efectivo y bancos) --------------------------------- */

// SaldoCuenta es lo que entró por una cuenta de cobro. Es una proyección de los
// pagos de documentos y de los cobros posteriores.
type SaldoCuenta struct {
	CuentaID string `json:"cuentaId"`
	Titular  string `json:"titular"`
	Tipo     string `json:"tipo"`
	Datos    string `json:"datos"`
	Moneda   string `json:"moneda"`
	// Entradas en la moneda de la cuenta, y su equivalente en bolívares.
	Entradas    float64 `json:"entradas"`
	EntradasBs  float64 `json:"entradasBs"`
	Operaciones int     `json:"operaciones"`
}

// ResumenTesoreria son los saldos por cuenta más el efectivo de caja.
type ResumenTesoreria struct {
	Cuentas []SaldoCuenta `json:"cuentas"`
	// EfectivoBs y EfectivoUSD es el efectivo cobrado en el mostrador, que no
	// entra a ninguna cuenta bancaria: está en la gaveta.
	EfectivoBs  float64 `json:"efectivoBs"`
	EfectivoUSD float64 `json:"efectivoUsd"`
	// TotalBs es todo lo cobrado expresado en bolívares.
	TotalBs float64 `json:"totalBs"`
	// VueltoEntregadoBs es lo que salió de la gaveta como vuelto: sin restarlo, el
	// efectivo declarado sería mayor que el que hay.
	VueltoEntregadoBs float64 `json:"vueltoEntregadoBs"`
}

// SaldosDeTesoreria proyecta lo cobrado por cuenta y el efectivo en caja.
//
// Es una proyección de entradas, no un saldo bancario real: no hay conciliación
// todavía, y la interfaz lo declara. Lo que sí es exacto es cuánto entró por cada
// medio, que es lo que el dueño necesita para cuadrar el día.
func (s *Service) SaldosDeTesoreria(empresaID string) ResumenTesoreria {
	res := ResumenTesoreria{Cuentas: []SaldoCuenta{}}
	porCuenta := map[string]*SaldoCuenta{}
	for _, cc := range s.cuentasCobro.List(empresaID) {
		porCuenta[cc.ID] = &SaldoCuenta{
			CuentaID: cc.ID, Titular: cc.Titular, Tipo: cc.Tipo, Datos: cc.Datos, Moneda: cc.Moneda,
		}
	}

	sumar := func(cuentaID, metodo, moneda string, monto, montoBs float64) {
		res.TotalBs += montoBs
		if c, ok := porCuenta[cuentaID]; ok && cuentaID != "" {
			c.Entradas += monto
			c.EntradasBs += montoBs
			c.Operaciones++
			return
		}
		// Sin cuenta: es efectivo en la gaveta.
		if moneda == "USD" || metodo == fiscal.PagoEfectivoUSD {
			res.EfectivoUSD += monto
			return
		}
		res.EfectivoBs += monto
	}

	// Anulaciones: sus pagos no cuentan (la reversa deshace la venta).
	anulados := map[string]bool{}
	for _, d := range s.documentos.List(empresaID) {
		if d.Tipo == fiscal.TipoAnulacion && d.RefDocumentoID != "" {
			anulados[d.RefDocumentoID] = true
		}
	}
	for _, d := range s.documentos.List(empresaID) {
		if d.Tipo != fiscal.TipoFactura || anulados[d.ID] {
			continue
		}
		for _, pg := range d.Pagos {
			montoBs := pg.Monto
			if pg.EnDivisa && d.TasaCambio > 0 {
				montoBs = round2(pg.Monto * d.TasaCambio)
			}
			sumar(pg.CuentaID, pg.Metodo, pg.Moneda, pg.Monto, montoBs)
		}
		// El vuelto se pliega parte a parte (puede ser MIXTO). El efectivo devuelto
		// baja el conteo de su moneda (Bs o US$); el pago móvil es una transferencia
		// que no toca la gaveta pero sí el neto. Cada parte resta del total en Bs.
		for _, vp := range partesDeVuelto(d) {
			esEfectivo := vp.Metodo == "" || vp.Metodo == fiscal.VueltoEfectivo
			if esEfectivo {
				if vp.Moneda == empresa.MonedaUSD {
					res.EfectivoUSD -= vp.Monto
				} else if vp.Moneda == "" || vp.Moneda == empresa.MonedaVES {
					res.EfectivoBs -= vp.Monto
				}
			}
			res.VueltoEntregadoBs += vp.MontoBs
			res.TotalBs -= vp.MontoBs
		}
	}
	// Cobros posteriores (ventas a crédito).
	if s.cobros != nil {
		for _, c := range s.cobros.List(empresaID) {
			signo := 1.0
			if c.Reverso {
				signo = -1
			}
			sumar(c.CuentaID, c.Metodo, c.Moneda, signo*c.Monto, signo*c.MontoBs)
		}
	}

	for _, cc := range s.cuentasCobro.List(empresaID) {
		if c := porCuenta[cc.ID]; c != nil {
			c.Entradas = round2(c.Entradas)
			c.EntradasBs = round2(c.EntradasBs)
			res.Cuentas = append(res.Cuentas, *c)
		}
	}
	res.EfectivoBs = round2(res.EfectivoBs)
	res.EfectivoUSD = round2(res.EfectivoUSD)
	res.TotalBs = round2(res.TotalBs)
	res.VueltoEntregadoBs = round2(res.VueltoEntregadoBs)
	return res
}

/* --- Reporte de IGTF ------------------------------------------------------- */

// LineaIGTF es una operación que causó IGTF, para el reporte que se declara.
type LineaIGTF struct {
	DocumentoID    string `json:"documentoId"`
	NumeroCompleto string `json:"numeroCompleto"`
	Fecha          string `json:"fecha"`
	ClienteNombre  string `json:"clienteNombre"`
	// BaseDivisas es la porción de la factura pagada en divisas (en Bs).
	BaseDivisas float64 `json:"baseDivisas"`
	IGTF        float64 `json:"igtf"`
	Anulada     bool    `json:"anulada"`
}

// ReporteIGTF lista las operaciones con IGTF y su total, que es lo que se declara
// junto al IVA. Se deriva de los documentos: no hay un registro paralelo que
// pueda desincronizarse.
func (s *Service) ReporteIGTF(empresaID string) ([]LineaIGTF, float64) {
	anulados := map[string]bool{}
	for _, d := range s.documentos.List(empresaID) {
		if d.Tipo == fiscal.TipoAnulacion && d.RefDocumentoID != "" {
			anulados[d.RefDocumentoID] = true
		}
	}
	out := []LineaIGTF{}
	total := 0.0
	for _, d := range s.documentos.List(empresaID) {
		if d.Tipo != fiscal.TipoFactura || d.IGTF <= 0 {
			continue
		}
		// Se despeja la base con la tasa HISTÓRICA del documento (la que se aplicó
		// al emitir), no la config de hoy: usar otra tasa daría una base equivocada.
		// Fallback al default del sistema para documentos previos (tasa 0).
		tasa := d.AlicuotaIGTF
		if tasa <= 0 {
			tasa = fiscal.AlicuotaIGTF
		}
		base := 0.0
		if tasa > 0 {
			base = round2(d.IGTF / tasa)
		}
		l := LineaIGTF{
			DocumentoID: d.ID, NumeroCompleto: d.NumeroCompleto, Fecha: d.Fecha,
			ClienteNombre: d.ClienteNombre, BaseDivisas: base, IGTF: d.IGTF, Anulada: anulados[d.ID],
		}
		if !l.Anulada {
			total += d.IGTF
		}
		out = append(out, l)
	}
	// IGTF de ABONOS pagados en divisa (ventas a crédito cobradas después). Un cobro
	// reversado anula su IGTF, igual que una factura anulada.
	if s.cobros != nil {
		reversados := map[string]bool{}
		cobros := s.cobros.List(empresaID)
		for _, c := range cobros {
			if c.Reverso && c.RefCobroID != "" {
				reversados[c.RefCobroID] = true
			}
		}
		for _, c := range cobros {
			if c.Reverso || c.IGTF <= 0 {
				continue
			}
			anulada := reversados[c.ID]
			out = append(out, LineaIGTF{
				DocumentoID: c.DocumentoID, NumeroCompleto: c.DocumentoNum + " · abono",
				Fecha: c.Fecha, ClienteNombre: c.ClienteNombre,
				BaseDivisas: round2(c.MontoBs), IGTF: c.IGTF, Anulada: anulada,
			})
			if !anulada {
				total += c.IGTF
			}
		}
	}
	// Más reciente primero.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Fecha > out[j-1].Fecha; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, round2(total)
}
