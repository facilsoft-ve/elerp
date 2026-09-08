# ESTADO — punto de retome de ElERP

> Documento de handoff. Está escrito para alguien que no vio ninguna conversación previa.
> Sin credenciales ni secretos: donde hacen falta, se indica el archivo que las contiene.

---

## 1 · OBJETIVO

Construir **ElERP**, un ERP SaaS multi-tenant para el mercado venezolano (IVA 16%, IGTF 3%, retenciones, libros fiscales SENIAT, doble moneda Bs/US$ a tasa BCV, modo contingencia sin internet). El diseño funcional ya está validado y entregado como prototipo; el trabajo actual es **acoplar la aplicación real a ese diseño y completar el backend que falta**, módulo por módulo.

Jerarquía de cuentas: **Organización → Empresa (RIF propio) → Sede → Caja**. El tenant es la Empresa.

---

## 2 · ACLARACIONES DEL USUARIO

Reglas y decisiones ya acordadas. **No volver a proponerlas ni a discutirlas.**

### Gobernanza de la documentación

- **El prototipo interactivo es el árbitro final** de toda duda visual o funcional. Palabras del usuario: «en todo lo visual y en las dudas funcionales a nivel de interfaz recurrir a las vistas de prototipo que te pasé, eso es importantísimo». Está en `/root/dashboard-uploads/1785476436393-Dise__o_prototipo_ElERP.zip`, carpeta `entrega/`. Se abre con Playwright; login: cualquier correo con la contraseña de demostración indicada en `entrega/LÉEME.md`, MFA cualquier 6 dígitos. El botón «Demo» abajo a la izquierda cambia de rol y salta a Modo caja, Sistema de diseño, Onboarding y Super Admin.
- **Orden de autoridad cuando los documentos se contradicen:** prototipo → `03 - Especificación de Vistas` → `02 - Flujos, Permisos y Seguridad` → PDF de marca. Las 12 contradicciones ya resueltas están en `Documentos/sistema-visual.md`; **no re-litigarlas**.
- **Cuando el usuario sube archivos, revisar la carpeta completa** (`/root/dashboard-uploads/`), no solo la ruta que menciona. Motivo: subió dos archivos y solo se abrió uno, y se trabajó sobre la referencia equivocada durante horas.

### Idioma y voz

- **Español de Venezuela, trato de «tú»**, nunca voseo. Motivo: lo fija `03 §1.6`. Ya se corrigieron 20 formas de voseo en 8 pantallas («Cargá»→«Carga», «podés»→«puedes»). Si aparece voseo nuevo, es un error.
- Los botones nombran la acción concreta con su monto («Cobrar Bs 3.480,00»), no verbos genéricos.
- Los términos fiscales se explican en lenguaje llano la primera vez (prorrateo, IGTF, contingencia).

### Diseño visual

- **El menú lateral va BLANCO** con los iconos de módulo, no azul. Motivo: así lo dibuja el prototipo. El PDF de branding dice lo contrario («sidebar azul») y **está descartado** en este punto.
- **El verde `#09B69B` es el color del DINERO**, exclusivo de Cobrar / Registrar pago / Liquidar / Vender ahora. **Crear algo nuevo es azul.** Motivo: lo define la pantalla `/design-system` del prototipo. El PDF de branding lo llama «CTA / crear» y **está descartado**.
- La marca se escribe **«ElERP»** en mayúsculas dentro de la interfaz (así lo hace el prototipo), aunque el Manual de marca dibuje el logotipo como «ElERP».
- **Cifras destacadas y KPI en Poppins; cifras de tabla, RIF, SKU y hashes en IBM Plex Mono tabular.** Decisión explícita del usuario.
- Tipografía de interfaz: **IBM Plex Sans** (verificado en el navegador contra el prototipo). Fondo de app **#FAFBFC**. `03 §1.3` dice «Inter» y `#F5F7FA`: **descartado**, manda el prototipo.
- **Los gráficos complementarios (racimo modular) van SOLO en superficies de marca** — login, onboarding, paneles de bienvenida, cabeceras de Super Admin. **Nunca sobre áreas de trabajo con datos.** Racimos de 3 a 4 piezas con un único punto verde.
- **El ámbar `#92600A` es color semántico oficial** de advertencia y contingencia. El PDF de branding dice «rojo solo para errores» y no contempla advertencias: descartado.
- **Nada de datos sintéticos en pantalla.** Si un dato no existe, va un estado vacío que dice qué falta y por qué. Motivo: el propio Manual UX cita a Few y Tufte — «evitar decoración que no informa». Ya se eliminó un gráfico con serie inventada.

