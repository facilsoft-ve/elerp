/** @type {import('tailwindcss').Config} */
// Tema ElERP (navy + acento teal). Los tokens neutros del prototipo se mapean
// sobre la escala `slate` de Tailwind para que TODA la superficie herede la
// paleta oficial sin reescribir cada pantalla. Claro = fiel al prototipo;
// oscuro = coherente (tonos 700-950 permanecen oscuros para superficies dark).
export default {
  content: ['./index.html', './src/**/*.{js,jsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // Rampa navy de marca. Anclada a los tokens del handoff:
        //   50  = --hb-azul-suave   #E9EDF6 (fondos seleccionados, panel informativo)
        //   100 = --hb-azul-suave-2 #D4DCEE (segundo tinte de la esquina modular)
        //   500 = --hb-azul         #1D3477 (marca, primario)
        //   600 = --hb-azul-hover   #16295F · 700 = --hb-azul-active #112049
        elerp: {
          50: '#F3EFFE', 100: '#E8DEFC', 200: '#D0BEF9', 300: '#B295F4',
          400: '#8D5FF0', 500: '#6A2CF0', 600: '#5A21DB', 700: '#4A1AB4',
          800: '#3B168C', 900: '#2A2440', 950: '#1A1330',
        },
        // Acento verde (hub). El azul estructura, el verde señala: acción de crear,
        // éxito, estado activo y foco. Nunca como fondo grande.
        //   50 = --hb-verde-suave #E2F7F3 · 500 = --hb-verde #09B69B
        //   600 = --hb-verde-hover #079C85 · 700 = --hb-verde-active #068672
        teal: {
          50: '#F3EFFE', 100: '#E8DEFC', 200: '#D0BEF9', 300: '#B295F4',
          400: '#8D5FF0', 500: '#6A2CF0', 600: '#5A21DB', 700: '#4A1AB4',
          800: '#3B168C', 900: '#2A2440',
        },
        // Neutros ElERP mapeados sobre `slate`. 50-400 claros (también texto en dark),
        // 700-950 oscuros (superficies/bordes en dark; texto primario/cuerpo en light).
        slate: {
          50: '#FAFBFC',   // bg app · hover de fila · superficie tenue
          100: '#F0F2EF',  // hover superficie · divisores
          200: '#E8EAEF',  // borde por defecto
          300: '#D7DBD8',  // borde de inputs / botón secundario
          400: '#8B94A3',  // texto tenue (faint)
          500: '#5C6470',  // texto atenuado (muted)
          600: '#4A515C',  // texto atenuado oscuro
          700: '#3E4650',  // texto cuerpo (light) · borde (dark)
          800: '#232834',  // superficie/borde (dark)
          900: '#1F2430',  // texto primario (light) · superficie (dark)
          950: '#12151C',  // bg app (dark)
        },
        // Semánticos exactos del prototipo (mantienen tonos claros/oscuros para dark).
        emerald: {
          50: '#E9F5EE', 100: '#D2ECDD', 200: '#A9DABF', 300: '#6FD19E',
          400: '#34B473', 500: '#1E7A4C', 600: '#166B41', 700: '#0F5232',
          800: '#0B3D26', 900: '#07281A',
        },
        red: {
          50: '#FBEDEB', 100: '#F6D7D3', 200: '#ECC8C4', 300: '#E0A69F',
          400: '#D07C72', 500: '#C4463A', 600: '#B3362C', 700: '#93291F',
          800: '#6E1E17', 900: '#4A140F',
        },
        rose: {
          50: '#FBEDEB', 100: '#F6D7D3', 200: '#ECC8C4', 300: '#E0A69F',
          400: '#D07C72', 500: '#C4463A', 600: '#B3362C', 700: '#93291F',
          800: '#6E1E17', 900: '#4A140F',
        },
        amber: {
          50: '#FDF6E3', 100: '#F8ECC4', 200: '#EBD9A9', 300: '#DEBF6E',
          400: '#C79A2E', 500: '#A97A0E', 600: '#92600A', 700: '#754C08',
          800: '#573905', 900: '#3A2603',
        },
      },
      fontFamily: {
        sans: ['"IBM Plex Sans"', 'ui-sans-serif', 'system-ui', 'sans-serif'],
        display: ['Poppins', '"IBM Plex Sans"', 'ui-sans-serif', 'system-ui', 'sans-serif'],
        mono: ['"IBM Plex Mono"', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'monospace'],
      },
      boxShadow: {
        // Sombras tintadas de azul de marca (--hb-sombra). La interfaz se apoya en
        // bordes, no en sombras: las cards de métrica van SIN sombra en reposo.
        brand: '0 4px 14px rgba(29,52,119,0.10)',
        card: '0 1px 2px 0 rgba(29,52,119,0.05)',
        pop: '0 8px 28px rgba(29,52,119,0.14)',
        modal: '0 8px 28px rgba(29,52,119,0.14)',
      },
      borderRadius: {
        xl: '12px',   // --hb-radio-card
        icon: '11px', // --hb-radio-icono (cuadro de icono de módulo)
        tile: '16px', // --hb-radio-loseta (losetas táctiles POS)
      },
    },
  },
  plugins: [],
}
