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
RUN npm run build

FROM nginx:1.27-alpine
COPY frontend/nginx.conf /etc/nginx/conf.d/default.conf
# App ERP bajo /app (SPA con base=/app/).
COPY --from=build /app/dist /usr/share/nginx/html/app
EXPOSE 80
