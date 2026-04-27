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
WORKDIR /root/
COPY --from=builder /app/server .
COPY --from=builder /app/migrations ./migrations
EXPOSE 8082
CMD ["./server"]