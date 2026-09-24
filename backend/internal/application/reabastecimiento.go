package application

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mornix/elerp/internal/domain/compra"
	"github.com/mornix/elerp/internal/domain/inventario"
)

// REABASTECIMIENTO. Ver domain/inventario/reabastecimiento.go para el porqué.
//
// LA REGLA NO COMPRA. Prepara una SOLICITUD de compra —la que se manda a los
// proveedores para que coticen— y una persona decide. Una regla que emitiera
// órdenes sola convertiría un mínimo mal tecleado en una deuda con un proveedor, y
// nadie revisa lo que funciona solo hasta que llega la factura.
//
// SE MIRA SIEMPRE ANTES DE GENERAR, igual que en el conteo físico: hay una
// revisión que no escribe nada y dice qué se pediría y por qué.
var (
	// ErrReglaProductoNoExiste: la regla apunta a un SKU que no está en el catálogo.
	ErrReglaProductoNoExiste = errors.New("el producto de la regla no existe")
	// ErrReglaNiveles: el máximo tiene que superar al mínimo. Si fueran iguales,
	// cada venta dispararía un pedido del tamaño de esa venta.
	ErrReglaNiveles = errors.New("el máximo tiene que ser mayor que el mínimo")
	// ErrReglaNegativa: un nivel negativo no describe ningún inventario.
	ErrReglaNegativa = errors.New("los niveles no pueden ser negativos")
	// ErrReglaDuplicada: dos reglas para el mismo producto y ámbito se pisarían, y
	// cuál gana dependería del orden de lectura.
	ErrReglaDuplicada = errors.New("ya hay una regla para ese producto en ese ámbito")
	// ErrReglaNoExiste: la regla no está o no es de esta empresa.
	ErrReglaNoExiste = errors.New("la regla no existe")
	// ErrSinNadaQuePedir: ninguna regla está por debajo de su mínimo.
	ErrSinNadaQuePedir = errors.New("no hay nada bajo mínimos: no hay solicitud que generar")
)

// ConReglasReabastecimiento cablea el maestro. Sin él no se repone nada solo, que
// es como funcionaba antes.
func (s *Service) ConReglasReabastecimiento(r inventario.ReglaReabastecimientoRepo) *Service {
	s.reglasReabastecimiento = r
	return s
}

// ReglasReabastecimiento lista las reglas de una empresa, ordenadas por SKU.
func (s *Service) ReglasReabastecimiento(empresaID, sedeID string) []inventario.ReglaReabastecimiento {
	if s.reglasReabastecimiento == nil {
		return []inventario.ReglaReabastecimiento{}
	}
	out := []inventario.ReglaReabastecimiento{}
	for _, r := range s.reglasReabastecimiento.List(empresaID) {
		if sedeID != "" && r.SedeID != sedeID {
			continue
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].SKU < out[j].SKU })
	return out
}

// CrearReglaReabastecimiento da de alta una regla.
func (s *Service) CrearReglaReabastecimiento(empresaID, actor, origen string, r inventario.ReglaReabastecimiento) (inventario.ReglaReabastecimiento, error) {
	if s.reglasReabastecimiento == nil {
		return inventario.ReglaReabastecimiento{}, errors.New("el reabastecimiento no está disponible")
	}
	p, ok := s.productos.BySKU(empresaID, r.SKU)
	if !ok {
		return inventario.ReglaReabastecimiento{}, fmt.Errorf("%w: %s", ErrReglaProductoNoExiste, r.SKU)
	}
	// Un combo o un plato no se reponen: lo que se compra son sus componentes, y
	// una regla sobre ellos pediría un producto que nunca entra al almacén.
	if p.EsCombo || p.EsPlato {
		return inventario.ReglaReabastecimiento{}, fmt.Errorf("%w: %s", ErrComboNoStockeable, r.SKU)
	}
	if err := validarNivelesRegla(r); err != nil {
		return inventario.ReglaReabastecimiento{}, err
	}
	for _, otra := range s.ReglasReabastecimiento(empresaID, "") {
		if otra.Activa && otra.ProductoID == p.ID && otra.SedeID == r.SedeID && otra.AlmacenID == r.AlmacenID {
			return inventario.ReglaReabastecimiento{}, fmt.Errorf("%w: %s", ErrReglaDuplicada, r.SKU)
		}
	}
	r.EmpresaID, r.ProductoID, r.SKU = empresaID, p.ID, p.SKU
	r.Activa = true
	r.Creada = ahora()
	out := s.reglasReabastecimiento.Create(r)
	s.audit.Append(evento(empresaID, actor, origen, "inventario.reabastecimiento.crear", out.SKU,
		fmt.Sprintf("min %.2f máx %.2f", out.Minimo, out.Maximo)))
	return out, nil
}

