# Design

## Context

La motivación está en `proposal.md`. Aquí solo figura el estado actual que condiciona el diseño, verificado contra la documentación y el código público de KrakenD y la especificación A2A 1.0.0 en noviembre de 2026.

Los dos objetivos son **Enterprise 3.0.4 o superior** y **Community 2.13.11**. El usuario eligió Community 2.13 porque es la última línea de Community que carga plugins Go: Community 3.0.0 los eliminó.

### Qué hace ya KrakenD y qué no

| Necesidad | Community 2.13.11 | Enterprise 3.0.4 | Qué aporta el plugin |
|---|---|---|---|
| Validar firma, caducidad, emisor y audiencia del JWT | Sí, `auth/validator` | Sí | Nada. Se reutiliza |
| Roles y scopes por endpoint | Sí | Sí | Mapa de operación A2A a scopes |
| Pasar claims verificados a cabeceras | Sí, `propagate_claims` | Sí | Es la fuente de identidad del plugin |
| Revocación de tokens | Sí, `auth/revoker` | Sí, más Revoke Server | Nada |
| mTLS | Sí | Sí | Nada |
| Varios IdP por endpoint, API keys, Basic | No | Sí | Nada. Se consume su identidad por cabeceras |
| Motor de reglas | CEL, `validation/cel` | `security/policies` | Cabeceras con agente, operación y versión para esas reglas |
| Streaming SSE | No documentado. El render `no-op` no hace flush | Sí, `no-op` y `sse` | Flush en el envoltorio de respuesta del borde |
| Plugins Go | Sí: servidor, cliente y modificadores | Sí: los anteriores y middleware | No aplica |
| Redis al alcance de los plugins | No | Sí, `RedisRegisterer` | Cliente mínimo propio |
| Publicar y reescribir Agent Cards | No | No | Sí |
| Distinguir operaciones JSON-RPC en una misma URL | No | No | Sí |
| Ligar tareas y contextos al principal | No | No | Sí |
| Validar destinos de webhooks push | No | No | Sí |

### Sobre el "token de sesión"

A2A no define un token de sesión. La identidad viaja en cabeceras HTTP estándar y la especificación prohíbe llevarla en el cuerpo. Lo más parecido a una sesión es la pareja `contextId` y `taskId`. Por eso "validar el token de sesión" se divide en dos problemas:

1. **Autenticar al llamante.** KrakenD ya lo hace en ambas ediciones con `auth/validator` y revocación. No se reimplementa.
2. **Comprobar que ese llamante puede tocar esa tarea o ese contexto.** KrakenD no lo hace, porque no conoce A2A. Es la aportación principal del plugin en seguridad.

### Restricciones verificadas

- **Mismos símbolos en ambas ediciones.** `HandlerRegisterer` con `RegisterHandlers` para la capa de router, `ModifierRegisterer` con `RegisterModifiers` para la capa de proxy, y `RegisterLogger`. Las firmas documentadas coinciden en 2.13 y en 3.0.
- **Community 2.13.11.** Última versión de la línea, del 8 de septiembre de 2026. Go 1.26.8, imagen `krakend/builder:2.13.11`. Configuración con `version: 3`, bloque `plugin` en la raíz y plugins declarados con una lista `name`. Los plugins de middleware son solo de Enterprise.
- **Enterprise 3.0.4.** Go 1.27.4, imagen `krakend/builder-ee:3.0.4`. Configuración con `version: 4`, bloque `plugin` dentro de `extra_config` y plugins declarados con una lista `plugins`. Las versiones 3.0.0 a 3.0.3 tienen un fallo en las interfaces de los modificadores, así que el mínimo es 3.0.4.
- **Orden del pipeline** en ambas ediciones: la validación JWT ocurre en la capa de handler, que envuelve a la capa de proxy. Un modificador de petición ve ya la identidad verificada.
- **A2A 1.0**: once operaciones JSON-RPC con nombres en PascalCase, cabecera `A2A-Version` cuya ausencia significa `0.3`, Agent Card en `/.well-known/agent-card.json` con `supportedInterfaces`, streaming SSE donde cada línea `data:` es un sobre JSON-RPC, y errores propios de `-32001` a `-32009`.
- **Entorno de desarrollo**: Windows. Los plugins Go no se compilan en Windows, así que los `.so` se construyen en Docker. El núcleo y sus tests sí corren nativos.

### Estado de la línea Community 2.13

