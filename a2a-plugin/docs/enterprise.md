# Instalación en KrakenD Enterprise

[Índice de guías](README.md) · [Read in English](en/enterprise.md)

El plugin se carga en el binario oficial de KrakenD Enterprise 3.0.4 o superior, sin modificarlo. Cada versión de Enterprise necesita un artefacto compilado con su imagen de compilación.

## 1. Construir

Desde el directorio `a2a-plugin`:

```bash
docker build -f build/enterprise/Dockerfile -t krakend-a2a:ee-3.0.4 .
```

```bash
docker build -f build/enterprise/Dockerfile --target artifacts -o type=local,dest=out/enterprise .
```

La construcción ejecuta todas las pruebas con el mismo Go que el binario de Enterprise y produce tres ficheros:

| Fichero | Contenido |
|---|---|
| `krakend-a2a-ee-3.0.4.so` | El plugin |
| `krakend-a2a-ee-3.0.4.manifest.json` | Edición, versión de KrakenD, Go, libc, arquitectura y sha256 |
| `check-artifact.sh` | Comprobación previa al despliegue |

Para otra versión de Enterprise, pasa `--build-arg KRAKEND_VERSION=<versión>`. Debe existir la imagen `krakend/builder-ee` de esa misma versión.

## 2. Comprobar antes de desplegar

Cargar un artefacto de otra edición o versión aborta KrakenD con un error fatal de Go. Compruébalo siempre en la imagen de destino:

```bash
docker run --rm --entrypoint sh -v "$PWD/out/enterprise:/p:ro" krakend/krakend-ee:3.0.4 /p/check-artifact.sh /p/krakend-a2a-ee-3.0.4.so
```

La prueba de carga del plugin necesita la licencia, que se monta en tiempo de ejecución desde fuera del repositorio:

```bash
KRAKEND_EE_LICENSE_FILE=/ruta/fuera/del/repo/LICENSE build/enterprise/plugin-test.sh krakend-a2a:ee-3.0.4
```

La imagen `krakend-a2a:ee-3.0.4` no contiene ninguna licencia.

## 3. Configurar

Genera la configuración con las plantillas y el [catálogo de agentes](agentes.md). Así cada endpoint interno lleva la autenticación nativa, la propagación de identidad y el modificador de autorización.

```bash
config/templates/render.sh enterprise https://gw.example.com
```

El resultado queda en `out/rendered-enterprise.json`. El ejemplo de `deploy/compose/enterprise.yml` ensambla el catálogo y renderiza al arrancar.

Si escribes la configuración a mano, en Enterprise 3.x:

- El fichero usa `"version": 4`.
- El bloque `plugin` va dentro de `extra_config` de la raíz.
- El borde se declara en `plugin/http-server` con `"plugins": [{"name": "a2a-gateway", "config": {...}}]`.
- Cada endpoint interno declara `plugin/req-resp-modifier` con `"plugins": [{"name": "a2a-authz", "config": {"agent": "<id>"}}]`.

La referencia completa del bloque A2A está en `configuracion.md`.

## 4. Arrancar

```bash
docker run -p 8080:8080 \
  -e KRAKEND_LICENSE_PATH=/run/krakend-license/LICENSE \
  -v /ruta/fuera/del/repo/LICENSE:/run/krakend-license/LICENSE:ro \
  -v "$PWD/out/rendered-enterprise.json:/etc/krakend/krakend.json:ro" \
  krakend-a2a:ee-3.0.4 run -c /etc/krakend/krakend.json
```

El registro de arranque muestra el plugin cargado:

```
[SERVICE: Handler Plugin] Total plugins loaded: 1
[SERVICE: Modifier Plugin] Total plugins loaded: 1
[PLUGIN: a2a] versión 0.1.0 cargada con 2 agente(s)
```

Si el bloque A2A es inválido, KrakenD no arranca y el registro nombra cada campo incorrecto.

## 5. Actualizar KrakenD Enterprise

1. Recompila el plugin con la imagen `krakend/builder-ee` de la nueva versión.
2. Ejecuta `check-artifact.sh` en la nueva imagen de KrakenD.
3. Ejecuta `plugin-test.sh` con la licencia.
4. Despliega el nuevo artefacto junto con la nueva versión de KrakenD, nunca por separado.

Evita Enterprise 3.0.0 a 3.0.3: tienen un fallo en las interfaces de los modificadores.

## Clientes con el SDK oficial

Cada agente se publica bajo su propia ruta. El resolutor del SDK oficial usa como URL de la card cualquier URL que tenga ruta, así que hay que darle la URL completa de la card:

```go
card, err := agentcard.DefaultResolver.Resolve(ctx, "https://gw.example.com/a2a/billing/.well-known/agent-card.json")
```

Con solo el dominio, el resolutor obtiene la card del agente por defecto, si está configurado.

## Notas de rendimiento

En la prueba de carga de la fase de integración, un KrakenD Enterprise 3.0.4 sin el plugin atendía unas 230 peticiones por segundo en `/__health`, frente a más de 14 000 en Community con la misma máquina. Con el plugin se obtuvo la misma cifra, así que el límite no lo introduce el plugin. No se investigó su origen para no inspeccionar la licencia disponible; revísalo con la configuración y la licencia de producción.
