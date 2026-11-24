# Guía de desarrollo

[Índice de guías](README.md) · [Read in English](en/development.md)

## Requisitos

- Docker con Docker Compose.
- Go 1.26 o superior para compilar y probar el núcleo en el equipo local.
- Los plugins `.so` no se compilan en Windows: se compilan siempre en Docker, con la imagen de compilación de cada edición.

## Imágenes por edición

| Uso | Community 2.13.11 | Enterprise 3.0.4 |
|---|---|---|
| Ejecución | `krakend:2.13.11` | `krakend/krakend-ee:3.0.4` |
| Compilación del plugin | `krakend/builder:2.13.11` | `krakend/builder-ee:3.0.4` |

Cada artefacto solo carga en el binario de su edición y versión. Un artefacto equivocado aborta el arranque del gateway.

## Versiones de KrakenD

Las versiones se fijan en un único fichero, `versions.env`:

| Variable | Qué fija |
|---|---|
| `KRAKEND_CE_VERSION` | Versión de Community: imágenes `krakend/builder` y `krakend` |
| `KRAKEND_EE_VERSION` | Versión de Enterprise: imágenes `krakend/builder-ee` y `krakend/krakend-ee` |
| `CI_GO_VERSION` | Go del equipo de integración continua, para el formato y las pruebas fuera de Docker |

`scripts/ci.sh`, `config/templates/render.sh`, `build/enterprise/plugin-test.sh` y GitHub Actions lo leen al ejecutarse. Docker y la documentación no pueden leerlo, así que llevan una copia:

- El valor por defecto de `KRAKEND_VERSION` en los dos Dockerfiles.
- El valor por defecto de la versión en los ficheros de Compose.
- Los nombres de imagen y de artefacto en los comandos de la documentación.

Las pruebas de `config/conformance` fallan si alguna copia nombra otra versión, e indican el fichero y la línea. No revisan el prototipo ni `validacion-supuestos.md`, que son el registro de la fase 0.

### Probar una versión nueva de KrakenD

Una versión nueva no exige cambiar el código del plugin, pero sí compilarlo de nuevo: cada artefacto solo carga en su versión exacta. Para probarla sin tocar ningún fichero, define la variable al ejecutar:

```bash
KRAKEND_EE_VERSION=3.2.0 bash scripts/ci.sh
```

El script compila el plugin con la imagen de compilación de esa versión, ejecuta las pruebas con su Go y comprueba el artefacto en su imagen oficial. Con `KRAKEND_EE_LICENSE_FILE` definida, prueba además la carga del plugin. Tiene que existir la imagen `krakend/builder-ee` de esa versión, y la versión debe ser exacta: no valen `3.2` ni `latest`.

La misma variable sirve para las plantillas y para el entorno de extremo a extremo:

```bash
KRAKEND_EE_VERSION=3.2.0 bash config/templates/render.sh enterprise
```

```bash
KRAKEND_EE_VERSION=3.2.0 docker compose -f deploy/compose/enterprise.yml up -d --build
```

En GitHub, el workflow `a2a-plugin` se puede lanzar a mano con la versión como parámetro.

Todo el contacto con KrakenD está en el adaptador, `plugin/`. Si una versión cambia alguno de estos puntos, la corrección va ahí y no en `core/`:

| Punto de contacto | Dónde está | Cómo se nota si cambia |
|---|---|---|
| Firmas de `RegisterHandlers` y `RegisterModifiers` | `plugin/main.go` | Falla la prueba de carga del plugin |
| Forma del mapa de configuración que recibe el plugin | `pluginConfig`, en `plugin/main.go` | KrakenD no arranca o fallan las pruebas de extremo a extremo |
| Envoltorio de la petición que recibe el modificador | `headerReader` y `contextReader`, en `plugin/main.go` | Los endpoints internos responden 500 y fallan las pruebas de extremo a extremo |
| Logger que entrega KrakenD | `krakendLogger`, en `plugin/logger.go` | Nada falla: los registros del plugin salen sin el formato de KrakenD. Hay que mirarlo en los registros |
| Formato de `krakend.json` y comandos del CLI | `config/templates/` y `build/` | Falla `render.sh` o la construcción de la imagen |

### Adoptar una versión

