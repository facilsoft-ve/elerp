package application

import (
	"fmt"
	"strings"

	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/inventario"
)

// Carga masiva de productos (CSV) — UPSERT con validación previa.
//
// El flujo tiene dos fases sobre el MISMO endpoint:
//   - confirmar=false ⇒ VISTA PREVIA (dry run): valida cada fila y devuelve su
//     estado (nuevo | actualiza | error) sin tocar nada. Una fila cuyo SKU ya
//     existe se marca "actualiza" para que el usuario lo revise y confirme.
//   - confirmar=true  ⇒ APLICA, pero es TODO-O-NADA: si alguna fila trae error no
//     se aplica ninguna (así el usuario corrige el archivo y reintenta sin dejar
//     una carga a medias). Los nuevos se crean; los existentes se actualizan.
//
// Reglas de negocio del pedido:
//   - La unidad, si se declara, DEBE existir en el maestro de unidades de la
//     empresa (se rechaza un símbolo desconocido). Vacío ⇒ unidad por defecto.
//   - La existencia inicial sólo se carga en productos NUEVOS (ajuste auditado
//     "carga inicial"); en un producto existente se ignora para no duplicar stock
//     —el ajuste de existencia de un producto ya cargado se hace por su Kardex—.

// Estados por fila del resultado de importación.
const (
	FilaImportNueva     = "nuevo"
	FilaImportActualiza = "actualiza"
	FilaImportError     = "error"
)

// FilaImportacionProducto es una fila cruda de la carga masiva (ya parseada del
// CSV por el adaptador HTTP). Los strings llegan sin normalizar.
type FilaImportacionProducto struct {
	SKU               string
	Nombre            string
	Rubro             string
	Unidad            string // símbolo del maestro; vacío ⇒ unidad por defecto
	TipoVenta         string // "" | unidad | peso
	Precio            float64
	Moneda            string // "" ⇒ moneda principal de la empresa
	ExentoIVA         bool
	CodigoBarras      string
	ExistenciaInicial float64
}

// FilaResultadoImport es el veredicto de una fila (para la vista previa y el
// resumen de aplicación).
type FilaResultadoImport struct {
	Indice  int    `json:"indice"` // fila en el archivo (0-based)
	SKU     string `json:"sku"`
	Nombre  string `json:"nombre"`
	Estado  string `json:"estado"`  // nuevo | actualiza | error
	Mensaje string `json:"mensaje"` // motivo del error, o nota (p. ej. existencia ignorada)
}

// ResultadoImportacion resume la corrida completa.
type ResultadoImportacion struct {
	Filas                []FilaResultadoImport `json:"filas"`
	Nuevos               int                   `json:"nuevos"`
	Actualizados         int                   `json:"actualizados"`
	Errores              int                   `json:"errores"`
	RequiereConfirmacion bool                  `json:"requiereConfirmacion"` // hay SKUs existentes por actualizar
	Aplicado             bool                  `json:"aplicado"`             // se escribieron los cambios
}

// filaPreparada guarda lo ya normalizado de una fila válida para no revalidar al
// aplicar.
type filaPreparada struct {
	sku, nombre, rubro, unidad, moneda, tipoVenta string
	precio, existencia                            float64
	exento                                        bool
	codigoBarras                                  string
	existe                                        bool
	valida                                        bool
}

