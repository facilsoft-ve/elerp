# ElERP — ERP web para negocios venezolanos

ERP SaaS multi-tenant para el mercado venezolano, con motor de cumplimiento
fiscal parametrizable. Reimplementa el prototipo de diseño siguiendo el stack
del equipo (el mismo de Tesotrix): **backend Go/Fiber (hexagonal)**, **frontend
React/Vite/Tailwind**, **MongoDB**, **Hubmy** para auth (SSO) + IA, todo con
**Docker Compose**.

> Español es el locale base (producto, UI, dominio). Los documentos fuente
> (SAD, manual UX, flujos/permisos/seguridad) viven en `Documentos/`.

## Pruebas

**Backend (Go)** — Go no está instalado en el toolbox; las pruebas corren en contenedor:

```bash
# Opción A: build de pruebas (falla si go vet o alguna prueba falla)
docker build -f backend/Dockerfile.test -t huberp-test ./backend
# Opción B: suite completa con cobertura
docker run --rm -v "$PWD/backend":/src -v /root/go:/go -w /src golang:1.22-alpine go test ./... -cover
```

**Frontend (Vitest)**:

```bash
cd frontend && npm run test        # 115 pruebas de la lógica pura de src/lib
```

La suite (~257 funciones de prueba en Go + 115 en el frontend) cubre:

- **Motor fiscal**: IVA solo sobre la base gravable (con exentos), **IGTF 3%** sobre
  la porción pagada en divisas y **no** sobre el vuelto, numeración correlativa,
  anulación/NC/ND append-only, descuento de inventario, **venta fraccionaria por
  peso**, y que **la emisión exige un RIF/cédula válido del receptor**.
- **Documento fiscal (RIF/cédula)**: **dígito verificador del RIF (SENIAT módulo
  11)** validado en backend y frontend con el mismo algoritmo; cédula V/E, RIF J/G
  con verificador, pasaporte P.
- **Cobro y vuelto**: cobro **mixto** multi-cuenta/multi-moneda, **vuelto mixto**
  (varias monedas/medios; en divisa solo efectivo; reparto que cuadra), rechazo de
  cobro que no cubre el total.
- **Caja**: apertura (PIN, sede, caja tomada, retomar sin duplicar), **arqueo**,
  **autorización de supervisor** por PIN (auditada), aislamiento por tenant.
- **Tasa de cambio**: raspado estricto del BCV contra un fixture real, rechazo de
  cifras alteradas, cuarentena por variación anómala, degradado a la última tasa
  buena, una consulta al día, precio en US$ convertido con la tasa del servidor.
- **Compras / Ventas / Maestros**: factura de proveedor que difiere de lo recibido
  (cuadre contable), cotización→pedido→factura, notas de compra, dispositivos
  (balanza), clientes/proveedores/numeración, edición de empresa y tenancy.
- **Kardex / Contabilidad (dominio)**: costo promedio ponderado, `Asiento.Cuadra`,
  plan de cuentas; más cupón, promoción, tasa vigente, tema y roles.

## Estado (iteración actual)

**Fundación + verticales de Inventario y Fiscal/POS end-to-end.**

- ✅ Backend Go/Fiber hexagonal (dominio · aplicación · adaptadores) con
  selección de persistencia por config (`MONGO_URI` → Mongo, si no in-memory).
- ✅ Multi-tenancy `Organización → Empresa → Sede` con aislamiento por empresa
  **a nivel de query en Mongo** (Mongo no tiene RLS) + validación de membresía
  por petición (`X-Empresa-ID` / `X-Sede-ID`).
- ✅ Auth: modo demo (`DEV_LOGIN`), login nativo (email/contraseña, bcrypt),
  SSO Hubmy y aceptación de invitaciones; sesión por cookie opaca.
- ✅ RBAC server-side por rol (dueno · vendedor · cajero · contadora ·
  desarrollador): el módulo de inventario rechaza a Cajero y limita la
  escritura a Dueña/Desarrollador.
- ✅ **Inventario** como **ledger de movimientos append-only**: la existencia y
  el Kardex (saldo + costo promedio) son proyecciones derivadas, nunca
  contadores editables. Catálogo con presentaciones, ajustes auditados
  (requieren motivo) y transferencias entre sedes con máquina de estados.
- ✅ **Fiscal / POS**: emisión de factura por punto de venta con numeración
  fiscal serializada por empresa+sede (atómica), IVA 16% + **IGTF 3%** sobre la
  porción en divisas, cobro mixto multi-cuenta, documento **inmutable**;
  descuenta inventario vía el ledger. Nota de crédito / anulación por **reversa**
  (reingresa stock; el original queda `anulado` derivado, nunca se edita).
  Clientes (CRM mínimo, documento único V/E/J/G) y cuentas de cobro.
