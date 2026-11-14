#!/usr/bin/env bash
# Prueba de carga (tarea 10.3) contra el entorno de deploy/compose.
#
# Uso: e2e/load.sh <ce|ee> [duración]
#
# Mide la memoria de cada nodo antes y después, y comprueba que no quedan
# entradas de correlación vivas al terminar.
set -euo pipefail
EDITION="${1:?uso: load.sh <ce|ee> [duración]}"
DURATION="${2:-30s}"
case "$EDITION" in
  ce) BASE=18000; PROJECT=a2a-demo-ce ;;
  ee) BASE=28000; PROJECT=a2a-demo-ee ;;
  *) echo "edición desconocida" >&2; exit 2 ;;
esac
cd "$(dirname "$0")"
NODES="${PROJECT}-gateway-a-1 ${PROJECT}-gateway-b-1"

mem() { docker stats --no-stream --format '{{.Name}} {{.MemUsage}}' $NODES; }
live() {
  for n in $NODES; do
    printf '%s ' "$n"
    docker exec "$n" wget -qO- http://127.0.0.1:8080/__a2a/_stats 2>&1 | grep -vi licen
    echo
  done
}

echo "=== Memoria antes ==="; mem
echo "=== Carga durante $DURATION ==="
go run ./cmd/load -gw "http://localhost:$((BASE+80)),http://localhost:$((BASE+81))" -idp "http://localhost:$((BASE+90))" -duration "$DURATION" -concurrency 16
echo "=== Memoria después ==="; mem
sleep 2
echo "=== Entradas de correlación vivas ==="; out=$(live); echo "$out"
if echo "$out" | grep -q '"liveEntries":[1-9]'; then
  echo "ERROR: quedan entradas de correlación sin liberar" >&2; exit 1
fi
echo "OK"
