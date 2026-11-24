# Configuración del plugin A2A

[Índice de guías](README.md) · [Read in English](en/configuration.md)

Las plantillas ensamblan la configuración compartida y el [catálogo de agentes](agentes.md) en el bloque JSON que recibe el plugin. El bloque es idéntico en KrakenD Community 2.13 y en Enterprise 3.0. Lo que cambia es el fichero que lo envuelve.

El esquema JSON del bloque está en `a2a-plugin/config/schema/a2a-gateway.schema.json`. Los ejemplos válidos e inválidos están en `a2a-plugin/config/examples/`.

## Dónde va el bloque

Community 2.13, con `version: 3`. El bloque va bajo la clave con el nombre del plugin:

```json
{
  "version": 3,
  "plugin": { "pattern": ".so", "folder": "/opt/krakend/plugins/" },
  "extra_config": {
    "plugin/http-server": {
      "name": ["a2a-gateway"],
      "a2a-gateway": { "public_url": "https://gw.example.com", "agents": [] }
    }
  }
}
```

Enterprise 3.0, con `version: 4`. El bloque va en el campo `config` del plugin:

```json
{
  "version": 4,
  "extra_config": {
    "plugin": { "pattern": ".so", "folder": "/opt/krakend/plugins/" },
    "plugin/http-server": {
      "plugins": [
        { "name": "a2a-gateway", "config": { "public_url": "https://gw.example.com", "agents": [] } }
      ]
    }
  }
}
```

## Ejemplo mínimo

```json
{
  "public_url": "https://gw.example.com",
  "agents": [
    {
      "id": "billing",
      "card": { "source": "upstream", "url": "https://billing.example.com/.well-known/agent-card.json" }
    }
  ]
}
```

Con esto, el agente se publica en `https://gw.example.com/a2a/billing` y su Agent Card en `https://gw.example.com/a2a/billing/.well-known/agent-card.json`.

## Cómo se valida

El plugin valida el bloque al arrancar. Si hay un error, KrakenD no arranca.

- **Campos desconocidos.** Se rechazan. Un nombre mal escrito no se ignora en silencio.
- **Valores null.** Se rechazan. Para usar el valor por defecto, omite el campo.
- **Todos los errores a la vez.** El mensaje los lista todos, cada uno con la ruta del campo. Los agentes se nombran por su `id`, por ejemplo `agents[billing].ownership.unknown_task`.
- **Duraciones.** Se escriben como en Go: `500ms`, `5m`, `168h`.
- **Avisos.** Algunas combinaciones válidas pero arriesgadas generan un aviso en el registro de arranque. Están listadas al final.

## Campos de la raíz

| Campo | Tipo | Por defecto | Descripción |
|---|---|---|---|
| `public_url` | URL | Obligatorio | URL con la que los clientes ven el gateway: esquema, host y puerto, sin ruta. Las Agent Cards publicadas anuncian esta URL. Con `http` se emite un aviso. |
| `base_path` | Ruta | `/a2a` | Ruta pública bajo la que se publica cada agente. |
| `internal_prefix` | Ruta | `/__a2a` | Prefijo de las rutas internas. Desde fuera responden 404. No puede solaparse con `base_path`. |
| `route_granularity` | `class` u `operation` | `class` | `class` usa dos endpoints internos por agente, `unary` y `stream`. `operation` usa uno por operación. |
| `default_agent` | Id de agente | Ninguno | Agente cuya Agent Card se publica también en `/.well-known/agent-card.json`. |
| `limits` | Objeto | | Límites de memoria por petición. |
| `identity` | Objeto | | Origen de la identidad del llamante. |
| `authorization` | Objeto | | Scopes por operación. |
| `ownership` | Objeto | | Propiedad de tareas y contextos. |
| `push_notifications` | Objeto | | Guardia de webhooks. |
| `challenge` | Objeto | | Desafío de autenticación y metadatos de recurso protegido. |
| `agents` | Lista | Obligatorio | Agentes publicados. Al menos uno. |

