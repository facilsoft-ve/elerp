package application

import (
	"fmt"
	"strings"
	"time"

	"github.com/mornix/elerp/internal/domain/mesa"
)

/* VALIDACIONES Y AVISOS DE RESERVA (notas de la contadora, 15:06 y 15:07).
 *
 *   «trabajar validaciones en reservas: AFORO / SECTOR DEL RESTAURANTE / HORA»
 *   «notificar cuando no existan mesas con la capacidad de personas completas,
 *    es decir que se deben sentar en mesas distintas» — y aclaró: ES UNA
 *    ADVERTENCIA.
 *
 * La distinción entre RECHAZAR y AVISAR es la decisión de diseño de este
 * archivo, y sigue la misma regla que el resto del sistema: se rechaza lo que
 * es imposible o es un error de datos; se avisa lo que es una decisión del
 * negocio. Un anfitrión que no puede tomar una reserva porque el sistema cree
 * saber más que él termina anotándola en un papel, y ahí se pierde entera.
 *
 *   RECHAZA  → reservar para ayer; una zona que no existe en la sede.
 *   AVISA    → fuera del horario de atención; el salón supera su aforo en esa
 *              franja; no hay una sola mesa para el grupo (hay que separarlo).
 */

// Códigos de aviso. Son estables para que la pantalla decida cómo mostrarlos
// sin comparar textos, que son para leer y se reescriben.
const (
	AvisoFueraDeHorario = "fuera_de_horario"
	AvisoAforoSuperado  = "aforo_superado"
	AvisoSinMesaUnica   = "sin_mesa_unica"
)

// AvisoReserva es una advertencia: no impide guardar, informa.
type AvisoReserva struct {
	Codigo  string `json:"codigo"`
	Mensaje string `json:"mensaje"`
}

// AvisosDeReserva evalúa las advertencias de una reserva SIN crearla. La
// pantalla la llama al escribir y al guardar; el resultado nunca bloquea.
//
// `excluirID` deja fuera a la propia reserva cuando se está editando: si no, al
// mover la hora de una reserva de 8 personas, esas 8 se contarían dos veces
// contra el aforo y el aviso saldría siempre.
func (s *Service) AvisosDeReserva(in EntradaReserva, excluirID string) []AvisoReserva {
	avisos := []AvisoReserva{}
	if s.reservas == nil || s.mesas == nil {
		return avisos
	}
	fecha, hora := strings.TrimSpace(in.Fecha), strings.TrimSpace(in.Hora)
	if fecha == "" || hora == "" || in.Personas <= 0 {
		return avisos
	}
	mesas := s.mesas.List(in.EmpresaID, in.SedeID)

	// 1. Fuera del horario de atención. Solo si la sede lo declaró: un local que
	//    no lo configuró no puede recibir esta advertencia todo el día.
	if s.configSalon != nil {
		if cfg, ok := s.configSalon.Get(in.EmpresaID, in.SedeID); ok && cfg.HorarioServicio() && !cfg.DentroDelServicio(hora) {
			avisos = append(avisos, AvisoReserva{
				Codigo: AvisoFueraDeHorario,
				Mensaje: fmt.Sprintf("Las %s quedan fuera del horario de atención (%s a %s).",
					hora, cfg.HoraApertura, cfg.HoraCierre),
			})
		}
	}

	// 2. No hay UNA mesa para el grupo: hay que sentarlos en mesas distintas. Es
	//    lo que pidió la contadora, y es aviso y no rechazo a propósito — juntar
	//    dos mesas es lo que hace un salón todos los días.
	if mayor := mayorCapacidad(mesas, in.Zona); mayor > 0 && in.Personas > mayor {
		donde := "el salón"
		if strings.TrimSpace(in.Zona) != "" {
			donde = "la zona " + in.Zona
		}
		avisos = append(avisos, AvisoReserva{
			Codigo: AvisoSinMesaUnica,
			Mensaje: fmt.Sprintf("No hay una sola mesa para %d personas en %s (la mayor es de %d): habrá que sentarlos en mesas distintas.",
				in.Personas, donde, mayor),
		})
	}

	// 3. Aforo de la franja. Se cuenta la gente ya reservada en la ventana más
	//    la de esta reserva, contra la capacidad total de las mesas activas.
	if aforo := aforoDe(mesas); aforo > 0 {
		if ocupadas := personasEnLaFranja(s, in, excluirID); ocupadas+in.Personas > aforo {
			avisos = append(avisos, AvisoReserva{
				Codigo: AvisoAforoSuperado,
				Mensaje: fmt.Sprintf("Con esta reserva la franja queda en %d personas y el salón tiene capacidad para %d.",
					ocupadas+in.Personas, aforo),
			})
		}
	}
	return avisos
}