- No hay versiones de Community 2.13 posteriores a la 3.0.0, publicada el 30 de septiembre de 2026.
- Las correcciones de seguridad del 12 de noviembre de 2026 salieron en Community 3.0.4 y en Enterprise 2.13.14. No hay una versión equivalente de Community 2.13.
- Enterprise 2.x tiene fin de vida anunciado para el 5 de abril de 2027. Para Community 2.x no hay fecha publicada.

La línea está, a efectos prácticos, congelada. El diseño lo asume y mantiene el núcleo independiente de la edición para poder cambiar de objetivo en Community sin rehacer el trabajo.

## Goals / Non-Goals

**Goals:**

- Un solo código de plugin con el mismo comportamiento en ambas ediciones.
- Que cada operación A2A pase por el pipeline nativo de KrakenD, de modo que autenticación, límites, telemetría y políticas se configuren como en cualquier endpoint.
- No duplicar ninguna función de seguridad que KrakenD ya tenga.
- Un mismo bloque de configuración A2A y una misma lista de agentes para las dos ediciones.
- Fallar cerrado ante cualquier duda de identidad, propiedad o configuración.

**Non-Goals:**

- Implementar un agente A2A, un SDK o un cliente.
- Interpretar o transformar el contenido de los mensajes, sus partes o artefactos.
- Traducir entre versiones 0.3 y 1.0 o entre bindings.
- Entregar notificaciones push. El gateway solo vigila su configuración.
- Sustituir a un IdP o emitir tokens.
- Dar soporte a Community 3.x, que no carga plugins.

## Decisions

### D1. Un núcleo y un único adaptador de plugin, compilado dos veces

El núcleo es una biblioteca Go que solo conoce `net/http` y expone dos piezas: un envoltorio de `http.Handler` para el borde y una función de decisión para la autorización. Un único adaptador las registra con los símbolos de plugin, que son los mismos en las dos ediciones.

| Pieza | Community 2.13.11 | Enterprise 3.0.4+ |
|---|---|---|
| Borde, antes de enrutar | `HandlerRegisterer` | El mismo |
| Autorización, después de autenticar | `ModifierRegisterer`, modificador de petición | El mismo |
| Imagen de compilación | `krakend/builder:2.13.11`, Go 1.26.8 | `krakend/builder-ee:3.0.4`, Go 1.27.4 |
| Formato de configuración | `version: 3`, lista `name` | `version: 4`, lista `plugins` |
| Comprobaciones | `krakend check-plugin`, `krakend test-plugin` | `krakend plugin check`, `krakend plugin test` |
| Artefacto | `krakend-a2a-ce-2.13.11.so` | `krakend-a2a-ee-3.0.4.so` |

Las únicas diferencias de código entre ediciones son la forma en que llega la configuración al plugin, que el adaptador detecta, y la imagen con la que se compila.

**Por qué.** Usar solo los tipos de plugin que existen en ambas ediciones permite un único código fuente. El plugin de middleware de Enterprise sería algo más cómodo para la autorización, pero obligaría a mantener dos caminos.

**Alternativas descartadas para Community.**

- *Distribución propia de Community 3.x.* Daba una línea con soporte y paridad total. El usuario la descartó.
- *Servicio acompañante detrás de Community 3.x sin modificar.* Mantiene binarios oficiales, pero añade un proceso y pierde el streaming y los límites por operación.
- *Solo configuración o scripts Lua.* No alcanza para identificar operaciones ni para la propiedad de tareas.
- *Solo Enterprise.* Contradice el requisito de cubrir ambas ediciones.

### D2. Modo delegado: el plugin encamina, KrakenD hace de proxy

El borde no reenvía tráfico al agente. Valida la petición, reescribe la ruta a una ruta interna canónica y cede el control al enrutador de KrakenD. El endpoint interno es configuración estándar con `output_encoding: no-op`.

```
cliente A2A
   |  POST /a2a/billing        JSON-RPC, Authorization: Bearer ...
   v
+------------------- KrakenD (Community 2.13 o Enterprise 3) + plugin A2A --------------+
| [1] Borde A2A              capa router, antes de enrutar                              |
|     limpia cabeceras reservadas, valida sobre y versión, identifica la operación,     |
|     sirve Agent Cards, guardia de webhooks, reescribe la ruta a /__a2a/billing/unary  |
|        |                                                                              |
|        v                                                                              |
| [2] Endpoint interno       configuración estándar de KrakenD                          |
|     auth/validator -> límites -> reglas -> telemetría                                 |
|        |  cabeceras de identidad por propagate_claims                                 |
|        v                                                                              |
| [3] Autorización A2A       capa proxy, modificador de petición                        |
|     identidad obligatoria, scopes por operación, propiedad de tarea y contexto        |
|        |                                                                              |
|        v                                                                              |
|     backend no-op  ----------------------------------------------->  agente A2A       |
|        |                                                                              |
| [1'] Borde A2A en la respuesta                                                        |
|     registra propiedad, reescribe la card extendida, normaliza errores, hace flush    |
+---------------------------------------------------------------------------------------+
```

