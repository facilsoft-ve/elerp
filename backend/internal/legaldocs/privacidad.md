# Política de Privacidad y Tratamiento de Datos — ElERP

**Versión:** 1.0 (borrador)
**Fecha de vigencia:** [pendiente de definir — no publicar hasta revisión legal]
**Última actualización del borrador:** 2026-08-25
**Responsable de la plataforma:** Mornix, C.A. («Mornix») — RIF [PENDIENTE], domicilio [PENDIENTE], República Bolivariana de Venezuela.
**Contacto de privacidad:** [correo de contacto de privacidad — PENDIENTE]

---

> ### ⚠️ AVISO IMPORTANTE — BORRADOR SUJETO A REVISIÓN LEGAL
>
> Este documento es un **borrador** elaborado con estándar profesional como base de trabajo. **NO constituye asesoría jurídica.** Debe ser **revisado, adaptado y validado por un abogado habilitado en la República Bolivariana de Venezuela** antes de su uso en producción. Venezuela **no cuenta con una ley integral única de protección de datos personales** equivalente a la LGPD, el RGPD o la Ley 1581 de Colombia; el marco se construye a partir del derecho constitucional al *habeas data* (art. 28) y a la privacidad (art. 60), de normas sectoriales y de criterios jurisprudenciales. Esta Política adopta, además, buenas prácticas regionales e internacionales como estándar de referencia, sin que ello implique afirmar el cumplimiento de un régimen legal extranjero. Las referencias normativas marcadas *[verificar con asesoría]* deben confirmarse.

---

## 1. Objeto y alcance

1.1. Esta Política describe cómo se tratan los datos personales en el uso de ElERP, la plataforma SaaS de gestión empresarial (ERP) de Mornix, incluyendo su aplicación web (PWA), API, agente fiscal local y servicios asociados.

1.2. Esta Política forma parte integrante de los Términos y Condiciones de Uso de ElERP. Al aceptar los Términos, el Cliente y sus Usuarios declaran haber leído y comprendido esta Política.

1.3. Aplica a: (a) las **personas naturales Usuarias** de la Plataforma (por ejemplo, la Dueña/Admin, Vendedores, Cajeros, Contadora); y (b) los **datos personales de terceros** que el Cliente carga o procesa en la Plataforma (por ejemplo, sus clientes finales, proveedores o empleados).

---

## 2. Roles en el tratamiento: Responsable y Encargado

Esta es una distinción central y determina las obligaciones de cada Parte.

2.1. **Mornix como Responsable.** Respecto de los datos de **registro, autenticación, facturación del servicio, soporte, seguridad y telemetría** de los Usuarios de la Plataforma, Mornix actúa como **Responsable del tratamiento** y decide sobre sus fines y medios, conforme a esta Política.

2.2. **Mornix como Encargado.** Respecto de los **Datos del Cliente** —incluidos los datos personales de terceros que el Cliente introduce en ElERP para operar su negocio (clientes finales, proveedores, empleados)— el **Cliente es el Responsable del tratamiento** y **Mornix actúa únicamente como Encargado del tratamiento** («procesador»), tratando esos datos **por cuenta y según las instrucciones del Cliente**, con el único fin de prestar el servicio.

2.3. **Obligaciones de Mornix como Encargado:**
   (a) tratar los Datos del Cliente solo conforme a las instrucciones del Cliente y a esta Política;
   (b) no usar esos datos para fines propios ni cederlos a terceros salvo lo necesario para prestar el servicio o por obligación legal;
   (c) aplicar medidas de seguridad razonables (cláusula 8);
   (d) mantener la confidencialidad y exigirla a su personal y subencargados;
   (e) asistir razonablemente al Cliente en la atención de solicitudes de titulares y en la notificación de brechas;
   (f) al terminar el servicio, devolver o eliminar los datos conforme a la cláusula 9.

2.4. **Obligaciones del Cliente como Responsable:** contar con base legítima para tratar y cargar los datos de terceros; informar a los titulares; atender el ejercicio de sus derechos; y cargar únicamente los datos necesarios para su operación (minimización). El Cliente es el punto de contacto principal frente a sus clientes finales, proveedores y empleados.

---

## 3. Datos que se tratan

