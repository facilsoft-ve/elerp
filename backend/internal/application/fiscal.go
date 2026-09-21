package application

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/inventario"
	"github.com/mornix/elerp/internal/domain/tasa"
)

// arqueo calcula lo cobrado en bolívares y el vuelto a devolver.
//
// Multimoneda: cada pago trae SU tasa (Bs por unidad de su divisa), grabada al
// emitir; lo cobrado en Bs es la suma de cada pago convertido con su propia tasa,
// no con una tasa única del documento.
//
// Devuelve lo cobrado en Bs, el EXCEDENTE en bolívares (0 si no hay vuelto) y la
// moneda/tasa del vuelto DERIVADO por defecto: si el cliente pagó en efectivo en
// divisas (US$, €) se le devuelve en ESA divisa a su tasa (con varios efectivos
// en divisa, el del ÚLTIMO recibido, regla simple y documentada); si pagó en
// bolívares, el vuelto va en bolívares. La caja puede DECLARAR otra moneda; esa
// decisión la toma EmitirFactura, aquí solo se calcula el default retrocompatible.
func arqueo(pagos []fiscal.Pago, total float64) (cobrado, excedenteBs float64, defMoneda string, defTasa float64) {
	// Moneda y tasa del último efectivo en divisa: es en esa divisa que se
	// devuelve el excedente por defecto (la caja entrega billetes de esa moneda).
	vueltoMoneda := ""
	vueltoTasa := 0.0
	for _, pg := range pagos {
		if pg.EnDivisa {
			if pg.TasaCambio <= 0 {
				// Sin tasa no se puede convertir; EmitirFactura ya lo rechazó antes
				// de llegar acá, pero no se inventa un equivalente por si acaso.
				continue
			}
			cobrado += pg.Monto * pg.TasaCambio
			if pg.Metodo == fiscal.PagoEfectivoUSD {
				vueltoMoneda = pg.Moneda
				vueltoTasa = pg.TasaCambio
			}
			continue
		}
		cobrado += pg.Monto
	}
	cobrado = round2(cobrado)
	excedente := round2(cobrado - total)
	if excedente <= 0.004 {
		return cobrado, 0, "", 0
	}
	// Default: el excedente de un pago en efectivo en divisas se devuelve en esa
	// divisa; en cualquier otro caso, en bolívares.
	if vueltoMoneda != "" && vueltoTasa > 0 {
		return cobrado, excedente, vueltoMoneda, vueltoTasa
	}
	return cobrado, excedente, empresa.MonedaVES, 1
}

// monedaDelPrecio resuelve en qué moneda está expresado el precio de un
// producto: la suya si la declara, y si no la principal de la empresa. Los
// productos creados antes de R10 no traen el campo y se leen como la principal,
// que es exactamente lo que eran.
func monedaDelPrecio(p inventario.Producto, monedaEmpresa string) string {
	if p.Moneda != "" {
		return p.Moneda
	}
	if monedaEmpresa != "" {
		return monedaEmpresa
	}
	return empresa.MonedaVES
}

// diasCreditoPorDefecto es el plazo cuando no se indica otro: 30 días es lo
// habitual entre comercios.
const diasCreditoPorDefecto = 30

var (
	ErrDocumentoVacio    = errors.New("el documento no tiene líneas")
	ErrCobroInsuficiente = errors.New("el cobro registrado no cubre el total del documento")
	ErrCreditoSinCliente = errors.New("una venta a crédito necesita un cliente identificado: al consumidor final no se le fía")
	// ErrReceptorDocumentoInvalido: al emitir con un cliente identificado, su
	// documento (RIF/cédula) debe ser válido. El consumidor final (sin cliente) no
	// lleva documento y queda permitido para la venta minorista.
	ErrReceptorDocumentoInvalido = errors.New("el documento del cliente (receptor) no es válido para facturar")
	// ErrReceptorSinDireccion: la factura venezolana exige el domicilio fiscal del
	// receptor. Se pide con el mismo criterio que el RIF —solo al cliente
	// identificado— porque al consumidor final tampoco se le pide RIF.
	ErrReceptorSinDireccion = errors.New("el cliente no tiene dirección fiscal y la factura la exige")
	ErrDocumentoNoExiste    = errors.New("documento no existe")
	ErrYaAnulado            = errors.New("el documento ya fue anulado")
	ErrNotaCreditoVacia     = errors.New("la nota de crédito no acredita ninguna cantidad")
	ErrNotaCreditoSKU       = errors.New("el producto no está en la factura original")
	ErrNotaCreditoExcede    = errors.New("la cantidad a acreditar excede lo facturado (descontando notas de crédito previas)")
	ErrNotaDebitoVacia      = errors.New("la nota de débito no carga ningún monto")
	ErrNotaDebitoConcepto   = errors.New("la nota de débito necesita un concepto que explique el cargo")
	ErrSinTasaDivisa        = errors.New("no hay tasa cargada para esa divisa: sin ella no se puede convertir el pago a bolívares")
	// ErrComboNoFacturable: un combo nunca entra como línea de la factura. El
	// frontend lo explota en las líneas de sus componentes antes de emitir; esta
	// guarda de servidor evita que un combo entre como línea por error.
	ErrComboNoFacturable = errors.New("un combo debe facturarse por sus componentes")

	ErrVueltoMonedaInvalida = errors.New("la moneda del vuelto debe ser VES o una divisa activa con tasa cargada")
	ErrVueltoPagoMovilDatos = errors.New("un vuelto por pago móvil exige banco, cédula y teléfono del cliente")
	ErrVueltoMetodoInvalido = errors.New("el medio del vuelto debe ser efectivo o pago móvil")
	ErrVueltoNoCuadra       = errors.New("las partes del vuelto no suman el excedente a devolver")
)

// LineaEntrada es un renglón recibido del POS.
type LineaEntrada struct {
	SKU      string
	Cantidad float64
	// PrecioUnitario resuelve el precio de la línea al EMITIR (ver EmitirFactura)
	// con este contrato:
	//   - >= 0 ⇒ es AUTORITATIVO y ya viene en BOLÍVARES (la moneda de emisión):
	//     se usa TAL CUAL, sin volver a convertir por la tasa. El POS/Ventas ya
	//     convierten a Bs en el front, así que el servidor no re-multiplica. 0 es
	//     una línea GRATIS legítima (p. ej. la que un cupón deja en cero) y se
	//     respeta.
	//   - < 0 ⇒ "usar el precio del CATÁLOGO": se toma el precio del producto en
	//     SU moneda (la del catálogo, que puede ser US$, R10) y se convierte a Bs
	//     con la tasa vigente del servidor. Es la ÚNICA ruta que multiplica por la
	//     tasa; el documento guarda esa tasa (memoria histórica).
	//
	// Nota: la construcción de cotizaciones (armarLineasCotizacion) conserva su
	// propia resolución (0 = catálogo), porque la cotización se arma en Bs.
	PrecioUnitario float64
	// Descripcion es texto libre opcional por línea (usado en cotizaciones).
	Descripcion string
	// Descuento es el porcentaje 0..100 aplicado a la línea (usado en cotizaciones).
	Descuento float64
}

// PagoEntrada es una línea de cobro recibida del POS.
type PagoEntrada struct {
	Metodo     string
	CuentaID   string
	Monto      float64
	Moneda     string
	Referencia string
}

// VueltoParteEntrada es una parte del vuelto declarada por la caja: en qué
// moneda y por qué medio se devuelve, y cuánto (en esa moneda). El servidor
// resuelve su tasa y su equivalente en Bs, y valida que la SUMA de las partes en
// bolívares cuadre con el excedente calculado. Los datos de pago móvil solo
// aplican a las partes con Metodo == "pago_movil".
type VueltoParteEntrada struct {
	Moneda   string
	Metodo   string
	Monto    float64
	Banco    string
	Cedula   string
	Telefono string
}