// ActualizarReglaReabastecimiento edita los niveles o desactiva la regla.
func (s *Service) ActualizarReglaReabastecimiento(empresaID, id, actor, origen string, cambios inventario.ReglaReabastecimiento) (inventario.ReglaReabastecimiento, error) {
	if s.reglasReabastecimiento == nil {
		return inventario.ReglaReabastecimiento{}, errors.New("el reabastecimiento no está disponible")
	}
	r, ok := s.reglasReabastecimiento.ByID(empresaID, id)
	if !ok {
		return inventario.ReglaReabastecimiento{}, ErrReglaNoExiste
	}
	r.Minimo, r.Maximo, r.Multiplo = cambios.Minimo, cambios.Maximo, cambios.Multiplo
	r.ProveedorID, r.Activa = cambios.ProveedorID, cambios.Activa
	if err := validarNivelesRegla(r); err != nil {
		return inventario.ReglaReabastecimiento{}, err
	}
	out, ok := s.reglasReabastecimiento.Update(r)
	if !ok {
		return inventario.ReglaReabastecimiento{}, ErrReglaNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "inventario.reabastecimiento.editar", out.SKU,
		fmt.Sprintf("min %.2f máx %.2f", out.Minimo, out.Maximo)))
	return out, nil
}

func validarNivelesRegla(r inventario.ReglaReabastecimiento) error {
	if r.Minimo < 0 || r.Maximo < 0 || r.Multiplo < 0 {
		return ErrReglaNegativa
	}
	if r.Maximo <= r.Minimo {
		return ErrReglaNiveles
	}
	return nil
}

// FilaReabastecimiento es lo que hay que pedir de un producto, y por qué.
//
// Lleva las tres cifras que explican la decisión —lo que hay, lo comprometido y lo
// que viene en camino— porque la pregunta que todo el mundo hace al ver una
// sugerencia de compra es «¿por qué esto?». Sin el desglose, la respuesta es
// «porque lo dice el sistema», y entonces se deja de usar.
type FilaReabastecimiento struct {
	SKU         string  `json:"sku"`
	Nombre      string  `json:"nombre"`
	SedeID      string  `json:"sedeId"`
	AlmacenID   string  `json:"almacenId,omitempty"`
	Existencia  float64 `json:"existencia"`
	Apartado    float64 `json:"apartado"`
	Disponible  float64 `json:"disponible"`
	EnCamino    float64 `json:"enCamino"`
	Cobertura   float64 `json:"cobertura"`
	Minimo      float64 `json:"minimo"`
	Maximo      float64 `json:"maximo"`
	APedir      float64 `json:"aPedir"`
	ProveedorID string  `json:"proveedorId,omitempty"`
	Proveedor   string  `json:"proveedor,omitempty"`
	// YaSolicitado avisa de que ese producto ya está en una solicitud abierta. NO
	// impide pedirlo —puede que aquella se quede sin respuesta— pero verlo evita
	// generar dos veces lo mismo sin darse cuenta.
	YaSolicitado bool `json:"yaSolicitado,omitempty"`
}

// RevisionReabastecimiento es el resultado de mirar, sin tocar nada.
type RevisionReabastecimiento struct {
	Fecha    string                 `json:"fecha"`
	SedeID   string                 `json:"sedeId"`
	Reglas   int                    `json:"reglas"`
	Filas    []FilaReabastecimiento `json:"filas"`
	Cubierto int                    `json:"cubierto"`
}

// enCaminoDe suma lo pedido a proveedores y aún no recibido de un producto.
//
// Solo cuentan las órdenes CONFIRMADAS o recibidas a medias: un borrador no es un
// compromiso con nadie y una cancelada dejó de serlo. Contar un borrador haría que
// la regla no pidiera nada mientras alguien tuviera una orden a medio escribir.
func (s *Service) enCaminoDe(empresaID, sedeID, productoID string) float64 {
	if s.ordenesCompra == nil {
		return 0
	}
	total := 0.0
	for _, o := range s.ordenesCompra.List(empresaID) {
		if o.Estado != compra.OCConfirmada && o.Estado != compra.OCRecibidaParcial {
			continue
		}
		if sedeID != "" && o.SedeID != sedeID {
			continue
		}
		for _, l := range o.Lineas {
			if l.ProductoID != productoID {
				continue
			}
			if p := l.Cantidad - l.CantidadRecibida; p > 0 {
				total += p
			}
		}
	}
	return round2(total)
}

// productosEnSolicitudesAbiertas devuelve los SKU que ya figuran en una solicitud
// que todavía no se convirtió en orden.
func (s *Service) productosEnSolicitudesAbiertas(empresaID, sedeID string) map[string]bool {
	out := map[string]bool{}
	if s.solicitudes == nil {
		return out
	}
	for _, sol := range s.solicitudes.List(empresaID) {
		if sol.OrdenGeneradaID != "" {
			continue // ya se convirtió: lo pendiente lo cuenta enCaminoDe
		}
		if sedeID != "" && sol.SedeID != sedeID {
			continue
		}
		for _, l := range sol.Lineas {
			out[l.SKU] = true
		}
	}
	return out
}