- ✅ **Tasa de cambio automática**: se obtiene del BCV (raspado con parser
  estricto), con API de respaldo y carga manual auditada como último recurso.
  Histórico de solo-anexado; las lecturas con variación anómala quedan en
  **cuarentena** y las aplica una persona. La tasa **no se teclea** en la
  operación y la interfaz siempre declara su origen y su fecha. `EmitirFactura`
  la toma del servidor y la copia al documento (Art. 177).
- ✅ **Moneda principal por empresa y precio homólogo calculado**: el precio se
  guarda una vez, en la moneda en que la empresa lo piensa; el equivalente se
  deriva con la tasa del día y nunca se guarda duplicado.
- ✅ **Cobro de mostrador completo**: modal de cobro con métodos en tarjetas,
  **monto recibido y vuelto en vivo**, cobro **mixto** multi-cuenta y
  multi-moneda, aviso de IGTF antes de cobrar y referencia del pago electrónico.
  El vuelto lo calcula y lo guarda el servidor (arqueo del turno).
- ✅ **IVA exento** por producto: la factura separa base imponible de base exenta
  (cesta básica venezolana), y el IVA sale solo de la primera.
- ✅ **Modo caja a pantalla completa** para el cajero: sin sidebar ni topbar,
  conmutadores diestro/zurdo y claro/oscuro **por dispositivo**, teclado numérico
  de **PIN de supervisor** para quitar líneas o salir (validado en el servidor y
  auditado). El cajero entra directo a su caja, en la **sede que tiene asignada**.
- ✅ **Configuración**: usuarios y roles con sede, cajas y sesiones (auditoría del
  arqueo), moneda y tasa, dispositivos e integraciones con su estado real.
- ✅ **Código de barras por producto** (R11): único por empresa, resuelto también
  desde las presentaciones; en el POS y el Modo caja, Enter prueba primero el
  código exacto, que es lo que manda la pistola lectora. Imagen del producto
  subible desde su ficha.
- ✅ **Ventas en espera** (2.6): el carrito apartado vive en el servidor, lo puede
  retomar otro cajero de la sede y retomarlo lo consume (nadie lo cobra dos veces).
- ✅ **Tesorería**: venta **a crédito**, cuentas por cobrar con saldo **derivado** y
  antigüedad, ledger de cobros append-only (se corrige con reversos), efectivo y
  bancos con el vuelto restado, y reporte de IGTF derivado de los documentos.
- ✅ **Contabilidad**: plan de cuentas, **libro diario de solo-anexado** con los
  asientos **derivados** de las operaciones (venta, costo de ventas, anulación,
  cobro), balance de comprobación y estado de resultados como proyecciones. Todo
  asiento cuadra por construcción; se corrige con su contrario, nunca se edita.
- ✅ **Venta por peso + balanza**: productos con tipo de venta *unidad* o *peso (kg)*
  (precio Bs/kg); en el POS se abre el modal de pesaje con lectura de **balanza**
  (dispositivo con puerto/protocolo) o entrada manual. La balanza se administra
  junto a las impresoras fiscales.
- ✅ **Vuelto y cobro mixto asistidos**: el vuelto se reparte en varias monedas/medios
  (en divisa solo efectivo) y autosugiere el restante; el cobro mixto sugiere el
  saldo al agregar cada pago y solo deja agregar si falta por cubrir.
- ✅ **Precios comerciales**: listas de precio (venta/compra), cupones (porcentual o
  monto) y promociones (slider de la pantalla del cliente), aplicables en POS y en
  cotizaciones sin tocar el motor fiscal.
- ✅ **Compras y proveedores**: proveedores, solicitudes de presupuesto (RFQ),
  órdenes de compra con impuestos, recepción (mueve inventario) y **factura de
  proveedor flexible** (líneas propias que pueden diferir de lo recibido; la
  diferencia va a la cuenta 5202 y el Kardex no se re-valúa), NC/ND de proveedor y
  **cuentas por pagar**.
- ✅ **Ventas forma libre**: cotización (lista de precio, dirección de entrega,
  almacén de despacho, líneas con impuesto) → confirmación/pedido → cobro completo →
  factura, con comprobante **PDF** previsualizable/enviable.
