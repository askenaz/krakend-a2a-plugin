# Spec Delta

## Purpose

Hace que el gateway entienda el binding JSON-RPC de A2A: valida cada petición, negocia la versión del protocolo, identifica la operación y la encamina por el pipeline nativo de KrakenD hacia el agente correcto, incluidas las respuestas en streaming.

## ADDED Requirements

### Requirement: Punto de entrada JSON-RPC por agente
El gateway MUST aceptar peticiones A2A JSON-RPC en `POST {base_path}/{agent}` para cada agente configurado. Las peticiones a un agente no configurado MUST recibir `404`, y los métodos HTTP distintos de `POST` sobre esa ruta MUST recibir `405` con la cabecera `Allow: POST`.

#### Scenario: Agente configurado
- **WHEN** un cliente envía `POST /a2a/billing` con una petición `SendMessage` válida
- **THEN** el gateway procesa la petición para el agente `billing`

#### Scenario: Agente no configurado
- **WHEN** un cliente envía `POST /a2a/desconocido`
- **THEN** el gateway responde `404` sin contactar con ningún agente

#### Scenario: Método HTTP no admitido
- **WHEN** un cliente envía `PUT /a2a/billing`
- **THEN** el gateway responde `405` con `Allow: POST`

### Requirement: Validación del sobre JSON-RPC
El gateway MUST rechazar, sin contactar con el agente, los cuerpos que no son una petición JSON-RPC 2.0 individual y válida. Los errores de protocolo MUST devolverse como respuesta JSON-RPC con estado HTTP `200`, repitiendo el `id` de la petición o `null` si no pudo leerse.

#### Scenario: JSON mal formado
- **WHEN** el cuerpo no es JSON válido
- **THEN** el gateway responde con el error `-32700` e `id` nulo

#### Scenario: Sobre inválido
- **WHEN** el cuerpo es JSON válido pero falta `"jsonrpc": "2.0"` o `method` no es una cadena
- **THEN** el gateway responde con el error `-32600`

#### Scenario: Lote JSON-RPC
- **WHEN** el cuerpo es un array de peticiones
- **THEN** el gateway responde con el error `-32600`

### Requirement: Límite de tamaño de la petición
El gateway MUST rechazar con HTTP `413` las peticiones cuyo cuerpo supera el tamaño máximo configurado, sin contactar con el agente.

#### Scenario: Cuerpo demasiado grande
- **WHEN** el cuerpo de la petición supera `max_body_bytes`
- **THEN** el gateway responde `413`

### Requirement: Reconocimiento de operaciones
El gateway MUST reconocer las once operaciones de A2A 1.0 (`SendMessage`, `SendStreamingMessage`, `GetTask`, `ListTasks`, `CancelTask`, `SubscribeToTask`, las cuatro de configuración de notificaciones push y `GetExtendedAgentCard`) y sus nombres equivalentes de A2A 0.3. Cualquier otro método MUST recibir el error `-32601`.

#### Scenario: Nombre de la versión 1.0
- **WHEN** llega una petición con `A2A-Version: 1.0` y método `GetTask`
- **THEN** el gateway la trata como la operación `GetTask`

#### Scenario: Nombre de la versión 0.3
- **WHEN** llega una petición sin cabecera `A2A-Version` y con método `tasks/get`
- **THEN** el gateway la trata como la operación `GetTask`

#### Scenario: Método desconocido
- **WHEN** llega una petición con método `DoSomething`
- **THEN** el gateway responde con el error `-32601` y el `id` de la petición

### Requirement: Negociación de versión
El gateway MUST leer la versión solicitada de la cabecera `A2A-Version` o, si falta, del parámetro de consulta del mismo nombre, y MUST asumir `0.3` cuando no viene ninguna. Las versiones no habilitadas para el agente MUST recibir el error `-32009`, y los nombres de método que no corresponden a la versión solicitada MUST recibir `-32601`.

#### Scenario: Versión habilitada
- **WHEN** el agente admite `1.0` y la petición trae `A2A-Version: 1.0`
- **THEN** el gateway la procesa y reenvía la cabecera `A2A-Version` al agente sin cambios

#### Scenario: Versión no habilitada
- **WHEN** el agente solo admite `1.0` y la petición no trae `A2A-Version`
- **THEN** el gateway responde con el error `-32009`

#### Scenario: Nombre de método de otra versión
- **WHEN** la petición trae `A2A-Version: 1.0` y el método `message/send`
- **THEN** el gateway responde con el error `-32601`

### Requirement: Operaciones habilitadas por agente
El gateway MUST rechazar, sin contactar con el agente, las operaciones deshabilitadas para ese agente: `-32004` para las de streaming y para cualquier operación deshabilitada explícitamente, y `-32003` para las de notificaciones push cuando estas están deshabilitadas.

#### Scenario: Streaming deshabilitado
- **WHEN** el streaming está deshabilitado para `billing` y llega `SendStreamingMessage`
- **THEN** el gateway responde con el error `-32004`

