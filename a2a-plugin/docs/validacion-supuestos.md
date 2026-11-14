# Validación de supuestos · fase 0

Fecha: 13 de noviembre de 2026.
Cambio: `add-a2a-gateway-plugin`, grupo 2 de `tasks.md`.

El prototipo de `a2a-plugin/prototype/` se compiló a partir de un único código fuente para las dos ediciones y se probó con el agente simulado de `a2a-plugin/deploy/mock-agent/`. La herramienta `prototype/tools/probe` ejecutó los mismos escenarios contra cada gateway.

## Entorno

| Pieza | Community | Enterprise |
|---|---|---|
| Binario | `krakend:2.13.11` | `krakend/krakend-ee:3.0.4` |
| Versión de Go del binario | 1.26.8 | 1.27.4 |
| Imagen de compilación | `krakend/builder:2.13.11` | `krakend/builder-ee:3.0.4` |
| Versión de Go del compilador | 1.26.8 | 1.27.4 |
| Base | Alpine 3.23, musl | Alpine 3.23, musl |
| Formato de configuración | `version: 3` | `version: 4` |

## Resultados por supuesto

| # | Supuesto | Community 2.13.11 | Enterprise 3.0.4 | Evidencia |
|---|---|---|---|---|
| S1 | El plugin de servidor reescribe la ruta y el enrutador la respeta | Confirmado | Confirmado | T1, T3, T10 |
| S2 | El flush del envoltorio entrega el SSE sin retención | Confirmado | Confirmado | T7, control T8 |
| S3 | Los dos símbolos del mismo `.so` comparten estado | Confirmado | Confirmado | T1, T5, T9, T11 |
| S4 | El modificador ve la identidad propagada y corta el pipeline | Confirmado | Confirmado | T1b, T5, T9 |
| S5 | Un código compila y carga en ambas; la configuración llega con la forma documentada | Confirmado | Confirmado | Compilación, `test-plugin`, registro de arranque |
| S6 | La configuración supera `krakend check` | Confirmado | Confirmado | `krakend check -t -l -c` |
| S7 | Los plugins cargan sin depender del módulo AI Gateway | No aplica | Parcial | Ver nota |

**Nota sobre S7.** El plugin carga y funciona con la licencia suministrada, y la configuración no usa ningún componente de AI Gateway. No se inspeccionó qué módulos incluye la licencia, para no exponer sus datos. Por eso no queda demostrado que funcione con una licencia sin AI Gateway. La documentación de KrakenD sitúa los plugins Go en el núcleo de Enterprise, no en ese módulo.

## Escenarios ejecutados

Los mismos escenarios pasaron en ambas ediciones.

| Id | Escenario | Resultado esperado |
|---|---|---|
| T1 | `SendMessage` con token válido | Llega al agente por la ruta interna, con el cuerpo intacto y las cabeceras de contexto |
| T1b | Identidad propagada | El agente ve el sujeto del JWT en la cabecera de identidad |
| T1c | Token del cliente | No llega al agente |
| T2 | Cabeceras reservadas enviadas por el cliente | Se eliminan; el agente ve la identidad del token |
| T3 | Acceso directo a la ruta interna | `404`, el agente no recibe nada |
| T4 | Sin token | `401` de la validación nativa, el agente no recibe nada |
| T5 | Sujeto denegado, operación unaria | Error `-32001`, el agente no recibe nada |
| T6 | Cuerpo que no es JSON | Error `-32700`, el agente no recibe nada |
| T7 | `SendStreamingMessage` con flush del plugin | Eventos entregados según se emiten |
| T8 | Control: mismo flujo sin flush del plugin | Informativo |
| T9 | Sujeto denegado, operación de streaming | Error `-32001`, el agente no recibe nada |
| T10 | Ruta no A2A | Sigue funcionando |
| T11 | Entradas de correlación | Ninguna queda viva al terminar |

### Streaming

El agente simulado emite tres eventos separados por 1,5 segundos. La tabla muestra cuándo llega cada evento al cliente, contado desde el envío de la petición.

| Edición | Con flush del plugin | Sin flush del plugin |
|---|---|---|
| Community 2.13.11 | 0 s · 1,5 s · 3 s | 3 s · 3 s · 3 s |
| Enterprise 3.0.4 | 0 s · 1,4 s · 2,8 s | 0 s · 1,5 s · 3 s |

Community retiene todo el flujo hasta el final si el plugin no hace flush. Enterprise hace flush por su cuenta. El flush del plugin es imprescindible en Community e inocuo en Enterprise.

## Hallazgos que afectan al diseño

1. **Forma de la configuración.** Community 2.13 entrega al plugin el mapa del espacio de nombres, con la configuración bajo la clave del nombre del plugin. Enterprise 3.0 entrega directamente el contenido de `config`. El adaptador distingue ambas formas sin ambigüedad.
2. **Artefacto de la edición equivocada.** Cargar el `.so` de una edición en el binario de la otra aborta el proceso con un error fatal del runtime de Go (`no plugin module data`). No es un error controlado, así que las opciones de KrakenD para fallos de carga de plugins no lo evitan: el gateway no arranca. Hace falta nombrar cada artefacto con su edición y versión, y comprobarlo antes de desplegar.
3. **Licencia en las comprobaciones.** En Enterprise, `krakend check` funciona sin licencia, pero `krakend plugin test` la necesita. Una integración continua que pruebe la carga del plugin de Enterprise necesita la licencia como secreto en tiempo de ejecución.
4. **El token no llega al agente.** Con la configuración propuesta, la cabecera `Authorization` no se reenvía al agente, porque no figura en `input_headers`. Es lo deseable por defecto. Un agente que necesite el token del llamante debe pedirlo de forma explícita en la configuración.
5. **Instancias del modificador.** KrakenD crea una instancia del modificador por endpoint. El estado compartido debe vivir a nivel de paquete, como en el prototipo, y no en cada instancia.
6. **Registros de Enterprise.** Al arrancar, Enterprise escribe líneas sobre la licencia. Los registros no deben compartirse sin filtrar.

## Cómo reproducir

Ver `a2a-plugin/prototype/README.md`.