// EmitirEntrada son los datos para emitir una factura.
//
// Ojo: NO lleva tasa de cambio. La tasa no se teclea ni se acepta del cliente
// (R9); la toma el servidor de la fuente oficial y la copia al documento, que
// es lo que la vuelve memoria histórica confiable (Art. 177). Si el POS pudiera
// mandarla, el IGTF y los montos en divisas dependerían del navegador.
type EmitirEntrada struct {
	ClienteID    string
	Lineas       []LineaEntrada
	Pagos        []PagoEntrada
	Moneda       string
	Contingencia bool
	// Credito: venta a crédito. Lo que no se cobró en el mostrador queda por
	// cobrar en Tesorería. Exige cliente identificado —no se le da crédito a un
	// «consumidor final» que nadie puede volver a encontrar— y días de plazo.
	Credito     bool
	DiasCredito int
	// SinCaja distingue el CANAL de facturación: false = por CAJA (POS, exige
	// una caja abierta a nombre del actor); true = FORMA LIBRE (módulo Ventas,
	// personal administrativo sin caja registradora). La factura legal es la
	// misma; solo cambia la trazabilidad de arqueo.
	SinCaja bool

	// SedeID sobreescribe —solo si viene— la sede de la que se DESCUENTA el
	// inventario (el almacén de despacho). Vacío ⇒ la salida sale de la sede del
	// contexto (sedeID), como siempre. El documento fiscal, su numeración y su
	// arqueo siguen atados a la sede del contexto; esto mueve únicamente el ledger
	// de inventario, para modelar «facturo en esta sede, despacho de aquel almacén».
	SedeID string
	// AlmacenID sobreescribe —solo si viene y pertenece a la sede de despacho— el
	// almacén del que se descuenta el stock. Vacío ⇒ el almacén PRINCIPAL de la sede
	// de despacho (el POS despacha del principal).
	AlmacenID string

	// Datos del VUELTO declarados por la caja (todos opcionales). El monto del
	// vuelto lo calcula el servidor; la caja solo decide CÓMO devolverlo:
	//   - VueltoMoneda: en qué moneda se entrega el vuelto. Vacío ⇒ se deriva como
	//     siempre (la del efectivo en divisa recibido, o Bs). Puede ser "VES"
	//     aunque se haya cobrado en US$ (la caja quizá no tiene divisas), o una
	//     divisa ACTIVA con tasa cargada.
	//   - VueltoMetodo: "efectivo" (default) | "pago_movil".
	//   - VueltoBanco/VueltoCedula/VueltoTelefono: obligatorios si el vuelto es por
	//     pago móvil (a quién se le transfiere).
	VueltoMoneda   string
	VueltoMetodo   string
	VueltoBanco    string
	VueltoCedula   string
	VueltoTelefono string
	// VueltoPartes reparte el vuelto en varias partes (moneda + medio + monto en
	// esa moneda). Es la vía nueva y preferida: si viene, MANDA sobre los campos
	// únicos de arriba y el servidor valida que la suma en Bs cuadre con el
	// excedente. Vacía ⇒ se usa el vuelto de una sola parte (retrocompat).
	VueltoPartes []VueltoParteEntrada

	// CuponCodigo es el código del cupón APLICADO a esta venta (opcional). El
	// front ya bajó el precioUnitario de las líneas con el descuento (el motor
	// fiscal no recalcula el cupón); este código sirve para CONSUMIR el cupón al
	// emitir con éxito (sube UsosActuales, para que el tope UsosMax se aplique de
	// verdad). Vacío ⇒ sin cupón.
	CuponCodigo string
}

// DocumentoView es un documento con su estado derivado (anulado si tiene reversa).
type DocumentoView struct {
	fiscal.Documento
	Anulado bool `json:"anulado"`
}

// Documentos lista los documentos fiscales con estado derivado.
func (s *Service) Documentos(empresaID string) []DocumentoView {
	docs := s.documentos.List(empresaID)
	anulados := map[string]bool{}
	for _, d := range docs {
		if d.Tipo == fiscal.TipoAnulacion && d.RefDocumentoID != "" {
			anulados[d.RefDocumentoID] = true
		}
	}
	out := make([]DocumentoView, 0, len(docs))
	for _, d := range docs {
		out = append(out, DocumentoView{Documento: d, Anulado: anulados[d.ID]})
	}
	return out
}

// Documento devuelve un documento por id.
func (s *Service) Documento(empresaID, id string) (fiscal.Documento, bool) {
	return s.documentos.ByID(empresaID, id)
}

