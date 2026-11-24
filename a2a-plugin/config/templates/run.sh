#!/bin/sh
# Assemble the source catalog before invoking KrakenD Flexible Configuration.
set -eu

if [ "$(id -u)" = 0 ]; then
  exec su-exec krakend "$0" "$@"
fi

source_settings=${FC_SETTINGS:?set FC_SETTINGS to the source settings directory}
generated=$(mktemp -d)
trap 'rm -rf "$generated"' EXIT

for file in "$source_settings"/*.json; do
  [ -f "$file" ] || continue
  [ "${file##*/}" != a2a.json ] || continue
  cp "$file" "$generated/"
done

/usr/local/bin/a2a-catalog "$source_settings" > "$generated/a2a.json"
export FC_SETTINGS="$generated"

# KrakenD becomes PID 1 in Compose. The temporary settings live for the
# lifetime of the container when exec succeeds.
exec krakend "$@"
