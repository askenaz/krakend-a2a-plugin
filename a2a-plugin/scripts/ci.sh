#!/usr/bin/env bash
# Integración continua del plugin A2A.
#
# Comprueba el mismo código con los dos compiladores de destino:
#   - Go 1.26.8, el de KrakenD Community 2.13.11.
#   - Go 1.27.4, el de KrakenD Enterprise 3.0.4.
# Un uso de API posterior a Go 1.26 falla en "go vet" (el módulo declara
# go 1.26) y en la compilación de Community.
#
# Variables opcionales:
#   KRAKEND_EE_LICENSE_FILE  licencia de Enterprise, fuera del repositorio, para
#                            la prueba de carga del plugin en Enterprise
#   A2A_TEST_REDIS           host:puerto de un Redis para la prueba de integración
#   SKIP_IMAGES=1            no construir las imágenes
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) export MSYS_NO_PATHCONV=1; HOSTROOT=$(cygpath -w "$ROOT") ;;
  *) HOSTROOT=$ROOT ;;
esac

step() { printf '\n=== %s ===\n' "$*"; }

step "Formato"
unformatted=$(gofmt -l core plugin config/conformance deploy e2e 2>/dev/null || true)
if [ -n "$unformatted" ]; then echo "Sin formatear:"; echo "$unformatted"; exit 1; fi

step "go vet y pruebas con el Go local ($(go version | awk '{print $3}'))"
for m in core plugin config/conformance; do
  (cd "$m" && go vet ./... && go test -count=1 ./...)
done

step "Las pruebas de extremo a extremo compilan"
# No se ejecutan aquí: necesitan el entorno de deploy/compose levantado.
# Compilarlas detecta, en un clon limpio, un paquete que no haya llegado a git.
(cd e2e && go vet ./...)

step "El núcleo no tiene dependencias externas"
deps=$(cd core && go list -m all | grep -v '^krakend-a2a.local/core$' || true)
if [ -n "$deps" ]; then echo "Dependencias inesperadas en core: $deps"; exit 1; fi

step "Pruebas con Go 1.26.8 (compilador de Community 2.13.11)"
docker run --rm -e CGO_ENABLED=0 -v "$HOSTROOT:/src" -w /src/plugin krakend/builder:2.13.11 \
  sh -c 'go vet ./... && go test -count=1 ./... && cd ../core && go vet ./... && go test -count=1 ./...'

step "Pruebas con Go 1.27.4 (compilador de Enterprise 3.0.4)"
docker run --rm -e CGO_ENABLED=0 -v "$HOSTROOT:/src" -w /src/plugin krakend/builder-ee:3.0.4 \
  sh -c 'go vet ./... && go test -count=1 ./... && cd ../core && go vet ./... && go test -count=1 ./...'

if [ -n "${A2A_TEST_REDIS:-}" ]; then
  step "Integración con Redis ($A2A_TEST_REDIS)"
  (cd core && go test -count=1 -run TestRedisStore ./ownership/)
fi

if [ "${SKIP_IMAGES:-}" != "1" ]; then
  step "Imagen y artefactos de Community 2.13.11"
  docker build -f build/community/Dockerfile -t krakend-a2a:ce-2.13.11 .
  docker build -f build/community/Dockerfile --target artifacts -o type=local,dest=out/community .

  step "Imagen y artefactos de Enterprise 3.0.4"
  docker build -f build/enterprise/Dockerfile -t krakend-a2a:ee-3.0.4 .
  docker build -f build/enterprise/Dockerfile --target artifacts -o type=local,dest=out/enterprise .

  step "Cada edición rechaza el artefacto de la otra"
  if docker run --rm --entrypoint sh -v "$HOSTROOT/out/community:/a:ro" krakend-a2a:ee-3.0.4 \
       /opt/krakend/plugins/check-artifact.sh /a/krakend-a2a-ce-2.13.11.so; then
    echo "ERROR: Enterprise aceptó el artefacto de Community"; exit 1
  fi
  if docker run --rm --entrypoint sh -v "$HOSTROOT/out/enterprise:/a:ro" krakend-a2a:ce-2.13.11 \
       /opt/krakend/plugins/check-artifact.sh /a/krakend-a2a-ee-3.0.4.so; then
    echo "ERROR: Community aceptó el artefacto de Enterprise"; exit 1
  fi

  if [ -n "${KRAKEND_EE_LICENSE_FILE:-}" ]; then
    step "Prueba de carga en Enterprise con la licencia montada"
    bash build/enterprise/plugin-test.sh krakend-a2a:ee-3.0.4
  else
    echo "(sin KRAKEND_EE_LICENSE_FILE: se omite la prueba de carga en Enterprise)"
  fi
fi

step "Todo correcto"