**Por qué.** El valor de un gateway es su pipeline. Así el operador configura límites, circuit breaker, balanceo, descubrimiento de servicios y trazas del agente igual que en cualquier otro endpoint, y la telemetría nativa queda desglosada por agente y clase de operación sin código.

**Alternativa descartada.** Un proxy inverso dentro del plugin. Necesita menos configuración, pero deja fuera todo el pipeline y obliga a reimplementar autenticación, límites y resiliencia.

### D3. Dos puntos de control y una correlación en proceso

El borde [1] se ejecuta antes de la autenticación, así que no conoce la identidad. La autorización [3] se ejecuta después y sí la conoce. Se conectan así:

1. El borde analiza la petición una vez, crea una entrada de correlación en memoria con agente, operación, versión, `id` JSON-RPC, `taskId`, `contextId` y URLs de webhook, y la identifica con un valor aleatorio que viaja en la cabecera reservada `X-A2A-Request-Id`.
2. El modificador de petición [3] recupera la entrada con esa cabecera, lee las cabeceras de identidad, decide y anota el veredicto y el principal en la entrada. Si deniega, devuelve un error y el pipeline se corta antes de llamar al agente.
3. El borde, al volver la respuesta, lee el veredicto. Si hubo denegación, descarta lo que produjo el pipeline y escribe la respuesta A2A correcta. Si hubo permiso, usa el principal anotado para registrar la propiedad de lo que se cree.
4. La entrada se elimina cuando termina la petición.

Los dos símbolos viven en el mismo `.so`, que Go carga una sola vez por ruta, de modo que comparten estado. Los modificadores no comparten contexto entre sí, y por eso la correlación viaja en una cabecera y no en el contexto de la petición.

**Fallo cerrado.** Si [3] no encuentra la entrada, deniega: la petición no pasó por el borde. Si el borde ve que [3] no llegó a ejecutarse en un agente con control de acceso, retiene la respuesta, devuelve `-32603` y registra un error crítico de configuración.

**Alternativas descartadas.**

- *Plugin de middleware para [3].* Solo existe en Enterprise.
- *Validar el JWT dentro del plugin.* Daría identidad en el borde, pero duplica código crítico de seguridad, pierde la revocación nativa y contradice el objetivo de no reimplementar lo que KrakenD ya hace.
- *Leer los claims sin verificar en el borde y confiar en que el endpoint rechace los tokens falsos.* Crea un oráculo de existencia de tareas y hace depender la seguridad de la disciplina de configuración.
- *Decidir solo en la respuesta.* Llega tarde para operaciones con efecto como `CancelTask`.

### D4. La identidad sale solo de KrakenD

El plugin define cabeceras de identidad configurables, por defecto `X-A2A-Iss`, `X-A2A-Sub` y `X-A2A-Scope`, que el operador rellena con `propagate_claims`, o en Enterprise con el mecanismo equivalente de API keys. El borde elimina de toda petición entrante esas cabeceras y todo el prefijo `X-A2A-`, así que solo pueden llegar a [3] si las puso KrakenD tras autenticar.

**Por qué.** Sirve con cualquier autenticador de KrakenD, y un endpoint sin autenticación produce una petición sin identidad, que se rechaza.

**El token del cliente no llega al agente.** La cabecera `Authorization` no figura en `input_headers`, así que el gateway termina la autenticación y el agente solo recibe la identidad ya verificada. Un agente que necesite el token del llamante debe pedirlo de forma explícita en su configuración, y las plantillas lo marcan como opción con riesgo.

### D5. Granularidad de las rutas internas

| Granularidad | Rutas por agente | Cuándo usarla |
|---|---|---|
| `class`, por defecto | `/__a2a/{agent}/unary` y `/__a2a/{agent}/stream` | Caso general. Dos endpoints por agente, con tiempos de espera distintos |
| `operation` | `/__a2a/{agent}/{Operación}`, once rutas | Cuando se quieren límites, scopes o tiempos nativos distintos por operación |

`SendStreamingMessage` y `SubscribeToTask` van a `stream`. Las otras nueve operaciones van a `unary`. Con `class`, los scopes por operación los aplica [3] a partir del mapa `operation_scopes`. Con `operation` pueden declararse además de forma nativa en cada `auth/validator`.