1. Cambia la variable en `versions.env`.
2. Ejecuta `go test ./...` en `config/conformance` y actualiza cada referencia que liste.
3. Ejecuta `bash scripts/ci.sh` y las pruebas de extremo a extremo.
4. Añade la fila de la versión a `compatibilidad.md`, con el Go y la libc del manifiesto del artefacto.
5. Revisa el texto que nombra la versión anterior: `git grep -n "<versión anterior>"`.

## Licencia de KrakenD Enterprise

Reglas:

- La licencia vive **fuera del repositorio**, en una ruta que elige cada desarrollador.
- Nunca se copia al repositorio, a una imagen ni a un artefacto de pruebas. `.gitignore` y `.dockerignore` excluyen los nombres habituales de ficheros de licencia como red de seguridad, no como mecanismo principal.
- Se monta solo en tiempo de ejecución, en modo lectura, y KrakenD la localiza con la variable `KRAKEND_LICENSE_PATH`.
- Los registros de Enterprise incluyen líneas sobre la licencia. No compartas registros sin filtrarlos.

### Arrancar Enterprise con la licencia

Los ficheros de Compose de Enterprise leen la ruta de la licencia de la variable `KRAKEND_EE_LICENSE_FILE`. Si no está definida, Compose se niega a arrancar.

Desde PowerShell:

```powershell
$env:KRAKEND_EE_LICENSE_FILE = "C:/ruta/fuera/del/repo/LICENSE"
docker compose -f a2a-plugin/prototype/compose.ee.yml up -d --build
```

Desde Git Bash:

```bash
KRAKEND_EE_LICENSE_FILE="C:/ruta/fuera/del/repo/LICENSE" docker compose -f a2a-plugin/prototype/compose.ee.yml up -d --build
```

Dentro del contenedor, la licencia queda montada en `/run/krakend-license/LICENSE` y `KRAKEND_LICENSE_PATH` apunta a esa ruta.

Para una comprobación puntual con `docker run`:

```bash
docker run --rm -e KRAKEND_LICENSE_PATH=/run/krakend-license/LICENSE -v "C:/ruta/fuera/del/repo/LICENSE:/run/krakend-license/LICENSE:ro" krakend/krakend-ee:3.0.4 plugin test -sm /plugins/a2a-proto.so
```

### Revisar los registros sin exponer la licencia

```bash
docker logs a2a-proto-ee-krakend-1 2>&1 | grep -vi licen
```

## Comandos útiles

| Acción | Community 2.13.11 | Enterprise 3.0.4 |
|---|---|---|
| Comprobar configuración | `krakend check -t -l -c krakend.json` | Igual, sin licencia |
| Probar la carga del plugin | `krakend test-plugin -sm plugin.so` | `krakend plugin test -sm plugin.so`, con licencia |
| Versiones de Go y libc del binario | `krakend version` | `krakend version`, sin licencia |

En Git Bash, los volúmenes de `docker run` necesitan `MSYS_NO_PATHCONV=1` y rutas de Windows, porque si no Git Bash reescribe las rutas.

## Pruebas

| Nivel | Comando, desde `a2a-plugin` | Qué necesita |
|---|---|---|
| Núcleo y adaptador | `cd core && go test ./...` y `cd plugin && go test ./...` | Go 1.26 o superior |
| Catálogo de agentes | `cd config/catalog && go test ./...` | Go 1.26 o superior |
| Esquema, ejemplos, documentación y versiones | `cd config/conformance && go test ./...` | Go |
| Redis real | `A2A_TEST_REDIS=host:puerto go test -run TestRedisStore ./ownership/` en `core` | Un Redis |
| Plantillas con KrakenD real | `bash config/templates/render.sh community` y `enterprise` | Docker |
| Extremo a extremo | `A2A_EDITION=ce go test ./...` en `e2e` | El entorno de `deploy/compose` levantado |
| Carga | `bash e2e/load.sh ce 30s` | El entorno de `deploy/compose` levantado |
| Agentes Pydantic AI | `docker compose run --rm pruebas` en `demos/pydantic-ai` | Su entorno levantado; ver su README |
| Integración continua: formato, pruebas con los dos Go, imágenes, artefactos y cruces entre ediciones | `bash scripts/ci.sh` | Docker; licencia opcional |

`scripts/ci.sh` prueba el mismo código con el Go de la imagen de compilación de cada edición; las versiones exactas están en `compatibilidad.md`. Un uso de API posterior a Go 1.26 falla en `go vet`, porque los módulos declaran `go 1.26`, y en la compilación de Community.

