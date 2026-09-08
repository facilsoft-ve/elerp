// Package tasafuente obtiene la tasa de cambio Bs/US$ de fuentes externas.
//
// Implementa el puerto tasa.Proveedor bajo las nueve condiciones de seguridad
// acordadas con el cliente (backlog-ux.md → R9). Las que se cumplen AQUÍ:
//
//	1. Solo tráfico saliente: son peticiones GET desde el backend. No abre
//	   ningún puerto ni endpoint entrante.
//	3. Nada de lo que llega se ejecuta: el HTML se recorre con un extractor
//	   estricto de números. No se parsea el DOM, no se evalúa JavaScript y no
//	   hay navegador headless en producción.
//	4. TLS verificado siempre: nunca InsecureSkipVerify, TLS 1.2 mínimo. Si el
//	   certificado falla, la obtención falla y se usa la última tasa buena.
//	5. Timeout corto y cuerpo acotado: la caja no se bloquea esperando al BCV.
//	6. La petición no lleva nada nuestro: sin cookies, sin credenciales, sin
//	   identificadores de tenant. Es una consulta anónima a un sitio público.
//
// La validación del valor (condición 2), la auditoría (8) y la periodicidad
// diaria (7) viven en la capa de aplicación, que es donde está la regla de
// negocio.
package tasafuente

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mornix/elerp/internal/domain/tasa"
)

// URL por defecto del sitio del BCV. La tasa oficial vive en el HTML de la
// portada: el BCV no publica API.
const URLBCVPorDefecto = "https://www.bcv.org.ve/"

const (
	// timeout es corto a propósito (condición 5).
	timeout = 8 * time.Second
	// maxCuerpo acota lo que se lee de la respuesta. La portada del BCV pesa
	// unos cientos de KiB; 2 MiB deja margen sin permitir que una fuente
	// comprometida nos haga tragar un cuerpo infinito.
	maxCuerpo = 2 << 20
)

// ErrSinDato indica que la fuente respondió pero no se encontró una tasa
// reconocible. Es el caso de «el BCV cambió su HTML».
var ErrSinDato = errors.New("la fuente no expuso una tasa reconocible")

/* Reparación de la cadena del BCV.
 *
 * Hallazgo verificado (31 jul 2026): `www.bcv.org.ve` presenta un certificado
 * emitido por «Sectigo Public Server Authentication CA DV R36» pero envía como
 * intermedio el viejo «Sectigo RSA Domain Validation Secure Server CA», que NO
 * firma ese certificado. La cadena queda incompleta, así que CUALQUIER cliente
 * correcto (Go, curl, wget) falla con «unable to verify». Es un error de
 * configuración del BCV, no nuestro.
 *
 * La condición 4 de seguridad prohíbe `InsecureSkipVerify`, y se respeta: la
 * salida es cargar el intermedio correcto en un PEM y apuntarlo con
 * TASA_CA_EXTRA. Con eso la verificación sigue siendo real —firma, vigencia y
 * nombre de host— solo que el ancla de confianza para ese host es el intermedio
 * que nosotros instalamos deliberadamente. Sin el PEM, la fuente primaria falla
 * y se usa el respaldo: degradado, auditado y nunca silencioso.
 */
const EnvCAExtra = "TASA_CA_EXTRA"

// poolDeConfianza devuelve el pool de raíces del sistema, más los certificados
// del PEM de TASA_CA_EXTRA si está configurado. Devuelve nil si no hay extras,
// para que Go use su verificación estándar tal cual.
func poolDeConfianza() *x509.CertPool {
	ruta := strings.TrimSpace(os.Getenv(EnvCAExtra))
	if ruta == "" {
		return nil
	}
	pem, err := os.ReadFile(ruta)
	if err != nil {
		return nil
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil
	}
	return pool
}

// cliente construye el http.Client compartido por los proveedores.
func cliente() *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			// TLS verificado: no se toca InsecureSkipVerify jamás (condición 4).
			TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: poolDeConfianza()},
			TLSHandshakeTimeout: 5 * time.Second,
			DisableKeepAlives:   true,
		},
		// Una redirección es normal (http→https, con/sin www); una cadena larga
		// es señal de que nos están paseando.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("demasiadas redirecciones")
			}
			return nil
		},
	}
}

