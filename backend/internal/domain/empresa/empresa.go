// Package empresa modela la Empresa: el TENANT de ElERP. Todo dato de negocio
// lleva un empresaID no vacío y las consultas se aíslan por ese campo.
package empresa

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
)

// Modalidades de facturación válidas (Providencia 000102 y afines).
const (
	ModalidadFormaLibre    = "forma_libre"
	ModalidadMaquinaFiscal = "maquina_fiscal"
	ModalidadImprentaDig   = "imprenta_digital"
)

// Monedas que una empresa puede elegir como principal. La factura legal SIEMPRE
// se emite en bolívares (lo exige el SENIAT); la moneda principal es en la que
// la empresa piensa y captura sus precios.
const (
	MonedaVES = "VES"
	MonedaUSD = "USD"
)

// Fuentes de tasa que la empresa elige en el onboarding (las tres opciones del
// prototipo, paso «Monedas y tu primer almacén»).
const (
	// FuenteTasaBCV: «Tasa oficial BCV (automática)».
	FuenteTasaBCV = "bcv"
	// FuenteTasaMercado: «Tasa promedio del mercado».
	FuenteTasaMercado = "mercado"
	// FuenteTasaManual: «La escribo yo cada día».
	FuenteTasaManual = "manual"
)

// MonedaValida valida la MONEDA PRINCIPAL de la empresa: sigue siendo solo VES o
// USD (la base legal es VES; el USD es la única otra moneda en la que una empresa
// venezolana "piensa" sus precios). Las DEMÁS divisas se activan aparte, en
// MonedasActivas, y se validan con DivisaConocida — no como moneda principal.
func MonedaValida(m string) bool { return m == MonedaVES || m == MonedaUSD }

// FuenteTasaValida valida la fuente de tasa elegida por la empresa.
func FuenteTasaValida(f string) bool {
	return f == FuenteTasaBCV || f == FuenteTasaMercado || f == FuenteTasaManual
}

// Divisas EXTRA que una empresa puede activar además del bolívar (base legal) y
// del dólar. La lista blanca acota qué códigos se aceptan: nada de teclear una
// moneda arbitraria.
const (
	MonedaEUR  = "EUR"
	MonedaCOP  = "COP"
	MonedaUSDT = "USDT"
)

// divisasConocidas es la lista blanca de divisas activables. VES no está: es la
// base legal (tasa 1), no una divisa que se active ni que lleve tasa.
var divisasConocidas = map[string]bool{
	MonedaUSD: true, MonedaEUR: true, MonedaCOP: true, MonedaUSDT: true,
}

// DivisaConocida valida un código de divisa activable contra la lista blanca.
func DivisaConocida(codigo string) bool {
	return divisasConocidas[strings.ToUpper(strings.TrimSpace(codigo))]
}

// MonedaActiva es una divisa que la empresa habilitó, con su fuente de tasa. La
// regla del negocio: la fuente `bcv` (automática) es SOLO para el dólar; toda
// otra divisa lleva tasa `manual` (o, si se configura, `mercado`), nunca BCV.
type MonedaActiva struct {
	Codigo string `json:"codigo" bson:"codigo"`
	Fuente string `json:"fuente" bson:"fuente"`
}

// FuenteActivaValida valida la fuente de una divisa activa: cualquiera de las
// fuentes conocidas, pero `bcv` SOLO cuando el código es USD.
func FuenteActivaValida(codigo, fuente string) bool {
	if !FuenteTasaValida(fuente) {
		return false
	}
	if fuente == FuenteTasaBCV && strings.ToUpper(strings.TrimSpace(codigo)) != MonedaUSD {
		return false
	}
	return true
}

