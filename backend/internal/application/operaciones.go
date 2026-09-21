package application

import (
	"errors"
	"sort"
	"strings"

	"github.com/mornix/elerp/internal/domain/almacen"
)

// TIPOS DE OPERACIÓN. Ver domain/almacen/operacion.go para el porqué.
//
// La regla que gobierna este archivo: SIN CONFIGURAR, TODO SIGUE IGUAL. Una empresa
// que no toque esta pantalla recibe y despacha exactamente como antes —un paso, al
// almacén principal—, y eso no es una cortesía: es lo que permite desplegar esto
// sobre datos vivos sin migrar nada ni avisar a nadie.
var (
	// ErrOperacionNoExiste: el tipo no está o no es de esta empresa.
	ErrOperacionNoExiste = errors.New("el tipo de operación no existe")
	// ErrOperacionSinCodigo: el código identifica la operación en las listas.
	ErrOperacionSinCodigo = errors.New("el tipo de operación necesita un código")
	// ErrOperacionCodigoEnUso: dos con el mismo código no se distinguen.
	ErrOperacionCodigoEnUso = errors.New("ya hay un tipo de operación con ese código")
	// ErrOperacionClaseInvalida: clase fuera de las que el código sabe leer.
	ErrOperacionClaseInvalida = errors.New("clase inválida (recepcion, entrega, ajuste o transferencia)")
	// ErrOperacionPasos: solo uno o dos pasos.
	ErrOperacionPasos = errors.New("una operación tiene uno o dos pasos")
	// ErrOperacionSinIntermedia: dos pasos sin ubicación intermedia no es un proceso
	// en dos pasos — es una recepción normal que además pierde el rastro del muelle.
	ErrOperacionSinIntermedia = errors.New("una operación en dos pasos necesita su ubicación intermedia (muelle o preparación)")
)

// ConTiposOperacion cablea el maestro. Sin él, todo se comporta como antes.
func (s *Service) ConTiposOperacion(r almacen.TipoOperacionRepo) *Service {
	s.tiposOperacion = r
	return s
}

// TiposOperacion lista los tipos de una empresa, ordenados por clase y código.
func (s *Service) TiposOperacion(empresaID string) []almacen.TipoOperacion {
	if s.tiposOperacion == nil {
		return []almacen.TipoOperacion{}
	}
	out := s.tiposOperacion.List(empresaID)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Clase != out[j].Clase {
			return out[i].Clase < out[j].Clase
		}
		return out[i].Codigo < out[j].Codigo
	})
	return out
}

// CrearTipoOperacion da de alta un tipo.
func (s *Service) CrearTipoOperacion(empresaID, actor, origen string, t almacen.TipoOperacion) (almacen.TipoOperacion, error) {
	if s.tiposOperacion == nil {
		return almacen.TipoOperacion{}, errors.New("los tipos de operación no están disponibles")
	}
	if err := s.validarTipoOperacion(empresaID, &t); err != nil {
		return almacen.TipoOperacion{}, err
	}
	for _, otro := range s.TiposOperacion(empresaID) {
		if otro.Activo && otro.Codigo == t.Codigo {
			return almacen.TipoOperacion{}, ErrOperacionCodigoEnUso
		}
	}
	t.EmpresaID = empresaID
	t.Activo = true
	t.Creado = ahora()
	if t.PorDefecto {
		s.quitarPorDefecto(empresaID, t.Clase, t.SedeID, "")
	}
	out := s.tiposOperacion.Create(t)
	s.audit.Append(evento(empresaID, actor, origen, "inventario.operacion.crear", out.Codigo, out.Clase))
	return out, nil
}

