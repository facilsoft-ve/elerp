package application

import (
	"errors"
	"strings"
	"time"

	"github.com/mornix/elerp/internal/domain/cupon"
)

var (
	ErrCuponNoDisponible  = errors.New("el maestro de cupones no está disponible")
	ErrCuponNoExiste      = errors.New("cupón no existe")
	ErrCuponCodigoDup     = errors.New("ya existe un cupón con ese código")
	ErrTipoCuponInvalido  = errors.New("tipo de cupón inválido (porcentaje o monto)")
	ErrCuponInactivo      = errors.New("el cupón no está activo")
	ErrCuponFueraVigencia = errors.New("el cupón está fuera de su periodo de vigencia")
	ErrCuponMontoMinimo   = errors.New("el subtotal no alcanza el monto mínimo del cupón")
	ErrCuponUsosAgotados  = errors.New("el cupón alcanzó su límite de usos")
)

// ConCupones cablea el maestro de cupones. Se configura aparte de New (como
// ConListasPrecio) para no romper las firmas de los constructores ya cableados
// en cmd/api; sin él, el servicio funciona igual y los cupones quedan vacíos.
func (s *Service) ConCupones(r cupon.Repository) *Service {
	s.cupones = r
	return s
}

// EntradaCupon son los datos para crear/editar un cupón.
type EntradaCupon struct {
	Codigo      string
	Descripcion string
	Tipo        string
	Valor       float64
	MontoMinimo float64
	Desde       string
	Hasta       string
	Activo      bool
	UsosMax     int
}

// ResultadoCupon es lo que devuelve ValidarCupon: el cupón resuelto y el
// descuento en bolívares que produce sobre el subtotal dado.
type ResultadoCupon struct {
	Cupon       cupon.Cupon `json:"cupon"`
	DescuentoBs float64     `json:"descuentoBs"`
}

// Cupones lista los cupones de la empresa.
func (s *Service) Cupones(empresaID string) []cupon.Cupon {
	if s.cupones == nil {
		return []cupon.Cupon{}
	}
	return s.cupones.List(empresaID)
}

// normalizarFechaCupon deja una fecha en YYYY-MM-DD: recorta espacios y toma los
// primeros 10 caracteres (tolera que llegue en ISO con hora). Vacío = sin límite.
func normalizarFechaCupon(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 10 {
		return v[:10]
	}
	return ""
}

// validarEntradaCupon arma un Cupon normalizado desde la entrada y valida las
// reglas de negocio: código requerido/único, tipo válido, valor>0 y %≤100. Al
// editar, `id` excluye al propio cupón del control de unicidad.
func (s *Service) validarEntradaCupon(empresaID, id string, in EntradaCupon) (cupon.Cupon, error) {
	cod := cupon.NormalizarCodigo(in.Codigo)
	if cod == "" {
		return cupon.Cupon{}, errors.New("código requerido")
	}
	if !cupon.TipoValido(in.Tipo) {
		return cupon.Cupon{}, ErrTipoCuponInvalido
	}
	if in.Valor <= 0 {
		return cupon.Cupon{}, errors.New("el valor del cupón debe ser mayor a 0")
	}
	if in.Tipo == cupon.TipoPorcentaje && in.Valor > 100 {
		return cupon.Cupon{}, errors.New("un cupón porcentual no puede superar 100%")
	}
	if in.MontoMinimo < 0 {
		return cupon.Cupon{}, errors.New("el monto mínimo no puede ser negativo")
	}
	if in.UsosMax < 0 {
		return cupon.Cupon{}, errors.New("los usos máximos no pueden ser negativos")
	}
	// Único por empresa (excluye el propio id al editar).
	if ex, ok := s.cupones.ByCodigo(empresaID, cod); ok && ex.ID != id {
		return cupon.Cupon{}, ErrCuponCodigoDup
	}
	return cupon.Cupon{
		ID: id, Codigo: cod, Descripcion: strings.TrimSpace(in.Descripcion),
		Tipo: in.Tipo, Valor: round2(in.Valor), MontoMinimo: round2(in.MontoMinimo),
		Desde: normalizarFechaCupon(in.Desde), Hasta: normalizarFechaCupon(in.Hasta),
		Activo: in.Activo, UsosMax: in.UsosMax,
	}, nil
}

// CrearCupon da de alta un cupón. UsosActuales arranca en 0.
func (s *Service) CrearCupon(empresaID, actor, origen string, in EntradaCupon) (cupon.Cupon, error) {
	if s.cupones == nil {
		return cupon.Cupon{}, ErrCuponNoDisponible
	}
	c, err := s.validarEntradaCupon(empresaID, "", in)
	if err != nil {
		return cupon.Cupon{}, err
	}
	c.EmpresaID = empresaID
	c.UsosActuales = 0
	out := s.cupones.Create(c)
	s.audit.Append(evento(empresaID, actor, origen, "ventas.cupon.crear", out.ID, out.Codigo))
	return out, nil
}

