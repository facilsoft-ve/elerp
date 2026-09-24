package application

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mornix/elerp/internal/domain/unidadmedida"
)

// CONVERSIÓN ENTRE UNIDADES DE MEDIDA.
//
// POR QUÉ EXISTE. Se compra en bulto y se vende por unidad: llegan dos sacos de
// 50 kg y el anaquel se lleva en kilos. Hasta ahora eso obligaba a hacer la cuenta
// a mano antes de teclear la recepción, y una cuenta a mano en una recepción es una
// existencia equivocada que nadie detecta hasta el conteo físico.
//
// EL LEDGER SIEMPRE GUARDA LA UNIDAD BASE DEL PRODUCTO. La conversión ocurre en la
// puerta —al recibir, al ajustar— y lo que se anexa ya viene en la unidad del
// producto. Si el ledger admitiera unidades mezcladas, el pliegue sumaría kilos con
// sacos y el saldo no significaría nada; y el error no fallaría: daría un número.
//
// SOLO DENTRO DE LA MISMA CATEGORÍA. Convertir kilos a litros exige saber la
// densidad del producto, que no está en ninguna parte. Aceptarlo con un factor
// cualquiera daría una existencia plausible y falsa — así que se niega, y se dice
// por qué.
var (
	// ErrUnidadSinFactor: la unidad no declara a cuánto equivale, así que no se
	// puede convertir con ella. Es el caso de todo lo anterior a esta función y de
	// las unidades que dependen del producto, como «caja».
	ErrUnidadSinFactor = errors.New("esa unidad no dice a cuánto equivale: declara su factor o usa la unidad del producto")
	// ErrUnidadesIncompatibles: son de categorías distintas.
	ErrUnidadesIncompatibles = errors.New("no se puede convertir entre unidades de distinta naturaleza (peso, volumen, conteo…)")
	// ErrFactorInvalido: un factor cero o negativo no describe ninguna equivalencia.
	ErrFactorInvalido = errors.New("el factor tiene que ser mayor que cero")
)

// unidadPorSimbolo busca una unidad del maestro por su símbolo.
func (s *Service) unidadPorSimbolo(empresaID, simbolo string) (unidadmedida.UnidadMedida, bool) {
	if s.unidades == nil {
		return unidadmedida.UnidadMedida{}, false
	}
	simbolo = unidadmedida.NormalizarSimbolo(simbolo)
	for _, u := range s.Unidades(empresaID) {
		if u.Simbolo == simbolo {
			return u, true
		}
	}
	return unidadmedida.UnidadMedida{}, false
}

// ConvertirCantidad pasa una cantidad de una unidad a otra.
//
// ES EL ÚNICO CAMINO, igual que almacenParaEscritura o cuentaInventarioDe: si cada
// pantalla hiciera su regla de tres, la primera que se equivocara en un decimal
// metería una existencia falsa que solo aparecería en el conteo físico, meses
// después y sin forma de saber de dónde salió.
//
// Cuando las dos unidades son la misma —o una viene vacía— devuelve la cantidad tal
// cual. Eso NO es un atajo: es el comportamiento de siempre, y es lo que hace que
// todo lo que ya existe siga funcionando sin declarar ningún factor.
func (s *Service) ConvertirCantidad(empresaID, desde, hacia string, cantidad float64) (float64, error) {
	desde = unidadmedida.NormalizarSimbolo(desde)
	hacia = unidadmedida.NormalizarSimbolo(hacia)
	if desde == "" || hacia == "" || desde == hacia {
		return cantidad, nil
	}
	uDesde, ok := s.unidadPorSimbolo(empresaID, desde)
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrUnidadNoExiste, desde)
	}
	uHacia, ok := s.unidadPorSimbolo(empresaID, hacia)
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrUnidadNoExiste, hacia)
	}
	if uDesde.Categoria != uHacia.Categoria {
		return 0, fmt.Errorf("%w: %s es %s y %s es %s",
			ErrUnidadesIncompatibles, desde, uDesde.Categoria, hacia, uHacia.Categoria)
	}
	if uDesde.Factor <= 0 {
		return 0, fmt.Errorf("%w: %s", ErrUnidadSinFactor, desde)
	}
	if uHacia.Factor <= 0 {
		return 0, fmt.Errorf("%w: %s", ErrUnidadSinFactor, hacia)
	}
	// Se redondea a 4 decimales y no a 2: una conversión intermedia redondeada a
	// céntimos arrastra el error a la cantidad, y aquí lo que se redondea son
	// unidades de mercancía, no dinero.
	return round4(cantidad * uDesde.Factor / uHacia.Factor), nil
}

