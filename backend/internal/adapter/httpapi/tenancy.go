package httpapi

import (
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/aplicacion"
	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// handleCrearEmpresa da de alta una empresa (y su organización si es la primera).
func (s *Server) handleCrearEmpresa(c *fiber.Ctx) error {
	var in struct{ Nombre, RIF string }
	if err := c.BodyParser(&in); err != nil || in.Nombre == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "nombre y RIF requeridos"})
	}
	emp, err := s.tenancy.CrearEmpresa(principalOf(c), origen(c), in.Nombre, in.RIF)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(emp)
}

// handleOnboarding fija giro y modalidad de facturación de una empresa.
func (s *Server) handleOnboarding(c *fiber.Ctx) error {
	id := c.Params("id")
	if !s.puedeAdministrar(c, id) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "sin permiso"})
	}
	var in struct {
		Giro                 string `json:"giro"`
		ModalidadFacturacion string `json:"modalidadFacturacion"`
		// Paso de monedas del onboarding (R9 + R10).
		MonedaPrincipal string `json:"monedaPrincipal"`
		FuenteTasa      string `json:"fuenteTasa"`
		PreciosEnUsd    bool   `json:"preciosEnUsd"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	emp, err := s.tenancy.Onboarding(id, principalOf(c).UserID, origen(c), in.Giro, in.ModalidadFacturacion, application.ConfigMoneda{
		MonedaPrincipal: in.MonedaPrincipal, FuenteTasa: in.FuenteTasa, PreciosEnUsd: in.PreciosEnUsd,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(emp)
}

// handleCrearSede añade una sede a una empresa.
func (s *Server) handleCrearSede(c *fiber.Ctx) error {
	id := c.Params("id")
	if !s.puedeAdministrar(c, id) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "sin permiso"})
	}
	var in struct{ Nombre, Direccion string }
	if err := c.BodyParser(&in); err != nil || in.Nombre == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "nombre requerido"})
	}
	sd, err := s.tenancy.CrearSede(id, principalOf(c).UserID, origen(c), in.Nombre, in.Direccion)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	// Invariante "una sede tiene ≥1 almacén": la sede nueva nace con su "Almacén
	// Principal". Best-effort: si el maestro no está cableado, la sede igual se creó.
	_, _ = s.svc.AsegurarAlmacenPrincipal(id, sd.ID, principalOf(c).UserID, origen(c))
	return c.Status(fiber.StatusCreated).JSON(sd)
}

// handleActualizarEmpresa edita los datos fiscales de cabecera del tenant.
func (s *Server) handleActualizarEmpresa(c *fiber.Ctx) error {
	id := c.Params("id")
	if !s.puedeAdministrar(c, id) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "sin permiso"})
	}
	var in struct {
		Nombre              string                      `json:"nombre"`
		RIF                 string                      `json:"rif"`
		RazonSocial         string                      `json:"razonSocial"`
		Direccion           string                      `json:"direccion"`
		Telefono            string                      `json:"telefono"`
		Email               string                      `json:"email"`
		Logo                string                      `json:"logo"`
		ColorMarca          string                      `json:"colorMarca"`
		LogoAlterno         string                      `json:"logoAlterno"`
		LogoBlanco          string                      `json:"logoBlanco"`
		LogoNegro           string                      `json:"logoNegro"`
		ExigirCedulaCliente bool                        `json:"exigirCedulaCliente"`
		BannerSuperior      string                      `json:"bannerSuperior"`
		BannerLateral       string                      `json:"bannerLateral"`
		PantallaClienteModo string                      `json:"pantallaClienteModo"`
		Publicidad          []empresa.AnuncioSlide      `json:"publicidad"`
		TemaPantalla        empresa.TemaPantallaCliente `json:"temaPantalla"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	emp, err := s.tenancy.ActualizarEmpresa(id, principalOf(c).UserID, origen(c), application.EmpresaEdit{
		Nombre: in.Nombre, RIF: in.RIF, RazonSocial: in.RazonSocial,
		Direccion: in.Direccion, Telefono: in.Telefono, Email: in.Email, Logo: in.Logo,
		ColorMarca: in.ColorMarca, LogoAlterno: in.LogoAlterno,
		LogoBlanco: in.LogoBlanco, LogoNegro: in.LogoNegro,
		ExigirCedulaCliente: in.ExigirCedulaCliente,
		BannerSuperior:      in.BannerSuperior, BannerLateral: in.BannerLateral,
		PantallaClienteModo: in.PantallaClienteModo, Publicidad: in.Publicidad,
		TemaPantalla: in.TemaPantalla,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(emp)
}

// handleActualizarImpuestos configura las alícuotas de IVA e IGTF de la empresa
// (fracciones, p. ej. 0.16). Solo dueño/desarrollador. Los documentos ya emitidos
// conservan su tasa; el cambio rige para documentos nuevos.
func (s *Server) handleActualizarImpuestos(c *fiber.Ctx) error {
	id := c.Params("id")
	if !s.puedeAdministrar(c, id) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "sin permiso"})
	}
	var in struct {
		AlicuotaIVA            float64 `json:"alicuotaIVA"`
		AlicuotaIGTF           float64 `json:"alicuotaIGTF"`
		AgenteRetencionIVA     bool    `json:"agenteRetencionIVA"`
		AgenteRetencionISLR    bool    `json:"agenteRetencionISLR"`
		RetencionIVAPorcentaje float64 `json:"retencionIVAPorcentaje"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	emp, err := s.tenancy.ActualizarImpuestos(id, principalOf(c).UserID, origen(c), in.AlicuotaIVA, in.AlicuotaIGTF,
		in.AgenteRetencionIVA, in.AgenteRetencionISLR, in.RetencionIVAPorcentaje)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(emp)
}

// handleActualizarSede cambia nombre y dirección de una sede.
func (s *Server) handleActualizarSede(c *fiber.Ctx) error {
	id := c.Params("id")
	if !s.puedeAdministrar(c, id) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "sin permiso"})
	}
	var in struct{ Nombre, Direccion string }
	if err := c.BodyParser(&in); err != nil || in.Nombre == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "nombre requerido"})
	}
	sd, err := s.tenancy.ActualizarSede(id, c.Params("sedeId"), principalOf(c).UserID, origen(c), in.Nombre, in.Direccion)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(sd)
}

// handleDesactivarSede da de baja (reversible) una sede.
func (s *Server) handleDesactivarSede(c *fiber.Ctx) error {
	id := c.Params("id")
	if !s.puedeAdministrar(c, id) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "sin permiso"})
	}
	if err := s.tenancy.DesactivarSede(id, c.Params("sedeId"), principalOf(c).UserID, origen(c)); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

// handleReactivarSede vuelve a activar una sede dada de baja.
func (s *Server) handleReactivarSede(c *fiber.Ctx) error {
	id := c.Params("id")
	if !s.puedeAdministrar(c, id) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "sin permiso"})
	}
	if err := s.tenancy.ReactivarSede(id, c.Params("sedeId"), principalOf(c).UserID, origen(c)); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

// inviteLink arma el enlace absoluto de aceptación de una invitación a partir del
// token. Apunta a la SPA (`…/?invite=<token>`), que muestra AcceptInvite. Usa la
// URL pública del frontend; la interfaz igual puede recomponerlo con su origin.
func (s *Server) inviteLink(token string) string {
	base := strings.TrimRight(s.cfg.FrontendURL, "/")
	return base + "/?invite=" + url.QueryEscape(token)
}

// handleInvitarMiembro crea (o reactiva) una invitación en estado pendiente y
// devuelve el token y el enlace de aceptación para compartir a mano.
//
// El correo automático de invitación todavía NO está conectado (no hay servicio de
// email real): por eso la respuesta trae el `enlace`, que el administrador comparte
// con la persona invitada. Ver InvitarMiembro para el punto de integración.
func (s *Server) handleInvitarMiembro(c *fiber.Ctx) error {
	id := c.Params("id")
	if !s.puedeAdministrar(c, id) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "sin permiso"})
	}
	var in struct {
		Email  string `json:"email"`
		Nombre string `json:"nombre"`
		Rol    string `json:"rol"`
		SedeID string `json:"sedeId"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	m, err := s.tenancy.InvitarMiembro(id, principalOf(c).UserID, origen(c), application.InviteInput{
		Email: in.Email, Nombre: in.Nombre, Rol: in.Rol, SedeID: in.SedeID,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id": m.ID, "email": m.Email, "nombre": m.Nombre, "rol": m.Rol, "sedeId": m.SedeID,
		"token": m.Token, "enlace": s.inviteLink(m.Token),
		// El envío automático del correo aún no está cableado: hay que compartir el enlace.
		"correoEnviado": false,
	})
}

