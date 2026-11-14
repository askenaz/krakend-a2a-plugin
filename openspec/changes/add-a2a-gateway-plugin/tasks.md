# Tasks

## 1. Preparación del repositorio y del entorno

- [x] 1.1 Inicializar git en la raíz y crear la estructura de `a2a-plugin/` descrita en design.md D15; verificar que existen los directorios `core/`, `plugin/`, `build/community/`, `build/enterprise/`, `config/`, `deploy/`, `e2e/` y `docs/`
- [x] 1.2 Añadir `.gitignore` y `.dockerignore` que excluyan ficheros de licencia y el directorio de secretos; verificar con `git check-ignore` que un fichero de licencia colocado dentro del árbol queda ignorado
- [x] 1.3 Documentar en `docs/desarrollo.md` cómo suministrar la licencia de Enterprise con `KRAKEND_LICENSE_PATH` desde una ruta externa, sin nombrar a su titular; verificar que el comando documentado arranca la imagen de Enterprise 3.0.4
- [x] 1.4 Crear el módulo Go de `core/` con directiva `go 1.26`; verificar que `go build ./...` y `go vet ./...` terminan sin errores y que `go list -m all` no muestra dependencias externas
- [x] 1.5 Crear un agente A2A simulado para pruebas en `deploy/mock-agent/` con respuestas unarias y SSE configurables; verificar con `curl` que responde a `SendMessage` y emite eventos espaciados en `SendStreamingMessage`

## 2. Validación de supuestos (fase 0)

- [x] 2.1 Prototipo de plugin de servidor que reescribe la ruta y cede el control, compilado con `krakend/builder:2.13.11` y con `krakend/builder-ee:3.0.4` (S1, S5); verificar en ambas ediciones que una petición a `/a2a/x` alcanza el endpoint `/__a2a/x/unary`
- [x] 2.2 Añadir al prototipo un envoltorio de respuesta que haga flush en cada escritura de `text/event-stream` (S2); verificar con el agente simulado, en ambas ediciones, que el primer evento SSE llega al cliente antes de que se emita el segundo
- [x] 2.3 Exportar en el mismo `.so` un modificador de petición y comprobar el estado compartido, la visibilidad de las cabeceras de `propagate_claims` y el corte del pipeline (S3, S4); verificar con un log en ambas ediciones que el modificador lee la entrada creada por el plugin de servidor y el sujeto del JWT, y que el agente no recibe la petición denegada
- [x] 2.4 Comprobar que el prototipo recibe la configuración con la forma documentada en cada edición y que el adaptador distingue ambas (S5); verificar con un test del adaptador y con el arranque real en las dos ediciones
- [ ] 2.5 Comprobar que la configuración de ejemplo de cada edición supera `krakend check` (S6) y que el plugin carga en Enterprise sin el módulo AI Gateway (S7); verificar con la salida de los comandos
- [x] 2.6 Registrar los resultados en `docs/validacion-supuestos.md` y actualizar design.md si algún supuesto falla; verificar que cada supuesto S1 a S7 tiene resultado y evidencia por edición

## 3. Núcleo: configuración y protocolo

- [x] 3.1 Implementar el modelo del bloque de configuración A2A y su validación estricta; verificar con tests que valores desconocidos e identificadores de agente duplicados producen un error que nombra el campo
- [x] 3.2 Publicar el esquema JSON del bloque en `config/schema/`; verificar con un test que los ejemplos válidos lo cumplen y los inválidos no
- [x] 3.3 Implementar el análisis superficial del sobre JSON-RPC con límite de tamaño; verificar con tests los casos `-32700`, `-32600`, lote y `413`
- [x] 3.4 Implementar la tabla de operaciones con nombres 1.0 y 0.3 y la negociación de `A2A-Version`; verificar con tests de tabla todas las combinaciones, incluidos `-32601` y `-32009`
- [x] 3.5 Implementar la extracción de `taskId`, `contextId` y URLs de webhook en peticiones, resultados y eventos de ambas versiones; verificar con tests basados en los ejemplos de la especificación
- [x] 3.6 Documentar el bloque de configuración en `docs/configuracion.md`; verificar que cada campo del esquema aparece documentado