- ✅ **Documento fiscal (RIF/cédula)**: validación del **dígito verificador del RIF**
  (SENIAT módulo 11) en backend y frontend; la emisión exige RIF válido del receptor
  (el consumidor final no lleva RIF). Tipos V/E (cédula), J/G (RIF), P (pasaporte).
- ✅ **Pantalla del cliente y marketing**: segunda ventana orientada al cliente con
  3 modos (solo productos / mixta / publicidad), slider de publicidad, editor de
  tema (fondos, colores, versión de logo) a nivel empresa con override de contraste
  por caja, e IGTF visible al pagar en divisas.
- ✅ **UX**: command palette (Ctrl/⌘+K) sobre todos los módulos/submódulos, navegación
  móvil, tablas manipulables (orden/densidad/export CSV) y accesibilidad de diálogos
  (foco atrapado, `aria-modal`). El PIN de supervisor acepta teclado físico y táctil.
  Códigos: cajas con prefijo **C-**, cajeros con **OP-** (para no confundirlos).
- ✅ **Asistente IA** (módulo instalable `asistente-ia`): botón flotante que responde
  en dos capas — **mecánica** (100% local: ventas, cartera, stock, por pagar,
  integridad del libro, acotado al rol) y una **capa de IA opt-in** (vía Hubmy, solo
  para preguntas abiertas, con contexto acotado al rol y sin acceso a internet). Las
  cifras salen siempre del ledger, nunca del modelo. Ver `Documentos/asistente-ia.md`.
- ✅ Auditoría append-only de acciones sensibles.
- ✅ Frontend con tema ElERP (navy/teal, Poppins/Inter/IBM Plex Mono), chrome
  (sidebar/topbar con selector Org▸Empresa▸Sede), Login/Onboarding/Dashboard,
  Inventario (Catálogo/Existencias/Kardex/Transferencias), **Fiscal (POS +
  Documentos)**, Clientes, command palette (Ctrl/⌘+K), y placeholders para el resto.
- ⏳ **Diferido** (requiere hardware / trámite / decisión): **homologación SENIAT**
  —integración real de impresora fiscal vía agente local, número de control,
  exportaciones oficiales TXT/XML de libros y sello de integridad diario
  (OpenTimestamps)— y **Cierre Z** atado al turno (se prueban con la impresora);
  enlaces de pago y ejecución real del pago móvil (pasarelas), envío real de
  PDF/correo/WhatsApp, portal público de verificación, endurecimiento de
  `DEV_LOGIN`, RRHH/Nómina, Reportes/BI, Super Admin, conciliación bancaria, MFA,
  offline-first real y motor de reglas fiscales versionadas.

## Cómo correr

### Docker Compose (todo junto)

```sh
cp .env.example .env          # opcional: completá HUBMY_* para login SSO real
docker compose up --build
# → Frontend:  http://localhost:3000   (nginx proxya /api y /auth al backend)
```

Entrá y usá **"Entrar en modo demo"** (sin Hubmy): inicia sesión como la Dueña
de la empresa demo *Bodega La Cima* con catálogo y existencias sembradas.

### Desarrollo (hot reload)

```sh
# Backend (:8080). Sin MONGO_URI usa datos in-memory efímeros.
cd backend && cp .env.example .env && go run ./cmd/api

# Frontend (:5173, proxya /api y /auth al :8080)
cd frontend && npm install && npm run dev
```

## Verificación

- Backend: `cd backend && go build ./... && go vet ./...` (o `docker build ./backend`).
- Frontend: `cd frontend && npm run build`.
- Smoke test de la API (con el stack arriba): dev-login → `/api/me` →
  `/api/bootstrap` → `/api/inventario/*` con cabeceras `X-Empresa-ID` +
  `X-Sede-ID`.

## Arquitectura del backend

```
backend/
  cmd/api/main.go                     # composición: elige adaptador de persistencia
  internal/
    config/                           # env → Config
    domain/                           # entidades + puertos (interfaces), sin deps de adaptadores
      authn organizacion empresa sede usuario credencial auditoria inventario
      fiscal cliente caja tasa venta tesoreria contabilidad
    application/                      # casos de uso (Service, TenancyService)
    adapter/
      inmem/    # repos en memoria + seed demo (fuente de siembra de Mongo)
      mongo/    # repos sobre Mongo con filtro obligatorio por empresaid + sesiones
      hubmy/    # SSO validate + AI proxy
      tasafuente/ # tasa Bs/US$: raspado del BCV + API de respaldo (solo saliente)
      session/  # store de sesiones en memoria
      httpapi/  # Fiber: server + middleware (auth, contexto de tenant) + handlers
```

