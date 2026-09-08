import { useState, useMemo, useRef, useEffect } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Wordmark } from '../components/Logo.jsx'
import { Button, useToast, useConfirm, Modal, Field, Input, Select, Badge } from '../components/primitives.jsx'
import { fmtCurrency, fmtNum, TIPOS_DOCUMENTO, validarRIF } from '../lib/format.js'
import { calcularTotales, IVA_TASA } from '../lib/fiscal.js'
import { useData, useTasa } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { useAuth } from '../context/AuthContext.jsx'
import { api } from '../lib/api.js'
import { useSesionCaja, AbrirCajaModal, ArqueoModal } from './Caja.jsx'
import { ImagenProducto, nivelStock, DisponibilidadModal } from '../components/producto.jsx'
import { CobroModal } from './Cobro.jsx'
import { ImpresionFiscalModal, impresoraActivaDeSede } from './ImpresionFiscal.jsx'
import { usePosCanal } from './PantallaCliente.jsx'
import { resolverTema } from '../lib/tema.js'
import { useEspera, DejarEnEsperaModal, EsperaModal } from './Espera.jsx'
import { precioListaEnBs, itemDeLista, monedaDe, porCodigo } from '../lib/precio.js'
import { explotarCombo } from '../lib/combo.js'
import { fechaCortaVE } from '../components/tasa.jsx'

/* Selecciona la BALANZA ACTIVA destino de la sede — mismo patrón que
 * impresoraActivaDeSede (ver ImpresionFiscal.jsx): se prefiere una balanza atada a
 * la sede activa; si no hay, una registrada para toda la empresa (sedeId vacío).
 * Solo dispositivos activos de tipo 'balanza'. */
export function balanzaActivaDeSede(dispositivos, sedeId) {
  const activas = (dispositivos || []).filter((d) => d && d.activo && d.tipo === 'balanza')
  return activas.find((d) => d.sedeId && d.sedeId === sedeId) || activas.find((d) => !d.sedeId) || null
}

// Un producto se vende POR PESO (en kilogramos) si tipoVenta === 'peso'. Vacío o
// ausente = por unidad (comportamiento por defecto).
const esPorPeso = (p) => p?.tipoVenta === 'peso'

// Redondeo del peso a 3 decimales (precisión en kg del pesaje).
const round3 = (v) => Math.round((Number(v) || 0) * 1000) / 1000

/* MODO CAJA — el puesto de cobro a pantalla completa (R2 + R3).
 *
 * Es la pantalla del prototipo «Modo caja»: sin barra lateral y sin barra de
 * navegación, porque el cajero no navega la aplicación, cobra. Lleva su propia
 * cabecera con quién está en la caja, la tasa del día y tres controles:
 *
 *   · diestro/zurdo — invierte las dos columnas. La caja es un puesto físico y
 *     hay gente que trabaja con la izquierda;
 *   · claro/oscuro — dentro de la caja, porque acá no hay topbar donde tenerlo;
 *   · salir — vuelve a la vista normal; si la empresa exige PIN de supervisor,
 *     lo pide (flujo 2.4) y el servidor lo valida.
 *
 * Las dos preferencias se guardan POR DISPOSITIVO (localStorage), no por
 * usuario: el puesto lo comparten los turnos.
 */