// traer hace la petición anónima y devuelve el cuerpo acotado.
func traer(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// La petición no lleva nada nuestro (condición 6): ni cookies, ni tokens,
	// ni identificadores de empresa. Solo un User-Agent honesto para no
	// aparecer como tráfico anónimo sospechoso.
	req.Header.Set("User-Agent", "ElERP/1.0 (consulta de tasa oficial)")
	req.Header.Set("Accept", "text/html,application/json")

	resp, err := cliente().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("la fuente respondió %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxCuerpo))
}

// --- Fuente primaria: raspado del BCV ---

// BCV lee la tasa oficial de la portada del Banco Central de Venezuela.
type BCV struct {
	URL string
}

// NewBCV construye el proveedor. Con url vacía usa la portada oficial.
func NewBCV(url string) *BCV {
	if strings.TrimSpace(url) == "" {
		url = URLBCVPorDefecto
	}
	return &BCV{URL: url}
}

// Nombre implementa tasa.Proveedor.
func (b *BCV) Nombre() string { return tasa.FuenteBCV }

// Obtener raspa la portada y extrae la tasa del dólar.
func (b *BCV) Obtener(ctx context.Context) (tasa.Lectura, error) {
	cuerpo, err := traer(ctx, b.URL)
	if err != nil {
		return tasa.Lectura{}, err
	}
	valor, fecha, err := ExtraerBCV(cuerpo)
	if err != nil {
		return tasa.Lectura{}, err
	}
	return tasa.Lectura{
		Valor: valor, FechaValor: fecha,
		Fuente: tasa.FuenteBCV, Detalle: hostDe(b.URL),
	}, nil
}

// Bloque del dólar en la portada del BCV: <div id="dolar"> … <strong>745,63</strong>.
// Se busca el ancla por id y luego el primer número dentro de una etiqueta
// <strong> cercana. Es un extractor de texto, NO un parser de HTML: no
// construye un árbol, no sigue referencias y no ejecuta nada (condición 3).
var (
	reAnclaDolar = regexp.MustCompile(`(?i)id\s*=\s*["']dolar["']`)
	reStrong     = regexp.MustCompile(`(?is)<strong[^>]*>\s*([0-9][0-9.,\s]*)\s*</strong>`)
	// Fecha valor publicada por el BCV: <span content="2026-07-31T00:00:00-04:00" …>
	reFechaValor = regexp.MustCompile(`content\s*=\s*["'](\d{4}-\d{2}-\d{2})T`)
)

// ventanaDolar limita cuánto se mira después del ancla del dólar. La cifra está
// a pocas decenas de bytes; una ventana corta evita capturar el euro o la lira.
const ventanaDolar = 600

// ExtraerBCV es la parte pura y verificable del raspado: recibe el HTML y
// devuelve el valor y la fecha valor (YYYY-MM-DD, vacía si no la declara).
//
// Se mantiene exportada porque es lo que se prueba contra un fixture del HTML
// real: si el BCV cambia su portada, esta función es la única que hay que tocar.
func ExtraerBCV(html []byte) (float64, string, error) {
	ancla := reAnclaDolar.FindIndex(html)
	if ancla == nil {
		return 0, "", fmt.Errorf("%w: no se encontró el bloque del dólar", ErrSinDato)
	}
	fin := ancla[1] + ventanaDolar
	if fin > len(html) {
		fin = len(html)
	}
	m := reStrong.FindSubmatch(html[ancla[1]:fin])
	if m == nil {
		return 0, "", fmt.Errorf("%w: el bloque del dólar no traía una cifra", ErrSinDato)
	}
	valor, err := ParsearDecimalVE(string(m[1]))
	if err != nil {
		return 0, "", err
	}
	fecha := ""
	if f := reFechaValor.FindSubmatch(html); f != nil {
		fecha = string(f[1])
	}
	return valor, fecha, nil
}

// reDecimalVE acepta solo la forma venezolana: punto de miles opcional y coma
// decimal. Cualquier otra cosa se rechaza — es la puerta por la que entraría un
// HTML alterado (condición 2).
var reDecimalVE = regexp.MustCompile(`^[0-9]{1,3}(?:\.[0-9]{3})*(?:,[0-9]{1,10})?$|^[0-9]{1,12}(?:,[0-9]{1,10})?$`)

// ParsearDecimalVE convierte "1.234,56" en 1234.56 con un formato estricto.
func ParsearDecimalVE(s string) (float64, error) {
	limpio := strings.Join(strings.Fields(s), "")
	if limpio == "" {
		return 0, fmt.Errorf("%w: cifra vacía", ErrSinDato)
	}
	if !reDecimalVE.MatchString(limpio) {
		return 0, fmt.Errorf("%w: cifra con formato inesperado (%q)", ErrSinDato, s)
	}
	normal := strings.ReplaceAll(limpio, ".", "")
	normal = strings.ReplaceAll(normal, ",", ".")
	v, err := strconv.ParseFloat(normal, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrSinDato, err)
	}
	return v, nil
}