// FuenteEfectivaDivisa resuelve la fuente REAL de una divisa: el USD respeta su
// configuración (bcv/mercado/manual, bcv por defecto); las demás nunca pueden ser
// bcv, así que caen en `manual` salvo que expresamente se pidiera `mercado`.
func FuenteEfectivaDivisa(codigo, fuente string) string {
	cod := strings.ToUpper(strings.TrimSpace(codigo))
	fuente = strings.TrimSpace(fuente)
	if cod == MonedaUSD {
		if FuenteTasaValida(fuente) {
			return fuente
		}
		return FuenteTasaBCV
	}
	if fuente == FuenteTasaMercado {
		return FuenteTasaMercado
	}
	return FuenteTasaManual
}

// NormalizarMonedasActivas valida y limpia la lista de divisas activas: descarta
// VES y los vacíos (VES es la base, no se activa), rechaza divisas fuera de la
// lista blanca y fuentes inválidas (bcv en algo que no sea USD), deduplica por
// código y fija la fuente efectiva de cada una. Devuelve error a la primera
// entrada inválida para que la configuración no se guarde a medias.
func NormalizarMonedasActivas(in []MonedaActiva) ([]MonedaActiva, error) {
	out := make([]MonedaActiva, 0, len(in))
	vistos := map[string]bool{}
	for _, m := range in {
		cod := strings.ToUpper(strings.TrimSpace(m.Codigo))
		if cod == "" || cod == MonedaVES {
			continue // VES es la base legal; no es una divisa activable
		}
		if !DivisaConocida(cod) {
			return nil, fmt.Errorf("divisa desconocida: %q", m.Codigo)
		}
		fuente := strings.TrimSpace(m.Fuente)
		if fuente == "" {
			fuente = FuenteEfectivaDivisa(cod, "")
		}
		if !FuenteActivaValida(cod, fuente) {
			return nil, fmt.Errorf("fuente %q inválida para %s (bcv solo aplica al USD)", fuente, cod)
		}
		if vistos[cod] {
			continue
		}
		vistos[cod] = true
		out = append(out, MonedaActiva{Codigo: cod, Fuente: FuenteEfectivaDivisa(cod, fuente)})
	}
	return out, nil
}

// NormalizarColorHex valida y limpia un color hex de marca/fondo: acepta las
// formas «#RGB» y «#RRGGBB» (con o sin «#», mayúsculas o minúsculas) y devuelve
// siempre «#rrggbb» en minúsculas. Cualquier valor inválido (o vacío) se reduce a
// cadena vacía: el vacío significa «usar el color por defecto», nunca un hex roto.
func NormalizarColorHex(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.TrimPrefix(s, "#")
	if len(s) == 3 {
		// Expande la forma corta #abc → #aabbcc.
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return ""
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return ""
		}
	}
	return "#" + strings.ToLower(s)
}

// Modos de la pantalla del cliente del POS (la segunda ventana orientada al
// cliente). Gobiernan si esa pantalla muestra el carrito, la publicidad, o ambos.
const (
	// PantallaModoSoloProductos: solo el carrito y los totales (sin publicidad).
	PantallaModoSoloProductos = "solo_productos"
	// PantallaModoMixta: carrito + un panel de publicidad al lado (y el carrusel
	// a pantalla grande cuando no hay venta).
	PantallaModoMixta = "mixta"
	// PantallaModoPublicidad: toda la pantalla es publicidad SIEMPRE (señalización),
	// sin carrito, en cualquier estado.
	PantallaModoPublicidad = "publicidad"
)

// PantallaClienteModoValido indica si el modo es uno de los tres conocidos.
func PantallaClienteModoValido(m string) bool {
	switch strings.TrimSpace(m) {
	case PantallaModoSoloProductos, PantallaModoMixta, PantallaModoPublicidad:
		return true
	}
	return false
}

// NormalizarPantallaClienteModo devuelve un modo válido, cayendo en
// `solo_productos` (el default seguro) ante cualquier valor desconocido o vacío.
// Así las empresas ya creadas no necesitan migración.
func NormalizarPantallaClienteModo(m string) string {
	m = strings.TrimSpace(m)
	if PantallaClienteModoValido(m) {
		return m
	}
	return PantallaModoSoloProductos
}