### Producto y flujos

- **La tasa de cambio NO se teclea.** Debe entrar automáticamente desde una fuente oficial y alimentar el cálculo de la caja; el campo manual desaparece. El usuario fue explícito: «hay una parte donde tú pones la tasa de cambio o la tipeas, y eso no debería estar».
- **El BCV no publica API.** Acordado: raspar `bcv.org.ve` como fuente primaria, una API de terceros como respaldo, y carga manual auditada como último recurso. **Aprobado por el usuario con una condición literal: «siempre y cuando no viole la seguridad de la app»** — las nueve condiciones de seguridad están en `Documentos/backlog-ux.md` → R9 y son obligatorias.
- **Multimoneda:** la empresa elige su **moneda principal**. Los precios se capturan en esa moneda y el homólogo se deriva con la tasa (× si la principal es US$, ÷ si es Bs). **El homólogo se calcula, no se guarda duplicado** — si se guardan los dos precios quedan desincronizados al cambiar la tasa.
- **Cajas:** el administrador crea cajas en cada sede. El cajero elige una caja y se identifica con **código + PIN**. Debe ser **configurable**. Sin caja abierta no se factura, y lo rechaza el backend.
- **Modo caja ampliado:** sin barra lateral, sin el título del módulo, más espacio, **tarjetas de producto más grandes y área del carrito más grande** para la practicidad de un punto de venta.
- **Accesibilidad de caja: conmutador diestro/zurdo** que intercambia el lado del carrito y el de los productos, según la comodidad del cajero. Más modo oscuro accesible desde dentro de la caja. Ambas preferencias **por dispositivo**, no por usuario: la caja es un puesto físico compartido entre turnos.
- **Imágenes de producto: bucket por cliente, alojado en el servidor.** Si un producto no tiene imagen, mostrar un **marcador explícito de «sin imagen asignada»** — nunca un hueco silencioso.
- **Botón de información en la esquina de cada tarjeta** de producto → abre la **disponibilidad por sede** (cuántas unidades hay y en qué tienda, incluidas las demás tiendas de la cadena). **El cajero sí puede consultarlo**: es el caso de uso pedido.
- **La barra de búsqueda se mantiene** porque es la entrada de la **pistola lectora de código de barras**, de uso intensivo en Venezuela. Cada producto debe poder llevar su **código de barras o QR**.
- **Al lado del buscador va una «búsqueda por catálogo»** que abre el panel visual de tarjetas con imágenes. Dos caminos al mismo producto: teclado/pistola para quien sabe qué busca, panel visual para quien necesita ver.
- **En toda vista de producto: el nombre bien visible y el código en pequeño**, para confirmar que es el producto correcto antes de cobrarlo.
- **Giro «Restaurante»:** se agrega como opción del wizard **ahora**; mesas, comandas, envío a cocina, división de cuenta y propina quedan como **módulo aparte a planificar** — es un modo de operación completo, no una variante de configuración.
- **Mapa de tiendas:** Leaflet + tiles de OpenStreetMap. Comportamiento offline aclarado por el usuario: **detección dinámica de conectividad**. Con red, mapa con pines. Sin red, el bloque del mapa se anuncia como **no disponible — nunca un error** — y el resto del módulo sigue plenamente disponible (listado, registro de tienda nueva, detalles).
- **Modo demo precargado con data de toda la gama de posibilidades**, para poder probar todos los casos.