// EmitirFactura emite una factura: calcula IVA/IGTF, asigna folio fiscal
// serializado, la persiste (inmutable) y descuenta inventario vía el ledger.
func (s *Service) EmitirFactura(empresaID, sedeID, modalidad, actor, origen string, in EmitirEntrada) (fiscal.Documento, error) {
	if len(in.Lineas) == 0 {
		return fiscal.Documento{}, ErrDocumentoVacio
	}

	// Canal CAJA (POS): REGLA DURA — sin una caja abierta a nombre de quien
	// factura, no se emite. Es la contrapartida de servidor de la vista «Sin
	// caja abierta» (03 §4.4). El canal FORMA LIBRE (módulo Ventas) NO usa caja:
	// lo opera personal administrativo, así que se omite el requisito y el
	// documento queda sin atribución de arqueo.
	var cajaID, cajaCodigo, cajeroNombre, sesionCajaID string
	if !in.SinCaja {
		sesion, abierta := s.sesiones.AbiertaDeActor(empresaID, actor)
		if !abierta {
			return fiscal.Documento{}, ErrSinCajaAbierta
		}
		// La caja abierta tiene que ser de la sede en la que se está facturando:
		// un turno de Chacao no puede emitir documentos de Sabana Grande.
		if sesion.SedeID != sedeID {
			return fiscal.Documento{}, ErrCajaOtraSede
		}
		cajaID, cajaCodigo, cajeroNombre, sesionCajaID = sesion.CajaID, sesion.CajaCodigo, sesion.CajeroNombre, sesion.ID
	}

	if in.Moneda == "" {
		in.Moneda = "VES"
	}

	// La tasa la pone el SERVIDOR, nunca el cliente (R9). Solo se exige cuando
	// realmente hace falta convertir: una venta íntegramente en bolívares con
	// precios en bolívares factura sin tasa, y así la caja sigue cobrando aunque
	// el BCV no haya respondido hoy.
	tasaActual, hayTasa := s.TasaVigente(empresaID)
	monedaEmpresa := s.MonedaPrincipalDe(empresaID)

	doc := fiscal.Documento{
		EmpresaID: empresaID, SedeID: sedeID, Tipo: fiscal.TipoFactura,
		Modalidad: modalidad, Contingencia: in.Contingencia,
		Moneda: in.Moneda, TasaCambio: tasaActual.Valor, TasaFuente: tasaActual.Fuente,
		Actor: actor, Fecha: ahora(),
		// Trazabilidad de arqueo: qué caja y qué cajero cobraron este documento.
		CajaID:       cajaID,
		CajaCodigo:   cajaCodigo,
		CajeroNombre: cajeroNombre,
		SesionCajaID: sesionCajaID,
	}

	// Líneas desde el catálogo.
	// Desglose por alícuota: una fila por tasa aplicada. Es lo que el libro de
	// ventas declara en columnas separadas y lo que la suma `IVA` ya no permite
	// descomponer.
	desglose := map[string]*fiscal.DocumentoImpuesto{}
	ordenDesglose := []string{}
	for _, l := range in.Lineas {
		p, ok := s.productos.BySKU(empresaID, l.SKU)
		if !ok {
			return fiscal.Documento{}, fmt.Errorf("%w: %s", ErrProductoNoExiste, l.SKU)
		}
		// Un combo no se factura como línea: se explota en sus componentes (lo hace
		// el frontend antes de enviar). Esta guarda evita que entre por error.
		if p.EsCombo {
			return fiscal.Documento{}, fmt.Errorf("%w: %s", ErrComboNoFacturable, l.SKU)
		}
		// Resolución del precio de línea (contrato de LineaEntrada.PrecioUnitario):
		//   - >= 0 ⇒ AUTORITATIVO y ya en Bs (moneda de emisión): se usa tal cual,
		//     sin reconvertir por la tasa (el front ya convirtió). 0 = línea gratis.
		//   - < 0 ⇒ "usar precio de catálogo": el precio del producto va en SU moneda
		//     (puede ser US$, R10) y se convierte a Bs con la tasa del servidor. Es la
		//     ÚNICA ruta que multiplica por la tasa; el documento guarda esa tasa.
		var precio float64
		if l.PrecioUnitario >= 0 {
			precio = l.PrecioUnitario
		} else {
			precio = p.Precio
			if monedaDelPrecio(p, monedaEmpresa) == empresa.MonedaUSD {
				if !hayTasa {
					return fiscal.Documento{}, ErrSinTasa
				}
				precio = round2(precio * tasaActual.Valor)
			}
		}
		total := precio * l.Cantidad
		// Clasificación fiscal del renglón, resuelta contra el maestro con la
		// vigencia del DÍA de emisión y SELLADA en la línea.
		al := s.alicuotaDeProducto(empresaID, p, doc.Fecha)
		doc.Lineas = append(doc.Lineas, fiscal.Linea{
			ProductoID: p.ID, SKU: p.SKU, Nombre: p.Nombre,
			Cantidad: l.Cantidad, PrecioUnitario: precio, Total: total,
			Exento:         !al.Grava(),
			AlicuotaCodigo: al.Codigo, Alicuota: al.Porcentaje, AlicuotaAdicional: al.Adicional,
			// Snapshot de la receta si es un plato: el inventario descontará sus
			// insumos, no el plato. Vacío para un producto normal.
			Insumos: s.recetaSnapshot(empresaID, p),
		})
		doc.Subtotal += total
		// El IVA sale solo de lo gravado: la cesta básica venezolana está exenta
		// y la factura tiene que separar las bases.
		if !al.Grava() {
			doc.BaseExenta += total
		} else {
			doc.BaseImponible += total
		}
		acumularImpuesto(desglose, &ordenDesglose, al, total)
	}
	doc.BaseImponible = round2(doc.BaseImponible)
	doc.BaseExenta = round2(doc.BaseExenta)
	// Subtotal COHERENTE con las bases ya redondeadas: el asiento de venta arma el
	// Haber sumando las bases redondeadas (+IVA+IGTF), así que derivar el Total del
	// subtotal crudo lo separaba hasta 1 céntimo del Haber → el asiento descuadraba
	// (>0,005) y se descartaba en silencio, dejando la factura SIN asiento de venta
	// ni de costo. Con esto, Total ≡ base+IVA+IGTF ≡ Haber y el asiento siempre cuadra.
	doc.Subtotal = round2(doc.BaseImponible + doc.BaseExenta)
	// Tasas configuradas por la empresa, GRABADAS en el documento: las derivaciones
	// (nota de crédito, base de IGTF) las leen de aquí, no de la config de mañana.
	doc.AlicuotaIVA = s.alicuotaIVA(empresaID)
	doc.AlicuotaIGTF = s.alicuotaIGTF(empresaID)
	// El IVA es la SUMA del desglose, no una base por una tasa: con varias
	// alícuotas conviviendo (16%, 8% y el recargo suntuario) multiplicar la base
	// total por una sola tasa daría cualquier cosa.
	doc.Impuestos, doc.IVA = cerrarDesglose(desglose, ordenDesglose)

	// Pagos + IGTF sobre la porción en divisas (cobrado en Bs, impuesto separado).
	// Multimoneda: cada pago se convierte con la tasa de SU divisa (USD, EUR, …),
	// no con una tasa única. La tasa la resuelve el SERVIDOR por la moneda del pago
	// (nunca la teclea el POS, R9) y se graba en el pago (memoria histórica).
	var divisasEntregadas float64
	for _, pg := range in.Pagos {
		enDivisa := pg.Moneda != "" && pg.Moneda != "VES"
		tasaPago := 1.0 // VES: no convierte
		if enDivisa {
			tp, ok := s.TasaVigenteDeMoneda(empresaID, pg.Moneda)
			if !ok {
				// Sin tasa de esa divisa no se puede convertir el cobro. El dólar
				// conserva el error histórico (ErrSinTasa); las demás divisas usan el
				// específico. En ambos casos se rechaza antes de emitir.
				if tasa.NormalizarMoneda(pg.Moneda) == empresa.MonedaUSD {
					return fiscal.Documento{}, ErrSinTasa
				}
				return fiscal.Documento{}, fmt.Errorf("%w: %s", ErrSinTasaDivisa, pg.Moneda)
			}
			tasaPago = tp.Valor
			divisasEntregadas += pg.Monto * tasaPago
		}
		doc.Pagos = append(doc.Pagos, fiscal.Pago{
			Metodo: pg.Metodo, CuentaID: pg.CuentaID, Monto: pg.Monto, Moneda: pg.Moneda,
			EnDivisa: enDivisa, TasaCambio: tasaPago, Referencia: pg.Referencia,
		})
	}
	// El IGTF grava la porción de LA FACTURA pagada en divisas, no el efectivo
	// que el cliente puso sobre el mostrador: si paga un billete de 20 US$ por una
	// compra de 116 Bs, lo gravado son 116, no 2.000. El vuelto no es un pago en
	// divisas. Por eso la base se topa con el documento (base + IVA).
	//
	// El IGTF tampoco se grava a sí mismo: se calcula sobre el documento y se
	// suma después, que es como opera el comercio y como lo muestra el prototipo.
	docBase := round2(doc.Subtotal + doc.IVA)
	baseDivisas := divisasEntregadas
	if baseDivisas > docBase {
		baseDivisas = docBase
	}
	doc.IGTF = round2(baseDivisas * doc.AlicuotaIGTF)
	doc.Total = round2(doc.Subtotal + doc.IVA + doc.IGTF)

	// Arqueo del cobro: cuánto entregó el cliente y cuánto hay que devolverle.
	// Se calcula en el SERVIDOR, con la tasa del servidor: el vuelto es dinero
	// que sale de la gaveta, no un número que pinta la interfaz.
	var excedenteBs, vueltoTasaDefault float64
	var vueltoMonedaDefault string
	doc.Cobrado, excedenteBs, vueltoMonedaDefault, vueltoTasaDefault = arqueo(doc.Pagos, doc.Total)
	if excedenteBs > 0.004 {
		partes, err := s.resolverVueltoPartes(empresaID, in, excedenteBs, vueltoTasaDefault, vueltoMonedaDefault)
		if err != nil {
			return fiscal.Documento{}, err
		}
		doc.VueltoPartes = partes
		// Campos de RESUMEN retrocompatibles: con una sola parte se comportan igual
		// que antes (Vuelto en la moneda de esa parte); con vuelto MIXTO, Vuelto es
		// el total en Bs y la moneda/medio quedan vacíos (no hay una sola).
		if len(partes) == 1 {
			p := partes[0]
			doc.Vuelto = p.Monto
			doc.VueltoMoneda = p.Moneda
			doc.VueltoMetodo = p.Metodo
			doc.VueltoBanco, doc.VueltoCedula, doc.VueltoTelefono = p.Banco, p.Cedula, p.Telefono
		} else {
			doc.Vuelto = round2(excedenteBs)
		}
	}

	// Cliente (o consumidor final). Si la venta identifica un cliente, tiene que
	// existir y su documento debe ser un RIF/cédula VÁLIDO (requisito fiscal del
	// receptor): el documento se estampa en el comprobante. El consumidor final
	// (sin ClienteID) queda permitido para la venta minorista sin RIF.
	if in.ClienteID != "" {
		cl, ok := s.clientes.ByID(empresaID, in.ClienteID)
		if !ok {
			return fiscal.Documento{}, ErrClienteNoExiste
		}
		if err := fiscal.ValidarDocumento(cl.TipoDocumento, cl.Documento); err != nil {
			return fiscal.Documento{}, fmt.Errorf("%w: %v", ErrReceptorDocumentoInvalido, err)
		}
		// El domicilio fiscal es requisito de la factura, y se exige con el MISMO
		// criterio que el RIF: solo cuando hay un cliente identificado. Al
		// consumidor final no se le pide ni RIF ni dirección — sería trabar la
		// venta de mostrador, que es la mayoría del volumen de una bodega.
		if strings.TrimSpace(cl.Direccion) == "" {
			return fiscal.Documento{}, fmt.Errorf("%w: «%s»", ErrReceptorSinDireccion, cl.Nombre)
		}
		doc.ClienteID = cl.ID
		doc.ClienteNombre = cl.Nombre
		doc.ClienteDocumento = cl.TipoDocumento + "-" + cl.Documento
		doc.ClienteDireccion = strings.TrimSpace(cl.Direccion)
	}
	if doc.ClienteNombre == "" {
		doc.ClienteNombre = "Consumidor final"
	}

	if in.Credito {
		// Venta a crédito: el saldo queda por cobrar, con su fecha de vencimiento.
		if doc.ClienteID == "" {
			return fiscal.Documento{}, ErrCreditoSinCliente
		}
		if doc.Cobrado > doc.Total+0.005 {
			return fiscal.Documento{}, errors.New("un abono no puede ser mayor que el total de la factura")
		}
		dias := in.DiasCredito
		if dias <= 0 {
			dias = diasCreditoPorDefecto
		}
		doc.Credito = true
		doc.VenceEl = enDias(dias)
	} else if doc.Cobrado < doc.Total-0.005 {
		// Contado: lo cobrado tiene que cubrir el total, HAYA o no pagos. Una factura
		// no-crédito con cero pagos (Cobrado 0) también se rechaza: dejarla pasar
		// carga el total al Debe de 1102 (CxC) mientras la proyección de CxC la
		// excluye por !Credito, y queda un activo colgado invisible. Una venta a
		// crédito es la única vía legítima de emitir con saldo pendiente.
		return fiscal.Documento{}, fmt.Errorf("%w: faltan %.2f Bs", ErrCobroInsuficiente, doc.Total-doc.Cobrado)
	}

	// UN LOTE VENCIDO NO SE VENDE. Se comprueba ANTES de numerar y de anexar el
	// documento: la factura es de solo anexado y fallar después dejaría el folio
	// quemado y una venta a medias. Acá todavía no se ha tocado nada.
	sedeChequeo := sedeID
	if in.SedeID != "" {
		sedeChequeo = in.SedeID
	}
	if err := s.validarLotesVendibles(empresaID, sedeChequeo, doc.Lineas); err != nil {
		return fiscal.Documento{}, err
	}

	// Numeración fiscal serializada por empresa+sede+serie.
	doc.Serie = s.serieDoc(empresaID, fiscal.SerieFactura, modalidad)
	doc.Numero = s.numerador.Siguiente(empresaID, sedeID, doc.Serie)
	doc.NumeroCompleto = fmt.Sprintf("%s-%08d", doc.Serie, doc.Numero)
	doc.NumeroControl = s.numeroControl(empresaID, modalidad)

	out := s.documentos.Append(doc)

	// Evento DocumentoFiscalEmitido → descuenta inventario (salida en el ledger).
	// El costo de lo vendido se acumula del costo promedio vigente: es lo que va al
	// asiento de costo de ventas, no un porcentaje estimado.
	//
	// La salida sale del almacén de DESPACHO: por defecto la sede del documento,
	// pero si la entrada trae SedeID (p. ej. una cotización con sede de despacho
	// distinta), el inventario se descuenta de esa sede. El documento fiscal y su
	// costo promedio se leen de esa misma sede para no cruzar Kardex entre almacenes.
	sedeInventario := sedeID
	if in.SedeID != "" {
		sedeInventario = in.SedeID
	}
	// El stock sale del almacén de despacho: el indicado o el principal de la sede.
	almacenInventario := s.almacenParaEscritura(empresaID, sedeInventario, in.AlmacenID)
	costoVendido := 0.0
	for _, l := range out.Lineas {
		// Un plato descuenta sus INSUMOS (receta snapshot), no el plato; un producto
		// normal descuenta su propio SKU. consumosDeLinea unifica ambos casos.
		for _, cs := range consumosDeLinea(l, l.Cantidad) {
			_, avg := fold(s.movimientos.List(empresaID, inventario.FiltroMovimiento{SedeID: sedeInventario, SKU: cs.SKU}))
			costoVendido += avg * cs.Cantidad
			// Por LOTES (FEFO): sale antes lo que vence antes. El mostrador no elige
			// lote — si lo eligiera cada pantalla, la primera que se olvidara dejaría
			// la trazabilidad mintiendo sin que nada fallara.
			s.anexarSalidaPorLotes(inventario.Movimiento{
				EmpresaID: empresaID, SedeID: sedeInventario, AlmacenID: almacenInventario, ProductoID: cs.ProductoID, SKU: cs.SKU,
				Tipo: inventario.MovSalida, Cantidad: -cs.Cantidad, CostoUnitario: avg,
				Motivo: "venta " + out.NumeroCompleto, RefTipo: "documento", RefID: out.ID, Actor: actor, Fecha: ahora(),
			})
		}
	}
	// Asiento contable DERIVADO de la venta (principio 4: no se reconcilia, sale
	// del mismo hecho).
	s.asentarVenta(empresaID, actor, out, costoVendido)
	s.audit.Append(evento(empresaID, actor, origen, "fiscal.documento.emitir", out.NumeroCompleto, out.Tipo))

	// Consumo del cupón aplicado: sube UsosActuales para que el tope UsosMax se
	// aplique de verdad. Best-effort a propósito: la factura YA quedó registrada
	// (ledger append-only), así que un fallo acá (cupón borrado, repo caído) NO
	// puede tumbar la emisión; se ignora.
	if in.CuponCodigo != "" {
		_ = s.ConsumirCupon(empresaID, in.CuponCodigo)
	}
	return out, nil
}

