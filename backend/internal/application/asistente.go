package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mornix/elerp/internal/domain/inventario"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// Asistente del ERP en dos capas (ver Documentos + plan del módulo "asistente-ia"):
//
//  1. Capa MECÁNICA (esta): 100% local, determinista y gratis. Un router de intents
//     clasifica la pregunta y responde con los MISMOS reportes derivados del ledger
//     que ve la app (PanelEjecutivo, ReporteVentas, ReporteCobranza, CuentasPorPagar,
//     ReporteInventario, Balance, VerificarIntegridad) más respuestas de AYUDA fijas
//     sobre el producto. Ningún dato sale de la instancia. Todo se acota al ROL: el
//     asistente nunca expone lo que el rol no vería en la interfaz.
//  2. Capa IA (opt-in, ver ResponderMecanica → si no hay match): solo si la empresa la
//     habilita, la pregunta abierta va al proxy de IA con un contexto acotado al rol.
//
// La IA NUNCA es la fuente de una cifra: los números salen de acá (del ledger); la IA
// solo orienta sobre el contexto que se le entrega.

// Tipos de respuesta del asistente.
const (
	RespMecanica     = "mecanica"
	RespIA           = "ia"
	RespSinRespuesta = "sin_respuesta"
)

// IAProxy es el puerto (hexagonal) de la capa de IA: dado un system prompt (que ya
// incluye el contexto acotado al rol) y el mensaje del usuario, devuelve la respuesta
// del modelo. El adapter (adapter/hubmy) lo implementa envolviendo el proxy de IA de
// Hubmy. La aplicación no conoce al proveedor concreto: solo este contrato.
type IAProxy interface {
	Chat(ctx context.Context, modelo, system, usuario string) (string, error)
}

// ConIA cablea el proxy de IA (opcional, como ConFuentesDeTasa). Sin él, la capa IA no
// existe y el asistente responde solo con la capa mecánica.
func (s *Service) ConIA(proxy IAProxy) *Service {
	s.ia = proxy
	return s
}

// ModeloIADefault es el modelo usado cuando la empresa no fija uno (barato y rápido).
const ModeloIADefault = "google/gemini-2.5-flash-lite"

// ErrIANoDisponible se devuelve cuando se pide una respuesta de IA pero la empresa no
// la habilitó o el proxy no está configurado.
var ErrIANoDisponible = errors.New("la capa de IA no está habilitada para esta empresa")

// ResponderIA arma el contexto acotado al rol + system prompt estricto y consulta al
// proxy. Solo procede si la empresa habilitó la IA y hay proxy cableado; si no,
// devuelve ErrIANoDisponible (el llamador entrega entonces una respuesta "sin_respuesta").
func (s *Service) ResponderIA(ctx context.Context, empresaID, rol, pregunta string) (RespuestaAsistente, error) {
	if s.ia == nil {
		return RespuestaAsistente{}, ErrIANoDisponible
	}
	e, ok := s.empresas.ByID(empresaID)
	if !ok || !e.AsistenteIAHabilitada {
		return RespuestaAsistente{}, ErrIANoDisponible
	}
	modelo := e.AsistenteIAModelo
	if modelo == "" {
		modelo = ModeloIADefault
	}
	system := s.ContextoIA(empresaID, rol)
	out, err := s.ia.Chat(ctx, modelo, system, pregunta)
	if err != nil {
		return RespuestaAsistente{}, err
	}
	return RespuestaAsistente{Tipo: RespIA, Respuesta: strings.TrimSpace(out), Modelo: modelo}, nil
}

// AsistenteConfig es la configuración de la capa IA de una empresa (para la pantalla
// de Configuración). No expone secretos: solo el estado opt-in y el modelo elegido.
type AsistenteConfig struct {
	Habilitada    bool   `json:"habilitada"`
	Modelo        string `json:"modelo"`
	ModeloDefault string `json:"modeloDefault"`
	// IADisponible indica si el servidor tiene un proxy de IA cableado (HUBMY_API_KEY).
	// Si es false, aunque se habilite no habrá inferencia (solo capa mecánica).
	IADisponible bool `json:"iaDisponible"`
}

