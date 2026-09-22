package application

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/sede"
	"github.com/mornix/elerp/internal/domain/usuario"
)

/* DEMO EFÍMERA: una copia por visitante.
 *
 * EL PROBLEMA QUE RESUELVE. La demostración era un tenant compartido: quien
 * entraba veía —y pisaba— lo que había hecho el visitante anterior. Para una
 * demostración comercial eso es lo peor que puede pasar, porque el prospecto no
 * distingue entre «así es el producto» y «esto lo dejó otro», y quien presenta no
 * puede tocar nada sin arruinarle la sesión a alguien más.
 *
 * Ahora cada visitante recibe SU COPIA: entra, factura, mueve mesas, despacha
 * pedidos, y nada de eso le llega a nadie. La copia se borra sola al vencer.
 *
 * LA COPIA NO ES UN RESPALDO. El exportador de respaldos existe pero se quedó
 * atrás —hoy no incluye el salón ni los pedidos—, así que clonar por ahí
 * entregaría una demo mutilada justo en los módulos que hay que mostrar. La
 * copia se hace en la capa de persistencia, documento por documento, y cubre lo
 * que existe y lo que venga sin que nadie tenga que acordarse.
 */

// CopiaDemo es el puerto de la copia efímera. Lo implementa el adaptador de
// persistencia porque la copia es documento a documento: el dominio no tiene por
// qué saber enumerar sus propios tipos para que alguien los duplique.
type CopiaDemo interface {
	// Clonar copia los datos de negocio de una empresa a otra. Devuelve cuántos
	// documentos copió.
	Clonar(origenID, destinoID string) int
	// Borrar elimina una copia efímera y todo lo suyo.
	Borrar(empresaID string) int
	// BarrerVencidas borra las copias cuya expiración ya pasó.
	BarrerVencidas() int
	// NuevoID genera el id de una copia (con el prefijo que la hace reconocible).
	NuevoID() string
}

var (
	// ErrDemoEfimeraNoDisponible: sin persistencia no hay copia que hacer. En
	// memoria cada arranque ya empieza limpio, así que la demo compartida es
	// inofensiva y se usa tal cual.
	ErrDemoEfimeraNoDisponible = errors.New("la demo efímera necesita persistencia")
)

// ConCopiaDemo cablea la copia efímera. Sin ella, el modo demo entra al tenant
// demo compartido, que es el comportamiento anterior.
func (s *Service) ConCopiaDemo(c CopiaDemo) *Service {
	s.copiaDemo = c
	return s
}

// HayDemoEfimera indica si esta instancia puede dar copias por visitante.
func (s *Service) HayDemoEfimera() bool { return s.copiaDemo != nil }

// TTLDemoMinutos es cuánto vive una copia. Tres horas alcanza de sobra para una
// demostración —incluso una larga, con preguntas— y es lo bastante corto para
// que una base no acumule copias abandonadas de la semana pasada.
const TTLDemoMinutos = 180

// SesionDemo es lo que se crea para un visitante: su usuario y sus empresas.
type SesionDemo struct {
	UsuarioID string
	Nombre    string
	Email     string
	Empresas  []string
	Copiados  int
}

