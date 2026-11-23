#!/usr/bin/env bash
# Carga las versiones de KrakenD de versions.env. Se usa con "source":
#   . scripts/versions.sh
#
# Una variable ya definida en el entorno tiene prioridad sobre el fichero, para
# probar otra versión sin editarlo:
#   KRAKEND_EE_VERSION=3.2.0 bash scripts/ci.sh
#
# Además de las variables del fichero, define los nombres que dependen de ellas:
#   KRAKEND_CE_BUILDER, KRAKEND_EE_BUILDER   imágenes de compilación
#   KRAKEND_CE_RUNTIME, KRAKEND_EE_RUNTIME   imágenes oficiales de ejecución
#   A2A_CE_IMAGE, A2A_EE_IMAGE               imágenes de KrakenD con el plugin
#   A2A_CE_ARTIFACT, A2A_EE_ARTIFACT         nombre del .so, sin extensión

_versions_file="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/versions.env"
if [ ! -f "$_versions_file" ]; then
  echo "ERROR: no existe $_versions_file" >&2
  return 1
fi
while IFS='=' read -r _key _value || [ -n "$_key" ]; do
  # El fichero puede llegar con fin de línea de Windows.
  _key=${_key%$'\r'}
  _value=${_value%$'\r'}
  case "$_key" in '' | \#*) continue ;; esac
  if [ -z "${!_key:-}" ]; then export "$_key=$_value"; fi
done <"$_versions_file"

# Cada artefacto solo carga en una versión exacta, así que no valen etiquetas
# móviles como "3.0" o "latest": el manifiesto no coincidiría con el binario.
for _key in KRAKEND_CE_VERSION KRAKEND_EE_VERSION; do
  if ! [[ "${!_key:-}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "ERROR: $_key='${!_key:-}' no es una versión exacta x.y.z" >&2
    return 1
  fi
done
unset _versions_file _key _value

export KRAKEND_CE_BUILDER="krakend/builder:$KRAKEND_CE_VERSION"
export KRAKEND_CE_RUNTIME="krakend:$KRAKEND_CE_VERSION"
export KRAKEND_EE_BUILDER="krakend/builder-ee:$KRAKEND_EE_VERSION"
export KRAKEND_EE_RUNTIME="krakend/krakend-ee:$KRAKEND_EE_VERSION"
export A2A_CE_IMAGE="krakend-a2a:ce-$KRAKEND_CE_VERSION"
export A2A_EE_IMAGE="krakend-a2a:ee-$KRAKEND_EE_VERSION"
export A2A_CE_ARTIFACT="krakend-a2a-ce-$KRAKEND_CE_VERSION"
export A2A_EE_ARTIFACT="krakend-a2a-ee-$KRAKEND_EE_VERSION"
