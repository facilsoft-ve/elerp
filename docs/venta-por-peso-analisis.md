# Venta de comida por peso — análisis del modelo

**Estado: ANÁLISIS. No implementado.** Documento para llevar a la Junta y decidir si se
construye, y en qué forma.

Redactado el 23 de septiembre de 2026, a partir del planteamiento de J. G. Mora (Mornix).

---

## 1. El modelo de negocio

Un restaurante con **comida lista servida en línea**: bandejas de contornos, ensaladas y
proteínas. El cliente recorre la fila, se sirve lo que quiere en la cantidad que quiere, y al
final **se pesa el plato y paga una tarifa por kilo**.

Variante del mismo modelo, con otras propiedades: la **barra de sándwiches**, donde el cliente
arma el suyo con vegetales frescos (tomate, lechuga) y embutidos de alto valor (pepperoni,
salchichón, jamón). Es la misma mecánica con una diferencia que importa mucho — ver §6.

No es el modelo de la mayoría de los restaurantes, pero es un modelo real y con operadores
suficientes como para que la pregunta sea de producto y no de curiosidad.

---

## 2. Por qué rompe un supuesto de ElERP

Toda la aplicación es **inventario perpetuo**: cada venta sabe qué se vendió, y la existencia es
la proyección del ledger de movimientos.

Este modelo no puede serlo. **La báscula de la caja sabe peso y dinero, no composición**: nadie
sabe si esos 480 gramos eran arroz o carne mechada. El consumo no se registra en el momento de
la venta — se *reconstruye* al cerrar la jornada.

Es decir: este pedazo del negocio es necesariamente **inventario periódico**. No es una carencia
del diseño propuesto; es la naturaleza del modelo. Conviene nombrarlo desde el principio porque
cambia qué se puede prometer y qué no.

### Lo que este modelo NUNCA va a poder responder

- **Margen por plato**: imposible. Solo margen por período.
- **Qué comida es más rentable**: incontestable. Sí se puede responder *cuál mueve más kilos*.
- **Robo vs. derrame vs. servicio generoso**: se obtiene un solo número; separarlo es humano.

Decirlo por adelantado evita construir un reporte que promete una precisión que no tiene.

---

## 3. La identidad que hay que cerrar

Por preparación, en una jornada:

```
consumido = peso_inicial + reposiciones − peso_final − tara
```

Y globalmente:

```
Σ consumido   vs.   Σ peso vendido en caja
```

**Esa diferencia es el número que justifica toda la función.** Hoy ElERP no la puede mostrar.
Si se consumieron 28 kg y la caja registró 24 kg vendidos, esos 4 kg son derrame, comida del
personal, servicio generoso o robo.

### Dos cosas que invalidan la medición si no se modelan bien

**La tara.** Si se pesa bandeja + comida, la tara tiene que ser propiedad del recipiente, nunca
tecleada cada vez. 200 g de error × 15 bandejas × 2 veces al día = **6 kg/día de merma
fantasma**. Este detalle solo decide si el dato sirve o no.

**Las reposiciones.** Se rellena el arroz a mediodía. Si eso no se pesa y registra, la
conciliación está mal y nadie va a saber por qué.

---

## 4. El precio por kilo: dónde está el error que se comete

### 4.1 El divisor

La fórmula intuitiva —sumar la utilidad al costo y dividir entre los kilos— tiene un error
sistemático: hay que dividir entre los kilos que se van a **vender**, no entre los que se
**ponen**. Nunca se vende el 100% de lo que se saca. Y el margen de merma va **en el divisor**,
no sumado encima:

```
precio_kg = costo_total × (1 + utilidad) / (kilos_ofrecidos × (1 − merma_esperada))
```

Con 15% de merma: dividir entre los kilos ofrecidos y después sumar 15% da ×1,15; dividir entre
0,85 da ×1,176. Parece poco, es sistemático, y se compone con el margen.

### 4.2 El error grande: si las sobras no se reaprovechan, el costo base cambia