## `limits`

| Campo | Tipo | Por defecto | Descripción |
|---|---|---|---|
| `max_body_bytes` | Entero | 4194304 (4 MiB) | Tamaño máximo del cuerpo de una petición A2A. Por encima se responde 413. Entre 1 KiB y 64 MiB. |
| `max_inspect_bytes` | Entero | 8388608 (8 MiB) | Tamaño máximo de una respuesta que el plugin inspecciona para registrar la propiedad. Entre 1 KiB y 256 MiB. |

## `identity`

El plugin no valida tokens. Lee la identidad que KrakenD ya verificó, normalmente con `propagate_claims` en `auth/validator`. El plugin borra estas cabeceras de toda petición entrante, así que solo llegan si las pone KrakenD.

| Campo | Tipo | Por defecto | Descripción |
|---|---|---|---|
| `required` | Booleano | `true` | Si es `true`, toda operación sin identidad se rechaza con 401. Con `false` se emite un aviso. |
| `headers` | Objeto | `issuer`: `X-A2A-Iss`, `subject`: `X-A2A-Sub`, `scopes`: `X-A2A-Scope` | Nombre lógico de cada dato de identidad y cabecera que lo transporta. `subject` es obligatorio. `scopes` lleva los scopes separados por espacios. Puedes añadir otros nombres, como `tenant`. Si indicas este objeto, sustituye por completo a los valores por defecto. |
| `owner_scope` | Lista | `issuer` y `subject`, si existen en `headers` | Nombres de `headers` que forman la clave de propietario. Con `tenant`, todos los usuarios de un mismo tenant comparten tareas. |

Las cabeceras de identidad no pueden ser cabeceras estándar ni de protocolo, como `Authorization`, `Cookie`, `Content-Type` o `A2A-Version`, ni las internas del plugin.

## `authorization`

| Campo | Tipo | Por defecto | Descripción |
|---|---|---|---|
| `operation_scopes` | Objeto | Vacío | Scopes que exige cada operación, con su nombre de A2A 1.0, como `GetTask`. El llamante debe tener todos los scopes de la lista. Requiere `identity.headers.scopes`. |

## `ownership`

Registra qué principal creó cada tarea y cada contexto, y deniega a los demás.

| Campo | Tipo | Por defecto | Descripción |
|---|---|---|---|
| `enabled` | Booleano | `true` | Activa el control de propiedad. Requiere `identity.required` a `true`. |
| `store` | `memory` o `redis` | `memory` | `memory` solo es válido con un único nodo y genera un aviso. `redis` comparte la propiedad entre nodos. |
| `ttl` | Duración | `168h` | Tiempo de vida de un registro desde su último uso. Entre 1 minuto y un año. |
| `unknown_task` | `deny` o `passthrough` | `deny` | Qué hacer con una tarea sin propietario registrado. `passthrough` sirve para un periodo de transición y genera un aviso. |
| `list_tasks` | `owned_context`, `deny` o `passthrough` | `owned_context` | `owned_context` exige que `ListTasks` indique un contexto del llamante. |
| `on_store_error` | `deny` o `passthrough` | `deny` | Qué hacer si el almacén no responde. `passthrough` genera un aviso. |
| `memory` | Objeto | | Almacén en memoria. |
| `redis` | Objeto | | Almacén Redis. Obligatorio con `store` a `redis`. |

### `ownership.memory`

| Campo | Tipo | Por defecto | Descripción |
|---|---|---|---|
| `max_entries` | Entero | 100000 | Número máximo de registros en memoria. |

### `ownership.redis`

Las credenciales nunca se escriben en la configuración: se indican los nombres de las variables de entorno que las contienen.

| Campo | Tipo | Por defecto | Descripción |
|---|---|---|---|
| `address` | `host:puerto` | Obligatorio | Servidor Redis. |
| `db` | Entero | 0 | Número de base de datos. |
| `tls` | Booleano | `false` | Conecta con TLS. |
| `username_env` | Nombre de variable | Ninguno | Variable de entorno con el usuario. |
| `password_env` | Nombre de variable | Ninguno | Variable de entorno con la contraseña. |
| `timeout` | Duración | `2s` | Tiempo máximo por operación. Entre 100 ms y 30 s. |

