FROM golang:1.24-alpine AS build
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /salas ./cmd/server
FROM alpine:3.21
RUN adduser -D -u 10001 app
COPY --from=build /salas /usr/local/bin/salas
COPY --from=build /app/web /app/web
WORKDIR /app
RUN mkdir -p /data && chown app:app /data
USER app
ENV DATABASE_PATH=/data/reservas.db
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://localhost:8080/ >/dev/null || exit 1
CMD ["salas"]
