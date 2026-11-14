# Prototipo de la fase 0

Código desechable que validó los supuestos S1 a S7 de `design.md`. No es la base del plugin: el plugin está en `a2a-plugin/core` y `a2a-plugin/plugin`.

Resultados: `a2a-plugin/docs/validacion-supuestos.md`.

## Contenido

| Ruta | Qué es |
|---|---|
| `main.go` | Plugin con el borde A2A y el modificador de autorización en un único `.so` |
| `config/ce/krakend.json` | Configuración para Community 2.13.11, `version: 3` |
| `config/ee/krakend.json` | Configuración para Enterprise 3.0.4, `version: 4` |
| `compose.ce.yml`, `compose.ee.yml` | Gateway y agente simulado por edición |
| `tools/probe` | Batería de escenarios, firma de tokens de prueba y generación de la clave |

El agente `x` usa el flush del plugin. El agente `y` lo desactiva, como control.

## Reproducir

Desde este directorio, con Git Bash o cualquier shell POSIX. En Git Bash, `MSYS_NO_PATHCONV=1` evita que se reescriban las rutas de los volúmenes.

Generar la clave simétrica de prueba, que queda en `.secrets/` y está excluida de git:

```bash
go run ./tools/probe -init
```

Compilar el plugin para cada edición:

```bash
MSYS_NO_PATHCONV=1 docker run --rm -v "$(pwd -W 2>/dev/null || pwd):/app" -w /app krakend/builder:2.13.11 go build -buildmode=plugin -o out/ce/a2a-proto.so .
```

```bash
MSYS_NO_PATHCONV=1 docker run --rm -v "$(pwd -W 2>/dev/null || pwd):/app" -w /app krakend/builder-ee:3.0.4 go build -buildmode=plugin -o out/ee/a2a-proto.so .
```

Community:

```bash
docker compose -f compose.ce.yml up -d --build
```

```bash
go run ./tools/probe -edition ce -gw http://localhost:8081 -agent http://localhost:8090
```

Enterprise, con la licencia fuera del repositorio según `docs/desarrollo.md`:

```bash
KRAKEND_EE_LICENSE_FILE="/ruta/fuera/del/repo/LICENSE" docker compose -f compose.ee.yml up -d --build
```

```bash
go run ./tools/probe -edition ee -gw http://localhost:8082 -agent http://localhost:8091
```

Token de prueba para llamadas manuales:

```bash
go run ./tools/probe -print-token alice
```