### Proceso y seguridad

- **Revisión vista por vista.** El usuario pidió recorrer las vistas de a una y que se le pidieran una por una; luego cambió a «sigue y termina de ajustar todas las vistas, yo reviso cuando estén todas ajustadas».
- **`DEV_LOGIN=true` queda encendido por ahora**, por decisión consciente del usuario para revisar cómodo, **aunque expone sesión de Dueña/Admin sin contraseña en el dominio público**. Está anotado como riesgo S1 en `Documentos/seguridad-pendientes.md` y debe apagarse antes de producción.
- **La auditoría de ciberseguridad se hace al final**, cuando estén todos los módulos. Pero mientras tanto **hay que ser precavido**: por eso se lleva un registro corriente en `Documentos/seguridad-pendientes.md` en lugar de acumular sorpresas.
- **Tesorería va de último** entre los módulos priorizados por el usuario.
- Hay que **terminar todo el backend que falta**, no solo la interfaz.
- El usuario quiere **pruebas unitarias** además de los ajustes.

---

## 3 · ULTIMAS INSTRUCCIONES

Lo último que pidió el usuario, textualmente resumido:

1. **Corregir los precios** (los del catálogo demo se ven como bolívares pero son valores de dólar mal etiquetados). Es el problema que resuelve R10.
2. **Aplicar lo que falta** de la rejilla y del bloque de caja.
3. **Darle usuarios de distintos roles** para poder probar, específicamente la caja ampliada.
4. **Seguir implementando todo**, incluido el backend faltante.

**Punto exacto en el que quedó:**

- **El modo caja ampliado NO está implementado.** Existe el backend completo y probado, el modal de abrir caja, la vista «Sin caja abierta», la barra del turno y la rejilla de productos. La pantalla completa con diestro/zurdo no se empezó.
- **R9 (tasa BCV) y R10 (moneda principal + precio homólogo) no se empezaron.** Están decididos y con sus condiciones de seguridad escritas, listos para implementar. Son **una sola pieza**: arreglar solo el formato de los precios sería tapar el síntoma, porque sin tasa confiable el precio homólogo no existe.
- **Las cuentas por rol no se pudieron crear.** El sistema de permisos bloquea la escritura de material de autenticación en la base. Hay un script listo que **debe ejecutar el usuario**; ver §7.
- Mientras tanto, para revisar la interfaz por rol **ya funciona** el selector «Ver la app como» en el menú del avatar (solo Dueña y Desarrollador). Cambia lo que la interfaz muestra, no los permisos del servidor.

---

## 4 · ESTADO ACTUAL

Desplegado en `https://elerp.tech` (Docker Compose: mongo + backend + frontend/nginx). Se verifica que el despliegue tomó comparando el hash del asset publicado con el de `frontend/dist/index.html`.

### Backend

| Qué | Cómo se verificó |
|---|---|
| Dominio `caja` (Caja, Cajero con PIN en bcrypt, Sesión de turno) + adaptadores in-memory y Mongo | Compila; **26 pruebas automatizadas pasan** y `go vet` limpio |
| Endpoints de cajas: listar, crear, habilitar/deshabilitar, abrir (409 si está tomada), cerrar, mi-sesión, cajeros | Probados con `curl` contra el contenedor: PIN incorrecto → 401, cajero de otra sede → 401, caja tomada → 409, apertura → 200 |
| **Regla dura: sin caja abierta no se factura**, y el turno debe ser de la misma sede | Probado: rechazo sin turno; con turno, emite y **graba el arqueo** (`cajaCodigo`, `cajeroNombre`, `sesionCajaId`) en el documento |
| Retomar el turno propio no duplica la apertura | Probado: misma `sesionId`, misma hora de apertura, una sola sesión, y la auditoría registra `caja.abrir` + `caja.retomar` |
| `GET /api/inventario/productos/{sku}/disponibilidad` — stock en todas las sedes con semáforo calculado en el servidor | Probado con `curl`: devuelve las 2 sedes, `estaTienda`, niveles y total de la red |
| Imágenes con **bucket por cliente** (`adapter/almacen`): validación por *magic bytes*, límite 4 MiB, defensa contra *path traversal*, servido autenticado | Probado: PNG → 200; `.txt` → 400; bucket de otro tenant → **403**; traversal a `/etc/passwd` → **404** |
| Motor fiscal | Pruebas de IVA 16%, IGTF solo sobre la porción en divisas, numeración correlativa sin duplicados, anulación que no edita el original y repone stock, descuento de inventario |
| Seed demo enriquecido: 14 productos, 35 movimientos, 14 documentos, 6 clientes, 6 cuentas de cobro, 5 transferencias, 5 rubros | Contado en Mongo tras reiniciar el backend |