// CrearSesionDemo arma una demostración privada: clona cada empresa base, crea
// un usuario propio del visitante y le da acceso a las copias.
//
// Se clonan TODAS las empresas demo y no solo la principal porque el selector de
// rubro es parte de la demostración: un prospecto de restaurante tiene que poder
// ver el salón, y uno de farmacia su catálogo. Entregar solo la bodega dejaría
// media demostración afuera.
func (s *Service) CrearSesionDemo(t *TenancyService, bases []string, actor, origen string) (SesionDemo, error) {
	if s.copiaDemo == nil {
		return SesionDemo{}, ErrDemoEfimeraNoDisponible
	}
	// El usuario es propio del visitante: dos personas en la demo a la vez no
	// pueden compartir identidad, o la bitácora diría que la misma persona hizo
	// las dos cosas.
	usuarioID := s.copiaDemo.NuevoID() + "_u"
	sesion := SesionDemo{
		UsuarioID: usuarioID,
		Nombre:    DemoNombre,
		Email:     fmt.Sprintf("demo+%s@elerp.tech", strings.TrimPrefix(usuarioID, "empdemo_")),
	}
	expira := time.Now().UTC().Add(TTLDemoMinutos * time.Minute).Format(time.RFC3339)

	for _, baseID := range bases {
		emp, err := t.CrearCopiaDemo(baseID, s.copiaDemo.NuevoID(), usuarioID, sesion.Nombre, sesion.Email, expira)
		if err != nil {
			continue // una base que no existe no puede frenar las demás
		}
		sesion.Copiados += s.copiaDemo.Clonar(baseID, emp.ID)
		sesion.Empresas = append(sesion.Empresas, emp.ID)
	}
	if len(sesion.Empresas) == 0 {
		return SesionDemo{}, ErrDemoEfimeraNoDisponible
	}
	return sesion, nil
}

// BarrerDemosVencidas limpia las copias que ya vencieron.
func (s *Service) BarrerDemosVencidas() int {
	if s.copiaDemo == nil {
		return 0
	}
	return s.copiaDemo.BarrerVencidas()
}

/* --- Tenancy: la cabecera de la copia ------------------------------------- */

// CrearCopiaDemo crea la empresa de una demostración privada.
//
// Las SEDES conservan su id original a propósito: todo el ledger las referencia
// (documentos, movimientos, cajas, mesas, pedidos), y como los repositorios
// filtran por empresa, dos tenants pueden tener una sede con el mismo id sin
// cruzarse. Remapearlas obligaría a reescribir cada referencia de cada
// colección, que es justo el trabajo que esta forma evita.
func (t *TenancyService) CrearCopiaDemo(baseID, nuevoID, usuarioID, nombre, email, expira string) (empresa.Empresa, error) {
	base, ok := t.emps.ByID(baseID)
	if !ok {
		return empresa.Empresa{}, ErrEmpresaNoExiste
	}
	nueva := base
	nueva.ID = nuevoID
	nueva.Sandbox = true
	nueva.OrigenSandboxID = baseID
	nueva.ExpiraEl = expira
	nueva.Activa = true
	nueva.Creada = ahora()
	emp := t.emps.Create(nueva)

	for _, sd := range t.sedes.List(baseID) {
		t.sedes.Create(sede.Sede{
			ID: sd.ID, EmpresaID: emp.ID, Nombre: sd.Nombre, Direccion: sd.Direccion,
			Lat: sd.Lat, Lon: sd.Lon, RadioM: sd.RadioM, Activa: sd.Activa,
		})
	}
	// El visitante entra como DUEÑA: una demostración con permisos recortados no
	// deja ver el producto, que es justamente lo que vino a ver.
	t.members.Create(usuario.Membresia{
		UsuarioID: usuarioID, Email: email, Nombre: nombre, EmpresaID: emp.ID,
		Rol: usuario.RolDueno, Estado: usuario.EstadoActiva,
	})
	return emp, nil
}

// BasesDemo son las empresas que se clonan para una demostración: exactamente
// las que ve el usuario demo compartido.
//
// Se derivan de sus membresías y no de una lista escrita a mano porque ya hay
// una fuente de verdad —lo que el seed le dio acceso— y agregar mañana una demo
// de rubro nuevo no puede exigir acordarse de tocar también esta función.
func (t *TenancyService) BasesDemo() []string {
	out := []string{}
	for _, m := range t.members.ByUsuario(DemoUserID) {
		if m.Estado != usuario.EstadoActiva {
			continue
		}
		if emp, ok := t.emps.ByID(m.EmpresaID); ok && !emp.Sandbox && emp.Activa {
			out = append(out, emp.ID)
		}
	}
	return out
}
