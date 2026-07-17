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
RUN mkdir -p /app/data && chown app:app /app/data
USER app
ENV DATABASE_PATH=data/reservas.db
VOLUME /app/data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD port="${ADDR##*:}"; wget -qO- "http://localhost:${port:-8080}/healthz" >/dev/null || exit 1
CMD ["salas"]
