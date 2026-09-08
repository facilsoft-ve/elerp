package application

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

// Errores de negocio de las retenciones (IVA e ISLR).
var (
	// ErrRetencionDuplicada: el documento ya tiene una retención de esa dirección e impuesto.
	ErrRetencionDuplicada = errors.New("ese documento ya tiene un comprobante de retención de ese impuesto registrado")
	// ErrRetencionDatos: falta el número del comprobante de retención.
	ErrRetencionDatos = errors.New("la retención requiere el número del comprobante")
	// ErrRetencionImpuesto: el impuesto debe ser "iva" o "islr".
	ErrRetencionImpuesto = errors.New("el impuesto de la retención debe ser IVA o ISLR")
	// ErrRetencionPorcentaje: el porcentaje debe estar en (0, 100].
	ErrRetencionPorcentaje = errors.New("el porcentaje de retención debe estar entre 0 y 100")
	// ErrRetencionSinIVA: no hay IVA que retener en el documento.
	ErrRetencionSinIVA = errors.New("ese documento no tiene IVA sobre el cual retener")
	// ErrRetencionSinBase: la retención de ISLR necesita una base gravable positiva.
	ErrRetencionSinBase = errors.New("la retención de ISLR requiere una base gravable mayor que cero")
	// ErrRetencionSinMonto: tras aplicar el sustraendo no queda monto que retener.
	ErrRetencionSinMonto = errors.New("la retención no arroja monto a retener")
	// ErrRetencionSoloCredito: la retención recibida solo aplica a facturas a
	// crédito; una de contado no deja saldo por cobrar que la retención pueda bajar.
	ErrRetencionSoloCredito = errors.New("solo se puede registrar retención recibida sobre facturas a crédito")
	// ErrRetencionNoConfig: el módulo de retenciones no está cableado.
	ErrRetencionNoConfig = errors.New("las retenciones no están configuradas")
	// ErrFacturaCompraNoExiste: no hay factura de compra con ese id.
	ErrFacturaCompraNoExiste = errors.New("esa factura de compra no existe")
	// ErrNoEsAgenteRetencion: la empresa no está configurada como agente de retención
	// de ese impuesto, así que no puede EMITIR comprobantes de retención.
	ErrNoEsAgenteRetencion = errors.New("la empresa no es agente de retención de ese impuesto: actívalo en Configuración › Impuestos")
)

// numeroComprobanteRetencion genera el número del comprobante de retención EMITIDO,
// con el formato SENIAT de 14 posiciones: AAAAMM + secuencial de 8 dígitos por mes.
// La secuencia es atómica por empresa+impuesto+período (reutiliza el Numerador con
// una serie que incluye el período, de modo que reinicia cada mes).
func (s *Service) numeroComprobanteRetencion(empresaID, impuesto, fecha string) string {
	periodo := fecha
	if len(periodo) < 7 {
		periodo = ahora()
	}
	aaaamm := periodo[0:4] + periodo[5:7] // "YYYYMM"
	serie := "RET" + strings.ToUpper(impuesto) + aaaamm
	seq := s.numerador.Siguiente(empresaID, "", serie)
	return fmt.Sprintf("%s%08d", aaaamm, seq)
}

// documentoAnulado indica si un documento de venta tiene una reversa (anulación)
// que lo deja sin efecto. El estado "anulado" es DERIVADO, no una marca.
func (s *Service) documentoAnulado(empresaID, docID string) bool {
	for _, d := range s.documentos.List(empresaID) {
		if d.Tipo == fiscal.TipoAnulacion && d.RefDocumentoID == docID {
			return true
		}
	}
	return false
}

