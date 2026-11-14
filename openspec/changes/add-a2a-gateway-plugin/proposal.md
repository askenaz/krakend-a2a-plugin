# Proposal

## Why

KrakenD entiende tráfico LLM y MCP, pero no el protocolo Agent2Agent (A2A): hoy un agente A2A solo puede publicarse como un proxy opaco (`no-op`), sin Agent Cards que apunten al gateway, sin distinguir las operaciones JSON-RPC que comparten una misma URL y sin ligar tareas y contextos al principal autenticado. A2A ya tiene especificación estable (1.0.0) y la hoja de ruta pública de la serie 3 de KrakenD no lo incluye, así que el hueco hay que cubrirlo con una extensión propia.

## What Changes

- Nuevo **plugin A2A Gateway** para KrakenD, con un núcleo único e independiente de la edición, que añade:
  - **Descubrimiento**: publicación de Agent Cards por agente (`/.well-known/agent-card.json`), tomadas de fichero o del agente, reescritas para que anuncien solo URLs, bindings, capacidades y esquemas de seguridad del gateway.
  - **Pasarela de protocolo**: validación del sobre JSON-RPC 2.0, reconocimiento de las 11 operaciones de A2A 1.0 (y sus nombres equivalentes de 0.3), negociación de `A2A-Version`, control de operaciones habilitadas por agente, streaming SSE y normalización de errores.
  - **Control de acceso A2A**: autorización por operación, propiedad de tareas y contextos por principal (la "sesión" de A2A), guardia anti-SSRF para webhooks de notificaciones push y desafío `WWW-Authenticate` con metadatos de recurso protegido.
- **Se reutiliza la autenticación nativa de KrakenD** (`auth/validator`, revocación, mTLS, y en Enterprise API keys, multi-IdP y políticas). El plugin no valida tokens: consume la identidad que KrakenD ya verificó. Esto responde a la pregunta de si KrakenD "ya lo hace": valida el token, pero no conoce la semántica A2A.
- Cada operación A2A se encamina por un endpoint interno de KrakenD, de modo que rate limiting, telemetría, circuit breaker, balanceo y políticas nativas se aplican sin código adicional.
- **Un mismo código de plugin, compilado para dos objetivos**:
  - **Enterprise 3.0.4 o superior**, con `krakend/builder-ee`.
  - **Community 2.13.11**, la última línea de Community que carga plugins Go, con `krakend/builder`. Community 3.0 eliminó los plugins y queda fuera.
- Plantillas de Flexible Config que generan, desde una única lista declarativa de agentes, el fichero de configuración de cada edición, más un esquema JSON del bloque A2A.
- Esta fase entrega **solo diseño y propuesta**; no se escribe código.

Fuera de alcance en este cambio: bindings gRPC y HTTP+JSON/REST, transcodificación entre versiones 0.3 y 1.0, refirmado de Agent Cards, catálogo o registro de agentes, la entrega de notificaciones push (va del agente al webhook, sin pasar por el gateway) y el soporte de Community 3.x.

## Capabilities

### New Capabilities

- `a2a-agent-discovery`: publicación, origen, reescritura, caché y firma de Agent Cards (públicas y extendidas) a través del gateway.
- `a2a-protocol-gateway`: entrada JSON-RPC por agente, validación, versiones, operaciones habilitadas, encaminamiento por el pipeline nativo, streaming y errores.
- `a2a-access-control`: identidad delegada en KrakenD, autorización por operación, propiedad de tareas y contextos, guardia de webhooks push y desafíos de autenticación.
- `a2a-edition-packaging`: artefactos de plugin para Enterprise y Community, paridad funcional, configuración por edición y tratamiento de la licencia de Enterprise.

### Modified Capabilities

Ninguna. El proyecto no tiene specs previas.

## Impact

- **Código nuevo** en `a2a-plugin/` (hoy vacío): núcleo Go sin dependencias externas, adaptador de plugin único, plantillas, esquema y entorno de demo.
- **Toolchain**: dos compilaciones del mismo código. Community 2.13.11 usa Go 1.26.8 y Enterprise 3.0.4 usa Go 1.27.4, así que el núcleo se limita a Go 1.26. Hace falta Docker para ambas imágenes de compilación. Los plugins Go no se compilan en Windows; el núcleo y sus tests sí.
- **Configuración**: el bloque A2A es idéntico en ambas ediciones, pero el fichero que lo envuelve no. Community 2.13 usa `version: 3` y Enterprise 3 usa `version: 4`, con formatos distintos para declarar plugins.
- **Operación**: el plugin debe recompilarse con cada versión de KrakenD. La propiedad de tareas en despliegues multinodo requiere Redis.
- **Riesgo de la línea Community 2.13**: no ha recibido versiones desde el 8 de septiembre de 2026. Las correcciones de seguridad del 12 de noviembre de 2026 se publicaron para Community 3.0.4 y Enterprise 2.13.14, no para Community 2.13. No hay fecha de fin de vida publicada.
- **Licencia de Enterprise**: se usa solo en tiempo de ejecución, montada desde fuera del repositorio; nunca se versiona ni se copia a imágenes.
- **Riesgo de producto**: si KrakenD publica soporte A2A nativo, la configuración declarativa de agentes está pensada para poder migrarse.
