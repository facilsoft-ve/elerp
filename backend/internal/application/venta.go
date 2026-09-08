package application

import (
	"errors"
	"strings"

	"github.com/mornix/elerp/internal/domain/venta"
)

// Errores de negocio de las ventas en espera.
var (
	ErrVentaEnEsperaVacia    = errors.New("no hay nada en el carrito para dejar en espera")
	ErrVentaEnEsperaNoExiste = errors.New("esa venta en espera ya no existe: alguien la retomó o la descartó")
)

// maxEnEsperaPorSede acota cuántos carritos se pueden apartar a la vez. No es una
// restricción de negocio arbitraria: una cola de veinte carritos apartados es
// siempre un olvido, y el cajero termina sin saber cuál es cuál.
const maxEnEsperaPorSede = 12

// DejarVentaEnEspera aparta el carrito para atender al siguiente cliente.
func (s *Service) DejarVentaEnEspera(empresaID, sedeID, actor, origen, nota, clienteID string, lineas []venta.Linea) (venta.EnEspera, error) {
	if s.ventasEnEspera == nil {
		return venta.EnEspera{}, errors.New("las ventas en espera no están configuradas")
	}
	if len(lineas) == 0 {
		return venta.EnEspera{}, ErrVentaEnEsperaVacia
	}
	if len(s.ventasEnEspera.List(empresaID, sedeID)) >= maxEnEsperaPorSede {
		return venta.EnEspera{}, errors.New("ya hay demasiadas ventas en espera en esta sede: retoma o descarta alguna primero")
	}

	// Los precios y la exención se toman del CATÁLOGO, no del cliente: el POS no
	// decide cuánto cuesta algo (misma regla que al emitir).
	total := 0.0
	limpias := make([]venta.Linea, 0, len(lineas))
	for _, l := range lineas {
		p, ok := s.productos.BySKU(empresaID, l.SKU)
		if !ok || l.Cantidad <= 0 {
			continue
		}
		precio := l.PrecioUnitario
		if precio <= 0 {
			precio = p.Precio
		}
		limpias = append(limpias, venta.Linea{
			SKU: p.SKU, Nombre: p.Nombre, Cantidad: l.Cantidad,
			PrecioUnitario: precio, Exento: p.ExentoIVA,
		})
		total += precio * l.Cantidad
	}
	if len(limpias) == 0 {
		return venta.EnEspera{}, ErrVentaEnEsperaVacia
	}

	// De quién es: si hay turno abierto se copia el arqueo, para que la lista diga
	// quién la apartó aunque la retome otro cajero.
	v := venta.EnEspera{
		EmpresaID: empresaID, SedeID: sedeID, Nota: strings.TrimSpace(nota),
		ClienteID: clienteID, ActorID: actor, Lineas: limpias,
		Total: round2(total), Creada: ahora(),
	}
	if ses, abierta := s.sesiones.AbiertaDeActor(empresaID, actor); abierta {
		v.CajeroNombre = ses.CajeroNombre
		v.CajaCodigo = ses.CajaCodigo
	}
	out := s.ventasEnEspera.Create(v)
	s.audit.Append(evento(empresaID, actor, origen, "pos.venta_en_espera", out.ID, out.Nota))
	return out, nil
}

// VentasEnEspera lista los carritos apartados de una sede.
func (s *Service) VentasEnEspera(empresaID, sedeID string) []venta.EnEspera {
	if s.ventasEnEspera == nil {
		return []venta.EnEspera{}
	}
	return s.ventasEnEspera.List(empresaID, sedeID)
}

// RetomarVenta devuelve el carrito apartado y lo CONSUME en la misma operación:
// así dos cajeros no pueden retomar la misma venta y cobrarla dos veces.
func (s *Service) RetomarVenta(empresaID, sedeID, actor, origen, id string) (venta.EnEspera, error) {
	if s.ventasEnEspera == nil {
		return venta.EnEspera{}, ErrVentaEnEsperaNoExiste
	}
	v, ok := s.ventasEnEspera.ByID(empresaID, id)
	if !ok {
		return venta.EnEspera{}, ErrVentaEnEsperaNoExiste
	}
	// Una venta apartada en otra sede no se retoma acá: sus precios y su stock son
	// de esa tienda.
	if sedeID != "" && v.SedeID != sedeID {
		return venta.EnEspera{}, errors.New("esa venta quedó en espera en otra sede")
	}
	if !s.ventasEnEspera.Delete(empresaID, id) {
		return venta.EnEspera{}, ErrVentaEnEsperaNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "pos.venta_retomada", id, v.Nota))
	return v, nil
}

// DescartarVenta tira un carrito apartado. Se audita: es la vía por la que una
// venta armada desaparece sin quedar registro fiscal.
func (s *Service) DescartarVenta(empresaID, actor, origen, id string) error {
	if s.ventasEnEspera == nil {
		return ErrVentaEnEsperaNoExiste
	}
	v, ok := s.ventasEnEspera.ByID(empresaID, id)
	if !ok {
		return ErrVentaEnEsperaNoExiste
	}
	if !s.ventasEnEspera.Delete(empresaID, id) {
		return ErrVentaEnEsperaNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "pos.venta_descartada", id, v.Nota))
	return nil
}