### Frontend

Verificado con capturas propias en un contenedor de preview (`127.0.0.1:3200`), sin errores de JavaScript en consola:

- **Tokens** `--hb-*` como custom properties + escala de Tailwind alineada. Botones de 40px en Poppins Medium con las cinco variantes del prototipo. Cinco estados fiscales canónicos (Emitida, Contingencia, Anulada, Borrador, Sincronizando). Chip de conexión real vía `navigator.onLine`. Tooltip «¿qué es esto?».
- **Menú lateral blanco** con los 9 iconos de módulo, nombres completos, grupos OPERACIÓN/FINANZAS/GESTIÓN, punto verde en el activo, colapsable.
- **Barra superior** con ámbito de 3 chips encadenados, buscador ⌘K, «+ Nuevo», Bs/US$, tasa rotulada «Tasa manual» (no miente diciendo «BCV») y estado de conexión.
- **Inicio por rol** (Dueña, Vendedor, Cajero, Contadora): saludo por hora del día con fecha larga, lanzador de 9 módulos, 4 KPIs con tooltip, ventas de 30 días en barras con **serie real derivada del ledger**, pendientes con contador y detalle concreto, tarjeta del Asistente. Los KPIs sin módulo detrás dicen «Sin datos aún» y nombran qué falta.
- **Bloque de caja:** modal «Abrir caja» en dos pasos (elegir caja → código + PIN), vista «Sin caja abierta», barra del turno activo.
- **Rejilla de productos:** imagen o marcador «Sin imagen» sobre uno de los seis matices pastel derivados del código, nombre visible, código en pequeño, semáforo de existencias (agotado en rojo y no agregable, bajo en ámbar), botón de información que abre el **modal de disponibilidad por tienda**.
- **Pantalla «Sistema de diseño»** interna (ruta `diseno`, solo Dueña y Desarrollador) que documenta cada token y componente citando su regla.
- **Racimo modular** en el login y el panel de bienvenida, con las medidas exactas medidas del prototipo.

---

## 5 · PENDIENTE

Ordenado por prioridad. **24 elementos.**

### Bloque inmediato

1. **R9 — Tasa de cambio automática.** Raspado de `bcv.org.ve` como fuente primaria, API de terceros como respaldo, carga manual auditada como último recurso. **Obligatorio cumplir las nueve condiciones de seguridad de `backlog-ux.md` → R9.** Quitar el campo manual del punto de venta.
2. **R10 — Moneda principal y precio homólogo.** `MonedaPrincipal` en el dominio `empresa` (elegida en el onboarding), que `Producto.Precio` declare su moneda, y el homólogo **calculado** con la tasa. Va junto con el 1.
3. **R11 — Código de barras / QR por producto.** Campo en el dominio y que el buscador lo resuelva, para que la pistola lectora encuentre el producto por su código y no solo por SKU.
4. **Subida de imagen desde la ficha del catálogo** (el endpoint ya está probado) + generación de miniaturas.
5. **R2 y R3 — Modo caja ampliado.** Pantalla completa sin menú ni título, tarjetas y carrito más grandes, conmutador **diestro/zurdo**, tema oscuro accesible desde dentro, y salida de la caja con PIN de supervisor opcional (reutiliza `Empresa.RequiereSupervisorPin`).
6. **Cuentas reales por rol.** Requiere que el usuario ejecute el script; ver §7.
7. **Commit del trabajo.** Todo está sin versionar sobre la rama `feat/scaffold-inventario`: `backend/`, `frontend/`, `Documentos/`, `docker-compose.yml`, `CLAUDE.md`, `README.md`, `.claude/` aparecen como *untracked*. Son varios días de trabajo en un solo commit pendiente.

