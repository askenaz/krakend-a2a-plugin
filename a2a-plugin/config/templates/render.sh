#!/usr/bin/env bash
# Renderiza y comprueba la configuración de una edición con el KrakenD real.
#
# Uso: config/templates/render.sh <community|enterprise> [public_url] [settings_dir]
# Escribe la configuración generada en out/rendered-<edición>.json.
# "krakend check" no necesita licencia en Enterprise.
# Usa el renderizador construido con la versión de versions.env.
set -euo pipefail
EDITION="${1:?uso: render.sh <community|enterprise> [public_url] [settings_dir]}"
PUBLIC_URL="${2:-http://localhost:8080}"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
SETTINGS=$(cd "${3:-$ROOT/config/templates/settings}" && pwd)
. "$ROOT/scripts/versions.sh"
case "$EDITION" in
  community) VERSION=$KRAKEND_CE_VERSION ;;
  enterprise) VERSION=$KRAKEND_EE_VERSION ;;
  *) echo "edición desconocida: $EDITION" >&2; exit 2 ;;
esac
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*)
    export MSYS_NO_PATHCONV=1
    HROOT=$(cygpath -w "$ROOT")
    HSETTINGS=$(cygpath -w "$SETTINGS")
    ;;
  *) HROOT=$ROOT; HSETTINGS=$SETTINGS ;;
esac
IMAGE="krakend-a2a-config:$EDITION-$VERSION"
docker build --target config-renderer --build-arg KRAKEND_VERSION="$VERSION" \
  -f "$HROOT/build/$EDITION/Dockerfile" -t "$IMAGE" "$HROOT"
mkdir -p "$ROOT/out"
output=$(mktemp "$ROOT/out/.rendered-$EDITION.XXXXXX")
trap 'rm -f "$output"' EXIT
docker run --rm --entrypoint sh \
  -e FC_ENABLE=1 -e FC_SETTINGS=/etc/krakend/settings -e FC_TEMPLATES=/etc/krakend/templates \
  -e FC_OUT=/tmp/rendered.json -e A2A_PUBLIC_URL="$PUBLIC_URL" \
  -v "$HSETTINGS:/etc/krakend/settings:ro" \
  -v "$HROOT/config/templates/templates:/etc/krakend/templates:ro" \
  -v "$HROOT/config/templates/$EDITION/krakend.tmpl:/etc/krakend/krakend.tmpl:ro" \
  "$IMAGE" -ec '/usr/local/bin/a2a-config check -t -c /etc/krakend/krakend.tmpl >&2; cat "$FC_OUT"' > "$output"
chmod 644 "$output"
mv "$output" "$ROOT/out/rendered-$EDITION.json"
