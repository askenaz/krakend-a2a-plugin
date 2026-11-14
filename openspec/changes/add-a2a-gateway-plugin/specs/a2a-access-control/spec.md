# Spec Delta

## Purpose

Garantiza que solo los principals autenticados por KrakenD operen sobre los agentes y que cada tarea o contexto A2A sea accesible únicamente para quien lo creó, y protege los webhooks de notificaciones push frente a destinos no permitidos.

## ADDED Requirements

### Requirement: Identidad delegada en la autenticación nativa
El gateway MUST obtener la identidad del llamante únicamente de las cabeceras de identidad que la autenticación nativa de KrakenD produce en el endpoint interno. El gateway MUST NOT validar tokens por su cuenta ni derivar la identidad del contenido de un token sin verificar.

#### Scenario: Token válido
- **WHEN** el endpoint interno valida un JWT y propaga sus claims de emisor y sujeto a las cabeceras de identidad configuradas
- **THEN** el gateway usa esos valores como identidad del llamante

#### Scenario: Token con firma inválida
- **WHEN** llega una petición con un JWT cuya firma no es válida
- **THEN** la validación nativa la rechaza y el gateway no toma ninguna decisión de acceso basada en los claims de ese token

### Requirement: Cabeceras reservadas no falsificables
El gateway MUST eliminar de toda petición entrante las cabeceras reservadas del módulo y las cabeceras de identidad configuradas, antes de cualquier otro procesamiento.

#### Scenario: Cabecera de identidad enviada por el cliente
- **WHEN** un cliente envía la cabecera de identidad de sujeto con el valor de otro usuario
- **THEN** ese valor no llega al endpoint interno ni se usa como identidad

### Requirement: Fallo cerrado sin identidad
Cuando el control de acceso exige identidad y el endpoint interno no la produce, el gateway MUST rechazar la petición con HTTP `401` y MUST NOT entregarla al agente.

#### Scenario: Endpoint interno sin autenticación
- **WHEN** la identidad es obligatoria y el endpoint interno no tiene configurada ninguna autenticación nativa
- **THEN** el gateway responde `401` y el agente no recibe la petición

### Requirement: Errores de autenticación a nivel HTTP
El gateway MUST conservar los estados `401` y `403` que produce la autenticación nativa. En las respuestas `401`, MUST añadir una cabecera `WWW-Authenticate` con el esquema esperado y, cuando está configurado, la referencia a los metadatos de recurso protegido.

#### Scenario: Petición sin credenciales
- **WHEN** un cliente invoca `SendMessage` sin token
- **THEN** recibe `401` con una cabecera `WWW-Authenticate` de esquema `Bearer`

#### Scenario: Rol insuficiente
- **WHEN** la validación nativa rechaza el token por falta de rol
- **THEN** el cliente recibe `403`

### Requirement: Autorización por operación
Cuando la configuración asocia scopes a una operación, el gateway MUST rechazar con HTTP `403`, sin entregarlas al agente, las peticiones cuyo llamante no posee los scopes exigidos.

#### Scenario: Scope insuficiente
- **WHEN** `CancelTask` exige el scope `a2a.write` y el llamante solo tiene `a2a.read`
- **THEN** el gateway responde `403` y el agente no recibe la petición

#### Scenario: Scope suficiente
- **WHEN** `GetTask` exige `a2a.read` y el llamante lo tiene
- **THEN** el gateway continúa con el resto de comprobaciones

### Requirement: Registro de la propiedad de tareas y contextos
Cuando una respuesta correcta del agente revela una tarea o un contexto nuevos, el gateway MUST registrar al llamante como su propietario antes de entregar al cliente esa respuesta o ese evento.

#### Scenario: Tarea creada por SendMessage
- **WHEN** el agente responde a `SendMessage` con una tarea nueva
- **THEN** la tarea y su contexto quedan registrados a nombre del llamante antes de que este reciba la respuesta

#### Scenario: Tarea creada en un flujo
- **WHEN** el primer evento de `SendStreamingMessage` contiene una tarea nueva
- **THEN** la propiedad queda registrada antes de que el cliente reciba ese primer evento

#### Scenario: Respuesta con error
- **WHEN** el agente responde con un error JSON-RPC
- **THEN** el gateway no registra ninguna propiedad

### Requirement: Acceso a tareas solo para su propietario
Las operaciones que referencian una tarea existente MUST rechazarse con el error `-32001` cuando el llamante no es su propietario, sin entregarlas al agente y con una respuesta indistinguible de la de una tarea inexistente.

#### Scenario: Propietario
- **WHEN** el propietario de la tarea `T1` invoca `GetTask` sobre `T1`
- **THEN** la petición llega al agente

#### Scenario: Otro principal
- **WHEN** un principal distinto del propietario invoca `CancelTask` sobre `T1`
- **THEN** el gateway responde con el error `-32001` y el agente no recibe la petición

#### Scenario: Configuración push de tarea ajena
- **WHEN** un principal invoca `GetTaskPushNotificationConfig` sobre una tarea de otro principal
- **THEN** el gateway responde con el error `-32001`

### Requirement: Acceso a contextos solo para su propietario
Un mensaje que referencia un contexto registrado a nombre de otro principal MUST rechazarse con el error `-32001`, sin entregarse al agente. Un contexto no registrado MUST tratarse como nuevo.

#### Scenario: Contexto ajeno
- **WHEN** un principal envía `SendMessage` con un `contextId` registrado a nombre de otro
- **THEN** el gateway responde con el error `-32001`

