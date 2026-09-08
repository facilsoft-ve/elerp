package application

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mornix/elerp/internal/domain/compra"
)

// Errores de negocio del submódulo Solicitudes de presupuesto (RFQ).
var (
	ErrSolicitudNoExiste     = errors.New("solicitud de presupuesto no existe")
	ErrSolicitudVacia        = errors.New("la solicitud no tiene líneas")
	ErrSolicitudSinProv      = errors.New("la solicitud no indica proveedores a los que pedir presupuesto")
	ErrSolicitudEstado       = errors.New("la solicitud no está en un estado que permita esta acción")
	ErrProvNoInvitado        = errors.New("el proveedor no fue incluido en esta solicitud")
	ErrSolicitudSinPrecios   = errors.New("el proveedor no cotizó ningún precio: no hay nada que convertir en orden")
	ErrSolicitudNoDisponible = errors.New("el registro de solicitudes de presupuesto no está disponible")
)

// serieSolicitud es la correlativa de las solicitudes de presupuesto (no es fiscal).
const serieSolicitud = "SOL"

// LineaSolicitudEntrada es un renglón al crear/editar una solicitud: solo qué
// producto y cuánto se pide cotizar (el precio lo pone cada proveedor).
type LineaSolicitudEntrada struct {
	SKU      string
	Cantidad float64
}

// EntradaSolicitud son los datos para crear/editar una solicitud de presupuesto.
type EntradaSolicitud struct {
	SedeID      string
	Notas       string
	Lineas      []LineaSolicitudEntrada
	Proveedores []string // ids de proveedores a los que se pide presupuesto
}

// RespuestaLineaEntrada es el precio que un proveedor cotiza para un SKU.
type RespuestaLineaEntrada struct {
	SKU            string
	PrecioUnitario float64
}

// ConSolicitudesCompra cablea el repositorio de solicitudes de presupuesto. Se
// configura aparte de New (como ConListasPrecio / ConNotasCompra) para no romper
// las firmas de los constructores ya cableados en cmd/api; sin él, el servicio
// funciona igual y las solicitudes quedan vacías.
func (s *Service) ConSolicitudesCompra(r compra.SolicitudCompraRepo) *Service {
	s.solicitudes = r
	return s
}

// SolicitudesCompra lista las solicitudes de presupuesto de la empresa.
func (s *Service) SolicitudesCompra(empresaID string) []compra.SolicitudCompra {
	if s.solicitudes == nil {
		return []compra.SolicitudCompra{}
	}
	return s.solicitudes.List(empresaID)
}

// SolicitudCompra devuelve una solicitud de presupuesto por id.
func (s *Service) SolicitudCompra(empresaID, id string) (compra.SolicitudCompra, bool) {
	if s.solicitudes == nil {
		return compra.SolicitudCompra{}, false
	}
	return s.solicitudes.ByID(empresaID, id)
}

// armarLineasSolicitud construye las líneas desde el catálogo (enriqueciendo
// nombre/productoID) validando que cada SKU exista y que la cantidad sea > 0.
func (s *Service) armarLineasSolicitud(empresaID string, in []LineaSolicitudEntrada) ([]compra.LineaSolicitud, error) {
	lineas := make([]compra.LineaSolicitud, 0, len(in))
	vistos := map[string]bool{}
	for _, l := range in {
		sku := strings.TrimSpace(l.SKU)
		if sku == "" || vistos[sku] {
			continue
		}
		if l.Cantidad <= 0 {
			return nil, fmt.Errorf("la cantidad de %s debe ser mayor a 0", sku)
		}
		p, ok := s.productos.BySKU(empresaID, sku)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrProductoNoExiste, sku)
		}
		// Un combo no se cotiza ni se compra: se piden presupuestos de sus componentes.
		if p.EsCombo {
			return nil, fmt.Errorf("%w: %s", ErrComboEnCompra, sku)
		}
		vistos[sku] = true
		lineas = append(lineas, compra.LineaSolicitud{
			ProductoID: p.ID, SKU: p.SKU, Nombre: p.Nombre, Cantidad: l.Cantidad,
		})
	}
	return lineas, nil
}

