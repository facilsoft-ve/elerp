package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// iaFalso es un IAProxy de prueba: captura lo que se le manda y devuelve una respuesta
// fija, para verificar la RAMA de IA sin llamar a Hubmy.
type iaFalso struct {
	modelo, system, usuario string
	llamado                 bool
	resp                    string
	err                     error
}

func (f *iaFalso) Chat(_ context.Context, modelo, system, usuario string) (string, error) {
	f.llamado = true
	f.modelo, f.system, f.usuario = modelo, system, usuario
	if f.err != nil {
		return "", f.err
	}
	if f.resp == "" {
		return "respuesta de prueba", nil
	}
	return f.resp, nil
}

func TestAsistenteMecanica_Ventas(t *testing.T) {
	svc, _ := nuevoServicio(t)
	resp, ok := svc.ResponderMecanica(empDemo, usuario.RolDueno, "¿Cuánto vendí este mes?")
	if !ok {
		t.Fatal("la pregunta de ventas debe resolverse por la capa mecánica")
	}
	if resp.Tipo != application.RespMecanica {
		t.Fatalf("tipo esperado mecanica, se obtuvo %q", resp.Tipo)
	}
	if !strings.Contains(resp.Respuesta, "ventas netas") {
		t.Fatalf("la respuesta debe hablar de ventas netas: %q", resp.Respuesta)
	}
	if resp.Fuente == "" {
		t.Fatal("la respuesta mecánica debe declarar su fuente")
	}
}

func TestAsistenteMecanica_Ayuda(t *testing.T) {
	svc, _ := nuevoServicio(t)
	casos := map[string]string{
		"¿Qué es el IGTF?":                    "IGTF",
		"¿Cómo transfiero stock entre sedes?": "Transferencias",
		"¿Qué es forma libre?":                "Forma libre",
	}
	for pregunta, esperado := range casos {
		resp, ok := svc.ResponderMecanica(empDemo, usuario.RolVendedor, pregunta)
		if !ok {
			t.Fatalf("la pregunta de ayuda %q debe resolverse mecánicamente", pregunta)
		}
		if !strings.Contains(resp.Respuesta, esperado) {
			t.Fatalf("la ayuda de %q debe mencionar %q; se obtuvo %q", pregunta, esperado, resp.Respuesta)
		}
		if resp.Fuente != "Ayuda" {
			t.Fatalf("la fuente de ayuda debe ser Ayuda, se obtuvo %q", resp.Fuente)
		}
	}
}

// El asistente NUNCA expone más que la interfaz: un rol sin acceso a un área recibe una
// respuesta mecánica de "sin acceso" (definitiva, no escala a IA).
func TestAsistenteMecanica_AcotadoPorRol(t *testing.T) {
	svc, _ := nuevoServicio(t)

	// El vendedor no ve contabilidad: "¿cuadra el libro?" → sin acceso, pero ok=true.
	resp, ok := svc.ResponderMecanica(empDemo, usuario.RolVendedor, "¿Cuadra el libro contable?")
	if !ok {
		t.Fatal("la pregunta de integridad debe resolverse (aunque sea negando acceso)")
	}
	if resp.Fuente != "Permisos" {
		t.Fatalf("un rol sin acceso debe recibir una respuesta de Permisos, se obtuvo %q", resp.Fuente)
	}

	// La dueña sí ve contabilidad.
	resp, ok = svc.ResponderMecanica(empDemo, usuario.RolDueno, "¿Cuadra el libro contable?")
	if !ok || resp.Fuente != "Integridad" {
		t.Fatalf("la dueña debe recibir la verificación de integridad, se obtuvo ok=%v fuente=%q", ok, resp.Fuente)
	}
}

func TestAsistenteMecanica_SinMatch(t *testing.T) {
	svc, _ := nuevoServicio(t)
	_, ok := svc.ResponderMecanica(empDemo, usuario.RolDueno, "¿Cuál es el sentido de la vida?")
	if ok {
		t.Fatal("una pregunta abierta no debe resolverse por la capa mecánica (debe escalar)")
	}
}