// ImportarProductos ejecuta la carga masiva. Ver el comentario del bloque para el
// contrato (dry run vs aplicar, todo-o-nada, unidad en maestro, existencia inicial).
func (s *Service) ImportarProductos(empresaID, sedeID, actor, origen string, filas []FilaImportacionProducto, confirmar bool) (ResultadoImportacion, error) {
	res := ResultadoImportacion{Filas: make([]FilaResultadoImport, 0, len(filas))}
	if len(filas) == 0 {
		return res, nil
	}

	// Símbolos válidos del maestro (activos), indexados en minúsculas para comparar
	// sin distinguir mayúsculas y recuperar el símbolo canónico.
	unidadesOK := map[string]string{}
	for _, u := range s.UnidadesActivas(empresaID) {
		unidadesOK[strings.ToLower(strings.TrimSpace(u.Simbolo))] = u.Simbolo
	}
	monPrincipal := s.MonedaPrincipalDe(empresaID)
	admiteUSD := s.admitePreciosUSD(empresaID)

	// Duplicados DENTRO del archivo (SKU y código de barras) — se detectan aparte de
	// los choques contra lo ya guardado.
	skuEnLote := map[string]int{}
	cbEnLote := map[string]int{}

	prep := make([]filaPreparada, len(filas))
	for i, f := range filas {
		sku := strings.TrimSpace(f.SKU)
		nombre := strings.TrimSpace(f.Nombre)
		fr := FilaResultadoImport{Indice: i, SKU: sku, Nombre: nombre}
		fail := func(msg string) {
			fr.Estado = FilaImportError
			fr.Mensaje = msg
			res.Errores++
		}

		switch {
		case sku == "":
			fail("El SKU es obligatorio.")
		case nombre == "":
			fail("El nombre es obligatorio.")
		}

		if fr.Estado != FilaImportError {
			if j, dup := skuEnLote[strings.ToLower(sku)]; dup {
				fail(fmt.Sprintf("SKU repetido en el archivo (ya aparece en la fila %d).", j+1))
			} else {
				skuEnLote[strings.ToLower(sku)] = i
			}
		}

		// Tipo de venta: "" / "unidad" / "peso" (cualquier otro es error). Un producto
		// por peso se cobra por kg y su unidad base la fuerza el servicio a "kg", así
		// que en ese caso la unidad del CSV es irrelevante y no se valida contra el
		// maestro.
		tipoVenta := strings.ToLower(strings.TrimSpace(f.TipoVenta))
		if fr.Estado != FilaImportError && tipoVenta != "" &&
			tipoVenta != inventario.TipoVentaUnidad && tipoVenta != inventario.TipoVentaPeso {
			fail(fmt.Sprintf("Tipo de venta %q inválido (usa 'unidad' o 'peso').", f.TipoVenta))
		}

		// Unidad: si se declara, debe existir en el maestro. Vacío ⇒ por defecto.
		// (Se omite la validación cuando el producto es por peso: la unidad será "kg".)
		unidad := strings.TrimSpace(f.Unidad)
		if fr.Estado != FilaImportError && tipoVenta != inventario.TipoVentaPeso && unidad != "" {
			if canon, ok := unidadesOK[strings.ToLower(unidad)]; ok {
				unidad = canon
			} else {
				fail(fmt.Sprintf("La unidad %q no está en el maestro de unidades; créala primero en Configuración.", unidad))
			}
		}
		if unidad == "" {
			unidad = inventario.UnidadUnidad
		}

		if fr.Estado != FilaImportError && f.Precio < 0 {
			fail("El precio no puede ser negativo.")
		}

		moneda := strings.TrimSpace(f.Moneda)
		if moneda == "" {
			moneda = monPrincipal
		}
		if fr.Estado != FilaImportError {
			if !empresa.MonedaValida(moneda) {
				fail(fmt.Sprintf("Moneda inválida %q (usa VES o USD).", moneda))
			} else if moneda == empresa.MonedaUSD && !admiteUSD {
				fail("La empresa no tiene habilitados los precios en US$.")
			}
		}

		cb := strings.TrimSpace(f.CodigoBarras)
		if fr.Estado != FilaImportError && cb != "" {
			if j, dup := cbEnLote[strings.ToLower(cb)]; dup {
				fail(fmt.Sprintf("Código de barras repetido en el archivo (fila %d).", j+1))
			} else {
				cbEnLote[strings.ToLower(cb)] = i
				if otro, ok := s.PorCodigoBarras(empresaID, cb); ok && !strings.EqualFold(otro.SKU, sku) {
					fail(fmt.Sprintf("El código de barras ya lo usa %q.", otro.Nombre))
				}
			}
		}

		if fr.Estado != FilaImportError && f.ExistenciaInicial < 0 {
			fail("La existencia inicial no puede ser negativa.")
		}

		_, existe := s.productos.BySKU(empresaID, sku)
		if fr.Estado != FilaImportError {
			if existe {
				fr.Estado = FilaImportActualiza
				res.Actualizados++
				if f.ExistenciaInicial > 0 {
					fr.Mensaje = "La existencia inicial se ignora: el producto ya existe (ajústalo por su Kardex)."
				}
			} else {
				fr.Estado = FilaImportNueva
				res.Nuevos++
			}
			prep[i] = filaPreparada{
				sku: sku, nombre: nombre, rubro: strings.TrimSpace(f.Rubro),
				unidad: unidad, moneda: moneda, tipoVenta: tipoVenta, precio: f.Precio,
				existencia: f.ExistenciaInicial, exento: f.ExentoIVA,
				codigoBarras: cb, existe: existe, valida: true,
			}
		}

		res.Filas = append(res.Filas, fr)
	}

	res.RequiereConfirmacion = res.Actualizados > 0

	// Vista previa, o hay errores ⇒ no se aplica nada (todo-o-nada).
	if !confirmar || res.Errores > 0 {
		return res, nil
	}

	for i := range prep {
		p := prep[i]
		if !p.valida {
			continue
		}
		if p.existe {
			cambios := CambiosProducto{
				Nombre: p.nombre, Rubro: p.rubro, UnidadBase: p.unidad, TipoVenta: p.tipoVenta,
				Precio: p.precio, Moneda: p.moneda,
				ExentoIVA:    boolPtr(p.exento),
				CodigoBarras: strPtr(p.codigoBarras),
			}
			if _, err := s.ActualizarProducto(empresaID, actor, origen, p.sku, cambios); err != nil {
				res.Filas[i].Estado = FilaImportError
				res.Filas[i].Mensaje = err.Error()
				res.Errores++
				res.Actualizados--
			}
			continue
		}
		// Producto nuevo: alta + (opcional) existencia inicial como ajuste auditado.
		nuevo := inventario.Producto{
			SKU: p.sku, Nombre: p.nombre, Rubro: p.rubro, UnidadBase: p.unidad, TipoVenta: p.tipoVenta,
			Precio: p.precio, Moneda: p.moneda, ExentoIVA: p.exento, CodigoBarras: p.codigoBarras,
		}
		if _, err := s.CrearProducto(empresaID, actor, origen, nuevo); err != nil {
			res.Filas[i].Estado = FilaImportError
			res.Filas[i].Mensaje = err.Error()
			res.Errores++
			res.Nuevos--
			continue
		}
		if p.existencia > 0 {
			if _, err := s.Ajustar(empresaID, sedeID, "", p.sku, "Carga inicial (importación masiva)", p.existencia, actor, origen); err != nil {
				// El producto quedó creado; sólo falló la carga de existencia. Se
				// reporta como nota, no invalida el alta.
				res.Filas[i].Mensaje = "Producto creado, pero no se pudo cargar la existencia inicial: " + err.Error()
			}
		}
	}
	res.Aplicado = res.Errores == 0
	s.audit.Append(evento(empresaID, actor, origen, "inventario.importacion",
		fmt.Sprintf("nuevos=%d actualizados=%d", res.Nuevos, res.Actualizados), ""))
	return res, nil
}

func boolPtr(b bool) *bool    { return &b }
func strPtr(s string) *string { return &s }
