package application

import (
	"errors"
	"fmt"

	"github.com/mornix/elerp/internal/domain/cotizacion"
	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

var (
	ErrCotizacionNoExiste   = errors.New("cotización no existe")
	ErrCotizacionVacia      = errors.New("la cotización no tiene líneas")
	ErrTransicionCotizacion = errors.New("la cotización no está en un estado que permita esta acción")
)

// serieCotizacion es la correlativa de las cotizaciones (no es fiscal).
const serieCotizacion = "COT"

// EntradaCotizacion son los datos para crear/editar una cotización desde el
// módulo Ventas. Igual que en el POS, la tasa NO se recibe del cliente: la pone
// el servidor. Los precios de línea 0 se resuelven contra el catálogo.
type EntradaCotizacion struct {
	ClienteID       string
	Lineas          []LineaEntrada
	Moneda          string
	Validez         string
	CondicionesPago string
	Terminos        string
	Notas           string
	// CuponCodigo: cupón aplicado (opcional). El front ya bajó el precio de las
	// líneas; se guarda para consumir el uso del cupón al facturar la cotización.
	CuponCodigo string
	// ListaPrecio, DireccionEntrega y SedeDespacho son aditivos: hoy la lista solo
	// puede ser base; la sede de despacho fija de qué almacén sale el inventario al
	// facturar (ver FacturarCotizacion).
	ListaPrecio      string
	DireccionEntrega string
	SedeDespacho     string
}

// Cotizaciones lista las cotizaciones de la empresa.
func (s *Service) Cotizaciones(empresaID string) []cotizacion.Cotizacion {
	return s.cotizaciones.List(empresaID)
}

// Cotizacion devuelve una cotización por id.
func (s *Service) Cotizacion(empresaID, id string) (cotizacion.Cotizacion, bool) {
	return s.cotizaciones.ByID(empresaID, id)
}

// armarLineasCotizacion construye las líneas desde el catálogo y calcula los
// totales con la MISMA lógica fiscal que EmitirFactura: precio del catálogo si
// no viene, herencia de la exención del producto, conversión USD→Bs con la tasa
// del servidor, IVA solo sobre lo gravado. El IGTF queda en 0 (se calcula al
// facturar según los pagos).
func (s *Service) armarLineasCotizacion(empresaID string, in EntradaCotizacion) ([]cotizacion.Linea, float64, float64, float64, error) {
	tasaActual, hayTasa := s.TasaVigente(empresaID)
	monedaEmpresa := s.MonedaPrincipalDe(empresaID)

	lineas := make([]cotizacion.Linea, 0, len(in.Lineas))
	var subtotal, baseImponible float64
	for _, l := range in.Lineas {
		p, ok := s.productos.BySKU(empresaID, l.SKU)
		if !ok {
			return nil, 0, 0, 0, fmt.Errorf("%w: %s", ErrProductoNoExiste, l.SKU)
		}
		// Contrato de PrecioUnitario idéntico al de EmitirFactura (POS): un valor
		// >=0 es AUTORITATIVO en bolívares (el front ya convirtió; 0 = línea gratis,
		// no se reconvierte); <0 = usar el precio del catálogo, convertido a Bs con la
		// tasa del servidor si está en dólares. Antes se reconvertía SIEMPRE el precio
		// en dólares, así que un precio ya en Bs se multiplicaba de nuevo por la tasa
		// (cobro 40× en empresas con moneda principal USD).
		precio := l.PrecioUnitario
		if precio < 0 {
			precio = p.Precio
			if monedaDelPrecio(p, monedaEmpresa) == empresa.MonedaUSD {
				if !hayTasa {
					return nil, 0, 0, 0, ErrSinTasa
				}
				precio = round2(precio * tasaActual.Valor)
			}
		}
		// Descuento porcentual por línea (0..100). El PrecioUnitario guardado es el
		// ORIGINAL de lista; el Total de la línea es el NETO tras el descuento.
		desc := l.Descuento
		if desc < 0 {
			desc = 0
		}
		if desc > 100 {
			desc = 100
		}
		total := round2(precio * l.Cantidad * (1 - desc/100))
		lineas = append(lineas, cotizacion.Linea{
			ProductoID: p.ID, SKU: p.SKU, Nombre: p.Nombre, Descripcion: l.Descripcion,
			Cantidad: l.Cantidad, PrecioUnitario: precio, Descuento: desc, Total: total,
			Exento: p.ExentoIVA,
		})
		subtotal += total
		if !p.ExentoIVA {
			baseImponible += total
		}
	}
	subtotal = round2(subtotal)
	iva := round2(round2(baseImponible) * s.alicuotaIVA(empresaID))
	total := round2(subtotal + iva)
	return lineas, subtotal, iva, total, nil
}

// CrearCotizacion arma una cotización en borrador con folio de la serie "COT".
func (s *Service) CrearCotizacion(empresaID, sedeID, actor, origen string, in EntradaCotizacion) (cotizacion.Cotizacion, error) {
	if len(in.Lineas) == 0 {
		return cotizacion.Cotizacion{}, ErrCotizacionVacia
	}
	lineas, subtotal, iva, total, err := s.armarLineasCotizacion(empresaID, in)
	if err != nil {
		return cotizacion.Cotizacion{}, err
	}
	tasaActual, _ := s.TasaVigente(empresaID)
	if in.Moneda == "" {
		in.Moneda = empresa.MonedaVES
	}

	c := cotizacion.Cotizacion{
		EmpresaID: empresaID, SedeID: sedeID, Estado: cotizacion.EstadoBorrador,
		Lineas: lineas, Subtotal: subtotal, IVA: iva, IGTF: 0, Total: total,
		Moneda: in.Moneda, TasaCambio: tasaActual.Valor,
		Validez: in.Validez, CondicionesPago: in.CondicionesPago, Terminos: in.Terminos, Notas: in.Notas,
		CuponCodigo: in.CuponCodigo,
		ListaPrecio: in.ListaPrecio, DireccionEntrega: in.DireccionEntrega, SedeDespacho: in.SedeDespacho,
		Actor: actor, Fecha: ahora(), Actualizada: ahora(),
	}
	// Cliente (opcional en una cotización: puede ser un presupuesto sin cerrar).
	if in.ClienteID != "" {
		if cl, ok := s.clientes.ByID(empresaID, in.ClienteID); ok {
			c.ClienteID = cl.ID
			c.ClienteNombre = cl.Nombre
			c.ClienteDocumento = cl.TipoDocumento + "-" + cl.Documento
		}
	}
	// Folio de la serie COT (no fiscal), serializado por empresa+sede.
	c.Numero = s.numerador.Siguiente(empresaID, sedeID, serieCotizacion)
	c.NumeroCompleto = fmt.Sprintf("%s-%06d", serieCotizacion, c.Numero)

	out := s.cotizaciones.Create(c)
	s.audit.Append(evento(empresaID, actor, origen, "ventas.cotizacion.crear", out.NumeroCompleto, out.Estado))
	return out, nil
}

// ActualizarCotizacion recalcula una cotización en borrador. Fuera de borrador
// es inmutable: una vez confirmada o facturada no se edita.
func (s *Service) ActualizarCotizacion(empresaID, id, actor, origen string, in EntradaCotizacion) (cotizacion.Cotizacion, error) {
	c, ok := s.cotizaciones.ByID(empresaID, id)
	if !ok {
		return cotizacion.Cotizacion{}, ErrCotizacionNoExiste
	}
	if c.Estado != cotizacion.EstadoBorrador {
		return cotizacion.Cotizacion{}, ErrTransicionCotizacion
	}
	if len(in.Lineas) == 0 {
		return cotizacion.Cotizacion{}, ErrCotizacionVacia
	}
	lineas, subtotal, iva, total, err := s.armarLineasCotizacion(empresaID, in)
	if err != nil {
		return cotizacion.Cotizacion{}, err
	}
	tasaActual, _ := s.TasaVigente(empresaID)
	if in.Moneda != "" {
		c.Moneda = in.Moneda
	}
	c.Lineas = lineas
	c.Subtotal, c.IVA, c.IGTF, c.Total = subtotal, iva, 0, total
	c.TasaCambio = tasaActual.Valor
	c.Validez, c.CondicionesPago, c.Terminos, c.Notas = in.Validez, in.CondicionesPago, in.Terminos, in.Notas
	c.CuponCodigo = in.CuponCodigo
	c.ListaPrecio, c.DireccionEntrega, c.SedeDespacho = in.ListaPrecio, in.DireccionEntrega, in.SedeDespacho
	// Cliente puede cambiar en el borrador (o quedar vacío).
	c.ClienteID, c.ClienteNombre, c.ClienteDocumento = "", "", ""
	if in.ClienteID != "" {
		if cl, ok := s.clientes.ByID(empresaID, in.ClienteID); ok {
			c.ClienteID = cl.ID
			c.ClienteNombre = cl.Nombre
			c.ClienteDocumento = cl.TipoDocumento + "-" + cl.Documento
		}
	}
	c.Actualizada = ahora()

	out, ok := s.cotizaciones.Update(c)
	if !ok {
		return cotizacion.Cotizacion{}, ErrCotizacionNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "ventas.cotizacion.actualizar", out.NumeroCompleto, out.Estado))
	return out, nil
}