// AsistenteConfigDe devuelve la configuración de la capa IA de la empresa.
func (s *Service) AsistenteConfigDe(empresaID string) AsistenteConfig {
	cfg := AsistenteConfig{ModeloDefault: ModeloIADefault, IADisponible: s.ia != nil}
	if e, ok := s.empresas.ByID(empresaID); ok {
		cfg.Habilitada = e.AsistenteIAHabilitada
		cfg.Modelo = e.AsistenteIAModelo
		if cfg.Modelo == "" {
			cfg.Modelo = ModeloIADefault
		}
	}
	return cfg
}

// ConfigurarAsistente actualiza el opt-in de IA y el modelo de una empresa (Dueña/Dev).
func (s *Service) ConfigurarAsistente(empresaID, actor, origen string, habilitada bool, modelo string) (AsistenteConfig, error) {
	e, ok := s.empresas.ByID(empresaID)
	if !ok {
		return AsistenteConfig{}, ErrEmpresaNoExiste
	}
	e.AsistenteIAHabilitada = habilitada
	e.AsistenteIAModelo = strings.TrimSpace(modelo)
	if _, ok := s.empresas.Update(e); !ok {
		return AsistenteConfig{}, ErrEmpresaNoExiste
	}
	if s.audit != nil {
		estado := "desactivada"
		if habilitada {
			estado = "activada"
		}
		s.audit.Append(evento(empresaID, actor, origen, "asistente.ia.configurar", empresaID, "IA "+estado))
	}
	return s.AsistenteConfigDe(empresaID), nil
}

// RespuestaAsistente es lo que devuelve el asistente al cliente.
type RespuestaAsistente struct {
	Tipo      string `json:"tipo"`             // mecanica | ia | sin_respuesta
	Respuesta string `json:"respuesta"`        // texto para el usuario
	Fuente    string `json:"fuente,omitempty"` // reporte/dato que originó la respuesta mecánica
	Modelo    string `json:"modelo,omitempty"` // modelo de IA usado (solo tipo ia)
}

// ResponderMecanica intenta responder la pregunta con la capa mecánica (datos +
// ayuda), acotada al rol. Devuelve ok=false SOLO cuando ningún intent aplica (ahí el
// llamador decide si escala a la capa IA). Un intent que aplica pero el rol no puede
// consultar devuelve una respuesta mecánica de "sin acceso" (ok=true): es una
// respuesta definitiva, no algo que deba ir a la IA.
func (s *Service) ResponderMecanica(empresaID, rol, pregunta string) (RespuestaAsistente, bool) {
	q := normalizarPregunta(pregunta)
	if q == "" {
		return RespuestaAsistente{}, false
	}

	// --- Intents de AYUDA (texto fijo del producto, sin datos; todos los roles). ---
	if contiene(q, "igtf") {
		return ayuda(textoIGTF), true
	}
	if contiene(q, "transf") && contiene(q, "stock", "sede", "almac", "deposito", "inventario") {
		return ayuda(textoTransferir), true
	}
	if contiene(q, "forma libre") {
		return ayuda(textoFormaLibre), true
	}
	if contiene(q, "numero de control") {
		return ayuda(textoNumeroControl), true
	}
	if contiene(q, "retencion") && !contiene(q, "vend", "venta") {
		return ayuda(textoRetencion), true
	}

	// --- Intents de DATOS (derivados del ledger, acotados al rol). ---

	// Integridad del libro / "¿cuadra?".
	if contiene(q, "integridad", "sello", "manipul") || (contiene(q, "cuadra") && contiene(q, "libro", "contabilidad", "diario", "balance")) {
		if !puedeConsultar(rol, "contabilidad") {
			return sinAcceso("la verificación de integridad contable"), true
		}
		return s.respIntegridad(empresaID), true
	}

	// Cuentas por pagar (proveedores). Debe ir ANTES que cobranza para captar "pagar".
	if contiene(q, "por pagar", "cuanto debo", "le debo", "debemos", "deuda con proveedor") ||
		(contiene(q, "proveedor") && contiene(q, "deb", "pag", "saldo")) {
		if !puedeConsultar(rol, "finanzas") {
			return sinAcceso("las cuentas por pagar"), true
		}
		return s.respPorPagar(empresaID), true
	}

	// Cobranza / cartera (clientes que deben).
	if contiene(q, "por cobrar", "cartera", "me deb", "quien debe", "quien me debe", "vencid", "cobranza", "deuda de cliente", "clientes con deuda") {
		if !puedeConsultar(rol, "cobranza") {
			return sinAcceso("la cartera por cobrar"), true
		}
		return s.respCobranza(empresaID), true
	}

	// IVA / impuestos del mes.
	if contiene(q, "iva", "impuesto", "debito fiscal") {
		if !puedeConsultar(rol, "contabilidad") {
			return sinAcceso("los impuestos del período"), true
		}
		return s.respImpuestos(empresaID), true
	}

	// Stock / existencias. "stock de <producto>" vs. panorama (bajo mínimo/agotados).
	if contiene(q, "stock", "existencia", "inventario", "agotad", "bajo minimo", "cuanto queda", "cuanto tengo", "cuantas unidades") {
		if !puedeConsultar(rol, "inventario") {
			return sinAcceso("el inventario"), true
		}
		if ent := extraerEntidad(q); ent != "" && contiene(q, "stock de", "existencia de", "queda de", "tengo de", "unidades de", "hay de") {
			return s.respStockProducto(empresaID, ent), true
		}
		return s.respInventario(empresaID), true
	}

	// Ventas (hoy | este mes). El cajero también puede (factura en el mostrador).
	if contiene(q, "vend", "venta", "factur", "ingreso") {
		if !puedeConsultar(rol, "ventas") {
			return sinAcceso("las ventas"), true
		}
		return s.respVentas(empresaID, q), true
	}

	// Resumen / "¿cómo va el negocio?" — panel ejecutivo condensado.
	if contiene(q, "resumen", "como va", "como vamos", "panorama", "panel", "como esta el negocio", "situacion") {
		if !puedeConsultar(rol, "contabilidad") {
			// Un vendedor recibe un resumen acotado (sus ventas del mes).
			if puedeConsultar(rol, "ventas") {
				return s.respVentas(empresaID, "mes"), true
			}
			return sinAcceso("el resumen del negocio"), true
		}
		return s.respResumen(empresaID), true
	}

	return RespuestaAsistente{}, false
}

