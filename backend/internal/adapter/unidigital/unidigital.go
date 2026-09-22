// Package unidigital habla con DigitalInvoice, la API de la imprenta digital
// Corporación Unidigital 1220 C.A.
//
// Todo lo que este paquete asume está verificado contra el sandbox y anotado en
// docs/unidigital-api-verificada.md. Tres cosas de esa verificación mandan acá:
//
//   - LA CONTRASEÑA VIAJA EN SHA-512 hexadecimal, no en texto plano.
//   - EL LOGIN NO ES LA FUENTE DE VERDAD DE LAS SERIES. Devolvió `series: []` en
//     una cuenta que sí tenía una. Las series se piden por `GET /series`.
//   - HAY QUE MIRAR `hasErrors`, NO EL CÓDIGO HTTP. La API responde 200 con
//     errores adentro.
package unidigital

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Sobre es el envoltorio de TODAS las respuestas de la API. Deserializarlo
// siempre —y no asumir que la carga viene en la raíz— es lo que evita leer un
// error como si fuera un resultado.
type Sobre struct {
	Result      json.RawMessage `json:"result"`
	Errors      []ErrorAPI      `json:"errors"`
	Warnings    []ErrorAPI      `json:"warnings"`
	Information []string        `json:"information"`
	HasErrors   bool            `json:"hasErrors"`
}

// ErrorAPI son los tres campos que la documentación insiste en registrar: es
// exactamente lo que soporte pide cuando algo se rechaza.
type ErrorAPI struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Extra   string `json:"extra"`
}

// Error implementa error para poder propagarlo con sus tres campos intactos.
func (e ErrorAPI) Error() string {
	if e.Extra != "" {
		return fmt.Sprintf("%s (%s · %s)", e.Message, e.Code, e.Extra)
	}
	if e.Code != "" {
		return fmt.Sprintf("%s (%s)", e.Message, e.Code)
	}
	return e.Message
}

// Fallo es un error de la API con su código HTTP, para que quien llame pueda
// decidir si reintenta. La distinción importa: un 429 se reintenta, un 400 no.
type Fallo struct {
	HTTP   int
	Errors []ErrorAPI
}

func (f *Fallo) Error() string {
	if len(f.Errors) == 0 {
		return fmt.Sprintf("HTTP %d", f.HTTP)
	}
	partes := make([]string, 0, len(f.Errors))
	for _, e := range f.Errors {
		partes = append(partes, e.Error())
	}
	return strings.Join(partes, "; ")
}

// Primero devuelve el primer error, para registrarlo con sus tres campos.
func (f *Fallo) Primero() ErrorAPI {
	if len(f.Errors) == 0 {
		return ErrorAPI{Code: fmt.Sprintf("HTTP%d", f.HTTP), Message: f.Error()}
	}
	return f.Errors[0]
}

// Temporal indica si vale la pena reintentar.
//
//	429 = la petición ANTERIOR de esta cuenta sigue procesándose. No es rate
//	      limiting: es el motivo por el que los envíos de una serie van de a uno.
//	401 = token vencido; se reintenta tras volver a autenticar.
//	5xx = problema del otro lado.
//
// Un 400 NO es temporal: es una regla de negocio incumplida, y reintentarlo a
// ciegas repite el error y quema otro correlativo.
func (f *Fallo) Temporal() bool {
	return f.HTTP == 429 || f.HTTP == 401 || f.HTTP >= 500 || f.HTTP == 0
}

// Cliente habla con una cuenta de la imprenta. Cachea el token (dura 8 h) y lo
// renueva solo. NO es un cliente por empresa: se construye con las credenciales
// de la empresa que va a emitir.
type Cliente struct {
	base     string
	usuario  string
	digest   string // SHA-512 hexadecimal de la contraseña
	http     *http.Client
	mu       sync.Mutex
	token    string
	expiraEl time.Time
}

// HashPassword devuelve el digest que la API espera. Está expuesto porque la
// configuración guarda el digest, no la contraseña: es lo que viaja, así que
// guardar el texto plano no aporta nada y sí expone.
func HashPassword(clara string) string {
	suma := sha512.Sum512([]byte(clara))
	return hex.EncodeToString(suma[:])
}

