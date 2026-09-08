# Frontend ElERP — la app (SPA Vite en /app) + proxy de /api,/auth,/papi,/consola.
# El SITIO PÚBLICO (landing + docs + legal) vive en el repo doc-elerp y se despliega
# aparte; Nginx Proxy Manager enruta / y /docs al contenedor del sitio, y /app,/api,
# /auth,/consola a este contenedor.
FROM node:20-alpine AS build
WORKDIR /app
COPY frontend/package*.json ./
RUN npm ci || npm install
COPY frontend/ ./
ARG VITE_API_BASE=""
ENV VITE_API_BASE=$VITE_API_BASE
# LINT ANTES DEL BUILD, y aquí dentro a propósito: es lo que hace imposible desplegar un
# bundle roto. `vite build` compila sin chistar un identificador que no existe (solo
# resuelve imports, no referencias), así que tres veces llegó a producción el módulo
# Restaurante en BLANCO por un helper borrado en un refactor — React no renderiza nada
# ante un ReferenceError y el usuario ve una pantalla vacía sin explicación.
# La regla que importa es `no-undef` (ver eslint.config.js). Si falla, la imagen no se
# construye: no depende de que alguien se acuerde de correr el linter.
RUN npm run lint
# Las pruebas de la lógica pura (src/lib) también corren acá: son 165 y tardan ~1 s.
RUN npm run test
RUN npm run build

FROM nginx:1.27-alpine
COPY frontend/nginx.conf /etc/nginx/conf.d/default.conf
# App ERP bajo /app (SPA con base=/app/).
COPY --from=build /app/dist /usr/share/nginx/html/app
EXPOSE 80
