package application

import (
	"errors"
	"strings"

	"github.com/mornix/elerp/internal/domain/cliente"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

var ErrDocumentoDuplicado = errors.New("ya existe un cliente con ese documento")

var ErrEmailInvalido = errors.New("el correo no tiene un formato válido")

var ErrClienteNoExiste = errors.New("cliente no existe")

// emailValido hace una verificación básica de formato (algo@algo.algo), sin
// pretender validar según RFC: el correo es opcional en el maestro de clientes.
func emailValido(s string) bool {
	at := strings.IndexByte(s, '@')
	if at <= 0 || at == len(s)-1 {
		return false
	}
	dominio := s[at+1:]
	if strings.ContainsAny(s, " \t\r\n") {
		return false
	}
	dot := strings.LastIndexByte(dominio, '.')
	return dot > 0 && dot < len(dominio)-1
}

// Clientes lista los clientes de la empresa.
func (s *Service) Clientes(empresaID string) []cliente.Cliente {
	return s.clientes.List(empresaID)
}

// CrearCliente da de alta un cliente validando documento único por empresa.
func (s *Service) CrearCliente(empresaID, actor, origen string, c cliente.Cliente) (cliente.Cliente, error) {
	c.EmpresaID = empresaID
	c.Documento = strings.TrimSpace(c.Documento)
	c.Email = strings.TrimSpace(c.Email)
	if c.Email != "" && !emailValido(c.Email) {
		return cliente.Cliente{}, ErrEmailInvalido
	}
	if c.TipoDocumento == "" {
		c.TipoDocumento = cliente.DocV
	}
	// El documento se valida según su tipo (formato SENIAT, DV del RIF para J/G).
	// El maestro guarda el número SIN prefijo, así que el tipo es la letra. Un
	// documento vacío no se valida (p. ej. registros mínimos), pero uno presente
	// sí: la UI solo avisa, el dominio es el que exige.
	if c.Documento != "" {
		if err := fiscal.ValidarDocumento(c.TipoDocumento, c.Documento); err != nil {
			return cliente.Cliente{}, err
		}
	}
	c.Nombre = strings.TrimSpace(c.Nombre)
	c.Telefono = strings.TrimSpace(c.Telefono)
	c.Direccion = strings.TrimSpace(c.Direccion)
	c.NombreComercial = strings.TrimSpace(c.NombreComercial)
	c.Contacto = strings.TrimSpace(c.Contacto)
	c.Notas = strings.TrimSpace(c.Notas)
	// Un cliente nace activo; si no se declara un origen (p. ej. una importación
	// desde Odoo lo marcaría), se asume carga manual en ElERP.
	c.Activo = true
	if strings.TrimSpace(c.Origen) == "" {
		c.Origen = cliente.OrigenManual
	}
	if _, ok := s.clientes.ByDocumento(empresaID, c.TipoDocumento, c.Documento); ok {
		return cliente.Cliente{}, ErrDocumentoDuplicado
	}
	out := s.clientes.Create(c)
	s.audit.Append(evento(empresaID, actor, origen, "crm.cliente.crear", out.ID, out.Nombre))
	return out, nil
}

// ActualizarCliente edita un cliente existente del tenant. Es un maestro (no un
// ledger): se persiste con Update del repositorio, nunca con el patrón
// append-only de los documentos fiscales. Valida existencia/pertenencia,
// normaliza, mantiene la unicidad de documento por empresa, valida el correo si
// viene, permite activar/desactivar y audita el cambio.
//
// Los campos de interoperabilidad (Origen/SistemaExterno/IdExterno) se PRESERVAN
// del registro almacenado: la edición manual no reescribe la procedencia de un
// cliente importado desde Odoo u otro CRM (así el round-trip de sincronización
// sigue siendo posible).
func (s *Service) ActualizarCliente(empresaID, actor, id string, edits cliente.Cliente) (cliente.Cliente, error) {
	cur, ok := s.clientes.ByID(empresaID, id)
	if !ok {
		return cliente.Cliente{}, ErrClienteNoExiste
	}

	nombre := strings.TrimSpace(edits.Nombre)
	if nombre == "" {
		return cliente.Cliente{}, errors.New("nombre requerido")
	}
	documento := strings.TrimSpace(edits.Documento)
	if documento == "" {
		return cliente.Cliente{}, errors.New("documento requerido")
	}
	tipoDoc := edits.TipoDocumento
	if tipoDoc == "" {
		tipoDoc = cur.TipoDocumento
	}
	if tipoDoc == "" {
		tipoDoc = cliente.DocV
	}
	email := strings.TrimSpace(edits.Email)
	if email != "" && !emailValido(email) {
		return cliente.Cliente{}, ErrEmailInvalido
	}
	// Documento válido según su tipo (DV del RIF para J/G). Aquí el documento es
	// obligatorio (se validó arriba que no esté vacío).
	if err := fiscal.ValidarDocumento(tipoDoc, documento); err != nil {
		return cliente.Cliente{}, err
	}

	// Unicidad de documento por empresa: solo si cambió, y sin chocar con OTRO.
	if tipoDoc != cur.TipoDocumento || documento != cur.Documento {
		if otro, ok := s.clientes.ByDocumento(empresaID, tipoDoc, documento); ok && otro.ID != id {
			return cliente.Cliente{}, ErrDocumentoDuplicado
		}
	}

	cur.Nombre = nombre
	cur.TipoDocumento = tipoDoc
	cur.Documento = documento
	cur.Telefono = strings.TrimSpace(edits.Telefono)
	cur.Direccion = strings.TrimSpace(edits.Direccion)
	cur.Email = email
	cur.NombreComercial = strings.TrimSpace(edits.NombreComercial)
	cur.Contacto = strings.TrimSpace(edits.Contacto)
	cur.Notas = strings.TrimSpace(edits.Notas)
	cur.Activo = edits.Activo
	// Origen/SistemaExterno/IdExterno NO se tocan: se conservan de `cur`.

	out, ok := s.clientes.Update(cur)
	if !ok {
		return cliente.Cliente{}, ErrClienteNoExiste
	}
	s.audit.Append(evento(empresaID, actor, "", "crm.cliente.actualizar", out.ID, out.Nombre))
	return out, nil
}