/* --- Handlers de datos (cada uno arma su respuesta desde un reporte) -------- */

func (s *Service) respVentas(empresaID, q string) RespuestaAsistente {
	// "hoy"/"día" ⇒ solo hoy; por defecto, el mes en curso.
	if contiene(q, "hoy", " dia", "del dia") {
		hoy := hoyVE()
		r := s.ReporteVentas(empresaID, hoy, hoy)
		txt := fmt.Sprintf("Hoy llevas Bs %s en ventas netas, en %d factura(s).", fmtBs(r.VentasNetas), r.CantidadFacturas)
		if r.CantidadFacturas > 0 {
			txt += fmt.Sprintf(" Ticket promedio: Bs %s.", fmtBs(r.TicketPromedio))
		}
		return RespuestaAsistente{Tipo: RespMecanica, Respuesta: txt, Fuente: "Ventas de hoy"}
	}
	desde, hasta := rangoMesActual()
	r := s.ReporteVentas(empresaID, desde, hasta)
	txt := fmt.Sprintf("Este mes llevas Bs %s en ventas netas (%d facturas, ticket promedio Bs %s).",
		fmtBs(r.VentasNetas), r.CantidadFacturas, fmtBs(r.TicketPromedio))
	if len(r.PorProducto) > 0 {
		txt += fmt.Sprintf(" Lo más vendido: %s.", r.PorProducto[0].Nombre)
	}
	return RespuestaAsistente{Tipo: RespMecanica, Respuesta: txt, Fuente: "Ventas del mes"}
}

func (s *Service) respCobranza(empresaID string) RespuestaAsistente {
	r := s.ReporteCobranza(empresaID)
	if r.TotalPorCobrar <= 0 {
		return RespuestaAsistente{Tipo: RespMecanica, Respuesta: "No tienes cuentas por cobrar pendientes. Tu cartera está en cero.", Fuente: "Cobranza"}
	}
	txt := fmt.Sprintf("Tus clientes te deben Bs %s en total", fmtBs(r.TotalPorCobrar))
	if r.TotalVencido > 0 {
		txt += fmt.Sprintf(", de los cuales Bs %s ya están VENCIDOS", fmtBs(r.TotalVencido))
	}
	txt += "."
	// Añade el tramo más viejo con saldo, si existe.
	for i := len(r.Aging) - 1; i >= 0; i-- {
		if r.Aging[i].Monto > 0 {
			txt += fmt.Sprintf(" El tramo %s días acumula Bs %s en %d documento(s).", r.Aging[i].Tramo, fmtBs(r.Aging[i].Monto), r.Aging[i].Documentos)
			break
		}
	}
	return RespuestaAsistente{Tipo: RespMecanica, Respuesta: txt, Fuente: "Cobranza"}
}