#### Scenario: Contexto nuevo aportado por el cliente
- **WHEN** un principal envía `SendMessage` con un `contextId` no registrado y el agente responde correctamente
- **THEN** el contexto queda registrado a nombre de ese principal

### Requirement: Tareas no registradas
Para las tareas que no figuran en el registro de propiedad, el gateway MUST aplicar la política configurada: denegar con el error `-32001`, que es el valor por defecto, o dejar pasar la petición.

#### Scenario: Política por defecto
- **WHEN** llega `GetTask` sobre una tarea sin registro y la política es denegar
- **THEN** el gateway responde con el error `-32001`

#### Scenario: Política permisiva
- **WHEN** llega `GetTask` sobre una tarea sin registro y la política es dejar pasar
- **THEN** la petición llega al agente

### Requirement: Listado de tareas acotado
El gateway MUST aplicar a `ListTasks` la política configurada: exigir un `contextId` propiedad del llamante, que es el valor por defecto, rechazar la operación, o dejarla pasar.

#### Scenario: Contexto propio
- **WHEN** la política exige contexto propio y el llamante envía `ListTasks` con un `contextId` suyo
- **THEN** la petición llega al agente

#### Scenario: Sin contexto
- **WHEN** la política exige contexto propio y el llamante envía `ListTasks` sin `contextId`
- **THEN** el gateway responde con el error `-32602`

#### Scenario: Contexto ajeno
- **WHEN** la política exige contexto propio y el `contextId` pertenece a otro principal
- **THEN** el gateway responde con el error `-32001`

### Requirement: Ámbito de propiedad configurable
La clave de propietario MUST derivarse de las cabeceras de identidad configuradas, por defecto emisor y sujeto, de forma que la propiedad pueda compartirse a un nivel más amplio como el cliente o el tenant.

#### Scenario: Propiedad por sujeto
- **WHEN** el ámbito es emisor y sujeto, y dos sujetos distintos del mismo emisor acceden a la misma tarea
- **THEN** solo el sujeto que la creó obtiene acceso

#### Scenario: Propiedad por tenant
- **WHEN** el ámbito es el claim de tenant y dos sujetos del mismo tenant acceden a la misma tarea
- **THEN** ambos obtienen acceso

### Requirement: Registro de propiedad compartido entre nodos
Con un almacén compartido configurado, la propiedad registrada por un nodo del gateway MUST ser respetada por los demás nodos.

#### Scenario: Petición atendida por otro nodo
- **WHEN** un nodo registra la tarea `T1` y la siguiente petición sobre `T1` la atiende otro nodo
- **THEN** el segundo nodo aplica la misma decisión de propiedad

### Requirement: Fallo del almacén de propiedad
Cuando el almacén de propiedad no está disponible, el gateway MUST rechazar con el error `-32603`, sin entregarlas al agente, las operaciones que necesitan consultarlo, salvo que la configuración indique explícitamente dejar pasar.

#### Scenario: Almacén caído con la política por defecto
- **WHEN** el almacén no responde y llega `GetTask`
- **THEN** el gateway responde con el error `-32603` y el agente no recibe la petición

### Requirement: Caducidad de los registros de propiedad
Los registros de propiedad MUST caducar cuando transcurre el tiempo de vida configurado desde su último uso.

#### Scenario: Registro caducado
- **WHEN** nadie usa la tarea `T1` durante más tiempo que el TTL configurado
- **THEN** `T1` pasa a tratarse como tarea no registrada

### Requirement: Guardia de destinos de notificaciones push
El gateway MUST validar la URL de webhook de toda configuración de notificaciones push, tanto en `CreateTaskPushNotificationConfig` como en la configuración incluida en `SendMessage`. Las URLs no permitidas MUST rechazarse con el error `-32602` sin entregar la petición al agente.

#### Scenario: Dirección privada
- **WHEN** la URL de webhook apunta a una dirección de loopback, privada o link-local
- **THEN** el gateway responde con el error `-32602`

#### Scenario: Esquema no seguro
- **WHEN** se exige HTTPS y la URL de webhook usa `http`
- **THEN** el gateway responde con el error `-32602`

#### Scenario: Host fuera de la lista permitida
- **WHEN** hay una lista de hosts permitidos y el host de la URL no coincide con ninguno
- **THEN** el gateway responde con el error `-32602`

#### Scenario: Destino permitido
- **WHEN** la URL usa HTTPS y su host coincide con la lista permitida
- **THEN** la petición continúa hacia el agente

### Requirement: Agent Card extendida solo con identidad
La operación `GetExtendedAgentCard` MUST exigir siempre identidad, con independencia de que el resto del control de acceso esté activado.

#### Scenario: Sin identidad
- **WHEN** llega `GetExtendedAgentCard` y el endpoint interno no produce identidad
- **THEN** el gateway responde `401`

### Requirement: Metadatos de recurso protegido
Cuando está configurado, el gateway MUST publicar para cada agente un documento de metadatos de recurso protegido OAuth que indique el recurso, los servidores de autorización y los scopes admitidos.

#### Scenario: Descubrimiento del servidor de autorización
- **WHEN** un cliente pide el documento de metadatos de recurso protegido del agente `billing`
- **THEN** recibe un JSON cuyo `resource` es la URL pública del agente y que lista los servidores de autorización configurados

### Requirement: Sin secretos en los registros
El gateway MUST NOT escribir en los registros tokens, credenciales de configuración push ni identificadores de principal en claro. Los principals MUST aparecer solo como un resumen no reversible.

#### Scenario: Decisión de acceso registrada
- **WHEN** el gateway deniega una operación por propiedad
- **THEN** el registro incluye el resumen del principal y el motivo, y no incluye el token ni el sujeto en claro