// armarProveedoresSolicitud construye la lista de proveedores invitados desde sus
// ids, validando que cada uno exista y deduplicando. Todos arrancan pendientes.
func (s *Service) armarProveedoresSolicitud(empresaID string, ids []string) ([]compra.ProveedorCotiza, error) {
	out := make([]compra.ProveedorCotiza, 0, len(ids))
	vistos := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || vistos[id] {
			continue
		}
		prov, ok := s.provs.ByID(empresaID, id)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrProveedorNoExiste, id)
		}
		vistos[id] = true
		out = append(out, compra.ProveedorCotiza{
			ProveedorID: prov.ID, ProveedorNombre: prov.Nombre,
			Estado: compra.CotizaPendiente, Respondida: false, Lineas: []compra.RespuestaLinea{},
		})
	}
	return out, nil
}

// CrearSolicitud arma una solicitud de presupuesto en borrador con folio de la
// serie "SOL". Valida que haya al menos una línea; los proveedores son opcionales
// al crear (se pueden agregar antes de enviar).
func (s *Service) CrearSolicitud(empresaID, sedeID, actor, origen string, in EntradaSolicitud) (compra.SolicitudCompra, error) {
	if s.solicitudes == nil {
		return compra.SolicitudCompra{}, ErrSolicitudNoDisponible
	}
	lineas, err := s.armarLineasSolicitud(empresaID, in.Lineas)
	if err != nil {
		return compra.SolicitudCompra{}, err
	}
	if len(lineas) == 0 {
		return compra.SolicitudCompra{}, ErrSolicitudVacia
	}
	provs, err := s.armarProveedoresSolicitud(empresaID, in.Proveedores)
	if err != nil {
		return compra.SolicitudCompra{}, err
	}
	sede := in.SedeID
	if sede == "" {
		sede = sedeID
	}
	sol := compra.SolicitudCompra{
		EmpresaID: empresaID, SedeID: sede,
		Serie: serieSolicitud, Estado: compra.SolBorrador,
		Fecha: ahora(), Notas: strings.TrimSpace(in.Notas),
		Lineas: lineas, Proveedores: provs,
		Actor: actor, Creada: ahora(), Actualizada: ahora(),
	}
	sol.Numero = s.numerador.Siguiente(empresaID, sede, serieSolicitud)
	sol.NumeroCompleto = fmt.Sprintf("%s-%06d", serieSolicitud, sol.Numero)

	out := s.solicitudes.Create(sol)
	s.audit.Append(evento(empresaID, actor, origen, "compras.solicitud.crear", out.NumeroCompleto, ""))
	return out, nil
}

// ActualizarSolicitud edita una solicitud en BORRADOR: líneas, proveedores y
// notas. Una vez enviada ya no se editan sus líneas ni proveedores (queda
// registrada). Los proveedores que ya existían conservan su respuesta si siguen
// invitados; los nuevos entran pendientes.
func (s *Service) ActualizarSolicitud(empresaID, id, actor, origen string, in EntradaSolicitud) (compra.SolicitudCompra, error) {
	if s.solicitudes == nil {
		return compra.SolicitudCompra{}, ErrSolicitudNoDisponible
	}
	sol, ok := s.solicitudes.ByID(empresaID, id)
	if !ok {
		return compra.SolicitudCompra{}, ErrSolicitudNoExiste
	}
	if sol.Estado != compra.SolBorrador {
		return compra.SolicitudCompra{}, ErrSolicitudEstado
	}
	lineas, err := s.armarLineasSolicitud(empresaID, in.Lineas)
	if err != nil {
		return compra.SolicitudCompra{}, err
	}
	if len(lineas) == 0 {
		return compra.SolicitudCompra{}, ErrSolicitudVacia
	}
	nuevos, err := s.armarProveedoresSolicitud(empresaID, in.Proveedores)
	if err != nil {
		return compra.SolicitudCompra{}, err
	}
	// Conserva la respuesta de un proveedor que siga invitado (no se pierde por editar).
	previos := map[string]compra.ProveedorCotiza{}
	for _, p := range sol.Proveedores {
		previos[p.ProveedorID] = p
	}
	for i, p := range nuevos {
		if ya, existe := previos[p.ProveedorID]; existe {
			nuevos[i] = ya
		}
	}
	sol.Lineas = lineas
	sol.Proveedores = nuevos
	sol.Notas = strings.TrimSpace(in.Notas)
	sol.Actualizada = ahora()

	out, ok := s.solicitudes.Update(sol)
	if !ok {
		return compra.SolicitudCompra{}, ErrSolicitudNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "compras.solicitud.actualizar", out.NumeroCompleto, ""))
	return out, nil
}