### 3.1. Datos de Usuarios de la Plataforma (Mornix como Responsable)
- **Identificación y cuenta:** nombre, correo electrónico, identificador de Usuario, rol asignado, Empresa/Sede asociada.
- **Autenticación:** credenciales gestionadas a través del proveedor de identidad (Hubmy SSO) y/o credenciales nativas; identificadores de sesión.
- **Datos de la Empresa/Cliente:** razón social, RIF, giro, modalidad de facturación, sedes, configuración de monedas y tasa.
- **Uso y telemetría:** registros técnicos, dirección IP, tipo de dispositivo/navegador, marca temporal (UTC), eventos de auditoría (actor, rol, acción, origen, fecha).
- **Soporte:** comunicaciones y datos que el Usuario aporte al solicitar asistencia.

### 3.2. Datos del Cliente que pueden incluir datos personales de terceros (Mornix como Encargado)
- **Clientes finales del Cliente:** nombre/razón social, RIF/cédula, dirección, contacto, historial de compras y documentos fiscales.
- **Proveedores:** datos de identificación y contacto.
- **Empleados del Cliente:** datos que el Cliente decida gestionar en módulos de RRHH/nómina (según se implementen).
- **Datos comerciales y fiscales:** facturas, notas, retenciones, inventario, contabilidad, tesorería.

> El Cliente es responsable de determinar qué datos carga. Mornix recomienda **no cargar datos sensibles** (salud, biometría, etc.) salvo que sea imprescindible y con base legal adecuada.

---

## 4. Finalidades del tratamiento

4.1. **Datos de Usuarios (Responsable):**
   (a) prestar, operar y mantener la Plataforma; (b) autenticar y controlar accesos (RBAC/ABAC); (c) garantizar seguridad, prevención de fraude y auditoría; (d) facturar el servicio y gestionar la relación contractual; (e) brindar soporte; (f) cumplir obligaciones legales; (g) mejorar el producto mediante analítica agregada; (h) comunicar avisos operativos y, con base separada, comunicaciones comerciales cuando corresponda.

4.2. **Datos del Cliente (Encargado):** exclusivamente **procesarlos por cuenta del Cliente** para que este opere su negocio (emitir documentos, llevar inventario/contabilidad, generar reportes), conforme a sus instrucciones. Mornix **no** usa estos datos para fines propios de marketing ni los comercializa.

---

## 5. Base de legitimación

5.1. En el marco venezolano, el tratamiento se sustenta en:
   (a) el **consentimiento** informado del titular/Usuario, manifestado al aceptar los Términos y esta Política;
   (b) la **ejecución del contrato** de servicio con el Cliente;
   (c) el **cumplimiento de obligaciones legales** (por ejemplo, conservación de documentos fiscales, deberes formales del Código Orgánico Tributario);
   (d) el **interés legítimo** de Mornix en la seguridad, prevención de fraude y mejora del servicio, ponderado con los derechos de los titulares;
   (e) el marco constitucional de *habeas data* (art. 28) y protección de la privacidad (art. 60) de la Constitución. *[verificar con asesoría]*

5.2. Como estándar de referencia se observan buenas prácticas de la LGPD (Brasil), Ley 25.326 (Argentina), Ley 1581/2012 (Colombia), LFPDPPP (México) y el RGPD (UE), **sin que ello implique sujeción a dichos regímenes** salvo que resulten aplicables por vínculo de conexión.

---

## 6. El asistente de IA y el tratamiento de datos

6.1. ElERP incluye un asistente organizado en dos capas:

   (a) **Capa mecánica (local, por defecto):** un enrutador de intenciones acotado al rol que opera **dentro de la Plataforma**, sin enviar datos a un proveedor externo de IA. Es la capa activa por defecto.

   (b) **Capa de IA (opcional, opt-in):** solo si el Cliente/Usuario la **activa expresamente**, determinadas consultas se procesan mediante un proveedor externo de IA (a través del proxy de IA de Hubmy). En ese caso se transmite **contexto acotado al rol** del Usuario, estrictamente lo necesario para responder la consulta.

