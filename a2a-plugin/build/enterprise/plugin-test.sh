#!/usr/bin/env bash
# Prueba de carga del plugin en KrakenD Enterprise, con la licencia montada en
# tiempo de ejecución. La licencia nunca entra en la imagen ni en el repositorio.
#
# Uso:
#   KRAKEND_EE_LICENSE_FILE=/ruta/fuera/del/repo/LICENSE build/enterprise/plugin-test.sh [imagen]
set -euo pipefail

IMAGE="${1:-krakend-a2a:ee-3.0.4}"
LICENSE_FILE="${KRAKEND_EE_LICENSE_FILE:?Define KRAKEND_EE_LICENSE_FILE con la ruta a la licencia, fuera del repositorio}"
[ -f "$LICENSE_FILE" ] || { echo "ERROR: no existe el fichero indicado en KRAKEND_EE_LICENSE_FILE" >&2; exit 1; }

case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) LICENSE_FILE=$(cygpath -m "$LICENSE_FILE"); export MSYS_NO_PATHCONV=1 ;;
esac

docker run --rm --entrypoint sh \
  -e KRAKEND_LICENSE_PATH=/run/krakend-license/LICENSE \
  -v "$LICENSE_FILE:/run/krakend-license/LICENSE:ro" \
  "$IMAGE" -c '
    set -eu
    so=$(ls /opt/krakend/plugins/krakend-a2a-ee-*.so)
    m="${so%.so}.manifest.json"
    gov=$(sed -n "s/.*\"go_version\": \"\(.*\)\".*/\1/p" "$m")
    libc=$(sed -n "s/.*\"libc\": \"\(.*\)\".*/\1/p" "$m")
    : > /tmp/go.sum
    krakend plugin check -g "$gov" -l "$libc" -s /tmp/go.sum
    krakend plugin test -sm "$so"
  ' 2>&1 | grep -vi licen