// EnviarSolicitud registra el envío de la solicitud a los proveedores: pasa de
// borrador a enviada y a partir de ahí queda a la espera de respuestas. Exige al
// menos un proveedor al que pedir presupuesto.
func (s *Service) EnviarSolicitud(empresaID, id, actor, origen string) (compra.SolicitudCompra, error) {
	if s.solicitudes == nil {
		return compra.SolicitudCompra{}, ErrSolicitudNoDisponible
	}
	sol, ok := s.solicitudes.ByID(empresaID, id)
	if !ok {
		return compra.SolicitudCompra{}, ErrSolicitudNoExiste
	}
	if sol.Estado != compra.SolBorrador {
		return compra.SolicitudCompra{}, ErrSolicitudEstado
	}
	if len(sol.Proveedores) == 0 {
		return compra.SolicitudCompra{}, ErrSolicitudSinProv
	}
	sol.Estado = compra.SolEnviada
	sol.Actualizada = ahora()
	out, ok := s.solicitudes.Update(sol)
	if !ok {
		return compra.SolicitudCompra{}, ErrSolicitudNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "compras.solicitud.enviar", out.NumeroCompleto, out.Estado))
	return out, nil
}

// RegistrarRespuesta carga los precios que respondió un proveedor invitado a la
// solicitud. Solo se admite si la solicitud ya se envió (enviada o respondida) y
// el proveedor fue incluido. Calcula el total del proveedor (precio × cantidad de
// cada SKU cotizado) para poder comparar, y mueve la solicitud a "respondida".
func (s *Service) RegistrarRespuesta(empresaID, id, actor, origen, proveedorID string, lineas []RespuestaLineaEntrada) (compra.SolicitudCompra, error) {
	if s.solicitudes == nil {
		return compra.SolicitudCompra{}, ErrSolicitudNoDisponible
	}
	sol, ok := s.solicitudes.ByID(empresaID, id)
	if !ok {
		return compra.SolicitudCompra{}, ErrSolicitudNoExiste
	}
	if sol.Estado != compra.SolEnviada && sol.Estado != compra.SolRespondida {
		return compra.SolicitudCompra{}, ErrSolicitudEstado
	}
	idx := -1
	for i, p := range sol.Proveedores {
		if p.ProveedorID == proveedorID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return compra.SolicitudCompra{}, ErrProvNoInvitado
	}
	// Cantidad pedida por SKU (para valorizar la respuesta). Solo se aceptan
	// precios de SKU que están en la solicitud; los demás se ignoran.
	cantPorSKU := map[string]float64{}
	for _, l := range sol.Lineas {
		cantPorSKU[l.SKU] = l.Cantidad
	}
	resp := make([]compra.RespuestaLinea, 0, len(lineas))
	var total float64
	vistos := map[string]bool{}
	for _, l := range lineas {
		sku := strings.TrimSpace(l.SKU)
		cant, ok := cantPorSKU[sku]
		if !ok || vistos[sku] || l.PrecioUnitario < 0 {
			continue
		}
		vistos[sku] = true
		precio := round2(l.PrecioUnitario)
		resp = append(resp, compra.RespuestaLinea{SKU: sku, PrecioUnitario: precio})
		total += precio * cant
	}
	sol.Proveedores[idx].Lineas = resp
	sol.Proveedores[idx].Total = round2(total)
	sol.Proveedores[idx].Respondida = len(resp) > 0
	sol.Proveedores[idx].Estado = compra.CotizaPendiente
	if len(resp) > 0 {
		sol.Proveedores[idx].Estado = compra.CotizaRespondida
		sol.Proveedores[idx].RespondidaEn = ahora()
	}
	// La solicitud queda "respondida" en cuanto algún proveedor tiene respuesta.
	if sol.Estado == compra.SolEnviada && sol.Proveedores[idx].Respondida {
		sol.Estado = compra.SolRespondida
	}
	sol.Actualizada = ahora()
	out, ok := s.solicitudes.Update(sol)
	if !ok {
		return compra.SolicitudCompra{}, ErrSolicitudNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "compras.solicitud.respuesta", out.NumeroCompleto, sol.Proveedores[idx].ProveedorNombre))
	return out, nil
}