| Preparación | kg | Bs/kg | Costo |
|---|---:|---:|---:|
| Arroz | 10 | 30 | 300 |
| Pollo guisado | 8 | 120 | 960 |
| Carne mechada | 6 | 180 | 1.080 |
| Ensalada | 5 | 50 | 250 |
| Plátano | 6 | 40 | 240 |
| **Total** | **35** | | **2.830** |

Costo promedio: **80,86 Bs/kg**. Con 35% de utilidad → **109,16 Bs/kg** por la fórmula
intuitiva; **128,42 Bs/kg** corrigiendo el divisor con 15% de merma.

El día real: el cliente carga proteína y deja el arroz. Se consumen los 6 kg de carne, los 8 de
pollo, 3 de arroz, 2 de ensalada y 2 de plátano → **21 kg vendidos, 2.310 Bs de costo consumido
= 110 Bs/kg realizado**.

- **A 109,16 Bs/kg**: se ingresan 2.292 contra 2.310 consumidos → **pérdida**, con un «margen»
  nominal del 35%.
- **A 128,42 Bs/kg**: se ingresan 2.697, hay 387 de ganancia sobre lo consumido… pero quedaron
  **520 Bs de comida en la línea**. Si el arroz y el plátano no se reaprovechan, el costo del
  día fue 2.830 y **también hay pérdida**.

Para cubrir el costo total puesto con 21 kg vendidos harían falta ~182 Bs/kg. **Entre 109 y 182
hay un 67% de diferencia, y todo depende de cuánto de la sobra se reaprovecha.** Esa es la
variable más grande del modelo — más que la mezcla.

### 4.3 El riesgo de mezcla

Un solo precio por kilo significa que **el cliente elige la mezcla y el negocio carga con la
varianza del costo**. El costo por kilo vendido no es un número: es una variable aleatoria cuya
media es el promedio ponderado y cuya dispersión viene del sesgo hacia lo caro.

El «margen de seguridad» es, en rigor, **pricear en un percentil, no en la media**. Y para saber
dónde está ese percentil hacen falta datos: tras unas semanas de conciliación se conoce la
distribución real de la mezcla, no una estimación.

**Ese es el argumento más fuerte para construir el ciclo de pesaje primero y el modelo de precio
después**: el modelo sin datos es aritmética sobre números inventados.

**Mitigación estructural que usan los operadores reales:** sacar la proteína del precio por kilo
y cobrarla aparte. Es la solución más común, y significa que el ticket tiene que mezclar una
línea por peso con líneas discretas. Es una decisión de producto previa a construir, porque
cambia el flujo del punto de venta.

---

## 5. Qué pasa con lo que queda: no es un binario

Plantearlo como «merma o inventario» pierde justo la información que sirve.

| Destino | Qué es contablemente | Qué hay que saber después |
|---|---|---|
| **Merma** | Costo del período | Cuánto y de qué |
| **Vuelve a inventario** | Sigue siendo activo | Que **ya no es el mismo producto**: es «carne mechada del 23/09» con vida útil corta |
| **Reproceso** | Insumo de otra fabricación | El pollo de hoy es el relleno de la empanada de mañana |
| **Consumo de personal** | Costo, pero no merma | En muchos sitios tiene además tratamiento de beneficio laboral |

El **reproceso** merece salir del montón: es más seguro y más valioso que revender tal cual, y
**ya es una orden de fabricación** cuyo insumo es la sobra. Cierra solo con el módulo existente.

### 5.1 Salubridad: por qué no puede ser una casilla

La comida preparada casi nunca debería venderse al día siguiente. Pero el sistema no debería
*permitir elegir* — debería hacer que lo seguro sea el default y lo inseguro sea **deliberado y
trazable**.

Y hay una razón que va más allá de la operación: **si ElERP registra «vuelve a inventario» sobre
una bandeja de pollo que pasó seis horas a temperatura ambiente, y eso se vende y alguien se
enferma, el registro es prueba.** La bitácora append-only corta para los dos lados: protege
cuando se hizo bien y condena cuando no. Esta función tiene una dimensión de responsabilidad
legal que la mayoría de las de inventario no tiene.

