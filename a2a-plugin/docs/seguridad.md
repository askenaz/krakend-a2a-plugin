# Modelo de seguridad

Este documento explica qué protege el plugin A2A, qué deja en manos de KrakenD y qué queda fuera de su alcance.

## Reparto de responsabilidades

| Pregunta | Quién la responde |
|---|---|
| ¿El token es auténtico, vigente y del emisor esperado? | KrakenD, con `auth/validator` y la revocación nativa |
| ¿Tiene el rol que exige el endpoint? | KrakenD, con `auth/validator` |
| ¿Puede este cliente usar este agente? | KrakenD, con el `auth/validator` de los endpoints internos del agente. Las plantillas lo ajustan por agente con `gateway.auth` |
| ¿Es el gateway quien llama al agente? | KrakenD, con la configuración del backend. Las plantillas la ajustan por agente con `gateway.backend_extra_config` |
| ¿Tiene los scopes que exige esta operación A2A? | El plugin, con `authorization.operation_scopes` |
| ¿Puede este principal tocar esta tarea o este contexto? | El plugin, con el registro de propiedad |
| ¿Es aceptable la URL de webhook que pide el cliente? | El plugin, con la guardia de webhooks |
| ¿Qué Agent Card ven los clientes? | El plugin, que la reescribe |

El plugin no valida tokens. Lee la identidad que KrakenD ya verificó y propagó con `propagate_claims` a las cabeceras de `identity.headers`.

## A2A no tiene "token de sesión"

A2A lleva la identidad en las cabeceras HTTP estándar, nunca en el cuerpo. Lo más parecido a una sesión es la pareja de contexto y tarea. Por eso el plugin divide el problema en dos:

1. **Autenticar al llamante.** Lo hace KrakenD en cada petición.
2. **Comprobar que el llamante puede tocar esa tarea o ese contexto.** Lo hace el plugin: quien crea una tarea o un contexto queda registrado como propietario, y nadie más puede usarlo.

## Recorrido de una petición

1. **Borde, antes de enrutar.** El plugin borra de la petición todas las cabeceras `X-A2A-*` y las de identidad, valida el sobre JSON-RPC, negocia la versión, identifica la operación, aplica las operaciones habilitadas y la guardia de webhooks, y encamina la petición a una ruta interna.
2. **Endpoint interno de KrakenD.** La autenticación nativa valida el token y propaga la identidad. Después se aplican los demás componentes nativos configurados, como el límite de peticiones.
3. **Autorización A2A.** El modificador `a2a-authz` decide con la identidad ya verificada: identidad obligatoria, scopes de la operación y propiedad.
4. **Agente.** Solo recibe peticiones autorizadas. Recibe la identidad verificada, pero no el token del cliente.
5. **Borde, en la respuesta.** Registra la propiedad de lo que se haya creado antes de entregarlo, reescribe la card extendida y normaliza los errores.

## Fallo cerrado

Ante cualquier duda, el plugin deniega:

- **Sin identidad.** Una operación que exige identidad y no la tiene recibe 401, aunque el endpoint interno no tenga autenticación configurada.
- **Sin paso por el borde.** Una petición que llega a una ruta interna sin pasar por el borde se deniega: no tiene entrada de correlación.
- **Sin modificador.** Si un endpoint interno no tiene `a2a-authz` y el agente responde, el borde retiene la respuesta, devuelve `-32603` y registra un error crítico. El agente ya habrá ejecutado la operación una vez, así que las plantillas colocan siempre el modificador.
- **Almacén caído.** Con `on_store_error` a `deny`, las operaciones que necesitan consultar la propiedad se rechazan con `-32603`.
- **Tarea desconocida.** Con `unknown_task` a `deny`, una tarea sin propietario registrado se trata como inexistente.
- **Configuración inválida.** El gateway no arranca.
- **Artefacto equivocado.** El script de comprobación rechaza un `.so` de otra edición o versión antes de desplegarlo.

## No revelar la existencia de recursos ajenos

Una tarea de otro principal responde exactamente igual que una tarea inexistente: `-32001 Task not found`. Así un atacante no puede averiguar qué identificadores existen.