El borde responde `404` a cualquier petición externa al prefijo interno, de modo que esas rutas solo son alcanzables tras la reescritura.

### D6. Direccionamiento por ruta

Cada agente se publica en `{base_path}/{agent}`, y su card en `{base_path}/{agent}/.well-known/agent-card.json`. Un agente puede marcarse como agente por defecto para ocupar también `/.well-known/agent-card.json` en la raíz. El campo `tenant` de A2A no se usa para elegir agente: se conserva en la card y se reenvía intacto, porque pertenece al agente.

**Alternativa descartada.** Una URL única con selección por `tenant` o por credenciales. Es válida en A2A, pero impide dar a cada agente endpoints, límites y telemetría propios.

### D7. Agent Card: reescritura que conserva la forma

La card se trata como un árbol JSON genérico y solo se tocan los campos conocidos: URLs de interfaces, bindings, capacidades, esquemas de seguridad y firmas. Lo demás se copia tal cual. La misma rutina sirve para cards con forma 1.0 (`supportedInterfaces`) y 0.3 (`url`, `preferredTransport`, `additionalInterfaces`), y para la card extendida.

- **Origen**: `static` desde la configuración o un fichero, o `upstream` desde la URL de descubrimiento del agente, con TTL, obsolescencia máxima, límite de tamaño y `ETag`.
- **Firmas**: una card modificada ya no valida su firma original, así que `signatures` se elimina. La verificación de la card remota contra claves de confianza es opcional. Refirmar con una clave del gateway queda para más adelante.
- **URL pública**: se toma de `public_url` en la configuración, no de la cabecera `Host`, para evitar inyección de cabeceras.
- **Esquemas de seguridad del gateway**: se configuran siempre en formato 1.0. Si la card del agente tiene forma 0.3, se traducen a esa forma al publicarla: cada `*SecurityScheme` pasa a su equivalente con `type`, y cada requisito `{"schemes": {nombre: {"list": [...]}}}` pasa a `{nombre: [...]}`.

**Por qué un árbol genérico y no estructuras tipadas.** La especificación 1.0 tiene inconsistencias conocidas, por ejemplo `securityRequirements` frente a `security`, y seguirá evolucionando. Copiar lo desconocido evita romper agentes que usen campos nuevos.

### D8. Versiones: 1.0 como principal, 0.3 por nombres, sin traducir

El plugin es transparente respecto al contenido. Reconoce la operación por el nombre del método en cualquiera de las dos versiones, comprueba que la versión esté habilitada y que el nombre corresponda a ella, y reenvía el cuerpo sin cambios.

| Operación | Nombre 1.0 | Nombre 0.3 |
|---|---|---|
| Enviar mensaje | `SendMessage` | `message/send` |
| Enviar con streaming | `SendStreamingMessage` | `message/stream` |
| Consultar tarea | `GetTask` | `tasks/get` |
| Listar tareas | `ListTasks` | No existe |
| Cancelar tarea | `CancelTask` | `tasks/cancel` |
| Suscribirse a tarea | `SubscribeToTask` | `tasks/resubscribe` |
| Configuración push | `Create`, `Get`, `List`, `Delete` + `TaskPushNotificationConfig` | `tasks/pushNotificationConfig/set`, `get`, `list`, `delete` |
| Card extendida | `GetExtendedAgentCard` | `agent/getAuthenticatedExtendedCard` |

Para la propiedad solo hace falta localizar unos pocos campos, con una tabla de rutas por versión: `params.message.taskId`, `params.message.contextId`, `params.id`, `params.taskId` y la URL de webhook en la petición, y el identificador de tarea y de contexto en el resultado o en el primer evento. El análisis es superficial y no decodifica las partes del mensaje, que pueden pesar megabytes.

### D9. Streaming con flush en el borde

Los endpoints de la clase `stream` usan `no-op` y un tiempo de espera largo declarado en el propio endpoint. El envoltorio de respuesta del borde hace flush después de cada escritura cuando la respuesta es `text/event-stream`. Así el streaming no depende del render de cada edición ni de la función de streaming de Enterprise, y el mismo mecanismo sirve para Community 2.13, cuyo render `no-op` copia sin flush.

El envoltorio solo inspecciona el flujo hasta registrar la propiedad con el primer evento. Después reenvía sin analizar. Un bloque sin campo `data`, como un comentario de keepalive, no es un evento según SSE: se entrega al momento y se sigue esperando al primero que tenga datos.

Si la validación previa demostrara que en Community 2.13 algo retiene los eventos antes del envoltorio, se aplica la degradación de la spec `a2a-edition-packaging`: anunciar `streaming: false` y rechazar con `-32004`.

