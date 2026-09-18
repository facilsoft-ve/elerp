# UniDigital DigitalInvoice — contrato verificado contra el sandbox

Verificado el **18 de septiembre de 2026** contra `https://qa.unidigital.global/digitalinvoice-core`
con las credenciales de integrador de Mornix.

Este documento **corrige y completa** la guía de desarrollo que teníamos, que se armó leyendo la
colección publicada y dejaba unos diez puntos «por confirmar», tres de ellos bloqueantes. Todo lo de
acá está comprobado: o se llamó al sandbox, o se leyó la colección completa de Postman (587 KB, de
`documenter.gw.postman.com/api/collections/7522266/2sAXqwXec7`).

> Las credenciales NO viven en este repositorio. Van por variable de entorno.

---

## 1. Lo que la guía decía mal

| La guía decía | Lo verificado |
|---|---|
| «Lee `series[].strongId` del login y cachéalo» | **El login devolvió `series: []`** y el aviso «La compañía no tiene series configuradas». La serie sí existe y sale por `GET /series`. **El login no es la fuente de verdad de las series.** |
| «`DocumentType` de NC/ND por confirmar» | `POST /documenttypes/list` los da: `FA`, `NC`, `ND`, `GD`, `RT`, `NO`, `NN` y las anulaciones `AF`/`AN`/… |
| «Endpoint de consulta por confirmar» | `GET /documents?strongId=…`, `GET /documents/info/{guid}` y **`POST /documents/searchbysystemreference`** (con nuestro propio id). |
| «Anulación por confirmar» | `POST /documents/anulled` con `{"Control": <número de control>}`. |
| «PDF por confirmar» | `POST /documents/view/?documentStrongId=…` (URL), `POST /documents/view/{guid}` (stream PDF), `POST /documents/viewjson/{guid}` (JSON). |
| «Catálogos por confirmar» | `GET /companies/tax`, `/companies/paymentMethod`, `/companies/unitMeasure`, `/companies/retentionCodes`, `GET /commercialOffice/states`. |
| «El paginado va en el cuerpo» | Va en la **query string**: `POST /documents/list?page=1&thefrom=…&theTo=…`. |

---

## 2. Estado de la cuenta de pruebas (bloqueante)

```
POST /user/login        → 200. JWT. information: "La compañía no tiene series configuradas"
GET  /series            → 200. [{ strongId: 29f1864d-…, name: "0", lastControlNumberUsed: -1,
                                  useSystemReferenceAsKey: true, calculatedByCompany: false }]
POST /series/list       → 200. []            ← la serie NO está configurada para emitir
GET  /commercialOffice  → 200. totalCount: 0 ← sin sucursales
POST /documents/createandapprove → 400 "La serie no es válida 29f1864d-…"
```

**Todavía no se puede emitir.** Hay que pedirle a UniDigital (api@unidigital.global) que configure,
para `mornix@unidigital.global`:

1. Una **serie habilitada** para emisión (la serie `0` existe pero no está asignada).
2. Al menos una **sucursal** (`SucursalStrongId` es obligatorio en el cuerpo).
3. La **plantilla de correo** de la serie (el propio login avisa que falta).

---

## 3. Endpoints que nos importan

### Sesión y catálogos
| Método | Ruta | Para qué |
|---|---|---|
| POST | `/user/login` | `{UserName, Password}`, contraseña en **SHA-512 hexadecimal**. Token de 8 h. |
| GET | `/series` | Series existentes con su `strongId`. **Fuente de verdad**, no el login. |
| POST | `/series/list` | Series **configuradas** para emitir. Vacío = no se puede facturar. |
| GET | `/series/counters` | Último `Number` usado **por serie y tipo**. Con esto se concilia nuestro correlativo. |
| POST | `/documenttypes/list` | Tipos soportados con su `codeName`. |
| GET | `/commercialOffice` | Sucursales → `SucursalStrongId`. Paginado. |
| GET | `/companies/tax` | Alícuotas con su `regulatedCode` (`G` 16 %, `R` 8 %, lujo 31 %). |
| GET | `/companies/paymentMethod` | `p2p`, `cash`, `tdd`, `tdc`, `trf`, `other`, `change`, `custom`, `cashea`. |

### Emisión
| Método | Ruta | Para qué |
|---|---|---|
| POST | `/documents/createandapprove` | Emisión en línea. Devuelve el **strongId** del documento en `result`. |
| POST | `/documents/createretention` | Comprobantes de retención (IVA e ISLR). |
| POST | `/batch/open` · `/documents/create` · `/batch/close` · `/batch/approve` · `/batch/cancel` | Emisión por lotes. |

### Consulta (lo que cierra el ciclo asíncrono)
| Método | Ruta | Para qué |
|---|---|---|
| GET | `/documents?strongId=…` | **Número de control** y `status` (`Assigned` = ya es fiscal). |
| POST | `/documents/searchbysystemreference` | `{"SystemReference": "…"}` — **idempotencia con nuestro id**. |
| GET | `/documents/info/{guid}` | Documento completo. |
| POST | `/documents/list?page=1&thefrom=…&theTo=…` | Listado por rango de fechas. |

### Entrega al cliente
| Método | Ruta | Para qué |
|---|---|---|
| POST | `/documents/short/{guid}` | **Código corto único**, pensado para enlaces cortos (WhatsApp). |
| POST | `/documents/view/?documentStrongId=…` | URL de visualización. |
| POST | `/documents/view/{guid}` | Stream del PDF. |
| POST | `/documents/notified` | Reenviar el documento por correo. |

### Anulación
| Método | Ruta | Cuerpo |
|---|---|---|
| POST | `/documents/anulled` | `{"Control": <número de control>}` — se anula **por número de control**, no por id. |

---

## 4. Lo que manda el diseño del módulo

1. **La emisión es asíncrona.** `createandapprove` devuelve un `strongId`, no el número de control. Ese
   llega entre 1 y 5 minutos después, por consulta. El documento tiene que nacer **pendiente**.
2. **El correlativo `Number` lo llevamos nosotros**, ascendente estricto por serie + tipo.
   `GET /series/counters` es contra qué conciliarlo.
3. **`429` significa «tu petición anterior sigue en proceso»**, no rate limiting: un solo consumidor
   por serie, en cola.
4. **`useSystemReferenceAsKey: true`** en la serie: `SystemReference` es la clave de idempotencia, y
   ahí va nuestro id de documento.
5. **Hora de Venezuela (UTC-04:00) explícita** en `EmissionDateAndTime`.
6. **Los montos los calculamos nosotros** y ellos los validan; en decimal, nunca en float.

---

## 5. Lo que queda por preguntar

- Política exacta de **redondeo** (su ejemplo trae `IGTFAmount` con 4 decimales).
- Catálogo de **`OperationCode`** (`C001` en todos los ejemplos) y de **`ProductType`**.
- Ventana de tiempo para **anular**.
- Qué implica `calculatedByCompany: false` en la serie.