### Módulos de negocio (orden del `LÉEME` del paquete de entrega)

8. **Tesorería** — cuentas de cobro, cobro mixto multi-cuenta, IGTF, CxC/CxP, conciliación. Desbloquea los dos KPIs del Inicio que hoy dicen «Sin datos aún».
9. **Contabilidad append-only** — plan de cuentas, libro diario alimentado por eventos, estados financieros, cierre de período.
10. **Compras y Proveedores** — órdenes con aprobación por umbral, comparador de cotizaciones, escaneo de factura con IA (la IA propone, el humano confirma).
11. **Ventas y CRM completo** — cotizaciones, embudo, comisiones, listas de precios.
12. **RRHH y Nómina** — trabajador, nómina por pasos, prestaciones (LOTTT).
13. **Reportes y BI** sobre réplica analítica.
14. **Plataforma Super Admin** — organizaciones, planes y feature flags, respaldos, errores reportados, soporte. Identidad propia en azul profundo.

### Piezas fiscales y transversales pendientes

15. **Retenciones** (IVA/ISLR) con carga de comprobante y exportación SENIAT en TXT/XML/ARCV.
16. **Cierres Z** por sede, con alerta de cierre pendiente del día anterior.
17. **Libros fiscales** de ventas y compras, con prorrateo del crédito fiscal explicado en lenguaje llano.
18. **Notificaciones** — `GET /notificaciones`, cada aviso con acción directa. Hoy las alertas del Inicio se derivan del estado de los datos porque el endpoint no existe.
19. **Ventas en espera** — dejar un carrito en espera y que otra caja de la sede lo retome.
20. **Configuración** — usuarios y roles con matriz de permisos visible, dispositivos fiscales, integraciones, cajas y sesiones (interruptor del PIN de supervisor).
21. **R7 y R8 — Mapa de tiendas y ubicación de sede.** Leaflet + OSM con el estado degradado offline; extender `sede` con `estado`, `municipio`, `lat`, `lng` y cargar el dataset de estados y municipios de Venezuela.
22. **Restaurante: mesas y comandas** como módulo aparte, a planificar.

### Calidad y seguridad

23. **Cerrar los 9 riesgos de `Documentos/seguridad-pendientes.md`** — apagar `DEV_LOGIN`, MFA obligatorio para roles administrativos, rate limiting, auditoría de solo lectura a nivel de esquema, sello de integridad del ledger, cifrado en reposo, secretos de base de datos, expiración y refresco de sesión.
24. **Extender las pruebas** — RBAC por rol contra endpoints ajenos, aislamiento de tenant en el resto de los módulos, y el checklist de pre-producción de `02 §13.6`. Al final: la **auditoría de ciberseguridad completa**.

---

## 6 · ARCHIVOS CLAVE

### Documentación (leer antes de tocar código)

| Ruta | Para qué |
|---|---|
| `CLAUDE.md` | Arquitectura, stack, principios y comandos de verificación |
| `Documentos/sistema-visual.md` | **Arbitraje entre documentos:** qué manda cuando se contradicen, las 12 contradicciones resueltas y los 6 vacíos detectados |
| `Documentos/backlog-ux.md` | Los 13 requerimientos del usuario con estado, decisiones tomadas y las 9 condiciones de seguridad de la tasa |
| `Documentos/seguridad-pendientes.md` | Registro corriente de seguridad: 9 riesgos abiertos, lo ya resuelto y el checklist de pre-producción |
| `Documentos/flujos-permisos-seguridad-backend.md` | Matriz de permisos y contrato REST por módulo |
| `/root/dashboard-uploads/1785476436393-Dise__o_prototipo_ElERP.zip` | **El prototipo interactivo: el árbitro.** Carpeta `entrega/`, con los manuales de marca en `entrega/assets/marca/` |