// AnularDocumento emite una reversa total (tipo anulacion) que referencia a la
// factura y reingresa el inventario. Nunca edita el documento original.
// almacenesDeSalidaDoc mapea SKU→almacén de las salidas de inventario de un
// documento fiscal. Sirve para reingresar la anulación/NC en el MISMO almacén del
// que salió el stock (no siempre el principal, si la venta despachó de otro).
func (s *Service) almacenesDeSalidaDoc(empresaID, docID string) map[string]string {
	out := map[string]string{}
	for _, m := range s.movimientos.List(empresaID, inventario.FiltroMovimiento{}) {
		if m.RefTipo == "documento" && m.RefID == docID && m.Cantidad < 0 {
			out[m.SKU] = m.AlmacenID
		}
	}
	return out
}

func (s *Service) AnularDocumento(empresaID, sedeID, actor, origen, refID, motivo string) (fiscal.Documento, error) {
	orig, ok := s.documentos.ByID(empresaID, refID)
	if !ok || orig.Tipo != fiscal.TipoFactura {
		return fiscal.Documento{}, ErrDocumentoNoExiste
	}
	// La reversa (documento, numeración e inventario) va SIEMPRE en la sede del
	// documento ORIGINAL, no en la del header de quien anula: si no, el reingreso de
	// stock caería en otra sede (stock fantasma en ambas) y el Cierre Z por sede
	// descuadraría (la sede original nunca vería la anulación).
	sedeID = orig.SedeID
	for _, d := range s.documentos.List(empresaID) {
		if d.Tipo == fiscal.TipoAnulacion && d.RefDocumentoID == refID {
			return fiscal.Documento{}, ErrYaAnulado
		}
	}
	rev := fiscal.Documento{
		EmpresaID: empresaID, SedeID: sedeID, Tipo: fiscal.TipoAnulacion, Modalidad: orig.Modalidad,
		ClienteID: orig.ClienteID, ClienteNombre: orig.ClienteNombre, ClienteDocumento: orig.ClienteDocumento,
		ClienteDireccion: orig.ClienteDireccion,
		Lineas:           orig.Lineas, Subtotal: -orig.Subtotal, IVA: -orig.IVA, IGTF: -orig.IGTF, Total: -orig.Total,
		// Las dos bases también se revierten: los libros fiscales se cuadran por
		// base imponible y base exenta, no solo por el total.
		BaseImponible: -orig.BaseImponible, BaseExenta: -orig.BaseExenta,
		// La reversa hereda la tasa del original: revierte la misma operación,
		// con la misma conversión. Usar la tasa de hoy descuadraría el asiento.
		Moneda: orig.Moneda, TasaCambio: orig.TasaCambio, TasaFuente: orig.TasaFuente,
		// La reversa hereda las alícuotas del original por consistencia: revierte la
		// misma operación con las mismas tasas (los montos ya vienen negados).
		AlicuotaIVA: orig.AlicuotaIVA, AlicuotaIGTF: orig.AlicuotaIGTF,
		RefDocumentoID: refID, Motivo: motivo,
		Actor: actor, Fecha: ahora(),
	}
	rev.Serie = s.serieDoc(empresaID, fiscal.SerieAnulacion, orig.Modalidad)
	rev.Numero = s.numerador.Siguiente(empresaID, sedeID, rev.Serie)
	rev.NumeroCompleto = fmt.Sprintf("%s-%08d", rev.Serie, rev.Numero)
	rev.NumeroControl = s.numeroControl(empresaID, orig.Modalidad)
	out := s.documentos.Append(rev)

	// Reingresa el inventario (entrada por la anulación) en el MISMO almacén del que
	// salió cada línea (fallback: principal de la sede).
	almOrig := s.almacenesDeSalidaDoc(empresaID, orig.ID)
	costoReingresado := 0.0
	for _, l := range orig.Lineas {
		// Reingresa lo mismo que salió: los insumos del plato (snapshot en la línea)
		// o el propio SKU. Así el reingreso NO depende de que la receta del catálogo
		// siga igual que al vender.
		for _, cs := range consumosDeLinea(l, l.Cantidad) {
			_, avg := fold(s.movimientos.List(empresaID, inventario.FiltroMovimiento{SedeID: sedeID, SKU: cs.SKU}))
			costoReingresado += avg * cs.Cantidad
			almID := almOrig[cs.SKU]
			if almID == "" {
				almID = s.almacenParaEscritura(empresaID, sedeID, "")
			}
			s.movimientos.Append(inventario.Movimiento{
				EmpresaID: empresaID, SedeID: sedeID, AlmacenID: almID, ProductoID: cs.ProductoID, SKU: cs.SKU,
				Tipo: inventario.MovEntrada, Cantidad: cs.Cantidad, CostoUnitario: avg,
				Motivo: "anulación " + orig.NumeroCompleto, RefTipo: "documento", RefID: out.ID, Actor: actor, Fecha: ahora(),
			})
		}
	}
	// El asiento de la venta NO se toca: se anexa su contrario.
	s.asentarReversaFiscal(empresaID, actor, out, orig, costoReingresado)
	s.audit.Append(evento(empresaID, actor, origen, "fiscal.documento.anular", orig.NumeroCompleto, motivo))
	return out, nil
}

