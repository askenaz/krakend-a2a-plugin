# Demostración del plugin A2A para KrakenD con agentes Pydantic AI

Dos agentes de [Pydantic AI](https://ai.pydantic.dev/) servidos con el protocolo A2A mediante FastA2A, detrás de KrakenD Community 2.13.11 con el plugin A2A:

- **soporte** responde dudas de clientes con una pequeña base de conocimiento, a través de la herramienta `buscar_articulo`.
- **coordinador** atiende al cliente y consulta al agente de soporte por A2A con la herramienta `consultar_soporte`. Esa consulta también pasa por el gateway, con un token propio del coordinador, y la respuesta llega por streaming SSE.

```
cliente ──► KrakenD + plugin A2A ──► soporte
                 ▲    │
                 │    └────────────► coordinador
                 └── consultar_soporte (A2A 0.3 en streaming, token del coordinador)
```

KrakenD valida el token con su autenticación nativa. El plugin publica las Agent Cards reescritas, encamina las operaciones A2A y comprueba quién es propietario de cada tarea y conversación.

## Requisitos

- Docker con Docker Compose.

La demostración vive dentro del repositorio del plugin, en `a2a-plugin/demos/pydantic-ai`. De ese mismo repositorio salen la imagen de KrakenD con el plugin, sus plantillas de configuración y el IdP simulado, así que basta con clonarlo.

## Arrancar y probar

Desde la raíz del repositorio clonado:

```bash
cd a2a-plugin/demos/pydantic-ai
```

```bash
docker compose up -d --build
```

```bash
docker compose run --rm pruebas
```

El primer arranque genera en `.secrets`, excluido de git, los secretos de los clientes del IdP. Para rotarlos, borra esa carpeta y vuelve a levantar el entorno.

Para pararlo:

```bash
docker compose down
```

| Servicio | Puerto local |
|---|---|
| Gateway KrakenD con el plugin | 19080 |
| IdP simulado | 19090 |

Los agentes no publican ningún puerto: solo se alcanzan a través del gateway.

## Modelo de los agentes

Sin credenciales, los agentes usan un modelo local y determinista, un `FunctionModel` de Pydantic AI. Llama a las herramientas y redacta la respuesta con reglas fijas, y en streaming emite la respuesta a trozos. Así la demostración prueba el gateway sin coste y sin claves.

Con la variable `ANTHROPIC_API_KEY` definida al levantar el entorno, los agentes usan Claude Opus 5.5 (`anthropic:claude-opus-5-5`). Tienen activado el reintento del lado del servidor en otro modelo si Claude rechaza una petición por sus clasificadores de seguridad.

```bash
ANTHROPIC_API_KEY=<tu clave> docker compose up -d --build
```

`DEMO_MODELO=local` o `DEMO_MODELO=claude` fuerzan la elección. Con Claude, las pruebas comprueban el flujo pero no el texto exacto de las respuestas.

## Qué comprueban las pruebas

`pruebas/probar_demo.py` hace todas las peticiones al gateway, nunca directamente a los agentes:

- **Descubrimiento.** Cada Agent Card se publica con `ETag`, anuncia solo la URL del gateway, no filtra las URLs internas y declara el esquema Bearer. La raíz publica la card del agente por defecto.
- **Autenticación y validación.** Sin token, la respuesta es 401 con desafío Bearer. Las rutas internas, los agentes no configurados, los métodos desconocidos y los cuerpos que no son UTF-8 se rechazan.
- **Conversación en A2A 1.0.** `SendMessage` y `GetTask` completan la tarea.
- **Propiedad.** Otro usuario no ve la tarea, ni con una cabecera `X-A2A-Sub` falsificada, y no puede escribir en la conversación de otro. El propietario sí la continúa.
- **A2A 0.3.** `message/send` y `tasks/get` funcionan sin cabecera `A2A-Version`.
- **Streaming.** `SendStreamingMessage` llega por SSE evento a evento, no al final.
- **Agente a agente.** El coordinador consulta a soporte a través del gateway y su respuesta incluye la de soporte.

## Probar a mano

Un token de la usuaria `ana`, con su secreto de `.secrets/idp/clients.json`:

```bash
TOKEN=$(curl -s -u "ana-app:$(python -c "import json;print(json.load(open('.secrets/idp/clients.json'))['ana-app']['secret'])")" -d grant_type=client_credentials http://localhost:19090/token | python -c "import sys,json;print(json.load(sys.stdin)['access_token'])")
```

La Agent Card que publica el gateway:

```bash
curl -s http://localhost:19080/a2a/soporte/.well-known/agent-card.json
```

Un mensaje al coordinador. La respuesta trae el id de la tarea:

```bash
curl -s http://localhost:19080/a2a/coordinador -H "Authorization: Bearer $TOKEN" -H "A2A-Version: 1.0" -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","id":1,"method":"SendMessage","params":{"message":{"messageId":"m1","role":"ROLE_USER","parts":[{"text":"Donde descargo mi factura"}]}}}'
```

El estado y el resultado de la tarea, con el id de la respuesta anterior. El coordinador tarda unos segundos, porque espera a soporte y el modelo local emite la respuesta a trozos: repite la petición hasta ver `TASK_STATE_COMPLETED`.

```bash
curl -s http://localhost:19080/a2a/coordinador -H "Authorization: Bearer $TOKEN" -H "A2A-Version: 1.0" -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","id":2,"method":"GetTask","params":{"id":"<id de la tarea>"}}'
```

En Git Bash sobre Windows, escribe el texto de los mensajes sin tildes ni eñes: la consola puede enviarlos con otra codificación que no es UTF-8, y el gateway rechaza el cuerpo con `-32700`.

## Notas

- **ListTasks deshabilitado.** FastA2A 2.1.1 no implementa `ListTasks`, así que la configuración del gateway la deshabilita y el gateway la rechaza con `-32004`.
- **Notificaciones push.** Los agentes no las ofrecen y el gateway no las expone.
- **Propiedad en memoria.** Hay un solo nodo del gateway. Con varios nodos, la propiedad debe ir a Redis.
- **Versiones.** pydantic-ai-slim 2.68.0, fasta2a 2.1.1 y KrakenD Community 2.13.11. Pydantic AI 2 retiró `Agent.to_a2a()`, y los agentes se sirven con `fasta2a.pydantic_ai.agent_to_a2a`.

## Estructura

| Ruta | Contenido |
|---|---|
| `compose.yml` | Entorno completo: secretos, IdP, agentes, gateway y pruebas |
| `gateway/settings/a2a.json` | Configuración del gateway y de los agentes para las plantillas del plugin |
| `python/demo/` | Agentes, modelo local, cliente A2A del coordinador y generador de secretos |
| `pruebas/probar_demo.py` | Pruebas de extremo a extremo contra el gateway |
