# Guía de desarrollo

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
| Esquema, ejemplos y documentación | `cd config/conformance && go test ./...` | Go |
| Redis real | `A2A_TEST_REDIS=host:puerto go test -run TestRedisStore ./ownership/` en `core` | Un Redis |
| Plantillas con KrakenD real | `bash config/templates/render.sh community` y `enterprise` | Docker |
| Extremo a extremo | `A2A_EDITION=ce go test ./...` en `e2e` | El entorno de `deploy/compose` levantado |
| Carga | `bash e2e/load.sh ce 30s` | El entorno de `deploy/compose` levantado |
| Integración continua: formato, pruebas con los dos Go, imágenes, artefactos y cruces entre ediciones | `bash scripts/ci.sh` | Docker; licencia opcional |

`scripts/ci.sh` prueba el mismo código con Go 1.26.8 y Go 1.27.4, las versiones de las dos ediciones. Un uso de API posterior a Go 1.26 falla en `go vet`, porque los módulos declaran `go 1.26`, y en la compilación de Community.

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