6.2. **Controles de la capa de IA:**
   - es opt-in y puede desactivarse en Configuración › Asistente;
   - se procura minimizar el contexto enviado (acotado al rol y a la consulta);
   - se recomienda no incluir datos personales sensibles en las consultas;
   - el tratamiento por el proveedor externo se rige también por los términos de dicho proveedor;
   - las respuestas son orientativas y pueden contener errores; no constituyen asesoría profesional.

6.3. Mornix no utiliza los Datos del Cliente para entrenar modelos propios sin base legal y aviso adecuado. *[verificar redacción con asesoría y con los términos del proveedor de IA]*

---

## 7. Cookies, almacenamiento local y telemetría

7.1. La Plataforma utiliza cookies y mecanismos de almacenamiento (por ejemplo, cookie de sesión, `localStorage`/`IndexedDB` para el modo offline y preferencias) **necesarios para su funcionamiento y seguridad**.

7.2. Puede emplear telemetría y analítica técnica para diagnóstico, seguridad y mejora del servicio. En la medida en que se usen cookies no esenciales o analítica ampliada, se solicitará el consentimiento correspondiente y se ofrecerán controles. *[verificar exigencias de consentimiento aplicables]*

---

## 8. Seguridad de la información

8.1. Mornix aplica medidas técnicas y organizativas razonables acordes con las prácticas de la industria, incluyendo:
   - **aislamiento estricto entre *tenants*** (filtro obligatorio por identificador de Empresa en cada consulta de la capa de persistencia);
   - control de acceso basado en roles y atributos evaluado en el servidor (mínimo privilegio);
   - **registro de auditoría inmutable** (solo-anexado) de acciones sensibles;
   - **inmutabilidad de documentos fiscales** (append-only, correcciones mediante documentos de reverso) y **sello de integridad** (por ejemplo, sellado sobre blockchain / OpenTimestamps);
   - cifrado en tránsito y controles de acceso a la infraestructura.

8.2. **Sin sobredeclaración de certificaciones.** Mornix **no** afirma poseer certificaciones (por ejemplo, ISO 27001 o SOC 2) salvo que se declare expresamente por escrito. Las referencias a dichos marcos describen una **postura de seguridad aspiracional y de referencia**, no una certificación vigente.

8.3. Ninguna medida de seguridad es infalible. El Cliente y sus Usuarios comparten la responsabilidad de la seguridad (gestión de credenciales, roles, dispositivos).

---

## 9. Conservación y eliminación

9.1. Los datos se conservan durante el tiempo necesario para las finalidades descritas y mientras exista relación contractual, y luego durante los **plazos legales de conservación** aplicables (en particular, la conservación de documentos y libros fiscales por los plazos de prescripción tributaria).

9.2. Terminada la relación, Mornix ofrecerá al Cliente la **exportación de sus datos** durante un período razonable (por ejemplo, 30 días) y luego procederá a su eliminación o anonimización, salvo obligación legal de conservación. Los registros de auditoría inmutables se conservan por su naturaleza probatoria durante el plazo que corresponda.

9.3. El Cliente es responsable de exportar y conservar sus documentos fiscales por los plazos legales, con independencia de la conservación que realice Mornix.

---

## 10. Comunicación y transferencia de datos a terceros

10.1. Mornix puede compartir datos con **subencargados y proveedores** estrictamente necesarios para prestar el servicio, sujetos a deberes de confidencialidad y seguridad, entre ellos:
   - **proveedor de identidad/SSO y de IA** (Hubmy y el proveedor de IA integrado);
   - **proveedor de infraestructura en la nube / alojamiento** [PENDIENTE — indicar proveedor y región];
   - **servicios de sellado de integridad** (blockchain / OpenTimestamps);
   - **pasarelas de pago y fuentes de tasa oficial**, cuando apliquen.

10.2. Mornix **no vende** datos personales.

10.3. Mornix podrá divulgar datos cuando lo exija una autoridad competente o la ley (por ejemplo, requerimientos del SENIAT o judiciales), conforme a derecho.

---

## 11. Almacenamiento y transferencia internacional (nube)

11.1. Los datos se almacenan en MongoDB y en infraestructura que **puede estar ubicada fuera de Venezuela** [PENDIENTE — confirmar región del proveedor]. El Cliente y los Usuarios reconocen y consienten que la prestación del servicio puede implicar **almacenamiento o procesamiento transfronterizo** de datos.