func (s *Service) respPorPagar(empresaID string) RespuestaAsistente {
	r := s.CuentasPorPagar(empresaID)
	if r.TotalPorPagar <= 0 {
		return RespuestaAsistente{Tipo: RespMecanica, Respuesta: "No tienes cuentas por pagar pendientes con proveedores.", Fuente: "Cuentas por pagar"}
	}
	txt := fmt.Sprintf("Debes Bs %s a %d proveedor(es) (%d orden(es) con saldo).",
		fmtBs(r.TotalPorPagar), r.ProveedoresConSaldo, r.OrdenesConSaldo)
	// Proveedor con mayor saldo (la lista viene ordenada de mayor a menor).
	for _, p := range r.Proveedores {
		if p.Saldo > 0 {
			txt += fmt.Sprintf(" A quien más le debes: %s (Bs %s).", p.ProveedorNombre, fmtBs(p.Saldo))
			break
		}
	}
	return RespuestaAsistente{Tipo: RespMecanica, Respuesta: txt, Fuente: "Cuentas por pagar"}
}

func (s *Service) respInventario(empresaID string) RespuestaAsistente {
	r := s.ReporteInventario(empresaID)
	txt := fmt.Sprintf("Tienes %d producto(s) por Bs %s de valorización total.", r.NumeroProductos, fmtBs(r.ValorizacionTotal))
	switch {
	case r.Agotados == 0 && r.BajoMinimo == 0:
		txt += " Ninguno está agotado ni bajo mínimo."
	default:
		txt += fmt.Sprintf(" %d agotado(s) y %d bajo mínimo.", r.Agotados, r.BajoMinimo)
		// Lista hasta 3 con menor stock positivo (bajo mínimo).
		bajos := make([]InventarioValorItem, 0)
		for _, it := range r.TopPorValor {
			if it.Cantidad > 0 && it.Cantidad <= UmbralStockBajo {
				bajos = append(bajos, it)
			}
		}
		sort.Slice(bajos, func(i, j int) bool { return bajos[i].Cantidad < bajos[j].Cantidad })
		if len(bajos) > 0 {
			nombres := make([]string, 0, 3)
			for i, it := range bajos {
				if i == 3 {
					break
				}
				nombres = append(nombres, fmt.Sprintf("%s (%s)", it.Nombre, fmtCant(it.Cantidad)))
			}
			txt += " Atención: " + strings.Join(nombres, ", ") + "."
		}
	}
	return RespuestaAsistente{Tipo: RespMecanica, Respuesta: txt, Fuente: "Inventario"}
}

func (s *Service) respStockProducto(empresaID, nombre string) RespuestaAsistente {
	p, ok := s.buscarProducto(empresaID, nombre)
	if !ok {
		return RespuestaAsistente{Tipo: RespMecanica, Respuesta: fmt.Sprintf("No encontré un producto que coincida con «%s». Revisa el nombre o el SKU en el Catálogo.", nombre), Fuente: "Inventario"}
	}
	if p.EsCombo {
		return RespuestaAsistente{Tipo: RespMecanica, Respuesta: fmt.Sprintf("«%s» es un combo: no lleva stock propio, depende de sus componentes.", p.Nombre), Fuente: "Inventario"}
	}
	cant, avg := s.stockTotal(empresaID, p.ID)
	txt := fmt.Sprintf("Quedan %s de «%s» (SKU %s) en toda la empresa.", fmtCant(cant), p.Nombre, p.SKU)
	if avg > 0 {
		txt += fmt.Sprintf(" Costo promedio: Bs %s; valor en stock: Bs %s.", fmtBs(avg), fmtBs(cant*avg))
	}
	if cant <= 0 {
		txt += " Está AGOTADO."
	} else if cant <= UmbralStockBajo {
		txt += " Está por debajo del mínimo."
	}
	return RespuestaAsistente{Tipo: RespMecanica, Respuesta: txt, Fuente: "Inventario"}
}