## 4. Descubrimiento de agentes

- [x] 4.1 Implementar la reescritura de Agent Cards sobre árbol JSON genérico para las formas 1.0 y 0.3; verificar con tests que no queda ninguna URL interna, que se filtran bindings y que los campos desconocidos se conservan
- [x] 4.2 Implementar la sustitución de esquemas de seguridad, las capacidades efectivas y la eliminación de `signatures`; verificar con tests los escenarios de la spec `a2a-agent-discovery`
- [x] 4.3 Implementar los orígenes estático y remoto con TTL, obsolescencia máxima y límite de tamaño; verificar con tests de reloj simulado los escenarios de caché vigente, copia obsoleta y `503`
- [x] 4.4 Implementar las rutas de descubrimiento por agente y del agente por defecto con `ETag`, `Cache-Control` y `304`; verificar con tests HTTP
- [x] 4.5 Implementar la verificación opcional de firma de la card remota; verificar con una card firmada de prueba que una firma inválida impide su publicación

## 5. Pasarela de protocolo en el borde

- [x] 5.1 Implementar el envoltorio `http.Handler` del borde: limpieza de cabeceras reservadas, entrada por agente, `404` y `405`; verificar con tests HTTP
- [x] 5.2 Implementar el control de operaciones habilitadas por agente y la reescritura a rutas internas en granularidad `class` y `operation`; verificar con tests que el cuerpo llega intacto al siguiente handler y que las cabeceras de contexto están presentes
- [x] 5.3 Bloquear el acceso externo al prefijo interno; verificar con un test que devuelve `404` sin invocar al siguiente handler
- [x] 5.4 Implementar el envoltorio de respuesta con flush por escritura en SSE y propagación de cierre; verificar con un test de streaming que mide la llegada incremental y la cancelación
- [x] 5.5 Implementar la normalización de errores del pipeline y el paso sin cambios de respuestas del agente; verificar con tests los casos `429`, `5xx` sin cuerpo y error JSON-RPC del agente
- [x] 5.6 Implementar el registro estructurado por petición sin contenido sensible; verificar con un test que el log no contiene el cuerpo ni la cabecera `Authorization`

## 6. Adaptador de plugin y artefacto de Enterprise

- [x] 6.1 Implementar el adaptador único con `HandlerRegisterer` y `RegisterLogger`, que acepta la forma de configuración de ambas ediciones; verificar con tests del adaptador las dos formas
- [x] 6.2 Crear `build/enterprise/` con `krakend/builder-ee` de la versión fijada, el artefacto nombrado con edición y versión, su manifiesto, el script que compara el manifiesto con `krakend version`, y las comprobaciones `krakend plugin check` y `krakend plugin test` con la licencia como secreto; verificar que el arranque muestra el plugin cargado, que el script rechaza el artefacto de Community y que la imagen resultante no contiene ningún fichero de licencia
- [x] 6.3 Escribir las plantillas de Flexible Config y el ejemplo de Enterprise con `version: 4`; verificar que la configuración generada supera `krakend check`
- [x] 6.4 Prueba de integración de descubrimiento, operación unaria y streaming contra el agente simulado en Enterprise; verificar que pasan los escenarios de las specs `a2a-agent-discovery` y `a2a-protocol-gateway`
- [x] 6.5 Documentar instalación y actualización en `docs/enterprise.md`; verificar que los pasos reproducen el despliegue desde cero

## 7. Artefacto de Community 2.13