// ConfirmarCotizacion pasa un borrador a confirmada (pedido/prefactura).
func (s *Service) ConfirmarCotizacion(empresaID, id, actor, origen string) (cotizacion.Cotizacion, error) {
	c, ok := s.cotizaciones.ByID(empresaID, id)
	if !ok {
		return cotizacion.Cotizacion{}, ErrCotizacionNoExiste
	}
	if c.Estado != cotizacion.EstadoBorrador {
		return cotizacion.Cotizacion{}, ErrTransicionCotizacion
	}
	c.Estado = cotizacion.EstadoConfirmada
	c.Actualizada = ahora()
	out, ok := s.cotizaciones.Update(c)
	if !ok {
		return cotizacion.Cotizacion{}, ErrCotizacionNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "ventas.cotizacion.confirmar", out.NumeroCompleto, out.Estado))
	return out, nil
}

// EntradaFacturacion son los datos de COBRO que recibe el facturado de una
// cotización: el MISMO arreglo de pagos + los datos de vuelto (y crédito) que
// acepta EmitirFactura en el POS. Las líneas, el cliente y la moneda NO se reciben
// del caller: salen de la cotización. Así el módulo Ventas cobra igual que el
// mostrador (cobro mixto, multimoneda, IGTF y vuelto) por la misma ruta fiscal.
type EntradaFacturacion struct {
	Pagos []PagoEntrada
	// Credito: la parte no cobrada queda por cobrar en Tesorería (exige cliente).
	Credito     bool
	DiasCredito int
	// Vuelto declarado por quien factura (todos opcionales), idéntico a EmitirEntrada:
	// en qué moneda y por qué medio se entrega el excedente. El monto lo calcula el
	// servidor. Vacíos ⇒ vuelto derivado y por efectivo.
	VueltoMoneda   string
	VueltoMetodo   string
	VueltoBanco    string
	VueltoCedula   string
	VueltoTelefono string
	// VueltoPartes reparte el vuelto en varias partes (moneda + medio + monto).
	// Igual que en el POS: si viene, manda sobre los campos únicos de arriba.
	VueltoPartes []VueltoParteEntrada
}