## `push_notifications`

El plugin revisa la URL de webhook de toda configuración de notificaciones push antes de que llegue al agente. Es defensa en profundidad: quien llama al webhook es el agente, así que su red también debe limitar las salidas.

| Campo | Tipo | Por defecto | Descripción |
|---|---|---|---|
| `require_https` | Booleano | `true` | Rechaza webhooks sin HTTPS. Con `false` se emite un aviso. |
| `deny_private_networks` | Booleano | `true` | Rechaza webhooks hacia direcciones de loopback, privadas, link-local o de metadatos de nube. Con `false` se emite un aviso. |
| `allowed_hosts` | Lista | Vacía | Hosts permitidos, en minúsculas, con `*.` opcional al principio. Vacía permite cualquier host público. |

## `challenge`

| Campo | Tipo | Por defecto | Descripción |
|---|---|---|---|
| `scheme` | Texto | `Bearer` | Esquema que se anuncia en `WWW-Authenticate` en las respuestas 401. |
| `realm` | Texto | Ninguno | Valor de `realm` en `WWW-Authenticate`. Sin comillas ni saltos de línea. |
| `resource_metadata` | Booleano | `false` | Publica el documento de metadatos de recurso protegido (RFC 9728) de cada agente. |
| `authorization_servers` | Lista de URL | Vacía | Servidores de autorización que se anuncian. Obligatorio con `resource_metadata` a `true`. |
| `scopes_supported` | Lista | Vacía | Scopes que se anuncian en los metadatos. |

## `agents`

Cada elemento publica un agente.

| Campo | Tipo | Por defecto | Descripción |
|---|---|---|---|
| `id` | Texto | Obligatorio | Identificador del agente y parte de su URL. Minúsculas, dígitos y guiones, hasta 63 caracteres. Único. |
| `versions` | Lista | `["1.0"]` | Versiones de A2A habilitadas: `1.0` y `0.3`. Una petición sin cabecera `A2A-Version` es de la versión 0.3. |
| `card` | Objeto | Obligatorio | Origen de la Agent Card. |
| `security` | Objeto | Ninguno | Esquemas de seguridad del gateway que sustituyen a los de la card. |
| `capabilities` | Objeto | | Capacidades que habilita el gateway. |
| `disabled_operations` | Lista | Vacía | Operaciones que el gateway rechaza para este agente, con su nombre de A2A 1.0. |
| `ownership` | Objeto | Hereda | Políticas de propiedad solo para este agente. |

### `agents[].card`

| Campo | Tipo | Por defecto | Descripción |
|---|---|---|---|
| `source` | `static` o `upstream` | Obligatorio | `static` publica una card de la configuración o de un fichero. `upstream` la obtiene del agente. |
| `inline` | Objeto | Ninguno | Agent Card escrita en la configuración. Solo con `static`, y no a la vez que `file`. |
| `file` | Ruta | Ninguno | Fichero local con la Agent Card. Solo con `static`, y no a la vez que `inline`. |
| `url` | URL | Obligatorio con `upstream` | URL de descubrimiento del agente. |
| `ttl` | Duración | `5m` | Tiempo durante el que se reutiliza la card obtenida del agente. |
| `max_stale` | Duración | `1h` | Antigüedad máxima de la última copia válida si el agente no responde. No puede ser menor que `ttl`. |
| `max_bytes` | Entero | 1048576 (1 MiB) | Tamaño máximo de la card obtenida del agente. |
| `http_max_age` | Duración | `5m` | `max-age` de `Cache-Control` en las respuestas de descubrimiento. `0s` desactiva la caché del cliente. |
| `signature` | Objeto | | Verificación de la firma de la card remota. |

### `agents[].card.signature`