// RevisarReabastecimiento dice qué habría que pedir, SIN crear nada.
//
// Existe por lo mismo que la vista previa del conteo: una sugerencia de compra que
// aparece ya convertida en documento no se lee, se firma.
func (s *Service) RevisarReabastecimiento(empresaID, sedeID string) RevisionReabastecimiento {
	res := RevisionReabastecimiento{Fecha: ahora(), SedeID: sedeID, Filas: []FilaReabastecimiento{}}
	reglas := s.ReglasReabastecimiento(empresaID, sedeID)
	res.Reglas = len(reglas)
	yaPedidos := s.productosEnSolicitudesAbiertas(empresaID, sedeID)

	for _, r := range reglas {
		if !r.Activa {
			continue
		}
		p, ok := s.productos.ByID(empresaID, r.ProductoID)
		if !ok {
			continue
		}
		existencia, _ := fold(s.movimientos.List(empresaID, inventario.FiltroMovimiento{
			SedeID: r.SedeID, AlmacenID: r.AlmacenID, ProductoID: r.ProductoID,
		}))
		apartado := s.Apartado(empresaID, r.SedeID, r.ProductoID)
		disponible := round2(existencia - apartado)
		enCamino := s.enCaminoDe(empresaID, r.SedeID, r.ProductoID)
		cobertura := round2(disponible + enCamino)

		aPedir := r.CantidadAReponer(cobertura)
		if aPedir <= 0.0001 {
			res.Cubierto++
			continue
		}
		fila := FilaReabastecimiento{
			SKU: p.SKU, Nombre: p.Nombre, SedeID: r.SedeID, AlmacenID: r.AlmacenID,
			Existencia: round2(existencia), Apartado: apartado, Disponible: disponible,
			EnCamino: enCamino, Cobertura: cobertura,
			Minimo: r.Minimo, Maximo: r.Maximo, APedir: round2(aPedir),
			ProveedorID: r.ProveedorID, YaSolicitado: yaPedidos[p.SKU],
		}
		if r.ProveedorID != "" {
			if pr, ok := s.Proveedor(empresaID, r.ProveedorID); ok {
				fila.Proveedor = pr.Nombre
			}
		}
		res.Filas = append(res.Filas, fila)
	}
	sort.SliceStable(res.Filas, func(i, j int) bool { return res.Filas[i].SKU < res.Filas[j].SKU })
	return res
}

// GenerarSolicitudesDeReabastecimiento crea las solicitudes de compra con lo que
// está bajo mínimos.
//
// SE AGRUPA POR PROVEEDOR, y no es cosmético: una solicitud es un pedido de
// presupuesto que se le manda a alguien. Mezclar en un mismo documento productos de
// proveedores distintos obliga a quien la recibe a cotizar cosas que no vende, y a
// quien la emite a partirla a mano antes de enviarla.
//
// Lo que no tiene proveedor asignado va en una solicitud aparte, sin destinatario:
// es exactamente lo que hay que decidir al pedir presupuesto.
func (s *Service) GenerarSolicitudesDeReabastecimiento(empresaID, sedeID, actor, origen string) ([]compra.SolicitudCompra, error) {
	rev := s.RevisarReabastecimiento(empresaID, sedeID)
	if len(rev.Filas) == 0 {
		return nil, ErrSinNadaQuePedir
	}
	porProveedor := map[string][]FilaReabastecimiento{}
	orden := []string{}
	for _, f := range rev.Filas {
		if _, visto := porProveedor[f.ProveedorID]; !visto {
			orden = append(orden, f.ProveedorID)
		}
		porProveedor[f.ProveedorID] = append(porProveedor[f.ProveedorID], f)
	}
	sort.Strings(orden)

	out := []compra.SolicitudCompra{}
	for _, provID := range orden {
		filas := porProveedor[provID]
		lineas := make([]LineaSolicitudEntrada, 0, len(filas))
		skus := make([]string, 0, len(filas))
		for _, f := range filas {
			lineas = append(lineas, LineaSolicitudEntrada{SKU: f.SKU, Cantidad: f.APedir})
			skus = append(skus, f.SKU)
		}
		provs := []string{}
		if provID != "" {
			provs = append(provs, provID)
		}
		sol, err := s.CrearSolicitud(empresaID, sedeID, actor, origen, EntradaSolicitud{
			SedeID: sedeID,
			// La nota dice de dónde salió. Una solicitud que aparece sin explicación
			// obliga a quien la revisa a reconstruir por qué se pidió justo eso.
			Notas:       "Generada por reabastecimiento: " + strings.Join(skus, ", ") + " bajo mínimos.",
			Lineas:      lineas,
			Proveedores: provs,
		})
		if err != nil {
			return out, fmt.Errorf("generando la solicitud: %w", err)
		}
		out = append(out, sol)
	}
	s.audit.Append(evento(empresaID, actor, origen, "inventario.reabastecimiento.generar",
		fmt.Sprintf("%d solicitud(es)", len(out)), fmt.Sprintf("%d producto(s) bajo mínimos", len(rev.Filas))))
	return out, nil
}
