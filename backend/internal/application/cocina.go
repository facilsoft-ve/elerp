package application

import (
	"errors"
	"sort"
	"strings"

	"github.com/mornix/elerp/internal/domain/cocina"
	"github.com/mornix/elerp/internal/domain/cuenta"
)

var (
	ErrCocinaNoDisponible   = errors.New("la configuración de cocina no está disponible")
	ErrConexionInvalida     = errors.New("modo de conexión inválido (local o red)")
	ErrImpresoraSinHost     = errors.New("una impresora de red necesita su IP o host")
	ErrImpresoraNoExiste    = errors.New("la comandera no existe")
	ErrUltimaPredeterminada = errors.New("tiene que quedar una comandera predeterminada: es la que recibe lo que no encaja en ningún rubro")
)

// ConImpresoras cablea las comanderas (impresoras de comandas).
func (s *Service) ConImpresoras(r cocina.Repository) *Service {
	s.impresoras = r
	return s
}

// Impresoras devuelve las comanderas de la sede, la predeterminada primero.
func (s *Service) Impresoras(empresaID, sedeID string) []cocina.Impresora {
	if s.impresoras == nil {
		return []cocina.Impresora{}
	}
	out := s.impresoras.List(empresaID, sedeID)
	for i := range out {
		if out[i].Rubros == nil {
			out[i].Rubros = []string{}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Predeterminada != out[j].Predeterminada {
			return out[i].Predeterminada
		}
		return out[i].Nombre < out[j].Nombre
	})
	return out
}

// ImpresoraBody es el cuerpo de configuración de una comandera.
type ImpresoraBody struct {
	ID             string   `json:"id"`
	Nombre         string   `json:"nombre"`
	Conexion       string   `json:"conexion"`
	Host           string   `json:"host"`
	Puerto         int      `json:"puerto"`
	AnchoMM        int      `json:"anchoMm"`
	Rubros         []string `json:"rubros"`
	Predeterminada bool     `json:"predeterminada"`
	Activa         bool     `json:"activa"`
}

// GuardarImpresora crea o actualiza una comandera de la sede.
func (s *Service) GuardarImpresora(empresaID, sedeID, actor, origen string, b ImpresoraBody) (cocina.Impresora, error) {
	if s.impresoras == nil {
		return cocina.Impresora{}, ErrCocinaNoDisponible
	}
	conexion := cocina.NormalizarConexion(b.Conexion)
	if !cocina.ConexionValida(conexion) {
		return cocina.Impresora{}, ErrConexionInvalida
	}
	host := strings.TrimSpace(b.Host)
	puerto := b.Puerto
	if conexion == cocina.ConexionRed {
		if host == "" {
			return cocina.Impresora{}, ErrImpresoraSinHost
		}
		if puerto <= 0 {
			puerto = 9100 // RAW/JetDirect por defecto
		}
	}
	ancho := b.AnchoMM
	if ancho != 58 && ancho != 80 {
		ancho = 80
	}
	nombre := strings.TrimSpace(b.Nombre)
	if nombre == "" {
		nombre = "Cocina"
	}

	existentes := s.impresoras.List(empresaID, sedeID)
	// La PRIMERA comandera de una sede es la predeterminada por fuerza: si no, no
	// habría dónde imprimir lo que no encaje en ningún rubro.
	predeterminada := b.Predeterminada || len(existentes) == 0

	imp := cocina.Impresora{
		ID: strings.TrimSpace(b.ID), EmpresaID: empresaID, SedeID: sedeID,
		Nombre: nombre, Conexion: conexion, Host: host, Puerto: puerto, AnchoMM: ancho,
		Rubros: limpiarRubros(b.Rubros), Predeterminada: predeterminada,
		Activa: b.Activa, Actualizada: ahora(),
	}

	var out cocina.Impresora
	if imp.ID == "" {
		out = s.impresoras.Create(imp)
	} else {
		if _, ok := s.impresoras.ByID(empresaID, imp.ID); !ok {
			return cocina.Impresora{}, ErrImpresoraNoExiste
		}
		var ok bool
		out, ok = s.impresoras.Update(imp)
		if !ok {
			return cocina.Impresora{}, ErrImpresoraNoExiste
		}
	}
	// Exactamente UNA predeterminada por sede: marcar esta degrada a las demás.
	if out.Predeterminada {
		for _, otra := range existentes {
			if otra.ID != out.ID && otra.Predeterminada {
				otra.Predeterminada = false
				otra.Actualizada = ahora()
				s.impresoras.Update(otra)
			}
		}
	}
	s.audit.Append(evento(empresaID, actor, origen, "restaurante.comandera.guardar", out.Nombre,
		conexion+" · rubros: "+strings.Join(out.Rubros, ", ")))
	return out, nil
}

