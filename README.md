# FB_FAROL

Sistema FB_FAROL — FBTax Cloud

## Stack

- **Backend:** Go 1.26 + PostgreSQL 15 + Redis
- **Frontend:** React 18 + Vite + TypeScript + Tailwind CSS + shadcn/ui
- **Infra:** Docker + Coolify (Hostinger) + Traefik

## URLs

- **Produção:** https://farol.fbtax.cloud
- **Porta dev backend:** 8083

## Desenvolvimento Local

```bash
# 1. Configure o banco
sudo bash setup_db_local.sh

# 2. Configure o .env do backend
cp backend/.env.example backend/.env
# edite backend/.env conforme necessário

# 3. Suba o ambiente
./dev.sh
```

## Docker (dev)

```bash
docker compose up --build
```

## Deploy (Coolify)

Configure as variáveis do `coolify-env-template.txt` no painel Coolify.
