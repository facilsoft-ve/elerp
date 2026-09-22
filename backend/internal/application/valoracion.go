package application

import (
	"sort"

	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/inventario"
)

// INFORME DE VALORACIÓN.
//
// Responde dos preguntas que el Libro de Inventario no responde, porque él
// consolida el contribuyente entero:
//
//  1. DÓNDE está el valor — por almacén y ubicación. Es lo que se lleva al conteo
//     físico: sin el desglose, contar un depósito exige recorrer producto por
//     producto adivinando cuáles están ahí.
//
//  2. SI LA CONTABILIDAD Y EL ANAQUEL DICEN LO MISMO — por cuenta contable. Esta
//     es la que cierra la cuenta de inventario por rubro: separar el inventario en
//     varias cuentas no sirve de nada si nadie puede comprobar que cada una
//     corresponde a mercancía que existe. Y el descuadre que busca es justo el que
//     no se ve: el balance cuadra igual esté el saldo en la cuenta que sea.
//
// EL COSTO ES DEL PRODUCTO EN LA SEDE, no de la casilla. Con promedio ponderado el
// costo no depende de en qué estante esté la caja, así que una ubicación se valora
// con la cantidad que tiene por el promedio de su producto. Un costo por ubicación
// sería otro sistema de valoración, no un desglose.

// FilaValoracion es la existencia valorizada de un producto en una casilla
// concreta: almacén y, dentro de él, ubicación.
type FilaValoracion struct {
	AlmacenID     string  `json:"almacenId"`
	AlmacenNombre string  `json:"almacenNombre"`
	UbicacionID   string  `json:"ubicacionId"`
	Ubicacion     string  `json:"ubicacion"`
	SKU           string  `json:"sku"`
	Nombre        string  `json:"nombre"`
	Rubro         string  `json:"rubro,omitempty"`
	Cuenta        string  `json:"cuenta"`
	Cantidad      float64 `json:"cantidad"`
	CostoPromedio float64 `json:"costoPromedio"`
	Valor         float64 `json:"valor"`
}

// TotalValoracion agrupa el valor bajo una clave (un almacén, una cuenta).
type TotalValoracion struct {
	Clave    string  `json:"clave"`
	Nombre   string  `json:"nombre"`
	Valor    float64 `json:"valor"`
	Lineas   int     `json:"lineas"`
	Contable float64 `json:"contable,omitempty"`
	// Diferencia es Valor − Contable. Solo la llevan los totales por cuenta, y es
	// EL dato del informe: mientras sea cero, el inventario y la contabilidad
	// cuentan la misma historia.
	Diferencia float64 `json:"diferencia,omitempty"`
	Cuadra     bool    `json:"cuadra"`
}

// ValoracionResult es el informe completo.
type ValoracionResult struct {
	Fecha        string            `json:"fecha"`
	SedeID       string            `json:"sedeId,omitempty"`
	Filas        []FilaValoracion  `json:"filas"`
	PorAlmacen   []TotalValoracion `json:"porAlmacen"`
	PorCuenta    []TotalValoracion `json:"porCuenta"`
	ValorTotal   float64           `json:"valorTotal"`
	TodoCuadrado bool              `json:"todoCuadrado"`
}

