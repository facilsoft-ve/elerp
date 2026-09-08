package application

import (
	"errors"
	"strings"

	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/proveedor"
)

// Errores de negocio del maestro de proveedores.
var (
	ErrProveedorNoExiste  = errors.New("proveedor no existe")
	ErrProveedorSinNombre = errors.New("el proveedor necesita un nombre")
)

// Proveedores lista los proveedores de la empresa.
func (s *Service) Proveedores(empresaID string) []proveedor.Proveedor {
	if s.provs == nil {
		return []proveedor.Proveedor{}
	}
	return s.provs.List(empresaID)
}

// Proveedor devuelve un proveedor por id.
func (s *Service) Proveedor(empresaID, id string) (proveedor.Proveedor, bool) {
	if s.provs == nil {
		return proveedor.Proveedor{}, false
	}
	return s.provs.ByID(empresaID, id)
}

// CrearProveedor da de alta un proveedor. Nace activo.
func (s *Service) CrearProveedor(empresaID, actor, origen string, p proveedor.Proveedor) (proveedor.Proveedor, error) {
	p.Nombre = strings.TrimSpace(p.Nombre)
	if p.Nombre == "" {
		return proveedor.Proveedor{}, ErrProveedorSinNombre
	}
	p.EmpresaID = empresaID
	p.Documento = strings.TrimSpace(p.Documento)
	// El documento del proveedor viene CON prefijo de tipo ("J-…"). Se valida si
	// está presente (es opcional en el maestro de proveedores).
	if p.Documento != "" {
		if err := fiscal.ValidarDocumentoStr(p.Documento); err != nil {
			return proveedor.Proveedor{}, err
		}
	}
	p.Activo = true
	p.Creado = ahora()
	out := s.provs.Create(p)
	s.audit.Append(evento(empresaID, actor, origen, "compras.proveedor.crear", out.ID, out.Nombre))
	return out, nil
}

// ActualizarProveedor edita los datos de contacto del proveedor. No cambia el
// estado Activo: eso se hace con DesactivarProveedor (baja reversible).
func (s *Service) ActualizarProveedor(empresaID, id, actor, origen string, cambios proveedor.Proveedor) (proveedor.Proveedor, error) {
	p, ok := s.provs.ByID(empresaID, id)
	if !ok {
		return proveedor.Proveedor{}, ErrProveedorNoExiste
	}
	if n := strings.TrimSpace(cambios.Nombre); n != "" {
		p.Nombre = n
	}
	// El resto de campos se puede vaciar a propósito, así que se copian tal cual.
	p.Documento = strings.TrimSpace(cambios.Documento)
	if p.Documento != "" {
		if err := fiscal.ValidarDocumentoStr(p.Documento); err != nil {
			return proveedor.Proveedor{}, err
		}
	}
	p.Email = strings.TrimSpace(cambios.Email)
	p.Telefono = strings.TrimSpace(cambios.Telefono)
	p.Direccion = strings.TrimSpace(cambios.Direccion)
	out, ok := s.provs.Update(p)
	if !ok {
		return proveedor.Proveedor{}, ErrProveedorNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "compras.proveedor.editar", out.ID, out.Nombre))
	return out, nil
}

// DesactivarProveedor da de baja un proveedor SIN borrarlo (reversible): queda
// Activo=false vía Update, y su histórico de órdenes de compra sigue intacto.
func (s *Service) DesactivarProveedor(empresaID, id, actor, origen string) (proveedor.Proveedor, error) {
	p, ok := s.provs.ByID(empresaID, id)
	if !ok {
		return proveedor.Proveedor{}, ErrProveedorNoExiste
	}
	p.Activo = false
	out, ok := s.provs.Update(p)
	if !ok {
		return proveedor.Proveedor{}, ErrProveedorNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "compras.proveedor.desactivar", out.ID, out.Nombre))
	return out, nil
}