Eso apunta a tres cosas concretas:

1. **Vida útil tras preparación** como atributo del producto, en horas. El vencimiento se calcula
   desde que se **preparó**, no desde que se cerró la línea.
2. Si la vida restante es cero o negativa, **«vuelve a inventario» ni se ofrece**. No una
   advertencia — no se ofrece.
3. Si se ofrece y se elige, queda quién lo decidió y cuándo, y el lote resultante arrastra la
   fecha original. El Kardex de mañana no dice «carne mechada»: dice «carne mechada, preparada
   ayer, vence hoy 14:00».

### 5.2 La decisión tiene que estar mayormente decidida de antemano

Si a las 9 de la noche se le pregunta al cocinero qué hacer con cada bandeja, va a contestar
quince veces lo mismo mecánicamente — y entonces la vez que sí importaba también la contesta
mecánicamente.

El destino por defecto va en el **producto**; el cierre solo pregunta donde de verdad es un
juicio. **Una decisión que se pide todos los días y es la misma el 95% de las veces deja de ser
una decisión.**

### 5.3 El incentivo perverso

Si «vuelve a inventario» es fácil, se convierte en el clic por defecto: **la merma se ve mal en
un reporte y la reincorporación se ve neutra**. El encargado que no quiere explicar 4 kg de
merma marca todo como recuperable, y al día siguiente lo bota igual — donde vuelve a ser merma,
pero un día después y ya desconectada de la sobreproducción que la causó.

Eso vuelve inútil la métrica en un mes. Lo que lo evita:

- La sobra reincorporada **lleva una marca**, y su merma posterior se imputa a la **fecha de
  producción original**. No se puede lavar la sobreproducción difiriéndola.
- El reporte muestra **merma total del período**, incluida la diferida.
- Una preparación **no se puede reincorporar dos veces**. Si sobró dos días seguidos, no sobró:
  se produjo de más.

### 5.4 Las reposiciones rompen «la sobra»

Si se rellena el arroz dos veces, lo que queda al cerrar es una **mezcla de tandas con horas de
preparación distintas**. La pregunta «¿vuelve a inventario?» deja de tener respuesta única.

Rastrear tandas dentro de la bandeja no lo va a hacer nadie. La salida practicable es que **lo
que queda herede la hora de preparación MÁS VIEJA** de esa bandeja: conservador, defendible, y
hace que la respuesta segura sea la automática. La tentación es la contraria —heredar la más
nueva— y esa es exactamente la que envenena gente.

---

## 6. La barra de sándwiches: por qué una sola tasa de recuperación no alcanza

Dentro de **una misma línea** conviven perfiles opuestos:

| Insumo | Vida útil | Recuperable | Valor unitario |
|---|---|---|---|
| Lechuga, tomate | horas | ~0% | bajo |
| Pan | 1 día | bajo | bajo |
| Quesos | días | alto | medio |
| Embutidos (pepperoni, salchichón) | días, sellados | **alto** | **alto** |
| Salsas | días | alto | bajo |

**El valor y la perecibilidad están correlacionados al revés de lo que conviene:** lo caro
(embutidos) es lo que el cliente carga *y* lo que sí se recupera; lo barato (lechuga) es lo que
se bota.

Las dos fuerzas se compensan parcialmente:

- **El riesgo de mezcla es peor** que en el buffet caliente, porque la dispersión de valor entre
  insumos es mayor.
- **La tasa de recuperación es mejor**, porque la parte cara sobrevive.

Dónde queda el neto solo lo dice el dato real. Pero la conclusión de diseño es firme: **la tasa
de recuperación y la vida útil son por producto, nunca por línea ni por módulo.**

### 6.1 Consecuencia sobre el precio

Esto convierte el mayor agujero del §4.2 en algo **medible, y por preparación**. El costo que
hay que recuperar hoy deja de ser «todo lo que puse»:

