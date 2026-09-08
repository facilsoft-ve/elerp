# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repository currently is

ElERP is now an **implemented app in progress**, built on the design/architecture baseline. Current state: **foundation + the Inventario and Fiscal/POS verticals end-to-end** (see `README.md`).

- `backend/` — Go 1.22 + Fiber v2, hexagonal. Module `github.com/mornix/huberp`. Domain (`authn organizacion empresa sede usuario credencial auditoria inventario fiscal cliente`) + application (`Service`, `TenancyService`) + adapters (`inmem`, `mongo`, `hubmy`, `session`, `httpapi`). Entry `cmd/api`.
- `frontend/` — React 18 + Vite 5 + Tailwind v3. Contexts (Auth/Data/UI), chrome, Inventario + Fiscal/POS + Clientes screens + placeholders. State-based routing, manual PWA.
- `docker-compose.yml` — mongo + backend + frontend (nginx proxies `/api`,`/auth`).
- `Documentos/` — source-of-truth docs: brand manual, UI/branding, the flows/permissions/security PDF, and **`flujos-permisos-seguridad-backend.md`** (the extracted backend spec: roles, tenancy, REST contract — authoritative).
- `Logos/` — SVG logos + Affinity source. `Diseño prototipo ElERP.zip` — the clickable design-tool prototype (bundled export; visual reference only).

### Build, run & verify

