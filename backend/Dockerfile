# Build Stage — FB_FAROL
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ .

RUN CGO_ENABLED=0 GOOS=linux go build -v -ldflags="-w -s" -o server .

# Run Stage
FROM alpine:latest
RUN apk --no-cache add tzdata ca-certificates && \
    ln -sf /usr/share/zoneinfo/America/Sao_Paulo /etc/localtime
# Usuário sem privilégio (21/10, migração de segurança) — o backend grava
# em disco (uploads/, backend/handlers/sp_csv.go e farol_v2_import.go),
# então, diferente de um serviço read-only, aqui é preciso garantir que o
# diretório de trabalho continue gravável pelo novo usuário. WORKDIR
# continua /root/ de propósito — é o path que o volume `api_uploads` do
# docker-compose.prod.yml já monta; só o dono muda.
RUN adduser -D -u 1000 appuser && chown appuser:appuser /root
WORKDIR /root/
COPY --from=builder --chown=appuser:appuser /app/server .
COPY --from=builder --chown=appuser:appuser /app/migrations ./migrations
USER appuser
EXPOSE 8082
CMD ["./server"]