```
costo_a_recuperar = Σᵢ costoᵢ × (1 − reincorporaciónᵢ × supervivenciaᵢ)
```

donde `supervivencia` es la probabilidad observada de que lo reincorporado **efectivamente se
venda** en vez de terminar en merma mañana. Las dos tasas salen del mismo pesaje tras unas
semanas.

En el ejemplo: el arroz (0% recuperable) tiene que cubrir su costo completo hoy; la carne mechada
(60% recuperable, 80% de supervivencia) solo tiene que cubrir el 52%. Eso da un precio por kilo
mucho más fino que un 15% de merma aplicado a todo por parejo — y explica por qué dos
restaurantes con el mismo costo promedio necesitan precios distintos.

---

## 7. Esto no es una función de restaurante

La venta por peso desde una línea **no es solo comida**: frutos secos, dulces a granel, tornillos
por kilo, alimento para mascotas, productos de limpieza decantados. Ahí la sobra es 100%
recuperable y la merma es casi cero.

Es exactamente la misma maquinaria —pesar al abrir, pesar al cerrar, conciliar— y **lo único que
las distingue es la tasa de recuperación y la vida útil, que son parámetros, no módulos
distintos**. Misma conclusión a la que se llegó con delivery y con fabricación: construirlo
genérico y que el rubro sea configuración.

---

## 8. Relación con el módulo de Fabricación

### 8.1 Lo que ya encaja

- **La bandeja ES la salida de una orden de fabricación.** Se cocinan 8 kg de carne mechada
  desde insumos y el módulo ya entrega el costo derivado. Ese es el costo de entrada de la línea.
- El ledger sigue append-only: la conciliación **emite movimientos** (salida por consumo, ajuste
  por merma), no edita nada.
- La contabilidad ya tiene dónde caer: una salida sin factura asienta costo de ventas y un ajuste
  negativo asienta merma.

### 8.2 Lo que no encaja y hay que corregir

El campo `pesoUnitario` de la orden asume «1 torta que pesa 2 kg». Acá es al revés: se producen
**8,4 kg de carne mechada**, sin unidades. Para preparaciones de línea la orden tiene que
producir **directo en kilos**. Son dos casos distintos y confundirlos ensucia el Kardex.

### 8.3 Fórmulas estrictas: qué falta hoy y por qué este modelo lo exige

Hoy `Receta` es una lista plana de `{SKU, cantidad}` y la orden consume exactamente eso. Alcanza
para una torta. **No alcanza** para producción seria, y este modelo lo deja en evidencia:

1. **Rendimiento (yield).** 10 kg de pollo crudo rinden 6,5 kg de pollo cocido. La fórmula debe
   expresar **entrada → salida con factor de rendimiento**. Sin él no se puede distinguir «se
   produjo menos de lo esperado» de «se planificó mal».
2. **Mermas de preparación por insumo.** Pelado del tomate (10%), limpieza de la carne (15%).
   Son por ingrediente, no por fórmula.
3. **Unidades y conversiones.** La fórmula dice «200 g de pepperoni» y se compra en piezas de
   3 kg. Hoy la cantidad se asume en la unidad base del producto, sin validación.
4. **Tamaño de lote.** Una fórmula es «para 10 kg de tanda», no «para 1 unidad». Y no todo escala
   lineal (condimentos, tiempos).
5. **Sub-fórmulas.** La salsa es una fórmula; el sándwich usa la salsa. Hoy la receta es plana y
   no se explota recursivamente: si el componente es a su vez un preparado sin stock, falla.
6. **Versionado.** La orden congela sus consumos, pero la **fórmula** no está versionada: no se
   puede responder «qué decía la fórmula en agosto».
7. **Tolerancias.** Una fórmula estricta declara varianza aceptable y avisa cuando el consumo
   real se desvía.

**El punto 7 es el que une las dos cosas.** Hoy la orden no puede detectar sobreconsumo, porque
el consumo real *es* el teórico por construcción: consume exactamente lo que dice la fórmula.

