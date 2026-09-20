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
	// ErrRetencionPorcentajeProveedor: un porcentaje de retención fuera de (0, 100].
	ErrRetencionPorcentajeProveedor = errors.New("el porcentaje de retención del proveedor debe estar entre 0 y 100")
	// ErrRetencionSustraendoNegativo: el sustraendo resta, pero nunca es negativo.
	ErrRetencionSustraendoNegativo = errors.New("el sustraendo de ISLR no puede ser negativo")
	// ErrRetencionISLRSinConcepto: el ISLR se retiene POR CONCEPTO; sin él no hay
	// forma de justificar el porcentaje ante el SENIAT.
	ErrRetencionISLRSinConcepto = errors.New("la retención de ISLR necesita el concepto (honorarios, servicios, fletes…)")
	// ErrSujetoISLRInvalido: el tipo de sujeto decide cuál de las dos tarifas del
	// concepto aplica, así que no puede quedar sin declarar.
	ErrSujetoISLRInvalido = errors.New("indica si el proveedor es persona natural residente o jurídica domiciliada")
	// ErrConceptoISLRDelProveedorNoExiste: el código no está en el maestro para ese
	// sujeto. Se avisa al guardar la ficha, no al emitir el comprobante.
	ErrConceptoISLRDelProveedorNoExiste = errors.New("ese concepto de ISLR no existe en el maestro para ese tipo de sujeto")
)

// saneaPerfilProveedor normaliza y valida el perfil comercial y fiscal. Los
// porcentajes se guardan como número (75 = 75%), igual que en los comprobantes de
// retención, para no mezclar dos convenciones en el mismo dominio.
func (s *Service) saneaPerfilProveedor(empresaID string, p *proveedor.Proveedor) error {
	p.CondicionPago = strings.TrimSpace(p.CondicionPago)
	p.ConceptoISLRCodigo = strings.TrimSpace(p.ConceptoISLRCodigo)
	p.SujetoISLR = strings.TrimSpace(p.SujetoISLR)

	// Un porcentaje solo se valida si el impuesto se retiene: apagar la retención
	// deja de exigir el resto de los datos, y no se borran por si se vuelve a activar.
	if p.RetieneIVA && (p.RetencionIVAPorcentaje < 0 || p.RetencionIVAPorcentaje > 100) {
		return ErrRetencionPorcentajeProveedor
	}
	if p.RetieneISLR {
		if p.ConceptoISLRCodigo == "" {
			return ErrRetencionISLRSinConcepto
		}
		if !fiscal.SujetoValido(p.SujetoISLR) {
			return ErrSujetoISLRInvalido
		}
		// Se comprueba contra el maestro ACÁ, al guardar la ficha, y no al emitir el
		// comprobante: un concepto que no existe se descubre cuando se configura el
		// proveedor, no seis semanas después con la factura en la mano.
		if !fiscal.ExisteConceptoPara(s.ConceptosISLR(empresaID), p.ConceptoISLRCodigo, p.SujetoISLR) {
			return ErrConceptoISLRDelProveedorNoExiste
		}
	}
	return nil
}

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
	if err := s.saneaPerfilProveedor(empresaID, &p); err != nil {
		return proveedor.Proveedor{}, err
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
	// Perfil comercial y fiscal: se copia completo (igual que el contacto, se puede
	// vaciar a propósito) y se valida antes de persistir.
	p.CondicionPago = cambios.CondicionPago
	p.ContribuyenteEspecial = cambios.ContribuyenteEspecial
	p.RetieneIVA = cambios.RetieneIVA
	p.RetencionIVAPorcentaje = cambios.RetencionIVAPorcentaje
	p.RetieneISLR = cambios.RetieneISLR
	p.ConceptoISLRCodigo = cambios.ConceptoISLRCodigo
	p.SujetoISLR = cambios.SujetoISLR
	if err := s.saneaPerfilProveedor(empresaID, &p); err != nil {
		return proveedor.Proveedor{}, err
	}
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