func TestAsistenteMecanica_StockDeProducto(t *testing.T) {
	svc, _ := nuevoServicio(t)
	prods := svc.Productos(empDemo)
	if len(prods) == 0 {
		t.Skip("el seed no tiene productos")
	}
	// Busca el primer producto NO combo (los combos no tienen stock propio).
	var nombre string
	for _, p := range prods {
		if !p.EsCombo {
			nombre = p.Nombre
			break
		}
	}
	resp, ok := svc.ResponderMecanica(empDemo, usuario.RolDueno, "¿Cuánto stock de "+nombre+" tengo?")
	if !ok {
		t.Fatalf("la consulta de stock de un producto debe resolverse mecánicamente")
	}
	if !strings.Contains(resp.Respuesta, nombre) {
		t.Fatalf("la respuesta debe nombrar al producto %q: %q", nombre, resp.Respuesta)
	}
}

// La rama de IA solo procede si la empresa la habilitó y hay proxy cableado.
func TestAsistenteIA_DeshabilitadaPorDefecto(t *testing.T) {
	svc, _ := nuevoServicio(t)
	svc.ConIA(&iaFalso{})
	// La demo no tiene la IA habilitada: debe devolver ErrIANoDisponible.
	_, err := svc.ResponderIA(context.Background(), empDemo, usuario.RolDueno, "opina sobre mi negocio")
	if !errors.Is(err, application.ErrIANoDisponible) {
		t.Fatalf("con la IA deshabilitada debe devolver ErrIANoDisponible, se obtuvo %v", err)
	}
}

func TestAsistenteIA_SinProxy(t *testing.T) {
	svc, _ := nuevoServicio(t)
	// Habilitar la IA pero SIN cablear proxy: sigue sin poder responder.
	if _, err := svc.ConfigurarAsistente(empDemo, actorA, origenTst, true, ""); err != nil {
		t.Fatalf("configurar asistente: %v", err)
	}
	_, err := svc.ResponderIA(context.Background(), empDemo, usuario.RolDueno, "opina")
	if !errors.Is(err, application.ErrIANoDisponible) {
		t.Fatalf("sin proxy debe devolver ErrIANoDisponible, se obtuvo %v", err)
	}
}

// Con la IA habilitada y un proxy falso, la rama IA arma system prompt + contexto
// acotado al rol y devuelve tipo "ia" — sin llamar a ningún proveedor real.
func TestAsistenteIA_ConProxyFalso(t *testing.T) {
	svc, _ := nuevoServicio(t)
	fake := &iaFalso{resp: "Tu negocio va bien."}
	svc.ConIA(fake)
	if _, err := svc.ConfigurarAsistente(empDemo, actorA, origenTst, true, ""); err != nil {
		t.Fatalf("configurar asistente: %v", err)
	}

	resp, err := svc.ResponderIA(context.Background(), empDemo, usuario.RolDueno, "¿cómo ves mi negocio?")
	if err != nil {
		t.Fatalf("ResponderIA: %v", err)
	}
	if resp.Tipo != application.RespIA {
		t.Fatalf("tipo esperado ia, se obtuvo %q", resp.Tipo)
	}
	if resp.Respuesta != "Tu negocio va bien." {
		t.Fatalf("debe devolver la respuesta del proxy, se obtuvo %q", resp.Respuesta)
	}
	if resp.Modelo != application.ModeloIADefault {
		t.Fatalf("sin modelo configurado debe usar el default, se obtuvo %q", resp.Modelo)
	}
	if !fake.llamado {
		t.Fatal("el proxy debió ser llamado")
	}
	// El system prompt debe traer las reglas estrictas + el contexto de datos.
	if !strings.Contains(fake.system, "No tienes acceso a internet") {
		t.Fatalf("el system prompt debe prohibir internet: %q", fake.system)
	}
	if !strings.Contains(fake.system, "Ventas del mes") {
		t.Fatalf("el system prompt debe incluir el contexto de datos (Ventas del mes): %q", fake.system)
	}
	// El mensaje del usuario viaja tal cual.
	if fake.usuario != "¿cómo ves mi negocio?" {
		t.Fatalf("el mensaje del usuario debe viajar al proxy, se obtuvo %q", fake.usuario)
	}
}

// El contexto de IA se ACOTA al rol: un vendedor no debe llevar el balance contable.
func TestAsistenteIA_ContextoAcotadoAlRol(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sysVendedor := svc.ContextoIA(empDemo, usuario.RolVendedor)
	if strings.Contains(sysVendedor, "Contabilidad: balance") {
		t.Fatalf("el contexto del vendedor NO debe incluir el balance contable: %q", sysVendedor)
	}
	sysDueno := svc.ContextoIA(empDemo, usuario.RolDueno)
	if !strings.Contains(sysDueno, "Contabilidad: balance") {
		t.Fatalf("el contexto de la dueña SÍ debe incluir el balance contable: %q", sysDueno)
	}
}