func (s *Service) respImpuestos(empresaID string) RespuestaAsistente {
	desde, hasta := rangoMesActual()
	r := s.ReporteVentas(empresaID, desde, hasta)
	txt := fmt.Sprintf("En el mes en curso el IVA débito de tus ventas suma Bs %s (%d facturas, Bs %s de base neta).",
		fmtBs(r.IVADebito), r.CantidadFacturas, fmtBs(r.VentasNetas))
	if r.IGTF > 0 {
		txt += fmt.Sprintf(" IGTF cobrado: Bs %s.", fmtBs(r.IGTF))
	}
	txt += " Es el débito fiscal; la declaración resta el crédito fiscal de tus compras."
	return RespuestaAsistente{Tipo: RespMecanica, Respuesta: txt, Fuente: "Impuestos del mes"}
}

func (s *Service) respIntegridad(empresaID string) RespuestaAsistente {
	sellos := s.VerificarIntegridad(empresaID)
	bal := s.Balance(empresaID)
	var partes []string
	todoIntegro := true
	for _, r := range sellos {
		if r.Integro {
			partes = append(partes, fmt.Sprintf("el libro de %s está íntegro (%d/%d sellados)", r.Libro, r.Sellados, r.Total))
		} else {
			todoIntegro = false
			partes = append(partes, fmt.Sprintf("⚠ el libro de %s tiene la cadena ROTA en %s", r.Libro, r.RotoEn))
		}
	}
	txt := strings.Join(partes, "; ") + "."
	if bal.Cuadra {
		txt = "El balance de comprobación CUADRA (debe = haber). " + strings.ToUpper(txt[:1]) + txt[1:]
	} else {
		todoIntegro = false
		txt = fmt.Sprintf("⚠ El balance NO cuadra: debe Bs %s vs. haber Bs %s. ", fmtBs(bal.TotalDebe), fmtBs(bal.TotalHaber)) + txt
	}
	if todoIntegro {
		txt += " Todo en orden para una auditoría."
	}
	return RespuestaAsistente{Tipo: RespMecanica, Respuesta: txt, Fuente: "Integridad"}
}

func (s *Service) respResumen(empresaID string) RespuestaAsistente {
	p := s.PanelEjecutivo(empresaID)
	txt := fmt.Sprintf("Este mes: Bs %s en ventas netas (%d facturas). Por cobrar: Bs %s (Bs %s vencido). Inventario: Bs %s en %d productos (%d agotados). Órdenes de compra abiertas: %d.",
		fmtBs(p.Ventas.VentasNetas), p.Ventas.CantidadFacturas,
		fmtBs(p.TotalPorCobrar), fmtBs(p.TotalVencido),
		fmtBs(p.Inventario.ValorizacionTotal), p.Inventario.NumeroProductos, p.Inventario.Agotados,
		p.OrdenesAbiertas)
	return RespuestaAsistente{Tipo: RespMecanica, Respuesta: txt, Fuente: "Panel ejecutivo"}
}

/* --- Contexto para la capa IA (opt-in) ------------------------------------- */