// Nuevo construye el cliente. `digest` ya viene hasheado.
func Nuevo(base, usuario, digest string) *Cliente {
	return &Cliente{
		base: strings.TrimRight(base, "/"), usuario: usuario, digest: digest,
		// Timeout generoso: la emisión es una operación de escritura fiscal y
		// cortarla a los 5 segundos deja la duda de si el documento se creó.
		http: &http.Client{Timeout: 60 * time.Second},
	}
}

// Login autentica y guarda el token. Devuelve los avisos de configuración, que
// son donde la API dice «esta serie no tiene plantilla de correo» y cosas así.
func (c *Cliente) Login(ctx context.Context) ([]string, error) {
	cuerpo := map[string]string{"UserName": c.usuario, "Password": c.digest}
	var resp struct {
		AccessToken string   `json:"accessToken"`
		Information []string `json:"information"`
		Environment string   `json:"environment"`
	}
	// El login NO devuelve el sobre estándar: responde el objeto directo.
	if err := c.crudo(ctx, http.MethodPost, "/user/login", cuerpo, &resp, false); err != nil {
		return nil, err
	}
	if strings.TrimSpace(resp.AccessToken) == "" {
		return resp.Information, fmt.Errorf("la imprenta no devolvió token de acceso")
	}
	c.mu.Lock()
	c.token = resp.AccessToken
	// El token dura 8 h; se renueva a las 7 para no descubrirlo venciendo en
	// mitad de una emisión.
	c.expiraEl = time.Now().Add(7 * time.Hour)
	c.mu.Unlock()
	return resp.Information, nil
}

// asegurarToken autentica si hace falta.
func (c *Cliente) asegurarToken(ctx context.Context) error {
	c.mu.Lock()
	vigente := c.token != "" && time.Now().Before(c.expiraEl)
	c.mu.Unlock()
	if vigente {
		return nil
	}
	_, err := c.Login(ctx)
	return err
}

// llamar hace una petición autenticada y deserializa `result` en `destino`.
// Ante un 401 vuelve a autenticar y reintenta UNA vez: el token vence a las 8 h
// y no tiene sentido fallar una factura por eso.
func (c *Cliente) llamar(ctx context.Context, metodo, ruta string, cuerpo, destino any) error {
	if err := c.asegurarToken(ctx); err != nil {
		return err
	}
	err := c.conSobre(ctx, metodo, ruta, cuerpo, destino)
	var f *Fallo
	if err != nil && esFallo(err, &f) && f.HTTP == 401 {
		c.mu.Lock()
		c.token = ""
		c.mu.Unlock()
		if err2 := c.asegurarToken(ctx); err2 != nil {
			return err2
		}
		return c.conSobre(ctx, metodo, ruta, cuerpo, destino)
	}
	return err
}

func esFallo(err error, destino **Fallo) bool {
	f, ok := err.(*Fallo)
	if ok {
		*destino = f
	}
	return ok
}

// conSobre hace la petición y evalúa `hasErrors`, que es la bandera que manda
// aunque el HTTP sea 200.
func (c *Cliente) conSobre(ctx context.Context, metodo, ruta string, cuerpo, destino any) error {
	var sobre Sobre
	if err := c.crudo(ctx, metodo, ruta, cuerpo, &sobre, true); err != nil {
		return err
	}
	if sobre.HasErrors {
		return &Fallo{HTTP: 200, Errors: sobre.Errors}
	}
	if destino != nil && len(sobre.Result) > 0 {
		return json.Unmarshal(sobre.Result, destino)
	}
	return nil
}

