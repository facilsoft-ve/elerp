import { useState, useEffect } from 'react'
import { Icon } from '../components/Icon.jsx'
import { PageHeader } from '../components/primitives.jsx'
import { Catalogo } from './Catalogo.jsx'
import { Existencias } from './Existencias.jsx'
import { Kardex } from './Kardex.jsx'
import { Transferencias } from './Transferencias.jsx'
import { Movimientos } from './Movimientos.jsx'
import { ComprasRecepcion } from './ComprasRecepcion.jsx'

// Contenedor del módulo Inventario. Las 4 vistas viven en pestañas. Acepta rutas
// "inventario", "inventario:<sub>" y "kardex:<sku>" (esta última abre Kardex con
// el SKU preseleccionado, p. ej. desde la paleta de comandos).
const TABS = [
  { id: 'catalogo', label: 'Catálogo', icon: <Icon.Package size={15} /> },
  { id: 'existencias', label: 'Existencias', icon: <Icon.Boxes size={15} /> },
  { id: 'recepcion', label: 'Recepción', icon: <Icon.ClipboardList size={15} /> },
  { id: 'kardex', label: 'Kardex', icon: <Icon.History size={15} /> },
  { id: 'movimientos', label: 'Movimientos', icon: <Icon.ArrowDown size={15} /> },
  { id: 'transferencias', label: 'Transferencias', icon: <Icon.ArrowLeftRight size={15} /> },
]

export function Inventario({ route }) {
  const [head, arg] = (route || 'inventario').split(':')
  const initialTab = head === 'kardex' ? 'kardex' : (arg || 'catalogo')
  const initialSku = head === 'kardex' ? arg : ''
  const [tab, setTab] = useState(initialTab)
  const [kardexSku, setKardexSku] = useState(initialSku)

  // Al cambiar la ruta externa (deep link desde paleta / dashboard), sincronizar.
  useEffect(() => {
    setTab(initialTab)
    if (initialSku) setKardexSku(initialSku)
  }, [route]) // eslint-disable-line react-hooks/exhaustive-deps

  const openKardex = (sku) => { setKardexSku(sku); setTab('kardex') }

  return (
    <div className="p-4 md:p-6 lg:px-8 lg:py-7">
      <PageHeader
        breadcrumb={['Inventario', TABS.find((t) => t.id === tab)?.label]}
        title="Inventario"
        sub="Catálogo, existencias por sede, recepción de mercancía, Kardex valorado y transferencias entre sedes."
        tabs={TABS} activeTab={tab} onTab={setTab} />
      {tab === 'catalogo' ? <Catalogo onKardex={openKardex} /> : null}
      {tab === 'existencias' ? <Existencias onKardex={openKardex} /> : null}
      {tab === 'recepcion' ? <ComprasRecepcion /> : null}
      {tab === 'kardex' ? <Kardex sku={kardexSku} setSku={setKardexSku} /> : null}
      {tab === 'movimientos' ? <Movimientos onKardex={openKardex} /> : null}
      {tab === 'transferencias' ? <Transferencias /> : null}
    </div>
  )
}
