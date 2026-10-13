#!/bin/bash
# Crée la base de test si elle n'existe pas encore (appelé par l'image Postgres au premier démarrage).
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    SELECT 'CREATE DATABASE washguard_test OWNER $POSTGRES_USER'
    WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'washguard_test')\gexec
EOSQL
