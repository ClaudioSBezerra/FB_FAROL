-- Migration 124: renomeia schema smartpick → farol
-- O código Go referencia todas as tabelas SP como farol.*, mas as migrations
-- anteriores criaram o schema com o nome antigo "smartpick". Esta migration
-- alinha o schema ao nome esperado pelo código.

ALTER SCHEMA smartpick RENAME TO farol;