El tiempo de espera del endpoint limita la duración del flujo. Los clientes reanudan con `SubscribeToTask`, como prevé A2A.

### D10. Almacén de propiedad

- **Modelo**: clave `{agent}:task:{taskId}` o `{agent}:ctx:{contextId}`, valor igual al resumen SHA-256 de las cabeceras de identidad que forman el ámbito de propiedad. Nunca se guarda el sujeto en claro.
- **Escritura**: gana el primero que escribe. Un segundo intento con otro propietario no sobrescribe.
- **Caducidad**: TTL deslizante, renovado en cada acceso permitido.
- **Implementaciones**: memoria con tamaño acotado, para un nodo o para desarrollo, y Redis para varios nodos.
- **Cliente Redis**: mínimo y propio, sobre la biblioteca estándar, con las tres órdenes necesarias, autenticación y TLS. Community 2.13 no ofrece Redis a los plugins y su binario no incluye ninguna biblioteca de Redis, así que enlazar una rompería la regla de D13. Solo admite un servidor Redis o un punto de entrada único. En Enterprise, usar los pools del gateway con `RedisRegisterer` queda como mejora posterior.
- **Fallos**: cerrado por defecto.

Solo se inspeccionan las respuestas de las operaciones que pueden crear tareas o contextos, `SendMessage` y `SendStreamingMessage`, y la de `GetExtendedAgentCard` para reescribirla. El resto pasa sin almacenarse en memoria. La inspección de `SendMessage` tiene un límite de tamaño. Si se supera, la respuesta se entrega y la tarea queda sin registrar, con lo que los accesos posteriores se deniegan por defecto.

### D11. Guardia de webhooks push

Se valida el esquema, el host contra una lista de patrones permitidos y que la dirección no sea de loopback, privada, link-local o de metadatos de nube, tanto si viene literal como tras resolver el nombre. Es defensa en profundidad: quien realmente llama al webhook es el agente, así que el control de salida de red del agente sigue siendo necesario. Queda documentado como límite.

La guardia se aplica en el borde y no en la autorización: no necesita identidad, y así una URL rechazada no llega ni al pipeline. Un host que no se puede resolver se rechaza, salvo que figure de forma explícita en `allowed_hosts`. Las URLs se buscan en los campos de las dos versiones del protocolo a la vez.

### D12. Mapa de errores

| Situación | Estado HTTP | Cuerpo |
|---|---|---|
| JSON mal formado, sobre inválido, lote | 200 | `-32700` o `-32600` |
| Método desconocido o de otra versión | 200 | `-32601` |
| Versión no habilitada | 200 | `-32009` |
| Operación deshabilitada | 200 | `-32004` o `-32003` |
| Cuerpo demasiado grande | 413 | Error JSON-RPC `-32600` |
| Sin credenciales o token inválido | 401 del validador nativo o del plugin | `-32603` con el texto del estado; se añade `WWW-Authenticate` |
| Rol o scope insuficiente | 403 | `-32603` con el texto del estado |
| Tarea o contexto ajeno o no registrado | 200 | `-32001`, igual que si no existiera |
| `ListTasks` sin contexto exigido, webhook no permitido | 200 | `-32602` |
| `ListTasks` con `list_tasks` a `deny` | 200 | `-32004` |
| Almacén de propiedad caído | 200 | `-32603` |
| Endpoint interno sin el modificador `a2a-authz` y el agente respondió | 200 | `-32603`; registro crítico |
| Error del pipeline sin cuerpo, por ejemplo 429, 503 o 504 | Se conserva | Se añade `-32603` |
| Respuesta del agente | Se conserva | Sin cambios |
| Descubrimiento sin card publicable | 503 | `{"error": ...}`, con `Retry-After` |

JSON-RPC no tiene códigos de autenticación y A2A no los define, así que las denegaciones a nivel HTTP llevan `-32603` con el texto del estado HTTP como mensaje. El límite de peticiones nativo responde 503 en ambas ediciones.

La autenticación y la autorización por scopes se expresan en HTTP, como recomienda A2A. La propiedad se expresa como "no encontrado" para no revelar la existencia de recursos ajenos. Como es el borde quien escribe la respuesta final, el resultado no depende de cómo cada edición traduce los errores de un modificador.

### D13. Solo biblioteca estándar y Go 1.26 como mínimo común

Un plugin debe coincidir con el binario que lo carga en la versión de Go y en la de cada dependencia compartida. Con dos binarios distintos como objetivo, cada dependencia externa duplica el riesgo de fallo de carga. Por eso ni el núcleo ni el adaptador importan nada externo, y tampoco el SDK oficial `a2a-go`, que arrastra gRPC y protobuf.

