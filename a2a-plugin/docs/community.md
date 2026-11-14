# Instalación en KrakenD Community

El plugin se carga en el binario oficial de KrakenD Community 2.13.11, sin modificarlo.

## Aviso sobre la línea 2.13

KrakenD Community 3.0 eliminó el soporte de plugins Go. La 2.13.11 es la última versión de Community que los carga, y la línea está congelada:

- No ha recibido versiones desde el 8 de septiembre de 2026.
- Las correcciones de seguridad del 12 de noviembre de 2026 salieron para Community 3.0.4 y Enterprise 2.13.14, no para Community 2.13.
- No hay fecha de fin de vida publicada.

**No expongas KrakenD Community 2.13 directamente a internet.** Ponle delante una capa que sí reciba correcciones, como un balanceador o un WAF gestionado, y vigila los avisos de seguridad de KrakenD.

## 1. Construir

Desde el directorio `a2a-plugin`:

```bash
docker build -f build/community/Dockerfile -t krakend-a2a:ce-2.13.11 .
```

```bash
docker build -f build/community/Dockerfile --target artifacts -o type=local,dest=out/community .
```

La construcción ejecuta todas las pruebas con Go 1.26.8, el mismo que el binario de Community, y después, ya en la imagen oficial de destino:

- `check-artifact.sh`, que compara el manifiesto con `krakend version`.
- `krakend check-plugin`, la comprobación de compatibilidad de KrakenD.
- `krakend test-plugin`, la prueba de carga de los dos símbolos.

Si alguna falla, la imagen no se construye.

| Fichero | Contenido |
|---|---|
| `krakend-a2a-ce-2.13.11.so` | El plugin |
| `krakend-a2a-ce-2.13.11.manifest.json` | Edición, versión de KrakenD, Go, libc, arquitectura y sha256 |
| `check-artifact.sh` | Comprobación previa al despliegue |

## 2. Comprobar antes de desplegar

```bash
docker run --rm --entrypoint sh -v "$PWD/out/community:/p:ro" krakend:2.13.11 /p/check-artifact.sh /p/krakend-a2a-ce-2.13.11.so
```

Un artefacto de Enterprise en Community, o al revés, se rechaza con las discrepancias explicadas.

## 3. Configurar

Con las plantillas:

```bash
config/templates/render.sh community https://gw.example.com
```

Si escribes la configuración a mano, en Community 2.13:

- El fichero usa `"version": 3`.
- El bloque `plugin` va en la raíz del fichero, fuera de `extra_config`.
- El borde se declara en `plugin/http-server` con `"name": ["a2a-gateway"]` y el bloque A2A bajo la clave `"a2a-gateway"`.
- Cada endpoint interno declara `plugin/req-resp-modifier` con `"name": ["a2a-authz"]` y `"a2a-authz": {"agent": "<id>"}`.

El bloque A2A es idéntico al de Enterprise; ver `configuracion.md`.

## 4. Arrancar

```bash
docker run -p 8080:8080 -v "$PWD/out/rendered-community.json:/etc/krakend/krakend.json:ro" krakend-a2a:ce-2.13.11 run -c /etc/krakend/krakend.json
```

## 5. Streaming

KrakenD Community no hace flush de las respuestas `no-op`: sin el plugin, un flujo SSE llega entero al final. El plugin hace flush en cada escritura de `text/event-stream`, así que en Community los eventos llegan según los emite el agente. Está verificado en las pruebas de extremo a extremo.

## 6. Diferencias con Enterprise

| Aspecto | Community 2.13.11 | Enterprise 3.0.4 |
|---|---|---|
| Comportamiento A2A | Idéntico | Idéntico |
| Bloque de configuración A2A | Idéntico | Idéntico |
| Formato del fichero | `version: 3` | `version: 4` |
| Respuesta del límite de peticiones nativo | 503 | 503 |
| Correcciones de seguridad de KrakenD | No | Sí |

## Clientes con el SDK oficial

Pasa al resolutor del SDK la URL completa de la card, como se explica en `enterprise.md`.
