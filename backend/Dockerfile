# Backend HUBERP — build multi-stage (Go 1.22 → alpine).
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget && adduser -D -u 10001 app \
    # Bucket de archivos por cliente: la carpeta debe existir con el dueño
    # correcto ANTES de USER app, para que el volumen nombrado herede el
    # propietario y el proceso pueda escribir sin correr como root.
    && mkdir -p /data/archivos && chown -R app:app /data
COPY --from=build /out/api /usr/local/bin/api
USER app
EXPOSE 8080
ENTRYPOINT ["api"]
