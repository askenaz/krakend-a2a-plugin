#!/usr/bin/env bash
# Renderiza y comprueba la configuración de una edición con el KrakenD real.
#
# Uso: config/templates/render.sh <community|enterprise> [public_url]
# Escribe la configuración generada en out/rendered-<edición>.json.
# "krakend check" no necesita licencia en Enterprise.
set -euo pipefail
EDITION="${1:?uso: render.sh <community|enterprise> [public_url]}"
PUBLIC_URL="${2:-http://localhost:8080}"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
case "$EDITION" in
  community) IMAGE=krakend:2.13.11 ;;
  enterprise) IMAGE=krakend/krakend-ee:3.0.4 ;;
  *) echo "edición desconocida: $EDITION" >&2; exit 2 ;;
esac
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) export MSYS_NO_PATHCONV=1; HROOT=$(cygpath -w "$ROOT") ;;
  *) HROOT=$ROOT ;;
esac
mkdir -p "$ROOT/out"
docker run --rm \
  -e FC_ENABLE=1 -e FC_SETTINGS=/etc/krakend/settings -e FC_TEMPLATES=/etc/krakend/templates \
  -e FC_OUT=/out/rendered-"$EDITION".json -e A2A_PUBLIC_URL="$PUBLIC_URL" \
  -v "$HROOT/config/templates/settings:/etc/krakend/settings:ro" \
  -v "$HROOT/config/templates/templates:/etc/krakend/templates:ro" \
  -v "$HROOT/config/templates/$EDITION/krakend.tmpl:/etc/krakend/krakend.tmpl:ro" \
  -v "$HROOT/out:/out" \
  "$IMAGE" check -t -c /etc/krakend/krakend.tmpl 2>&1 | grep -vi licen
