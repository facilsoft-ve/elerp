import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// La consola se sirve en la raíz de su propio dominio (p. ej. admin.elerp.tech).
// En dev, Vite proxya /papi al BFF local (puerto 8090), como el ERP proxya /api.
// La consola se sirve como subruta del dominio principal (/consola) detrás del proxy,
// para reusar su DNS + certificado. El `base` debe coincidir con PLATFORM_BASE_PATH del
// BFF. /papi es absoluto (no lleva el prefijo). Se puede sobreescribir con VITE_BASE.
export default defineConfig({
  plugins: [react()],
  base: process.env.VITE_BASE || '/consola/',
  server: {
    port: 5174,
    proxy: {
      '/papi': 'http://127.0.0.1:8090',
    },
  },
})