// EmitirNotaCredito emite una nota de crédito PARCIAL (tipo nota_credito) que
// referencia a una factura y acredita solo las cantidades indicadas: es la
// devolución/descuento legal, la versión parcial de AnularDocumento. Nunca edita
// la factura original (append-only) y reingresa al inventario solo lo devuelto.
func (s *Service) EmitirNotaCredito(empresaID, sedeID, actor, origen, refID, motivo string, lineas []LineaEntrada) (fiscal.Documento, error) {
	orig, ok := s.documentos.ByID(empresaID, refID)
	if !ok || orig.Tipo != fiscal.TipoFactura {
		return fiscal.Documento{}, ErrDocumentoNoExiste
	}
	// La NC (documento, numeración y reingreso de inventario) va en la sede del
	// documento ORIGINAL, no en la del header de quien la emite (ver AnularDocumento).
	sedeID = orig.SedeID
	// Una factura anulada ya no tiene efecto: no se le puede acreditar encima.
	// Misma derivación de «anulado» que usa Documentos.
	docs := s.documentos.List(empresaID)
	for _, d := range docs {
		if d.Tipo == fiscal.TipoAnulacion && d.RefDocumentoID == refID {
			return fiscal.Documento{}, ErrYaAnulado
		}
	}

	// Cantidad ya acreditada por notas de crédito previas de esta factura, por SKU.
	// Guard anti-sobre-crédito: nunca se puede acreditar más de lo facturado.
	yaAcreditado := map[string]float64{}
	for _, d := range docs {
		if d.Tipo != fiscal.TipoNotaCredito || d.RefDocumentoID != refID {
			continue
		}
		// SOLO cuentan las notas que DEVOLVIERON mercancía. Una nota por descuento
		// o por ajuste de precio no devolvió nada, así que no puede consumir el
		// cupo de una devolución real posterior.
		if !fiscal.NCDevuelveMercancia(d.MotivoCodigo) {
			continue
		}
		for _, l := range d.Lineas {
			yaAcreditado[l.SKU] += l.Cantidad
		}
	}
	// Índice de líneas de la factura original por SKU (precio y exención de origen).
	origPorSKU := map[string]fiscal.Linea{}
	for _, l := range orig.Lineas {
		origPorSKU[l.SKU] = l
	}

	nc := fiscal.Documento{
		EmpresaID: empresaID, SedeID: sedeID, Tipo: fiscal.TipoNotaCredito, Modalidad: orig.Modalidad,
		ClienteID: orig.ClienteID, ClienteNombre: orig.ClienteNombre, ClienteDocumento: orig.ClienteDocumento,
		ClienteDireccion: orig.ClienteDireccion,
		// La nota hereda la tasa del original: acredita la misma operación con la
		// misma conversión. Usar la tasa de hoy descuadraría el asiento.
		Moneda: orig.Moneda, TasaCambio: orig.TasaCambio, TasaFuente: orig.TasaFuente,
		// La nota hereda las alícuotas del original: acredita la misma operación con
		// las mismas tasas. Usar la config de hoy descuadraría el asiento.
		AlicuotaIVA: orig.AlicuotaIVA, AlicuotaIGTF: orig.AlicuotaIGTF,
		RefDocumentoID: refID, Motivo: motivo, MotivoCodigo: fiscal.MotivoNCDevolucion,
		Actor: actor, Fecha: ahora(),
	}

	var subtotal, baseImponible, baseExenta float64
	for _, in := range lineas {
		if in.Cantidad <= 0 {
			continue
		}
		ol, ok := origPorSKU[in.SKU]
		if !ok {
			return fiscal.Documento{}, fmt.Errorf("%w: %s", ErrNotaCreditoSKU, in.SKU)
		}
		disponible := round2(ol.Cantidad - yaAcreditado[in.SKU])
		if in.Cantidad > disponible+0.0005 {
			return fiscal.Documento{}, fmt.Errorf("%w: %s (máximo %.2f)", ErrNotaCreditoExcede, in.SKU, disponible)
		}
		// MISMO precio unitario de la línea original: no se recalcula contra catálogo.
		total := round2(ol.PrecioUnitario * in.Cantidad)
		nc.Lineas = append(nc.Lineas, fiscal.Linea{
			ProductoID: ol.ProductoID, SKU: ol.SKU, Nombre: ol.Nombre,
			Cantidad: in.Cantidad, PrecioUnitario: ol.PrecioUnitario, Total: total,
			Exento: ol.Exento,
			// Hereda el snapshot de receta del renglón original: el reingreso de la NC
			// devuelve los INSUMOS del plato (por la cantidad acreditada), no el plato.
			Insumos: ol.Insumos,
		})
		subtotal += total
		if ol.Exento {
			baseExenta += total
		} else {
			baseImponible += total
		}
	}
	if len(nc.Lineas) == 0 {
		return fiscal.Documento{}, ErrNotaCreditoVacia
	}
	// Montos NEGATIVOS (misma convención que AnularDocumento): la nota resta.
	// El IGTF no se acredita: es un impuesto sobre el medio de pago, no sobre la
	// mercancía devuelta; una devolución de bienes rebaja base + IVA.
	// Se redondea en positivo y se niega al final, la misma convención de
	// AnularDocumento: así el total y sus partes se redondean sobre las mismas
	// magnitudes y no puede aparecer un céntimo de diferencia entre ellos. (round2
	// ya redondea bien en ambos signos; esto es por coherencia del cálculo, no por
	// una limitación del redondeo.)
	// Tasa histórica: la del documento original, no la config de hoy. Es una
	// devolución que debe cuadrar con la factura que acredita. Fallback al default
	// del sistema para documentos previos a esta configuración (tasa 0).
	tasaIVA := orig.AlicuotaIVA
	if tasaIVA <= 0 {
		tasaIVA = fiscal.AlicuotaIVA
	}
	ivaPos := round2(baseImponible * tasaIVA)
	nc.Subtotal = -round2(subtotal)
	nc.BaseImponible = -round2(baseImponible)
	nc.BaseExenta = -round2(baseExenta)
	nc.IVA = -ivaPos
	nc.Total = -round2(subtotal + ivaPos)

	// Numeración de serie propia de notas de crédito.
	nc.Serie = s.serieDoc(empresaID, fiscal.SerieNotaCredito, orig.Modalidad)
	nc.Numero = s.numerador.Siguiente(empresaID, sedeID, nc.Serie)
	nc.NumeroCompleto = fmt.Sprintf("%s-%08d", nc.Serie, nc.Numero)
	nc.NumeroControl = s.numeroControl(empresaID, orig.Modalidad)
	out := s.documentos.Append(nc)

	// Reingresa al inventario SOLO lo devuelto (entrada parcial), en el MISMO almacén
	// del que salió cada línea (fallback: principal de la sede).
	almOrig := s.almacenesDeSalidaDoc(empresaID, orig.ID)
	costoReingresado := 0.0
	for _, l := range out.Lineas {
		// Reingresa los insumos del plato (o el SKU normal) por la cantidad acreditada.
		for _, cs := range consumosDeLinea(l, l.Cantidad) {
			_, avg := fold(s.movimientos.List(empresaID, inventario.FiltroMovimiento{SedeID: sedeID, SKU: cs.SKU}))
			costoReingresado += avg * cs.Cantidad
			almID := almOrig[cs.SKU]
			if almID == "" {
				almID = s.almacenParaEscritura(empresaID, sedeID, "")
			}
			s.movimientos.Append(inventario.Movimiento{
				EmpresaID: empresaID, SedeID: sedeID, AlmacenID: almID, ProductoID: cs.ProductoID, SKU: cs.SKU,
				Tipo: inventario.MovEntrada, Cantidad: cs.Cantidad, CostoUnitario: avg,
				Motivo: "nota de crédito " + out.NumeroCompleto, RefTipo: "documento", RefID: out.ID, Actor: actor, Fecha: ahora(),
			})
		}
	}
	// El asiento de la venta NO se toca: se anexa su contrario por la parte
	// acreditada. Se usa la propia nota como origen de los montos, re-positivados
	// (los guarda en negativo), igual que la reconstrucción de la contabilidad.
	//
	// La reversa debe distribuirse como estaba la factura ORIGINAL: la porción
	// cobrada se devuelve por Caja/Bancos (reembolso real) y la NO cobrada baja
	// Cuentas por Cobrar (el cliente ya no debe eso). Antes se ponía todo como
	// "cobrado" (reembolso en efectivo) aun en ventas a crédito, dejando CxC sin
	// tocar y Caja en negativo por un reembolso que nunca pasó. Se prorratea el
	// monto acreditado según la proporción cobrada del documento original.
	creditado := -out.Total
	cobradoProrrateado := creditado
	if orig.Total > 0 {
		cobradoOrig := orig.Cobrado
		if cobradoOrig > orig.Total {
			cobradoOrig = orig.Total
		}
		cobradoProrrateado = round2(creditado * cobradoOrig / orig.Total)
	}
	base := fiscal.Documento{
		NumeroCompleto: orig.NumeroCompleto,
		BaseImponible:  -out.BaseImponible, BaseExenta: -out.BaseExenta,
		IVA: -out.IVA, IGTF: -out.IGTF, Total: creditado, Cobrado: cobradoProrrateado,
	}
	s.asentarReversaFiscal(empresaID, actor, out, base, round2(costoReingresado))
	s.audit.Append(evento(empresaID, actor, origen, "fiscal.documento.nota_credito", orig.NumeroCompleto, motivo))
	return out, nil
}

