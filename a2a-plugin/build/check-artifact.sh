#!/bin/sh
# Comprueba, antes de desplegar, que un artefacto del plugin A2A corresponde al
# binario de KrakenD que lo va a cargar.
#
# Cargar el .so de otra edición o versión aborta KrakenD con un error fatal de
# Go que las opciones de carga de plugins no contienen (hallazgo de la fase 0).
# Este script lo detecta antes y explica la discrepancia.
#
# Uso, dentro de la imagen de KrakenD de destino:
#   sh check-artifact.sh /opt/krakend/plugins/krakend-a2a-<edición>-<versión>.so
#
# Lee el manifiesto que acompaña al artefacto (mismo nombre, .manifest.json).
# No necesita licencia: krakend version funciona sin ella.
set -eu

SO="${1:?uso: check-artifact.sh <ruta al .so>}"
MANIFEST="${SO%.so}.manifest.json"
KRAKEND="${KRAKEND_BIN:-krakend}"

fail() { echo "ERROR: $*" >&2; exit 1; }

[ -f "$SO" ] || fail "no existe el artefacto $SO"
[ -f "$MANIFEST" ] || fail "no existe el manifiesto $MANIFEST"

field() { sed -n "s/.*\"$1\": *\"\([^\"]*\)\".*/\1/p" "$MANIFEST" | head -n 1; }

want_edition=$(field edition)
want_krakend=$(field krakend_version)
want_go=$(field go_version)
want_libc=$(field libc)
want_arch=$(field arch)
want_sha=$(field sha256)

out=$("$KRAKEND" version 2>&1) || fail "no se pudo ejecutar '$KRAKEND version'"
have_krakend=$(echo "$out" | sed -n 's/^KrakenD Version: *//p' | head -n 1)
have_go=$(echo "$out" | sed -n 's/^Go Version: *//p' | head -n 1)
have_libc=$(echo "$out" | sed -n 's/^Glibc Version: *//p' | head -n 1)
case "$have_krakend" in
  *-ee) have_edition=ee; have_krakend=${have_krakend%-ee} ;;
  *) have_edition=ce ;;
esac
case "$(uname -m)" in
  x86_64) have_arch=amd64 ;;
  aarch64|arm64) have_arch=arm64 ;;
  *) have_arch=$(uname -m) ;;
esac

errors=0
check() {
  if [ "$2" != "$3" ]; then
    echo "DISCREPANCIA en $1: el artefacto es para '$2' y el binario es '$3'" >&2
    errors=$((errors + 1))
  fi
}
check "edición" "$want_edition" "$have_edition"
check "versión de KrakenD" "$want_krakend" "$have_krakend"
check "versión de Go" "$want_go" "$have_go"
check "arquitectura" "$want_arch" "$have_arch"
case "$have_libc" in
  "$want_libc"*) ;;
  *) echo "DISCREPANCIA en libc: el artefacto es para '$want_libc' y el binario usa '$have_libc'" >&2; errors=$((errors + 1)) ;;
esac

have_sha=$(sha256sum "$SO" | cut -d' ' -f1)
check "sha256 del artefacto" "$want_sha" "$have_sha"

if [ "$errors" -gt 0 ]; then
  fail "el artefacto $(basename "$SO") no se puede cargar en este KrakenD ($errors discrepancia(s))"
fi
echo "OK: $(basename "$SO") corresponde a KrakenD $have_edition $have_krakend, Go $have_go, $have_libc, $have_arch"