11.2. Mornix procurará que dichas transferencias cuenten con salvaguardas contractuales de confidencialidad y seguridad razonables. *[verificar requisitos de transferencia internacional aplicables]*

---

## 12. Derechos de los titulares (habeas data)

12.1. De conformidad con el derecho constitucional al *habeas data* (art. 28 de la Constitución) y las buenas prácticas de referencia, los titulares pueden ejercer los derechos de **acceso, rectificación, actualización, supresión** y, cuando aplique, **oposición y portabilidad** respecto de sus datos personales.

12.2. **Cómo ejercerlos:**
   - **Usuarios de la Plataforma:** dirigir su solicitud a Mornix al contacto de privacidad [PENDIENTE], acreditando su identidad.
   - **Clientes finales, proveedores o empleados del Cliente (datos tratados por Mornix como Encargado):** deben dirigirse **al Cliente**, que es el Responsable. Si la solicitud llega a Mornix, esta la remitirá al Cliente y le asistirá razonablemente en su atención.

12.3. Mornix atenderá o canalizará las solicitudes en un plazo razonable conforme a la normativa aplicable. Algunos derechos pueden estar limitados por obligaciones legales de conservación (por ejemplo, integridad e inmutabilidad de documentos fiscales y de la bitácora de auditoría, que por ley no pueden alterarse ni borrarse a solicitud).

---

## 13. Datos de menores

13.1. La Plataforma está dirigida a empresas y profesionales; no se orienta a menores de edad ni se recaban conscientemente sus datos como Usuarios.

---

## 14. Brechas de seguridad y notificación

14.1. Ante un incidente de seguridad que comprometa datos personales, Mornix, como Encargado, **notificará al Cliente sin dilación indebida** tras tener conocimiento razonable del incidente, aportando la información disponible sobre su naturaleza, alcance y medidas adoptadas o recomendadas.

14.2. Cuando Mornix actúe como Responsable (datos de Usuarios), evaluará y realizará las notificaciones que correspondan a los titulares y/o autoridades conforme a la normativa aplicable.

14.3. El Cliente, como Responsable de los datos de sus terceros, es quien decide y ejecuta las notificaciones a los titulares afectados y a las autoridades que correspondan, con la asistencia razonable de Mornix.

---

## 15. Cambios en esta Política

15.1. Mornix podrá actualizar esta Política por cambios legales, técnicos o del servicio. Los cambios materiales se comunicarán con antelación razonable y, cuando proceda, se solicitará **nueva aceptación** al iniciar sesión, registrándose la versión aceptada de forma auditable (versión, fecha/hora UTC, Usuario e IP).

15.2. El uso continuado de la Plataforma tras la vigencia de una nueva versión implica conformidad, sin perjuicio de la aceptación expresa cuando se solicite.

---

## 16. Contacto

16.1. Para consultas sobre privacidad o para ejercer derechos: [correo/dirección de contacto de privacidad — PENDIENTE].

---

### Anexo de referencias normativas (a verificar por asesoría jurídica venezolana)

- Constitución de la República Bolivariana de Venezuela — art. 28 (*habeas data*), art. 60 (privacidad, honor, reputación). *[verificar]*
- Ausencia de ley integral única de protección de datos en Venezuela — marco construido con normas sectoriales, Ley de Infogobierno (2013), Ley Especial contra los Delitos Informáticos (G.O. 37.313, 30/10/2001) y criterios jurisprudenciales. *[verificar]*
- Decreto con Fuerza de Ley sobre Mensajes de Datos y Firmas Electrónicas — G.O. 37.148 (28/02/2001). *[verificar]*
- Código Orgánico Tributario (reforma 2020) — conservación de documentos/libros y deberes formales. *[verificar]*
- Estándares de referencia (no vinculantes salvo aplicabilidad): RGPD (UE), LGPD (Brasil), Ley 25.326 (Argentina), Ley 1581/2012 (Colombia), LFPDPPP (México). *[referencia de buenas prácticas]*

---

*Fin del borrador de Política de Privacidad y Tratamiento de Datos — ElERP v1.0. Documento no vigente hasta su validación legal y la fijación de la fecha de vigencia.*
