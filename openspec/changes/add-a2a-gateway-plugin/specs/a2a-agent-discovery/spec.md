# Spec Delta

## Purpose

Permite que los clientes A2A descubran a través del gateway los agentes publicados, recibiendo Agent Cards que apuntan al gateway y describen cómo autenticarse contra él, sin exponer la topología interna de los agentes.

## ADDED Requirements

### Requirement: Publicación de la Agent Card por agente
El gateway MUST servir la Agent Card pública de cada agente configurado en `GET {base_path}/{agent}/.well-known/agent-card.json`, con `Content-Type: application/json`, sin exigir credenciales.

#### Scenario: Agente configurado
- **WHEN** un cliente envía `GET /a2a/billing/.well-known/agent-card.json` y `billing` es un agente configurado
- **THEN** el gateway responde `200` con la Agent Card de `billing` en JSON

#### Scenario: Agente no configurado
- **WHEN** un cliente envía `GET /a2a/desconocido/.well-known/agent-card.json` y `desconocido` no está configurado
- **THEN** el gateway responde `404` y no contacta con ningún agente

### Requirement: Agent Card del agente por defecto en la raíz
Cuando la configuración declara un agente por defecto, el gateway MUST servir su Agent Card también en `GET /.well-known/agent-card.json`. Cuando no hay agente por defecto, el gateway MUST dejar esa ruta al enrutador normal de KrakenD.

#### Scenario: Con agente por defecto
- **WHEN** `default_agent` vale `billing` y un cliente envía `GET /.well-known/agent-card.json`
- **THEN** el gateway responde `200` con la misma Agent Card que publica en la ruta de `billing`

#### Scenario: Sin agente por defecto
- **WHEN** no hay `default_agent` y un cliente envía `GET /.well-known/agent-card.json`
- **THEN** la petición la resuelve KrakenD como cualquier otra ruta, sin intervención del módulo A2A

### Requirement: Origen estático de la Agent Card
El gateway MUST poder publicar una Agent Card definida en la configuración o en un fichero local, sin contactar con el agente.

#### Scenario: Card estática
- **WHEN** el agente `billing` declara una card de origen estático y un cliente pide su Agent Card
- **THEN** el gateway responde con esa card, ya transformada, sin hacer ninguna petición al agente

### Requirement: Origen remoto de la Agent Card con caché
El gateway MUST poder obtener la Agent Card desde la URL de descubrimiento del agente y MUST reutilizarla durante el tiempo de vida configurado. Si el agente no responde, MUST servir la última copia válida hasta el máximo de obsolescencia configurado, y `503` si no tiene copia.

#### Scenario: Card en caché vigente
- **WHEN** el gateway obtuvo la card de `billing` hace menos tiempo que el TTL configurado y llega una nueva petición de descubrimiento
- **THEN** el gateway responde desde la caché sin contactar con el agente

#### Scenario: Agente caído con copia obsoleta admisible
- **WHEN** el TTL ha vencido, el agente no responde y la copia tiene menos antigüedad que `max_stale`
- **THEN** el gateway responde `200` con la copia obsoleta

#### Scenario: Agente caído sin copia
- **WHEN** el gateway nunca obtuvo la card de `billing` y el agente no responde
- **THEN** el gateway responde `503`

### Requirement: Reescritura de interfaces hacia el gateway
La Agent Card publicada MUST anunciar únicamente URLs del gateway: cada interfaz conservada apunta a la URL pública del agente en el gateway, y las interfaces con bindings no habilitados en el gateway se eliminan. Ninguna URL interna del agente MUST aparecer en la card publicada.

#### Scenario: Card 1.0 con varios bindings
- **WHEN** la card del agente declara en `supportedInterfaces` un binding `JSONRPC` y otro `GRPC`, y el gateway solo habilita `JSONRPC`
- **THEN** la card publicada contiene una única interfaz `JSONRPC` cuya `url` es `{public_url}{base_path}/billing`

#### Scenario: Card con forma 0.3
- **WHEN** la card del agente usa los campos `url`, `preferredTransport` y `additionalInterfaces` de A2A 0.3
- **THEN** la card publicada conserva esa forma y sus URLs apuntan al gateway

#### Scenario: Interfaz con tenant
- **WHEN** una interfaz conservada declara un valor `tenant`
- **THEN** la card publicada mantiene ese mismo valor de `tenant` en la interfaz

### Requirement: Esquemas de seguridad del gateway
Cuando la configuración define esquemas y requisitos de seguridad para un agente, la Agent Card publicada MUST declarar esos y no los del agente, de modo que el cliente sepa cómo autenticarse contra el gateway.

#### Scenario: Esquemas sustituidos
- **WHEN** el agente declara un esquema de API key interno y la configuración del gateway define un esquema OpenID Connect para ese agente
- **THEN** la card publicada declara solo el esquema OpenID Connect y sus requisitos

#### Scenario: Sin esquemas configurados
- **WHEN** la configuración no define esquemas de seguridad para el agente
- **THEN** la card publicada conserva los esquemas y requisitos que declara el agente

### Requirement: Capacidades efectivas
Las capacidades `streaming` y `pushNotifications` de la card publicada MUST ser verdaderas solo si el agente las declara y además el gateway las tiene habilitadas para ese agente.

#### Scenario: Streaming deshabilitado en el gateway
- **WHEN** el agente declara `capabilities.streaming: true` y el gateway tiene el streaming deshabilitado para ese agente
- **THEN** la card publicada declara `capabilities.streaming: false`

### Requirement: Preservación de campos no gestionados
El gateway MUST conservar sin cambios todos los campos de la Agent Card que no transforma, incluidos los que no conoce.

#### Scenario: Campo desconocido
- **WHEN** la card del agente incluye un campo que la especificación soportada no define
- **THEN** la card publicada incluye ese campo con el mismo valor

### Requirement: Firmas de la Agent Card
El gateway MUST eliminar el campo `signatures` de toda card que haya transformado. Cuando la verificación de firma está activada para un agente, el gateway MUST rechazar las cards remotas cuya firma no se valide con las claves de confianza configuradas.

#### Scenario: Card transformada
- **WHEN** la card del agente trae `signatures` y el gateway reescribe sus interfaces
- **THEN** la card publicada no contiene `signatures`

#### Scenario: Firma remota inválida
- **WHEN** la verificación de firma está activada y la card remota no tiene ninguna firma válida
- **THEN** el gateway no publica esa card y trata la obtención como fallida

### Requirement: Caché HTTP de la Agent Card
Las respuestas de descubrimiento MUST incluir `ETag` y `Cache-Control`, y el gateway MUST responder `304` a las peticiones condicionales cuyo validador coincide.

#### Scenario: Petición condicional
- **WHEN** un cliente envía `If-None-Match` con el `ETag` vigente de la card
- **THEN** el gateway responde `304` sin cuerpo

### Requirement: Agent Card extendida
El resultado de la operación `GetExtendedAgentCard` MUST recibir las mismas transformaciones que la card pública antes de entregarse al cliente.

#### Scenario: Card extendida reescrita
- **WHEN** un cliente autenticado invoca `GetExtendedAgentCard` y el agente devuelve una card con sus URLs internas
- **THEN** el resultado que recibe el cliente anuncia solo URLs del gateway