// crudo hace la petición HTTP. `autenticada` decide si manda el token.
func (c *Cliente) crudo(ctx context.Context, metodo, ruta string, cuerpo, destino any, autenticada bool) error {
	var lector io.Reader
	if cuerpo != nil {
		datos, err := json.Marshal(cuerpo)
		if err != nil {
			return err
		}
		lector = bytes.NewReader(datos)
	}
	req, err := http.NewRequestWithContext(ctx, metodo, c.base+ruta, lector)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if autenticada {
		c.mu.Lock()
		tok := c.token
		c.mu.Unlock()
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	res, err := c.http.Do(req)
	if err != nil {
		// Sin respuesta: se trata como temporal (HTTP 0) para que la cola
		// reintente en vez de dar la factura por perdida.
		return &Fallo{HTTP: 0, Errors: []ErrorAPI{{Code: "RED", Message: err.Error()}}}
	}
	defer res.Body.Close()
	datos, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))

	if res.StatusCode >= 400 {
		var sobre Sobre
		_ = json.Unmarshal(datos, &sobre)
		return &Fallo{HTTP: res.StatusCode, Errors: sobre.Errors}
	}
	if destino == nil {
		return nil
	}
	return json.Unmarshal(datos, destino)
}

/* --- Operaciones ---------------------------------------------------------- */

// Serie es una serie de facturación de la cuenta.
type Serie struct {
	StrongID                string `json:"strongId"`
	Name                    string `json:"name"`
	LastControlNumberUsed   int    `json:"lastControlNumberUsed"`
	UseSystemReferenceAsKey bool   `json:"useSystemReferenceAsKey"`
}

// Series lista las series EXISTENTES. Es la fuente de verdad: el login devolvió
// `series: []` en una cuenta que sí tenía una.
func (c *Cliente) Series(ctx context.Context) ([]Serie, error) {
	var out []Serie
	err := c.llamar(ctx, http.MethodGet, "/series", nil, &out)
	return out, err
}

// SeriesConfiguradas lista las series HABILITADAS para emitir. Vacío significa
// que la cuenta no puede facturar todavía, por más que `Series` devuelva algo:
// es exactamente el estado en que nos entregaron el sandbox.
func (c *Cliente) SeriesConfiguradas(ctx context.Context) ([]Serie, error) {
	var out []Serie
	err := c.llamar(ctx, http.MethodPost, "/series/list", map[string]any{}, &out)
	return out, err
}

// Contador es en qué número va la imprenta para una serie y un tipo de
// documento. Es el punto de partida del correlativo: el avance lo lleva quien
// emite, pero de DÓNDE arranca lo dice la imprenta, y arrancar en otro número
// hace que rechace la primera factura por estar fuera de orden.
type Contador struct {
	StrongID     string `json:"strongId"`
	Serie        string `json:"serie"`
	DocumentType string `json:"documentType"`
	Counter      int    `json:"counter"`
}

// Contadores trae el correlativo actual de cada serie y tipo.
func (c *Cliente) Contadores(ctx context.Context) ([]Contador, error) {
	var out []Contador
	err := c.llamar(ctx, http.MethodGet, "/series/counters", nil, &out)
	return out, err
}

// NombreTipo traduce el `codeName` que usamos al nombre con el que la imprenta
// rotula sus contadores. Se hace acá y no en la aplicación porque es vocabulario
// de ellos: si mañana lo cambian, se cambia en un solo sitio.
func NombreTipo(codeName string) string {
	switch codeName {
	case "FA":
		return "Factura"
	case "NC":
		return "Nota de Crédito"
	case "ND":
		return "Nota de Débito"
	case "GD":
		return "Guia de Despacho"
	}
	return ""
}

// Sucursal es una oficina comercial; su StrongID es obligatorio al emitir.
type Sucursal struct {
	StrongID string `json:"strongId"`
	Name     string `json:"name"`
}

// Sucursales lista las oficinas comerciales. La respuesta viene paginada.
func (c *Cliente) Sucursales(ctx context.Context) ([]Sucursal, error) {
	var pagina struct {
		Result []Sucursal `json:"result"`
	}
	if err := c.llamar(ctx, http.MethodGet, "/commercialOffice", nil, &pagina); err != nil {
		return nil, err
	}
	return pagina.Result, nil
}

// MetodoPago es una forma de pago del catálogo de la imprenta.
type MetodoPago struct {
	Name string `json:"name"`
	Code string `json:"code"`
}

