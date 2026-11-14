# Spec Delta

## Purpose

Define cómo se entrega la misma funcionalidad A2A como plugin en KrakenD Enterprise y en KrakenD Community, qué diferencias entre ediciones son admisibles y cómo se trata la licencia de Enterprise.

## ADDED Requirements

### Requirement: Artefacto para Enterprise
La funcionalidad A2A MUST entregarse para KrakenD Enterprise 3.0.4 o superior como un plugin que el gateway carga con su mecanismo estándar, sin modificar el binario de Enterprise.

#### Scenario: Carga del plugin
- **WHEN** KrakenD Enterprise arranca con el plugin en su carpeta de plugins y el bloque de configuración A2A
- **THEN** el registro de arranque muestra el plugin cargado y las rutas A2A responden

#### Scenario: Versión no coincidente
- **WHEN** el plugin se compiló para una versión de Enterprise distinta de la que lo carga
- **THEN** la comprobación de compatibilidad previa al despliegue falla e indica la discrepancia

### Requirement: Artefacto para Community
La funcionalidad A2A MUST entregarse para KrakenD Community 2.13.11 como un plugin que el binario oficial de esa versión carga con su mecanismo estándar, sin modificarlo.

#### Scenario: Carga del plugin
- **WHEN** KrakenD Community 2.13.11 arranca con el plugin en su carpeta de plugins y el bloque de configuración A2A
- **THEN** el registro de arranque muestra el plugin cargado y las rutas A2A responden

#### Scenario: Versión no coincidente
- **WHEN** el plugin se compiló para una versión de Community distinta de la que lo carga
- **THEN** la comprobación de compatibilidad previa al despliegue falla e indica la discrepancia

### Requirement: Un único código fuente para ambas ediciones
Los artefactos de ambas ediciones MUST construirse a partir del mismo código fuente, sin ramas ni variantes de comportamiento por edición.

#### Scenario: Dos compilaciones del mismo código
- **WHEN** se compila una misma revisión del código con la imagen de compilación de cada edición
- **THEN** se obtienen los dos artefactos y cada uno carga en su edición

### Requirement: Paridad funcional entre ediciones
Las capacidades de descubrimiento, pasarela de protocolo y control de acceso MUST comportarse igual en ambas ediciones para un mismo bloque de configuración A2A. Toda excepción MUST estar documentada en la matriz de compatibilidad publicada con cada versión.

#### Scenario: Misma batería de pruebas
- **WHEN** la batería de conformidad A2A se ejecuta contra Enterprise y contra Community con el mismo bloque de configuración A2A
- **THEN** obtiene los mismos resultados en ambas, salvo las excepciones documentadas

### Requirement: Bloque de configuración común
El bloque de configuración A2A MUST tener el mismo esquema en ambas ediciones, de modo que pueda copiarse sin cambios del fichero de una edición al de la otra.

#### Scenario: Mismo bloque en las dos ediciones
- **WHEN** el mismo bloque A2A se coloca en el fichero de configuración de Community y en el de Enterprise
- **THEN** ambas ediciones arrancan y publican los mismos agentes con el mismo comportamiento

### Requirement: Validación de la configuración al arrancar
El gateway MUST validar el bloque de configuración A2A al arrancar y MUST negarse a arrancar si es inválido, con un mensaje que identifique el campo incorrecto.

#### Scenario: Campo inválido
- **WHEN** un agente declara una política de tareas no registradas con un valor desconocido
- **THEN** el gateway no arranca y el mensaje de error nombra ese campo y el agente afectado

#### Scenario: Agentes duplicados
- **WHEN** dos agentes declaran el mismo identificador
- **THEN** el gateway no arranca e indica el identificador duplicado

### Requirement: Streaming en ambas ediciones
Ambas ediciones MUST entregar las respuestas en streaming de forma incremental. Si una edición no puede garantizarlo, MUST anunciar `capabilities.streaming: false` en las Agent Cards y rechazar las operaciones de streaming con el error `-32004`.

#### Scenario: Community con streaming
- **WHEN** un cliente invoca `SendStreamingMessage` contra Community
- **THEN** recibe los eventos de forma incremental, igual que en Enterprise

#### Scenario: Degradación explícita
- **WHEN** el streaming no está disponible en la edición en ejecución
- **THEN** las Agent Cards publicadas declaran `capabilities.streaming: false` y `SendStreamingMessage` recibe el error `-32004`

### Requirement: Plantillas de configuración por edición
El plugin MUST incluir plantillas que generen, a partir de una única lista declarativa de agentes, el fichero de configuración de cada edición con los endpoints internos de cada agente. Cada fichero generado MUST superar la comprobación de configuración de su edición.

#### Scenario: Alta de un agente
- **WHEN** se añade un agente a la lista declarativa y se regenera la configuración de ambas ediciones
- **THEN** cada fichero resultante contiene los endpoints internos de ese agente y supera la comprobación de configuración de su edición

#### Scenario: Piezas de seguridad presentes
- **WHEN** se genera la configuración de un agente con control de acceso activado
- **THEN** cada endpoint interno generado incluye la autenticación nativa, la propagación de identidad y la autorización A2A

### Requirement: Licencia de Enterprise fuera de los artefactos
El fichero de licencia de KrakenD Enterprise MUST NOT incluirse en el repositorio, en las imágenes construidas ni en los artefactos de pruebas. MUST suministrarse solo en tiempo de ejecución, desde una ubicación externa al repositorio.

#### Scenario: Imagen construida
- **WHEN** se inspecciona la imagen de Enterprise con el plugin
- **THEN** no contiene ningún fichero de licencia

#### Scenario: Repositorio
- **WHEN** se intenta añadir al control de versiones un fichero de licencia situado dentro del árbol del proyecto
- **THEN** las reglas de exclusión del repositorio lo impiden

### Requirement: Matriz de compatibilidad publicada
Cada versión del plugin MUST declarar las versiones exactas de KrakenD Enterprise, de KrakenD Community y de la especificación A2A con las que se ha verificado, y MUST indicar el estado de soporte de cada línea de KrakenD.

#### Scenario: Consulta de compatibilidad
- **WHEN** un operador consulta la documentación de una versión del plugin
- **THEN** encuentra las versiones exactas de ambas ediciones de KrakenD y de A2A verificadas para esa versión

#### Scenario: Línea sin correcciones
- **WHEN** una de las líneas de KrakenD soportadas ya no recibe correcciones de seguridad
- **THEN** la matriz lo indica de forma visible junto a esa línea
