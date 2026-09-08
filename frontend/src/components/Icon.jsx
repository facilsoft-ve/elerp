/* Iconos estilo Lucide, SVG inline. Portado del prototipo Tesotrix + añadidos
 * para Inventario y módulos ERP. */
const _i = (path) => ({ size = 16, className = '', stroke = 1.6, fill = 'none', ...rest }) => (
  <svg xmlns="http://www.w3.org/2000/svg" width={size} height={size} viewBox="0 0 24 24"
    fill={fill} stroke="currentColor" strokeWidth={stroke} strokeLinecap="round" strokeLinejoin="round"
    className={className} {...rest}>
    {path}
  </svg>
)

export const Icon = {
  Home: _i(<><path d="M3 11.5 12 4l9 7.5" /><path d="M5 10v10h14V10" /></>),
  Bank: _i(<><path d="M3 10 12 4l9 6" /><path d="M5 10v8" /><path d="M9 10v8" /><path d="M15 10v8" /><path d="M19 10v8" /><path d="M3 20h18" /></>),
  Wallet: _i(<><path d="M3 7a2 2 0 0 1 2-2h13a2 2 0 0 1 2 2v3h-3a2 2 0 1 0 0 4h3v3a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z" /><path d="M17 12h.01" /></>),
  Arrows: _i(<><path d="M7 4v16" /><path d="m3 8 4-4 4 4" /><path d="M17 20V4" /><path d="m13 16 4 4 4-4" /></>),
  Tag: _i(<><path d="M3 3h7l11 11-7 7L3 10V3Z" /><circle cx="7" cy="7" r="1.4" fill="currentColor" /></>),
  Refresh: _i(<><path d="M21 12a9 9 0 0 1-15.5 6.4L3 16" /><path d="M3 12a9 9 0 0 1 15.5-6.4L21 8" /><path d="M21 4v4h-4" /><path d="M3 20v-4h4" /></>),
  Calendar: _i(<><rect x="3" y="5" width="18" height="16" rx="2" /><path d="M8 3v4" /><path d="M16 3v4" /><path d="M3 10h18" /></>),
  Chart: _i(<><path d="M3 3v18h18" /><path d="m7 15 4-5 3 3 5-7" /></>),
  Plug: _i(<><path d="M9 2v6" /><path d="M15 2v6" /><path d="M6 8h12v3a6 6 0 0 1-12 0V8Z" /><path d="M12 17v5" /></>),
  Settings: _i(<><circle cx="12" cy="12" r="3" /><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1Z" /></>),
  Search: _i(<><circle cx="11" cy="11" r="7" /><path d="m20 20-3.5-3.5" /></>),
  Bell: _i(<><path d="M6 8a6 6 0 0 1 12 0c0 7 3 8 3 8H3s3-1 3-8" /><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0" /></>),
  Eye: _i(<><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z" /><circle cx="12" cy="12" r="3" /></>),
  EyeOff: _i(<><path d="M9.9 4.2A10 10 0 0 1 12 4c6.5 0 10 7 10 7a18 18 0 0 1-3.2 4.1" /><path d="M6.6 6.6A18 18 0 0 0 2 12s3.5 7 10 7a10 10 0 0 0 5.3-1.4" /><path d="m3 3 18 18" /><path d="M9.9 9.9a3 3 0 0 0 4.2 4.2" /></>),
  Sun: _i(<><circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M4 12H2M22 12h-2M5 5l1.5 1.5M17.5 17.5 19 19M5 19l1.5-1.5M17.5 6.5 19 5" /></>),
  Moon: _i(<><path d="M20 14.5A8 8 0 1 1 9.5 4 7 7 0 0 0 20 14.5Z" /></>),
  ChevDown: _i(<><path d="m6 9 6 6 6-6" /></>),
  ChevRight: _i(<><path d="m9 6 6 6-6 6" /></>),
  ChevLeft: _i(<><path d="m15 6-6 6 6 6" /></>),
  ChevUp: _i(<><path d="m6 15 6-6 6 6" /></>),
  Plus: _i(<><path d="M12 5v14" /><path d="M5 12h14" /></>),
  Minus: _i(<><path d="M5 12h14" /></>),
  X: _i(<><path d="M6 6 18 18" /><path d="m6 18 12-12" /></>),
  Check: _i(<><path d="m5 12 5 5 9-12" /></>),
  Filter: _i(<><path d="M3 5h18" /><path d="M6 12h12" /><path d="M10 19h4" /></>),
  // Familia del mostrador: impresora fiscal, WhatsApp, sin conexión, teléfono
  // (pago móvil) y cobro mixto.
  // Modo caja: mano (diestro/zurdo) y minimizar (salir de pantalla completa).
  Hand: _i(<><path d="M8 12V5.5a1.5 1.5 0 0 1 3 0V11" /><path d="M11 11V4.5a1.5 1.5 0 0 1 3 0V11" /><path d="M14 11V6.5a1.5 1.5 0 0 1 3 0V13" /><path d="M17 11.5a1.5 1.5 0 0 1 3 0V15a6 6 0 0 1-6 6h-2a6 6 0 0 1-6-6v-3l-1.4 1a1.5 1.5 0 0 1-1.9-2.3L8 10" /></>),
  Maximize: _i(<><path d="M8 3H5a2 2 0 0 0-2 2v3" /><path d="M16 3h3a2 2 0 0 1 2 2v3" /><path d="M8 21H5a2 2 0 0 1-2-2v-3" /><path d="M16 21h3a2 2 0 0 0 2-2v-3" /></>),
  Minimize: _i(<><path d="M9 3H5a2 2 0 0 0-2 2v4" /><path d="M15 3h4a2 2 0 0 1 2 2v4" /><path d="M9 21H5a2 2 0 0 1-2-2v-4" /><path d="M15 21h4a2 2 0 0 0 2-2v-4" /></>),
  Printer: _i(<><path d="M6 9V4h12v5" /><rect x="4" y="9" width="16" height="7" rx="2" /><path d="M8 16h8v4H8z" /><path d="M17 12h.01" /></>),
  Message: _i(<><path d="M21 12a8 8 0 0 1-11.6 7.1L4 21l1.9-5.1A8 8 0 1 1 21 12Z" /></>),
  WifiOff: _i(<><path d="m2 2 20 20" /><path d="M5 12.5a10 10 0 0 1 4-2.4" /><path d="M8.5 16a5.5 5.5 0 0 1 2.5-1.4" /><path d="M12 20h.01" /><path d="M15.5 10.2A10 10 0 0 1 19 12.5" /></>),
  Smartphone: _i(<><rect x="6" y="2.5" width="12" height="19" rx="2.5" /><path d="M11 18.5h2" /></>),
  Shuffle: _i(<><path d="M16 3h5v5" /><path d="M4 20 21 3" /><path d="M21 16v5h-5" /><path d="M15 15l6 6" /><path d="M4 4l5 5" /></>),
  Upload: _i(<><path d="M12 20V8" /><path d="m7 13 5-5 5 5" /><path d="M4 4h16" /></>),
  Download: _i(<><path d="M12 4v12" /><path d="m7 11 5 5 5-5" /><path d="M4 20h16" /></>),
  CircleAlert: _i(<><circle cx="12" cy="12" r="9" /><path d="M12 7v6" /><circle cx="12" cy="16" r=".5" fill="currentColor" /></>),
  CircleCheck: _i(<><circle cx="12" cy="12" r="9" /><path d="m8 12 3 3 5-6" /></>),
  CircleX: _i(<><circle cx="12" cy="12" r="9" /><path d="m9 9 6 6" /><path d="m15 9-6 6" /></>),
  Lock: _i(<><rect x="4" y="11" width="16" height="10" rx="2" /><path d="M8 11V8a4 4 0 0 1 8 0v3" /></>),
  Mail: _i(<><rect x="3" y="5" width="18" height="14" rx="2" /><path d="m3 7 9 7 9-7" /></>),
  User: _i(<><circle cx="12" cy="8" r="4" /><path d="M4 21a8 8 0 0 1 16 0" /></>),
  Users: _i(<><circle cx="9" cy="8" r="4" /><path d="M2 21a7 7 0 0 1 14 0" /><circle cx="17" cy="6" r="3" /><path d="M22 19a5 5 0 0 0-5-5" /></>),
  UserPlus: _i(<><circle cx="9" cy="8" r="4" /><path d="M2 21a7 7 0 0 1 14 0" /><path d="M18 8v6" /><path d="M15 11h6" /></>),
  Copy: _i(<><rect x="9" y="9" width="12" height="12" rx="2" /><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" /></>),
  Info: _i(<><circle cx="12" cy="12" r="9" /><path d="M12 11v5" /><path d="M12 8h.01" /></>),
  Logout: _i(<><path d="M15 17l5-5-5-5" /><path d="M20 12H9" /><path d="M9 4H6a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h3" /></>),
  Link: _i(<><path d="M10 14a5 5 0 0 0 7 0l3-3a5 5 0 0 0-7-7l-1 1" /><path d="M14 10a5 5 0 0 0-7 0l-3 3a5 5 0 0 0 7 7l1-1" /></>),
  Sparkles: _i(<><path d="M12 3v4" /><path d="M12 17v4" /><path d="M3 12h4" /><path d="M17 12h4" /><path d="m5 5 2.5 2.5" /><path d="m16.5 16.5 2.5 2.5" /><path d="m5 19 2.5-2.5" /><path d="m16.5 7.5 2.5-2.5" /></>),
  Menu: _i(<><path d="M3 6h18" /><path d="M3 12h18" /><path d="M3 18h18" /></>),
  Shield: _i(<><path d="M12 2 4 5v6c0 5 3.5 9 8 11 4.5-2 8-6 8-11V5l-8-3Z" /></>),
  Key: _i(<><circle cx="8" cy="15" r="4" /><path d="m10.6 12.4 9.4-9.4" /><path d="m17 5 3 3" /><path d="m15 7 2 2" /></>),
  ArrowDown: _i(<><path d="M12 5v14" /><path d="m5 12 7 7 7-7" /></>),
  ArrowUp: _i(<><path d="M12 19V5" /><path d="m5 12 7-7 7 7" /></>),
  ArrowRight: _i(<><path d="M5 12h14" /><path d="m12 5 7 7-7 7" /></>),
  Receipt: _i(<><path d="M4 3v18l2-1 2 1 2-1 2 1 2-1 2 1 2-1V3l-2 1-2-1-2 1-2-1-2 1-2-1-2 1Z" /><path d="M8 8h8M8 12h8M8 16h5" /></>),
  FileText: _i(<><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8Z" /><path d="M14 3v5h5" /><path d="M9 13h6M9 17h6" /></>),
  Image: _i(<><rect x="3" y="3" width="18" height="18" rx="2" /><circle cx="9" cy="9" r="2" /><path d="m21 15-4.5-4.5L5 21" /></>),
  Utensils: _i(<><path d="M4 3v6a2 2 0 0 0 4 0V3" /><path d="M6 11v10" /><path d="M17 3c-1.66 0-3 2-3 5v4h3" /><path d="M17 3v18" /></>),
  Layers: _i(<><path d="m12 3 9 5-9 5-9-5 9-5Z" /><path d="m3 13 9 5 9-5" /><path d="m3 17 9 5 9-5" /></>),
  Inbox: _i(<><path d="M21 13H16l-2 3h-4l-2-3H3" /><path d="M5 6h14l2 7v6a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1v-6Z" /></>),
  Send: _i(<><path d="M22 2 11 13" /><path d="M22 2l-7 20-4-9-9-4 20-7Z" /></>),
  Clock: _i(<><circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" /></>),
  Banknote: _i(<><rect x="2" y="6" width="20" height="12" rx="2" /><circle cx="12" cy="12" r="2.5" /><path d="M6 10h.01M18 14h.01" /></>),
  Activity: _i(<><path d="M3 12h4l3-9 4 18 3-9h4" /></>),
  Globe: _i(<><circle cx="12" cy="12" r="9" /><path d="M3 12h18" /><path d="M12 3a14 14 0 0 1 0 18" /><path d="M12 3a14 14 0 0 0 0 18" /></>),
  History: _i(<><path d="M3 12a9 9 0 1 0 3-6.7L3 8" /><path d="M3 4v4h4" /><path d="M12 8v5l3 2" /></>),
  Repeat: _i(<><path d="m17 1 4 4-4 4" /><path d="M3 11V9a4 4 0 0 1 4-4h14" /><path d="m7 23-4-4 4-4" /><path d="M21 13v2a4 4 0 0 1-4 4H3" /></>),
  Star: _i(<><path d="m12 3 2.9 5.9 6.5.9-4.7 4.6 1.1 6.5L12 17.8 6.2 21l1.1-6.5L2.6 9.8l6.5-.9L12 3Z" /></>),
  Pencil: _i(<><path d="M12 20h9" /><path d="M16.5 3.5a2.1 2.1 0 1 1 3 3L7 19l-4 1 1-4Z" /></>),
  Trash: _i(<><path d="M3 6h18" /><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" /><path d="M5 6v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V6" /></>),
  Package: _i(<><path d="m12 2 8.5 4.9v10.2L12 22l-8.5-4.9V6.9L12 2Z" /><path d="m3.5 7 8.5 5 8.5-5" /><path d="M12 22V12" /></>),
  Boxes: _i(<><path d="M7 3 3 5.5 7 8l4-2.5L7 3Z" /><path d="M17 3l-4 2.5L17 8l4-2.5L17 3Z" /><path d="M12 12l-4 2.5L12 17l4-2.5L12 12Z" /><path d="M3 5.5v6L7 14M21 5.5v6L17 14M7 8v6M17 8v6" /></>),
  Truck: _i(<><path d="M3 6h11v9H3z" /><path d="M14 9h4l3 3v3h-7z" /><circle cx="7" cy="18" r="1.8" /><circle cx="17" cy="18" r="1.8" /></>),
  Book: _i(<><path d="M4 4a2 2 0 0 1 2-2h13v18H6a2 2 0 0 0-2 2Z" /><path d="M4 20a2 2 0 0 1 2-2h13" /></>),
  Cart: _i(<><circle cx="9" cy="20" r="1.4" /><circle cx="18" cy="20" r="1.4" /><path d="M2 3h3l2.4 12.4a1 1 0 0 0 1 .8h8.7a1 1 0 0 0 1-.8L21 7H6" /></>),
  ClipboardList: _i(<><rect x="8" y="3" width="8" height="4" rx="1" /><path d="M8 5H6a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2h-2" /><path d="M9 12h7M9 16h7" /></>),
  Command: _i(<><path d="M6 4a3 3 0 1 1-3 3h3Zm0 0v14a3 3 0 1 0 3-3H6Zm12 0a3 3 0 1 0-3 3h3Zm0 0v14a3 3 0 1 1-3-3h3Z" /></>),
  ArrowLeftRight: _i(<><path d="M8 3 4 7l4 4" /><path d="M4 7h16" /><path d="m16 21 4-4-4-4" /><path d="M20 17H4" /></>),

  /* --------------------------------------------------------------------------
   * Glifos de módulo — familia oficial del handoff de branding §05.
   * Retícula 24×24, trazo 1.8 (lo aplica <ModuleIcon>), un estilo compartido:
   * la identidad del módulo está en el glifo, nunca en un color propio.
   * Se usan a través de MODULOS en components/brand.jsx.
   * ------------------------------------------------------------------------ */
  // Fiscal y Facturación — documento con signo de moneda.
  ModFiscal: _i(<><rect x="4.5" y="2.8" width="15" height="18.4" rx="2.6" /><path d="M12 6.6v10.8" /><path d="M14.3 9.3a2.3 2.3 0 0 0-2.3-1.6c-1.3 0-2.3.85-2.3 2s.85 1.7 2.3 2 2.3.85 2.3 2-1 2-2.3 2a2.3 2.3 0 0 1-2.3-1.6" /></>),
  // Inventario y Operaciones — cubo (la caja).
  ModInventario: _i(<><path d="M12 2.6 20.5 7.3v9.4L12 21.4 3.5 16.7V7.3L12 2.6Z" /><path d="M3.5 7.3 12 12l8.5-4.7" /><path d="M12 12v9.4" /></>),
  // Contabilidad y Finanzas — balanza.
  ModContabilidad: _i(<><path d="M12 3.4v13.2" /><path d="M8 16.6h8" /><path d="M4.4 7.6h15.2" /><path d="M4.4 7.6 1.9 13.2h5L4.4 7.6Z" /><path d="M19.6 7.6l-2.5 5.6h5l-2.5-5.6Z" /></>),
  // Tesorería y Pagos — tarjeta.
  ModTesoreria: _i(<><rect x="2.6" y="5" width="18.8" height="14" rx="2.6" /><path d="M2.6 9.6h18.8" /><path d="M6.2 14.6h4.2" /></>),
  // RRHH y Nómina — credencial.
  ModRRHH: _i(<><rect x="2.6" y="4.6" width="18.8" height="14.8" rx="2.6" /><circle cx="8.6" cy="10.4" r="1.9" /><path d="M5.6 15.9c.55-1.55 1.7-2.35 3-2.35s2.45.8 3 2.35" /><path d="M14.8 9.6h4.2M14.8 13.2h4.2" /></>),
  // Reportes y BI — barras.
  ModReportes: _i(<><path d="M3.6 20h16.8" /><path d="M7 20v-4.6" /><path d="M11 20v-8.8" /><path d="M15 20v-6.2" /><path d="M19 20v-11" /></>),
  // Configuración — deslizadores.
  ModConfig: _i(<><path d="M3.6 7.4h9.4M17.4 7.4h3" /><circle cx="15.2" cy="7.4" r="2.2" /><path d="M3.6 16.6h5.2M13 16.6h7.4" /><circle cx="10.9" cy="16.6" r="2.2" /></>),
}