// MetodosPago trae el catálogo (p2p, cash, tdd, tdc, trf…). Se lee de ellos en
// vez de escribirlo acá: es su vocabulario, no el nuestro.
func (c *Cliente) MetodosPago(ctx context.Context) ([]MetodoPago, error) {
	var out []MetodoPago
	err := c.llamar(ctx, http.MethodGet, "/companies/paymentMethod", nil, &out)
	return out, err
}

// CrearYAprobar emite un documento. Devuelve el strongId que asigna la imprenta.
// NO devuelve número de control: ese llega después, por consulta.
func (c *Cliente) CrearYAprobar(ctx context.Context, doc map[string]any) (string, error) {
	var id string
	if err := c.llamar(ctx, http.MethodPost, "/documents/createandapprove", doc, &id); err != nil {
		return "", err
	}
	return id, nil
}

// Documento es lo que devuelve la consulta. `Status` en "Assigned" y
// `ControlNumber` distinto de cero significan que ya es fiscal.
type Documento struct {
	StrongID      string `json:"strongId"`
	CodeName      string `json:"codeName"`
	Serie         string `json:"serie"`
	Number        int    `json:"number"`
	ControlNumber int    `json:"controlNumber"`
	// ControlNumberFormatted es el número tal como se imprime («00-00000002») y es
	// el que el cliente ve y reclama. El entero sirve para anular; este, para
	// mostrar. Guardar solo el entero obligaría a re-inventar el formato acá, y el
	// formato lo decide la imprenta.
	ControlNumberFormatted string  `json:"controlNumberFormatted"`
	Status                 string  `json:"status"`
	Name                   string  `json:"name"`
	GrandTotal             float64 `json:"grandTotal"`
}

// PorStrongID consulta un documento. Es la llamada que cierra el ciclo
// asíncrono: de acá sale el número de control.
func (c *Cliente) PorStrongID(ctx context.Context, strongID string) (Documento, error) {
	var out Documento
	err := c.llamar(ctx, http.MethodGet, "/documents?strongId="+url.QueryEscape(strongID), nil, &out)
	return out, err
}

// PorSystemReference busca por NUESTRO identificador. Es la salvaguarda de
// idempotencia: si un envío se cortó sin respuesta, esto dice si el documento
// llegó a crearse, antes de reintentar y quemar otro correlativo.
func (c *Cliente) PorSystemReference(ctx context.Context, ref string) (Documento, error) {
	var out Documento
	err := c.llamar(ctx, http.MethodPost, "/documents/searchbysystemreference",
		map[string]string{"SystemReference": ref}, &out)
	return out, err
}

// CodigoCorto pide el código corto del documento, pensado para compartirlo.
//
// Va por GET: con POST la API responde 405. Verificado contra el sandbox el
// 22/09/2026 — la colección lo documentaba como POST.
func (c *Cliente) CodigoCorto(ctx context.Context, strongID string) (string, error) {
	var out string
	if err := c.llamar(ctx, http.MethodGet, "/documents/short/"+url.PathEscape(strongID), nil, &out); err != nil {
		return "", err
	}
	return out, nil
}

// URLDocumento pide la URL de visualización del documento.
//
// `result` es un OBJETO {url}, no el texto de la URL —a diferencia del código
// corto, que sí viene pelado—. Leerlo como texto fallaba en silencio: el
// documento quedaba fiscal y sin enlace, y la página pública del cliente perdía
// el botón para ver y descargar su factura, que es a lo que fue.
func (c *Cliente) URLDocumento(ctx context.Context, strongID string) (string, error) {
	var out struct {
		URL string `json:"url"`
	}
	ruta := "/documents/view/?documentStrongId=" + url.QueryEscape(strongID)
	if err := c.llamar(ctx, http.MethodPost, ruta, nil, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

// Anular anula un documento POR SU NÚMERO DE CONTROL, no por su id. Es como lo
// espera la API, y tiene sentido: el número de control es lo que lo hizo fiscal.
func (c *Cliente) Anular(ctx context.Context, numeroControl int) error {
	return c.llamar(ctx, http.MethodPost, "/documents/anulled",
		map[string]int{"Control": numeroControl}, nil)
}
