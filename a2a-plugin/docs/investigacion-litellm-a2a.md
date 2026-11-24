# Investigación: LiteLLM como gateway Agent2Agent (A2A)

| | |
|---|---|
| Fecha de la investigación | 24 de noviembre de 2026 |
| Código examinado | `BerriAI/litellm`, rama `main`, commit [`55d4288`](https://github.com/BerriAI/litellm/commit/55d42888213f77582131f00134979bb8c6f8f157) (24-11-2026). `pyproject.toml` declara la versión [`1.112.0`](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/pyproject.toml#L3) |
| Última etiqueta estable | `v1.110.3` (24-11-2026), según la [lista de releases](https://github.com/BerriAI/litellm/releases). El código de `main` puede ir por delante de esa etiqueta |
| Documentación examinada | `BerriAI/litellm-docs`, commit [`090f0d9`](https://github.com/BerriAI/litellm-docs/commit/090f0d98f65aa5444e058ddb32e3f2452d0207b3) (24-11-2026), que es el origen de `docs.litellm.ai` |
| Especificación A2A | [a2a-protocol.org, versión 1.0.0](https://a2a-protocol.org/latest/specification/), consultada solo para comprobar términos |
| Método | Lectura directa de los ficheros fuente y del Markdown de la documentación en los commits indicados, notas de versión, issues y PR. No se ejecutó LiteLLM |
| Alcance de las búsquedas | Leídos enteros: `proxy/agent_endpoints/` (`a2a_endpoints.py`, `endpoints.py`, `agent_registry.py`, `a2a_routing.py`, `kill_switch.py`, `auth/agent_permission_handler.py`, `auth/agent_caller.py`, `auth/agent_access_groups.py`), todo `proxy/a2a/`, `a2a_protocol/` (`main.py`, `card_resolver.py`, `exception_mapping_utils.py`, `cost_calculator.py`, `streaming_iterator.py`), `llms/a2a/common_utils.py` y `types/agents.py`. Leídos en parte o solo con búsquedas por texto: el resto de esos directorios (puentes de proveedor, identidades Entra, OAuth de Databricks, adaptador de guardrails), y `litellm/proxy/`, `litellm/types/` y `litellm/litellm_core_utils/`. Documentación y notas de versión: búsqueda por texto sobre todo el repositorio y lectura completa de las páginas de la sección «Agent Gateway» (salvo la de Databricks, de la que leí solo la estructura, y la de Skills Gateway, ajena a A2A). El código Rust no se leyó; su árbol no contiene ficheros con `a2a` en el nombre. Un «no encontrado» significa no encontrado dentro de ese alcance |

**Cómo leer el nivel de respaldo.** Cada afirmación lleva una de estas marcas:

- **[C]** confirmado en la fuente enlazada (código o documentación).
- **[NE]** no encontrado en la documentación ni en el código examinados.
- **[NC]** no pude comprobarlo (la fuente existe pero no la verifiqué, o las fuentes se contradicen).

Las versiones «desde» salen de las notas de versión. Cuando no constan, se indica el PR.

---

## 1. Tabla resumen

| Capacidad | Qué hace | Estado | Fuente |
|---|---|---|---|
| Endpoint por agente | `POST /a2a/{agent_id}` recibe JSON-RPC 2.0; `{agent_id}` es el UUID o el nombre | [C] Etiqueta OpenAPI `[beta] A2A Agents`. Desde v1.80.8 | [`a2a_endpoints.py` L777-L811](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L777-L811), [notas v1.80.8](https://github.com/BerriAI/litellm-docs/blob/090f0d98f65aa5444e058ddb32e3f2452d0207b3/release_notes/v1.80.8-stable/index.md) |
| Versiones de A2A | Sirve 0.3 o 1.0 por agente y convierte las respuestas al formato fijado | [C] Desde v1.92.0 | [`version_convert.py`](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/version_convert.py#L6-L25), [notas v1.92.0](https://github.com/BerriAI/litellm-docs/blob/090f0d98f65aa5444e058ddb32e3f2452d0207b3/release_notes/v1.92.0/index.md) |
| Bindings | Solo JSON-RPC sobre HTTP. Sin gRPC ni HTTP+JSON hacia el cliente | [C] por la card servida; [NE] para los otros dos | [`agent_card.py` L220-L226](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/agent_card.py#L220-L226) |
| Métodos | Once métodos. `message/send` y `message/stream` pasan por el cliente integrado; el resto se reenvía tal cual al agente | [C] Desde v1.89.0 ([PR #29566](https://github.com/BerriAI/litellm/pull/29566)) | [`a2a_endpoints.py` L1036-L1211](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1036-L1211) |
| Agent Card servida | Card almacenada en el registro, con la URL reescrita al gateway y el esquema de seguridad sustituido por el del gateway | [C] Fusión desde v1.88.0 | [`agent_card.py` L163-L235](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/agent_card.py#L163-L235) |
| Firma de cards | No verifica firmas del agente ni firma la card que sirve | [NE] | [`agent_card.py` L98-L132](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/agent_card.py#L98-L132) |
| Card extendida autenticada | Reenvía `agent/getAuthenticatedExtendedCard` y reescribe la URL del resultado | [C] | [`a2a_endpoints.py` L1158-L1170](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1158-L1170) |
| Registro de agentes | Alta por interfaz, API `/v1/agents` o clave `agents:` del fichero de configuración | [C] | [`endpoints.py` L619-L742](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/endpoints.py#L619-L742), [doc](https://docs.litellm.ai/docs/a2a#adding-your-agent) |
| Catálogo público | `GET /public/agent_hub` lista sin autenticación las cards marcadas como públicas | [C] Desde v1.80.0 | [`public_endpoints.py` L334-L355](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/public_endpoints/public_endpoints.py#L334-L355) |
| Búsqueda semántica | `GET /v1/agents?query=` y herramienta MCP `agent_search` | [C] Desde v1.100.0 | [doc](https://docs.litellm.ai/docs/a2a#search-the-registry) |
| Autenticación de clientes | Clave virtual en `Authorization` o `x-litellm-api-key`; JWT con licencia Enterprise | [C] | [doc](https://docs.litellm.ai/docs/auth_overview) |
| Permisos por agente | Lista de agentes en clave y equipo (intersección), grupos de acceso, denegación por defecto opcional | [C] Desde v1.80.8; `agent_access_default_deny` desde v1.108.0 | [`agent_permission_handler.py` L115-L161](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/auth/agent_permission_handler.py#L115-L161) |
| Credenciales hacia el agente | Cabeceras estáticas, cabeceras reenviadas del cliente, convención `x-a2a-{agente}-{cabecera}`, `api_key` como Bearer, Entra, OAuth M2M de Databricks, SigV4 | [C] | [doc](https://docs.litellm.ai/docs/a2a_agent_headers), [`a2a_endpoints.py` L984-L1021](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L984-L1021) |
| Identidad del llamante | Envía `X-LiteLLM-User-Id`, `X-LiteLLM-Team-Id`, `X-LiteLLM-Trace-Id` y `X-LiteLLM-Agent-Id` al agente | [C] | [`a2a_endpoints.py` L203-L253](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L203-L253) |
| Propiedad de tareas | No la aplica. Reenvía `tasks/*` y deja el aislamiento al agente | [C] por ausencia de almacén de tareas | [`a2a_endpoints.py` L1116-L1185](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1116-L1185) |
| Notificaciones push | Reenvía los métodos de configuración; valida la URL solo en `set`; no tiene receptor de webhooks | [C] | [`a2a_endpoints.py` L190-L200](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L190-L200), [`agent_card.py` L85-L96](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/agent_card.py#L85-L96) |
| Streaming | SSE (`text/event-stream`), keepalive opcional, `tasks/resubscribe` reenviado | [C] SSE desde v1.99.0 | [`a2a_endpoints.py` L476-L709](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L476-L709) |
| Límites | TPM/RPM por agente y por sesión, tope de iteraciones y presupuesto por sesión, límites y presupuestos de clave y equipo | [C] | [doc](https://docs.litellm.ai/docs/proxy/users#agents) |
| Costes | Precio fijo por consulta o por token, atribuido a clave, equipo y agente | [C] Desde v1.80.10 | [`cost_calculator.py`](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/a2a_protocol/cost_calculator.py#L30-L83) |
| Observabilidad | Fila en SpendLogs con `agent_id`, callbacks de logging estándar, cabeceras de traza | [C] | [doc](https://docs.litellm.ai/docs/a2a#tracking-agent-logs) |
| Guardrails | Sobre el texto de entrada y salida, también en streaming; configurables por agente | [C] | [`a2a_endpoints.py` L955-L1033](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L955-L1033) |
| Resiliencia | Comprobación de salud bajo demanda. Sin reintentos generales, balanceo ni alternativas entre agentes en `/a2a` | [C] / [NE] | [`endpoints.py` L271-L302](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/endpoints.py#L271-L302) |
| Puentes | Agentes A2A por `/chat/completions` (`a2a/<nombre>`); proveedores no A2A servidos como A2A | [C] | [`a2a_routing.py`](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_routing.py#L124-L195) |
| Kill switch | Webhook de parada por agente, disparado por un administrador | [C] Desde v1.104.0 | [doc](https://docs.litellm.ai/docs/a2a_kill_switch) |
| Identidad de agentes | Agentes ligados a identidades Entra ID, modos autónomo y delegado | [C] Desde v1.105.0; exige JWT (Enterprise) | [`agent_identity.py`](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/types/proxy/agent_identity.py#L9-L48) |
| Errores | JSON-RPC `-32000`, `-32600`, `-32601`, `-32602`, `-32603` con estado HTTP 4xx/5xx. No emite códigos A2A (`-32001` a `-32009`) | [C] | [`a2a_endpoints.py` L285-L334](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L285-L334) |

---

## 2. Detalle por área

### 2.1 Endpoints y forma de las rutas

**Plano de datos** [C] ([`a2a_endpoints.py` L712-L811](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L712-L811), [doc](https://docs.litellm.ai/docs/a2a#api-reference)):

| Ruta | Uso |
|---|---|
| `POST /a2a/{agent_id}` | Endpoint JSON-RPC principal. Es la URL que anuncia la card |
| `POST /a2a/{agent_id}/message/send`, `POST /v1/a2a/{agent_id}/message/send` | Alias |
| `POST /a2a/{agent_id}/message/stream`, `POST /v1/a2a/{agent_id}/message/stream` | Alias. Servidos desde v1.109.0 ([PR #49471](https://github.com/BerriAI/litellm/pull/49471)); antes daban 404 ([issue #49433](https://github.com/BerriAI/litellm/issues/49433)) |
| `GET /a2a/{agent_id}/.well-known/agent-card.json` y `.../agent.json` | Agent Card del agente tal como la sirve el gateway |
| `POST /v1/a2a/discover` | Solo administrador. Descarga la card del agente remoto durante el alta ([`proxy/a2a/endpoints.py` L67-L113](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/endpoints.py#L67-L113)) |

- `{agent_id}` admite el UUID o el nombre registrado [C] ([doc](https://docs.litellm.ai/docs/a2a#endpoints), [`a2a_endpoints.py` L353-L359](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L353-L359)).
- Las cinco rutas POST comparten un único manejador y el método se lee del cuerpo, no de la ruta [C] ([L806-L848](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L806-L848)). La documentación describe los alias como «solo `message/send`» ([doc](https://docs.litellm.ai/docs/a2a#endpoints)); en el código no hay restricción por ruta.
- Las rutas de la card exigen clave válida y permiso sobre el agente (404 si no existe, 403 si no está permitido). No son públicas [C] ([L712-L768](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L712-L768)).
- No hay una card ni un catálogo en la raíz del gateway (`/.well-known/agent-card.json`) [NE]. El catálogo es `GET /v1/agents` y `GET /public/agent_hub` (ver 2.4).
- Las rutas de invocación se clasifican como plano de datos y las de registro como plano de control, con interruptores separados (`DISABLE_LLM_API_ENDPOINTS`, `DISABLE_ADMIN_ENDPOINTS`) [C] ([`_types.py` L547-L566](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/_types.py#L547-L566)).

**Plano de control** [C] ([`endpoints.py`](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/endpoints.py#L410-L1455)): `GET/POST /v1/agents`, `GET/PUT/PATCH/DELETE /v1/agents/{agent_id}`, `POST /v1/agents/{agent_id}/make_public`, `POST /v1/agents/make_public`, `POST /v1/agents/{agent_id}/kill_switch`, `GET /v1/agents/{agent_id}/identity`, `GET /v1/agents/identity/providers`, `GET /agent/daily/activity`.

**Estado.** Todas las rutas llevan la etiqueta OpenAPI `[beta] A2A Agents` [C] ([L714](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L712-L716)). La documentación no las marca como beta. No encontré comprobaciones de licencia (`premium_user`) en los módulos `proxy/agent_endpoints`, `proxy/a2a`, `a2a_protocol` ni `llms/a2a` [NE].

### 2.2 Protocolo: versiones, bindings y operaciones

**Versiones.**

- El gateway sirve A2A 0.3 o 1.0 por agente. Se fija con `agent_card_params.protocolVersion` (`"0.3"` o `"1.0"`; acepta formas semver como `0.3.0`). Cualquier otro valor se rechaza con HTTP 400 en el alta [C] ([`endpoints.py` L104-L114](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/endpoints.py#L104-L114), [`agent_card.py` L17-L45](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/agent_card.py#L17-L45), [doc](https://docs.litellm.ai/docs/a2a#protocol-versioning)). Semver aceptado desde v1.95.0 ([notas](https://github.com/BerriAI/litellm-docs/blob/090f0d98f65aa5444e058ddb32e3f2452d0207b3/release_notes/v1.95.0/index.md)).
- Sin versión fijada, decide por la petición: método en PascalCase (`SendMessage`...) o cabecera `a2a-version: 1.x` dan 1.0; el resto, 0.3 [C] ([`a2a_endpoints.py` L175-L187](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L175-L187)). La documentación recomienda fijar siempre la versión [C] ([doc](https://docs.litellm.ai/docs/a2a#when-protocolversion-is-not-pinned)).
- En el alta por API la fusión escribe siempre `protocolVersion` en la card almacenada (la del agente si es 0.3 o 1.0; si no, 1.0) [C] ([`agent_card.py` L197-L198](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/agent_card.py#L197-L198)). El caso «sin fijar» queda, en la práctica, para agentes del fichero de configuración sin `protocolVersion` y para filas antiguas.
- La conversión detecta la forma del payload (campo `kind` frente a envoltorio protobuf-JSON) y es «best effort»: si falla, devuelve el payload sin convertir [C] ([`version_convert.py` L6-L54](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/version_convert.py#L6-L54)).
- Hacia el agente, `message/send` y `message/stream` usan `a2a-sdk` 1.x (`>=1.1.0,<2.0`; fijado a 1.1.0 en los grupos de desarrollo) [C] ([`pyproject.toml` L139](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/pyproject.toml#L139), [`main.py` L52-L74](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/a2a_protocol/main.py#L52-L74)). La versión hacia el agente se puede forzar con `litellm_params.a2a_protocol_version` (desde v1.108.0, [PR #44463](https://github.com/BerriAI/litellm/pull/44463)) [C] ([`card_resolver.py` L113-L153](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/a2a_protocol/card_resolver.py#L113-L153)).
- **Versión no soportada en la petición:** no hay negociación. Una cabecera `a2a-version` que no empiece por `1.` se trata como 0.3. El gateway no devuelve `VersionNotSupportedError` (`-32009`) ni ningún otro código específico de A2A [C] por ausencia: no aparecen en el código ([búsqueda sobre `a2a_endpoints.py`](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L175-L187)).

**Bindings.**

- Hacia el cliente solo JSON-RPC: la card fusionada publica una única interfaz `protocolBinding: "JSONRPC"` [C] ([`agent_card.py` L220-L226](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/agent_card.py#L220-L226)).
- gRPC y HTTP+JSON/REST hacia el cliente: [NE]. `additionalInterfaces` se elimina de la card a propósito, para no publicar URLs del backend [C] ([`agent_card.py` L98-L107](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/agent_card.py#L98-L107)).
- Hacia el agente, el cliente SDK elige el transporte según la card remota; el gateway solo normaliza mayúsculas de `protocolBinding` (`jsonrpc`, `http+json`, `grpc`) [C] ([`card_resolver.py` L94-L153](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/a2a_protocol/card_resolver.py#L94-L153)). Que funcione de extremo a extremo con un agente solo gRPC: [NC].

**Operaciones** [C] ([`a2a_endpoints.py` L111-L123](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L111-L123) y [L1036-L1211](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1036-L1211), [doc](https://docs.litellm.ai/docs/a2a_agent_card#supported-a2a-methods)):

| Método 0.3 | Alias 1.0 | Tratamiento |
|---|---|---|
| `message/send` | `SendMessage` | Cliente `a2a-sdk` integrado: logging, guardrails, coste |
| `message/stream` | `SendStreamingMessage` | Igual, en streaming SSE |
| `tasks/get` | `GetTask` | Reenvío JSON-RPC a `agent_card_params.url` |
| `tasks/list` | `ListTasks` | Reenvío |
| `tasks/cancel` | `CancelTask` | Reenvío |
| `tasks/resubscribe` | `SubscribeToTask` | Reenvío en SSE |
| `tasks/pushNotificationConfig/set` | `CreateTaskPushNotificationConfig` | Reenvío, con validación de URL |
| `tasks/pushNotificationConfig/get`, `/list`, `/delete` | `Get...`, `List...s`, `Delete...` | Reenvío |
| `agent/getAuthenticatedExtendedCard` | `GetExtendedAgentCard` | Reenvío y reescritura de URL |

- Los alias PascalCase se traducen al nombre 0.3 antes de procesar [C] ([L847-L848](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L847-L848)).
- **Método desconocido:** JSON-RPC `-32601` «Method '…' not found» con HTTP 400 [C] ([L1210-L1211](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1210-L1211)).
- En los métodos reenviados, cuando se sirve 1.0 los parámetros se rebajan a 0.3 antes de reenviar y la respuesta de `tasks/get`, `tasks/cancel` y `tasks/list` se convierte a la versión servida [C] ([L1128-L1129](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1128-L1129), [L1172](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1172), [`version_convert.py` L88-L150](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/version_convert.py#L88-L150)). La documentación dice que los métodos de tareas se reenvían «sin cambios» ([doc](https://docs.litellm.ai/docs/a2a#when-protocolversion-is-not-pinned)); el código contradice esa frase.
- El reenvío usa siempre el nombre 0.3 del método y no añade la cabecera `A2A-Version` [C] ([L1151-L1157](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1151-L1157), [L388-L409](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L388-L409)). Consecuencia en 2.17 (issues #49867 y #44531).
- Los agentes definidos solo con proveedor (sin `url`) admiten únicamente `message/send` y `message/stream`; el resto responde `-32000` «has no URL configured» con HTTP 500 [C] ([L1126-L1127](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1126-L1127), [doc](https://docs.litellm.ai/docs/a2a_agent_card#requirements)).
- El gateway no interpreta `skillId`: reenvía el mensaje completo, metadatos incluidos [C] ([doc](https://docs.litellm.ai/docs/a2a_agent_card#skill-routing)).

### 2.3 Agent Cards

**Qué card se sirve.**

- La card servida es la almacenada en el registro (`agent_card_params`), no una consulta en vivo al agente [C] ([`a2a_endpoints.py` L756-L768](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L756-L768)).
- Al servirla se reescriben `url` y `supportedInterfaces[0].url` a `{base del gateway}/a2a/{agent_id}`, usando `PROXY_BASE_URL` si está definido, y se convierte a la versión servida [C] ([L762-L765](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L762-L765), [`agent_card.py` L54-L69](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/agent_card.py#L54-L69)).
- Al registrar o actualizar por API (`POST`, `PUT`, `PATCH /v1/agents`), la card recibida se fusiona [C] ([`agent_card.py` L163-L235](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/agent_card.py#L163-L235), [`endpoints.py` L117-L137](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/endpoints.py#L117-L137)):
  - `supportedInterfaces` pasa a ser una sola entrada JSON-RPC con la URL del gateway.
  - `securitySchemes` y `security` se sustituyen por un único esquema `LiteLLMKey` (`http`, `bearer`). Lo que anunciara el agente se descarta.
  - `capabilities` conserva solo `streaming`. Se eliminan `pushNotifications`, `extendedAgentCard` y `extensions`, con comentarios `TODO` que explican el motivo.
  - Se rellenan valores por defecto: `version` `1.0.0`, modos `text`, una skill `chat`, proveedor «LiteLLM Proxy».
  - Solo sobreviven las claves de una lista cerrada; `additionalInterfaces` y los campos de extensión `x-` se eliminan ([issue #27371](https://github.com/BerriAI/litellm/issues/27371), cerrado).
  - `url` se conserva con la dirección real del agente, porque el enrutado la lee de ahí.
- Los agentes declarados en el fichero de configuración se cargan sin pasar por la fusión [C] ([`agent_registry.py` L466-L505](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/agent_registry.py#L466-L505)). Para ellos la card servida es la del fichero con la URL reescrita, sin sustitución del esquema de seguridad.
- Tabla de campos soportados en la documentación: [doc](https://docs.litellm.ai/docs/a2a_agent_card#agent-card-support).

**Descubrimiento de la card remota.**

- En el alta, `POST /v1/a2a/discover` prueba `/.well-known/agent-card.json`, `/.well-known/agent.json` y `/agent.json`; tiene un modo `langgraph_platform` que añade `?assistant_id=`; tiempo de espera de 10 s [C] ([`discovery.py` L31-L50](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/discovery.py#L31-L50)). Desde v1.88.0 ([PR #28860](https://github.com/BerriAI/litellm/pull/28860)).
- Esa descarga pasa por la protección SSRF en cada salto de redirección [C] ([`discovery.py` L116-L133](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/discovery.py#L116-L133)).
- En cada `message/send` y `message/stream`, el cliente SDK vuelve a descargar la card del agente (`/.well-known/agent-card.json`, `/.well-known/agent.json`, `/agentCard/v1.0`, o la ruta fijada en `litellm_params.agent_card_path`) [C] ([`main.py` L836-L917](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/a2a_protocol/main.py#L836-L917), [`card_resolver.py` L229-L277](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/a2a_protocol/card_resolver.py#L229-L277)). No usa la card registrada. Ver issues #40586 y #49868.
- Si la card remota anuncia una URL `localhost`, `127.0.0.1` o `0.0.0.0` y la conexión falla, reintenta una vez contra la URL registrada [C] ([`exception_mapping_utils.py` L104-L164](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/a2a_protocol/exception_mapping_utils.py#L104-L164), [PR #20604](https://github.com/BerriAI/litellm/pull/20604)).

**Caché.**

- No hay caché con caducidad ni refresco automático de la card almacenada. La actualización es manual: botón «Re-sync from upstream» de la interfaz, que muestra un diff [C] ([doc](https://docs.litellm.ai/docs/a2a_agent_card#editing-the-agent-card)).
- La respuesta de la card no lleva cabeceras de caché (`ETag`, `Cache-Control`) [NE] ([L768](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L768)).
- La card remota usada al invocar no se cachea entre peticiones [NE] ([`main.py` L888-L895](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/a2a_protocol/main.py#L888-L895)); solo se reutiliza el cliente HTTP ([PR #35978](https://github.com/BerriAI/litellm/pull/35978)).

**Cards estáticas.** Sí: la card puede escribirse entera en `agents:` del fichero de configuración o enviarse en el cuerpo de `POST /v1/agents`, sin que el agente publique ninguna [C] ([doc](https://docs.litellm.ai/docs/a2a#define-agents-in-configyaml)). Aun así, la invocación por `message/send` intenta descargar la card remota (ver arriba).

**Firmas.**

- No hay código que verifique firmas JWS de la card remota ni que firme la card servida [NE] (búsqueda de `signature`, `jws`, `jwks` en los módulos A2A).
- La documentación marca `signatures` como no soportado ([doc](https://docs.litellm.ai/docs/a2a_agent_card#agentcard-441)). El código mantiene `signatures` en la lista de claves que sobreviven a la fusión [C] ([`agent_card.py` L125](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/agent_card.py#L125)); si el agente la traía, se serviría una firma que ya no corresponde al contenido modificado. Efecto real: [NC].

**Card extendida autenticada.**

- `agent/getAuthenticatedExtendedCard` se reenvía al agente con las cabeceras del agente. En el resultado se reescriben `url` y `supportedInterfaces[0].url` al gateway y se convierte a la versión servida [C] ([L1158-L1170](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1158-L1170)).
- El resultado no pasa por la fusión: no se sustituyen los esquemas de seguridad ni se filtran capacidades. `additionalInterfaces` solo se elimina al rebajar a 0.3 [C] ([`version_convert.py` L118-L130](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/version_convert.py#L118-L130), [L299-L317](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/version_convert.py#L299-L317)).
- La card servida no anuncia la capacidad `extendedAgentCard` [C] ([`agent_card.py` L93-L96](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/agent_card.py#L93-L96)), aunque el método funciona.

### 2.4 Registro de agentes y descubrimiento

**Alta.**

| Vía | Detalle | Fuente |
|---|---|---|
| Interfaz | «Agentic > Agents > Add New Agent»: nombre, URL, versión de protocolo, descubrimiento de la card, selección de skills | [C] [doc](https://docs.litellm.ai/docs/a2a#add-a2a-agents) |
| API | `POST /v1/agents` con `agent_name`, `agent_card_params`, `litellm_params`, `static_headers`, `extra_headers`, `tpm_limit`, `rpm_limit`, `session_tpm_limit`, `session_rpm_limit`, `object_permission`, `access_group_ids`, `kill_switch`, `identity`. Solo administrador del proxy | [C] [`types/agents.py` L255-L270](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/types/agents.py#L255-L270), [`endpoints.py` L250-L262](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/endpoints.py#L250-L262) |
| Fichero | Clave `agents:` (antes `agent_list`). Requiere `agent_name` y `agent_card_params`. Leída correctamente desde v1.96.0 | [C] [doc](https://docs.litellm.ai/docs/a2a#define-agents-in-configyaml), [`proxy_server.py` L7555](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/proxy_server.py#L7555) |
| CLI y Terraform | Existen un comando `agents` y recursos Terraform | [C] existen ([`cli/commands/agents.py`](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/client/cli/commands/agents.py)); contenido [NC] |

- Almacenamiento: tabla `LiteLLM_AgentsTable` más un registro en memoria; si un nombre no está en memoria se consulta la base de datos (pensado para varias réplicas) [C] ([`schema.prisma` L269-L295](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/schema.prisma#L269-L295), [`registry_read_through.py` L334-L340](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/common_utils/registry_read_through.py#L334-L340)).
- El nombre es único (400 o 409 si se repite). Si un agente de base de datos y uno del fichero comparten nombre, gana el de base de datos [C] ([`endpoints.py` L683-L689](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/endpoints.py#L683-L689), [`agent_registry.py` L466-L505](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/agent_registry.py#L466-L505)).
- El identificador de un agente del fichero es el SHA-256 de su nombre [C] ([`agent_registry.py` L452-L453](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/agent_registry.py#L452-L453)).
- Los valores sensibles de `litellm_params` y del kill switch se devuelven enmascarados (`REDACTED_BY_LITELM`) a todos, administradores incluidos. `static_headers` se oculta a no administradores pero se devuelve en claro a administradores [C] ([`endpoints.py` L210-L247](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/endpoints.py#L210-L247); [issue #49430](https://github.com/BerriAI/litellm/issues/49430), abierto).
- Alta automática desde catálogos de nube (Vertex, Foundry): [NE]; es una petición abierta ([issue #45430](https://github.com/BerriAI/litellm/issues/45430)).

**Descubrimiento para clientes.**

- `GET /v1/agents`: lista los agentes a los que la clave tiene concesión explícita; un administrador ve todos. Desde v1.101.0 una clave sin concesiones recibe lista vacía, aunque pueda invocar agentes por nombre [C] ([`agent_permission_handler.py` L845-L878](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/auth/agent_permission_handler.py#L845-L878), [notas v1.101.0](https://github.com/BerriAI/litellm-docs/blob/090f0d98f65aa5444e058ddb32e3f2452d0207b3/release_notes/v1.101.0/index.md)).
- A no administradores se les devuelve la URL del gateway en lugar de la del backend (desde v1.109.0, [PR #49483](https://github.com/BerriAI/litellm/pull/49483)) [C] ([`endpoints.py` L185-L207](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/endpoints.py#L185-L207)).
- `GET /v1/agents?health_check=true` filtra los agentes cuya URL no responde (ver 2.14) [C].
- `GET /v1/agents?query=<tarea>&top_k=N` ordena por similitud semántica sobre nombre, descripción y skills. Requiere `litellm_settings.agent_search_embedding_model`; sin él, 400 `agent_search_not_configured`; si falla el embedding, 503 `agent_search_unavailable` [C] ([doc](https://docs.litellm.ai/docs/a2a#search-the-registry), [`endpoints.py` L315-L338](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/endpoints.py#L315-L338)). Desde v1.100.0 ([PR #38609](https://github.com/BerriAI/litellm/pull/38609)).
- **AI Hub / Agent Hub.** `POST /v1/agents/{id}/make_public` o `POST /v1/agents/make_public` añaden agentes a `litellm_settings.public_agent_groups`. `GET /public/agent_hub` devuelve sus cards con `url` apuntando al gateway. Es una ruta pública, sin autenticación [C] ([`public_endpoints.py` L334-L355](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/public_endpoints/public_endpoints.py#L334-L355), [`_types.py` L822](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/_types.py#L822), [doc](https://docs.litellm.ai/docs/proxy/ai_hub#agents)). Desde v1.80.0 ([notas](https://github.com/BerriAI/litellm-docs/blob/090f0d98f65aa5444e058ddb32e3f2452d0207b3/release_notes/v1.80.0-stable/index.md)).

### 2.5 Autenticación de clientes y control de acceso

**Autenticación.**

- Clave virtual de LiteLLM en `Authorization: Bearer sk-…` o en `x-litellm-api-key`. La segunda se recomienda cuando `Authorization` lleva un token destinado al agente. Las rutas A2A aceptan además los alias `API-Key`, `x-api-key`, `x-goog-api-key` y `Ocp-Apim-Subscription-Key` [C] ([doc](https://docs.litellm.ai/docs/auth_overview#1-client--litellm-authenticating-the-caller)).
- JWT: la autenticación JWT del proxy se aplica también a A2A. `agent_id_jwt_field` liga un claim verificado a un agente registrado. Es función Enterprise [C] ([doc](https://docs.litellm.ai/docs/proxy/jwt_auth_reference#bind-jwts-to-registered-agents)).
- OAuth 2.0 del lado del cliente específico para A2A (flujos anunciados en la card, introspección): [NE]. La card fusionada solo anuncia el esquema `LiteLLMKey` [C] ([`agent_card.py` L72-L83](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/agent_card.py#L72-L83)). Existe una página genérica de OAuth2 del proxy ([doc](https://docs.litellm.ai/docs/proxy/oauth2)); su aplicación a `/a2a`: [NC].
- mTLS de cliente: [NE].

**Permisos por agente.**

- Campos: `object_permission.agents` (lista de identificadores) y `object_permission.agent_access_groups` en clave y en equipo; también grupos de acceso unificados (`access_group_ids`) [C] ([doc](https://docs.litellm.ai/docs/a2a_agent_permissions), [`agent_permission_handler.py` L333-L394](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/auth/agent_permission_handler.py#L333-L394)).
- Resolución: si clave y equipo restringen, intersección; si solo uno restringe, vale ese; si ninguno, acceso a todos los agentes [C] ([L115-L127](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/auth/agent_permission_handler.py#L115-L127), [doc](https://docs.litellm.ai/docs/a2a_agent_permissions#how-it-works)).
- **Abierto por defecto.** `general_settings.agent_access_default_deny: true` lo invierte para claves no administradoras. Desde v1.108.0 ([PR #43979](https://github.com/BerriAI/litellm/pull/43979)) [C] ([L67-L73](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/auth/agent_permission_handler.py#L67-L73), [doc](https://docs.litellm.ai/docs/proxy/access_management/agents)).
- **Fallo abierto.** Si la consulta de permisos falla (por ejemplo, base de datos inaccesible), devuelve acceso sin restricción, salvo en modo estricto, que responde 503 [C] ([L170-L192](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/auth/agent_permission_handler.py#L170-L192), [doc](https://docs.litellm.ai/docs/proxy/security_owasp_llm_top10)).
- Solo dos niveles (clave y equipo). Usuario final, usuario interno y organización «no se resuelven hoy» para A2A [C] ([doc](https://docs.litellm.ai/docs/auth_overview#4-authorization-rbac-and-access-groups)). Las claves personales heredan las concesiones de los equipos del propietario ([PR #46293](https://github.com/BerriAI/litellm/pull/46293)) [C] ([L396-L440](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/auth/agent_permission_handler.py#L396-L440)).
- Etiquetar agentes de base de datos con `agent_access_groups` solo es posible desde la interfaz; la API no expone el campo [C] ([doc](https://docs.litellm.ai/docs/a2a_agent_permissions#agent-access-groups); [issue #42766](https://github.com/BerriAI/litellm/issues/42766), abierto).
- Denegación: HTTP 403; en las rutas JSON-RPC, envuelto como error `-32000` (ver 2.16). Las denegaciones se registran desde v1.109.0 ([PR #49492](https://github.com/BerriAI/litellm/pull/49492)) [C] ([`a2a_endpoints.py` L881-L899](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L881-L899)).
- `check_feature_access_for_user(..., "agents")` permite bloquear las rutas `/v1/agents` a usuarios internos mediante ajustes generales [C] ([`rbac_utils.py` L17-L45](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/common_utils/rbac_utils.py#L17-L45)).
- Permisos por skill o por método A2A (por ejemplo, permitir `message/send` pero no `tasks/cancel`): [NE].

**El agente como principal.**

- Una clave creada con `agent_id` identifica al agente cuando llama al gateway (modelos, MCP, otros agentes) [C] ([doc](https://docs.litellm.ai/docs/proxy/access_management/agents#act-for-the-caller)).
- Si el agente devuelve las cabeceras `X-LiteLLM-User-Id` y `X-LiteLLM-Team-Id` en sus propias llamadas, el gateway recorta sus permisos a lo que el llamante original puede alcanzar. Solo estrechan, nunca amplían [C] ([`agent_caller.py` L1-L36](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/auth/agent_caller.py#L1-L36)). Desde v1.104.0 ([PR #42478](https://github.com/BerriAI/litellm/pull/42478)).
- Los grupos de acceso adjuntos a un agente actúan de techo para modelos, servidores MCP y otros agentes. Desde v1.104.0 ([PR #41634](https://github.com/BerriAI/litellm/pull/41634)) [C] ([`agent_access_groups.py` L74-L110](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/auth/agent_access_groups.py#L74-L110)).
- Identidades Entra ID por agente (`identity`), con modos `autonomous`, `delegated` y `both`. Para estos agentes la política es estricta: solo concesiones explícitas y releídas de la base de datos. Desde v1.105.0 ([PR #43721](https://github.com/BerriAI/litellm/pull/43721), [PR #43722](https://github.com/BerriAI/litellm/pull/43722)) [C] ([`agent_identity.py` L9-L48](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/types/proxy/agent_identity.py#L9-L48), [`agent_permission_handler.py` L234-L271](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/auth/agent_permission_handler.py#L234-L271)). No hay página de documentación dedicada en la barra lateral de «Agent Gateway» [NE].

### 2.6 Credenciales e identidad hacia el agente

No existe un campo `auth_type` en el agente: el modo se deduce del contenido de `litellm_params` [C] ([doc](https://docs.litellm.ai/docs/auth_overview#a2a-auth-mode-inferred-from-litellm_params)).

| Mecanismo | Configuración | Fuente |
|---|---|---|
| Cabeceras estáticas | `static_headers: {Nombre: valor}` en el agente. Se envían siempre y ganan en caso de conflicto | [C] [doc](https://docs.litellm.ai/docs/a2a_agent_headers#method-1-static-headers) |
| Reenvío de cabeceras del cliente | `extra_headers: [nombre, …]`: lista de nombres permitidos; el valor lo pone el cliente | [C] [doc](https://docs.litellm.ai/docs/a2a_agent_headers#method-2-forward-client-headers) |
| Convención sin configuración | El cliente envía `x-a2a-{nombre_o_id}-{cabecera}` y se reenvía como `{cabecera}` solo a ese agente | [C] [`a2a_endpoints.py` L1000-L1008](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1000-L1008) |
| Clave del agente | `litellm_params.api_key` se envía como `Authorization: Bearer`. Desde v1.109.0 ([PR #49474](https://github.com/BerriAI/litellm/pull/49474)); antes se guardaba y no se enviaba | [C] [`common_utils.py` L169-L180](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/llms/a2a/common_utils.py#L169-L180) |
| Microsoft Entra | Campos Entra en `litellm_params`; el gateway obtiene el token. Desde v1.103.0 | [C] [doc](https://docs.litellm.ai/docs/providers/azure_ai_agents#foundry-agents-over-a2a) |
| OAuth M2M de Databricks | Bloque `databricks_oauth`. Desde v1.89.0 ([PR #29586](https://github.com/BerriAI/litellm/pull/29586)) | [C] [doc](https://docs.litellm.ai/docs/a2a_databricks_agent#authentication) |
| SigV4 | Solo Bedrock AgentCore, cuando no hay `api_key` | [C] [doc](https://docs.litellm.ai/docs/providers/bedrock_agentcore#a2a-gateway-authentication) |

- Precedencia: dinámicas (reenvío y convención), después estáticas, y por encima la credencial que obtiene el propio gateway (Entra, Databricks, `api_key`). La comparación de nombres no distingue mayúsculas [C] ([`interactions/agents/utils.py` L16-L38](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/interactions/agents/utils.py#L16-L38), [`a2a_endpoints.py` L237-L253](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L237-L253)).
- La convención `x-a2a-…` no tiene lista de permitidos: el cliente puede fijar `Authorization` o `Cookie` hacia el agente si no hay una cabecera estática del mismo nombre [C] ([L1000-L1008](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1000-L1008); [issue #49427](https://github.com/BerriAI/litellm/issues/49427), abierto).
- Las cabeceras cuyo nombre empieza por `x-litellm-` nunca se reenvían por estas vías, aunque estén en `extra_headers` [C] ([L245-L249](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L245-L249); [issue #43450](https://github.com/BerriAI/litellm/issues/43450), abierto).

**Identidad propagada** [C] ([`a2a_endpoints.py` L203-L253](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L203-L253), [`main.py` L536-L548](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/a2a_protocol/main.py#L536-L548)):

| Cabecera | Contenido |
|---|---|
| `X-LiteLLM-User-Id` | `user_id` de la clave, si lo tiene. Si llama una clave de agente en nombre de alguien, el usuario original |
| `X-LiteLLM-Team-Id` | `team_id` de la clave, si lo tiene |
| `X-LiteLLM-Trace-Id` | Identificador de traza o sesión |
| `X-LiteLLM-Agent-Id` | Identificador del agente invocado (solo en `message/send` y `message/stream`) |

- Las cabeceras de usuario y equipo se envían en `message/send` y `message/stream` desde el [PR #40305](https://github.com/BerriAI/litellm/pull/40305) y en los métodos de tareas desde el [PR #29566](https://github.com/BerriAI/litellm/pull/29566). Versión de publicación del primero: [NC] (el [issue #43450](https://github.com/BerriAI/litellm/issues/43450) lo sitúa en v1.102.0).
- La clave virtual del cliente y el identificador de usuario final (`x-litellm-end-user-id`) no se reenvían automáticamente [C] ([doc](https://docs.litellm.ai/docs/a2a#sub-agent-identity-propagation)).
- No hay firma de la petición hacia el agente: nada equivalente al guardrail `mcp_jwt_signer` de MCP. La documentación dice «no aplicable a A2A hoy» [C] ([doc](https://docs.litellm.ai/docs/auth_overview#6-guardrails-on-the-gateway-path)). El agente debe confiar en la red para creer las cabeceras de identidad.
- Intercambio de token en nombre del usuario (OBO) por petición: [NE] en `main`; hay un PR en borrador ([#53480](https://github.com/BerriAI/litellm/pull/53480)).
- mTLS del gateway hacia el agente configurable por agente: [NE].

### 2.7 Propiedad y aislamiento de tareas

- El gateway no guarda tareas ni qué cliente creó cada una. No hay almacén ni comprobación de propietario en el manejador [C] por ausencia ([`a2a_endpoints.py` L1116-L1185](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1116-L1185)).
- `tasks/get`, `tasks/cancel`, `tasks/list`, `tasks/resubscribe` y los métodos de push se reenvían al agente. El único control del gateway es el permiso sobre el agente: cualquier clave autorizada a invocarlo puede pedir cualquier identificador de tarea [C] ([L881-L899](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L881-L899), [L1151-L1157](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1151-L1157)).
- El aislamiento se delega en el agente, al que se le envían `X-LiteLLM-User-Id` y `X-LiteLLM-Team-Id` «para que pueda acotar el acceso a las tareas al llamante autenticado» [C] (mensaje del commit [`48c9fab`](https://github.com/BerriAI/litellm/commit/48c9fabb26d948ab3f4f5e4906bec9af8daa6d11), [PR #29566](https://github.com/BerriAI/litellm/pull/29566)). Si la clave no tiene `user_id` ni `team_id`, no viaja ninguna identidad [C] ([L203-L218](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L203-L218)).
- `tasks/list` no se filtra por llamante en el gateway [C] ([L1116-L1157](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1116-L1157)).
- `contextId`: para agentes A2A puros llega al agente tal como lo envió el cliente; dos claves que usen el mismo `contextId` comparten conversación si el agente indexa por él [C] ([issue #49431](https://github.com/BerriAI/litellm/issues/49431), abierto).
- Excepciones: en Bedrock AgentCore y LangFlow el `contextId` se prefija con un hash de la clave que llama, de modo que dos claves nunca comparten sesión [C] ([`a2a_protocol/utils.py` L183-L196](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/a2a_protocol/utils.py#L183-L196), [doc](https://docs.litellm.ai/docs/providers/bedrock_agentcore#session-isolation)). AgentCore desde v1.101.0 ([PR #39371](https://github.com/BerriAI/litellm/pull/39371)).

### 2.8 Notificaciones push

- Los cuatro métodos `tasks/pushNotificationConfig/*` se reenvían al agente [C] ([`a2a_endpoints.py` L1116-L1157](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1116-L1157)).
- En `set`, antes de reenviar, valida la URL de `params.url` y de `params.pushNotificationConfig.url` [C] ([L1130-L1150](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1130-L1150), [L190-L200](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L190-L200)):
  - Exige esquema `https`; si no, HTTP 400 «Push notification URL must use HTTPS».
  - Aplica `validate_url`: resuelve DNS y rechaza toda dirección que no sea globalmente enrutable (privadas, loopback, link-local, metadatos de nube). Admite excepciones por host con `user_url_allowed_hosts` [C] ([`url_utils.py` L193-L209](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/litellm_core_utils/url_utils.py#L193-L209), [L344-L400](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/litellm_core_utils/url_utils.py#L344-L400)).
- La validación ocurre una sola vez, en el registro de la configuración. La entrega la hace el agente directamente al webhook del cliente, sin pasar por el gateway: no hay comprobación de clave, registro ni atribución de gasto en ese tramo [C] ([issue #49623](https://github.com/BerriAI/litellm/issues/49623), abierto).
- Una configuración de push incluida dentro de `message/send` o `message/stream` (`configuration.pushNotificationConfig`) no se valida: se reenvía sin comprobar [C] ([L1036-L1057](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1036-L1057); [issue #49622](https://github.com/BerriAI/litellm/issues/49622), abierto; [PR #49637](https://github.com/BerriAI/litellm/pull/49637), sin fusionar).
- El gateway no tiene receptor de webhooks propio. El código lo dice: «proxy has no webhook plumbing yet» [C] ([`agent_card.py` L92](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/agent_card.py#L85-L96)).
- La card servida elimina `capabilities.pushNotifications`, aunque los métodos sí se reenvían [C] ([`agent_card.py` L96](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/agent_card.py#L85-L96), [doc](https://docs.litellm.ai/docs/a2a_agent_card#agentcapabilities-443)).
- Filtrado o reescritura de credenciales del webhook (`authentication`, `token`): [NE]; se reenvían sin tocar.
- Validación de la firma o del origen de las notificaciones: [NE], porque no pasan por el gateway.

### 2.9 Streaming (SSE)

- `message/stream` responde `text/event-stream`, con cada objeto JSON-RPC como `data: <json>\n\n`. Antes de v1.99.0 era NDJSON ([PR #35037](https://github.com/BerriAI/litellm/pull/35037)) [C] ([`a2a_endpoints.py` L135-L137](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L135-L137), [L556-L709](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L556-L709)). La página general de la documentación aún dice «NDJSON/SSE» ([doc](https://docs.litellm.ai/docs/a2a#supported-json-rpc-methods)).
- No es un reenvío byte a byte. El gateway consume el stream del agente con el cliente SDK, convierte cada evento a la versión servida y lo vuelve a serializar, pasando por los hooks de streaming (guardrails, coste) [C] ([L620-L680](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L620-L680), [`main.py` L376-L390](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/a2a_protocol/main.py#L376-L390)).
- **Keepalive.** Desactivado por defecto. Con `litellm_settings.sse_keepalive_ping_interval_seconds` envía comentarios SSE de ping y añade `Cache-Control: no-cache` y `X-Accel-Buffering: no` [C] ([L476-L489](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L476-L489), [doc](https://docs.litellm.ai/docs/proxy/timeout#keepalive-pings-for-idle-streaming-connections)). Desde v1.99.0 ([PR #37368](https://github.com/BerriAI/litellm/pull/37368)).
- **Tiempos de espera** [C]:

  | Tramo | Valor | Fuente |
  |---|---|---|
  | `message/send` y `message/stream` (cliente SDK) | `DEFAULT_A2A_AGENT_TIMEOUT`, 6000 s por defecto, variable de entorno | [`constants.py` L706](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/constants.py#L706) |
  | Métodos de tareas reenviados | 60 s fijos | [`a2a_endpoints.py` L397-L400](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L397-L400) |
  | `tasks/resubscribe` | Sin tiempo de espera | [L433-L436](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L433-L436) |
  | Descubrimiento de card en el alta | 10 s | [`discovery.py` L50](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/discovery.py#L50) |

  Tiempo de espera configurable por agente para `/a2a`: [NE].
- **`tasks/resubscribe`.** Se reenvía al agente con `Accept: text/event-stream`; los eventos se normalizan y pasan por el hook de guardrails de streaming [C] ([L1187-L1208](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1187-L1208), [L412-L473](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L412-L473)).
- **Reconexión.** El gateway no guarda eventos, no repone los perdidos y no maneja `Last-Event-ID` [NE]. La reanudación depende por completo del agente.
- **Errores a mitad de stream.** Se emiten como un evento SSE con error JSON-RPC `-32603`; el estado HTTP ya es 200 [C] ([L681-L707](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L681-L707)). Un error HTTP del agente al abrir `tasks/resubscribe` se convierte en un único evento de error [C] ([L441-L455](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L441-L455)).
- Si el cliente corta, se factura lo recibido hasta ese momento [C] ([`main.py` L828-L833](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/a2a_protocol/main.py#L828-L833)).
- Por `/chat/completions`, si la card del agente declara que no hay streaming, una petición con `stream: true` se ejecuta como `message/send` bloqueante y se devuelve en un único fragmento [C] ([doc](https://docs.litellm.ai/docs/a2a_agent_card#agentcapabilities-443), [`transformation.py` L124-L129](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/llms/a2a/chat/transformation.py#L124-L129)).

### 2.10 Límites

| Límite | Configuración | Alcance | Fuente |
|---|---|---|---|
| TPM/RPM por agente | `tpm_limit`, `rpm_limit` en el agente | Todo el tráfico del agente | [C] [doc](https://docs.litellm.ai/docs/proxy/users#agents) |
| TPM/RPM por sesión | `session_tpm_limit`, `session_rpm_limit` en el agente | Por sesión (id de traza) | [C] [doc](https://docs.litellm.ai/docs/proxy/users#agents) |
| Tope de iteraciones | `litellm_params.max_iterations` | Llamadas hechas por el agente con su clave, por sesión. HTTP 429 | [C] [`max_iterations_limiter.py` L1-L11](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/hooks/max_iterations_limiter.py#L1-L11) |
| Presupuesto por sesión | `litellm_params.max_budget_per_session` | Igual, en dólares. HTTP 429 | [C] [`max_budget_per_session_limiter.py` L1-L15](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/hooks/max_budget_per_session_limiter.py#L1-L15) |
| Traza obligatoria | `require_trace_id_on_calls_to_agent`, `require_trace_id_on_calls_by_agent`; ajuste de equipo `require_trace_id` (v1.108.0) | HTTP 400 si falta `x-litellm-trace-id` o `x-litellm-session-id` | [C] [`a2a_endpoints.py` L362-L376](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L362-L376), [doc](https://docs.litellm.ai/docs/a2a_iteration_budgets) |
| Clave, equipo, usuario | Presupuestos y límites habituales de LiteLLM | Se aplican en la lógica previa común a todas las rutas | [C] [`a2a_endpoints.py` L971-L982](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L971-L982) |

- Límites por agente y por sesión añadidos en el [PR #22849](https://github.com/BerriAI/litellm/pull/22849) (marzo de 2026); versión de publicación: [NC].
- El limitador de tasa toma como agente la clave del agente (`agent_id`), el agente invocado (`invoked_agent_id`) y `metadata.agent_id` [C] ([`parallel_request_limiter_v3_native.py` L677-L686](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/hooks/parallel_request_limiter_v3_native.py#L677-L686)). La aplicación efectiva está en código nativo que no examiné: [NC].
- Los contadores de iteraciones y de presupuesto por sesión expiran en una hora por defecto (`LITELLM_MAX_ITERATIONS_TTL`, `LITELLM_MAX_BUDGET_PER_SESSION_TTL`) y funcionan entre réplicas con Redis [C] ([doc](https://docs.litellm.ai/docs/a2a_iteration_budgets#environment-variables)).
- Esos contadores se indexan solo por id de sesión: dos agentes en la misma traza comparten contador [C] ([issue #43190](https://github.com/BerriAI/litellm/issues/43190), abierto).
- La documentación se contradice sobre dónde se guardan `max_iterations` y `max_budget_per_session` (metadatos de la clave frente a `litellm_params` del agente) ([doc](https://docs.litellm.ai/docs/a2a_iteration_budgets#configuring-via-ui)). El código lee primero `litellm_params` del agente y, para `max_iterations`, cae a los metadatos de la clave [C] ([`max_iterations_limiter.py` L158-L179](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/hooks/max_iterations_limiter.py#L158-L179)).
- **Tamaño de cuerpo.** No hay un límite específico de A2A [NE]. Existe `general_settings.max_request_size_mb`, global y solo activo con licencia Enterprise [C] ([`proxy_server.py` L3164-L3182](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/proxy_server.py#L3164-L3182)).
- Límite de profundidad de anidamiento, de número de partes o de tamaño de ficheros en mensajes A2A: [NE].
- Límite de conexiones SSE simultáneas por agente o por clave: [NE].

### 2.11 Costes y uso por agente

- Precio configurable en `litellm_params` del agente: `cost_per_query` (fijo), o `input_cost_per_token` y `output_cost_per_token`. Prioridad: `response_cost` explícito, después `cost_per_query`, después por token, y si no hay nada, 0 [C] ([`cost_calculator.py` L30-L83](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/a2a_protocol/cost_calculator.py#L30-L83), [doc](https://docs.litellm.ai/docs/a2a_cost_tracking)). Desde v1.80.10 ([PR #17780](https://github.com/BerriAI/litellm/pull/17780)).
- Los tokens los cuenta el gateway con su tokenizador sobre las partes de texto de la petición y de la respuesta; no usa el uso que informe el agente [C] ([`main.py` L201-L215](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/a2a_protocol/main.py#L201-L215), [`streaming_iterator.py` L140-L170](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/a2a_protocol/streaming_iterator.py#L140-L170)).
- En streaming, una respuesta con error se factura a 0 [C] ([`streaming_iterator.py` L169](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/a2a_protocol/streaming_iterator.py#L169)).
- El gasto se atribuye a la clave, el equipo y el usuario que llaman, y al agente (`agent_id` en SpendLogs). Hay tabla diaria por agente y `GET /agent/daily/activity`; la interfaz tiene la pestaña «Agent Usage (A2A)» [C] ([`endpoints.py` L1385-L1455](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/endpoints.py#L1385-L1455), [doc](https://docs.litellm.ai/docs/a2a_cost_tracking#view-spend-in-usage-page)).
- Para que el gasto en modelos que hace el propio agente se impute al agente y a la misma traza, el agente debe devolver `X-LiteLLM-Trace-Id` y `X-LiteLLM-Agent-Id` en sus llamadas al gateway [C] ([doc](https://docs.litellm.ai/docs/a2a#forwarding-litellm-context-headers), [`litellm_pre_call_utils.py` L2250-L2259](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/litellm_pre_call_utils.py#L2250-L2259)).
- Los agentes servidos por puente (LangGraph, Databricks...) se facturan a su propio precio bajo `a2a_agent/<nombre>` desde v1.109.0 ([PR #49150](https://github.com/BerriAI/litellm/pull/49150)) [C].
- Los métodos de tareas reenviados no pasan por el cálculo de coste de `message/send` [C] ([`a2a_endpoints.py` L1157-L1185](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1157-L1185)). Si generan fila de gasto: [NC].
- La medición de peticiones facturables de la licencia y la telemetría de endpoints solo cuentan rutas A2A que terminan en `/message/send`; las llamadas a `POST /a2a/{agent_id}` no se cuentan [C] ([doc](https://docs.litellm.ai/docs/proxy/telemetry), [doc](https://docs.litellm.ai/docs/proxy/billing_metrics)).

### 2.12 Observabilidad

- Cada `message/send` y `message/stream` genera el registro estándar de LiteLLM con modelo `a2a_agent/<nombre>` y proveedor `a2a_agent`, visible en la pestaña Logs con usuario, clave, equipo, latencia y coste. El contenido de petición y respuesta requiere `store_prompts_in_spend_logs: true` [C] ([doc](https://docs.litellm.ai/docs/a2a#tracking-agent-logs), [`a2a_endpoints.py` L940-L953](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L940-L953)).
- Al usar el pipeline estándar, los callbacks de logging configurados (Langfuse, OpenTelemetry, etc.) reciben las llamadas A2A [C] para Langfuse ([doc](https://docs.litellm.ai/docs/a2a#result), [PR #45597](https://github.com/BerriAI/litellm/pull/45597)); para el resto de integraciones, una por una: [NC].
- **Identificador de sesión.** Se toma de `x-litellm-trace-id`, después `x-litellm-session-id`, después cabeceras `x-<proveedor>-session-id`. Se guarda en `session_id` de SpendLogs y se propaga a llamadas MCP y A2A anidadas [C] ([`litellm_pre_call_utils.py` L1209-L1236](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/litellm_pre_call_utils.py#L1209-L1236), [doc](https://docs.litellm.ai/docs/proxy/request_headers)).
- El `contextId` de A2A no se usa como identificador de traza en el código actual [NE] (existió un commit con ese propósito, [PR #22626](https://github.com/BerriAI/litellm/pull/22626); hoy `contextId` solo aparece en los puentes de proveedor).
- El registro distingue agente actor, agente destino y agente al que se factura [C] ([`litellm_pre_call_utils.py` L2472-L2483](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/litellm_pre_call_utils.py#L2472-L2483)).
- Las peticiones rechazadas antes de despachar (agente desconocido, sin permiso) se registran como fallo con llamante y agente destino, desde v1.109.0 [C] ([`a2a_endpoints.py` L256-L282](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L256-L282)).
- En streaming, el identificador de llamada y la traza enviada al agente son el `id` JSON-RPC del cliente; como SpendLogs usa ese valor como clave primaria, las llamadas que reutilizan un `id` se pierden [C] ([`main.py` L616-L636](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/a2a_protocol/main.py#L616-L636), [L776-L778](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/a2a_protocol/main.py#L776-L778); [issue #49424](https://github.com/BerriAI/litellm/issues/49424), abierto).
- La página de actividad por endpoint clasifica el tráfico en `llm`, `mcp` y `a2a` [C] ([doc](https://docs.litellm.ai/docs/proxy/endpoint_activity)).
- Métricas Prometheus con etiqueta de agente: [NE] (sin referencias a agentes en `integrations/prometheus.py`).
- Registro de auditoría: el disparo del kill switch escribe una fila `kill_switch_fired`; el registro de auditoría requiere licencia Enterprise [C] ([doc](https://docs.litellm.ai/docs/a2a_kill_switch#audit-log)).

### 2.13 Guardrails y políticas de contenido

- El marco de guardrails de LiteLLM cubre las rutas A2A: un adaptador extrae el texto de las partes `kind: "text"` de la petición y de las respuestas (mensaje directo, mensaje anidado, artefactos de tarea, mensaje de estado, `artifact-update` en streaming), aplica el guardrail y reescribe el texto [C] ([README del adaptador](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/llms/a2a/chat/guardrail_translation/README.md), [doc](https://docs.litellm.ai/docs/proxy/security_owasp_llm_top10)).
- Solo se inspecciona texto. Partes de fichero o de datos: [NE] en el adaptador.
- Guardrails por agente: `litellm_params.guardrails` del agente se suma a los de la petición [C] ([`a2a_endpoints.py` L955-L964](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L955-L964), [L1023-L1033](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1023-L1033)). Soporte de guardrails en agentes desde el [PR #20619](https://github.com/BerriAI/litellm/pull/20619); en salida de streaming desde v1.81.12 ([PR #21206](https://github.com/BerriAI/litellm/pull/21206)).
- Guardrails por petición: los parámetros propios de LiteLLM presentes en `params` (por ejemplo `guardrails`) se extraen y se retiran antes de reenviar; `params.metadata` se respeta como campo de A2A [C] ([L850-L863](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L850-L863)). El README del adaptador documenta `params.metadata.guardrails`; cuál de las dos formas funciona hoy: [NC].
- En streaming se analizan los fragmentos; `tasks/resubscribe` también pasa por el hook de streaming, y las respuestas de los métodos de tareas por el hook posterior a la llamada [C] ([L529-L544](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L529-L544), [L1173-L1180](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1173-L1180)).
- Con `reject_unregistered_guardrails` activo, un agente que nombre un guardrail no registrado hace fallar la petición [C] ([L955-L964](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L955-L964), [PR #53214](https://github.com/BerriAI/litellm/pull/53214)).
- Guardrail en modo `during_call` para A2A: la documentación lo deja en blanco frente a MCP [C] ([doc](https://docs.litellm.ai/docs/auth_overview#6-guardrails-on-the-gateway-path)).
- Validación del mensaje contra el esquema de A2A: solo la que hace el SDK al parsear los parámetros de `message/send`; error `-32602` si no encajan [C] ([L156-L172](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L156-L172), [L1049-L1052](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1049-L1052)). Los parámetros de los métodos reenviados no se validan [C] ([L1128-L1157](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1128-L1157)).
- Políticas por tipo de contenido (lista de tipos MIME permitidos, bloqueo de ficheros): [NE].

### 2.14 Resiliencia

- **Salud.** Solo bajo demanda: `GET /v1/agents?health_check=true` hace un `GET` a la URL de cada agente; lo considera sano si responde con estado menor que 500, y elimina de la lista los que no. 5 s por agente y 30 s en total (`LITELLM_AGENT_HEALTH_CHECK_TIMEOUT`, `LITELLM_AGENT_HEALTH_CHECK_GATHER_TIMEOUT`) [C] ([`endpoints.py` L271-L302](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/endpoints.py#L271-L302), [L499-L521](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/endpoints.py#L499-L521), [`constants.py` L297-L298](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/constants.py#L297-L298)).
- Comprobaciones de salud periódicas en segundo plano, o exclusión automática de agentes caídos al invocar: [NE].
- **Reintentos.** El único reintento es el de la URL `localhost` de la card (un reintento) [C] ([`main.py` L335-L373](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/a2a_protocol/main.py#L335-L373)). Reintentos generales ante 5xx o errores de red en `/a2a`: [NE].
- **Balanceo y alternativas.** La documentación marca «Load Balancing» como soportado sin explicar cómo ([doc](https://docs.litellm.ai/docs/a2a)). En el camino `/a2a/{agent_id}` cada nombre resuelve a un único agente y no encontré balanceo, alternativas (fallback) ni cortacircuitos entre agentes [NE]. A qué se refiere la documentación: [NC].
- **Kill switch.** Webhook configurable por agente (`kill_switch`: URL, método, cabeceras, cuerpo, autenticación bearer, api_key o basic). `POST /v1/agents/{agent_id}/kill_switch`, solo administrador; 10 s de espera, sin seguir redirecciones; 200 si el webhook responde 2xx y 502 en otro caso. No cambia el estado del agente en LiteLLM [C] ([`kill_switch.py` L193-L230](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/kill_switch.py#L193-L230), [doc](https://docs.litellm.ai/docs/a2a_kill_switch)). Desde v1.104.0 ([PR #42841](https://github.com/BerriAI/litellm/pull/42841)).
- Varias réplicas: registro en memoria con lectura de la base de datos cuando falta una entrada [C] ([`registry_read_through.py` L334-L340](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/common_utils/registry_read_through.py#L334-L340)).

### 2.15 Puentes

**Agente A2A por `/chat/completions`.**

- Modelo `a2a/<nombre-del-agente>` en `/chat/completions`. El gateway busca el agente en el registro, comprueba el permiso (los administradores lo saltan) y envía JSON-RPC a la URL del agente [C] ([`a2a_routing.py` L124-L195](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_routing.py#L124-L195), [doc](https://docs.litellm.ai/docs/a2a_invoking_agents#chatcompletions-api-openai-sdk)). Desde v1.81.9 ([PR #20358](https://github.com/BerriAI/litellm/pull/20358), [PR #20362](https://github.com/BerriAI/litellm/pull/20362)).
- La conversión aplana todos los mensajes en un único texto con el formato `rol: contenido`, lo envía como un solo mensaje de usuario con `message/send` (bloqueante) o `message/stream`, y devuelve el texto extraído de la respuesta [C] ([`transformation.py` L229-L284](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/llms/a2a/chat/transformation.py#L229-L284), [`common_utils.py` L39-L68](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/llms/a2a/common_utils.py#L39-L68)). No envía `contextId` ni maneja tareas de larga duración [C] (mismo fichero).
- `temperature`, `max_tokens` y `top_p` se declaran soportados y se descartan ([issue #45944](https://github.com/BerriAI/litellm/issues/45944), abierto).
- La nota de v1.80.10 lista también `/messages` y `/responses` para agentes LangGraph y Azure AI Foundry [C] ([notas](https://github.com/BerriAI/litellm-docs/blob/090f0d98f65aa5444e058ddb32e3f2452d0207b3/release_notes/v1.80.10-stable/index.md)).

**Proveedores no A2A servidos como A2A («completion bridge»).**

- Si el agente tiene `litellm_params.custom_llm_provider`, `message/send` y `message/stream` se convierten a una llamada `litellm.acompletion` y la respuesta se devuelve como eventos A2A sintéticos: tarea `submitted`, estado `working`, `artifact-update` y estado `completed` [C] ([README del puente](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/a2a_protocol/litellm_completion_bridge/README.md), [`handler.py` L1-L11](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/a2a_protocol/litellm_completion_bridge/handler.py#L1-L11)).
- Proveedores documentados: Vertex AI Agent Engine, LangGraph, Azure AI Foundry, Bedrock AgentCore, Pydantic AI ([doc](https://docs.litellm.ai/docs/a2a)); Databricks Agent Platform ([doc](https://docs.litellm.ai/docs/a2a_databricks_agent), v1.108.0); watsonx Orchestrate y LangFlow ([notas v1.89.0](https://github.com/BerriAI/litellm-docs/blob/090f0d98f65aa5444e058ddb32e3f2452d0207b3/release_notes/v1.89.0/index.md)) [C]. Configuraciones A2A específicas en el código: `pydantic_ai_agents`, Bedrock AgentCore, `langflow`, `watsonx_orchestrate` [C] ([`config_manager.py` L19-L61](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/a2a_protocol/providers/config_manager.py#L19-L61)).
- Estos agentes no tienen tareas consultables: `tasks/*` exige una URL A2A real [C] ([doc](https://docs.litellm.ai/docs/a2a_agent_card#requirements)).

**Relación con MCP.**

- Mismo proxy, mismas claves y mismo modelo de `object_permission`; MCP resuelve permisos en seis niveles y A2A en dos [C] ([doc](https://docs.litellm.ai/docs/auth_overview#4-authorization-rbac-and-access-groups)).
- Herramienta MCP virtual `agent_search(query, top_k)` para buscar agentes desde un cliente MCP (requiere `mcp_tool_search_enabled` en la clave) [C] ([doc](https://docs.litellm.ai/docs/mcp_tool_search)).
- Un agente con clave propia puede tener permisos MCP propios (`object_permission.mcp_servers` en el agente) [C] ([`types/agents.py` L178-L184](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/types/agents.py#L178-L184)).
- Exponer un agente A2A como herramienta MCP, o un servidor MCP como agente A2A: [NE]. Hay un PR abierto para que los agentes por puente usen herramientas MCP y deleguen en otros agentes ([#30823](https://github.com/BerriAI/litellm/pull/30823)).

**SDK de Python.** `litellm.a2a_protocol.asend_message`, `asend_message_streaming`, `create_a2a_client` y `aget_agent_card` se pueden usar sin el proxy [C] ([`main.py` L441-L963](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/a2a_protocol/main.py#L441-L963)).

### 2.16 Errores

Errores generados por el gateway en `POST /a2a/{agent_id}` [C] ([`a2a_endpoints.py`](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L285-L334)):

| Situación | Código JSON-RPC | HTTP | Fuente |
|---|---|---|---|
| `jsonrpc` distinto de `"2.0"` | `-32600` | 400 | [L839-L840](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L839-L840) |
| Método desconocido | `-32601` | 400 | [L1210-L1211](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1210-L1211) |
| Parámetros de `message/send` inválidos; `BadRequestError` | `-32602` | 400 | [L1049-L1052](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1049-L1052), [L1225-L1226](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1225-L1226) |
| Agente no encontrado | `-32000` | 404 | [L866-L877](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L866-L877) |
| Clave inválida, sin permiso sobre el agente, presupuesto o límite de tasa, traza obligatoria ausente, URL de push rechazada | `-32000`, con `data: {type, param, code}` | El del rechazo (401, 403, 429, 400…) | [L315-L350](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L315-L350) |
| Agente sin URL configurada | `-32000` | 500 | [L933-L934](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L933-L934) |
| Credencial Entra rechazada por el agente | `-32000` | 401 | [L1227-L1228](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1227-L1228) |
| `a2a-sdk` no instalado; cualquier otra excepción | `-32603` | 500 | [L1040-L1046](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1040-L1046), [L1229](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1229) |
| Error durante el streaming | `-32603` dentro de un evento SSE | 200 | [L681-L707](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L681-L707) |
| Error JSON-RPC devuelto por el agente en un método reenviado | Se retransmite el del agente | 200 | [L401-L409](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L401-L409) |

- Los rechazos del proxy se envuelven en JSON-RPC `-32000` desde v1.109.0 ([PR #49476](https://github.com/BerriAI/litellm/pull/49476)); antes llegaban como error de estilo OpenAI ([issue #49426](https://github.com/BerriAI/litellm/issues/49426)).
- Ese envoltorio solo cubre las tres rutas registradas en el router JSON-RPC (`/a2a/{id}`, `/a2a/{id}/message/send`, `/v1/a2a/{id}/message/send`). Los dos alias `/message/stream` están en el router normal, así que sus rechazos no se envuelven [C] ([L777-L805](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L777-L805), [L337-L350](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L337-L350)).
- Los errores JSON-RPC salen con estado HTTP 4xx/5xx, no 200 [C] ([L285-L299](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L285-L299); [issue #49869](https://github.com/BerriAI/litellm/issues/49869), abierto).
- Cuerpo vacío o no parseable: no hay `-32700`. Un cuerpo vacío da `-32603` con HTTP 500; JSON mal formado da un 400 de estilo OpenAI [C] ([L830-L834](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L830-L834); [issue #49569](https://github.com/BerriAI/litellm/issues/49569), abierto; [PR #53029](https://github.com/BerriAI/litellm/pull/53029), sin fusionar).
- Códigos específicos de A2A (`-32001` TaskNotFound a `-32009` VersionNotSupported, según la [especificación](https://a2a-protocol.org/latest/specification/)): el gateway no genera ninguno [C] por ausencia en el código; solo pueden llegar retransmitidos desde el agente.
- Un `id` nulo en la petición desaparece de la respuesta, porque se serializa excluyendo valores nulos [C] ([L1181-L1185](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1181-L1185); [issue #49869](https://github.com/BerriAI/litellm/issues/49869)).
- Fuera de JSON-RPC: 404 y 403 en las rutas de la card; 400 por `protocolVersion` no soportado, 409 por nombre duplicado y 403 por no ser administrador en `/v1/agents`; 502 si falla el kill switch [C] ([L742-L760](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L742-L760), [`endpoints.py` L104-L114](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/endpoints.py#L104-L114), [L1156-L1157](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/endpoints.py#L1156-L1157)).
- El ejemplo de 403 de la documentación de permisos (`"Access denied to agent"`) no coincide con el mensaje del código actual (`"Agent '…' is not allowed for your key/team…"`) ([doc](https://docs.litellm.ai/docs/a2a_agent_permissions#3-test-access), [L886-L889](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L886-L889)).

---

## 3. Limitaciones y fallos conocidos

La columna «En el código examinado» indica si comprobé que el comportamiento descrito sigue presente en el commit `55d4288`.

### 3.1 Abiertos

| Issue | Resumen | En el código examinado |
|---|---|---|
| [#49867](https://github.com/BerriAI/litellm/issues/49867), [#44531](https://github.com/BerriAI/litellm/issues/44531) | Los métodos de tareas se reenvían con nombres 0.3 y sin `A2A-Version`; un agente solo 1.0 los rechaza. PR sin fusionar: [#50388](https://github.com/BerriAI/litellm/pull/50388), [#44769](https://github.com/BerriAI/litellm/pull/44769) | Sigue presente ([L1151-L1157](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1151-L1157)) |
| [#49622](https://github.com/BerriAI/litellm/issues/49622) | La URL de push no se valida cuando va dentro de `message/send`. PR sin fusionar: [#49637](https://github.com/BerriAI/litellm/pull/49637) | Sigue presente |
| [#49623](https://github.com/BerriAI/litellm/issues/49623) | Petición: receptor de webhooks propio del gateway para notificaciones push | No existe ([`agent_card.py` L92](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/agent_card.py#L85-L96)) |
| [#49431](https://github.com/BerriAI/litellm/issues/49431) | Petición: acotar `contextId` a la clave que llama en todos los agentes A2A | No implementado para A2A puro |
| [#49427](https://github.com/BerriAI/litellm/issues/49427) | La convención `x-a2a-{agente}-{cabecera}` deja al cliente fijar `Authorization` y `Cookie`. PR sin fusionar: [#49564](https://github.com/BerriAI/litellm/pull/49564) | Sigue presente ([L1000-L1008](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L1000-L1008)) |
| [#49430](https://github.com/BerriAI/litellm/issues/49430) | `GET /v1/agents` devuelve `static_headers` en claro a administradores | Sigue presente ([`endpoints.py` L232-L246](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/endpoints.py#L232-L246)) |
| [#49424](https://github.com/BerriAI/litellm/issues/49424) | Las llamadas se registran con el `id` JSON-RPC del cliente y SpendLogs descarta los repetidos. PR sin fusionar: [#49477](https://github.com/BerriAI/litellm/pull/49477) | Presente en streaming ([`main.py` L616-L636](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/a2a_protocol/main.py#L616-L636)); camino sin streaming: [NC] |
| [#49869](https://github.com/BerriAI/litellm/issues/49869) | Errores JSON-RPC con HTTP 400 y `id` nulo eliminado. PR sin fusionar: [#50196](https://github.com/BerriAI/litellm/pull/50196) | Sigue presente |
| [#49569](https://github.com/BerriAI/litellm/issues/49569) | Cuerpo no parseable: sin `-32700`. PR sin fusionar: [#53029](https://github.com/BerriAI/litellm/pull/53029) | Sigue presente |
| [#40586](https://github.com/BerriAI/litellm/issues/40586), [#49868](https://github.com/BerriAI/litellm/issues/49868) | `message/send` vuelve a descargar la card del agente en vez de usar la registrada, y la busca bajo la URL del agente, no en el origen. PR sin fusionar: [#50193](https://github.com/BerriAI/litellm/pull/50193), [#50197](https://github.com/BerriAI/litellm/pull/50197) | Sigue presente; paliado con `litellm_params.agent_card_path` |
| [#49570](https://github.com/BerriAI/litellm/issues/49570) | `a2a-sdk` fijado a 1.1.0, que rechaza respuestas con campos desconocidos. PR sin fusionar: [#49614](https://github.com/BerriAI/litellm/pull/49614) | Fijado a 1.1.0 en los grupos de desarrollo ([`pyproject.toml` L285](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/pyproject.toml#L285)) |
| [#38996](https://github.com/BerriAI/litellm/issues/38996), [#50679](https://github.com/BerriAI/litellm/issues/50679) | Por `/chat/completions` con `a2a/<agente>`: permisos de agente esquivados en un caso, y denegación indebida por permisos de modelo en otro. PR sin fusionar: [#51174](https://github.com/BerriAI/litellm/pull/51174) | [NC] |
| [#37581](https://github.com/BerriAI/litellm/issues/37581) | `a2a/<nombre>` por `/chat/completions` da «no healthy deployments» si hay grupos de modelos comodín. PR sin fusionar: [#47578](https://github.com/BerriAI/litellm/pull/47578) | [NC] |
| [#50677](https://github.com/BerriAI/litellm/issues/50677) | El puente de chat toma mensajes de estado no terminales como respuesta final. PR sin fusionar: [#50681](https://github.com/BerriAI/litellm/pull/50681), [#50699](https://github.com/BerriAI/litellm/pull/50699) | [NC] |
| [#45944](https://github.com/BerriAI/litellm/issues/45944) | `temperature`, `max_tokens` y `top_p` declarados y descartados | [NC] |
| [#43450](https://github.com/BerriAI/litellm/issues/43450) | `x-litellm-api-key` no se reenvía aunque esté en `extra_headers` | Sigue presente ([L245-L249](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/agent_endpoints/a2a_endpoints.py#L245-L249)) |
| [#43190](https://github.com/BerriAI/litellm/issues/43190) | `max_iterations` y `max_budget_per_session` compartidos por todos los agentes de una misma traza | Clave de contador solo por sesión ([`max_iterations_limiter.py`](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/hooks/max_iterations_limiter.py#L181-L188)) |
| [#42766](https://github.com/BerriAI/litellm/issues/42766) | `agent_access_groups` no se puede fijar por API en agentes de base de datos | Coincide con la documentación |
| [#40601](https://github.com/BerriAI/litellm/issues/40601) | Petición: que una lista de agentes vacía no signifique «todos» | Paliado con `agent_access_default_deny` (v1.108.0) |
| [#49451](https://github.com/BerriAI/litellm/issues/49451) | Petición: enlazar `contextId` con sesiones de Vertex AI Agent Engine. PR sin fusionar: [#49582](https://github.com/BerriAI/litellm/pull/49582), [#49490](https://github.com/BerriAI/litellm/pull/49490) | [NC] |
| [#45430](https://github.com/BerriAI/litellm/issues/45430) | Petición: descubrir y registrar automáticamente agentes de catálogos de nube | No existe |

Otros PR abiertos sin issue enlazado en esta lista: [#53100](https://github.com/BerriAI/litellm/pull/53100) (deja de enviar `stream` en el cuerpo JSON-RPC de `message/stream`), [#53480](https://github.com/BerriAI/litellm/pull/53480) (intercambio de token OBO por petición, borrador), [#49915](https://github.com/BerriAI/litellm/pull/49915) (enviar credenciales del agente al descubrir la card, borrador).

### 3.2 Cerrados relevantes

| Issue | Resumen | Cierre |
|---|---|---|
| [#25421](https://github.com/BerriAI/litellm/issues/25421) | Cualquier clave válida podía invocar agentes sin respetar grupos de acceso | Cerrado como «not planned» (julio de 2026). Si quedó corregido por los cambios de permisos de v1.104 a v1.108: [NC] |
| [#26411](https://github.com/BerriAI/litellm/issues/26411) | LiteLLM no manejaba `tasks/get` (tareas de larga duración) | Cerrado como «not planned»; los métodos de tareas llegaron con el [PR #29566](https://github.com/BerriAI/litellm/pull/29566) |
| [#22672](https://github.com/BerriAI/litellm/issues/22672) | Petición: autenticación hacia el agente y paso directo de la card | Cerrado como «not planned»; la autenticación hacia el agente existe hoy, el paso directo de la card no |
| [#30795](https://github.com/BerriAI/litellm/issues/30795) | La card anunciaba 1.0 pero el endpoint hablaba 0.3 | Corregido en v1.92.0 ([PR #30950](https://github.com/BerriAI/litellm/pull/30950)) |
| [#49425](https://github.com/BerriAI/litellm/issues/49425), [#49426](https://github.com/BerriAI/litellm/issues/49426), [#49428](https://github.com/BerriAI/litellm/issues/49428), [#49429](https://github.com/BerriAI/litellm/issues/49429), [#49433](https://github.com/BerriAI/litellm/issues/49433) | 403 sin registrar; rechazo de presupuesto fuera de JSON-RPC; `api_key` no enviada; URL del backend visible a no administradores; alias de streaming con 404 | Corregidos en v1.109.0 ([notas](https://github.com/BerriAI/litellm-docs/blob/090f0d98f65aa5444e058ddb32e3f2452d0207b3/release_notes/v1.109.0/index.md)) |
| [#51187](https://github.com/BerriAI/litellm/issues/51187) | Duplicado de #49867 | Cerrado como duplicado |
| [#32609](https://github.com/BerriAI/litellm/issues/32609), [#32608](https://github.com/BerriAI/litellm/issues/32608) | El puente de chat usaba solo nombres 0.3 y no aplicaba `static_headers` | Cerrados; el código actual sigue enviando `message/send` y `message/stream` en ese puente ([`transformation.py` L268-L272](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/llms/a2a/chat/transformation.py#L268-L272)). Estado real: [NC] |

### 3.3 Limitaciones que constan en la documentación o en el código

- Acceso a agentes abierto mientras ningún nivel defina una lista, y sin restricción si falla la consulta de permisos ([doc](https://docs.litellm.ai/docs/proxy/security_owasp_llm_top10)).
- Permisos A2A en dos niveles; sin usuario final, usuario interno ni organización ([doc](https://docs.litellm.ai/docs/auth_overview#4-authorization-rbac-and-access-groups)).
- Sin firma de identidad hacia el agente ([doc](https://docs.litellm.ai/docs/auth_overview#6-guardrails-on-the-gateway-path)).
- La card no publica `pushNotifications`, `extensions`, `extendedAgentCard`, `signatures`, `tenant` ni `securityRequirements` por skill ([doc](https://docs.litellm.ai/docs/a2a_agent_card#agent-card-support)).
- `capabilities.streaming` solo se publica si el agente lo anunciaba; un `TODO` en el código indica que el streaming no estaba cubierto de extremo a extremo por pruebas cuando se escribió ([`agent_card.py` L88-L91](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/agent_card.py#L85-L96)).
- Las extensiones de protocolo «no se validan ni se reenvían todavía» ([`agent_card.py` L95](https://github.com/BerriAI/litellm/blob/55d42888213f77582131f00134979bb8c6f8f157/litellm/proxy/a2a/agent_card.py#L85-L96)).
- Los agentes del fichero de configuración no se pueden editar ni borrar desde la interfaz ([doc](https://docs.litellm.ai/docs/a2a#define-agents-in-configyaml)).
- Los valores de `static_headers` se guardan en la base de datos y la API los devuelve ([doc](https://docs.litellm.ai/docs/a2a_agent_headers#agent-response)).

---

## 4. Preguntas abiertas

Lo que no pude confirmar:

1. **Balanceo de carga.** La documentación lo marca como soportado. No encontré el mecanismo en el camino `/a2a/{agent_id}`. Puede referirse al enrutador de modelos cuando se usa `a2a/<nombre>` por `/chat/completions`.
2. **Diferencias entre `main` y v1.110.3.** Leí `main`. No comparé fichero a fichero con la etiqueta estable.
3. **Límites TPM/RPM por agente y por sesión.** La configuración y los descriptores están confirmados; la aplicación efectiva está en código nativo que no leí. Tampoco comprobé si cuentan las invocaciones `/a2a` entrantes, las llamadas salientes del agente, o ambas.
4. **Métodos de tareas y gasto.** No comprobé si `tasks/get` y similares generan fila en SpendLogs ni si cuentan para los límites de tasa.
5. **Forma efectiva de pedir guardrails por petición.** `params.guardrails` (código) frente a `params.metadata.guardrails` (README del adaptador).
6. **`signatures` en la card servida.** La documentación dice que no se sirve; el código conserva la clave. No comprobé qué llega al cliente con un agente que firme su card.
7. **Card extendida.** No comprobé con un agente real qué campos del backend (esquemas de seguridad, interfaces adicionales en 1.0) quedan expuestos, dado que no pasa por la fusión.
8. **Agentes solo gRPC o solo HTTP+JSON.** No comprobé si `message/send` funciona de extremo a extremo.
9. **OAuth2 e introspección del proxy aplicados a `/a2a`.** Existe documentación genérica; no verifiqué su efecto en estas rutas.
10. **Integraciones de observabilidad.** Solo confirmé Langfuse por documentación y notas de versión. No verifiqué atributos OpenTelemetry específicos de A2A.
11. **Issues de `/chat/completions` con `a2a/`** (#38996, #50679, #37581, #50677): no los reproduje ni los contrasté línea a línea.
12. **Versión exacta de publicación** de: cabeceras de identidad en `message/send` ([PR #40305](https://github.com/BerriAI/litellm/pull/40305)), límites por agente y sesión ([PR #22849](https://github.com/BerriAI/litellm/pull/22849)), cabeceras estáticas y reenviadas (commits de marzo de 2026) y guardrails por agente ([PR #20619](https://github.com/BerriAI/litellm/pull/20619)).
13. **Interfaz de administración.** No la examiné; lo que digo de ella sale de la documentación.
14. **CLI y proveedor Terraform de agentes.** Confirmé que existen; no leí su contenido.
15. **Estado «beta».** Sale solo de la etiqueta OpenAPI del código. No encontré una declaración de estabilidad en la documentación.

---

## 5. Contraste con el plugin A2A para KrakenD

Esta sección no describe LiteLLM: compara lo anterior con el plugin de este repositorio, tal como está en el commit `c18741f`. Se escribió el 24 de noviembre de 2026, después de la investigación.

Criterio: solo merece adoptarse lo que resuelve un problema real que ni el plugin ni KrakenD cubren ya, y que cabe en el diseño actual. El diseño delega en KrakenD la autenticación, los límites y la telemetría, y deja fuera el contenido de los mensajes (`openspec/changes/add-a2a-gateway-plugin/design.md`, «Goals / Non-Goals»).

### 5.1 Merece la pena

| Qué | En LiteLLM | En el plugin hoy | Propuesta |
|---|---|---|---|
| Comentarios SSE antes del primer evento | Emite comentarios de keepalive en los streams (2.9) | **Fallo.** El plugin registra la propiedad a partir del primer bloque SSE. Si ese bloque es un comentario, como `: ping`, no registra nada y deja pasar el resto sin mirarlo. Quien creó la tarea recibe después `-32001` al consultarla. Falla cerrado: nadie más accede | Saltar los bloques sin `data:` hasta el primer evento real, y añadir la prueba. Es una corrección en `core/gateway/recorder.go`, no una función nueva |
| Credenciales del gateway hacia el agente | `static_headers`, `api_key` como Bearer y OAuth de máquina a máquina (2.6) | Las plantillas generan el `backend` de cada agente sin `extra_config`. El agente solo puede fiarse de las cabeceras de identidad si es inalcanzable por otra vía (`seguridad.md`, «Qué recibe el agente») | Admitir en las plantillas un `extra_config` de backend por agente, para usar lo que KrakenD ya tiene: `auth/client-credentials` o una cabecera fija. Sin código del plugin. Hay que validarlo en las dos ediciones y con streaming |
| Permisos por agente | Lista de agentes permitidos por clave y por equipo (2.5) | `authorization.operation_scopes` es global, y las plantillas usan un único bloque `auth` para todos los agentes. Cualquier cliente autenticado puede llamar a cualquier agente | Scopes o roles por agente en el `auth/validator` de sus endpoints internos, desde las plantillas. Sin código del plugin |

El fallo de la primera fila se reprodujo con una prueba temporal sobre el arnés de `core/gateway`: un agente que envía `: ping` antes del primer evento deja a la propietaria sin acceso a su tarea.

**Estado, 24 de noviembre de 2026: las tres están implementadas.**

- El fallo está corregido en `core/gateway/recorder.go`, con sus pruebas.
- Las plantillas admiten `gateway.auth` y `gateway.backend_extra_config`, documentados en el README del plugin. El entorno de demostración exige `a2a.write` en el agente research, y una prueba de extremo a extremo lo comprueba.
- La credencial hacia el agente se verificó en Community 2.13.11 y Enterprise 3.0.4, con `auth/client-credentials` y con `modifier/martian`, en peticiones unarias y en streaming.

### 5.2 Solo si aparece la necesidad

- **Keepalive generado por el gateway.** Sirve cuando un balanceador corta conexiones inactivas y el agente no envía los suyos. Los comentarios del agente ya atraviesan el gateway. Toca el camino más delicado del plugin, así que no compensa sin un caso real.
- **Cabeceras contra el buffering** (`X-Accel-Buffering: no`, `Cache-Control: no-cache`). Las del agente ya se reenvían. Añadirlas por defecto solo ayuda si hay delante un proxy que retenga la respuesta y el agente no las pone.

### 5.3 Ya cubierto, igual o mejor

| Área | LiteLLM | Plugin |
|---|---|---|
| Propiedad de tareas y contextos | No la aplica; la delega en el agente (2.7) | Registro de propietario en memoria o Redis, con denegación por defecto |
| URL de webhook dentro de `message/send` | No se valida; fallo abierto (2.8) | Se valida en las dos versiones, y se rechaza si el push está deshabilitado |
| Guardia de webhooks | HTTPS, resolución DNS y rechazo de direcciones no globales | Lo mismo, con lista de hosts permitidos |
| Agent Card | Card guardada, sin caché ni firmas; la vuelve a descargar en cada mensaje (2.3) | Caché con `ETag` y última copia válida, verificación de firmas JWS y card extendida reescrita |
| Errores | Solo códigos genéricos, con estado HTTP 4xx/5xx; sin `-32700` (2.16) | Códigos de A2A, `-32700` y recursos ajenos indistinguibles de inexistentes |
| Versión no soportada | La trata como 0.3, sin negociar (2.2) | Negocia `A2A-Version` y rechaza lo que no corresponde |
| Cabeceras del cliente hacia el agente | La convención `x-a2a-…` deja fijar `Authorization`; fallo abierto (2.6) | Lista cerrada de cabeceras; el token del cliente no llega al agente |
| Fallo del almacén de permisos | Falla en abierto (3.3) | Falla cerrado por defecto |
| Tamaño de cuerpo | Sin límite propio de A2A (2.10) | `limits.max_body_bytes` |
| Límite de peticiones por agente | TPM y RPM por agente | Límites nativos de KrakenD en cada endpoint interno |

### 5.4 Descartado

| Qué | Por qué no |
|---|---|
| Conversión entre 0.3 y 1.0 | Es un no objetivo del diseño (D8). En LiteLLM es «best effort» y arrastra fallos abiertos (#49867, #44531) |
| Catálogo público, registro y búsqueda de agentes | Fuera de alcance en la propuesta. A2A no define un catálogo estándar |
| Costes, presupuestos y topes de iteraciones | Son funciones de un proxy de modelos: necesitan contar tokens y conocer precios |
| Guardrails sobre el contenido | El plugin no interpreta ni transforma mensajes; es un no objetivo |
| Kill switch | Es un webhook hacia la plataforma del agente. En KrakenD un agente se retira con la configuración |
| Convención `x-a2a-{agente}-{cabecera}` | Es el origen de un fallo de seguridad abierto en LiteLLM (#49427) |
| Deducir la versión por el nombre del método | Relaja una comprobación que hoy evita ambigüedades |
| Puentes con `/chat/completions` y con proveedores | Convertirían el gateway en un cliente o en un agente |
| Traza obligatoria y registro de sesiones | La telemetría es de KrakenD |
| Comprobación de salud bajo demanda | No cambia el enrutado ni en LiteLLM |