// ContextoIA arma el system prompt de la capa IA: identidad + reglas estrictas (solo
// el contexto dado, sin internet ni herramientas) + un SNAPSHOT compacto de la
// instancia ACOTADO AL ROL. Solo se usa si la empresa habilitó la IA. Devuelve el
// texto completo listo para enviar como mensaje "system".
func (s *Service) ContextoIA(empresaID, rol string) string {
	nombre := "la empresa"
	if e, ok := s.empresas.ByID(empresaID); ok && e.Nombre != "" {
		nombre = e.Nombre
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Eres el asistente de ElERP para «%s», un ERP venezolano. ", nombre)
	b.WriteString("Respondes SIEMPRE en español, claro y breve. ")
	b.WriteString("Responde ÚNICAMENTE con el CONTEXTO y el conocimiento del producto que se te da abajo. ")
	b.WriteString("Si la respuesta no está en el contexto, dilo con honestidad y sugiere dónde mirarla en la app; NO inventes cifras ni datos. ")
	b.WriteString("No tienes acceso a internet ni a ninguna herramienta externa. ")
	b.WriteString("Las cifras fiscales y contables SIEMPRE provienen del sistema, no de ti: no las recalcules ni las estimes.\n\n")
	b.WriteString("CONTEXTO ACTUAL (acotado al rol del usuario):\n")

	// Snapshot de datos, solo lo que el rol puede ver (espejo de la capa mecánica).
	if puedeConsultar(rol, "ventas") {
		desde, hasta := rangoMesActual()
		v := s.ReporteVentas(empresaID, desde, hasta)
		fmt.Fprintf(&b, "- Ventas del mes: Bs %s netas, %d facturas, IVA débito Bs %s.\n", fmtBs(v.VentasNetas), v.CantidadFacturas, fmtBs(v.IVADebito))
	}
	if puedeConsultar(rol, "cobranza") {
		c := s.ReporteCobranza(empresaID)
		fmt.Fprintf(&b, "- Por cobrar: Bs %s (Bs %s vencido).\n", fmtBs(c.TotalPorCobrar), fmtBs(c.TotalVencido))
	}
	if puedeConsultar(rol, "finanzas") {
		pp := s.CuentasPorPagar(empresaID)
		fmt.Fprintf(&b, "- Por pagar a proveedores: Bs %s (%d con saldo).\n", fmtBs(pp.TotalPorPagar), pp.ProveedoresConSaldo)
	}
	if puedeConsultar(rol, "inventario") {
		inv := s.ReporteInventario(empresaID)
		fmt.Fprintf(&b, "- Inventario: %d productos, Bs %s valorizado, %d agotados, %d bajo mínimo.\n", inv.NumeroProductos, fmtBs(inv.ValorizacionTotal), inv.Agotados, inv.BajoMinimo)
	}
	if puedeConsultar(rol, "contabilidad") {
		bal := s.Balance(empresaID)
		estado := "cuadra"
		if !bal.Cuadra {
			estado = "NO cuadra"
		}
		fmt.Fprintf(&b, "- Contabilidad: balance de comprobación %s (%d asientos).\n", estado, bal.Asientos)
	}
	return b.String()
}

/* --- Ayuda (texto fijo del producto) --------------------------------------- */

const (
	textoIGTF          = "El IGTF (Impuesto a las Grandes Transacciones Financieras) es un 3% que se cobra sobre los pagos en moneda distinta al bolívar (divisas o criptomonedas). En ElERP se calcula automáticamente en el cobro del punto de venta cuando registras un pago en USD: se muestra como una línea aparte y se totaliza en el reporte de IGTF de Tesorería."
	textoTransferir    = "Para transferir stock entre sedes o almacenes: entra a Inventario › Transferencias, crea una transferencia indicando origen, destino y los productos con su cantidad. Al confirmarla se generan dos movimientos en el ledger (una salida del origen y una entrada al destino), así el Kardex de ambos queda cuadrado. Nunca se edita un contador de existencias: todo son movimientos auditados."
	textoFormaLibre    = "«Forma libre» es la venta fuera del mostrador: en Ventas armas una cotización, la confirmas y la facturas, reutilizando el mismo motor fiscal del punto de venta (cobro mixto multimoneda, IGTF y vuelto). Sirve para ventas a crédito, pedidos o clientes que no pasan por caja."
	textoNumeroControl = "El Número de Control es un correlativo obligatorio del SENIAT, distinto del número de factura, dentro del rango autorizado por tu providencia. En ElERP se configura en Configuración › Numeración (prefijo y rango desde/hasta) y se asigna automáticamente al emitir cada documento fiscal, de forma continua y sin saltos."
	textoRetencion     = "Una retención es el impuesto (IVA o ISLR) que un agente de retención descuenta al pagar una factura y entera al SENIAT en tu nombre. Si tu empresa es agente de retención (se activa en Configuración › Impuestos), al registrar la retención ElERP genera el comprobante con su número correlativo. Las retenciones que te hacen a ti bajan tu cuenta por cobrar; las que tú emites bajan lo que le debes al proveedor."
)

func ayuda(texto string) RespuestaAsistente {
	return RespuestaAsistente{Tipo: RespMecanica, Respuesta: texto, Fuente: "Ayuda"}
}

func sinAcceso(area string) RespuestaAsistente {
	return RespuestaAsistente{
		Tipo:      RespMecanica,
		Respuesta: fmt.Sprintf("Tu rol no tiene acceso a %s en ElERP, así que no puedo mostrártelo desde el asistente.", area),
		Fuente:    "Permisos",
	}
}

/* --- Helpers de rol, texto y formato --------------------------------------- */

// puedeConsultar refleja lo que cada rol ya ve en la app: el asistente no expone más
// que la interfaz. Vendedor ve ventas/inventario/cobranza de lo suyo; cajero solo
// ventas (factura en el mostrador); contadora/dueña/dev ven finanzas y contabilidad.
func puedeConsultar(rol, area string) bool {
	switch area {
	case "ventas":
		return true
	case "inventario":
		return rol != usuario.RolCajero
	case "cobranza":
		return rol == usuario.RolDueno || rol == usuario.RolDesarrollador || rol == usuario.RolContadora || rol == usuario.RolVendedor
	case "finanzas", "contabilidad":
		return rol == usuario.RolDueno || rol == usuario.RolDesarrollador || rol == usuario.RolContadora
	}
	return false
}

// normalizarPregunta pasa a minúsculas y quita acentos para comparar sin tildes.
func normalizarPregunta(q string) string {
	q = strings.ToLower(strings.TrimSpace(q))
	return strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
	).Replace(q)
}