// Versiones del logo que la pantalla del cliente puede mostrar arriba a la
// izquierda. Son las tres cargas de logo de la empresa: la de color (identidad,
// también va en comprobantes), la blanca (para fondos oscuros) y la negra (para
// fondos claros). El tema elige cuál usar (ver TemaPantallaCliente.LogoVersion).
const (
	// TemaLogoColor usa empresa.Logo (la de identidad; el default).
	TemaLogoColor = "color"
	// TemaLogoBlanco usa empresa.LogoBlanco (contraste sobre fondo oscuro).
	TemaLogoBlanco = "blanco"
	// TemaLogoNegro usa empresa.LogoNegro (contraste sobre fondo claro).
	TemaLogoNegro = "negro"
)

// NormalizarTemaLogoVersion acota la versión de logo a color|blanco|negro. Un
// valor vacío o desconocido queda en "" = usar el default (color); así las
// empresas ya creadas no necesitan migración.
func NormalizarTemaLogoVersion(v string) string {
	switch strings.TrimSpace(v) {
	case TemaLogoColor, TemaLogoBlanco, TemaLogoNegro:
		return strings.TrimSpace(v)
	}
	return ""
}

// TemaPantallaCliente es la APARIENCIA configurable de la pantalla del cliente
// del POS: los fondos (general y por espacio), los colores de texto y de énfasis,
// y qué versión del logo se muestra. Unifica en un solo lugar lo que antes estaba
// disperso (ColorMarca de empresa + ColorFondo/LogoVersion de la caja). Todos los
// campos son OPCIONALES: un color vacío hereda del Fondo general (o del default
// navy/teal/blanco), de modo que las empresas ya creadas no necesitan migración.
type TemaPantallaCliente struct {
	// Fondo es el fondo GENERAL de la pantalla. Vacío = degradado navy por defecto.
	Fondo string `json:"fondo" bson:"fondo"`
	// Fondos por ESPACIO; vacío = heredar del Fondo general (o su default).
	FondoCabecera   string `json:"fondoCabecera" bson:"fondocabecera"`
	FondoProductos  string `json:"fondoProductos" bson:"fondoproductos"`
	FondoTotales    string `json:"fondoTotales" bson:"fondototales"`
	FondoPublicidad string `json:"fondoPublicidad" bson:"fondopublicidad"`
	// ColorTexto es el color del texto principal. Vacío = blanco.
	ColorTexto string `json:"colorTexto" bson:"colortexto"`
	// ColorEnfasis es el acento (total, RIF, «vuelto»). Vacío = teal de marca.
	// Reemplaza el uso de ColorMarca en el display.
	ColorEnfasis string `json:"colorEnfasis" bson:"colorenfasis"`
	// LogoVersion elige qué logo se muestra en la pantalla del cliente:
	// color (Logo) | blanco (LogoBlanco) | negro (LogoNegro). Vacío = color.
	LogoVersion string `json:"logoVersion" bson:"logoversion"`
}

// Normalizar limpia el tema: normaliza cada color a un hex válido o a vacío
// (vacío = heredar/default) y acota la versión de logo. No falla nunca: un tema
// mal formado se reduce a sus defaults.
func (t TemaPantallaCliente) Normalizar() TemaPantallaCliente {
	return TemaPantallaCliente{
		Fondo:           NormalizarColorHex(t.Fondo),
		FondoCabecera:   NormalizarColorHex(t.FondoCabecera),
		FondoProductos:  NormalizarColorHex(t.FondoProductos),
		FondoTotales:    NormalizarColorHex(t.FondoTotales),
		FondoPublicidad: NormalizarColorHex(t.FondoPublicidad),
		ColorTexto:      NormalizarColorHex(t.ColorTexto),
		ColorEnfasis:    NormalizarColorHex(t.ColorEnfasis),
		LogoVersion:     NormalizarTemaLogoVersion(t.LogoVersion),
	}
}