// EntradaRetencion son los datos de un comprobante de retención (IVA o ISLR). El
// monto NO se pide: se DERIVA de la base por el porcentaje (menos el sustraendo en
// ISLR), para que el comprobante no pueda declarar una retención inconsistente con
// la factura que la origina.
//
// Para IVA la base es SIEMPRE el IVA del documento (los campos Base/Concepto/
// Sustraendo se ignoran). Para ISLR la base, el concepto y el sustraendo los
// aporta quien registra (dependen del concepto y de la tabla del reglamento).
type EntradaRetencion struct {
	Impuesto          string  // "iva" | "islr"; vacío se interpreta como IVA
	NumeroComprobante string  // el del comprobante de retención (lo emite el agente)
	Fecha             string  // fecha del comprobante
	Porcentaje        float64 // 75 | 100 típicamente; en (0, 100]
	// Solo ISLR:
	Base       float64 // base gravable de ISLR (el monto sujeto a retención)
	Concepto   string  // etiqueta del concepto ISLR (honorarios, arrendamientos, …)
	Sustraendo float64 // se resta al calcular la retención de ISLR
}

// impuestoNorm normaliza el impuesto de la entrada: vacío → IVA (compatibilidad),
// y valida que sea uno de los dos soportados.
func impuestoNorm(imp string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(imp)) {
	case "", fiscal.ImpuestoIVA:
		return fiscal.ImpuestoIVA, nil
	case fiscal.ImpuestoISLR:
		return fiscal.ImpuestoISLR, nil
	}
	return "", ErrRetencionImpuesto
}

// calcularRetencion valida la entrada y deriva (base, monto) según el impuesto.
// ivaDoc es el IVA del documento sobre el que se retiene (solo relevante en IVA).
//   - IVA:  base = ivaDoc (exige > 0); monto = round2(base * %/100).
//   - ISLR: base = in.Base (exige > 0); monto = max(0, round2(base*%/100 −
//     sustraendo)) (exige > 0).
func calcularRetencion(impuesto string, ivaDoc float64, in EntradaRetencion) (base, monto float64, err error) {
	if in.Porcentaje <= 0 || in.Porcentaje > 100 {
		return 0, 0, ErrRetencionPorcentaje
	}
	if impuesto == fiscal.ImpuestoISLR {
		base = in.Base
		if base <= 0.004 {
			return 0, 0, ErrRetencionSinBase
		}
		monto = round2(base*in.Porcentaje/100 - in.Sustraendo)
		if monto < 0 {
			monto = 0
		}
		if monto <= 0.004 {
			return 0, 0, ErrRetencionSinMonto
		}
		return base, monto, nil
	}
	// IVA: la base es el IVA del documento; no hay sustraendo.
	base = ivaDoc
	if base <= 0.004 {
		return 0, 0, ErrRetencionSinIVA
	}
	monto = round2(base * in.Porcentaje / 100)
	return base, monto, nil
}