El código se limita a lo disponible en Go 1.26, la versión de Community 2.13.11, y se compila y prueba también con Go 1.27, la de Enterprise 3.0.4.

El SDK oficial sí se usa como cliente y agente de referencia en las pruebas de conformidad, en un módulo Go separado que no forma parte del plugin.

### D14. Configuración

El bloque A2A tiene el mismo esquema en ambas ediciones:

```json
{
  "public_url": "https://gw.example.com",
  "base_path": "/a2a",
  "internal_prefix": "/__a2a",
  "route_granularity": "class",
  "default_agent": "billing",
  "limits": { "max_body_bytes": 4194304, "max_inspect_bytes": 8388608 },
  "identity": {
    "required": true,
    "headers": { "issuer": "X-A2A-Iss", "subject": "X-A2A-Sub", "scopes": "X-A2A-Scope" },
    "owner_scope": ["issuer", "subject"]
  },
  "authorization": {
    "operation_scopes": { "GetTask": ["a2a.read"], "CancelTask": ["a2a.write"] }
  },
  "ownership": {
    "enabled": true, "store": "redis", "ttl": "168h",
    "redis": { "address": "redis.internal:6379", "tls": true, "password_env": "A2A_REDIS_PASSWORD" },
    "unknown_task": "deny", "list_tasks": "owned_context", "on_store_error": "deny"
  },
  "push_notifications": {
    "require_https": true, "deny_private_networks": true,
    "allowed_hosts": ["*.hooks.example.com"]
  },
  "challenge": { "authorization_servers": ["https://idp.example.com/realms/agents"] },
  "agents": [
    {
      "id": "billing",
      "versions": ["1.0", "0.3"],
      "card": { "source": "upstream", "url": "https://billing.internal/.well-known/agent-card.json", "ttl": "5m", "max_stale": "1h" },
      "capabilities": { "streaming": true, "push_notifications": false },
      "disabled_operations": ["ListTasks"]
    }
  ]
}
```

Lo que cambia entre ediciones es el fichero que envuelve ese bloque.

Community 2.13, con `BLOQUE` como el bloque A2A anterior:

```json
{
  "version": 3,
  "plugin": { "pattern": ".so", "folder": "/opt/krakend/plugins/" },
  "extra_config": {
    "plugin/http-server": { "name": ["a2a-gateway"], "a2a-gateway": "BLOQUE" }
  }
}
```

Enterprise 3:

```json
{
  "version": 4,
  "extra_config": {
    "plugin": { "pattern": ".so", "folder": "/opt/krakend/plugins/" },
    "plugin/http-server": { "plugins": [ { "name": "a2a-gateway", "config": "BLOQUE" } ] }
  }
}
```

El modificador de autorización se declara en cada endpoint interno con `plugin/req-resp-modifier`, también con la lista `name` en Community y la lista `plugins` en Enterprise.

- **Validación.** Estricta al arrancar, con esquema JSON publicado. Un bloque inválido impide el arranque. Los secretos se referencian por nombre de variable de entorno y no se escriben en el fichero.
- **Plantillas.** Flexible Config existe en ambas ediciones. Se entrega un juego de plantillas por edición que parte de la misma lista de agentes y genera el fichero completo, con `input_headers`, `propagate_claims`, tiempos de espera y el modificador de autorización ya colocados. Es la forma recomendada de no olvidar ninguna pieza de seguridad.

### D15. Estructura del repositorio y construcción

```
a2a-plugin/
  core/                 módulo Go sin dependencias externas, Go 1.26
    config/             bloque A2A, validación
    protocol/           sobre JSON-RPC, operaciones, versiones, extracción de campos
    card/               origen, caché, reescritura, firmas
    edge/               punto [1]: envoltorio http.Handler, respuesta, SSE
    authz/              punto [3]: decisión con identidad
    ownership/          interfaz de almacén, memoria y cliente Redis mínimo
    pushguard/          validación de webhooks
  plugin/               package main: símbolos de plugin, único para ambas ediciones
  build/
    community/          Dockerfile con krakend/builder:2.13.11
    enterprise/         Dockerfile con krakend/builder-ee:3.0.4
  config/               esquema JSON, plantillas por edición, ejemplos
  deploy/               entorno de demo con agente simulado, IdP y Redis
  e2e/                  conformidad con el SDK oficial como cliente, módulo propio
  docs/
```