// NotaDebitoEntrada son los datos de una nota de débito de cliente: un CARGO
// ADICIONAL (cargos, intereses de mora, corrección de precio al alza) sobre una
// factura ya emitida. A diferencia de la nota de crédito —que acredita líneas de
// la factura original— la nota de débito introduce un concepto NUEVO con su
// monto base; no está atado a los productos de la factura porque no devuelve ni
// re-vende mercancía, solo aumenta el valor a cobrar.
type NotaDebitoEntrada struct {
	// Concepto explica el cargo (queda como Nombre de la única línea y como Motivo
	// del documento). Obligatorio: un cargo sin explicación no es fiscalizable.
	Concepto string
	// Monto es la BASE del cargo, en la moneda del documento original (Bs). El IVA
	// se calcula encima con la alícuota histórica de la factura, salvo que el cargo
	// sea exento.
	Monto float64
	// Exento marca el cargo como NO gravado con IVA (p. ej. intereses de mora, que
	// no causan IVA). Por defecto el cargo es gravado, como un mayor precio.
	Exento bool
	// MotivoCodigo es el motivo del catálogo (fiscal.MotivosNotaDebito). La
	// contadora declaró que hoy las notas de débito son por DIFERENCIAL CAMBIARIO
	// o porque SE COBRÓ DE MENOS; tipificarlo permite declarar y auditar por
	// motivo en vez de leer texto libre. Vacío = sin tipificar (compatibilidad).
	MotivoCodigo string
	// Porcentaje (0–100) calcula el cargo como un % en vez de un monto fijo. Es
	// como se pacta de verdad —«5 % por mora», «el diferencial es 8 %»— y hacer
	// esa regla de tres a mano es justo donde se cuela el céntimo que después no
	// cuadra contra la factura. Se usa Monto O Porcentaje, nunca los dos.
	Porcentaje float64
	// SKU acota el porcentaje a UN renglón de la factura. Vacío = sobre el total.
	SKU string
}

// EmitirNotaDebito emite una nota de débito (tipo nota_debito) que referencia a
// una factura y AUMENTA el monto a cobrar por un cargo adicional. Es el ESPEJO
// POSITIVO de EmitirNotaCredito: en vez de acreditar líneas de la factura, carga
// un concepto nuevo con su IVA. Append-only, inmutable, con serie propia (-ND) y
// numeración atómica. No toca inventario (es un ajuste de valor, no mercancía).
func (s *Service) EmitirNotaDebito(empresaID, sedeID, actor, origen, refID string, in NotaDebitoEntrada) (fiscal.Documento, error) {
	orig, ok := s.documentos.ByID(empresaID, refID)
	if !ok || orig.Tipo != fiscal.TipoFactura {
		return fiscal.Documento{}, ErrDocumentoNoExiste
	}
	// La ND (documento y numeración) va en la sede del documento ORIGINAL, no en la
	// del header de quien la emite (ver AnularDocumento). La ND no mueve inventario.
	sedeID = orig.SedeID
	// Una factura anulada ya no tiene efecto: no se le puede cargar encima. Misma
	// derivación de «anulado» que usan Documentos y EmitirNotaCredito.
	for _, d := range s.documentos.List(empresaID) {
		if d.Tipo == fiscal.TipoAnulacion && d.RefDocumentoID == refID {
			return fiscal.Documento{}, ErrYaAnulado
		}
	}
	if strings.TrimSpace(in.Concepto) == "" {
		return fiscal.Documento{}, ErrNotaDebitoConcepto
	}
	if in.Porcentaje < 0 || in.Porcentaje > 100 {
		return fiscal.Documento{}, ErrNCPorcentaje
	}
	if in.Porcentaje > 0 && in.Monto > 0 {
		// Los dos a la vez: no se puede adivinar cuál quiso decir, y elegir por él
		// daría un cargo distinto del pactado.
		return fiscal.Documento{}, ErrNotaDebitoVacia
	}
	// El cargo se reparte entre gravado y exento igual que en la nota de crédito
	// (misma aritmética, signo opuesto). Sin tope: un cargo posterior SÍ puede
	// superar el renglón que lo originó.
	baseGravada, baseExenta, _, err := repartirBase(orig, in.SKU, in.Monto, in.Porcentaje, in.Exento)
	if err != nil {
		return fiscal.Documento{}, err
	}
	monto := round2(baseGravada + baseExenta)
	if monto <= 0.004 {
		return fiscal.Documento{}, ErrNotaDebitoVacia
	}
	// El motivo se acota al catálogo; uno inventado se descarta en vez de
	// guardarse, porque un código que no existe rompe cualquier consulta por
	// motivo sin que nadie lo note.
	motivoND := strings.TrimSpace(in.MotivoCodigo)
	if motivoND != "" && !fiscal.MotivoNDValido(motivoND) {
		return fiscal.Documento{}, ErrNotaDebitoConcepto
	}

	nd := fiscal.Documento{
		EmpresaID: empresaID, SedeID: sedeID, Tipo: fiscal.TipoNotaDebito, Modalidad: orig.Modalidad,
		ClienteID: orig.ClienteID, ClienteNombre: orig.ClienteNombre, ClienteDocumento: orig.ClienteDocumento,
		ClienteDireccion: orig.ClienteDireccion,
		// La nota hereda la tasa del original: carga sobre la misma operación con la
		// misma conversión. Usar la tasa de hoy descuadraría contra la factura.
		Moneda: orig.Moneda, TasaCambio: orig.TasaCambio, TasaFuente: orig.TasaFuente,
		// La nota hereda las alícuotas del original (ADR de tasa histórica): el cargo
		// se grava con la tasa vigente al emitir la factura, no con la config de hoy.
		AlicuotaIVA: orig.AlicuotaIVA, AlicuotaIGTF: orig.AlicuotaIGTF,
		RefDocumentoID: refID, Motivo: strings.TrimSpace(in.Concepto),
		MotivoCodigo: motivoND, Actor: actor, Fecha: ahora(),
	}

	// Montos POSITIVOS (espejo de la NC, que los guarda negativos): la nota de
	// débito suma. Un renglón por condición: si el cargo porcentual tocó base
	// gravada y exenta a la vez, van separados para que el IVA salga bien.
	etiqueta := strings.TrimSpace(in.Concepto)
	if in.Porcentaje > 0 {
		sobre := "sobre " + orig.NumeroCompleto
		if strings.TrimSpace(in.SKU) != "" {
			sobre = "sobre " + strings.TrimSpace(in.SKU)
		}
		etiqueta = fmt.Sprintf("%s (%.2f%% %s)", etiqueta, in.Porcentaje, sobre)
	}
	nd.Lineas = nil
	if baseGravada > 0 {
		nd.Lineas = append(nd.Lineas, fiscal.Linea{
			Nombre: etiqueta, Cantidad: 1, PrecioUnitario: baseGravada, Total: baseGravada,
		})
	}
	if baseExenta > 0 {
		nombre := etiqueta
		if baseGravada > 0 {
			nombre += " (exento)"
		}
		nd.Lineas = append(nd.Lineas, fiscal.Linea{
			Nombre: nombre, Cantidad: 1, PrecioUnitario: baseExenta, Total: baseExenta, Exento: true,
		})
	}
	nd.Subtotal = monto
	// Tasa histórica: la del documento original, con fallback al default del sistema
	// para facturas previas a la configuración de alícuotas (misma regla que la NC).
	tasaIVA := orig.AlicuotaIVA
	if tasaIVA <= 0 {
		tasaIVA = fiscal.AlicuotaIVA
	}
	nd.BaseImponible = baseGravada
	nd.BaseExenta = baseExenta
	nd.IVA = round2(baseGravada * tasaIVA)
	// El IGTF no aplica: no hay pago en divisas: la nota de débito es un ajuste de
	// valor a cobrar, no un cobro. Total = base + IVA.
	nd.Total = round2(nd.Subtotal + nd.IVA)

	// Numeración de serie propia de notas de débito (-ND), atómica por empresa+sede.
	nd.Serie = s.serieDoc(empresaID, fiscal.SerieNotaDebito, orig.Modalidad)
	nd.Numero = s.numerador.Siguiente(empresaID, sedeID, nd.Serie)
	nd.NumeroCompleto = fmt.Sprintf("%s-%08d", nd.Serie, nd.Numero)
	nd.NumeroControl = s.numeroControl(empresaID, orig.Modalidad)
	out := s.documentos.Append(nd)

	// Asiento DERIVADO: la nota de débito INCREMENTA lo que el cliente debe. Debe
	// Cuentas por cobrar (1102) / Haber ingresos (4101/4102) + IVA débito (2201).
	// No toca inventario (no hay mercancía). Cuadra por construcción.
	s.asentarNotaDebito(empresaID, actor, out)
	s.audit.Append(evento(empresaID, actor, origen, "fiscal.documento.nota_debito", orig.NumeroCompleto, out.Motivo))
	return out, nil
}