// ActualizarCupon edita un cupón existente del tenant (maestro editable, no
// ledger). Conserva UsosActuales: el conteo de usos no se reescribe al editar.
func (s *Service) ActualizarCupon(empresaID, id, actor, origen string, in EntradaCupon) (cupon.Cupon, error) {
	if s.cupones == nil {
		return cupon.Cupon{}, ErrCuponNoDisponible
	}
	cur, ok := s.cupones.ByID(empresaID, id)
	if !ok {
		return cupon.Cupon{}, ErrCuponNoExiste
	}
	c, err := s.validarEntradaCupon(empresaID, id, in)
	if err != nil {
		return cupon.Cupon{}, err
	}
	c.EmpresaID = empresaID
	c.UsosActuales = cur.UsosActuales
	out, ok := s.cupones.Update(c)
	if !ok {
		return cupon.Cupon{}, ErrCuponNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "ventas.cupon.actualizar", out.ID, out.Codigo))
	return out, nil
}

// cuponVigente indica si `now` cae dentro del periodo [Desde, Hasta] del cupón.
// Compara por día en formato YYYY-MM-DD (comparación lexicográfica, correcta para
// ese formato). Los extremos vacíos significan «sin límite» en ese lado.
func cuponVigente(c cupon.Cupon, now time.Time) bool {
	hoy := now.Format("2006-01-02")
	if c.Desde != "" && hoy < c.Desde {
		return false
	}
	if c.Hasta != "" && hoy > c.Hasta {
		return false
	}
	return true
}

// descuentoDe calcula el descuento en Bs de un cupón sobre un subtotal, topado al
// propio subtotal (un cupón nunca deja el total en negativo).
func descuentoDe(c cupon.Cupon, subtotal float64) float64 {
	var d float64
	switch c.Tipo {
	case cupon.TipoPorcentaje:
		d = subtotal * c.Valor / 100
	case cupon.TipoMonto:
		d = c.Valor
	}
	if d > subtotal {
		d = subtotal
	}
	if d < 0 {
		d = 0
	}
	return round2(d)
}

// ValidarCupon resuelve un código de cupón contra el subtotal (en Bs) de la
// compra y devuelve el descuento que produce, o un error claro por el que no
// aplica. NO recalcula impuestos ni modifica nada: quien lo llama baja el
// precioUnitario de las líneas con el descuento y deja que el motor fiscal
// calcule el IVA sobre la base ya descontada.
func (s *Service) ValidarCupon(empresaID, codigo string, subtotalBs float64) (ResultadoCupon, error) {
	if s.cupones == nil {
		return ResultadoCupon{}, ErrCuponNoDisponible
	}
	cod := cupon.NormalizarCodigo(codigo)
	if cod == "" {
		return ResultadoCupon{}, errors.New("código de cupón requerido")
	}
	c, ok := s.cupones.ByCodigo(empresaID, cod)
	if !ok {
		return ResultadoCupon{}, ErrCuponNoExiste
	}
	if !c.Activo {
		return ResultadoCupon{}, ErrCuponInactivo
	}
	if !cuponVigente(c, time.Now().UTC()) {
		return ResultadoCupon{}, ErrCuponFueraVigencia
	}
	if c.UsosMax > 0 && c.UsosActuales >= c.UsosMax {
		return ResultadoCupon{}, ErrCuponUsosAgotados
	}
	if subtotalBs < c.MontoMinimo {
		return ResultadoCupon{}, ErrCuponMontoMinimo
	}
	return ResultadoCupon{Cupon: c, DescuentoBs: descuentoDe(c, subtotalBs)}, nil
}

// ConsumirCupon incrementa el contador de usos (UsosActuales) de un cupón tras
// aplicarlo en una emisión con éxito. Es lo que hace que el tope UsosMax se
// aplique de verdad: sin este incremento, un cupón de un solo uso se podría usar
// infinitas veces (ValidarCupon solo lee el contador, no lo sube).
//
// Está pensado como BEST-EFFORT desde EmitirFactura: se llama DESPUÉS de emitir,
// así que el llamador debe IGNORAR el error (la factura ya está registrada; el
// ledger fiscal manda por encima del conteo de usos de un maestro editable).
func (s *Service) ConsumirCupon(empresaID, codigo string) error {
	if s.cupones == nil {
		return ErrCuponNoDisponible
	}
	cod := cupon.NormalizarCodigo(codigo)
	if cod == "" {
		return errors.New("código de cupón requerido")
	}
	c, ok := s.cupones.ByCodigo(empresaID, cod)
	if !ok {
		return ErrCuponNoExiste
	}
	c.UsosActuales++
	if _, ok := s.cupones.Update(c); !ok {
		return ErrCuponNoExiste
	}
	return nil
}