- [x] 7.1 Crear `build/community/` con `krakend/builder:2.13.11`, el artefacto nombrado con edición y versión, su manifiesto y las comprobaciones `krakend check-plugin` y `krakend test-plugin`; verificar que el binario oficial 2.13.11 muestra el plugin cargado y que el script de comprobación rechaza el artefacto de Enterprise
- [x] 7.2 Añadir a la integración continua la compilación del mismo código con Go 1.26 y Go 1.27; verificar que un uso de API posterior a Go 1.26 hace fallar la compilación de Community
- [x] 7.3 Escribir las plantillas de Flexible Config y el ejemplo de Community con `version: 3`, a partir de la misma lista de agentes que Enterprise; verificar que la configuración generada supera `krakend check`
- [x] 7.4 Ejecutar la misma prueba de integración de 6.4 contra Community 2.13.11; verificar que los resultados coinciden con los de Enterprise, incluido el streaming incremental
- [x] 7.5 Documentar instalación, límites y estado de soporte de la línea 2.13 en `docs/community.md`; verificar que los pasos reproducen el despliegue y que el aviso de línea sin correcciones es visible

## 8. Control de acceso

- [x] 8.1 Implementar la correlación entre el borde y la autorización con fallo cerrado; verificar con tests que una petición sin entrada de correlación se deniega y que las entradas se liberan al terminar
- [x] 8.2 Implementar la decisión de identidad obligatoria y scopes por operación; verificar con tests los escenarios `401` y `403`
- [x] 8.3 Implementar la interfaz de almacén de propiedad y la versión en memoria con TTL deslizante y tamaño acotado; verificar con tests de concurrencia y de caducidad
- [x] 8.4 Implementar las comprobaciones de propiedad de tareas y contextos, y las políticas `unknown_task` y `list_tasks`; verificar con tests todos los escenarios de la spec `a2a-access-control`
- [x] 8.5 Implementar el registro de propiedad desde respuestas unarias y desde el primer evento de un flujo, antes de entregarlos; verificar con un test que una consulta inmediata del propietario ya está permitida
- [x] 8.6 Implementar la guardia de webhooks push; verificar con tests de direcciones privadas, esquema, lista de hosts y resolución de nombres
- [x] 8.7 Implementar el desafío `WWW-Authenticate`, los metadatos de recurso protegido y la exigencia de identidad en la card extendida; verificar con tests HTTP
- [x] 8.8 Exportar la autorización como modificador de petición con `ModifierRegisterer` y añadirla a las plantillas de ambas ediciones; verificar en Community y en Enterprise que una tarea ajena recibe `-32001` y el agente no recibe la petición
- [x] 8.9 Implementar el cliente Redis mínimo sobre la biblioteca estándar, con autenticación y TLS; verificar con tests contra un Redis real y con dos instancias del gateway que la propiedad registrada en una se respeta en la otra
- [x] 8.10 Documentar el modelo de seguridad, sus límites y la activación progresiva en `docs/seguridad.md`; verificar que cubre identidad, propiedad, webhooks y fallo cerrado

## 9. Entorno de demostración

- [x] 9.1 Crear `deploy/docker-compose` con gateway, agente simulado, IdP y Redis para cada edición; verificar que un único comando levanta cada variante
- [x] 9.2 Añadir un guion de demostración con obtención de token, descubrimiento, mensaje, streaming y acceso denegado a tarea ajena; verificar que el guion termina con los resultados esperados en ambas ediciones

## 10. Integración y conformidad

- [x] 10.1 Crear la batería de conformidad en `e2e/` usando el SDK oficial de A2A como cliente; verificar que cubre las once operaciones en versión 1.0 y los nombres de 0.3
- [x] 10.2 Ejecutar la batería contra Enterprise 3.0.4 y contra Community 2.13.11 con el mismo bloque A2A; verificar que los resultados coinciden salvo excepciones documentadas
- [x] 10.3 Ejecutar una prueba de carga con peticiones unarias y flujos concurrentes en ambas ediciones; verificar que la memoria se mantiene acotada y que no quedan entradas de correlación sin liberar
- [x] 10.4 Publicar la matriz de compatibilidad con las versiones verificadas y el estado de soporte de cada línea de KrakenD; verificar que coincide con las versiones usadas en 10.2

## Workflow follow-up

- Archivar el cambio cuando la implementación esté revisada.