- Tests del núcleo: nativos en Windows con `go test`.
- Cada compilación ejecuta la comprobación de compatibilidad y la de carga de su edición.
- Cada artefacto se nombra con su edición y versión de destino, por ejemplo `krakend-a2a-ce-2.13.11.so`, y va acompañado de un manifiesto con edición, versión de KrakenD y versión de Go. Un script compara el manifiesto con `krakend version` antes de desplegar.
- En Enterprise, la prueba de carga del plugin necesita la licencia. La integración continua la recibe como secreto en tiempo de ejecución.
- **Licencia de Enterprise**: se monta en tiempo de ejecución mediante `KRAKEND_LICENSE_PATH` desde una ruta fuera del repositorio. Reglas de exclusión en `.gitignore` y `.dockerignore`. Ningún documento, log o artefacto nombra a su titular.

### D16. Observabilidad

El pipeline nativo ya da métricas y trazas por endpoint interno, es decir, por agente y clase de operación. El plugin añade un registro estructurado por petición con agente, operación, versión, decisión, código de error y resumen del principal, a través del logger que le inyecta KrakenD. No importa bibliotecas de telemetría, por la razón de D13.

`GET {internal_prefix}/_stats` devuelve las entradas de correlación vivas, solo para peticiones desde loopback. Lo usa la prueba de carga desde dentro del contenedor y no es accesible desde fuera.

### D17. Decisiones tomadas durante la implementación

- **El modificador sabe a qué agente pertenece.** `a2a-authz` admite la opción `agent`. Si un endpoint interno recibe una petición de otro agente, por un error de configuración, deniega y registra un error crítico.
- **GET sobre la URL del agente responde 405.** La spec lo exige. El resolutor del SDK oficial trata cualquier URL con ruta como la URL de la card, así que los clientes del SDK deben usar la URL completa `{base_path}/{agent}/.well-known/agent-card.json`. Se documenta en las guías de instalación.
- **Firma de cards y forma compatible.** El SDK firma la card 1.0, pero su productor compatible publica campos de 0.3 que no estaban firmados, así que una card compatible firmada no verifica ni con el propio SDK. El entorno de pruebas usa una card 1.0 firmada por el SDK para verificar la compatibilidad de la verificación JCS.
- **El agente de referencia confía en `X-A2A-Sub`.** El SDK acota `ListTasks` al usuario de la llamada. Un agente detrás del gateway puede tomar ese usuario de la identidad propagada, siempre que solo sea accesible a través del gateway.
- **Los registros concatenan los argumentos.** El logger de KrakenD separa con espacios los argumentos de cada llamada; el plugin los concatena antes.
- **Rendimiento de Enterprise.** En la prueba de carga, un Enterprise 3.0.4 sin el plugin rindió igual que con él, unas 230 peticiones por segundo en `/__health`, frente a más de 14 000 en Community en la misma máquina. El límite no lo introduce el plugin; no se investigó para no inspeccionar la licencia disponible.

## Risks / Trade-offs

- **[Community 2.13 está congelada y no recibe correcciones de seguridad]** → Se documenta en la matriz de compatibilidad. El núcleo es independiente de la edición, así que cambiar más adelante a otra vía para Community solo afecta al empaquetado. Se recomienda no exponer Community 2.13 a internet sin una capa de protección delante.
- **[Dos versiones de Go y dos binarios como objetivo]** → Núcleo limitado a Go 1.26, sin dependencias, y compilación y pruebas con ambas imágenes en cada cambio.
- **[El plugin es frágil ante cambios de versión de KrakenD]** → Compilación fijada a la versión exacta, comprobaciones de compatibilidad en cada build y matriz de compatibilidad publicada.
- **[El cableado de plugins de la línea 2.13 no pudo verificarse en el código fuente, solo en la documentación]** → Es el primer punto de la validación previa.
- **[KrakenD podría publicar soporte A2A nativo]** → La lista declarativa de agentes y las plantillas hacen que la migración sea de configuración. El plugin no ocupa espacios de nombres oficiales.
- **[Un endpoint interno sin el modificador de autorización deja pasar la petición al agente antes de detectarlo]** → El borde retiene la respuesta y registra un error crítico. Las plantillas colocan el modificador siempre. Una operación con efecto podría ejecutarse una vez antes de corregir la configuración.
- **[Un cliente Redis propio es menos completo que una biblioteca]** → Alcance mínimo y acotado, con pruebas contra un Redis real. Sin soporte de clúster ni de sentinel en esta versión.
- **[Sin Redis, la propiedad no se comparte entre nodos]** → El almacén en memoria se documenta como válido solo para un nodo, y el arranque avisa si se usa con control de acceso activado.
- **[Almacenar en memoria peticiones y algunas respuestas consume memoria]** → Límites configurables y análisis superficial. Solo se inspeccionan las respuestas de tres operaciones.
- **[La guardia de webhooks no impide que el agente resuelva otra dirección después]** → Se documenta como defensa en profundidad y se recomienda control de salida en la red del agente.
- **[La especificación A2A 1.0 tiene inconsistencias y evolucionará]** → Reescritura tolerante, tabla de rutas de campos por versión y pruebas de conformidad contra el SDK oficial.
- **[Tareas creadas antes de activar el control de acceso quedan sin propietario]** → Política `unknown_task` configurable, con posibilidad de un periodo de transición en modo permisivo.
- **[Dos formatos de fichero de configuración]** → Plantillas por edición a partir de la misma lista de agentes.

