import { useRef, useState } from 'react'
import { Icon } from './Icon.jsx'
import { Button, Input, useToast } from './primitives.jsx'
import { api } from '../lib/api.js'

/* SubidorImagen — subidor REAL de imágenes (multipart) reutilizable, con
 * requisitos de tamaño a la vista. Sube al bucket del tenant vía api.subirArchivo
 * y devuelve por onChange la RUTA relativa servida (/api/archivos/…), la misma
 * que usan las imágenes de producto. Se usa en el editor de Promociones y en el
 * editor de slides de la pantalla del cliente (Ajustes).
 *
 * Mantiene la URL como alternativa: quien prefiera pegar un enlace (o un data URI)
 * puede hacerlo en el campo de abajo. La vista previa es 16:9, la proporción con
 * la que se ve el carrusel de la pantalla del cliente.
 *
 * Props: value (url actual) · onChange(url) · carpeta (promociones|publicidad|
 * empresa) · disabled. */

// Requisitos que se validan/comunican. El backend admite hasta 4 MiB y valida el
// contenido real (magic bytes); acá somos más estrictos (2 MB) y damos el mensaje.
const MAX_BYTES = 2 * 1024 * 1024
const TIPOS_OK = ['image/jpeg', 'image/png', 'image/webp']
const REQUISITOS = 'JPG o PNG · horizontal 16:9 · recomendado 1920 × 1080 o 1280 × 720 · máx. 2 MB'

export function SubidorImagen({ value, onChange, carpeta = 'publicidad', disabled = false, className = '' }) {
  const inputRef = useRef(null)
  const toast = useToast()
  const [subiendo, setSubiendo] = useState(false)
  const [drag, setDrag] = useState(false)
  const [aviso, setAviso] = useState('') // advertencia suave (proporción), no bloquea
  const url = (value || '').trim()

  // Validación de tipo/tamaño en cliente, con mensaje claro (no solo al enviar).
  const validar = (file) => {
    if (!file) return 'Elige un archivo de imagen.'
    if (!TIPOS_OK.includes(file.type)) return 'Formato no admitido. Usa una imagen JPG o PNG.'
    if (file.size > MAX_BYTES) return `La imagen pesa ${(file.size / 1024 / 1024).toFixed(1)} MB y el máximo es 2 MB. Reduce su peso e intenta de nuevo.`
    return ''
  }

  // Chequeo suave de proporción: avisa (sin bloquear) si la imagen no es ~16:9,
  // porque en la pantalla del cliente se verá con franjas o recortada.
  const chequearProporcion = (file) => {
    try {
      const u = URL.createObjectURL(file)
      const img = new Image()
      img.onload = () => {
        URL.revokeObjectURL(u)
        const r = img.naturalWidth / img.naturalHeight
        setAviso(Math.abs(r - 16 / 9) > 0.4
          ? 'Esta imagen no es horizontal 16:9: puede verse con franjas o recortes en la pantalla del cliente.'
          : '')
      }
      img.onerror = () => URL.revokeObjectURL(u)
      img.src = u
    } catch { /* sin preview de proporción, no pasa nada */ }
  }

  const subir = async (file) => {
    if (disabled) return
    const err = validar(file)
    if (err) { toast({ title: 'No se pudo subir', body: err, kind: 'error' }); return }
    chequearProporcion(file)
    setSubiendo(true)
    try {
      const { url: nueva } = await api.subirArchivo(file, carpeta)
      onChange(nueva)
      toast({ title: 'Imagen subida', body: 'Se guardó en el almacenamiento de tu empresa.' })
    } catch (e) {
      toast({ title: 'No se pudo subir', body: e?.message || 'Error', kind: 'error' })
    } finally {
      setSubiendo(false)
    }
  }

  return (
    <div className={`space-y-2 ${className}`}>
      {/* Zona de imagen: arrastra o elige un archivo; con imagen, la vista previa 16:9. */}
      <div
        onDragOver={(e) => { e.preventDefault(); if (!disabled) setDrag(true) }}
        onDragLeave={() => setDrag(false)}
        onDrop={(e) => { e.preventDefault(); setDrag(false); subir(e.dataTransfer?.files?.[0]) }}
        className={`relative rounded-xl border-2 border-dashed transition-colors overflow-hidden ${drag
          ? 'border-teal-500 bg-teal-50/60 dark:bg-teal-900/20'
          : 'border-slate-300 dark:border-slate-700'}`}>
        <div className="aspect-[16/9] w-full flex items-center justify-center bg-slate-50 dark:bg-slate-900/40">
          {url ? (
            <img src={url} alt="Vista previa" className="h-full w-full object-contain"
              onError={(e) => { e.currentTarget.style.visibility = 'hidden' }} />
          ) : (
            <div className="text-center px-4">
              <Icon.Upload size={22} className="mx-auto text-slate-400" />
              <div className="mt-1.5 text-[12.5px] text-slate-500 dark:text-slate-400">
                Arrastra una imagen o <span className="text-teal-600 dark:text-teal-400 font-medium">elige un archivo</span>
              </div>
            </div>
          )}
        </div>
        {/* Overlay clickeable para abrir el selector (no tapa los botones de abajo). */}
        {!disabled ? (
          <button type="button" onClick={() => inputRef.current?.click()}
            className="absolute inset-0 w-full h-full cursor-pointer" aria-label="Elegir imagen" />
        ) : null}
      </div>

      <input ref={inputRef} type="file" accept="image/png,image/jpeg,image/webp" className="hidden"
        onChange={(e) => { subir(e.target.files?.[0]); e.target.value = '' }} />

      {!disabled ? (
        <div className="flex items-center gap-2 flex-wrap">
          <Button size="sm" variant="ghost" icon={<Icon.Upload size={14} />} loading={subiendo}
            onClick={() => inputRef.current?.click()}>
            {url ? 'Cambiar imagen' : 'Subir imagen'}
          </Button>
          {url ? (
            <Button size="sm" variant="ghost" icon={<Icon.Trash size={14} />} onClick={() => { onChange(''); setAviso('') }}>Quitar</Button>
          ) : null}
          <span className="text-[11px] text-slate-400 ml-auto">{REQUISITOS}</span>
        </div>
      ) : (
        <div className="text-[11px] text-slate-400">{REQUISITOS}</div>
      )}

      {/* Alternativa: pegar una URL (o un data URI) en vez de subir un archivo. */}
      {!disabled ? (
        <Input value={value || ''} placeholder="…o pega la URL de la imagen (https://… o /api/archivos/…)"
          icon={<Icon.Link size={14} />} onChange={(e) => onChange(e.target.value)} />
      ) : null}

      {aviso ? (
        <div className="flex items-start gap-1.5 text-[11.5px] text-amber-700 dark:text-amber-400">
          <Icon.CircleAlert size={13} className="mt-0.5 shrink-0" /><span>{aviso}</span>
        </div>
      ) : null}
    </div>
  )
}