// Tipos de slide del carrusel de publicidad de la pantalla del cliente.
const (
	// SlideTexto: un mensaje de marca (texto grande + subtexto), sin imagen.
	SlideTexto = "texto"
	// SlideImagen: una imagen (URL o data URI), a object-contain.
	SlideImagen = "imagen"
)

// AnuncioSlide es un slide del carrusel de publicidad de la pantalla del cliente.
// Puede ser de TEXTO (un mensaje de marca) o de IMAGEN (una URL o data URI). Los
// campos vacíos según el tipo se descartan al normalizar (ver la aplicación).
type AnuncioSlide struct {
	Tipo     string `json:"tipo" bson:"tipo"`
	Texto    string `json:"texto" bson:"texto"`
	Subtexto string `json:"subtexto" bson:"subtexto"`
	Imagen   string `json:"imagen" bson:"imagen"`
}

// UnmarshalBSONValue tolera los DATOS VIEJOS del campo `publicidad`, que antes era
// un `[]string` de URLs. Al decodificar cada elemento del arreglo, un elemento que
// no sea un documento (p. ej. un string suelto de la forma antigua) no rompe la
// carga de la empresa: queda como un slide vacío, que luego se filtra. Un documento
// se decodifica normalmente.
func (a *AnuncioSlide) UnmarshalBSONValue(t bsontype.Type, data []byte) error {
	*a = AnuncioSlide{}
	if t != bsontype.EmbeddedDocument {
		return nil // forma antigua (string suelto u otro): slide vacío, se filtra
	}
	type plano AnuncioSlide // evita recursión al reusar el decoder por defecto
	var p plano
	if err := bson.Unmarshal(data, &p); err != nil {
		return nil // tolerante: cualquier documento raro queda vacío
	}
	*a = AnuncioSlide(p)
	return nil
}