El detector de carreras necesita un compilador C. Se ejecuta en un contenedor:

```bash
docker run --rm -v "$PWD/core:/src" -w /src golang:1.26.8-bookworm go test -race ./...
```

## Secretos de la demostración

El repositorio no contiene claves ni secretos. Al levantar `deploy/compose`, el servicio `secrets` ejecuta `deploy/demo-secrets`, que genera en `deploy/.secrets`, excluido de git:

- La clave privada con la que el agente de referencia firma su Agent Card.
- El JWKS con el que el gateway verifica esa firma.
- Los clientes del IdP simulado, con secretos aleatorios.

No sobrescribe lo que ya existe. Para rotar el material, borra `deploy/.secrets` y vuelve a levantar el entorno.

`deploy/demo.sh`, las pruebas de extremo a extremo y la prueba de carga leen los secretos de `deploy/.secrets/idp/clients.json`, o del fichero que indique `A2A_CLIENTS_FILE`. El prototipo genera su propia clave de prueba con `go run ./tools/probe -init`.

## Análisis estático con SonarQube

`scripts/sonar.sh` analiza el código con un SonarQube local. Genera la cobertura de Go y ejecuta el escáner en Docker, en la misma red que el contenedor de SonarQube. El token de análisis se pasa en `SONAR_TOKEN` y nunca se guarda en el repositorio. La configuración del análisis está en `sonar-project.properties`, en la raíz del repositorio.

```bash
SONAR_TOKEN=<token> bash scripts/sonar.sh
```

El proyecto se analiza con todas las reglas de Go y de Dockerfile activas. El análisis debe terminar con el quality gate en verde, sin incidencias ni hotspots de seguridad.

## Análisis de seguridad

Además de SonarQube, el código que se compila (`core`, `plugin` y `config/catalog`) y las imágenes se revisan con estas herramientas. Las versiones son las de la última revisión, el 24 de noviembre de 2026. Los comandos se ejecutan desde `a2a-plugin`.

| Herramienta | Qué revisa | Comando |
|---|---|---|
| gosec 2.30.0 | Errores de seguridad en el código Go | `go run github.com/securego/gosec/v2/cmd/gosec@v2.30.0 ./...` en `core`, `plugin` y `config/catalog` |
| govulncheck 1.10.0 | Vulnerabilidades conocidas que alcanza el código, con el Go de cada edición | `docker run --rm -v "$PWD:/src:ro" -w /src/core --entrypoint sh krakend/builder:2.13.11 -c 'go run golang.org/x/vuln/cmd/govulncheck@v1.10.0 ./...'`, también en `plugin` y `config/catalog` y con `krakend/builder-ee:3.0.4` |
| Trivy 0.76.2 | Dependencias vulnerables, Dockerfiles y secretos | `docker run --rm -v "$PWD:/src:ro" aquasec/trivy:0.76.2 fs --scanners vuln,misconfig,secret --skip-dirs /src/deploy/.secrets --skip-dirs /src/out /src` |
| Semgrep 1.185.0 | Reglas de Go, OWASP Top 10, Dockerfile y secretos | `docker run --rm -v "$PWD:/src:ro" -w /src semgrep/semgrep:1.185.0 semgrep scan --metrics=off --config p/golang --config p/owasp-top-ten --config p/dockerfile --config p/secrets --exclude .secrets --exclude out` |

Ninguna encuentra incidencias en el código que se compila. Lo que queda es conocido:

- **govulncheck en Community.** Encuentra las vulnerabilidades de la biblioteca estándar de Go 1.26.8 que explica la [guía de Community](community.md). En `config/catalog` señala además GO-2026-6604, que solo afecta a Windows; el catálogo se ejecuta en la imagen de Linux. Enterprise no tiene ninguna.
- **Trivy, `HEALTHCHECK`.** Las imágenes no la declaran: depende del puerto y de la configuración de cada despliegue, así que la definen Compose o el orquestador.
- **Semgrep, servidores sin TLS.** Son el IdP simulado y los agentes de la demostración, que solo hablan dentro de la red de Docker.

## Sitio de documentación

Desde `a2a-plugin/site`, ejecuta `npm ci`, `npm run build` y `npm run preview`. Actualiza juntas las páginas Markdown en español e inglés. El sitio estático queda en `a2a-plugin/out/docs-site/`.