export function ModoCaja({ onSalir }) {
  const { db, reload, tasaDe } = useData()
  const { ui, setUi } = useUI()
  const { user } = useAuth()
  const tasa = useTasa()
  const toast = useToast()
  const confirm = useConfirm()

  const productos = (db.PRODUCTOS || []).filter((p) => p.activo !== false)
  const existencias = db.EXISTENCIAS || []
  const monedaEmpresa = db.EMPRESA?.monedaPrincipal || 'VES'
  const sedeNombre = db.SEDE_ACTIVA?.nombre || ''
  const requierePin = !!db.EMPRESA?.requiereSupervisorPin
  // Exigir identificar al cliente (cédula/RIF) antes de cobrar (Ajustes). Apagado
  // = comportamiento actual (Consumidor final por defecto).
  const exigeCedula = !!db.EMPRESA?.exigirCedulaCliente
  // Balanza activa de la sede (para pesar los productos vendidos por peso). Si no
  // hay ninguna configurada, el pesaje sigue disponible con entrada manual.
  const balanza = useMemo(
    () => balanzaActivaDeSede(db.DISPOSITIVOS, db.SEDE_ACTIVA?.id),
    [db.DISPOSITIVOS, db.SEDE_ACTIVA?.id],
  )

  const { sesion, cargando, recargar } = useSesionCaja()
  const [abrir, setAbrir] = useState(false)

  const [q, setQ] = useState('')
  const [cart, setCart] = useState([])
  const [cliente, setCliente] = useState(null) // null = Consumidor final
  const [identificando, setIdentificando] = useState(false)
  const [infoSku, setInfoSku] = useState('')
  const [cobrando, setCobrando] = useState(false)
  const [emitida, setEmitida] = useState(null)
  const [pidiendoPin, setPidiendoPin] = useState(null) // {accion, alAutorizar}
  const [arqueando, setArqueando] = useState(false)
  const [verPrecio, setVerPrecio] = useState(false) // consulta de precio (sin tocar el carrito)
  // Pesaje de un producto por peso. null = cerrado. Al agregar/escanear un producto
  // por peso o al «Editar peso» de una línea, se llena con lo necesario para el modal.
  const [pesaje, setPesaje] = useState(null) // {sku,nombre,precioBs,exento,monedaOriginal,pesoInicial,editando} | null
  // Señal de "venta nueva": se incrementa al abrir turno, tras emitir, al vaciar,
  // al retomar una espera y tras el arqueo. Cuando la empresa exige identificar al
  // cliente (exigeCedula), el modal de identificación se ABRE SOLO en cada venta
  // nueva (es requisito, no un aviso amarillo). Ver el efecto más abajo.
  const [ventaSenal, setVentaSenal] = useState(0)
  const buscarRef = useRef(null)

  /* LISTAS DE PRECIO + CUPÓN en el mostrador (espejo de Ventas → Cotizaciones).
   * El descuento (lista y/o cupón) se HORNEA en el `precioUnitario` de cada línea
   * del carrito; el motor de totales / IVA / IGTF (calcularTotales, el servidor)
   * NO se toca: recibe precios ya ajustados y calcula sobre la base descontada.
   *
   * NOTA HONESTA (deuda técnica): lo ideal a futuro es resolver precio + cupón en
   * el SERVIDOR (EmitirFactura), para que el POS y Ventas NO puedan divergir en la
   * forma de aplicar lista/cupón. Por ahora se replica el enfoque de Ventas
   * (ajuste del precio de línea en el front, reusando lib/precio.js). */
  // Lista de precio: 'base' (precio del catálogo) o el id de una lista de VENTA
  // activa. Al elegirla, las líneas usan el precio de la lista para los SKU que
  // estén en ella (los demás, su precio base). precioListaEnBs convierte a Bs con
  // la tasa del día si la lista está en divisa.
  const listasVenta = useMemo(
    () => (db.LISTAS_PRECIO || []).filter((l) => l.tipo === 'venta' && l.activa),
    [db.LISTAS_PRECIO],
  )
  const [listaPrecio, setListaPrecio] = useState('base')
  const listaSel = listasVenta.find((l) => l.id === listaPrecio) || null
  // Precio en Bs de un producto bajo la lista elegida (o base si 'base' / no listado).
  const precioBsDe = (p) => precioListaEnBs(p, listaSel, monedaEmpresa, tasaDe)

  // Cupón de descuento. Al aplicarlo se baja el precioUnitario de las líneas por un
  // factor uniforme (así el IVA se recalcula sobre la base ya descontada, sin tocar
  // el motor de totales). `cupon` guarda el código, el descuento en Bs y los precios
  // PREVIOS por SKU, para poder quitarlo y restaurar.
  const [cuponCodigo, setCuponCodigo] = useState('')
  const [cupon, setCupon] = useState(null)
  const [cuponBusy, setCuponBusy] = useState(false)
  const [cuponErr, setCuponErr] = useState('')
  // Redondeo a 2 decimales (misma regla que round2 del backend) para no arrastrar
  // colas de flotante al bajar precios de línea por lista/cupón.
  const round2Caja = (v) => Math.round((Number(v) || 0) * 100) / 100

  // Ventas en espera (2.6).
  const espera = useEspera()
  const [dejando, setDejando] = useState(false)
  const [verEspera, setVerEspera] = useState(false)
  const [verAtajos, setVerAtajos] = useState(false)
  const retomar = (v) => {
    setCart((v.lineas || []).map((l) => ({
      sku: l.sku, nombre: l.nombre, cantidad: l.cantidad,
      precioUnitario: l.precioUnitario, exento: !!l.exento,
      ...(l.tipoVenta === 'peso' ? { tipoVenta: 'peso' } : {}),
    })))
    espera.recargar()
    resetDescuento()
    setVentaSenal((n) => n + 1)
    toast({ title: 'Venta retomada', body: v.nota || `${v.lineas?.length || 0} ítem(s)` })
  }

  const existenciaDe = (sku) => {
    const e = existencias.find((x) => x.sku === sku)
    return e ? e.cantidad : 0
  }

  const resultados = useMemo(() => {
    const term = q.trim().toLowerCase()
    if (!term) return productos.slice(0, 12)
    return productos.filter((p) =>
      p.nombre.toLowerCase().includes(term)
      || (p.sku || '').toLowerCase().includes(term)
      || (p.codigoBarras || '').includes(term)
      || (p.presentaciones || []).some((pr) => (pr.codigoBarras || '').includes(term)),
    ).slice(0, 24)
  }, [q, productos])

  const agregar = (p) => {
    // COMBO: no viaja como una línea de combo (el servidor la rechaza); se EXPLOTA
    // en una línea por componente con el precio del paquete prorrateado (IVA por
    // ítem). Cada línea explotada se fusiona por SKU igual que un alta normal.
    if (p?.esCombo) {
      const lineas = explotarCombo(p, (sku) => productos.find((x) => x.sku === sku), 1, precioBsDe)
      if (!lineas) {
        toast({ title: 'No se pudo agregar el combo', body: `Falta la tasa o algún componente de ${p.nombre}.`, kind: 'warn' })
        return
      }
      setCart((c) => {
        let next = c
        for (const nl of lineas) {
          const i = next.findIndex((l) => l.sku === nl.sku)
          if (i >= 0) next = next.map((l, j) => (j === i ? { ...l, cantidad: l.cantidad + nl.cantidad } : l))
          else next = [...next, { sku: nl.sku, nombre: nl.nombre, cantidad: nl.cantidad, precioUnitario: nl.precioUnitario, exento: nl.exento, monedaOriginal: nl.monedaOriginal, combo: nl.combo, comboNombre: nl.comboNombre }]
        }
        return next
      })
      setQ('')
      buscarRef.current?.focus()
      return
    }
    // El precio toma la lista de precio seleccionada (o el base si es 'base' o si
    // el SKU no está en la lista). La lista puede estar en divisa: precioBsDe la
    // convierte a Bs con la tasa del día.
    const bs = precioBsDe(p)
    if (bs === null) {
      const m = listaSel && itemDeLista(listaSel, p.sku) ? listaSel.moneda : monedaDe(p, monedaEmpresa)
      toast({ title: 'Falta la tasa', body: `${p.nombre} se cobra en ${m} y no hay tasa cargada.`, kind: 'warn' })
      return
    }
    if (existenciaDe(p.sku) <= 0) {
      toast({ title: 'Sin existencias', body: `${p.nombre} está agotado en esta sede.`, kind: 'warn' })
      return
    }
    // POR PESO: no se incrementa cantidad 1; se abre el modal de pesaje (balanza) y
    // la línea entra con cantidad = peso en kg. La pistola lectora que resuelve a un
    // producto por peso (Enter en el buscador) también cae acá y abre el pesaje.
    if (esPorPeso(p)) {
      setPesaje({
        sku: p.sku, nombre: p.nombre, precioBs: bs, exento: !!p.exentoIva,
        monedaOriginal: monedaDe(p, monedaEmpresa), pesoInicial: '', editando: false,
      })
      setQ('')
      return
    }
    setCart((c) => {
      const i = c.findIndex((l) => l.sku === p.sku)
      if (i >= 0) return c.map((l, j) => (j === i ? { ...l, cantidad: l.cantidad + 1 } : l))
      return [...c, { sku: p.sku, nombre: p.nombre, cantidad: 1, precioUnitario: bs, exento: !!p.exentoIva, monedaOriginal: monedaDe(p, monedaEmpresa) }]
    })
    setQ('')
    buscarRef.current?.focus()
  }

  // Cambiar de lista de precio: reaplica el precio de la nueva lista a TODAS las
  // líneas del carrito (los SKU no listados vuelven a su precio base). Si falta la
  // tasa para convertir un precio, esa línea conserva el que tenía. Un cupón
  // aplicado quedaría sobre precios que ya no existen: se auto-quita.
  const cambiarLista = (id) => {
    setListaPrecio(id)
    if (cupon) { setCupon(null); setCuponErr('') }
    const nueva = listasVenta.find((l) => l.id === id) || null
    setCart((c) => c.map((l) => {
      const p = productos.find((pr) => pr.sku === l.sku)
      if (!p) return l
      const bs = precioListaEnBs(p, nueva, monedaEmpresa, tasaDe)
      return bs === null ? l : { ...l, precioUnitario: bs }
    }))
  }

  // Aplicar un cupón: valida el código contra el subtotal (servidor, fuente de
  // verdad) y baja cada precioUnitario por un factor uniforme. Para un cupón
  // porcentual el factor es (1 − %); para uno de monto fijo, (1 − descuento/subtotal),
  // que reparte el monto proporcional al importe de cada línea. Solo uno a la vez.
  const aplicarCupon = async () => {
    const cod = cuponCodigo.trim()
    if (!cod || cupon) return
    if (cart.length === 0) { setCuponErr('Agrega productos antes de aplicar un cupón.'); return }
    setCuponBusy(true); setCuponErr('')
    const subtotal = round2Caja(cart.reduce((s, l) => s + (Number(l.cantidad) || 0) * (Number(l.precioUnitario) || 0), 0))
    try {
      const res = await api.validarCupon({ codigo: cod, subtotal })
      const desc = Math.min(Number(res?.descuentoBs) || 0, subtotal)
      const factor = subtotal > 0 ? 1 - desc / subtotal : 1
      const previos = {}
      for (const l of cart) previos[l.sku] = l.precioUnitario
      setCart((c) => c.map((l) => ({ ...l, precioUnitario: round2Caja((Number(l.precioUnitario) || 0) * factor) })))
      setCupon({ codigo: res?.cupon?.codigo || cod.toUpperCase(), tipo: res?.cupon?.tipo, descuentoBs: round2Caja(desc), previos })
      setCuponCodigo('')
    } catch (e) {
      setCuponErr(e?.message || 'Cupón inválido')
    }
    setCuponBusy(false)
  }

  // Quitar el cupón: restaura los precioUnitario previos de las líneas que sigan
  // presentes (las agregadas después conservan su precio de lista/base).
  const quitarCupon = () => {
    if (!cupon) return
    setCart((c) => c.map((l) => (cupon.previos[l.sku] != null ? { ...l, precioUnitario: cupon.previos[l.sku] } : l)))
    setCupon(null)
    setCuponErr('')
  }

  // Reset del descuento del mostrador: al vaciar/retomar/emitir/cerrar caja, el
  // cupón y la lista no deben arrastrarse a la siguiente venta.
  const resetDescuento = () => { setCupon(null); setCuponCodigo(''); setCuponErr(''); setListaPrecio('base') }

  // Quitar una línea o vaciar el carrito es lo que el flujo 2.4 protege con el
  // PIN de supervisor: es la vía por la que se «desaparece» una venta.
  const conAutorizacion = (accion, hacer) => {
    if (!requierePin) { hacer(); return }
    setPidiendoPin({ accion, alAutorizar: hacer })
  }

  // Igual que conAutorizacion, pero cuando la empresa NO exige PIN pide una
  // confirmación explícita (acción destructiva): vaciar el carrito o salir.
  const conAutorizacionOConfirm = async (accion, opts, hacer) => {
    if (requierePin) { setPidiendoPin({ accion, alAutorizar: hacer }); return }
    if (await confirm(opts)) hacer()
  }

  const quitar = (sku) => conAutorizacion('quitar línea del carrito', () => setCart((c) => c.filter((l) => l.sku !== sku)))
  const vaciar = () => conAutorizacionOConfirm('vaciar el carrito', {
    title: '¿Vaciar el carrito?',
    body: 'Se quitarán todos los productos de esta venta en curso. No se puede deshacer.',
    confirmLabel: 'Vaciar carrito', tone: 'danger',
  }, () => { setCart([]); setCliente(null); resetDescuento(); setVentaSenal((n) => n + 1) })
  const cambiarCantidad = (sku, delta) => {
    const linea = cart.find((l) => l.sku === sku)
    if (!linea) return
    if (linea.cantidad + delta <= 0) { quitar(sku); return }
    setCart((c) => c.map((l) => (l.sku === sku ? { ...l, cantidad: l.cantidad + delta } : l)))
  }

  // PESAJE. Confirmar el modal agrega (o corrige) una línea de producto por peso con
  // cantidad = peso en kg. Editando: reemplaza el peso de la línea. Nueva: si la
  // línea ya existe suma el peso (otra porción del mismo producto); si no, la crea.
  // El motor de totales no cambia: total de línea = cantidad(kg) × precioUnitario(Bs/kg).
  const confirmarPesaje = (kg) => {
    const pj = pesaje
    const peso = round3(kg)
    if (!pj || !(peso > 0)) return
    if (pj.editando) {
      setCart((c) => c.map((l) => (l.sku === pj.sku ? { ...l, cantidad: peso } : l)))
    } else {
      setCart((c) => {
        const i = c.findIndex((l) => l.sku === pj.sku)
        if (i >= 0) return c.map((l, j) => (j === i ? { ...l, cantidad: round3(l.cantidad + peso) } : l))
        return [...c, {
          sku: pj.sku, nombre: pj.nombre, cantidad: peso, precioUnitario: pj.precioBs,
          exento: pj.exento, monedaOriginal: pj.monedaOriginal, tipoVenta: 'peso',
        }]
      })
    }
    setPesaje(null)
    buscarRef.current?.focus()
  }
  // Reabrir el pesaje de una línea por peso para corregir el peso. Usa el precio de
  // la línea (ya refleja lista de precio / cupón) y prefila el peso actual.
  const editarPeso = (l) => setPesaje({
    sku: l.sku, nombre: l.nombre, precioBs: l.precioUnitario,
    exento: !!l.exento, monedaOriginal: l.monedaOriginal, pesoInicial: l.cantidad, editando: true,
  })

  const salir = () => conAutorizacionOConfirm('salir del modo caja', {
    title: '¿Salir del modo caja?',
    body: 'Volverás a la vista normal de la aplicación. Si hay una venta sin cobrar en el carrito, se perderá.',
    confirmLabel: 'Salir', tone: 'danger',
  }, () => onSalir?.())

  // Cerrar el turno desde el propio puesto: el cajero termina su jornada acá, no
  // navegando a otra pantalla que no tiene. Se abre el ARQUEO (cuadre de la
  // gaveta); al confirmar se cierra el turno. Con carrito lleno el modal avisa
  // que la venta sin cobrar se perderá.
  const cerrarTurno = () => setArqueando(true)
  const trasArqueo = async () => {
    setCart([])
    setCliente(null)
    resetDescuento()
    setVentaSenal((n) => n + 1)
    await recargar()
  }

  const totales = useMemo(() => calcularTotales(cart, [], tasa.valor), [cart, tasa.valor])
  const zurdo = !!ui.zurdo
  // Bloqueo de cobro por cédula: la empresa exige cliente y aún es Consumidor final.
  const faltaCliente = exigeCedula && !cliente

  /* EXIGIR CÉDULA — modal abierto, no aviso amarillo. Cuando la empresa exige
   * identificar al cliente, el modal de identificación se ABRE SOLO al comenzar
   * cada venta nueva (señalizada por `ventaSenal`: turno abierto, tras emitir, al
   * vaciar, al retomar y tras el arqueo) mientras no haya cliente. El cajero puede
   * cerrarlo (p. ej. para consultar un precio) — el cobro queda bloqueado hasta
   * identificar, y el modal reaparece en la siguiente venta. En cada disparo la
   * venta está recién reseteada, así que `cliente` es null: no reabre al cambiar
   * de cliente en mitad de una venta. */
  useEffect(() => {
    if (exigeCedula && sesion && !cliente) setIdentificando(true)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [exigeCedula, sesion?.id, ventaSenal])

  /* PANTALLA DEL CLIENTE — segunda ventana en la misma máquina, sincronizada por
   * BroadcastChannel (sin servidor). Se publica el carrito + totales en cada
   * cambio; el modal de cobro publica el estado del cobro por su cuenta. Cuando
   * el display se abre después, saluda ('hello') y la caja reemite su estado. */
  const pantallaClienteRef = useRef(null)
  const publicarPos = usePosCanal((m) => { if (m?.tipo === 'hello') { emitirDisplay(); emitirBranding() } })

  // TEMA DE LA PANTALLA DEL CLIENTE. La empresa es la fuente (fondos por espacio,
  // texto, énfasis y versión de logo, en `empresa.temaPantalla`); la caja del turno
  // puede sobreescribir el CONTRASTE: el fondo general y la versión de logo. Se busca
  // la caja del turno abierto para leer su override; sin turno, o mientras carga, el
  // display cae al tema de la empresa (db.EMPRESA).
  const [cajaActual, setCajaActual] = useState(null)
  useEffect(() => {
    let cancel = false
    if (!sesion?.cajaId) { setCajaActual(null); return undefined }
    api.cajas().then((cs) => { if (!cancel) setCajaActual((cs || []).find((c) => c.id === sesion.cajaId) || null) }).catch(() => {})
    return () => { cancel = true }
  }, [sesion?.cajaId])

  const tema = useMemo(() => {
    const emp = db.EMPRESA || {}
    // Override por caja para contraste. La versión de logo previa (principal|alterno)
    // se mapea al modelo nuevo (color|blanco): 'alterno' era el logo de contraste.
    const override = {}
    if (cajaActual?.colorFondo) override.fondo = cajaActual.colorFondo
    if (cajaActual?.logoVersion === 'alterno') override.logoVersion = 'blanco'
    else if (cajaActual?.logoVersion === 'principal') override.logoVersion = 'color'
    return resolverTema(emp, override)
  }, [db.EMPRESA, cajaActual])

  // Mensaje 'branding' independiente de 'estado'/'cobro'/'emitida': el display no
  // conoce la caja, así que la caja le dicta el TEMA ya resuelto.
  const emitirBranding = () => publicarPos({ tipo: 'branding', tema })
  useEffect(() => { emitirBranding() }, [tema])

  const emitirDisplay = () => {
    if (emitida) {
      publicarPos({ tipo: 'emitida', doc: {
        numeroCompleto: emitida.numeroCompleto, total: emitida.total,
        vuelto: emitida.vuelto, vueltoMoneda: emitida.vueltoMoneda,
        // Desglose del vuelto MIXTO (para el «gracias» de la pantalla del cliente).
        vueltoPartes: emitida.vueltoPartes,
      } })
      return
    }
    publicarPos({
      tipo: 'estado',
      // Se incluye tipoVenta/unidad para que la pantalla del cliente muestre las
      // líneas por peso con su unidad (kg) y «/kg» sin romper las líneas por unidad.
      carrito: cart.map((l) => ({ sku: l.sku, nombre: l.nombre, cantidad: l.cantidad, precioUnitario: l.precioUnitario, exento: !!l.exento, tipoVenta: l.tipoVenta === 'peso' ? 'peso' : 'unidad', unidad: l.tipoVenta === 'peso' ? 'kg' : 'u' })),
      totales: { subtotal: totales.subtotal, baseImponible: totales.baseImponible, baseExenta: totales.baseExenta, iva: totales.iva, total: totales.total },
      cliente: cliente ? { nombre: cliente.nombre } : null,
    })
  }
  useEffect(() => { emitirDisplay() }, [cart, totales, cliente, emitida])

  // Abrir la pantalla del cliente (misma app/origen ⇒ comparte sesión). Se
  // recuerda la preferencia POR DISPOSITIVO; los navegadores bloquean reabrir
  // sin un gesto, así que al entrar a la caja se ofrece reabrirla.
  const abrirPantallaCliente = () => {
    const url = `${window.location.origin}${window.location.pathname}#pantalla-cliente`
    const w = window.open(url, 'huberp-pantalla-cliente', 'width=1100,height=760')
    if (w) {
      pantallaClienteRef.current = w
      try { localStorage.setItem('huberp:pantalla-cliente', '1') } catch { /* almacenamiento no disponible */ }
      w.focus()
      // Cuando la ventana termine de cargar saludará; igual reemitimos el estado.
      setTimeout(emitirDisplay, 800)
    }
  }
  useEffect(() => {
    let activa = false
    try { activa = localStorage.getItem('huberp:pantalla-cliente') === '1' } catch { activa = false }
    if (activa) toast({ title: 'Pantalla del cliente', body: 'Estaba activa en este equipo. Pulsa «Pantalla cliente» para reabrirla.' })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Atajos del puesto. Se evitan las teclas F SOLAS como atajo principal: en muchos
  // laptops F2/F8 son teclas multimedia (brillo/volumen) que requieren Fn, y algunos
  // navegadores atrapan otras F —así que "chocan". Los atajos primarios usan teclas
  // que SIEMPRE llegan a la app ("/" y Ctrl/⌘+Enter); las F quedan como alias por si
  // el teclado (p. ej. uno físico de POS) las manda tal cual.
  useEffect(() => {
    const enCampo = (t) => !!t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)
    const onKey = (e) => {
      // Buscar producto: "/" (desde cualquier lado, sin escribir la barra) o F2.
      if ((e.key === '/' && !enCampo(e.target)) || e.key === 'F2') { e.preventDefault(); buscarRef.current?.focus() }
      // Cobrar: Ctrl/⌘ + Enter (fiable en todo teclado) o F8. Requiere carrito.
      if (((e.key === 'Enter' && (e.ctrlKey || e.metaKey)) || e.key === 'F8') && cart.length && !faltaCliente) { e.preventDefault(); setCobrando(true) }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [cart.length, faltaCliente])

  /* TERMINAL DEL COBRO EN EL POS. En la caja la factura va a la IMPRESORA FISCAL,
   * no a un PDF: por eso tras emitir NO se muestra la pantalla completa «¡Factura
   * emitida!» (esa es de la facturación forma libre del módulo Ventas). Se muestra
   * SOLO el modal de impresión fiscal —con sus estados y el vuelto a entregar— y
   * al cerrarlo (imprimió, reintentó, o continuó sin imprimir) se vuelve directo a
   * una venta nueva en blanco. Nada de esto re-emite: el `Documento` ya existe. */
  if (emitida) {
    const dispositivoFiscal = impresoraActivaDeSede(db.DISPOSITIVOS, db.SEDE_ACTIVA?.id)
    const nuevaVenta = () => { setEmitida(null); setCart([]); setCliente(null); resetDescuento(); setVentaSenal((n) => n + 1); buscarRef.current?.focus() }
    return (
      <div className="min-h-screen bg-slate-50 dark:bg-slate-950">
        <ImpresionFiscalModal open canal="caja" doc={emitida} dispositivo={dispositivoFiscal} onCerrar={nuevaVenta} />
      </div>
    )
  }

  // Wash sutil del color de fondo de la caja en el mostrador: un degradado tenue
  // arriba que no toca la legibilidad del contenido (solo si la caja lo configuró).
  const brandStyle = tema.fondo
    ? { backgroundImage: `linear-gradient(180deg, ${tema.fondo}22, transparent 260px)` }
    : undefined

  return (
    <div className="min-h-screen flex flex-col bg-slate-50 dark:bg-slate-950 text-slate-900 dark:text-slate-100" style={brandStyle}>
      {/* Cabecera propia del modo caja */}
      <header className="sticky top-0 z-30 flex items-center gap-3 px-4 sm:px-6 py-3 bg-white dark:bg-slate-900 border-b border-slate-200 dark:border-slate-800"
        style={tema.enfasis ? { borderBottomColor: tema.enfasis } : undefined}>
        {/* La marca también en el puesto de cobro: es la pantalla que el cliente
            ve desde el otro lado del mostrador. El logo de la caja (versión elegida)
            acompaña al wordmark cuando la empresa cargó uno. */}
        <Wordmark size={26} className="shrink-0 mr-1 hidden sm:flex" />
        {tema.logo ? (
          <img src={tema.logo} alt="" className="h-7 w-auto object-contain shrink-0 hidden sm:block"
            onError={(e) => { e.currentTarget.style.display = 'none' }} />
        ) : null}
        <div className="h-10 w-10 shrink-0 rounded-full bg-teal-50 dark:bg-teal-500/15 text-teal-600 dark:text-teal-300 inline-flex items-center justify-center font-bold">
          {(sesion?.cajeroNombre || user?.nombre || '?').slice(0, 1).toUpperCase()}
        </div>
        <div className="min-w-0">
          <div className="text-[15px] font-semibold truncate">{sesion?.cajeroNombre || user?.nombre || 'Caja'}</div>
          <div className="text-[12.5px] text-slate-500 truncate">
            {sesion ? <>caja <span className="mono">{sesion.cajaCodigo}</span></> : 'sin turno abierto'}
            {sedeNombre ? ` · ${sedeNombre}` : ''}
          </div>
        </div>
        <div className="flex-1" />
        {tasa.hay ? (
          <div className="hidden md:block text-right leading-tight">
            <div className="text-[12.5px] num">Bs {fmtNum(tasa.valor, 2)} / US$</div>
            <div className="text-[10.5px] text-slate-400">{tasa.fuenteLabel} · {fechaCortaVE(tasa.fechaValor, tasa.esDeHoy)}</div>
          </div>
        ) : (
          <div className="hidden md:block text-[12px] text-amber-600 dark:text-amber-400">Sin tasa del día</div>
        )}
        {/* Verificar precio: consulta rápida del precio/disponibilidad de un
            producto que el cliente pregunta, SIN tocar el carrito ni salir de la
            venta en curso. Respeta la lista de precio seleccionada. */}
        <BotonCaja onClick={() => setVerPrecio(true)}
          title="Consultar el precio de un producto sin agregarlo al carrito">
          <Icon.Tag size={16} /> Verificar precio
        </BotonCaja>
        {/* Ventas en espera (2.6): acceso SIEMPRE visible en la cabecera, con la
            burbuja del contador — así el cajero sabe que el lugar existe aunque
            esté vacío. Cualquier cajero de la sede puede retomarlas para cobrar. */}
        <BotonCaja onClick={() => setVerEspera(true)}
          title="Ventas en espera — cualquier cajero puede retomarlas para cobrar">
          <Icon.Clock size={16} /> En espera
          <span className={`inline-flex items-center justify-center min-w-[19px] h-[19px] px-1 rounded-full text-[11px] font-bold ${
            espera.lista.length ? 'bg-elerp-500 text-white' : 'bg-slate-200 dark:bg-slate-700 text-slate-500 dark:text-slate-400'}`}>
            {espera.lista.length}
          </span>
        </BotonCaja>
        <BotonCaja onClick={() => setVerAtajos(true)} title="Ver los atajos de teclado del puesto">
          <Icon.CircleAlert size={16} /> Atajos
        </BotonCaja>
        {/* Pantalla del cliente: segunda ventana orientada al cliente que refleja
            en vivo el carrito, los totales y el vuelto (misma máquina). */}
        <BotonCaja onClick={abrirPantallaCliente}
          title="Abrir la pantalla que ve el cliente en una segunda ventana">
          <Icon.Maximize size={16} /> Pantalla cliente
        </BotonCaja>
        <BotonCaja onClick={() => setUi((u) => ({ ...u, zurdo: !u.zurdo }))}
          title="Cambiar el lado del carrito (zurdo / diestro)">
          <span className={zurdo ? 'scale-x-[-1] inline-flex' : 'inline-flex'}><Icon.Hand size={17} /></span>
          {zurdo ? 'Zurdo' : 'Diestro'}
        </BotonCaja>
        <BotonCaja onClick={() => setUi((u) => ({ ...u, dark: !u.dark }))} title="Modo claro / oscuro">
          {ui.dark ? <Icon.Sun size={16} /> : <Icon.Moon size={16} />}
          {ui.dark ? 'Claro' : 'Oscuro'}
        </BotonCaja>
        {sesion ? (
          <BotonCaja tone="danger" onClick={cerrarTurno} title="Cerrar el turno y hacer el arqueo de la caja">
            <Icon.Lock size={16} /> Cerrar caja
          </BotonCaja>
        ) : null}
        <BotonCaja tone="danger" onClick={salir} title="Volver a la vista normal">
          <Icon.Minimize size={16} /> Salir
        </BotonCaja>
      </header>

      {/* Sin turno: lo único que se puede hacer es abrir la caja */}
      {!cargando && !sesion ? (
        <div className="flex-1 flex items-center justify-center p-6">
          <div className="max-w-sm text-center">
            <div className="h-14 w-14 rounded-full bg-elerp-50 dark:bg-elerp-900/40 text-elerp-500 inline-flex items-center justify-center mb-3">
              <Icon.Lock size={26} />
            </div>
            <div className="font-display font-semibold text-[18px]">Abre tu caja para empezar</div>
            <p className="text-[13.5px] text-slate-500 mt-2">
              Identifícate con tu código y tu PIN. La caja queda a tu nombre hasta el cierre, y cada
              cobro del turno queda registrado con tu nombre.
            </p>
            <Button className="mt-5" size="lg" icon={<Icon.Lock size={17} />} onClick={() => setAbrir(true)}>Abrir caja</Button>
          </div>
        </div>
      ) : (
        <>
        {/* Con "exigir cédula" NO hay aviso amarillo al inicio: el modal de
            identificación se abre solo en cada venta nueva (ver el efecto de
            ventaSenal arriba). El cobro queda bloqueado hasta identificar. */}
        <div className={`flex-1 grid gap-5 p-4 sm:p-6 items-start
          ${zurdo ? 'lg:grid-cols-[2fr_3fr]' : 'lg:grid-cols-[3fr_2fr]'}`}>
          {/* Productos */}
          <div className={`min-w-0 ${zurdo ? 'lg:order-2' : ''}`}>
            <div className="flex items-center gap-3">
            <div className="flex-1 flex items-center gap-3 rounded-2xl bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 px-4">
              <Icon.Search size={19} className="text-slate-400 shrink-0" />
              <input ref={buscarRef} value={q} onChange={(e) => setQ(e.target.value)} autoFocus
                onKeyDown={(e) => {
                  if (e.key !== 'Enter') return
                  // La pistola lectora termina con Enter: primero se busca el
                  // código exacto, y solo si no existe se toma el primer resultado.
                  const exacto = porCodigo(productos, q)
                  if (exacto) { agregar(exacto); return }
                  if (resultados.length) agregar(resultados[0])
                }}
                placeholder="Buscar o escanear código…  ( / )"
                className="flex-1 bg-transparent outline-none py-4 text-[16px]" />
              {q ? (
                <button onClick={() => setQ('')} className="text-slate-400 hover:text-slate-600 p-1"><Icon.X size={18} /></button>
              ) : null}
            </div>
            </div>

            {resultados.length === 0 ? (
              <div className="mt-5 rounded-2xl border border-dashed border-slate-300 dark:border-slate-700 py-14 text-center text-slate-500 text-[15px]">
                No encontramos «{q}». Revisa el nombre o el código.
              </div>
            ) : (
              <div className="mt-5 grid gap-4 grid-cols-2 sm:grid-cols-3 xl:grid-cols-4">
                {resultados.map((p) => {
                  // Un combo no tiene existencia propia (no está en el ledger): nunca
                  // se marca agotado; su venta explota en los componentes.
                  const esCombo = !!p.esCombo
                  const cant = existenciaDe(p.sku)
                  const st = nivelStock(cant)
                  const agotado = !esCombo && cant <= 0
                  // El precio de la tarjeta refleja la lista seleccionada (o base).
                  const bs = precioBsDe(p)
                  return (
                    <div key={p.id} className="relative rounded-2xl overflow-hidden bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 shadow-card">
                      <button onClick={(e) => { e.stopPropagation(); setInfoSku(p.sku) }}
                        title="Disponibilidad por sede"
                        className="absolute top-2.5 right-2.5 z-10 h-8 w-8 rounded-full bg-white/90 dark:bg-slate-900/90 backdrop-blur text-slate-500 hover:text-elerp-500 inline-flex items-center justify-center shadow-card">
                        <Icon.CircleAlert size={16} />
                      </button>
                      <button onClick={() => agregar(p)} disabled={agotado}
                        className={`block w-full text-left ${agotado ? 'opacity-60 cursor-not-allowed' : 'active:scale-[.99]'}`}>
                        <ImagenProducto producto={p} alto={124} />
                        <div className="p-3.5">
                          <div className="text-[14.5px] font-semibold leading-tight line-clamp-2 min-h-[38px] flex items-start gap-1.5">
                            <span className="min-w-0">{p.nombre}</span>
                            {esCombo ? <Badge size="sm" color="huberp">Combo</Badge> : null}
                          </div>
                          <div className="mono text-[10.5px] text-slate-400 mt-1">
                            {p.codigoBarras || p.sku}{p.exentoIva ? ' · exento' : ''}
                          </div>
                          <div className="flex items-baseline gap-2 mt-1.5">
                            <span className="text-[18px] font-bold num">
                              {bs === null ? 'sin tasa' : fmtCurrency(bs, 'VES')}
                              {bs !== null && esPorPeso(p) ? <span className="text-[12px] font-semibold text-slate-400"> /kg</span> : null}
                            </span>
                            <span className={`text-[12px] num ${esCombo ? 'text-slate-400' : st.clase}`}>
                              {esCombo ? `${(p.componentes || []).length} ítems` : (agotado ? 'Agotado' : `${fmtNum(cant, 2)} disp.`)}
                            </span>
                          </div>
                        </div>
                      </button>
                    </div>
                  )
                })}
              </div>
            )}
          </div>

          {/* Carrito */}
          <div className={`lg:sticky lg:top-[86px] rounded-2xl bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 shadow-card overflow-hidden ${zurdo ? 'lg:order-1' : ''}`}>
            <div className="flex items-center gap-2.5 px-4 py-3.5 border-b border-slate-200 dark:border-slate-800">
              <Icon.Cart size={19} />
              <span className="text-[16px] font-semibold flex-1">Carrito</span>
              {cart.length ? (
                <button onClick={vaciar} className="text-[13px] font-semibold text-slate-500 hover:text-slate-700 dark:hover:text-slate-300 px-2 py-1 rounded-lg">Vaciar</button>
              ) : null}
            </div>

            {cart.length === 0 ? (
              <div className="py-12 px-6 text-center text-slate-500">
                <Icon.Cart size={30} className="mx-auto text-slate-300 dark:text-slate-600" />
                <div className="text-[14.5px] mt-3">Toca un producto para agregarlo</div>
              </div>
            ) : (
              <div className="max-h-[42vh] overflow-y-auto">
                {cart.map((l) => {
                  const peso = l.tipoVenta === 'peso'
                  return (
                  <div key={l.sku} className="flex items-center gap-3 px-4 py-3.5 border-b border-slate-100 dark:border-slate-800">
                    <span className="flex-1 min-w-0">
                      <span className="block text-[14.5px] font-semibold truncate">{l.nombre}</span>
                      <span className="block text-[12.5px] text-slate-500 num">
                        {fmtCurrency(l.precioUnitario, 'VES')} {peso ? '/kg' : 'c/u'}{l.exento ? ' · exento' : ''}
                      </span>
                    </span>
                    {peso ? (
                      // POR PESO: los +/− no aplican. El peso (kg, 3 decimales) se
                      // corrige reabriendo el pesaje con «Editar peso».
                      <button onClick={() => editarPeso(l)} title="Editar el peso (balanza)"
                        className="flex items-center gap-1.5 shrink-0 h-9 px-2.5 rounded-xl border border-slate-200 dark:border-slate-700 hover:bg-slate-50 dark:hover:bg-slate-800">
                        <span className="text-[14.5px] font-bold num">{fmtNum(l.cantidad, 3)} kg</span>
                        <Icon.Pencil size={13} className="text-slate-400" />
                      </button>
                    ) : (
                      <span className="flex items-center gap-1 shrink-0">
                        <button onClick={() => cambiarCantidad(l.sku, -1)}
                          className="h-9 w-9 rounded-xl border border-slate-200 dark:border-slate-700 text-[17px] font-semibold inline-flex items-center justify-center">−</button>
                        <span className="min-w-[46px] text-center text-[14.5px] font-bold num">{fmtNum(l.cantidad, l.cantidad % 1 ? 2 : 0)}</span>
                        <button onClick={() => cambiarCantidad(l.sku, 1)}
                          className="h-9 w-9 rounded-xl border border-slate-200 dark:border-slate-700 text-[17px] font-semibold inline-flex items-center justify-center">+</button>
                      </span>
                    )}
                    <span className="min-w-[92px] text-right text-[14.5px] font-bold num shrink-0">
                      {fmtCurrency(l.cantidad * l.precioUnitario, 'VES')}
                    </span>
                    <button onClick={() => quitar(l.sku)} title="Quitar"
                      className="h-9 w-9 rounded-xl text-slate-400 hover:text-red-500 inline-flex items-center justify-center shrink-0">
                      <Icon.X size={17} />
                    </button>
                  </div>
                  )
                })}
              </div>
            )}

            {/* Cliente de la venta. Por defecto Consumidor final; identificarlo
                habilita la venta a crédito (el CobroModal exige clienteId). Cuando la
                empresa exige cédula y aún no hay cliente, el bloque se resalta. */}
            <div className={`px-4 py-3 border-t border-slate-200 dark:border-slate-800 ${
              faltaCliente ? 'bg-elerp-50/60 dark:bg-elerp-900/15 ring-1 ring-inset ring-elerp-200 dark:ring-elerp-800/60' : ''}`}>
              {cliente ? (
                <div className="flex items-center gap-2.5">
                  <div className="h-9 w-9 shrink-0 rounded-full bg-elerp-50 dark:bg-elerp-900/40 text-elerp-500 inline-flex items-center justify-center">
                    <Icon.User size={17} />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="text-[14px] font-semibold truncate">{cliente.nombre}</div>
                    <div className="text-[12px] text-slate-500 num truncate">{cliente.tipoDocumento}-{cliente.documento}</div>
                  </div>
                  <button onClick={() => setIdentificando(true)}
                    className="text-[12.5px] font-semibold text-elerp-600 hover:text-elerp-700 px-2 py-1 rounded-lg">Cambiar</button>
                  <button onClick={() => setCliente(null)} title="Quitar cliente (volver a Consumidor final)"
                    className="h-8 w-8 rounded-lg text-slate-400 hover:text-red-500 inline-flex items-center justify-center shrink-0">
                    <Icon.X size={16} />
                  </button>
                </div>
              ) : (
                <button onClick={() => setIdentificando(true)}
                  className="w-full flex items-center gap-2.5 text-left group">
                  <div className="h-9 w-9 shrink-0 rounded-full bg-slate-100 dark:bg-slate-800 text-slate-400 inline-flex items-center justify-center">
                    <Icon.User size={17} />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="text-[14px] font-semibold text-slate-600 dark:text-slate-300">Consumidor final</div>
                    <div className="text-[12px] text-elerp-600 group-hover:text-elerp-700 font-semibold inline-flex items-center gap-1">
                      <Icon.Search size={12} /> Identificar cliente
                    </div>
                  </div>
                </button>
              )}
            </div>

            {/* Lista de precio + cupón del mostrador. El descuento se hornea en el
                precioUnitario de las líneas (ver bloque LISTAS DE PRECIO + CUPÓN
                arriba); el total a cobrar y lo que se emite ya lo reflejan, sin
                tocar el motor fiscal. La lista se ofrece aunque el carrito esté
                vacío (fija el precio de lo que se agregue); el cupón, solo con
                líneas. */}
            {listasVenta.length || cart.length ? (
              <div className="px-4 py-3 border-t border-slate-200 dark:border-slate-800 space-y-2.5">
                {listasVenta.length ? (
                  <div className="flex items-center gap-2">
                    <Icon.Tag size={15} className="text-slate-400 shrink-0" />
                    <span className="text-[12.5px] text-slate-500 shrink-0">Lista de precio</span>
                    <Select className="!h-9 text-[12.5px] flex-1" value={listaPrecio} onChange={(e) => cambiarLista(e.target.value)}>
                      <option value="base">Precio base</option>
                      {listasVenta.map((l) => <option key={l.id} value={l.id}>{l.nombre} ({l.moneda})</option>)}
                    </Select>
                  </div>
                ) : null}
                {cart.length ? (
                  cupon ? (
                    <div className="flex items-center gap-2 rounded-lg bg-teal-50 dark:bg-teal-500/10 border border-teal-200 dark:border-teal-800 px-3 py-1.5">
                      <Icon.Star size={14} className="text-teal-500 shrink-0" />
                      <div className="flex-1 min-w-0">
                        <div className="text-[12.5px] font-semibold num tracking-wide text-teal-800 dark:text-teal-300 truncate">{cupon.codigo}</div>
                        <div className="text-[11px] text-teal-700/80 dark:text-teal-400/80 num">−{fmtCurrency(cupon.descuentoBs, 'VES')} aplicado a las líneas</div>
                      </div>
                      <button type="button" onClick={quitarCupon} title="Quitar el cupón"
                        className="h-7 w-7 inline-flex items-center justify-center rounded-lg text-teal-700 dark:text-teal-300 hover:bg-teal-100 dark:hover:bg-teal-500/20"><Icon.X size={14} /></button>
                    </div>
                  ) : (
                    <div>
                      <div className="flex items-center gap-2">
                        <Input className="flex-1 num tracking-wide uppercase !h-9" placeholder="Cupón de descuento" value={cuponCodigo}
                          onChange={(e) => { setCuponCodigo(e.target.value); setCuponErr('') }}
                          onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); aplicarCupon() } }} />
                        <Button variant="secondary" size="sm" onClick={aplicarCupon} loading={cuponBusy} disabled={!cuponCodigo.trim() || cart.length === 0}>Aplicar</Button>
                      </div>
                      {cuponErr ? <div className="mt-1.5 text-[11.5px] text-red-600 dark:text-red-400 flex items-start gap-1"><Icon.CircleAlert size={13} className="mt-0.5 shrink-0" /><span>{cuponErr}</span></div> : null}
                    </div>
                  )
                ) : null}
              </div>
            ) : null}

            <div className="px-4 py-4 border-t border-slate-200 dark:border-slate-800 space-y-2">
              <Fila label="Base imponible" valor={totales.baseImponible} />
              {totales.baseExenta > 0 ? <Fila label="Exento de IVA" valor={totales.baseExenta} /> : null}
              <Fila label={`IVA ${IVA_TASA * 100}%`} valor={totales.iva} />
              <div className="flex items-center justify-between text-[22px] font-extrabold pt-1.5">
                <span>Total</span><span className="num">{fmtCurrency(totales.total, 'VES')}</span>
              </div>
              <div className="flex gap-2.5 mt-2">
                <Button size="lg" variant="secondary" disabled={cart.length === 0}
                  onClick={() => setDejando(true)} icon={<Icon.Clock size={17} />}
                  title="Apartar este carrito y atender al siguiente">Espera</Button>
                <Button className="flex-1 justify-center" size="lg" variant="dinero"
                  disabled={cart.length === 0 || faltaCliente} onClick={() => setCobrando(true)} icon={<Icon.Banknote size={18} />}
                  title={faltaCliente ? 'Identificá al cliente para cobrar' : undefined}>
                  Cobrar {fmtCurrency(totales.total, 'VES')}
                </Button>
              </div>
              {faltaCliente ? (
                <button onClick={() => setIdentificando(true)}
                  className="text-[11.5px] text-elerp-600 dark:text-elerp-300 text-center font-semibold inline-flex items-center justify-center gap-1.5 w-full ring-focus rounded-lg py-0.5">
                  <Icon.User size={13} /> Identifica al cliente para cobrar
                </button>
              ) : (
                <div className="text-[11px] text-slate-400 text-center">Ctrl/⌘+Enter para cobrar · / para buscar</div>
              )}
            </div>
          </div>
        </div>
        </>
      )}

      <AbrirCajaModal open={abrir} onClose={() => setAbrir(false)} onAbierta={recargar} />
      <DisponibilidadModal sku={infoSku} open={!!infoSku} onClose={() => setInfoSku('')} />
      <CobroModal open={cobrando} onClose={() => setCobrando(false)}
        lineas={cart} clienteId={cliente?.id || ''} clienteNombre={cliente?.nombre || 'Consumidor final'} contingencia={false}
        cuponCodigo={cupon?.codigo || ''}
        onEmitida={async (doc) => {
          setCobrando(false)
          setEmitida(doc)
          setCliente(null)
          await reload()
        }} />
      <SelectorClienteModal open={identificando} onClose={() => setIdentificando(false)}
        onSeleccionar={(c) => { setCliente(c); setIdentificando(false) }} />
      <VerificarPrecioModal open={verPrecio} onClose={() => setVerPrecio(false)}
        productos={productos} precioBsDe={precioBsDe} existenciaDe={existenciaDe}
        listaSel={listaSel} monedaEmpresa={monedaEmpresa} />
      <PesajeModal pesaje={pesaje} balanza={balanza}
        onAceptar={confirmarPesaje} onClose={() => setPesaje(null)} />
      <DejarEnEsperaModal open={dejando} onClose={() => setDejando(false)} lineas={cart} clienteId=""
        onGuardada={() => { setCart([]); espera.recargar() }} />
      <EsperaModal open={verEspera} onClose={() => setVerEspera(false)} lista={espera.lista}
        onRetomada={retomar} onCambio={espera.recargar} />
      <PinSupervisorModal pedido={pidiendoPin} onCerrar={() => setPidiendoPin(null)} />
      {sesion ? (
        <ArqueoModal open={arqueando} sesion={sesion} avisoCarrito={cart.length > 0}
          onClose={() => setArqueando(false)} onCerrada={trasArqueo} />
      ) : null}
      <AtajosModal open={verAtajos} onClose={() => setVerAtajos(false)} />
    </div>
  )
}