// Empresa es un contribuyente (RIF propio) dentro de una organización.
type Empresa struct {
	ID             string `json:"id" bson:"id"`
	OrganizacionID string `json:"organizacionId" bson:"organizacionid"`
	Nombre         string `json:"nombre" bson:"nombre"`
	RIF            string `json:"rif" bson:"rif"`
	Giro           string `json:"giro" bson:"giro"` // bodega|farmacia|ferreteria|servicios...
	Modalidad      string `json:"modalidadFacturacion" bson:"modalidad"`
	// Sandbox marca una empresa de PRUEBA (QA): clon de los maestros+config de otra,
	// con ledgers vacíos. Se excluye de la facturación/límites del plan y se rotula
	// en el selector de tenants. OrigenSandboxID apunta a la empresa de la que se
	// clonó; ExpiraEl (UTC RFC3339) es cuándo caduca (la plataforma la borra luego).
	Sandbox         bool   `json:"sandbox" bson:"sandbox"`
	OrigenSandboxID string `json:"origenSandboxId" bson:"origensandboxid"`
	ExpiraEl        string `json:"expiraEl" bson:"expirael"`
	// Datos fiscales de cabecera de factura. La factura legal los imprime; el
	// RIF y el nombre viven arriba (identidad del tenant), estos completan la
	// cabecera. RazonSocial es el nombre legal; Nombre es el comercial.
	RazonSocial string `json:"razonSocial" bson:"razonsocial"`
	Direccion   string `json:"direccion" bson:"direccion"`
	Telefono    string `json:"telefono" bson:"telefono"`
	Email       string `json:"email" bson:"email"`
	// Logo es una URL (o data URI base64) al logo de la empresa. Solo texto: la
	// subida de archivo como tal queda pendiente.
	Logo string `json:"logo" bson:"logo"`
	// ColorMarca es el color de acento de marca (hex, p. ej. «#09B69B»). Es la
	// FUENTE del branding de dos niveles: la pantalla del cliente lo usa como acento
	// (si está vacío, cae al teal por defecto). Opcional; se normaliza a un hex
	// válido o a vacío. Las empresas ya creadas no necesitan migración.
	ColorMarca string `json:"colorMarca" bson:"colormarca"`
	// LogoAlterno es la versión ALTERNA del logo (COMPAT: antes era la única otra
	// versión, para el contraste opuesto). Se conserva para no romper datos ya
	// guardados; el modelo nuevo usa LogoBlanco/LogoNegro. Opcional.
	LogoAlterno string `json:"logoAlterno" bson:"logoalterno"`
	// LogoBlanco y LogoNegro son versiones del logo para CONTRASTE (misma forma que
	// Logo: URL o data URI): la blanca para fondos oscuros, la negra para fondos
	// claros. El tema de la pantalla del cliente elige cuál mostrar (TemaPantalla.
	// LogoVersion). Opcionales; el logo de color (Logo) sigue siendo el default y el
	// que va en los comprobantes.
	LogoBlanco string `json:"logoBlanco" bson:"logoblanco"`
	LogoNegro  string `json:"logoNegro" bson:"logonegro"`
	// ExigirCedulaCliente obliga a identificar al cliente (cédula/RIF) antes de
	// cobrar en el POS. Desactivado por defecto (venta a Consumidor final, el
	// comportamiento actual). Las empresas ya creadas no necesitan migración.
	ExigirCedulaCliente bool `json:"exigirCedulaCliente" bson:"exigircedulacliente"`
	// BannerSuperior y BannerLateral son imágenes publicitarias del comercio que
	// se muestran en la pantalla auxiliar que ve el cliente en la caja (POS). Mismo
	// tipo/serialización que Logo: URL o data URI. Ambos opcionales.
	BannerSuperior string `json:"bannerSuperior" bson:"bannersuperior"`
	BannerLateral  string `json:"bannerLateral" bson:"bannerlateral"`
	// PantallaClienteModo elige QUÉ muestra la pantalla del cliente del POS:
	// `solo_productos` (carrito y totales), `mixta` (carrito + publicidad) o
	// `publicidad` (todo publicidad, señalización). Vacío se lee como
	// `solo_productos`: las empresas ya creadas no necesitan migración.
	PantallaClienteModo string `json:"pantallaClienteModo" bson:"pantallaclientemodo"`
	// Publicidad es la lista de slides que rotan en el carrusel de la pantalla del
	// cliente (en los modos `mixta` y `publicidad`). Cada slide es de TEXTO (mensaje
	// de marca) o de IMAGEN (URL o data URI). Vacía = solo la bienvenida. El comercio
	// la carga desde Configuración. Ver AnuncioSlide (tolera la forma antigua []string).
	Publicidad []AnuncioSlide `json:"publicidad" bson:"publicidad"`
	// TemaPantalla es la apariencia unificada de la pantalla del cliente (fondos por
	// espacio, colores de texto/énfasis, versión de logo). Se edita en Configuración
	// › Marketing con vista previa en vivo. Vacío = defaults (navy/teal/blanco): las
	// empresas ya creadas no necesitan migración. Ver TemaPantallaCliente.
	TemaPantalla TemaPantallaCliente `json:"temaPantalla" bson:"temapantalla"`

	Activa       bool `json:"activa" bson:"activa"`
	OnboardingOK bool `json:"onboardingOk" bson:"onboardingok"`
	// RequiereSupervisorPin: interruptor de seguridad de caja (flujo 2.4),
	// desactivado por defecto.
	RequiereSupervisorPin bool `json:"requiereSupervisorPin" bson:"requieresupervisorpin"`
	// MonedaPrincipal es la moneda en la que la empresa captura sus precios
	// (R10). Vacío se lee como VES: las empresas ya creadas no necesitan
	// migración.
	MonedaPrincipal string `json:"monedaPrincipal" bson:"monedaprincipal"`
	// FuenteTasa es de dónde viene la tasa de cambio para esta empresa (R9).
	FuenteTasa string `json:"fuenteTasa" bson:"fuentetasa"`
	// PreciosEnUsd habilita capturar precios de producto en dólares aunque la
	// moneda principal sea el bolívar.
	PreciosEnUsd bool `json:"preciosEnUsd" bson:"preciosenusd"`
	// MonedasActivas son las divisas que la empresa habilitó, cada una con su
	// fuente de tasa. VACÍO se comporta como hoy: solo USD activo con FuenteTasa
	// (retrocompat; las empresas ya creadas no necesitan migración). El bolívar
	// (VES) es la base legal y no aparece aquí: siempre está, con tasa 1.
	MonedasActivas []MonedaActiva `json:"monedasActivas" bson:"monedasactivas"`
	// AlicuotaIVA y AlicuotaIGTF son las tasas de impuesto configuradas por la
	// empresa (fracciones: 0.16 = 16%). Materializan «compliance as configuration»:
	// una nueva providencia se aplica sin recompilar. Valor 0 = usar el default del
	// sistema (fiscal.AlicuotaIVA / fiscal.AlicuotaIGTF); las empresas ya creadas no
	// necesitan migración. La tasa aplicada se GRABA en cada documento al emitir, así
	// que cambiarla aquí solo rige para documentos nuevos (ADR tasa histórica).
	AlicuotaIVA  float64 `json:"alicuotaIVA" bson:"alicuotaiva"`
	AlicuotaIGTF float64 `json:"alicuotaIGTF" bson:"alicuotaigtf"`
	// Agente de retención: si la empresa RETIENE IVA/ISLR a sus proveedores. Cuando
	// lo es, al registrar una retención EMITIDA el sistema genera el número de
	// comprobante correlativo (AAAAMM + secuencia mensual). Valor cero = no es agente
	// (las empresas ya creadas no necesitan migración). RetencionIVAPorcentaje es el
	// % por defecto de retención de IVA (0 ⇒ 75%); el de ISLR depende del concepto.
	AgenteRetencionIVA     bool    `json:"agenteRetencionIVA" bson:"agenteretencioniva"`
	AgenteRetencionISLR    bool    `json:"agenteRetencionISLR" bson:"agenteretencionislr"`
	RetencionIVAPorcentaje float64 `json:"retencionIVAPorcentaje" bson:"retencionivaporcentaje"`
	// Rango AUTORIZADO del Número de Control (SENIAT). Prefijo de 2 dígitos ("00" por
	// defecto) + rango [Desde, Hasta] autorizado por la providencia. El correlativo
	// vivo lo lleva el Numerador (serie "CTRL"); estos campos definen el prefijo, el
	// inicio (se fija el contador a Desde-1) y el tope para avisar/agotar. Hasta 0 =
	// sin tope declarado. Valor cero ⇒ prefijo "00", correlativo continuo desde 1.
	NumeroControlPrefijo string `json:"numeroControlPrefijo" bson:"numerocontrolprefijo"`
	NumeroControlDesde   int    `json:"numeroControlDesde" bson:"numerocontroldesde"`
	NumeroControlHasta   int    `json:"numeroControlHasta" bson:"numerocontrolhasta"`
	// Asistente IA (módulo "asistente-ia"). La capa MECÁNICA (respuestas sobre los
	// datos de la instancia) es siempre local y no requiere configuración. Estos
	// campos gobiernan la capa de IA OPT-IN: solo si AsistenteIAHabilitada es true la
	// pregunta abierta se envía al proxy de IA de Hubmy, con un resumen acotado al rol
	// como contexto. AsistenteIAModelo vacío ⇒ modelo por defecto del servidor.
	AsistenteIAHabilitada bool   `json:"asistenteIAHabilitada" bson:"asistenteiahabilitada"`
	AsistenteIAModelo     string `json:"asistenteIAModelo" bson:"asistenteiamodelo"`
	Creada                string `json:"creada" bson:"creada"`
}

// Repository es el puerto de persistencia de empresas.
type Repository interface {
	// List devuelve las empresas de una organización.
	List(organizacionID string) []Empresa
	ByID(id string) (Empresa, bool)
	Create(e Empresa) Empresa
	Update(e Empresa) (Empresa, bool)
}
