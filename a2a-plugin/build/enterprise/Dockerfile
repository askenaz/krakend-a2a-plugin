# Plugin A2A para KrakenD Enterprise 3.0.
#
# Contexto de construcción: el directorio a2a-plugin.
#   docker build -f build/enterprise/Dockerfile -t krakend-a2a:ee-3.0.4 .
#
# La imagen no contiene la licencia de Enterprise. La prueba de carga con
# "krakend plugin test" la necesita, así que se ejecuta aparte con
# build/enterprise/plugin-test.sh y la licencia montada en tiempo de ejecución.

ARG KRAKEND_VERSION=3.0.4

FROM krakend/builder-ee:${KRAKEND_VERSION} AS build
ARG KRAKEND_VERSION
ARG PLUGIN_VERSION=0.1.0
WORKDIR /src
COPY core ./core
COPY plugin ./plugin
COPY build/check-artifact.sh /out/check-artifact.sh
WORKDIR /src/plugin
RUN go vet ./... && go test -count=1 ./... && cd ../core && go vet ./... && go test -count=1 ./...
RUN set -eu; \
    name="krakend-a2a-ee-${KRAKEND_VERSION}"; \
    go build -buildmode=plugin -ldflags "-X main.Version=${PLUGIN_VERSION}" -o "/out/${name}.so" .; \
    gov=$(go env GOVERSION | sed 's/^go//'); \
    libc="MUSL-$(ldd --version 2>&1 | sed -n 's/^Version //p' | head -n 1)_(alpine-$(cat /etc/alpine-release))"; \
    sha=$(sha256sum "/out/${name}.so" | cut -d' ' -f1); \
    arch=$(go env GOARCH); \
    { \
      printf '{\n'; \
      printf '  "plugin": "krakend-a2a",\n'; \
      printf '  "plugin_version": "%s",\n' "${PLUGIN_VERSION}"; \
      printf '  "edition": "ee",\n'; \
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

FROM krakend/krakend-ee:${KRAKEND_VERSION}
ARG KRAKEND_VERSION
LABEL maintainer="Autores del plugin A2A para KrakenD"
COPY --from=build /out/ /opt/krakend/plugins/
RUN sh /opt/krakend/plugins/check-artifact.sh "/opt/krakend/plugins/krakend-a2a-ee-${KRAKEND_VERSION}.so"