// mayorCapacidad es la mesa más grande del salón (o de una zona). Solo cuenta
// las ACTIVAS: una mesa dada de baja no sirve para sentar a nadie.
func mayorCapacidad(mesas []mesa.Mesa, zona string) int {
	z := strings.TrimSpace(strings.ToLower(zona))
	mayor := 0
	for _, m := range mesas {
		if !m.Activa {
			continue
		}
		if z != "" && strings.ToLower(strings.TrimSpace(m.Zona)) != z {
			continue
		}
		if m.Capacidad > mayor {
			mayor = m.Capacidad
		}
	}
	return mayor
}

// aforoDe es la capacidad total del salón: la suma de sus mesas activas.
func aforoDe(mesas []mesa.Mesa) int {
	total := 0
	for _, m := range mesas {
		if m.Activa {
			total += m.Capacidad
		}
	}
	return total
}

// personasEnLaFranja suma la gente ya reservada alrededor de esa hora. Usa la
// misma ventana con la que el tablero aparta una mesa (VentanaReserva), para que
// el aviso y el tablero cuenten lo mismo.
func personasEnLaFranja(s *Service, in EntradaReserva, excluirID string) int {
	nueva, err := horaDeReserva(in.Fecha, in.Hora)
	if err != nil {
		return 0
	}
	total := 0
	for _, otra := range s.reservas.DelDia(in.EmpresaID, in.SedeID, in.Fecha) {
		if otra.ID == excluirID || !otra.Activa() {
			continue
		}
		t, err := horaDeReserva(otra.Fecha, otra.Hora)
		if err != nil {
			continue
		}
		if diferenciaMenorA(nueva, t, VentanaReserva) {
			total += otra.Personas
		}
	}
	return total
}

/* --- Validaciones que SÍ rechazan ----------------------------------------- */

// zonaExiste indica si la sede tiene alguna mesa activa en esa zona. Una zona
// inventada no es una decisión del negocio, es un dato mal escrito: la reserva
// quedaría apuntando a un sector que no existe y nadie la encontraría.
func zonaExiste(mesas []mesa.Mesa, zona string) bool {
	z := strings.TrimSpace(strings.ToLower(zona))
	if z == "" {
		return true
	}
	for _, m := range mesas {
		if m.Activa && strings.ToLower(strings.TrimSpace(m.Zona)) == z {
			return true
		}
	}
	return false
}

// esPasado indica si esa fecha y hora ya pasaron. Reservar para ayer no es una
// política del local: es imposible, y casi siempre un año o un mes mal tecleado.
func esPasado(fecha, hora string) bool {
	t, err := horaDeReserva(fecha, hora)
	if err != nil {
		return false
	}
	return t.Before(time.Now())
}

// GuardarHorarioServicioSalon fija el horario de atención de la sede, que es
// contra lo que se avisa una reserva fuera de hora.
//
// Vive junto a los avisos y no con el resto de la configuración del salón porque
// es el único ajuste que existe POR ellos: sin reservas, el horario de atención
// no lo consulta nadie.
func (s *Service) GuardarHorarioServicioSalon(empresaID, sedeID, actor, origen, apertura, cierre string) (mesa.ConfigSalon, error) {
	if s.configSalon == nil {
		return mesa.ConfigSalon{}, ErrAsignacionNoDisponible
	}
	apertura, cierre = strings.TrimSpace(apertura), strings.TrimSpace(cierre)
	// Vaciar los dos es válido y significa «sin horario declarado»: deja de
	// avisarse por la hora. Uno solo, no — un horario a medias no se puede
	// comparar contra nada.
	if (apertura == "") != (cierre == "") {
		return mesa.ConfigSalon{}, ErrHorarioServicioInvalido
	}
	if apertura != "" && (!horaHHMM(apertura) || !horaHHMM(cierre)) {
		return mesa.ConfigSalon{}, ErrHorarioServicioInvalido
	}
	actual, _ := s.configSalon.Get(empresaID, sedeID)
	actual.EmpresaID, actual.SedeID = empresaID, sedeID
	actual.HoraApertura, actual.HoraCierre = apertura, cierre
	actual.Actualizada = ahora()
	out := s.configSalon.Upsert(actual)
	s.audit.Append(evento(empresaID, actor, origen, "restaurante.config", "horario_servicio",
		strings.TrimSpace(apertura+" a "+cierre)))
	return out, nil
}

// horaHHMM valida una hora de pared "HH:MM" en 24 horas.
func horaHHMM(s string) bool {
	_, err := time.Parse("15:04", strings.TrimSpace(s))
	return err == nil && len(strings.TrimSpace(s)) == 5
}