| Campo | Tipo | Por defecto | Descripción |
|---|---|---|---|
| `verify` | Booleano | `false` | Rechaza la card remota si ninguna de sus firmas es válida. Solo con `upstream`. |
| `jwks_file` | Ruta | Obligatorio con `verify` | Fichero JWKS con las claves de confianza. |

La card que publica el gateway nunca lleva `signatures`: al reescribirla, la firma original deja de ser válida.

### `agents[].security`

Se escribe en el formato de A2A 1.0. Si la card del agente usa la forma de 0.3, el plugin la traduce al publicarla.

| Campo | Tipo | Por defecto | Descripción |
|---|---|---|---|
| `schemes` | Objeto | Obligatorio | Esquemas por nombre. Cada uno contiene exactamente uno de `apiKeySecurityScheme`, `httpAuthSecurityScheme`, `oauth2SecurityScheme`, `openIdConnectSecurityScheme` o `mtlsSecurityScheme`. |
| `requirements` | Lista | Vacía | Requisitos de seguridad. Cada uno tiene un objeto `schemes` que nombra esquemas definidos en `schemes`. |

```json
"security": {
  "schemes": {
    "oidc": { "openIdConnectSecurityScheme": { "openIdConnectUrl": "https://idp.example.com/.well-known/openid-configuration" } }
  },
  "requirements": [ { "schemes": { "oidc": { "list": ["a2a.read"] } } } ]
}
```

### `agents[].capabilities`

La card publicada declara una capacidad solo si la declara el agente y además está habilitada aquí.

| Campo | Tipo | Por defecto | Descripción |
|---|---|---|---|
| `streaming` | Booleano | `true` | Habilita `SendStreamingMessage` y `SubscribeToTask`. |
| `push_notifications` | Booleano | `false` | Habilita las notificaciones push y sus cuatro operaciones de configuración. Desactivado por defecto porque expone al agente a destinos elegidos por el cliente. |

### `agents[].ownership`

Un campo omitido hereda el valor de `ownership`.

| Campo | Tipo | Por defecto | Descripción |
|---|---|---|---|
| `enabled` | Booleano | Hereda | Activa o desactiva el control de propiedad solo para este agente. |
| `unknown_task` | `deny` o `passthrough` | Hereda | Política de tareas sin propietario para este agente. |
| `list_tasks` | `owned_context`, `deny` o `passthrough` | Hereda | Política de `ListTasks` para este agente. |

## Reglas que solo comprueba el plugin

El esquema JSON no puede expresar las reglas que relacionan unos campos con otros. El plugin las comprueba al arrancar:

- Los `id` de agente son únicos, y `default_agent` nombra un agente existente.
- `base_path` e `internal_prefix` no se solapan, y `base_path` no está bajo `/.well-known`. Ninguna ruta contiene segmentos `.` o `..`.
- Las cabeceras de `identity.headers` son distintas entre sí, y ninguna es estándar, de protocolo o interna del plugin.
- `owner_scope` solo nombra entradas de `identity.headers` y no incluye `scopes`.
- `operation_scopes` requiere `identity.headers.scopes` e `identity.required` a `true`.
- El control de propiedad, global o por agente, requiere `identity.required` a `true`.
- `max_stale` no es menor que `ttl`.
- Los requisitos de `security` solo nombran esquemas definidos en `schemes`.

## Avisos de arranque

Estas combinaciones son válidas, pero el plugin las anuncia en el registro:

- `public_url` con `http`.
- `identity.required` a `false`.
- `ownership.store` a `memory` con el control de propiedad activo.
- `unknown_task` a `passthrough`, global o por agente.
- `on_store_error` a `passthrough`.
- `require_https` o `deny_private_networks` a `false`.

## Comprobar una configuración

Las pruebas de conformidad validan los ejemplos contra el esquema y contra el validador del plugin, y comprueban que esta documentación cubre todos los campos:

```bash
cd a2a-plugin/config/conformance && go test ./...
```