En el modelo por peso, el consumo real **se mide de forma independiente** (pesando). Entonces:

> **Fórmula estricta (teórico) + conciliación por pesaje (real) = control de producción de
> verdad.** Ninguna de las dos por separado lo da.

---

## 9. ¿Extender Fabricación o crear un módulo?

**A favor de extender Fabricación:** la preparación de línea es una salida de producción; la
conciliación es «real vs. teórico», que es control de producción; y evita un tercer sitio que
mueve inventario —ya se comprobó que cada sitio nuevo es una oportunidad de que la contabilidad
se desvíe.

**A favor de un módulo aparte:** la mayoría de los restaurantes no trabaja así, y meterlo dentro
de Fabricación haría que todo fabricante vea conceptos de buffet; el flujo de cobro es distinto
(pesar en caja, precio único por kilo); y la cadencia es un ciclo diario de servicio, no una
orden.

### Recomendación: tres piezas, no dos extremos

1. **Fabricación (existente, reforzado con fórmulas estrictas)** — produce la preparación con
   costo real. Genérico: sirve igual a un taller que a una cocina.
2. **Línea de servicio** — módulo nuevo y **delgado**: abre con preparaciones, admite
   reposiciones, cierra con pesos, concilia. Activable. Depende de Fabricación.
3. **Venta por peso en el punto de venta** — parcialmente existe (`TipoVentaPeso`, balanza como
   dispositivo). Falta el cobro por peso total del plato.

**La razón de separar (2) de (1) es que la frontera de activación coincide con la frontera del
negocio:** un taller mecánico quiere (1) y nunca (2). Un buffet quiere las tres. Una bodega que
vende frutos secos a granel quiere (2) y (3) sin (1).

---

## 10. Riesgos

| Riesgo | Por qué importa | Mitigación |
|---|---|---|
| **Adopción** | Pesar 15 bandejas dos veces al día es trabajo real. Con balanza conectada son 20 s por bandeja y se hace; tecleando números se abandona en dos semanas **y el dato queda envenenado** | La integración con balanza no es un lujo: es lo que decide si la función existe |
| **Corrupción de la métrica** | El incentivo de §5.3 | Marca de origen, merma diferida, prohibir doble reincorporación |
| **Responsabilidad legal** | §5.1 | Vida útil como dato duro, no como advertencia |
| **Dependencia de hardware** | Sin balanza no hay modelo | Decidir el proveedor antes de comprometer alcance |
| **Tamaño del nicho** | Pocos restaurantes lo usan | Cuantificar antes de invertir (pregunta comercial, no técnica) |

---

## 11. Orden recomendado, si se aprueba

1. **Fórmulas estrictas en Fabricación.** Beneficia a **toda** la producción, no solo a este
   modelo, y es prerrequisito del control real. *Se puede hacer con independencia de la decisión
   sobre venta por peso.*
2. **El ciclo de pesaje y conciliación**, con balanza. Sin precio sugerido, sin nada más: que
   durante unas semanas simplemente mida consumo real, merma y la brecha contra lo vendido.
3. **Con esos datos**, el modelo de precio — ya con distribución real de mezcla y tasas reales de
   reincorporación y supervivencia, que son los números que hoy no existen.

---

## 12. Decisiones que necesita la Junta

1. **¿El nicho justifica el módulo?** Pregunta de mercado: cuántos prospectos reales operan así.
2. **¿La proteína entra en el precio por kilo o se cobra aparte?** Cambia el flujo del punto de
   venta, no solo la fórmula.
3. **¿Fórmulas estrictas ahora?** Beneficia a todos los fabricantes, no solo al buffet. Se puede
   aprobar por separado.
4. **¿El reproceso se modela desde el principio, o la primera versión solo distingue merma de
   reincorporación?** *Recomendación: reproceso después —ya tiene dónde vivir en Fabricación—
   pero la **marca de origen de la sobra hay que ponerla desde el día uno**, porque no se puede
   agregar hacia atrás.*
5. **¿Con qué balanza?** Es la dependencia que decide la factibilidad.