// Retenciones lista los comprobantes de retención de la empresa (más reciente
// primero).
func (s *Service) Retenciones(empresaID string) []fiscal.Retencion {
	if s.retenciones == nil {
		return []fiscal.Retencion{}
	}
	out := s.retenciones.List(empresaID)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Registrada > out[j-1].Registrada; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Retencion devuelve un comprobante de retención por id.
func (s *Service) Retencion(empresaID, id string) (fiscal.Retencion, bool) {
	if s.retenciones == nil {
		return fiscal.Retencion{}, false
	}
	return s.retenciones.ByID(empresaID, id)
}

// RegistrarRetencionRecibida registra el comprobante que un cliente agente de
// retención emitió sobre una FACTURA DE VENTA propia: retuvo un % del IVA o del
// ISLR. Baja la CxC por lo retenido y reconoce un activo (impuesto a favor). Es
// APPEND-ONLY y único por (dirección, impuesto, documento): una factura puede
// recibir a la vez un comprobante de IVA y uno de ISLR, pero no dos del mismo.
//
// Asiento derivado (= monto retenido):
//   - IVA:  Debe Retención IVA a favor (1104)  / Haber Cuentas por cobrar (1102).
//   - ISLR: Debe Retención ISLR a favor (1105)  / Haber Cuentas por cobrar (1102).
func (s *Service) RegistrarRetencionRecibida(empresaID, actor, origen, documentoID string, in EntradaRetencion) (fiscal.Retencion, error) {
	if s.retenciones == nil {
		return fiscal.Retencion{}, ErrRetencionNoConfig
	}
	impuesto, err := impuestoNorm(in.Impuesto)
	if err != nil {
		return fiscal.Retencion{}, err
	}
	// El comprobante RECIBIDO lo numeró el cliente (agente): su número es dato de
	// entrada obligatorio. (El EMITIDO, en cambio, lo numera el sistema.)
	if strings.TrimSpace(in.NumeroComprobante) == "" {
		return fiscal.Retencion{}, ErrRetencionDatos
	}
	doc, ok := s.documentos.ByID(empresaID, documentoID)
	if !ok || doc.Tipo != fiscal.TipoFactura {
		return fiscal.Retencion{}, ErrDocumentoNoExiste
	}
	if s.documentoAnulado(empresaID, doc.ID) {
		return fiscal.Retencion{}, ErrYaAnulado
	}
	// Coherencia asiento⇄proyección: el asiento ACREDITA Cuentas por cobrar (1102),
	// lo que presupone un saldo por cobrar; pero la proyección de CxC solo pliega
	// documentos a crédito (ver CuentasPorCobrar). En una factura de contado el
	// cliente ya pagó todo en el mostrador: no hay 1102 vivo que rebajar, así que
	// acreditarlo dejaría un saldo colgado que la proyección nunca reflejaría →
	// descuadre. Una retención sobre contado, de existir, se resolvería como vuelto/
	// menor cobro en caja, no como crédito de CxC. Por eso se restringe a crédito.
	if !doc.Credito {
		return fiscal.Retencion{}, ErrRetencionSoloCredito
	}
	if _, existe := s.retenciones.ByDocumento(empresaID, fiscal.RetencionRecibida, impuesto, doc.ID); existe {
		return fiscal.Retencion{}, ErrRetencionDuplicada
	}
	base, monto, err := calcularRetencion(impuesto, doc.IVA, in)
	if err != nil {
		return fiscal.Retencion{}, err
	}

	concepto, sustraendo := "", 0.0
	if impuesto == fiscal.ImpuestoISLR {
		concepto, sustraendo = strings.TrimSpace(in.Concepto), in.Sustraendo
	}
	r := s.retenciones.Append(fiscal.Retencion{
		EmpresaID: empresaID, Tipo: fiscal.RetencionRecibida, Impuesto: impuesto,
		DocumentoID: doc.ID, DocumentoNumero: doc.NumeroCompleto,
		NumeroComprobante: strings.TrimSpace(in.NumeroComprobante), Fecha: in.Fecha,
		TerceroNombre: doc.ClienteNombre, TerceroRIF: doc.ClienteDocumento,
		Base: base, Concepto: concepto, Sustraendo: sustraendo,
		Porcentaje: in.Porcentaje, MontoRetenido: monto,
		Actor: actor, Registrada: ahora(),
	})

	// Asiento: el cliente pagará neto de retención, así que su deuda baja; lo
	// retenido queda como un crédito del impuesto a favor de la empresa.
	s.asentarRetencion(empresaID, actor, r)
	s.audit.Append(evento(empresaID, actor, origen, "fiscal.retencion.recibida", r.NumeroComprobante, doc.NumeroCompleto))
	return r, nil
}

// RegistrarRetencionEmitida registra el comprobante que TÚ emites al retenerle el
// IVA o el ISLR a un proveedor sobre una FACTURA DE COMPRA. Baja la CxP por lo
// retenido y crea el pasivo (impuesto retenido por enterar al SENIAT). Es
// APPEND-ONLY y único por (dirección, impuesto, factura de compra).
//
// Asiento derivado (= monto retenido):
//   - IVA:  Debe Cuentas por pagar (2101) / Haber IVA retenido por enterar (2203).
//   - ISLR: Debe Cuentas por pagar (2101) / Haber ISLR retenido por enterar (2204).
func (s *Service) RegistrarRetencionEmitida(empresaID, actor, origen, facturaCompraID string, in EntradaRetencion) (fiscal.Retencion, error) {
	if s.retenciones == nil || s.facturasCompra == nil {
		return fiscal.Retencion{}, ErrRetencionNoConfig
	}
	impuesto, err := impuestoNorm(in.Impuesto)
	if err != nil {
		return fiscal.Retencion{}, err
	}
	// Solo un AGENTE DE RETENCIÓN puede emitir el comprobante (config de la empresa).
	emp, ok := s.empresas.ByID(empresaID)
	if !ok {
		return fiscal.Retencion{}, ErrEmpresaNoExiste
	}
	if (impuesto == fiscal.ImpuestoIVA && !emp.AgenteRetencionIVA) || (impuesto == fiscal.ImpuestoISLR && !emp.AgenteRetencionISLR) {
		return fiscal.Retencion{}, ErrNoEsAgenteRetencion
	}
	fc, ok := s.facturasCompra.ByID(empresaID, facturaCompraID)
	if !ok {
		return fiscal.Retencion{}, ErrFacturaCompraNoExiste
	}
	if _, existe := s.retenciones.ByDocumento(empresaID, fiscal.RetencionEmitida, impuesto, fc.ID); existe {
		return fiscal.Retencion{}, ErrRetencionDuplicada
	}
	// % por defecto de IVA (75% salvo config); el de ISLR lo aporta quien registra.
	if impuesto == fiscal.ImpuestoIVA && in.Porcentaje <= 0 {
		if in.Porcentaje = emp.RetencionIVAPorcentaje; in.Porcentaje <= 0 {
			in.Porcentaje = 75
		}
	}
	base, monto, err := calcularRetencion(impuesto, fc.IVA, in)
	if err != nil {
		return fiscal.Retencion{}, err
	}
	// El número del comprobante lo GENERA el sistema (agente): correlativo AAAAMM.
	if strings.TrimSpace(in.Fecha) == "" {
		in.Fecha = ahora()[:10]
	}
	in.NumeroComprobante = s.numeroComprobanteRetencion(empresaID, impuesto, in.Fecha)

	// Número mostrable de la factura del proveedor: su correlativo, y si no, el
	// número de control fiscal.
	numDoc := fc.NumeroFactura
	if numDoc == "" {
		numDoc = fc.NumeroControl
	}
	concepto, sustraendo := "", 0.0
	if impuesto == fiscal.ImpuestoISLR {
		concepto, sustraendo = strings.TrimSpace(in.Concepto), in.Sustraendo
	}
	r := s.retenciones.Append(fiscal.Retencion{
		EmpresaID: empresaID, Tipo: fiscal.RetencionEmitida, Impuesto: impuesto,
		DocumentoID: fc.ID, DocumentoNumero: numDoc,
		NumeroComprobante: strings.TrimSpace(in.NumeroComprobante), Fecha: in.Fecha,
		TerceroNombre: fc.ProveedorNombre, TerceroRIF: fc.ProveedorRIF,
		Base: base, Concepto: concepto, Sustraendo: sustraendo,
		Porcentaje: in.Porcentaje, MontoRetenido: monto,
		Actor: actor, Registrada: ahora(),
	})

	// Asiento: parte de la deuda con el proveedor deja de pagarse a él (baja CxP) y
	// pasa a deberse al SENIAT como impuesto retenido por enterar.
	s.asentarRetencion(empresaID, actor, r)
	s.audit.Append(evento(empresaID, actor, origen, "fiscal.retencion.emitida", r.NumeroComprobante, numDoc))
	return r, nil
}

// asentarRetencion deriva el asiento de un comprobante de retención según su
// dirección y su impuesto (todos cuadran, monto al debe = monto al haber):
//
//	Recibida IVA:  Debe 1104 Retención IVA a favor  / Haber 1102 Cuentas por cobrar.
//	Recibida ISLR: Debe 1105 Retención ISLR a favor / Haber 1102 Cuentas por cobrar.
//	Emitida IVA:   Debe 2101 Cuentas por pagar / Haber 2203 IVA retenido por enterar.
//	Emitida ISLR:  Debe 2101 Cuentas por pagar / Haber 2204 ISLR retenido por enterar.
//
// Es idempotente vía RefID: RecontabilizarPendientes se apoya en PorRef para no
// duplicar el asiento de una retención sembrada. El RefTipo se mantiene como
// "retencion_iva" por continuidad del histórico (cubre IVA e ISLR).
func (s *Service) asentarRetencion(empresaID, actor string, r fiscal.Retencion) {
	etiqueta := "IVA"
	if r.Impuesto == fiscal.ImpuestoISLR {
		etiqueta = "ISLR"
	}
	var lineas []contabilidad.Linea
	var glosa string
	if r.Tipo == fiscal.RetencionRecibida {
		ctaFavor := contabilidad.CtaRetencionIVAaFavor
		if r.Impuesto == fiscal.ImpuestoISLR {
			ctaFavor = contabilidad.CtaRetencionISLRaFavor
		}
		glosa = "Retención de " + etiqueta + " recibida " + r.NumeroComprobante + " (" + r.DocumentoNumero + ")"
		lineas = []contabilidad.Linea{
			{Codigo: ctaFavor, Debe: r.MontoRetenido},
			{Codigo: contabilidad.CtaCuentasPorCobrar, Haber: r.MontoRetenido},
		}
	} else {
		ctaPasivo := contabilidad.CtaIVARetenidoPorEnterar
		if r.Impuesto == fiscal.ImpuestoISLR {
			ctaPasivo = contabilidad.CtaISLRRetenidoPorEnterar
		}
		glosa = "Retención de " + etiqueta + " emitida " + r.NumeroComprobante + " (" + r.DocumentoNumero + ")"
		lineas = []contabilidad.Linea{
			{Codigo: contabilidad.CtaCuentasPorPagar, Debe: r.MontoRetenido},
			{Codigo: ctaPasivo, Haber: r.MontoRetenido},
		}
	}
	s.asentar(empresaID, actor, r.Fecha, glosa, "retencion_iva", r.ID, lineas)
}

// retencionesRecibidasPorDoc suma lo retenido por comprobantes RECIBIDOS por cada
// documento de venta, de CUALQUIER impuesto (IVA + ISLR). Alimenta Cuentas por
// cobrar: lo retenido cuenta como aplicado (el cliente paga neto de retención).
func (s *Service) retencionesRecibidasPorDoc(empresaID string) map[string]float64 {
	out := map[string]float64{}
	if s.retenciones == nil {
		return out
	}
	for _, r := range s.retenciones.List(empresaID) {
		if r.Tipo == fiscal.RetencionRecibida {
			out[r.DocumentoID] += r.MontoRetenido
		}
	}
	return out
}

// retencionesEmitidasPorFactura suma lo retenido por comprobantes EMITIDOS sobre
// cada factura de compra, de CUALQUIER impuesto (IVA + ISLR). Alimenta Cuentas por
// pagar: lo retenido cuenta como aplicado (parte de la deuda pasa al SENIAT, no al
// proveedor).
func (s *Service) retencionesEmitidasPorFactura(empresaID string) map[string]float64 {
	out := map[string]float64{}
	if s.retenciones == nil {
		return out
	}
	for _, r := range s.retenciones.List(empresaID) {
		if r.Tipo == fiscal.RetencionEmitida {
			out[r.DocumentoID] += r.MontoRetenido
		}
	}
	return out
}