- **[Un artefacto de la edición equivocada aborta el arranque del gateway]** → Cada artefacto lleva en el nombre su edición y versión de destino, y va acompañado de un manifiesto. Un script de comprobación compara el manifiesto con `krakend version` antes de desplegar. Validado en la fase 0: el error es fatal y las opciones de KrakenD para fallos de carga no lo contienen.

### Supuestos validados en la fase 0

Validados el 13 de noviembre de 2026 con el prototipo de `a2a-plugin/prototype/`. Detalle y evidencias en `a2a-plugin/docs/validacion-supuestos.md`.

| # | Supuesto | Community 2.13.11 | Enterprise 3.0.4 |
|---|---|---|---|
| S1 | Un plugin de servidor puede cambiar la ruta de la petición antes de ceder el control y el enrutador la respeta | Confirmado | Confirmado |
| S2 | El flush en el envoltorio de respuesta del plugin de servidor entrega el SSE sin retención con `no-op` | Confirmado | Confirmado |
| S3 | Los símbolos de servidor y de modificador del mismo `.so` comparten estado en memoria | Confirmado | Confirmado |
| S4 | El modificador de petición ve las cabeceras de `propagate_claims` listadas en `input_headers` y puede cortar el pipeline antes de llamar al agente | Confirmado | Confirmado |
| S5 | El mismo código compila y carga con las dos imágenes de compilación, y la configuración le llega con la forma documentada en cada edición | Confirmado | Confirmado |
| S6 | La configuración generada por las plantillas supera `krakend check` en cada edición | Confirmado | Confirmado |
| S7 | En Enterprise, la carga de plugins no depende de tener licenciado el módulo AI Gateway | No aplica | Parcial |

S7 es parcial porque no se inspeccionó qué módulos incluye la licencia disponible. No se aplicó ninguna de las alternativas previstas: ni la degradación de streaming ni el modificador a nivel de backend.

Sin el flush del plugin, Community 2.13 retiene todo el flujo SSE hasta el final. Enterprise hace flush por su cuenta, así que el flush del plugin es imprescindible en Community e inocuo en Enterprise.

## Migration Plan

Es un plugin nuevo y no hay nada que migrar. El despliegue es incremental:

1. **Fase 0, validación.** Prototipos mínimos para S1 a S7.
2. **Fase 1, pasarela sin estado.** Descubrimiento, protocolo, streaming y plantillas, con autenticación JWT nativa por clase de operación. Ya es útil para agentes que gestionan su propia autorización.
3. **Fase 2, control de acceso.** Punto [3], scopes por operación, propiedad con memoria y con Redis, guardia de webhooks, card extendida y metadatos de recurso protegido.
4. **Fase 3, endurecimiento.** Conformidad con el SDK oficial, pruebas de carga, verificación de firma de cards y matriz de compatibilidad.

Activación recomendada del control de acceso: primero con `unknown_task: passthrough` para poblar el registro, después con `deny`.

Vuelta atrás: basta con retirar el plugin de la configuración y publicar los agentes como endpoints `no-op` normales. El registro de propiedad puede descartarse sin pérdida de datos de negocio.

## Open Questions

- ¿Conviene compilar también para Enterprise 2.13? El código y el formato de configuración serían los de Community 2.13, así que el coste es bajo.
- ¿Se añadirá después el binding HTTP+JSON? El borde ya está pensado para normalizar sus rutas a las mismas rutas internas, pero no forma parte de este cambio.
- ¿Conviene refirmar las Agent Cards con una clave del gateway cuando los clientes empiecen a exigir firma?
- ¿Hace falta un catálogo de agentes no estándar además de las cards individuales?
- ¿Interesa exponer métricas propias del plugin, además de las nativas por endpoint, y por qué vía sin añadir dependencias?
- ¿Se publicará el plugin como código abierto y con qué licencia?
