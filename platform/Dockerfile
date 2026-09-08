# Consola de super usuario de HUBERP (plataforma Mornix), en un solo contenedor:
# el BFF (Go/Fiber) expone /papi/* y sirve el SPA construido. Contexto de build = ./platform.
#
# Este subárbol es autocontenido y se separa a huberp-platform sin cambios.

# 1) Frontend (React + Vite) → dist estático.
FROM node:20-alpine AS fe
WORKDIR /fe
COPY frontend/package*.json ./
RUN npm ci || npm install
COPY frontend/ ./
RUN npm run build

# 2) BFF (Go) → binario estático.
FROM golang:1.22-alpine AS be
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# 3) Runtime mínimo: binario + SPA.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget && adduser -D -u 10001 app
COPY --from=be /out/server /usr/local/bin/server
COPY --from=fe /fe/dist /web
ENV PLATFORM_WEB_DIR=/web PLATFORM_PORT=8090
USER app
EXPOSE 8090
ENTRYPOINT ["server"]