// handleReenviarInvitacion regenera el token de una invitación pendiente y
// devuelve el enlace nuevo (invalida el anterior).
func (s *Server) handleReenviarInvitacion(c *fiber.Ctx) error {
	id := c.Params("id")
	if !s.puedeAdministrar(c, id) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "sin permiso"})
	}
	m, err := s.tenancy.ReenviarInvitacion(id, c.Params("miembroId"), principalOf(c).UserID, origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"id": m.ID, "email": m.Email, "token": m.Token, "enlace": s.inviteLink(m.Token),
		"correoEnviado": false,
	})
}

// handleCancelarInvitacion elimina una invitación pendiente.
func (s *Server) handleCancelarInvitacion(c *fiber.Ctx) error {
	id := c.Params("id")
	if !s.puedeAdministrar(c, id) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "sin permiso"})
	}
	if err := s.tenancy.CancelarInvitacion(id, c.Params("miembroId"), principalOf(c).UserID, origen(c)); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

// handleBootstrap devuelve el contexto de arranque del tenant activo.
func (s *Server) handleBootstrap(c *fiber.Ctx) error {
	empID := empresaIDOf(c)
	emp, _ := c.Locals("empresa").(empresa.Empresa)
	return c.JSON(fiber.Map{
		"empresa":    emp,
		"sedes":      s.tenancy.Sedes(empID),
		"sedeActiva": sedeIDOf(c),
		"rol":        rolOf(c),
		"roles":      rolesMatrix(),
		"rubros":     s.svc.Rubros(empID),
		// Unidades de medida ACTIVAS del maestro (Configuración › Unidades) que
		// alimentan el select de UnidadBase del editor de producto, sin una llamada
		// extra al abrir el catálogo.
		"unidades": s.svc.UnidadesActivas(empID),
		// Promociones ACTIVAS y vigentes del módulo (Ventas › Promociones) que
		// alimentan el carrusel de la pantalla del cliente. Van en el bootstrap —no
		// por el endpoint gestionado de /ventas/promociones— para que la segunda
		// pantalla las muestre bajo CUALQUIER rol que opere la caja (incl. cajero),
		// junto a los slides manuales de empresa.publicidad.
		"promocionesActivas": promocionesSiMarketing(s, empID),
		// Módulos ACTIVOS de la empresa (core + comercializables instalados/activos):
		// la interfaz oculta lo apagado. Ver internal/application/aplicacion.go.
		"modulos": s.svc.ModulosActivos(empID),
		// Sesión de DEMOSTRACIÓN: el principal es el usuario demo (DEV_LOGIN). La
		// interfaz lo usa para mostrar la franja de «sesión limitada».
		"demo": principalOf(c).UserID == application.DemoUserID,
		// Tasa vigente con su origen y su fecha: la interfaz nunca la teclea ni
		// la rotula por su cuenta (R9).
		"tasa": s.svc.TasaDeEmpresa(empID),
		// Formatos de documento (Configuración › Formatos). Van en el bootstrap —no
		// por el endpoint gestionado /config/formatos, gateado a Dueña/Contadora—
		// para que CUALQUIER rol que imprima un documento (incl. cajero en el POS)
		// pueda resolver, del lado del cliente, qué formato usa su sede.
		"formatos": s.svc.Plantillas(empID),
		// Módulo Restaurante: mesas del salón de la sede activa y la config de la
		// impresora de comandas. Solo si el módulo está activo (si no, vacío); así
		// el mapa y el selector de mesas solo aparecen cuando corresponde.
		"mesas":             mesasSiRestaurante(s, empID, sedeIDOf(c)),
		"planoSalon":        planoSiRestaurante(s, empID, sedeIDOf(c)),
		"impresoraComandas": impresoraSiRestaurante(s, empID, sedeIDOf(c)),
		// Cuentas abiertas de la sede (tablero de la comandera): estado y total en
		// vivo de cada mesa. Solo si el módulo está activo.
		"cuentasAbiertas": cuentasSiRestaurante(s, empID, sedeIDOf(c)),
		// Asignación de mesas por mesonero y config del módulo: la comandera las usa para
		// destacar las mesas propias y para saber si tomar una ajena está permitido.
		"asignacionesMesas": asignacionesSiRestaurante(s, empID, sedeIDOf(c)),
		"configSalon":       configSalonSiRestaurante(s, empID, sedeIDOf(c)),
	})
}