// UnidadesCompatiblesCon lista las unidades con las que se puede expresar una
// cantidad de la unidad dada: las de su misma categoría que declaran factor.
//
// La pantalla la usa para no ofrecer una conversión que después se va a rechazar:
// un desplegable que deja elegir litros para un producto que se lleva en kilos
// solo sirve para que alguien lo elija.
func (s *Service) UnidadesCompatiblesCon(empresaID, simbolo string) []unidadmedida.UnidadMedida {
	out := []unidadmedida.UnidadMedida{}
	base, ok := s.unidadPorSimbolo(empresaID, simbolo)
	if !ok || base.Factor <= 0 {
		return out // sin factor, el producto solo admite su propia unidad
	}
	for _, u := range s.Unidades(empresaID) {
		if u.Activa && u.Categoria == base.Categoria && u.Factor > 0 {
			out = append(out, u)
		}
	}
	return out
}

// ActualizarFactorUnidad declara a cuánto equivale una unidad.
func (s *Service) ActualizarFactorUnidad(empresaID, id string, factor float64, actor, origen string) (unidadmedida.UnidadMedida, error) {
	if s.unidades == nil {
		return unidadmedida.UnidadMedida{}, errors.New("las unidades no están disponibles")
	}
	u, ok := s.unidades.ByID(empresaID, id)
	if !ok {
		return unidadmedida.UnidadMedida{}, ErrUnidadNoExiste
	}
	// Cero es válido y significa «sin declarar»: es cómo se vuelve atrás sin tener
	// que borrar la unidad, que rompería los productos que la usan.
	if factor < 0 {
		return unidadmedida.UnidadMedida{}, ErrFactorInvalido
	}
	u.Factor = factor
	out, ok := s.unidades.Update(u)
	if !ok {
		return unidadmedida.UnidadMedida{}, ErrUnidadNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "inventario.unidad.factor", out.Simbolo,
		strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", factor), "0"), ".")))
	return out, nil
}

// AsegurarFactoresDeUnidades rellena las equivalencias que faltan en las unidades
// del juego por defecto.
//
// POR QUÉ HACE FALTA. El campo nació después que los datos: las empresas que ya
// existían tienen sus unidades sin factor, y sin factor no se convierte nada. La
// función se desplegó VIVA Y VACÍA — el selector no aparecía nunca, y no fallaba:
// simplemente no estaba. Lo detectó una comprobación en producción, no una prueba.
//
// SOLO ACTÚA SI LA EMPRESA NO TIENE NINGUNA EQUIVALENCIA DECLARADA. En cuanto
// alguien toca una, el backfill no vuelve a intervenir: si no, poner deliberadamente
// un factor a cero —«esta unidad no se convierte»— se desharía solo en el siguiente
// arranque, que es la clase de cosa que hace desconfiar de un sistema.
//
// Y solo toca los SÍMBOLOS CONOCIDOS. Una unidad propia del cliente («bulto»,
// «paca») no tiene equivalencia universal: la declara quien sabe cuánto mide.
func (s *Service) AsegurarFactoresDeUnidades(empresaID, actor, origen string) int {
	if s.unidades == nil {
		return 0
	}
	actuales := s.Unidades(empresaID)
	for _, u := range actuales {
		if u.Factor > 0 {
			return 0 // ya hay equivalencias declaradas: no se toca nada
		}
	}
	conocidos := map[string]float64{}
	for _, d := range unidadmedida.PorDefecto(empresaID) {
		if d.Factor > 0 {
			conocidos[d.Simbolo] = d.Factor
		}
	}
	n := 0
	for _, u := range actuales {
		f, ok := conocidos[u.Simbolo]
		if !ok {
			continue
		}
		u.Factor = f
		if _, ok := s.unidades.Update(u); ok {
			n++
		}
	}
	if n > 0 {
		s.audit.Append(evento(empresaID, actor, origen, "inventario.unidad.factores_iniciales",
			fmt.Sprintf("%d unidad(es)", n), "equivalencias del juego por defecto"))
	}
	return n
}

// round4 redondea a cuatro decimales. Las cantidades convertidas no son dinero: un
// gramo expresado en kilos son 0,001 y redondearlo a céntimos lo dejaría en cero.
func round4(v float64) float64 {
	return float64(int64(v*10000+copiaSigno(0.5, v))) / 10000
}

func copiaSigno(magnitud, signo float64) float64 {
	if signo < 0 {
		return -magnitud
	}
	return magnitud
}
