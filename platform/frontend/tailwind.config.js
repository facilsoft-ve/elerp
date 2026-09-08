/** @type {import('tailwindcss').Config} */
// Tema de la consola de plataforma: misma línea gráfica ElERP (navy `huberp` + acento
// `teal`), neutros mapeados sobre `slate`. Se mantiene alineado con frontend/tailwind.config.js.
export default {
  content: ['./index.html', './src/**/*.{js,jsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        elerp: {
          50: '#F3EFFE', 100: '#E8DEFC', 200: '#D0BEF9', 300: '#B295F4',
          400: '#8D5FF0', 500: '#6A2CF0', 600: '#5A21DB', 700: '#4A1AB4',
          800: '#3B168C', 900: '#2A2440', 950: '#1A1330',
        },
        teal: {
          50: '#F3EFFE', 100: '#E8DEFC', 200: '#D0BEF9', 300: '#B295F4',
          400: '#8D5FF0', 500: '#6A2CF0', 600: '#5A21DB', 700: '#4A1AB4',
          800: '#3B168C', 900: '#2A2440',
        },
        slate: {
          50: '#FAFBFC', 100: '#F0F2EF', 200: '#E8EAEF', 300: '#D7DBD8',
          400: '#8B94A3', 500: '#5C6470', 600: '#4A515C', 700: '#3E4650',
          800: '#232834', 900: '#1F2430', 950: '#12151C',
        },
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
        brand: '0 4px 14px rgba(29,52,119,0.10)',
        card: '0 1px 2px 0 rgba(29,52,119,0.05)',
        pop: '0 8px 28px rgba(29,52,119,0.14)',
        modal: '0 8px 28px rgba(29,52,119,0.14)',
      },
      borderRadius: { xl: '12px', icon: '11px', tile: '16px' },
    },
  },
  plugins: [],
}
