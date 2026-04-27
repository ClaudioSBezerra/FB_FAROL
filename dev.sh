#!/usr/bin/env bash
# dev.sh — sobe backend + frontend em localhost para desenvolvimento
# Uso: ./dev.sh

set -e

ROOT="$(cd "$(dirname "$0")" && pwd)"
BACKEND="$ROOT/backend"
FRONTEND="$ROOT/frontend"

# Go SDK instalado em ~/go-sdk
export GOROOT="$HOME/go-sdk"
export PATH="$GOROOT/bin:$PATH"

# Carrega variáveis do .env do backend
set -a
# shellcheck disable=SC1091
source "$BACKEND/.env"
set +a

echo "======================================================"
echo "  FB_FAROL — Ambiente de Desenvolvimento Local"
echo "======================================================"
echo "  Backend  → http://localhost:$PORT"
echo "  Frontend → http://localhost:3000"
echo "  Banco    → $DATABASE_URL"
echo "======================================================"
echo ""

# Verifica PostgreSQL
if ! pg_isready -h localhost -p 5432 -q 2>/dev/null; then
  echo "[ERRO] PostgreSQL não está respondendo em localhost:5432"
  echo "       Inicie com: sudo service postgresql start"
  exit 1
fi

# Mata processos anteriores nas portas usadas (se houver)
fuser -k "$PORT/tcp" 2>/dev/null || true
fuser -k 3000/tcp 2>/dev/null || true

# Inicia backend em background
echo "[backend] Iniciando (go run) na porta $PORT..."
(
  cd "$BACKEND"
  go run . 2>&1 | sed 's/^/[backend] /'
) &
BACKEND_PID=$!

# Aguarda backend responder
echo "[backend] Aguardando subir..."
for i in $(seq 1 20); do
  if curl -sf "http://localhost:$PORT/health" >/dev/null 2>&1 || \
     curl -sf "http://localhost:$PORT/api/health" >/dev/null 2>&1; then
    echo "[backend] Pronto!"
    break
  fi
  sleep 1
done

# Inicia frontend
echo "[frontend] Iniciando Vite na porta 3000..."
(
  cd "$FRONTEND"
  npm run dev 2>&1 | sed 's/^/[frontend] /'
) &
FRONTEND_PID=$!

echo ""
echo "  Pressione Ctrl+C para encerrar tudo."
echo ""

# Encerra ambos ao sair
trap "echo ''; echo 'Encerrando...'; kill $BACKEND_PID $FRONTEND_PID 2>/dev/null; exit 0" INT TERM

wait