// Valoracion arma el informe. `sedeID` vacío mira toda la empresa.
//
// La comparación con la contabilidad solo se hace SIN filtrar por sede: el saldo
// de una cuenta contable es de la empresa entera, y contrastarlo con el inventario
// de una sola sede daría una diferencia que no significa nada — y que alguien
// intentaría cuadrar.
func (s *Service) Valoracion(empresaID, sedeID string) ValoracionResult {
	res := ValoracionResult{
		Fecha: ahora(), SedeID: sedeID,
		Filas: []FilaValoracion{}, PorAlmacen: []TotalValoracion{}, PorCuenta: []TotalValoracion{},
	}
	if s.productos == nil || s.movimientos == nil {
		return res
	}

	type clave struct{ Almacen, Ubicacion, Producto string }
	cantidades := map[clave]float64{}
	// Costo promedio por (producto, sede): con varias sedes, el mismo producto puede
	// tener promedios distintos y mezclarlos daría un valor que no es de nadie.
	costo := map[string]float64{}
	sedeDe := map[string]string{}

	for _, p := range s.productos.List(empresaID) {
		if p.EsCombo || p.EsPlato {
			continue // no se stockean: su existencia es la de sus componentes
		}
		movs := s.movimientos.List(empresaID, inventario.FiltroMovimiento{SedeID: sedeID, ProductoID: p.ID})
		for _, m := range movs {
			if m.Tipo == inventario.MovRevaluacion {
				continue // mueve valor, no unidades: ya está dentro del costo promedio
			}
			cantidades[clave{m.AlmacenID, m.UbicacionID, p.ID}] += m.Cantidad
			sedeDe[m.SedeID] = m.SedeID
		}
		_, avg := fold(movs)
		costo[p.ID] = avg
	}

	valorPorAlmacen := map[string]float64{}
	lineasPorAlmacen := map[string]int{}
	valorPorCuenta := map[string]float64{}
	lineasPorCuenta := map[string]int{}

	for k, cant := range cantidades {
		if cant > -0.0001 && cant < 0.0001 {
			continue
		}
		p, ok := s.productos.ByID(empresaID, k.Producto)
		if !ok {
			continue
		}
		cuenta := s.cuentaInventarioDe(empresaID, p.ID)
		f := FilaValoracion{
			AlmacenID: k.Almacen, UbicacionID: k.Ubicacion,
			SKU: p.SKU, Nombre: p.Nombre, Rubro: p.Rubro, Cuenta: cuenta,
			Cantidad: round2(cant), CostoPromedio: round2(costo[p.ID]),
			Valor: round2(cant * costo[p.ID]),
		}
		f.AlmacenNombre = "Sin almacén"
		if a, ok := s.almacenes.ByID(empresaID, k.Almacen); ok {
			f.AlmacenNombre = a.Nombre
		}
		f.Ubicacion = "SIN UBICAR"
		if k.Ubicacion != "" && s.ubicaciones != nil {
			if u, ok := s.ubicaciones.ByID(empresaID, k.Ubicacion); ok {
				f.Ubicacion = u.Codigo
			} else {
				f.Ubicacion = k.Ubicacion
			}
		}
		res.Filas = append(res.Filas, f)
		res.ValorTotal += f.Valor
		valorPorAlmacen[f.AlmacenNombre] += f.Valor
		lineasPorAlmacen[f.AlmacenNombre]++
		valorPorCuenta[cuenta] += f.Valor
		lineasPorCuenta[cuenta]++
	}
	res.ValorTotal = round2(res.ValorTotal)

	sort.SliceStable(res.Filas, func(i, j int) bool {
		if res.Filas[i].AlmacenNombre != res.Filas[j].AlmacenNombre {
			return res.Filas[i].AlmacenNombre < res.Filas[j].AlmacenNombre
		}
		if res.Filas[i].Ubicacion != res.Filas[j].Ubicacion {
			return res.Filas[i].Ubicacion < res.Filas[j].Ubicacion
		}
		return res.Filas[i].SKU < res.Filas[j].SKU
	})

	for nombre, valor := range valorPorAlmacen {
		res.PorAlmacen = append(res.PorAlmacen, TotalValoracion{
			Clave: nombre, Nombre: nombre, Valor: round2(valor), Lineas: lineasPorAlmacen[nombre], Cuadra: true,
		})
	}
	sort.SliceStable(res.PorAlmacen, func(i, j int) bool { return res.PorAlmacen[i].Nombre < res.PorAlmacen[j].Nombre })

	res.TodoCuadrado = true
	for cod, valor := range valorPorCuenta {
		t := TotalValoracion{Clave: cod, Valor: round2(valor), Lineas: lineasPorCuenta[cod], Cuadra: true}
		if c, ok := s.cuentaContable(empresaID, cod); ok {
			t.Nombre = c.Nombre
		} else {
			t.Nombre = cod
		}
		// La comparación con el diario, solo sobre la empresa entera. Con una sede
		// filtrada la diferencia sería el inventario de las demás sedes, y eso no es
		// un descuadre: es otra pregunta.
		if sedeID == "" {
			t.Contable = s.saldoContableDe(empresaID, cod)
			t.Diferencia = round2(t.Valor - t.Contable)
			t.Cuadra = t.Diferencia > -0.05 && t.Diferencia < 0.05
			if !t.Cuadra {
				res.TodoCuadrado = false
			}
		}
		res.PorCuenta = append(res.PorCuenta, t)
	}
	sort.SliceStable(res.PorCuenta, func(i, j int) bool { return res.PorCuenta[i].Clave < res.PorCuenta[j].Clave })
	return res
}

// saldoContableDe suma debe − haber de una cuenta en todo el diario. Es el saldo
// que un activo debería tener: deudor y positivo.
func (s *Service) saldoContableDe(empresaID, codigo string) float64 {
	if s.asientos == nil {
		return 0
	}
	saldo := 0.0
	for _, a := range s.LibroDiario(empresaID) {
		for _, l := range a.Lineas {
			if l.Codigo == codigo {
				saldo += l.Debe - l.Haber
			}
		}
	}
	return round2(saldo)
}

// CuentasDeInventario devuelve los códigos que hoy acumulan inventario: la cuenta
// general más las declaradas por los rubros. Es lo que hay que mirar en el balance
// para ver el inventario completo, y sin esta lista habría que recordarlo.
func (s *Service) CuentasDeInventario(empresaID string) []string {
	vistas := map[string]bool{contabilidad.CtaInventario: true}
	out := []string{contabilidad.CtaInventario}
	if s.rubros != nil {
		for _, r := range s.Rubros(empresaID) {
			if r.CuentaInventario != "" && !vistas[r.CuentaInventario] {
				vistas[r.CuentaInventario] = true
				out = append(out, r.CuentaInventario)
			}
		}
	}
	sort.Strings(out)
	return out
}
