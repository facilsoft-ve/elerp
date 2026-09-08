package application

import (
	"errors"
	"strings"

	"github.com/mornix/elerp/internal/domain/cocina"
)

var (
	ErrCocinaNoDisponible = errors.New("la configuración de cocina no está disponible")
	ErrConexionInvalida   = errors.New("modo de conexión inválido (local o red)")
	ErrImpresoraSinHost   = errors.New("una impresora de red necesita su IP o host")
)

// ConImpresoras cablea la configuración de impresora de comandas.
func (s *Service) ConImpresoras(r cocina.Repository) *Service {
	s.impresoras = r
	return s
}

// ImpresoraComandas devuelve la impresora configurada de la sede, o una por
// defecto (local, 80 mm, apagada) si aún no se configuró — así la interfaz
// siempre tiene con qué pintar el formulario.
func (s *Service) ImpresoraComandas(empresaID, sedeID string) cocina.Impresora {
	if s.impresoras != nil {
		if imp, ok := s.impresoras.Get(empresaID, sedeID); ok {
			return imp
		}
	}
	return cocina.Impresora{
		EmpresaID: empresaID, SedeID: sedeID, Nombre: "Cocina",
		Conexion: cocina.ConexionLocal, AnchoMM: 80, Activa: false,
	}
}

// ImpresoraBody es el cuerpo de configuración de la impresora de comandas.
type ImpresoraBody struct {
	Nombre   string `json:"nombre"`
	Conexion string `json:"conexion"`
	Host     string `json:"host"`
	Puerto   int    `json:"puerto"`
	AnchoMM  int    `json:"anchoMm"`
	Activa   bool   `json:"activa"`
}

// GuardarImpresoraComandas valida y persiste la impresora de comandas de la sede.
func (s *Service) GuardarImpresoraComandas(empresaID, sedeID, actor, origen string, b ImpresoraBody) (cocina.Impresora, error) {
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
	imp := cocina.Impresora{
		EmpresaID: empresaID, SedeID: sedeID, Nombre: nombre, Conexion: conexion,
		Host: host, Puerto: puerto, AnchoMM: ancho, Activa: b.Activa, Actualizada: ahora(),
	}
	out := s.impresoras.Upsert(imp)
	s.audit.Append(evento(empresaID, actor, origen, "restaurante.impresora.guardar", sedeID, nombre+" · "+conexion))
	return out, nil
}