// ActualizarTipoOperacion edita un tipo. El CÓDIGO y la CLASE no se tocan: cambiar
// la clase convertiría una recepción en una entrega sin que nadie lo pidiera, y el
// código es por donde se le reconoce en las listas ya impresas.
func (s *Service) ActualizarTipoOperacion(empresaID, id, actor, origen string, cambios almacen.TipoOperacion) (almacen.TipoOperacion, error) {
	if s.tiposOperacion == nil {
		return almacen.TipoOperacion{}, errors.New("los tipos de operación no están disponibles")
	}
	t, ok := s.tiposOperacion.ByID(empresaID, id)
	if !ok {
		return almacen.TipoOperacion{}, ErrOperacionNoExiste
	}
	if n := strings.TrimSpace(cambios.Nombre); n != "" {
		t.Nombre = n
	}
	if cambios.Pasos != 0 {
		if !almacen.PasosValidos(cambios.Pasos) {
			return almacen.TipoOperacion{}, ErrOperacionPasos
		}
		t.Pasos = cambios.Pasos
	}
	t.AlmacenID = cambios.AlmacenID
	t.UbicacionIntermediaID = cambios.UbicacionIntermediaID
	t.Activo = cambios.Activo
	// La comprobación se repite sobre el resultado, no sobre los cambios: bajar a un
	// paso y borrar la intermedia en la misma edición es válido, y validar campo a
	// campo lo rechazaría según el orden en que llegaran.
	if t.Pasos == 2 && t.UbicacionIntermediaID == "" {
		return almacen.TipoOperacion{}, ErrOperacionSinIntermedia
	}
	if cambios.PorDefecto && !t.PorDefecto {
		s.quitarPorDefecto(empresaID, t.Clase, t.SedeID, t.ID)
	}
	t.PorDefecto = cambios.PorDefecto
	out, ok := s.tiposOperacion.Update(t)
	if !ok {
		return almacen.TipoOperacion{}, ErrOperacionNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "inventario.operacion.editar", out.Codigo, out.Nombre))
	return out, nil
}

func (s *Service) validarTipoOperacion(empresaID string, t *almacen.TipoOperacion) error {
	t.Codigo = almacen.NormalizarCodigoOperacion(t.Codigo)
	if t.Codigo == "" {
		return ErrOperacionSinCodigo
	}
	if !almacen.ClaseOperacionValida(t.Clase) {
		return ErrOperacionClaseInvalida
	}
	if t.Pasos == 0 {
		t.Pasos = 1
	}
	if !almacen.PasosValidos(t.Pasos) {
		return ErrOperacionPasos
	}
	if t.Pasos == 2 && t.UbicacionIntermediaID == "" {
		return ErrOperacionSinIntermedia
	}
	t.Nombre = strings.TrimSpace(t.Nombre)
	if t.Nombre == "" {
		t.Nombre = t.Codigo
	}
	return nil
}

// quitarPorDefecto deja un solo tipo por defecto por clase y sede. Con dos, la
// operación dependería de cuál se leyera primero — que es un orden de mapa, o sea
// ninguno.
func (s *Service) quitarPorDefecto(empresaID, clase, sedeID, excepto string) {
	for _, otro := range s.TiposOperacion(empresaID) {
		if otro.ID == excepto || !otro.PorDefecto || otro.Clase != clase || otro.SedeID != sedeID {
			continue
		}
		otro.PorDefecto = false
		s.tiposOperacion.Update(otro)
	}
}

// OperacionPara resuelve qué tipo aplica a una clase en una sede.
//
// ES EL ÚNICO CAMINO, por el mismo motivo que almacenParaEscritura: si cada sitio
// eligiera por su cuenta, dos procesos de la misma clase acabarían con reglas
// distintas y la diferencia solo se vería en el ledger, semanas después.
//
// Devuelve (cero, false) cuando no hay nada configurado, y ESO ES LO NORMAL: sin
// tipos, cada proceso sigue con su comportamiento de siempre.
func (s *Service) OperacionPara(empresaID, sedeID, clase string) (almacen.TipoOperacion, bool) {
	if s.tiposOperacion == nil {
		return almacen.TipoOperacion{}, false
	}
	var deLaSede, deLaEmpresa almacen.TipoOperacion
	var okSede, okEmpresa bool
	for _, t := range s.TiposOperacion(empresaID) {
		if !t.Activo || !t.PorDefecto || t.Clase != clase {
			continue
		}
		// El de la sede gana al general: se configura para esta sede justamente
		// porque aquí el proceso es distinto.
		if t.SedeID == sedeID && sedeID != "" {
			deLaSede, okSede = t, true
		} else if t.SedeID == "" {
			deLaEmpresa, okEmpresa = t, true
		}
	}
	if okSede {
		return deLaSede, true
	}
	return deLaEmpresa, okEmpresa
}