// contiene devuelve true si q contiene ALGUNA de las subcadenas (ya normalizadas).
func contiene(q string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(q, sub) {
			return true
		}
	}
	return false
}

// extraerEntidad toma lo que sigue al PRIMER "de"/"del" de la pregunta (el nombre del
// producto en "¿cuánto stock hay de harina de maíz?") y le quita las coletillas
// finales ("… tengo?", "… hay"). Se usa el primer conector para no partir nombres que
// contienen "de" (p. ej. "harina de maíz").
func extraerEntidad(q string) string {
	idx, sepLen := -1, 0
	for _, sep := range []string{" de ", " del "} {
		if i := strings.Index(q, sep); i >= 0 && (idx == -1 || i < idx) {
			idx, sepLen = i, len(sep)
		}
	}
	if idx == -1 {
		return ""
	}
	ent := strings.Trim(strings.TrimSpace(q[idx+sepLen:]), "?¿.!, ")
	// Quitar coletillas finales frecuentes que no son parte del nombre del producto.
	for _, tail := range []string{"tengo", "hay", "quedan", "queda", "disponible", "disponibles", "en stock", "en inventario", "en existencia"} {
		if strings.HasSuffix(ent, " "+tail) {
			ent = strings.Trim(strings.TrimSuffix(ent, " "+tail), "?¿.!, ")
		}
	}
	return ent
}

// buscarProducto resuelve un producto por nombre o SKU (exacto o por inclusión).
func (s *Service) buscarProducto(empresaID, nombre string) (inventario.Producto, bool) {
	n := normalizarPregunta(nombre)
	if n == "" || s.productos == nil {
		return inventario.Producto{}, false
	}
	var best inventario.Producto
	found := false
	for _, p := range s.productos.List(empresaID) {
		if normalizarPregunta(p.Nombre) == n || normalizarPregunta(p.SKU) == n {
			return p, true
		}
		if !found && strings.Contains(normalizarPregunta(p.Nombre), n) {
			best, found = p, true
		}
	}
	return best, found
}

// stockTotal pliega el ledger de un producto en toda la empresa (todas las sedes).
func (s *Service) stockTotal(empresaID, productoID string) (float64, float64) {
	if s.movimientos == nil {
		return 0, 0
	}
	return fold(s.movimientos.List(empresaID, inventario.FiltroMovimiento{ProductoID: productoID}))
}

// fmtBs formatea un monto con separador de miles al estilo VE (1.234.567,89).
func fmtBs(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	cents := int64(v*100 + 0.5)
	entero, dec := cents/100, cents%100
	s := fmt.Sprintf("%d", entero)
	var out strings.Builder
	n := len(s)
	for i := 0; i < n; i++ {
		if i > 0 && (n-i)%3 == 0 {
			out.WriteByte('.')
		}
		out.WriteByte(s[i])
	}
	res := fmt.Sprintf("%s,%02d", out.String(), dec)
	if neg {
		res = "-" + res
	}
	return res
}

// fmtCant formatea una cantidad de stock: entera si no tiene decimales, con coma si sí.
func fmtCant(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d unidad(es)", int64(v))
	}
	return strings.Replace(fmt.Sprintf("%.2f unidad(es)", v), ".", ",", 1)
}
