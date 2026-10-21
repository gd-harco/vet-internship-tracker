# ── Stage 1 : build ──────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

WORKDIR /build

# Télécharger les dépendances en cache séparé
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Copier le code source et compiler un binaire statique
COPY backend/ .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o vet-tracker ./cmd/server

# ── Stage 2 : runtime ─────────────────────────────────────────────────────────
FROM alpine:3.21

RUN addgroup -S app && adduser -S app -G app

WORKDIR /app

# Binaire
COPY --from=builder /build/vet-tracker .

# Templates et assets statiques (chargés depuis le filesystem à l'exécution)
COPY --from=builder /build/internal/templates ./internal/templates
COPY --from=builder /build/static             ./static

# Répertoire de données (la DB SQLite y sera montée via volume)
RUN mkdir -p /data && chown app:app /data /app

USER app

EXPOSE 8080

ENV PORT=8080
ENV DB_PATH=/data/data.db

ENTRYPOINT ["./vet-tracker"]