// CerrarSolicitud cierra la negociación a mano (sin convertir a orden): pasa a
// "cerrada". Se admite desde borrador, enviada o respondida.
func (s *Service) CerrarSolicitud(empresaID, id, actor, origen string) (compra.SolicitudCompra, error) {
	if s.solicitudes == nil {
		return compra.SolicitudCompra{}, ErrSolicitudNoDisponible
	}
	sol, ok := s.solicitudes.ByID(empresaID, id)
	if !ok {
		return compra.SolicitudCompra{}, ErrSolicitudNoExiste
	}
	switch sol.Estado {
	case compra.SolBorrador, compra.SolEnviada, compra.SolRespondida:
		// permitido
	default:
		return compra.SolicitudCompra{}, ErrSolicitudEstado
	}
	sol.Estado = compra.SolCerrada
	sol.Actualizada = ahora()
	out, ok := s.solicitudes.Update(sol)
	if !ok {
		return compra.SolicitudCompra{}, ErrSolicitudNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "compras.solicitud.cerrar", out.NumeroCompleto, out.Estado))
	return out, nil
}

// ConvertirEnOrden crea una ORDEN DE COMPRA a partir de la solicitud usando las
// líneas y los precios que cotizó el proveedor elegido. Reusa CrearOrdenCompra
// (no reimplementa la OC): la orden nace en borrador con su propio folio "OC" y su
// flujo de recepción/asiento intacto. La solicitud queda "cerrada", enlazada a la
// orden generada. Exige que el proveedor haya respondido con al menos un precio.
func (s *Service) ConvertirEnOrden(empresaID, id, actor, origen, proveedorID string) (compra.SolicitudCompra, compra.OrdenCompra, error) {
	if s.solicitudes == nil {
		return compra.SolicitudCompra{}, compra.OrdenCompra{}, ErrSolicitudNoDisponible
	}
	sol, ok := s.solicitudes.ByID(empresaID, id)
	if !ok {
		return compra.SolicitudCompra{}, compra.OrdenCompra{}, ErrSolicitudNoExiste
	}
	if sol.Estado != compra.SolEnviada && sol.Estado != compra.SolRespondida {
		return compra.SolicitudCompra{}, compra.OrdenCompra{}, ErrSolicitudEstado
	}
	var cotiza *compra.ProveedorCotiza
	for i := range sol.Proveedores {
		if sol.Proveedores[i].ProveedorID == proveedorID {
			cotiza = &sol.Proveedores[i]
			break
		}
	}
	if cotiza == nil {
		return compra.SolicitudCompra{}, compra.OrdenCompra{}, ErrProvNoInvitado
	}
	if !cotiza.Respondida || len(cotiza.Lineas) == 0 {
		return compra.SolicitudCompra{}, compra.OrdenCompra{}, ErrSolicitudSinPrecios
	}
	// Precio cotizado por SKU; solo se llevan a la orden las líneas que el proveedor
	// coticó (con precio). Las cantidades salen de la solicitud.
	precioPorSKU := map[string]float64{}
	for _, l := range cotiza.Lineas {
		precioPorSKU[l.SKU] = l.PrecioUnitario
	}
	entrada := EntradaOC{
		ProveedorID: proveedorID, SedeID: sol.SedeID,
		Notas: fmt.Sprintf("Generada desde la solicitud de presupuesto %s", sol.NumeroCompleto),
	}
	for _, l := range sol.Lineas {
		precio, ok := precioPorSKU[l.SKU]
		if !ok {
			continue
		}
		entrada.Lineas = append(entrada.Lineas, LineaOCEntrada{
			SKU: l.SKU, Cantidad: l.Cantidad, CostoUnitario: precio,
		})
	}
	if len(entrada.Lineas) == 0 {
		return compra.SolicitudCompra{}, compra.OrdenCompra{}, ErrSolicitudSinPrecios
	}
	oc, err := s.CrearOrdenCompra(empresaID, sol.SedeID, actor, origen, entrada)
	if err != nil {
		return compra.SolicitudCompra{}, compra.OrdenCompra{}, err
	}
	sol.Estado = compra.SolCerrada
	sol.ProveedorElegidoID = proveedorID
	sol.OrdenGeneradaID = oc.ID
	sol.Actualizada = ahora()
	out, ok := s.solicitudes.Update(sol)
	if !ok {
		return compra.SolicitudCompra{}, compra.OrdenCompra{}, ErrSolicitudNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "compras.solicitud.convertir", out.NumeroCompleto, oc.NumeroCompleto))
	return out, oc, nil
}