// cuentasSiRestaurante devuelve las cuentas abiertas de la sede solo si el módulo
// Restaurante está activo; si no, vacío.
func cuentasSiRestaurante(s *Server, empID, sedeID string) any {
	if !s.svc.ModuloActivo(empID, aplicacion.ModRestaurante) {
		return []any{}
	}
	return s.svc.CuentasAbiertas(empID, sedeID)
}

// mesasSiRestaurante devuelve las mesas de la sede solo si el módulo Restaurante
// está activo; si no, vacío.
func mesasSiRestaurante(s *Server, empID, sedeID string) any {
	if !s.svc.ModuloActivo(empID, aplicacion.ModRestaurante) {
		return []any{}
	}
	return s.svc.Mesas(empID, sedeID)
}

// planoSiRestaurante devuelve la grilla del salón de la sede solo si el módulo
// Restaurante está activo; si no, nil.
func planoSiRestaurante(s *Server, empID, sedeID string) any {
	if !s.svc.ModuloActivo(empID, aplicacion.ModRestaurante) {
		return nil
	}
	return s.svc.PlanoSalon(empID, sedeID)
}

// asignacionesSiRestaurante devuelve la asignación de mesas por mesonero solo si el
// módulo Restaurante está activo; si no, nil.
func asignacionesSiRestaurante(s *Server, empID, sedeID string) any {
	if !s.svc.ModuloActivo(empID, aplicacion.ModRestaurante) {
		return nil
	}
	return s.svc.Asignaciones(empID, sedeID)
}