#### Scenario: Notificaciones push deshabilitadas
- **WHEN** las notificaciones push están deshabilitadas para `billing` y llega `CreateTaskPushNotificationConfig`
- **THEN** el gateway responde con el error `-32003`

#### Scenario: Operación deshabilitada explícitamente
- **WHEN** `ListTasks` figura entre las operaciones deshabilitadas de `billing` y llega `ListTasks`
- **THEN** el gateway responde con el error `-32004`

### Requirement: Encaminamiento por el pipeline nativo
Toda operación aceptada MUST atravesar un endpoint de KrakenD asociado al agente y a la operación, de modo que los componentes nativos configurados en ese endpoint se apliquen. El cuerpo de la petición MUST llegar al agente byte a byte igual al que envió el cliente.

#### Scenario: Componentes nativos aplicados
- **WHEN** el endpoint interno de `billing` tiene un límite de peticiones y un cliente lo supera con llamadas `SendMessage`
- **THEN** las llamadas excedentes reciben la respuesta de límite de KrakenD y no llegan al agente

#### Scenario: Cuerpo intacto
- **WHEN** el gateway acepta una petición `SendMessage`
- **THEN** el agente recibe exactamente los mismos bytes de cuerpo que envió el cliente

#### Scenario: Granularidad por operación
- **WHEN** la configuración elige granularidad por operación y llegan `GetTask` y `CancelTask`
- **THEN** cada una atraviesa un endpoint interno distinto, con su propia configuración nativa

### Requirement: Rutas internas no accesibles desde fuera
El gateway MUST responder `404` a cualquier petición externa dirigida al prefijo de rutas internas, sin que llegue a ningún agente.

#### Scenario: Acceso directo a una ruta interna
- **WHEN** un cliente envía `POST /__a2a/billing/unary` directamente
- **THEN** el gateway responde `404` y el agente no recibe nada

### Requirement: Respuestas en streaming
En `SendStreamingMessage` y `SubscribeToTask`, el gateway MUST entregar al cliente cada evento SSE del agente en orden y en cuanto lo recibe, sin esperar al final del flujo, y MUST propagar el cierre de la conexión en ambos sentidos.

#### Scenario: Entrega incremental
- **WHEN** el agente emite un primer evento y tarda varios segundos en emitir el segundo
- **THEN** el cliente recibe el primer evento antes de que el agente emita el segundo

#### Scenario: Cliente desconectado
- **WHEN** el cliente cierra la conexión durante un flujo
- **THEN** el gateway cierra la conexión con el agente

### Requirement: Respuestas del agente sin alterar
El gateway MUST entregar al cliente las respuestas JSON-RPC del agente sin modificar su contenido, salvo en las operaciones cuya transformación exige otra capacidad.

#### Scenario: Error del agente
- **WHEN** el agente responde con el error JSON-RPC `-32002`
- **THEN** el cliente recibe esa misma respuesta de error

### Requirement: Errores del pipeline como JSON-RPC
Cuando el pipeline de KrakenD produce una respuesta sin cuerpo JSON-RPC, el gateway MUST conservar el estado HTTP y cabeceras como `Retry-After`, y MUST añadir un cuerpo de error JSON-RPC `-32603` con el `id` de la petición.

#### Scenario: Tiempo de espera agotado
- **WHEN** el agente no responde dentro del tiempo del endpoint y KrakenD devuelve un error sin cuerpo
- **THEN** el cliente recibe el mismo estado HTTP con un cuerpo de error `-32603` que repite el `id` de su petición

#### Scenario: Límite de peticiones
- **WHEN** KrakenD rechaza la llamada con `429` y `Retry-After`
- **THEN** el cliente recibe `429`, la cabecera `Retry-After` y un cuerpo de error JSON-RPC

### Requirement: Cabeceras de extensiones
El gateway MUST reenviar al agente la cabecera `A2A-Extensions` de la petición y MUST devolver al cliente la que incluya el agente en su respuesta.

#### Scenario: Extensión solicitada
- **WHEN** el cliente envía `A2A-Extensions` con una URI de extensión
- **THEN** el agente recibe esa cabecera con el mismo valor

### Requirement: Contexto de la operación para componentes nativos
El gateway MUST exponer al endpoint interno el agente, la operación y la versión de cada petición en cabeceras reservadas, para que las reglas nativas de KrakenD puedan usarlas.

#### Scenario: Regla nativa sobre la operación
- **WHEN** el endpoint interno tiene una regla que rechaza la operación `CancelTask` y llega una petición `CancelTask`
- **THEN** la regla nativa evalúa la operación a partir de la cabecera reservada y rechaza la llamada

### Requirement: Registro de actividad sin contenido sensible
Por cada petición A2A procesada, el gateway MUST registrar el agente, la operación, la versión y el resultado, y MUST NOT registrar el contenido de los mensajes ni credenciales.

#### Scenario: Petición rechazada
- **WHEN** el gateway rechaza una petición con el error `-32009`
- **THEN** el registro incluye agente, operación, versión solicitada y el código de error, y no incluye el cuerpo de la petición