// --- Fuente de respaldo / mercado: API JSON de terceros ---

// APIJSON lee la tasa de una API que republica al BCV (o el promedio del
// mercado). Es el respaldo cuando el sitio del BCV no responde o cambió.
type APIJSON struct {
	URL string
	// fuente es la etiqueta con la que se registra (respaldo o mercado): la
	// interfaz muestra siempre el origen real, nunca «BCV» por defecto.
	fuente string
}

// NewRespaldo construye el proveedor de respaldo (tasa oficial republicada).
func NewRespaldo(url string) *APIJSON { return &APIJSON{URL: url, fuente: tasa.FuenteRespaldo} }

// NewMercado construye el proveedor de tasa promedio de mercado.
func NewMercado(url string) *APIJSON { return &APIJSON{URL: url, fuente: tasa.FuenteMercado} }

// Nombre implementa tasa.Proveedor.
func (a *APIJSON) Nombre() string { return a.fuente }

// respuestaAPI es el contrato mínimo que se le exige a la API de terceros. El
// decodificador ignora todo lo demás: nada de mapas dinámicos ni de datos que
// entren al dominio sin pasar por aquí.
type respuestaAPI struct {
	Price      float64 `json:"price"`
	LastUpdate string  `json:"last_update"`
}

// Obtener consulta la API y decodifica el precio de forma estricta.
func (a *APIJSON) Obtener(ctx context.Context) (tasa.Lectura, error) {
	if strings.TrimSpace(a.URL) == "" {
		return tasa.Lectura{}, errors.New("fuente de respaldo sin URL configurada")
	}
	cuerpo, err := traer(ctx, a.URL)
	if err != nil {
		return tasa.Lectura{}, err
	}
	var r respuestaAPI
	if err := json.Unmarshal(cuerpo, &r); err != nil {
		return tasa.Lectura{}, fmt.Errorf("%w: respuesta ilegible (%v)", ErrSinDato, err)
	}
	if r.Price <= 0 {
		return tasa.Lectura{}, fmt.Errorf("%w: la respuesta no traía un precio", ErrSinDato)
	}
	return tasa.Lectura{
		Valor: r.Price, FechaValor: fechaDe(r.LastUpdate),
		Fuente: a.fuente, Detalle: hostDe(a.URL),
	}, nil
}

// fechaDe extrae YYYY-MM-DD de una marca de tiempo de la API, si viene en un
// formato reconocible. Si no, devuelve vacío y la aplicación usa el día de hoy:
// preferimos no tener fecha a inventarla.
func fechaDe(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 {
		if _, err := time.Parse("2006-01-02", s[:10]); err == nil {
			return s[:10]
		}
	}
	return ""
}

// hostDe extrae el host de una URL para registrarlo como detalle de origen, sin
// arrastrar parámetros de consulta al histórico.
func hostDe(url string) string {
	s := strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://")
	if i := strings.IndexAny(s, "/?"); i > 0 {
		s = s[:i]
	}
	return s
}
