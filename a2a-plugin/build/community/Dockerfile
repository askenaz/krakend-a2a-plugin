# Plugin A2A para KrakenD Community 2.13.
#
# Contexto de construcción: el directorio a2a-plugin.
#   docker build -f build/community/Dockerfile -t krakend-a2a:ce-<versión> .
#
# El valor por defecto de KRAKEND_VERSION es una copia de KRAKEND_CE_VERSION de
# versions.env, para que el comando anterior funcione sin argumentos. Las
# pruebas de config/conformance fallan si las dos dejan de coincidir.
# Para otra versión: --build-arg KRAKEND_VERSION=<versión>.
#
# Etapas:
#   build      compila y prueba con el mismo Go que el binario de KrakenD
#   artifacts  solo el .so, su manifiesto y el script de comprobación
#   (final)    imagen oficial de KrakenD Community con el plugin instalado

ARG KRAKEND_VERSION=2.13.11

FROM krakend/builder:${KRAKEND_VERSION} AS build
ARG KRAKEND_VERSION
ARG PLUGIN_VERSION=0.1.0
WORKDIR /src
COPY core ./core
COPY plugin ./plugin
COPY build/check-artifact.sh /out/check-artifact.sh
WORKDIR /src/plugin
# Las pruebas se ejecutan con el compilador exacto de esta edición: un uso de
# API posterior a su versión de Go rompe aquí la construcción.
RUN go vet ./... && go test -count=1 ./... && cd ../core && go vet ./... && go test -count=1 ./...
RUN set -eu; \
    name="krakend-a2a-ce-${KRAKEND_VERSION}"; \
    go build -buildmode=plugin -ldflags "-X main.Version=${PLUGIN_VERSION}" -o "/out/${name}.so" .; \
    gov=$(go env GOVERSION | sed 's/^go//'); \
    libc="MUSL-$(ldd --version 2>&1 | sed -n 's/^Version //p' | head -n 1)_(alpine-$(cat /etc/alpine-release))"; \
    sha=$(sha256sum "/out/${name}.so" | cut -d' ' -f1); \
    arch=$(go env GOARCH); \
    { \
      printf '{\n'; \
      printf '  "plugin": "krakend-a2a",\n'; \
      printf '  "plugin_version": "%s",\n' "${PLUGIN_VERSION}"; \
      printf '  "edition": "ce",\n'; \
      printf '  "krakend_version": "%s",\n' "${KRAKEND_VERSION}"; \
      printf '  "go_version": "%s",\n' "${gov}"; \
      printf '  "libc": "%s",\n' "${libc}"; \
      printf '  "arch": "%s",\n' "${arch}"; \
      printf '  "sha256": "%s"\n' "${sha}"; \
      printf '}\n'; \
    } > "/out/${name}.manifest.json"; \
    cat "/out/${name}.manifest.json"

FROM scratch AS artifacts
COPY --from=build /out/ /

FROM krakend:${KRAKEND_VERSION}
ARG KRAKEND_VERSION
LABEL maintainer="Autores del plugin A2A para KrakenD"
COPY --from=build /out/ /opt/krakend/plugins/
# Comprobaciones previas al despliegue con el binario oficial de destino.
RUN set -eu; \
    so="/opt/krakend/plugins/krakend-a2a-ce-${KRAKEND_VERSION}.so"; \
    m="${so%.so}.manifest.json"; \
    sh /opt/krakend/plugins/check-artifact.sh "$so"; \
    gov=$(sed -n 's/.*"go_version": "\(.*\)".*/\1/p' "$m"); \
    libc=$(sed -n 's/.*"libc": "\(.*\)".*/\1/p' "$m"); \
    : > /tmp/go.sum; \
    krakend check-plugin -g "$gov" -l "$libc" -s /tmp/go.sum; \
    rm -f /tmp/go.sum; \
    krakend test-plugin -sm "$so"
