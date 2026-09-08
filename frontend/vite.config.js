/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// En dev, proxyeamos /api y /auth al backend Go para que frontend y backend
// compartan origen (localhost:5173) → las cookies de sesión (SameSite=Lax)
// viajan sin fricción. En producción se usa VITE_API_BASE.
//
// La PWA (manifest + service worker) es manual (public/manifest.webmanifest +
// public/sw.js), sin plugins: nivel 1 (instalable + app-shell offline).
export default defineConfig({
  // La app se sirve bajo /app (la raíz del dominio es la landing pública).
  // base reescribe las URLs de los assets a /app/assets/… en el build.
  base: '/app/',
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      // 127.0.0.1 (no 'localhost') para forzar IPv4 y evitar ECONNREFUSED ::1
      // cuando el backend Go escucha solo en IPv4.
      '/api': { target: 'http://127.0.0.1:8080', changeOrigin: true },
      '/auth': { target: 'http://127.0.0.1:8080', changeOrigin: true },
    },
  },
  // Vitest: pruebas unitarias de la lógica pura de src/lib/. Entorno `node`
  // (la lógica es pura: no toca DOM), globals para usar describe/it/expect sin
  // importarlos en cada archivo.
  test: {
    environment: 'node',
    globals: true,
    include: ['src/**/*.{test,spec}.{js,jsx}'],
  },
})
