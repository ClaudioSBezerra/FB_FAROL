#!/usr/bin/env bash
# setup_db_local.sh — cria banco e usuário local para desenvolvimento
# Rode UMA VEZ como root/sudo após instalar o PostgreSQL:
#   sudo bash setup_db_local.sh

set -e

DB_NAME="fb_farol"
DB_USER="postgres"

echo "Criando banco de dados local: $DB_NAME"

# Garante que o banco existe
sudo -u postgres psql -tc "SELECT 1 FROM pg_database WHERE datname='$DB_NAME'" | grep -q 1 \
  || sudo -u postgres psql -c "CREATE DATABASE $DB_NAME;"

echo "Banco '$DB_NAME' pronto."
echo ""
echo "String de conexão para backend/.env:"
echo "  DATABASE_URL=postgres://$DB_USER:postgres@localhost:5432/$DB_NAME?sslmode=disable"
echo ""
echo "Se a senha do usuário 'postgres' for diferente de 'postgres', ajuste backend/.env"
