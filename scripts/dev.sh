#!/usr/bin/env bash
#
# Développement : fait tourner en parallèle
#   - l'API Go sur le port 8080 (sans interface, seule l'API répond)
#   - le front Vite (HMR) sur le port 3000, qui relaie /api vers le port 8080
#
# Ctrl+C arrête proprement les deux processus.

set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [ ! -f "$ROOT/.env" ]; then
  echo "[dev] ATTENTION : $ROOT/.env absent — l'API Go risque de ne pas démarrer." >&2
fi

PIDS=()
cleanup() {
  echo ""
  echo "[dev] Arrêt des processus…"
  for pid in "${PIDS[@]:-}"; do
    kill "$pid" 2>/dev/null || true
  done
}
trap cleanup EXIT INT TERM

echo "[dev] API Go  → http://localhost:8080/api/v1"
# STATIC_DIR vide : le Go ne sert que l'API en dev ; le front est fourni par Vite.
( cd "$ROOT/backend" && API_PORT=8080 STATIC_DIR= go run . ) &
PIDS+=($!)

echo "[dev] Front Vite → http://localhost:3000"
npm run dev:web &
PIDS+=($!)

wait