### Backend (Go 1.22 + Fiber, hexagonal)

| Ruta | Para qué |
|---|---|
| `backend/internal/domain/caja/caja.go` | Caja, Cajero y Sesión de turno, con la regla de negocio documentada |
| `backend/internal/application/caja.go` | Casos de uso de caja: abrir con validaciones en orden, retomar, cerrar, crear |
| `backend/internal/application/fiscal.go` | Motor fiscal. **Contiene la regla dura de caja abierta** al inicio de `EmitirFactura` |
| `backend/internal/application/inventario.go` | Ledger, Kardex, y `Disponibilidad` multi-sede al final del archivo |
| `backend/internal/adapter/almacen/almacen.go` | Bucket de archivos por cliente, con toda la validación de seguridad |
| `backend/internal/adapter/inmem/seed.go` | **Datos demo.** Aquí viven las credenciales de caja de demostración y el catálogo que cubre toda la gama de casos |
| `backend/internal/adapter/mongo/seed.go` | Siembra de Mongo, con guards **por tenant** (no por colección) |
| `backend/internal/adapter/httpapi/` | Rutas y middleware; `caja.go` traduce los errores de negocio a códigos HTTP |
| `backend/internal/application/caja_test.go` · `fiscal_test.go` | Las 26 pruebas |
| `backend/Dockerfile.test` | Runner de pruebas en contenedor |

### Frontend (React 18 + Vite 5 + Tailwind v3)

| Ruta | Para qué |
|---|---|
| `frontend/src/index.css` | Los tokens `--hb-*` con el orden de autoridad documentado |
| `frontend/tailwind.config.js` | Escala alineada a esos tokens |
| `frontend/src/components/brand.jsx` | Gráficos complementarios: punto hub, racimo modular con medidas exactas, iconos de módulo, losetas táctiles, barra de progreso |
| `frontend/src/components/primitives.jsx` | Botones, badges, estados, tooltip, chip de conexión, estado vacío |
| `frontend/src/components/producto.jsx` | Tarjeta de producto, imagen con marcador, rejilla y **modal de disponibilidad** |
| `frontend/src/screens/Caja.jsx` | `useSesionCaja`, modal de apertura, «Sin caja abierta», barra del turno |
| `frontend/src/screens/POS.jsx` | Punto de venta, ya condicionado al turno de caja |
| `frontend/src/screens/Dashboard.jsx` | Inicio por rol |
| `frontend/src/screens/SistemaDiseno.jsx` | Pantalla de referencia del sistema visual |
| `frontend/src/lib/metricas.js` | Métricas derivadas del ledger real, agrupadas por día **local** |
| `frontend/src/lib/api.js` | Cliente HTTP |
| `frontend/src/context/UIContext.jsx` | Estado de UI; `zurdo`, `dark` y `menuColapsado` se guardan por dispositivo |

---

## 7 · TRAMPAS

Errores ya cometidos y caminos ya descartados. **No repetirlos.**

### De proceso

- **Se abrió solo uno de dos archivos subidos** y se trabajó horas sobre la referencia equivocada, construyendo un Inicio que no se parecía a la maqueta. **Revisar siempre la carpeta completa de subidas.**
- **Se siguió el PDF de branding por encima del prototipo** en dos decisiones grandes (menú lateral azul, botones verdes para «crear») y hubo que revertir ambas. **El prototipo manda.**
- **Se propuso al usuario decidir cosas ya decididas.** Todas las decisiones vivas están en §2 y en `backlog-ux.md`; consultarlas antes de preguntar.

### De entorno y herramientas