// EliminarImpresora borra una comandera. No deja a la sede sin predeterminada.
func (s *Service) EliminarImpresora(empresaID, sedeID, id, actor, origen string) error {
	if s.impresoras == nil {
		return ErrCocinaNoDisponible
	}
	imp, ok := s.impresoras.ByID(empresaID, id)
	if !ok {
		return ErrImpresoraNoExiste
	}
	if imp.Predeterminada {
		// Si queda otra, hereda; si era la última, se permite borrar (la sede se queda
		// sin comanderas, que es un estado válido: se ven las comandas en pantalla).
		var heredera *cocina.Impresora
		for _, otra := range s.impresoras.List(empresaID, sedeID) {
			if otra.ID != id {
				o := otra
				heredera = &o
				break
			}
		}
		if heredera != nil {
			heredera.Predeterminada = true
			heredera.Actualizada = ahora()
			s.impresoras.Update(*heredera)
		}
	}
	if !s.impresoras.Delete(empresaID, id) {
		return ErrImpresoraNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "restaurante.comandera.eliminar", imp.Nombre, ""))
	return nil
}

// ComandaImpresa es el ticket que sale por UNA comandera: sus renglones y a dónde va.
type ComandaImpresa struct {
	Impresora cocina.Impresora `json:"impresora"`
	Items     []cuenta.Item    `json:"items"`
}

// repartirComanda separa los renglones de una comanda por comandera, según el RUBRO del
// producto de cada renglón. Lo que no encaja en ningún rubro configurado va a la
// predeterminada — así un producto de un rubro nuevo no se pierde en el camino.
//
// Sin comanderas configuradas devuelve un solo ticket sin impresora: la comanda se ve en
// pantalla igual, que es como se opera mientras se configura el local.
func (s *Service) repartirComanda(empresaID, sedeID string, items []cuenta.Item) []ComandaImpresa {
	if len(items) == 0 {
		return nil
	}
	impresoras := s.Impresoras(empresaID, sedeID)
	if len(impresoras) == 0 {
		return []ComandaImpresa{{Items: items}}
	}

	// Rubro de cada SKU (una sola pasada por el catálogo).
	rubroDe := map[string]string{}
	if s.productos != nil {
		for _, p := range s.productos.List(empresaID) {
			rubroDe[p.SKU] = p.Rubro
		}
	}

	porImpresora := map[string][]cuenta.Item{}
	var predeterminada *cocina.Impresora
	for i := range impresoras {
		if impresoras[i].Predeterminada {
			predeterminada = &impresoras[i]
			break
		}
	}
	if predeterminada == nil {
		predeterminada = &impresoras[0]
	}

	for _, it := range items {
		destino := predeterminada
		for i := range impresoras {
			if impresoras[i].ImprimeRubro(rubroDe[it.SKU]) {
				destino = &impresoras[i]
				break
			}
		}
		porImpresora[destino.ID] = append(porImpresora[destino.ID], it)
	}

	out := make([]ComandaImpresa, 0, len(porImpresora))
	for _, imp := range impresoras {
		if its := porImpresora[imp.ID]; len(its) > 0 {
			out = append(out, ComandaImpresa{Impresora: imp, Items: its})
		}
	}
	return out
}

// limpiarRubros quita vacíos y duplicados (sin distinguir mayúsculas).
func limpiarRubros(xs []string) []string {
	visto := map[string]bool{}
	out := []string{}
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if x == "" {
			continue
		}
		k := strings.ToLower(x)
		if visto[k] {
			continue
		}
		visto[k] = true
		out = append(out, x)
	}
	return out
}