`domain` define interfaces; `application` orquesta; `adapter` implementa o
expone. El mismo código de servicio corre sobre in-memory o MongoDB según
`MONGO_URI` — solo cambia el adaptador cableado en `cmd/api`.

## Contrato REST (implementado)

| Método | Ruta | Descripción |
|---|---|---|
| GET | `/api/health` | Estado (ok, hubmy, devLogin) |
| GET | `/api/auth/login` · `/api/auth/dev-login` | SSO Hubmy · sesión demo |
| GET | `/auth/hubmy/callback` | Callback SSO |
| POST | `/api/auth/native-login` · `/api/auth/accept-invite` | Login local · aceptar invitación |
| POST | `/api/auth/logout` | Cerrar sesión |
| GET | `/api/me` | Organizaciones→empresas→sedes del usuario |
| POST | `/api/empresas` · `/api/empresas/:id/onboarding` · `/api/empresas/:id/sedes` | Onboarding |
| GET | `/api/bootstrap` | Contexto del tenant (empresa, sedes, roles, rubros) |
| GET/POST | `/api/inventario/productos` | Catálogo |
| POST | `/api/inventario/productos/:id/presentaciones` | Presentaciones de venta |
| GET | `/api/inventario/existencias?sede=` | Existencias por sede (proyección) |
| POST | `/api/inventario/existencias/:sku/ajustar` | Ajuste auditado (requiere motivo) |
| GET | `/api/inventario/kardex/:sku?sede=` | Kardex con saldo y costo corridos |
| GET/POST | `/api/inventario/transferencias` | Transferencias entre sedes |
| PATCH | `/api/inventario/transferencias/:id/estado` | Avanzar máquina de estados |
| GET/POST | `/api/fiscal/documentos` | Documentos fiscales · emitir factura (POS) |
| GET | `/api/fiscal/documentos/:id` | Detalle de documento |
| POST | `/api/fiscal/documentos/:id/anular` | Anular por reversa (dueña/desarrollador) |
| GET/POST | `/api/crm/clientes` | Clientes (documento único por empresa) |
| GET | `/api/tasa` · `/api/tasa/historial` | Tasa vigente con origen y fecha · histórico |
| POST | `/api/tasa/manual` · `/api/tasa/sincronizar` | Carga manual auditada · forzar consulta (429 si es pronto) |
| POST | `/api/tasa/cuarentena/:id/aprobar` | Aplicar una lectura rechazada por variación anómala |
| PUT | `/api/empresa/config/moneda` · `/api/empresa/config/seguridad` | Moneda (R10) · PIN de supervisor de caja |
| GET/PATCH | `/api/usuarios` · `/api/usuarios/:id` | Miembros con rol y sede · cambiar rol/sede (el cajero exige sede) |
| GET | `/api/cajas/sesiones` | Historial de turnos (auditoría del arqueo) |
| POST | `/api/cajas/autorizar` | Autorización de supervisor por PIN (auditada) |
| PATCH | `/api/inventario/productos/:sku` | Editar catálogo (precio, IVA, código de barras) |
| GET | `/api/inventario/buscar?codigo=` | Resolver un escaneo de la pistola lectora |
| GET/POST | `/api/pos/ventas-en-espera` · `/:id/retomar` · `DELETE /:id` | Carrito apartado (2.6) |
| GET | `/api/tesoreria/por-cobrar` · `/saldos` · `/igtf` · `/cobros` | Proyecciones de Tesorería |
| GET | `/api/contabilidad/plan` · `/diario` · `/balance` · `/resultados` | Plan de cuentas y libro diario |
| POST | `/api/contabilidad/diario/:id/revertir` | Asiento contrario (el original queda intacto) |
| POST | `/api/tesoreria/cobros` · `/cobros/:id/reversar` | Registrar un cobro · corregirlo por reverso |
| GET/POST | `/api/tesoreria/cuentas-cobro` | Cuentas de cobro de la empresa |

Rutas de datos exigen cabeceras `X-Empresa-ID` (tenant) y `X-Sede-ID`.

> La tabla lista el núcleo; el contrato creció con módulos posteriores —compras
> (`/api/compras/*`), cotizaciones/ventas, listas de precio, cupones, promociones,
> métodos de pago, dispositivos (impresora y balanza), marketing/pantalla del
> cliente, notas de crédito/débito y libros fiscales. La fuente autoritativa del
> contrato es `Documentos/flujos-permisos-seguridad-backend.md` + los handlers en
> `internal/adapter/httpapi/`.