// ubicacionDeRecepcion decide dónde aterriza lo que llega.
//
// Con UN paso es la que pidió la línea: quien recibe tiene la caja delante y sabe
// a qué estante va. Con DOS, es siempre el muelle — y se IGNORA la que pidiera la
// línea a propósito: el sentido de los dos pasos es que nadie decide el sitio
// definitivo hasta haber revisado la mercancía. Colocarla ya donde toca y llamarlo
// «dos pasos» sería un paso con un rodeo, y dejaría el segundo paso sin nada que
// hacer salvo confirmar algo que ya ocurrió.
func (s *Service) ubicacionDeRecepcion(empresaID, almacenID, pedida string, dosPasos bool, muelle string) string {
	if dosPasos {
		// Si el muelle configurado ya no vale (lo desactivaron, o es de otro almacén),
		// la mercancía cae a «sin ubicar» dentro del almacén y se ve en el desglose.
		// Es preferible a mandarla al sitio que pidió la línea: eso la daría por
		// colocada y el segundo paso no se haría nunca.
		return s.ubicacionParaEscritura(empresaID, almacenID, muelle)
	}
	return s.ubicacionParaEscritura(empresaID, almacenID, pedida)
}

// PendienteDeUbicar lista lo que está en una ubicación intermedia esperando el
// segundo paso. Es la pantalla del proceso en dos pasos: sin ella, la mercancía se
// queda en el muelle y nadie se entera — el total de existencia no lo delata,
// porque las unidades sí están.
func (s *Service) PendienteDeUbicar(empresaID, sedeID string) []SaldoUbicacion {
	op, hay := s.OperacionPara(empresaID, sedeID, almacen.ClaseRecepcion)
	if !hay || op.Pasos != 2 || op.UbicacionIntermediaID == "" {
		return []SaldoUbicacion{}
	}
	out := []SaldoUbicacion{}
	for _, p := range s.productos.List(empresaID) {
		for _, u := range s.ExistenciaPorUbicacion(empresaID, sedeID, p.SKU) {
			if u.UbicacionID == op.UbicacionIntermediaID && u.Cantidad > 0.0001 {
				u.SKU, u.NombreProducto = p.SKU, p.Nombre
				out = append(out, u)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].SKU < out[j].SKU })
	return out
}

// SembrarTiposOperacion crea los cuatro tipos por defecto de una empresa, TODOS de
// un paso: así una empresa existente no nota nada hasta que alguien decida cambiar
// uno a dos pasos. Es idempotente — si ya hay tipos, no toca nada.
func (s *Service) SembrarTiposOperacion(empresaID, actor, origen string) int {
	if s.tiposOperacion == nil || len(s.tiposOperacion.List(empresaID)) > 0 {
		return 0
	}
	base := []almacen.TipoOperacion{
		{Codigo: "REC", Nombre: "Recepción de compras", Clase: almacen.ClaseRecepcion},
		{Codigo: "ENT", Nombre: "Entrega a clientes", Clase: almacen.ClaseEntrega},
		{Codigo: "AJU", Nombre: "Ajuste de existencia", Clase: almacen.ClaseAjuste},
		{Codigo: "TRF", Nombre: "Transferencia entre almacenes", Clase: almacen.ClaseTransferencia},
	}
	n := 0
	for _, t := range base {
		t.Pasos, t.PorDefecto = 1, true
		if _, err := s.CrearTipoOperacion(empresaID, actor, origen, t); err == nil {
			n++
		}
	}
	return n
}