/* Selector de cliente del puesto de cobro. El cajero teclea la cédula/RIF; si el
 * cliente existe lo selecciona, y si no, lo registra en el acto (formulario
 * rápido) y queda seleccionado. Sin cliente la venta es a Consumidor final; con
 * cliente se habilita el cobro a crédito. */
function SelectorClienteModal({ open, onClose, onSeleccionar }) {
  const { db, reload } = useData()
  const toast = useToast()
  const [tipo, setTipo] = useState('V')
  const [doc, setDoc] = useState('')
  const [modo, setModo] = useState('buscar') // 'buscar' | 'registrar'
  // Formulario de alta rápida (se prellena con lo tecleado).
  const [nombre, setNombre] = useState('')
  const [tel, setTel] = useState('')
  const [dir, setDir] = useState('')
  const [email, setEmail] = useState('')
  const [touched, setTouched] = useState(false)
  const [busy, setBusy] = useState(false)

  // Al abrir/cerrar se limpia todo el estado.
  useEffect(() => {
    if (open) return
    setTipo('V'); setDoc(''); setModo('buscar')
    setNombre(''); setTel(''); setDir(''); setEmail(''); setTouched(false); setBusy(false)
  }, [open])

  const term = doc.trim().toLowerCase()
  const resultados = useMemo(() => {
    const lista = db.CLIENTES || []
    if (!term) return []
    return lista.filter((c) =>
      (c.documento || '').toLowerCase().includes(term)
      || (c.nombre || '').toLowerCase().includes(term),
    ).slice(0, 8)
  }, [db.CLIENTES, term])

  const exacto = (db.CLIENTES || []).some(
    (c) => c.tipoDocumento === tipo && (c.documento || '').trim() === doc.trim(),
  )

  const rifCheck = tipo === 'P' ? { valid: true, msg: '' } : validarRIF(`${tipo}${doc}`)
  const emailOk = !email.trim() || /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim())
  const errs = {
    nombre: !nombre.trim() ? 'Ingresa el nombre y apellido.' : '',
    documento: !doc.trim() ? 'Ingresa el documento.' : !rifCheck.valid ? rifCheck.msg : '',
    email: emailOk ? '' : 'El correo no tiene un formato válido.',
  }
  const puedeRegistrar = !errs.nombre && !errs.documento && !errs.email

  const irARegistrar = () => { setModo('registrar'); setTouched(false) }

  const guardar = async () => {
    setTouched(true)
    if (!puedeRegistrar) return
    setBusy(true)
    try {
      const c = await api.crearCliente({
        nombre: nombre.trim(), tipoDocumento: tipo, documento: doc.trim(),
        telefono: tel.trim(), direccion: dir.trim(), email: email.trim(),
      })
      toast({ title: 'Cliente registrado', body: `${c.nombre} · ${c.tipoDocumento}-${c.documento}` })
      await reload()
      onSeleccionar(c)
    } catch (e) {
      toast({ title: 'No se pudo registrar', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open={open} onClose={onClose} size="sm" icon={<Icon.User size={18} />}
      title={modo === 'registrar' ? 'Registrar cliente nuevo' : 'Identificar cliente'}
      sub={modo === 'registrar' ? 'Se guarda en tu maestro de clientes.' : 'Busca por cédula / RIF o nombre.'}
      footer={modo === 'registrar' ? (
        <>
          <Button variant="ghost" onClick={() => setModo('buscar')}>Volver</Button>
          <Button onClick={guardar} loading={busy} icon={<Icon.Check size={16} />}>Registrar y usar</Button>
        </>
      ) : (
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
      )}>
      <div className="space-y-3.5">
        <div className="grid grid-cols-[80px_1fr] gap-2">
          <Field label="Tipo">
            <Select value={tipo} onChange={(e) => setTipo(e.target.value)}>
              {TIPOS_DOCUMENTO.map((t) => <option key={t} value={t}>{t}</option>)}
            </Select>
          </Field>
          <Field label="Cédula / RIF" required
            error={modo === 'registrar' && touched ? errs.documento : ''}>
            <Input value={doc} onChange={(e) => setDoc(e.target.value)} placeholder="12345678" autoFocus
              invalid={modo === 'registrar' && touched && !!errs.documento} />
          </Field>
        </div>

        {modo === 'buscar' ? (
          <>
            {term && resultados.length > 0 ? (
              <div className="rounded-xl border border-slate-200 dark:border-slate-800 divide-y divide-slate-100 dark:divide-slate-800 overflow-hidden">
                {resultados.map((c) => (
                  <button key={c.id} onClick={() => onSeleccionar(c)}
                    className="w-full flex items-center gap-2.5 px-3 py-2.5 text-left hover:bg-slate-50 dark:hover:bg-slate-800/60">
                    <div className="min-w-0 flex-1">
                      <div className="text-[13.5px] font-semibold truncate">{c.nombre}</div>
                      <div className="text-[12px] text-slate-500 num">{c.tipoDocumento}-{c.documento}{c.telefono ? ` · ${c.telefono}` : ''}</div>
                    </div>
                    <Badge size="sm" color="teal">Usar</Badge>
                  </button>
                ))}
              </div>
            ) : term ? (
              <div className="rounded-xl border border-dashed border-slate-300 dark:border-slate-700 px-4 py-5 text-center">
                <div className="text-[13px] text-slate-500">No hay ningún cliente con «{doc.trim()}».</div>
                <Button className="mt-3" size="sm" icon={<Icon.Plus size={15} />} onClick={irARegistrar}>Registrar cliente nuevo</Button>
              </div>
            ) : (
              <div className="text-[12.5px] text-slate-400 text-center py-2">
                Escribe la cédula / RIF o el nombre para buscar.
              </div>
            )}
            {term && !exacto && resultados.length > 0 ? (
              <button onClick={irARegistrar}
                className="w-full text-[12.5px] font-semibold text-elerp-600 hover:text-elerp-700 inline-flex items-center justify-center gap-1">
                <Icon.Plus size={13} /> ¿No está? Registrar cliente nuevo
              </button>
            ) : null}
          </>
        ) : (
          <>
            <Field label="Nombre y apellido" required error={touched ? errs.nombre : ''}>
              <Input value={nombre} onChange={(e) => setNombre(e.target.value)}
                onBlur={() => setTouched(true)} invalid={touched && !!errs.nombre} autoFocus />
            </Field>
            <div className="grid grid-cols-2 gap-3">
              <Field label="Teléfono" hint="opcional"><Input value={tel} onChange={(e) => setTel(e.target.value)} placeholder="0412-…" /></Field>
              <Field label="Correo" hint="opcional" error={touched ? errs.email : ''}>
                <Input value={email} onChange={(e) => setEmail(e.target.value)} type="email" placeholder="correo@dominio.com" invalid={touched && !!errs.email} />
              </Field>
            </div>
            <Field label="Dirección" hint="opcional"><Input value={dir} onChange={(e) => setDir(e.target.value)} /></Field>
          </>
        )}
      </div>
    </Modal>
  )
}

/* VERIFICAR PRECIO — consulta de mostrador. El cliente pregunta «¿cuánto cuesta
 * esto?» y el cajero lo responde sin abandonar la venta que está armando: busca o
 * escanea el producto y ve su precio (respetando la lista de precio seleccionada)
 * y su disponibilidad. Es de SOLO LECTURA: no agrega al carrito ni cambia nada. */
function VerificarPrecioModal({ open, onClose, productos, precioBsDe, existenciaDe, listaSel, monedaEmpresa }) {
  const [q, setQ] = useState('')
  const inputRef = useRef(null)
  // Al abrir se limpia la búsqueda y se enfoca (la pistola lectora escribe aquí).
  useEffect(() => {
    if (!open) { setQ(''); return }
    const t = setTimeout(() => inputRef.current?.focus(), 60)
    return () => clearTimeout(t)
  }, [open])

  const term = q.trim().toLowerCase()
  const resultados = useMemo(() => {
    if (!term) return []
    // Primero el match exacto por código (pistola lectora), luego por nombre/sku.
    const exacto = porCodigo(productos, q)
    const porTexto = productos.filter((p) =>
      p.nombre.toLowerCase().includes(term)
      || (p.sku || '').toLowerCase().includes(term)
      || (p.codigoBarras || '').includes(term)
      || (p.presentaciones || []).some((pr) => (pr.codigoBarras || '').includes(term)),
    )
    const lista = exacto ? [exacto, ...porTexto.filter((p) => p.sku !== exacto.sku)] : porTexto
    return lista.slice(0, 12)
  }, [term, productos, q])

  return (
    <Modal open={open} onClose={onClose} size="sm" icon={<Icon.Tag size={18} />}
      title="Verificar precio"
      sub={listaSel ? `Precios de la lista «${listaSel.nombre}».` : 'Consulta sin agregar al carrito.'}
      footer={<Button variant="ghost" onClick={onClose}>Cerrar</Button>}>
      <div className="flex items-center gap-2.5 rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 px-3.5">
        <Icon.Search size={18} className="text-slate-400 shrink-0" />
        <input ref={inputRef} value={q} onChange={(e) => setQ(e.target.value)}
          placeholder="Buscar o escanear código…"
          className="flex-1 bg-transparent outline-none py-3 text-[15px]" />
        {q ? <button onClick={() => setQ('')} aria-label="Limpiar búsqueda" className="text-slate-400 hover:text-slate-600 p-1"><Icon.X size={17} /></button> : null}
      </div>

      <div className="mt-3">
        {!term ? (
          <div className="text-[12.5px] text-slate-500 text-center py-6">
            Escribe el nombre o escanea el código para ver el precio.
          </div>
        ) : resultados.length === 0 ? (
          <div className="rounded-xl border border-dashed border-slate-300 dark:border-slate-700 px-4 py-6 text-center text-[13px] text-slate-500">
            No encontramos «{q}». Revisa el nombre o el código.
          </div>
        ) : (
          <div className="rounded-xl border border-slate-200 dark:border-slate-800 divide-y divide-slate-100 dark:divide-slate-800 overflow-hidden max-h-[52vh] overflow-y-auto">
            {resultados.map((p) => {
              const bs = precioBsDe(p)
              const cant = existenciaDe(p.sku)
              const st = nivelStock(cant)
              const agotado = cant <= 0
              return (
                <div key={p.id} className="flex items-center gap-3 px-3.5 py-3">
                  <div className="min-w-0 flex-1">
                    <div className="text-[14px] font-semibold truncate">{p.nombre}</div>
                    <div className="mono text-[11px] text-slate-500 truncate">
                      {p.codigoBarras || p.sku}{p.exentoIva ? ' · exento de IVA' : ''}
                    </div>
                  </div>
                  <div className="text-right shrink-0">
                    <div className="text-[17px] font-bold num">{bs === null ? 'sin tasa' : fmtCurrency(bs, 'VES')}</div>
                    <div className={`text-[11.5px] num ${st.clase}`}>{agotado ? 'Agotado' : `${fmtNum(cant, 2)} disp.`}</div>
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>
    </Modal>
  )
}

/* PESAJE — modal de la balanza para productos vendidos POR PESO (kg).
 *
 * Se abre al agregar/escanear un producto por peso (o al «Editar peso» de una
 * línea del carrito). Muestra el producto, su precio por kg, el campo Peso (kg) y
 * el subtotal en vivo (peso × Bs/kg). «Leer balanza» toma el peso del dispositivo
 * activo de la sede; la entrada manual está SIEMPRE disponible. «Aceptar y añadir»
 * agrega la línea con cantidad = peso; «Cancelar» no agrega nada. Peso 0 o inválido
 * no deja aceptar (validación inline). */
function PesajeModal({ pesaje, balanza, onAceptar, onClose }) {
  const abierto = !!pesaje
  const [peso, setPeso] = useState('')
  const [touched, setTouched] = useState(false)
  const [leyendo, setLeyendo] = useState(false)
  const inputRef = useRef(null)

  // Al abrir: prefila el peso (edición) o queda vacío (nuevo), limpia el estado y
  // enfoca el campo. Se usa la coma como separador decimal (formato local).
  useEffect(() => {
    if (!abierto) return
    const inicial = pesaje.pesoInicial
    setPeso(inicial != null && inicial !== '' ? String(inicial).replace('.', ',') : '')
    setTouched(false)
    setLeyendo(false)
    const t = setTimeout(() => { inputRef.current?.focus(); inputRef.current?.select?.() }, 60)
    return () => clearTimeout(t)
  }, [abierto, pesaje?.sku, pesaje?.editando])

  if (!abierto) return null

  // Se acepta coma o punto como separador decimal; el peso se redondea a 3 decimales.
  const kg = round3(Number(String(peso).replace(',', '.')))
  const valido = Number.isFinite(kg) && kg > 0
  const precioBs = Number(pesaje.precioBs) || 0
  const subtotal = valido ? kg * precioBs : 0
  const error = touched && !valido
    ? (peso.trim() === '' ? 'Ingresa el peso en kg.' : 'El peso debe ser mayor que 0.')
    : ''

  /* PUNTO DE INTEGRACIÓN — BALANZA (agente local).
   * TODO(integración): la lectura REAL del peso la hará el AGENTE LOCAL por un canal
   * saliente, EXACTAMENTE igual que la impresora fiscal (ver enviarAImpresoraFiscal
   * en ImpresionFiscal.jsx): el navegador NO habla con la balanza; le pide al binario
   * local que lea el peso estable del puerto configurado (p. ej.
   * `GET http://127.0.0.1:<puerto>/peso` firmado) y este devuelve los kg. Hoy se
   * SIMULA el canal (~0,6 s) con un peso verosímil para poder probar sin hardware. */
  const leerBalanza = async () => {
    if (!balanza || leyendo) return
    setLeyendo(true)
    await new Promise((r) => setTimeout(r, 600))
    const simulado = round3(0.2 + Math.random() * 2.3) // ~0,200–2,500 kg
    setPeso(String(simulado).replace('.', ','))
    setTouched(true)
    setLeyendo(false)
    inputRef.current?.focus()
  }

  const aceptar = () => {
    setTouched(true)
    if (!valido) return
    onAceptar(kg)
  }

  const balanzaLabel = balanza
    ? ([balanza.marca, balanza.modelo].filter(Boolean).join(' ') || balanza.nombre || 'configurada')
    : ''

  return (
    <Modal open={abierto} onClose={onClose} size="sm" icon={<Icon.ModContabilidad size={18} />}
      title={pesaje.editando ? 'Editar peso' : 'Pesar producto'}
      sub="Producto vendido por peso (kilogramos)."
      footer={(
        <>
          <Button variant="ghost" onClick={onClose}>Cancelar</Button>
          <Button onClick={aceptar} disabled={!valido} icon={<Icon.Check size={16} />}>
            {pesaje.editando ? 'Guardar peso' : 'Aceptar y añadir'}
          </Button>
        </>
      )}>
      <div className="space-y-3.5">
        <div className="rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 px-3.5 py-3">
          <div className="text-[14.5px] font-semibold">{pesaje.nombre}</div>
          <div className="text-[12.5px] text-slate-500 num">{fmtCurrency(precioBs, 'VES')} /kg{pesaje.exento ? ' · exento de IVA' : ''}</div>
        </div>

        <Field label="Peso (kg)" required error={error}>
          <div className="flex items-center gap-2">
            <Input ref={inputRef} value={peso}
              onChange={(e) => { setPeso(e.target.value.replace(/[^0-9.,]/g, '')); setTouched(true) }}
              onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); aceptar() } }}
              inputMode="decimal" placeholder="0,000" invalid={!!error} className="flex-1 num text-[17px]" />
            <Button variant="secondary" onClick={leerBalanza} loading={leyendo}
              disabled={!balanza} icon={<Icon.ModContabilidad size={16} />}
              title={balanza ? 'Tomar el peso de la balanza' : 'No hay balanza configurada'}>
              Leer balanza
            </Button>
          </div>
        </Field>

        {balanza ? (
          <div className="text-[11.5px] text-slate-400 flex items-start gap-1.5">
            <Icon.ModContabilidad size={13} className="mt-0.5 shrink-0" />
            <span>Balanza: {balanzaLabel}. También puedes ingresar el peso a mano.</span>
          </div>
        ) : (
          <div className="text-[11.5px] text-slate-500 flex items-start gap-1.5">
            <Icon.CircleAlert size={13} className="mt-0.5 shrink-0 text-amber-500" />
            <span>No hay balanza configurada (Configuración › Dispositivos). Ingresa el peso manualmente.</span>
          </div>
        )}

        {/* Subtotal en vivo: peso × Bs/kg (el mismo cálculo que la línea del carrito). */}
        <div className="flex items-center justify-between rounded-xl bg-teal-50 dark:bg-teal-500/10 border border-teal-200 dark:border-teal-800 px-3.5 py-3">
          <span className="text-[13px] font-semibold text-teal-800 dark:text-teal-300">
            Subtotal {valido ? <span className="text-teal-600/70 dark:text-teal-400/70 num font-normal">({fmtNum(kg, 3)} kg × {fmtCurrency(precioBs, 'VES')})</span> : null}
          </span>
          <span className="num text-[20px] font-extrabold text-teal-800 dark:text-teal-200">{fmtCurrency(subtotal, 'VES')}</span>
        </div>
      </div>
    </Modal>
  )
}

// Modal de ayuda con los atajos de teclado del puesto de cobro (botón "Atajos").
function AtajosModal({ open, onClose }) {
  const atajos = [
    { teclas: ['/'], desc: 'Enfocar el buscador de productos' },
    { teclas: ['Enter'], desc: 'Agregar el producto buscado o escaneado' },
    { teclas: ['Ctrl', 'Enter'], desc: 'Cobrar (⌘ + Enter en Mac)' },
    { teclas: ['Esc'], desc: 'Cerrar el panel o diálogo abierto' },
  ]
  return (
    <Modal open={open} onClose={onClose} size="sm" icon={<Icon.CircleAlert size={18} />}
      title="Atajos del puesto de cobro" sub="Pensados para cobrar sin soltar el teclado.">
      <div className="space-y-2.5">
        {atajos.map((a, i) => (
          <div key={i} className="flex items-center justify-between gap-3 text-[13.5px] py-1 border-b border-slate-100 dark:border-slate-800 last:border-0">
            <span className="text-slate-600 dark:text-slate-300">{a.desc}</span>
            <span className="flex items-center gap-1 shrink-0">
              {a.teclas.map((t, j) => (
                <span key={j} className="inline-flex items-center gap-1">
                  {j > 0 ? <span className="text-slate-400 text-[11px]">+</span> : null}
                  <kbd>{t}</kbd>
                </span>
              ))}
            </span>
          </div>
        ))}
      </div>
      <p className="text-[12px] text-slate-500 mt-4 leading-relaxed">
        En muchos laptops las teclas <kbd>F2</kbd>/<kbd>F8</kbd> son de brillo/volumen y necesitan
        <kbd>Fn</kbd>; por eso los atajos principales usan <kbd>/</kbd> y <kbd>Ctrl</kbd>+<kbd>Enter</kbd>.
        Si tu teclado manda F2/F8 tal cual, también funcionan como alias.
      </p>
    </Modal>
  )
}

const BotonCaja = ({ children, tone, ...rest }) => (
  <button {...rest}
    className={`hidden sm:inline-flex items-center gap-2 h-10 px-3.5 rounded-xl border text-[13px] font-semibold ring-focus whitespace-nowrap disabled:opacity-50 ${
      tone === 'danger'
        ? 'border-red-200 dark:border-red-900/60 bg-red-50 dark:bg-red-900/20 text-[#B3362C] dark:text-red-300 hover:bg-red-100 dark:hover:bg-red-900/30'
        : 'border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 hover:bg-slate-50 dark:hover:bg-slate-800'
    }`}>
    {children}
  </button>
)

const Fila = ({ label, valor }) => (
  <div className="flex items-center justify-between text-[14px] text-slate-500">
    <span>{label}</span><span className="num">{fmtCurrency(valor, 'VES')}</span>
  </div>
)

/* PIN de supervisor con teclado numérico, como el prototipo: la caja es táctil y
 * puede no tener teclado. El PIN lo valida el SERVIDOR (`/api/cajas/autorizar`),
 * que además deja la autorización en la auditoría con la acción concreta. */
export function PinSupervisorModal({ pedido, onCerrar }) {
  const [pin, setPin] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const toast = useToast()

  const enviar = async (valor) => {
    setBusy(true); setError('')
    try {
      const r = await api.autorizarSupervisor(pedido.accion, valor)
      toast({ title: 'Autorizado', body: `${r.autorizadoPor} autorizó: ${pedido.accion}.` })
      pedido.alAutorizar?.()
      onCerrar()
    } catch (e) {
      setError(e?.status === 403 ? 'PIN incorrecto — intenta de nuevo.' : (e?.message || 'No se pudo autorizar.'))
      setPin('')
    } finally {
      setBusy(false)
    }
  }

  // Añadir/borrar un dígito. Usa actualización FUNCIONAL (no cierra sobre `pin`),
  // así el tecleo rápido no pierde dígitos; el envío al llegar a 4 lo dispara el
  // efecto de abajo (una sola vez), no acá dentro.
  const tecla = (t) => {
    if (t === 'del') { setPin((p) => p.slice(0, -1)); return }
    setPin((p) => (p.length >= 4 ? p : p + t))
  }

  useEffect(() => { setPin(''); setError('') }, [pedido])
  // Autoenvío al completar los 4 dígitos (sirve para el táctil y para el teclado).
  useEffect(() => {
    if (pedido && pin.length === 4 && !busy) enviar(pin)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pin])
  // Teclado FÍSICO además del táctil: en un puesto con teclado, el supervisor
  // escribe el PIN directo (dígitos), borra (Backspace) o cancela (Escape). El
  // teclado en pantalla sigue para las cajas táctiles.
  useEffect(() => {
    if (!pedido) return undefined
    const onKey = (e) => {
      if (busy) return
      if (/^[0-9]$/.test(e.key)) { e.preventDefault(); tecla(e.key) }
      else if (e.key === 'Backspace') { e.preventDefault(); tecla('del') }
      else if (e.key === 'Escape') { e.preventDefault(); onCerrar() }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pedido, busy])

  if (!pedido) return null

  return (
    <div className="fixed inset-0 z-[240] flex items-center justify-center p-5 bg-slate-900/55 backdrop-blur-sm">
      <div className="w-full max-w-[320px] rounded-2xl bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 shadow-modal p-6 text-center modal-in">
        <div className="h-12 w-12 rounded-full bg-amber-50 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400 inline-flex items-center justify-center">
          <Icon.Lock size={22} />
        </div>
        <div className="text-[16px] font-semibold mt-2.5">Se necesita un supervisor</div>
        <div className="text-[12.5px] text-slate-500 mt-1">
          Pide que ingrese su PIN para {pedido.accion}.
        </div>

        <div className="flex gap-2.5 justify-center mt-4">
          {[0, 1, 2, 3].map((i) => (
            <span key={i} className={`h-3.5 w-3.5 rounded-full border-[1.5px] ${i < pin.length ? 'bg-elerp-500 border-elerp-500' : 'border-slate-300 dark:border-slate-600'}`} />
          ))}
        </div>
        {error ? <div className="mt-2 text-[12.5px] text-[#B3362C] dark:text-red-400">{error}</div> : null}

        <div className="grid grid-cols-3 gap-2 mt-4">
          {['1', '2', '3', '4', '5', '6', '7', '8', '9', '', '0', 'del'].map((t, i) => (
            t === '' ? <span key={i} /> : (
              <button key={i} onClick={() => tecla(t)} disabled={busy}
                className="h-12 rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/60 text-[17px] font-semibold hover:bg-slate-100 dark:hover:bg-slate-800 disabled:opacity-50">
                {t === 'del' ? <Icon.ChevLeft size={18} className="mx-auto" /> : t}
              </button>
            )
          ))}
        </div>
        <button onClick={onCerrar} className="mt-3.5 text-[12.5px] font-semibold text-slate-500 hover:text-slate-700 dark:hover:text-slate-300">
          Cancelar
        </button>
      </div>
    </div>
  )
}