- **`npm run build` se muere por falta de memoria.** Usar siempre `NODE_OPTIONS=--max-old-space-size=1536`.
- **`docker compose up -d <servicio>` sin `build` arranca la imagen vieja.** Pasó: los endpoints nuevos daban 404 y parecía un error de rutas. Siempre `docker compose build <servicio> && docker compose up -d <servicio>`, y confirmar el hash del asset publicado.
- **Go no está instalado.** Compilar con `docker build ./backend`; probar con `docker build -f backend/Dockerfile.test -t huberp-test ./backend`.
- **El Node del sistema es 18 y Playwright exige 20.** Hay un Node 20 portátil y Chromium instalados en el directorio temporal de la sesión anterior; si no están, reinstalarlos fuera del proyecto para no ensuciar `package.json`.
- **`poppler-utils` hace falta** para leer los PDF (`pdftotext`, `pdftoppm`). Se instala con apt.
- **Un volumen Docker creado antes de que el Dockerfile prepare la carpeta nace con dueño root** y el proceso, que corre como usuario sin privilegios, no puede escribir. Hay que crear la carpeta y asignar el dueño **antes** de `USER` en el Dockerfile, y recrear el volumen si ya existía.
- **La escritura de material de autenticación en la base está bloqueada** por el sistema de permisos. El script que crea invitaciones por rol **lo tiene que ejecutar el usuario**; si el temporal de la sesión anterior ya se limpió, hay que regenerarlo. Genera los tokens en el momento de ejecución, no los escribe quien los pide.

### De implementación

- **Los guards de la siembra contaban la colección entera, no el tenant.** Como otra empresa ya tenía productos, el conteo global no era cero y la demo se quedó sin sembrar. **Contar siempre por `empresaid`.**
- **El cliente HTTP forzaba `Content-Type: application/json` en toda petición**, lo que rompe `FormData` porque impide al navegador poner su *boundary*. Ya está corregido; no reintroducirlo.
- **Un gráfico de importes mostraba un eje con valores negativos** porque el holgado inferior cruzaba el cero. Corregido: con series no negativas el eje arranca en 0.
- **Una imagen cuyo archivo ya no existe mostraba el icono de imagen rota del navegador**, que en una caja parece una falla del sistema. Corregido con un `onError` que cae al marcador «Sin imagen».
- **Se duplicó la acción primaria verde** en la misma vista (encabezado y estado vacío). Regla: **una sola acción primaria por vista.**
- **Se le mostró al Cajero la serie de ventas de toda la sede**, que excede su alcance de «una caja, una sede». Corregido: ve sus propios documentos del turno.
- **Fiber corre con `Immutable: true`** y es carga estructural: los strings de `c.Get`/`Params`/`Query`/`Cookies` apuntan a un buffer reutilizable. Nunca persistir uno sin copiar; ya corrompió datos entre peticiones una vez.
- **Mongo no tiene Row-Level Security:** el aislamiento de tenant vive en la capa de repositorio, con filtro obligatorio por `empresaid` en **toda** consulta.
- **Los ledgers fiscal y de inventario son append-only.** Nunca `UPDATE` ni `DELETE`: las correcciones son documentos o movimientos de reversa que referencian al original.

### Caminos descartados

- **Sidebar azul navy** (lo pedía el PDF de branding) — descartado, el prototipo lo dibuja blanco.
- **Verde para «crear algo nuevo»** — descartado, el verde es dinero.
- **Racimo modular en cards de datos y estados vacíos de listas** — descartado, solo en superficies de marca.
- **Inter como tipografía de interfaz** y **`#F5F7FA`** como fondo (los pedía `03 §1.3`) — descartados, el prototipo usa IBM Plex Sans y `#FAFBFC`.
- **Estados genéricos** «Activo / En proceso / Borrador / Error» — descartados por los cinco estados fiscales del prototipo.
- **Guardar el precio homólogo duplicado en la base** — descartado, se calcula.
- **Rotular «BCV» una tasa introducida a mano** — descartado, la interfaz declara el origen real.