// FacturarCotizacion emite la factura forma libre de una cotización confirmada.
// El descuento de inventario, el arqueo del cobro (conversión por divisa, IGTF,
// vuelto flexible) y el asiento contable los hace EmitirFactura —la MISMA ruta que
// el POS—; acá solo se construye la entrada desde la cotización, se le adjunta el
// cobro recibido (pagos + vuelto + crédito) y se enlaza el documento emitido.
func (s *Service) FacturarCotizacion(empresaID, id, actor, origen string, in EntradaFacturacion) (cotizacion.Cotizacion, fiscal.Documento, error) {
	c, ok := s.cotizaciones.ByID(empresaID, id)
	if !ok {
		return cotizacion.Cotizacion{}, fiscal.Documento{}, ErrCotizacionNoExiste
	}
	if c.Estado != cotizacion.EstadoConfirmada {
		return cotizacion.Cotizacion{}, fiscal.Documento{}, ErrTransicionCotizacion
	}
	// Venta forma libre: sin caja (canal administrativo, no POS). El cobro se procesa
	// por la MISMA ruta que el mostrador: EmitirFactura hace conversión, IGTF y vuelto.
	ent := EmitirEntrada{
		ClienteID: c.ClienteID, Moneda: c.Moneda, Pagos: in.Pagos, SinCaja: true,
		// La mercancía sale del almacén de despacho elegido en la cotización. La
		// factura fiscal se registra en la sede de la cotización (c.SedeID); solo la
		// salida de inventario se descuenta de SedeDespacho (vacío ⇒ misma sede).
		SedeID:  c.SedeDespacho,
		Credito: in.Credito, DiasCredito: in.DiasCredito,
		VueltoMoneda: in.VueltoMoneda, VueltoMetodo: in.VueltoMetodo,
		VueltoBanco: in.VueltoBanco, VueltoCedula: in.VueltoCedula, VueltoTelefono: in.VueltoTelefono,
		VueltoPartes: in.VueltoPartes,
		// El cupón (si lo hubo) va GUARDADO en la cotización: se consume aquí, al
		// facturar, por la misma vía del POS (EmitirFactura → ConsumirCupon).
		CuponCodigo: c.CuponCodigo,
	}
	for _, l := range c.Lineas {
		// PrecioUnitario ya está en bolívares (se convirtió al cotizar); se pasa
		// explícito para que la factura respete el precio pactado en la cotización.
		// Se aplica el descuento de línea: la factura recibe el precio NETO, ya que
		// el documento fiscal no modela descuento por renglón.
		neto := round2(l.PrecioUnitario * (1 - l.Descuento/100))
		ent.Lineas = append(ent.Lineas, LineaEntrada{SKU: l.SKU, Cantidad: l.Cantidad, PrecioUnitario: neto})
	}
	doc, err := s.EmitirFactura(empresaID, c.SedeID, empresa.ModalidadFormaLibre, actor, origen, ent)
	if err != nil {
		return cotizacion.Cotizacion{}, fiscal.Documento{}, err
	}
	c.Estado = cotizacion.EstadoFacturada
	c.DocumentoID = doc.ID
	c.Actualizada = ahora()
	out, ok := s.cotizaciones.Update(c)
	if !ok {
		return cotizacion.Cotizacion{}, fiscal.Documento{}, ErrCotizacionNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "ventas.cotizacion.facturar", out.NumeroCompleto, doc.NumeroCompleto))
	return out, doc, nil
}

// CancelarCotizacion marca una cotización como cancelada. Una ya facturada no se
// puede cancelar: la corrección de una factura es su anulación fiscal, no esto.
func (s *Service) CancelarCotizacion(empresaID, id, actor, origen, motivo string) (cotizacion.Cotizacion, error) {
	c, ok := s.cotizaciones.ByID(empresaID, id)
	if !ok {
		return cotizacion.Cotizacion{}, ErrCotizacionNoExiste
	}
	if c.Estado == cotizacion.EstadoFacturada || c.Estado == cotizacion.EstadoCancelada {
		return cotizacion.Cotizacion{}, ErrTransicionCotizacion
	}
	c.Estado = cotizacion.EstadoCancelada
	if motivo != "" {
		c.Notas = motivo
	}
	c.Actualizada = ahora()
	out, ok := s.cotizaciones.Update(c)
	if !ok {
		return cotizacion.Cotizacion{}, ErrCotizacionNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "ventas.cotizacion.cancelar", out.NumeroCompleto, motivo))
	return out, nil
}