## Defensas frente a interpretaciones distintas del mismo JSON

El agente y el gateway deben entender la misma petición. El plugin rechaza los objetos con claves repetidas, también las que solo difieren en mayúsculas, en el sobre y en todos los campos que intervienen en las decisiones.

Sin esa comprobación, `{"method":"SendMessage","METHOD":"CancelTask"}` se autorizaría como `SendMessage` y se ejecutaría como `CancelTask`. La biblioteca JSON estándar de Go, que usa el SDK oficial de A2A, asocia los campos sin distinguir mayúsculas y se queda con el último valor.

Las URLs de webhook se buscan en los campos de las dos versiones del protocolo a la vez, para que la guardia no pierda una URL que el agente sí aceptaría.

## Guardia de webhooks

El plugin rechaza, sin llegar al agente:

- **Esquema.** URLs que no usan HTTPS, si `require_https` está activo.
- **Hosts.** Hosts fuera de `allowed_hosts`, si la lista no está vacía.
- **Redes internas.** Direcciones de loopback, privadas, link-local, de metadatos de nube, multicast y rangos reservados, tanto escritas en la URL como tras resolver el nombre.

Un host que no se puede resolver se rechaza, salvo que esté en `allowed_hosts` de forma explícita.

**Límite.** Es defensa en profundidad. Quien llama al webhook es el agente, que podría resolver el nombre a otra dirección más tarde. La red del agente debe limitar sus salidas.

## Qué recibe el agente

| Recibe | No recibe |
|---|---|
| El cuerpo JSON-RPC sin cambios | El token del cliente |
| `A2A-Version` y `A2A-Extensions` | Cabeceras `X-A2A-*` enviadas por el cliente |
| La identidad verificada en `identity.headers` | |
| `X-A2A-Operation`, `X-A2A-Agent`, `X-A2A-Version`, `X-A2A-Request-Id` | |

Un agente puede confiar en las cabeceras de identidad solo si es accesible únicamente a través del gateway. El agente de referencia del entorno de demostración lo hace así para que el SDK acote `ListTasks` al llamante.

Si el agente no puede quedar aislado en la red, haz que el gateway se autentique ante él, por ejemplo con `auth/client-credentials` en `gateway.backend_extra_config`, y que el agente rechace lo que no traiga esa credencial.

## Agent Cards

- **URLs.** La card publicada solo anuncia URLs del gateway. Las URLs con el origen interno del agente se eliminan, también en campos como `documentationUrl`.
- **Seguridad.** Si el agente tiene `security` configurado, la card anuncia los esquemas del gateway en lugar de los del agente.
- **Firmas.** La card publicada nunca lleva `signatures`, porque la reescritura invalida la firma original. Con `signature.verify`, una card remota sin ninguna firma válida no se publica.
- **Card extendida.** `GetExtendedAgentCard` exige siempre identidad y su resultado se reescribe igual.

## Registros

Por cada petición se registra el agente, la operación, la versión, el resultado y un resumen no reversible del principal. Nunca se registran el cuerpo, el token ni el sujeto en claro.

En Enterprise, KrakenD escribe al arrancar líneas sobre la licencia. No compartas esos registros sin filtrarlos.

## Activación progresiva del control de propiedad

Las tareas creadas antes de activar el plugin no tienen propietario. Para no cortar a los clientes existentes:

1. Activa el plugin con `ownership.unknown_task` a `passthrough`. Las tareas nuevas quedan registradas y las antiguas siguen accesibles.
2. Cuando las tareas antiguas hayan terminado o caducado, cambia a `deny`.

## Riesgos conocidos

- **KrakenD Community 2.13 no recibe correcciones de seguridad.** No la expongas directamente a internet; ver `compatibilidad.md`.
- **El almacén en memoria no se comparte entre nodos.** Con más de un nodo usa Redis.
- **Cliente Redis mínimo.** Admite un único servidor o un punto de entrada único, con autenticación y TLS. No admite Redis Cluster ni Sentinel.
