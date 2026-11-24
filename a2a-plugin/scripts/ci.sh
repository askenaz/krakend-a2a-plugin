#!/usr/bin/env bash
# Integración continua del plugin A2A.
#
# Comprueba el mismo código con los dos compiladores de destino: el de la imagen
# de compilación de KrakenD Community y el de la de KrakenD Enterprise. Las
# versiones salen de versions.env. Un uso de API posterior a Go 1.26 falla en
# "go vet" (el módulo declara go 1.26) y en la compilación de Community.
#
# Variables opcionales:
#   KRAKEND_CE_VERSION       versión de Community, en lugar de la de versions.env
#   KRAKEND_EE_VERSION       versión de Enterprise, en lugar de la de versions.env
#   KRAKEND_EE_LICENSE_FILE  licencia de Enterprise, fuera del repositorio, para
#                            la prueba de carga del plugin en Enterprise
#   A2A_TEST_REDIS           host:puerto de un Redis para la prueba de integración
#   SKIP_IMAGES=1            no construir las imágenes
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"
. scripts/versions.sh
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) export MSYS_NO_PATHCONV=1; HOSTROOT=$(cygpath -w "$ROOT") ;;
  *) HOSTROOT=$ROOT ;;
esac

step() { printf '\n=== %s ===\n' "$*"; }

# Prueba el núcleo y el adaptador dentro de una imagen de compilación.
builder_tests() { # imagen
  docker run --rm -e CGO_ENABLED=0 -v "$HOSTROOT:/src" -w /src/plugin "$1" \
    sh -c 'go version && go vet ./... && go test -count=1 ./... && cd ../core && go vet ./... && go test -count=1 ./...'
}

step "Versiones: Community $KRAKEND_CE_VERSION y Enterprise $KRAKEND_EE_VERSION"

step "Formato"
unformatted=$(gofmt -l core plugin config/catalog config/conformance deploy e2e 2>/dev/null || true)
if [ -n "$unformatted" ]; then echo "Sin formatear:"; echo "$unformatted"; exit 1; fi

step "go vet y pruebas con el Go local ($(go version | awk '{print $3}'))"
for m in core plugin config/catalog config/conformance; do
  (cd "$m" && go vet ./... && go test -count=1 ./...)
done

step "Las pruebas de extremo a extremo compilan"
# No se ejecutan aquí: necesitan el entorno de deploy/compose levantado.
# Compilarlas detecta, en un clon limpio, un paquete que no haya llegado a git.
(cd e2e && go vet ./...)

step "Plantillas de Community y Enterprise"
bash config/templates/render.sh community
bash config/templates/render.sh enterprise
(cd config/conformance && go test -count=1 -run '^TestRenderedTemplates$')

step "El núcleo no tiene dependencias externas"
deps=$(cd core && go list -m all | grep -v '^krakend-a2a.local/core$' || true)
if [ -n "$deps" ]; then echo "Dependencias inesperadas en core: $deps"; exit 1; fi

step "Pruebas con el compilador de Community $KRAKEND_CE_VERSION"
builder_tests "$KRAKEND_CE_BUILDER"

step "Pruebas con el compilador de Enterprise $KRAKEND_EE_VERSION"
builder_tests "$KRAKEND_EE_BUILDER"

if [ -n "${A2A_TEST_REDIS:-}" ]; then
  step "Integración con Redis ($A2A_TEST_REDIS)"
  (cd core && go test -count=1 -run TestRedisStore ./ownership/)
fi

if [ "${SKIP_IMAGES:-}" != "1" ]; then
  step "Imagen y artefactos de Community $KRAKEND_CE_VERSION"
  docker build -f build/community/Dockerfile --build-arg KRAKEND_VERSION="$KRAKEND_CE_VERSION" -t "$A2A_CE_IMAGE" .
  docker build -f build/community/Dockerfile --build-arg KRAKEND_VERSION="$KRAKEND_CE_VERSION" \
    --target artifacts -o type=local,dest=out/community .

  step "Imagen y artefactos de Enterprise $KRAKEND_EE_VERSION"
  docker build -f build/enterprise/Dockerfile --build-arg KRAKEND_VERSION="$KRAKEND_EE_VERSION" -t "$A2A_EE_IMAGE" .
  docker build -f build/enterprise/Dockerfile --build-arg KRAKEND_VERSION="$KRAKEND_EE_VERSION" \
    --target artifacts -o type=local,dest=out/enterprise .

  step "Cada edición rechaza el artefacto de la otra"
  if docker run --rm --entrypoint sh -v "$HOSTROOT/out/community:/a:ro" "$A2A_EE_IMAGE" \
       /opt/krakend/plugins/check-artifact.sh "/a/$A2A_CE_ARTIFACT.so"; then
    echo "ERROR: Enterprise aceptó el artefacto de Community"; exit 1
  fi
  if docker run --rm --entrypoint sh -v "$HOSTROOT/out/enterprise:/a:ro" "$A2A_CE_IMAGE" \
       /opt/krakend/plugins/check-artifact.sh "/a/$A2A_EE_ARTIFACT.so"; then
    echo "ERROR: Community aceptó el artefacto de Enterprise"; exit 1
  fi

  if [ -n "${KRAKEND_EE_LICENSE_FILE:-}" ]; then
    step "Prueba de carga en Enterprise con la licencia montada"
    bash build/enterprise/plugin-test.sh "$A2A_EE_IMAGE"
  else
    echo "(sin KRAKEND_EE_LICENSE_FILE: se omite la prueba de carga en Enterprise)"
  fi
fi

step "Todo correcto: Community $KRAKEND_CE_VERSION y Enterprise $KRAKEND_EE_VERSION"