// alicuotaIVA resuelve la tasa de IVA vigente para una empresa: la que configuró
// si es > 0, si no el default del sistema (fiscal.AlicuotaIVA). Es la tasa que se
// aplica a documentos NUEVOS; los ya emitidos conservan la suya (ADR tasa
// histórica).
func (s *Service) alicuotaIVA(empresaID string) float64 {
	if e, ok := s.empresas.ByID(empresaID); ok && e.AlicuotaIVA > 0 {
		return e.AlicuotaIVA
	}
	return fiscal.AlicuotaIVA
}

// alicuotaIGTF resuelve la tasa de IGTF vigente para una empresa (config > 0, si
// no el default del sistema). Misma regla que alicuotaIVA.
func (s *Service) alicuotaIGTF(empresaID string) float64 {
	if e, ok := s.empresas.ByID(empresaID); ok && e.AlicuotaIGTF > 0 {
		return e.AlicuotaIGTF
	}
	return fiscal.AlicuotaIGTF
}

/* serieDoc resuelve el CÓDIGO DE SERIE con el que se numera un tipo de
 * documento. El prefijo configurado manda; sin configuración se deriva de la
 * modalidad, que es como numeraba antes de existir esta pantalla.
 *
 * La derivación como respaldo no es cortesía con el código viejo: es lo que hace
 * que una empresa que nunca abrió Configuración siga numerando igual después de
 * actualizar. Un correlativo que cambia solo es un problema fiscal, no un
 * detalle.
 */
func (s *Service) serieDoc(empresaID, tipo, modalidad string) string {
	if s.series != nil {
		if cfg, ok := s.series.Get(empresaID, tipo); ok && strings.TrimSpace(cfg.Prefijo) != "" {
			return strings.TrimSpace(cfg.Prefijo)
		}
	}
	return fiscal.CodigoModalidad(modalidad) + fiscal.SufijoSerie(tipo)
}

// serieControl es la serie del correlativo del Número de Control (por empresa).
const serieControl = "CTRL"

// prefijoNumeroControl resuelve el prefijo de 2 dígitos que acompaña al número
// de control. Vive en la empresa desde antes de la pantalla de correlativos; se
// lee de ahí para no tener dos fuentes de verdad del mismo dato.
func (s *Service) prefijoNumeroControl(empresaID string) string {
	if emp, ok := s.empresas.ByID(empresaID); ok && emp.NumeroControlPrefijo != "" {
		return emp.NumeroControlPrefijo
	}
	return "00"
}

// numeroControl asigna el "Número de Control" SENIAT: un correlativo PROPIO por
// empresa (formato NN-NNNNNNNN), distinto del número de factura. En MÁQUINA FISCAL
// lo asigna la impresora, así que aquí queda vacío.
func (s *Service) numeroControl(empresaID, modalidad string) string {
	if modalidad == "maquina_fiscal" {
		return ""
	}
	prefijo := s.prefijoNumeroControl(empresaID)
	seq := s.numerador.Siguiente(empresaID, "", serieControl)
	return fmt.Sprintf("%s-%08d", prefijo, seq)
}

// round2 redondea a dos decimales con la regla «medio hacia arriba en magnitud»
// (0,005 → 0,01 y −0,005 → −0,01), que es la de la contabilidad.
//
// El signo se trata aparte A PROPÓSITO. La conversión a int64 TRUNCA hacia cero,
// así que `int64(v*100+0.5)` solo redondea bien en positivo: aplicada a un
// negativo le come un céntimo de magnitud —round2(−5,00) devolvía −4,99—, y un
// céntimo perdido en una resta descuadra el asiento que la usa. Un asiento
// descuadrado NO SE GUARDA (ver asentarContrarioDe), así que el error no se veía
// como un importe raro sino como una operación entera sin contabilizar.
func round2(v float64) float64 {
	if v < 0 {
		return -float64(int64(-v*100+0.5)) / 100
	}
	return float64(int64(v*100+0.5)) / 100
}

// resolverVueltoPartes construye la lista de partes del vuelto y valida que su
// suma en bolívares cuadre con el excedente que el servidor calculó.
//
//   - Si la entrada trae VueltoPartes (vía nueva): cada parte se convierte con la
//     tasa de SU moneda (reusa TasaVigenteDeMoneda; VES ⇒ 1) y se valida que la
//     suma en Bs ≈ excedenteBs (tolerancia de centavos, escalada por la tasa de
//     cada divisa por el redondeo de su unidad mínima). El pago móvil por parte
//     exige banco/cédula/teléfono.
//   - Si no (retrocompat): una sola parte, con la moneda/medio declarados o el
//     default derivado del arqueo; el monto lo deriva el servidor del excedente.
func (s *Service) resolverVueltoPartes(empresaID string, in EmitirEntrada, excedenteBs, defTasa float64, defMoneda string) ([]fiscal.VueltoParte, error) {
	if len(in.VueltoPartes) > 0 {
		partes := make([]fiscal.VueltoParte, 0, len(in.VueltoPartes))
		sumaBs := 0.0
		tolerancia := 0.01
		for _, pe := range in.VueltoPartes {
			if pe.Monto <= 0 {
				continue // una parte sin monto no aporta nada.
			}
			moneda, tasaMoneda, err := s.tasaDeVuelto(empresaID, pe.Moneda)
			if err != nil {
				return nil, err
			}
			metodo := strings.TrimSpace(pe.Metodo)
			if metodo == "" {
				metodo = fiscal.VueltoEfectivo
			}
			if metodo != fiscal.VueltoEfectivo && metodo != fiscal.VueltoPagoMovil {
				return nil, ErrVueltoMetodoInvalido
			}
			vp := fiscal.VueltoParte{
				Moneda: moneda, Metodo: metodo,
				Monto: round2(pe.Monto), MontoBs: round2(pe.Monto * tasaMoneda),
			}
			if metodo == fiscal.VueltoPagoMovil {
				banco := strings.TrimSpace(pe.Banco)
				cedula := strings.TrimSpace(pe.Cedula)
				telefono := strings.TrimSpace(pe.Telefono)
				if banco == "" || cedula == "" || telefono == "" {
					return nil, ErrVueltoPagoMovilDatos
				}
				vp.Banco, vp.Cedula, vp.Telefono = banco, cedula, telefono
			}
			sumaBs = round2(sumaBs + vp.MontoBs)
			// El redondeo de la unidad mínima de cada moneda (un céntimo de esa
			// divisa) puede desviar la suma en Bs: se tolera esa deriva por parte.
			tolerancia += 0.01 * tasaMoneda
			partes = append(partes, vp)
		}
		if len(partes) == 0 {
			return nil, ErrVueltoNoCuadra
		}
		if math.Abs(sumaBs-excedenteBs) > tolerancia {
			return nil, ErrVueltoNoCuadra
		}
		return partes, nil
	}

	// Retrocompat: una sola parte. La caja DECLARA la moneda del vuelto (o se usa
	// el default derivado del arqueo) y el medio; el monto lo deriva el servidor.
	moneda, tasaMoneda := defMoneda, defTasa
	if in.VueltoMoneda != "" {
		m, t, err := s.tasaDeVuelto(empresaID, in.VueltoMoneda)
		if err != nil {
			return nil, err
		}
		moneda, tasaMoneda = m, t
	}
	if tasaMoneda <= 0 {
		tasaMoneda = 1
	}
	metodo := strings.TrimSpace(in.VueltoMetodo)
	if metodo == "" {
		metodo = fiscal.VueltoEfectivo
	}
	vp := fiscal.VueltoParte{
		Moneda: moneda, Metodo: metodo,
		Monto: round2(excedenteBs / tasaMoneda), MontoBs: round2(excedenteBs),
	}
	if metodo == fiscal.VueltoPagoMovil {
		banco := strings.TrimSpace(in.VueltoBanco)
		cedula := strings.TrimSpace(in.VueltoCedula)
		telefono := strings.TrimSpace(in.VueltoTelefono)
		if banco == "" || cedula == "" || telefono == "" {
			return nil, ErrVueltoPagoMovilDatos
		}
		vp.Banco, vp.Cedula, vp.Telefono = banco, cedula, telefono
	}
	return []fiscal.VueltoParte{vp}, nil
}

