# Catálogo de agentes

[Índice de guías](README.md) · [Read in English](en/agents.md)

Separa la configuración compartida de las definiciones de cada agente. El mismo catálogo genera el bloque A2A y las rutas internas en KrakenD Community y Enterprise.

## Estructura de archivos

```text
settings/
  a2a.json
  agents/
    research.json
    billing/
      agent.json
      gateway.json
      security.json
```

`a2a.json` contiene el servicio, el validador JWT, los claims, las rutas y los parámetros globales del plugin. El directorio `agents/` admite un archivo `<id>.json` con la definición completa o una carpeta `<id>/` con un `agent.json` obligatorio. El `id` declarado debe coincidir con el nombre del archivo o de la carpeta.

## Separar las secciones de un agente

`agents/billing/agent.json` contiene los campos principales del agente.

```json
{
  "id": "billing",
  "versions": ["1.0"],
  "card": {
    "source": "upstream",
    "url": "https://billing.example.com/.well-known/agent-card.json"
  }
}
```

Cada archivo adicional contiene el valor de una sección. Por ejemplo, `agents/billing/gateway.json` contiene el objeto de backend, con el host, la ruta y los tiempos de espera.

```json
{
  "host": "https://billing.example.com",
  "path": "/",
  "timeout": "10s",
  "stream_timeout": "120s"
}
```

Solo las plantillas usan `gateway`; no llega al plugin. Admite estos campos:

| Campo | Qué hace |
|---|---|
| `host`, `path` | Dirección del agente |
| `timeout`, `stream_timeout` | Tiempo de espera de los endpoints internos `unary` y `stream` |
| `extra_config` | Componentes de KrakenD en los endpoints internos del agente, como un límite de peticiones |
| `auth` | Claves de `auth/validator` que sustituyen a las de `a2a.json` solo para este agente. Sirve para exigir scopes o roles por agente |
| `backend_extra_config` | Componentes de KrakenD en la llamada al agente. Sirve para que el gateway se autentique ante él, con `auth/client-credentials` o con una cabecera fija de `modifier/martian` |

Por ejemplo, para que solo los clientes con `a2a.write` usen un agente, y que el gateway se presente ante él con un token propio de OAuth:

```json
{
  "host": "https://research.example.com",
  "path": "/",
  "timeout": "10s",
  "stream_timeout": "120s",
  "auth": { "scopes_key": "scope", "scopes": ["a2a.write"], "scopes_matcher": "all" },
  "backend_extra_config": {
    "auth/client-credentials": {
      "client_id": "gateway",
      "client_secret": "<secreto>",
      "token_url": "https://idp.example.com/token"
    }
  }
}
```

Las dos formas de credencial funcionan en las dos ediciones, también con streaming. El agente recibe la credencial del gateway, nunca el token del cliente.

Puedes separar `card`, `versions`, `security`, `capabilities`, `disabled_operations`, `ownership` y `gateway`. Si una sección aparece en dos archivos, el ensamblador muestra ambos nombres y detiene la generación. Cada agente necesita los campos de configuración aplicables y un `gateway` para que las plantillas puedan generar su backend. [Referencia de configuración](configuracion.md)

Para una Agent Card estática, guarda el documento en una subcarpeta como `assets/`, monta ese directorio en el contenedor y apunta `card.file` a la ruta montada. El archivo de sección `card.json` contiene las opciones `source` y `file`. El documento de la Agent Card y los JWKS de `card.signature.jwks_file` se leen desde el sistema de archivos de KrakenD.

## Renderizar y desplegar

Desde `a2a-plugin`, ejecuta el comando de tu edición. El tercer argumento es opcional y permite usar tu propio directorio `settings`.

```bash
bash config/templates/render.sh community https://gw.example.com ./config/templates/settings
bash config/templates/render.sh enterprise https://gw.example.com ./config/templates/settings
```

Los archivos resultantes quedan en `out/rendered-community.json` y `out/rendered-enterprise.json`. Despliega el de tu edición. Los ejemplos de Compose ensamblan y renderizan al arrancar, usando `/usr/local/bin/a2a-config run -c /etc/krakend/krakend.tmpl`. Para dar de alta un agente, agrega su archivo o carpeta y vuelve a desplegar.

## Migrar desde la lista anterior

La lista `agents` dentro de `a2a.json` sigue siendo válida. Puedes mover los agentes de uno en uno a `agents/`, quitando cada entrada original al moverla. El ensamblador rechaza IDs repetidos entre ambas fuentes.