// configSalonSiRestaurante devuelve la config del módulo en la sede solo si está activo.
func configSalonSiRestaurante(s *Server, empID, sedeID string) any {
	if !s.svc.ModuloActivo(empID, aplicacion.ModRestaurante) {
		return nil
	}
	return s.svc.ConfigSalon(empID, sedeID)
}

// impresoraSiRestaurante devuelve la config de impresora de comandas de la sede
// solo si el módulo Restaurante está activo; si no, nil.
func impresoraSiRestaurante(s *Server, empID, sedeID string) any {
	if !s.svc.ModuloActivo(empID, aplicacion.ModRestaurante) {
		return nil
	}
	return s.svc.ImpresoraComandas(empID, sedeID)
}

// promocionesSiMarketing devuelve las promociones activas solo si el módulo
// Marketing está activo en la empresa; si no, vacío (la pantalla del cliente no
// muestra publicidad cuando el módulo está apagado).
func promocionesSiMarketing(s *Server, empID string) any {
	if !s.svc.ModuloActivo(empID, aplicacion.ModMarketing) {
		return []any{}
	}
	return s.svc.PromocionesActivas(empID)
}

// puedeAdministrar verifica que el usuario sea Dueña/Desarrollador de la empresa.
func (s *Server) puedeAdministrar(c *fiber.Ctx, empresaID string) bool {
	_, rol, _, ok := s.tenancy.RolEnEmpresa(principalOf(c).UserID, empresaID)
	return ok && (rol == usuario.RolDueno || rol == usuario.RolDesarrollador)
}

// rolesMatrix describe los roles para la interfaz (etiqueta + resumen).
func rolesMatrix() map[string]map[string]string {
	return map[string]map[string]string{
		usuario.RolDueno:         {"label": "Dueña / Admin", "resumen": "Acceso total a la empresa"},
		usuario.RolVendedor:      {"label": "Vendedor", "resumen": "POS, ventas y existencias (lectura)"},
		usuario.RolCajero:        {"label": "Cajero", "resumen": "Solo POS y documentos"},
		usuario.RolContadora:     {"label": "Contadora", "resumen": "Contabilidad y tesorería; resto en consulta"},
		usuario.RolDesarrollador: {"label": "Desarrollador / IT", "resumen": "Total + modo desarrollador"},
		usuario.RolMesonero:      {"label": "Mesonero", "resumen": "Comandera del restaurante (una sede)"},
	}
}