// tasaDeVuelto resuelve la moneda normalizada y su tasa (Bs por unidad) para una
// parte del vuelto: VES ⇒ 1; una divisa debe estar ACTIVA y con tasa cargada
// (si no, no se puede convertir y se rechaza).
func (s *Service) tasaDeVuelto(empresaID, moneda string) (string, float64, error) {
	m := strings.ToUpper(strings.TrimSpace(moneda))
	if m == "" || m == empresa.MonedaVES {
		return empresa.MonedaVES, 1, nil
	}
	t, ok := s.TasaVigenteDeMoneda(empresaID, m)
	if !ok || t.Valor <= 0 {
		return "", 0, fmt.Errorf("%w: %s", ErrVueltoMonedaInvalida, m)
	}
	return tasa.NormalizarMoneda(m), t.Valor, nil
}

// partesDeVuelto normaliza el vuelto de un documento a una lista de partes, sea
// que traiga la lista nueva (VueltoPartes) o solo los campos únicos de resumen
// (documentos anteriores a la lista). Es el plegado que usan el arqueo de caja y
// los saldos de tesorería para no divergir en la lectura del vuelto.
func partesDeVuelto(d fiscal.Documento) []fiscal.VueltoParte {
	if len(d.VueltoPartes) > 0 {
		return d.VueltoPartes
	}
	if d.Vuelto <= 0.004 {
		return nil
	}
	metodo := d.VueltoMetodo
	if metodo == "" {
		metodo = fiscal.VueltoEfectivo
	}
	moneda := d.VueltoMoneda
	if moneda == "" {
		moneda = empresa.MonedaVES
	}
	montoBs := d.Vuelto
	if moneda != empresa.MonedaVES && d.TasaCambio > 0 {
		montoBs = round2(d.Vuelto * d.TasaCambio)
	}
	return []fiscal.VueltoParte{{
		Moneda: moneda, Metodo: metodo, Monto: d.Vuelto, MontoBs: montoBs,
		Banco: d.VueltoBanco, Cedula: d.VueltoCedula, Telefono: d.VueltoTelefono,
	}}
}

/* --- Maestro de impuestos: resolución y desglose --------------------------- */

// alicuotaDeProducto resuelve qué alícuota le toca a un producto EN UNA FECHA.
//
// Cae con gracia, en este orden: el código del producto contra el maestro
// vigente ese día; si el producto no tiene código (catálogo anterior al
// maestro), su viejo booleano `ExentoIVA`; y si tampoco hay maestro cargado, la
// tasa configurada de la empresa. Ninguna empresa puede quedarse sin poder
// facturar porque el maestro no esté sembrado.
func (s *Service) alicuotaDeProducto(empresaID string, p inventario.Producto, fecha string) fiscal.Alicuota {
	codigo := strings.TrimSpace(p.AlicuotaCodigo)
	if codigo == "" {
		// Catálogo viejo: el booleano sigue mandando.
		if p.ExentoIVA {
			codigo = fiscal.CodExento
		} else {
			codigo = fiscal.CodGeneral
		}
	}
	if s.alicuotas != nil {
		if a, ok := fiscal.VigenteEn(s.alicuotas.List(empresaID), codigo, fecha); ok && a.Activa {
			return a
		}
	}
	// Sin maestro: se reconstruye la clasificación mínima con la tasa de la
	// empresa, que es exactamente como se comportaba antes de todo esto.
	if codigo == fiscal.CodExento {
		return fiscal.Alicuota{Codigo: fiscal.CodExento, Nombre: "Exento", Tipo: fiscal.TipoExento, Activa: true}
	}
	return fiscal.Alicuota{
		Codigo: fiscal.CodGeneral, Nombre: "General", Tipo: fiscal.TipoGeneral,
		Porcentaje: s.alicuotaIVA(empresaID), Activa: true,
	}
}

// acumularImpuesto suma una base al desglose. Una alícuota suntuaria produce DOS
// filas sobre la MISMA base —la general y su recargo—, porque el libro de ventas
// las declara en columnas separadas y guardarlas juntas como un 31% haría
// imposible llenarlo.
func acumularImpuesto(desglose map[string]*fiscal.DocumentoImpuesto, orden *[]string, al fiscal.Alicuota, base float64) {
	sumar := func(clave, nombre, tipo string, pct float64) {
		f, ok := desglose[clave]
		if !ok {
			f = &fiscal.DocumentoImpuesto{Codigo: al.Codigo, Nombre: nombre, Tipo: tipo, Porcentaje: pct}
			desglose[clave] = f
			*orden = append(*orden, clave)
		}
		f.Base += base
	}
	if !al.Grava() {
		sumar(al.Codigo+"|exento", al.Nombre, fiscal.TipoExento, 0)
		return
	}
	sumar(al.Codigo+"|base", al.Nombre, al.Tipo, al.Porcentaje)
	if al.Adicional > 0 {
		sumar(al.Codigo+"|adicional", al.Nombre+" · adicional", fiscal.TipoAdicional, al.Adicional)
	}
}

// cerrarDesglose redondea cada fila y devuelve el total de impuesto. El monto se
// calcula POR FILA y después se suma: redondear al final sobre una base mezclada
// daría un céntimo distinto del que declara el libro, y el asiento de venta —que
// arma el Haber con estas cifras— descuadraría.
func cerrarDesglose(desglose map[string]*fiscal.DocumentoImpuesto, orden []string) ([]fiscal.DocumentoImpuesto, float64) {
	out := make([]fiscal.DocumentoImpuesto, 0, len(orden))
	var total float64
	for _, k := range orden {
		f := desglose[k]
		f.Base = round2(f.Base)
		f.Monto = round2(f.Base * f.Porcentaje)
		total = round2(total + f.Monto)
		out = append(out, *f)
	}
	return out, total
}

// ConSeries cablea la configuración de correlativos. Opcional: sin ella cada
// tipo numera con el prefijo derivado de la modalidad, como antes.
func (s *Service) ConSeries(r fiscal.SerieRepository) *Service {
	s.series = r
	return s
}

// ConAlicuotas cablea el maestro de impuestos. Opcional: sin él el motor usa la
// tasa única configurada en la empresa, que es como funcionaba antes.
func (s *Service) ConAlicuotas(r fiscal.AlicuotaRepo) *Service {
	s.alicuotas = r
	return s
}

// Alicuotas devuelve el maestro de impuestos de la empresa.
func (s *Service) Alicuotas(empresaID string) []fiscal.Alicuota {
	if s.alicuotas == nil {
		return []fiscal.Alicuota{}
	}
	return s.alicuotas.List(empresaID)
}