- Run everything: `docker compose up --build` → `http://localhost:3000` (use **"Entrar en modo demo"**). Backend build/vet: `cd backend && go build ./... && go vet ./...` (Go isn't installed in the default toolbox — use `docker build ./backend` to compile-check). Frontend: `cd frontend && npm run build`. Dev hot-reload: backend `go run ./cmd/api` (:8080), frontend `npm run dev` (:5173, proxies to :8080).
- Tests: backend `go test ./...` (dominio + application; ~257 funciones) — corre en contenedor (`docker run --rm -v "$PWD/backend":/src -v /root/go:/go -w /src golang:1.22-alpine go test ./...`) o `docker build -f backend/Dockerfile.test ./backend`; frontend **Vitest** (`cd frontend && npm run test`, lógica pura de `src/lib`). Data routes require `X-Empresa-ID` + `X-Sede-ID` headers.

### Core implemented pattern (reuse for other modules)

Inventory is an **append-only movement ledger** (`domain/inventario`): `Movimiento` records are insert-only; `Existencia` and the `Kardex` (running balance + weighted-average cost) are **projections folded from movements in the application layer**, never editable counters. Adjustments and transfers emit new movements (audited), never overwrite. Fiscal documents (`domain/fiscal`) follow the same rule: a `Documento` is immutable (Append-only, no Update/Delete); anulación/nota de crédito create a **reversal document** referencing the original, and `anulado` is derived (a reversal exists). Emitting a factura decrements stock by appending `salida` movements to the same inventory ledger; anulación re-appends `entrada`. Fiscal numbering is atomic per empresa+sede+serie (`Numerador`). This is the template for Contabilidad's libro diario.

**Fiber gotcha (load-bearing):** the app runs with `Immutable: true` in `fiber.New`. Strings from `c.Get`/`c.Params`/`c.Query`/`c.Cookies` are backed by a reusable request buffer; we persist `empresaId`/`sedeId` (from headers) into the store, so without `Immutable` the buffer is reused on the next request and **corrupts stored data** (observed: `sede_demo_1` → `emp_demoo_1`). Keep it on; never store non-immutable ctx strings.

### Multi-tenancy (as implemented)

`Organización → Empresa (tenant) → Sede`. Every business entity carries `empresaId`; the Mongo adapter enforces a **mandatory `empresaid` filter on every query** (stricter than Tesotrix, which filters mainly in the app layer — Mongo has no RLS). `httpapi` resolves the tenant per request in `empresaContext` middleware (validates membership via `TenancyService.RolEnEmpresa`), after `requireAuth`.

### Inspecting the prototype and docs

`unzip` is not installed; use Python. The zip contains, under `uploads/`:
- `arquitectura_extracted.txt` — full **Software Architecture Document (SAD) v1.0**. This is the primary spec.
- `Manual_UX_ERP_Reglas_Fundamentales.md` — binding UX rules for every screen.
- `ElERP_Documento_de_Arquitectura_v1.0.docx` — the SAD as Word.

```bash
python3 -c "import zipfile; z=zipfile.ZipFile('Diseño prototipo ElERP.zip'); print(z.read('uploads/arquitectura_extracted.txt').decode())"
```

The prototype itself (`ElERP Prototipo.dc.html`) is an export from a design tool: markup uses custom `<sc-if>`/`<x-dc>` tags with `{{ }}` bindings driven by `support.js`/`helpers.js` — it is a design reference, **not** the app codebase and should not be treated as production HTML.

## Product context (read before designing any feature)

ElERP is a **web-native, multi-tenant SaaS ERP for the Venezuelan market**, built to match/exceed the fiscal-compliance benchmark set by "CACHICAMO" while adding a native double-entry accounting core, treasury, HR/payroll, CRM, and BI. **Everything is in Spanish** (product, UI, docs, domain events) — keep it that way; español is the base locale.

The operating environment assumes **hostile connectivity** (power/internet outages are normal), **constantly-changing tax law** (SENIAT issued 3+ providencias in 2024–2025), and **SENIAT homologation as a product requirement** (integrity, traceability, immutability must be demonstrable).

## Architectural principles (conflicts resolve in favor of the lower number)

1. **Compliance as configuration** — tax rules (rates, thresholds, required fields, export formats) live in a versioned, auditable rules engine, never in code. A new providencia ships in days without redeploying the core.
2. **Integrity & immutability first** — accounting ledger and fiscal documents are **append-only**; corrections are reversal documents, never edits. Daily cryptographic seal on the Bitcoin blockchain (OpenTimestamps).
3. **Zero Trust security** — authn/authz at every layer; least privilege; strict tenant isolation. (The SAD assumed PostgreSQL Row-Level Security as the last-line enforcement; on MongoDB this is enforced in the repository layer — a mandatory `tenant_id` scope on every query — see the tech-stack note below.)
4. **One canonical data model** — sales, purchases, inventory, finance, and people share one model; accounting entries and KPIs are **derived** from operations, not reconciled manually.
5. **Modular monolith, microservices only when justified** — strict bounded contexts communicating solely via versioned contracts (internal API + domain events). No module reads another's tables.
6. **Offline-first is real** — the PWA operates offline with a local sync queue and pre-assigned contingency numbering (Providencia 000102).
7. **API-first & events-first** — every capability is an internal API + domain event first; the UI is just another consumer.
8. **Progressive simplicity on the surface** — fiscal complexity lives in the engine, not the screen (safe defaults, plain language, progressive disclosure). Governs all UI/UX work.
9. **Scale without artificial walls** — the same binary serves the solo entrepreneur and the multi-branch enterprise; plan limits are config, never schema constraints.
10. **Total observability & auditability** — every state change emits a traceable event (actor, tenant, source, UTC time).

## Reference architecture (six layers)

1. **Experience** — one responsive PWA (desktop + mobile), installable, full offline mode. Two satellites: the **local fiscal agent** (lightweight bridge to fiscal printers on Windows/Linux/Android) and a **public verification portal** (no-auth, verify a document by hash/QR against the day's blockchain seal).
2. **Access** — API Gateway (authn, per-plan rate limits, versioning `/v1`, access audit) exposing the public REST API, a BFF for the PWA, outbound webhooks (HMAC-signed, retried), and a realtime event channel (SSE/WebSocket).
3. **Business modules** — eight bounded contexts inside the modular monolith (see below).
4. **Fiscal compliance engine** — a cross-cutting layer no module may bypass; all tax/document/report calculation and validation happens here against rules versioned per providencia.
5. **Common platform** — identity & access, multi-tenancy, offline sync queue, immutable audit + blockchain seal, AI services, notifications.
6. **Data & infra** — MongoDB (persistence; tenant isolation enforced in the repository layer), plus (per SAD, add as needed) a read path for BI, Redis, event broker, S3-compatible object storage, Docker Compose → container platform, CI/CD + observability. See the tech-stack section for the concrete Go/Fiber + Mongo + React/Vite baseline.

### The eight business modules

Fiscal & Invoicing · Accounting & Finance · Inventory & Operations · Purchasing & Suppliers · Sales & CRM · Treasury & Payments · HR & Payroll · Reports & BI. Each owns its schema; integrate only via internal API (sync queries) or domain events (async facts). Contracts are versioned and verified in CI — breaking a contract breaks the build, not production.

### Load-bearing design decisions (key ADRs)

- **Append-only ledgers** for accounting, inventory, and audit. Current balance / stock is a **materialized projection**, never an editable counter. This gives a perfect Kardex and audit trail by construction.
- **Rules as versioned data** with temporal validity (valid-from/valid-to). Historical calculations always use the rule in force at the taxable event date, never today's.
- **Transactional outbox + at-least-once event broker** (NATS JetStream or RabbitMQ) — an event publishes iff its originating transaction committed. No phantom sales, no orphan entries.
- **CQRS at the infrastructure level** — BI and the AI assistant read only the analytical replica, never competing with invoicing.
- **UUIDv7 generated client-side** — required for offline-first; a document has a unique identity before it reaches the server.
- **Multi-currency**: always store amount + historical rate of the event date; never store only the converted value (Art. 177 requirement).
- **RBAC + ABAC** authorization evaluated server-side on every request. Roles per empresa (from `Documentos/flujos-permisos-seguridad-backend.md`, which supersedes the SAD's role list): **`dueno`** (Dueña/Admin), **`vendedor`** (one fixed sede), **`cajero`** (POS only), **`contadora`** (full Accounting/Treasury, read-only elsewhere), **`desarrollador`** (owner + dev mode); plus platform **`super_admin`** (Mornix, `/admin` only). Attributes: sede, amount, hours. The UI only hides; it never protects. Contadora is read-only outside Contabilidad/Tesorería (also serves the regulator's consultation access, Providencia 000121).

### Tech stack (the team's real stack — mirrors Tesotrix, `/root/tesotrix`)

> This **supersedes** the SAD's language-agnostic recommendation (which suggested NestJS + PostgreSQL). ElERP reuses the same stack and patterns the team already ships with in the Tesotrix treasury app. Read that repo's `CLAUDE.md`, `README.md`, and `backend/`/`frontend/` layout as the concrete reference implementation.

- **Backend — Go 1.22 + Fiber v2, hexagonal / DDD.** Layout:
  - `internal/domain/<aggregate>/` — entities + repository **ports** (interfaces). Rich aggregates graduate out of a catch-all `reference` package over time (in Tesotrix: `account`, `movement`, `payment`, `organization`, `membership`, `authn`, …). For ElERP the aggregates map to the eight modules' entities (documento fiscal, asiento, movimiento de inventario, tercero, etc.).
  - `internal/application/` — use-case services, one file per area; wired to ports, not adapters.
  - `internal/adapter/` — swappable behind the ports: `inmem` (seed/dev, same shape as the frontend's `db` object), `mongo` (production persistence), `hubmy` (auth + AI), `session` (session store), `httpapi` (Fiber routes/middleware). Entry point `cmd/api/main.go` picks the persistence adapter by config (`MONGO_URI` set → Mongo, else in-memory) — **same service code, different adapter**.
- **Database — MongoDB** (team decision for ElERP). Note this diverges from the SAD's PostgreSQL/RLS baseline: since Mongo has no Row-Level Security, **tenant isolation (Principle 3) and append-only fiscal integrity (Principle 2) must be enforced in the application/domain layer** — a mandatory non-null `tenant_id` filter on every repository query, and append-only collections with reversal documents rather than in-place edits. Treat this as a first-class design constraint when building each aggregate.
- **Frontend — React 18 + Vite 5 + Tailwind v3.** `context/` (Auth/Data/UI; `DataContext` exposes a `db` object mirroring the backend), `components/` (chrome + primitives + charts), `screens/` (one per screen — see the prototype's screen inventory below), `lib/` (client-side domain logic + `api.js`). State-based `route` pattern gated by `AuthContext` (no react-router). PWA is **manual** (level 1: installable + app-shell offline) via `public/manifest.webmanifest` + `public/sw.js`, no plugin. Tailwind theme lives in `tailwind.config.js` — port ElERP's palette/fonts there (see Design System below).
- **Auth & AI — Hubmy.** SSO: `/api/auth/login` → Hubmy `/authorize` → `/auth/hubmy/callback` validates and sets an opaque session cookie. `DEV_LOGIN=true` gives a demo session without Hubmy creds (**must be `false` in prod**). AI via Hubmy AI Proxy (`POST /api/ai/ask`) — this is how the SAD's "native AI service with swappable provider" is realized. Cookies: dev shares origin via the Vite proxy (`/api`,`/auth` → `127.0.0.1:8080`, IPv4 on purpose) so `SameSite=Lax` works; prod (separate domains) needs `COOKIE_SECURE=true` → `SameSite=None; Secure`.
- **Infra / deploy — Docker Compose** (mongo + backend + frontend/nginx that proxies `/api` and `/auth`). Frontend also publishable to Vercel Managed via the Hubmy MCP. Fly/Railway/Render configs exist in Tesotrix as references.

**Verify commands:** backend `cd backend && go build ./... && go vet ./...` (compile-check con `docker build ./backend`); frontend `cd frontend && npm run build`. Tests: backend `go test ./...` (en contenedor golang:1.22-alpine, o `docker build -f backend/Dockerfile.test ./backend`) y frontend `npm run test` (Vitest). Patrones a mantener: la validación de documento (RIF con **dígito verificador SENIAT módulo 11** en `domain/fiscal/documento.go` y su espejo en `frontend/src/lib/format.js`), la **venta por peso** (`Producto.TipoVenta` unidad/peso; balanza como `DispositivoFiscal` tipo `balanza`), y los prefijos de código **C-** (cajas) vs **OP-** (cajeros). La documentación de usuario vive en `site/docs/` (portal + páginas HTML, nav en `site/assets/docs.js`); la landing pública en `site/`. El local fiscal agent (SAD §9.3) sigue siendo un binario aparte, firmado y multiplataforma (Go), sobre un canal solo-saliente.

## Design system (extracted from the prototype)

**Fonts:** Inter (body/UI), IBM Plex Mono (numeric/code), Poppins (headings). *(The newer "para Backend" prototype uses Inter for body; the first prototype used IBM Plex Sans. The frontend `tailwind.config.js` ships Inter + Poppins + IBM Plex Mono.)*

**Core palette:** (Tailwind ramps `huberp` = navy, `teal` = accent)
- Primary brand blue `#1D3477`, hover/dark `#152C61`, tint `#EDF2F9`; mid navies `#2A4A8F`/`#243F79`
- **Accent teal `#09B69B`** (confirmed in the newer prototype + design docs)
- Text `#1F2430`, muted `#5C6470`, faint `#8B94A3`
- Backgrounds `#FAFBFC` (app), `#F5F7FA`, surface `#FFFFFF`; borders `#E8EAEF`, `#D7DBD8`
- Semantic: success green `#166B41`, error/danger red `#B3362C`, warning amber `#92600A`; neutral-green surfaces `#F0F2EF` / `#F3F4F2`

**Screen inventory** the prototype already lays out (use as the module/route map): Autenticación, Onboarding, Dashboard, Fiscal y Facturación (Punto de venta, Documentos fiscales, Retenciones, Cierres Z, Libros fiscales), Ventas y CRM, Contabilidad (Plan de cuentas, Libro diario, Estados financieros), Tesorería (CxC, CxP, Reporte IGTF, Enlaces de pago), Inventario (Catálogo, Existencias, Transferencias), Compras, RRHH y Nómina, Reportes y BI, Configuración (Usuarios y roles, Dispositivos fiscales, Integraciones, Marketplace de módulos), Super Admin, Asistente IA, plus a "Sistema de diseño" reference screen.

## Binding UX rules (from `Manual_UX_ERP_Reglas_Fundamentales.md`)

When building any screen, these are requirements, not suggestions:
- Navigate by **task/process, not database module**; max 3 levels deep; frequent actions in 1–2 clicks.
- **Global command palette** search (Ctrl/⌘+K) is the primary navigation.
- Home screen is a **workspace** (pending tasks, role KPIs, shortcuts, alerts), not a static module menu.
- **Progressive disclosure** — show essential fields by default; smart defaults + fuzzy autocomplete everywhere.
- **Inline, immediate, constructive validation** — never validate only on submit.
- High-density but manipulable **data tables** (sort/filter/pin/choose columns, saved views, inline edit, bulk actions, export, comfortable/compact density).
- Design **empty / loading (skeletons) / no-results / error** states intentionally.
- Optimize for the **keyboard-driven expert** without punishing novices; invoicing a repeat item must complete in <10s without the mouse.
- **Autosave / recoverable work**; minimize context switches (side panels, quick-create); design for interruption.
- **WCAG 2.2 AA** and i18n from day one; ≥4.5:1 contrast; the AI assistant is a first-class citizen reachable from any screen.
