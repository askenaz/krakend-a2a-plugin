# Matriz de compatibilidad

Versión del plugin: **0.1.0**. Verificada el 13 de noviembre de 2026.

## KrakenD

| Edición | Versión verificada | Go | libc | Imagen de compilación | Estado de la línea |
|---|---|---|---|---|---|
| Community | 2.13.11 | 1.26.8 | MUSL-1.2.5 (Alpine 3.23.5) | `krakend/builder:2.13.11` | **Sin correcciones de seguridad desde el 8 de septiembre de 2026.** Community 3.x no carga plugins Go. |
| Enterprise | 3.0.4 | 1.27.4 | MUSL-1.2.5 (Alpine 3.23.8) | `krakend/builder-ee:3.0.4` | Con soporte. Evitar 3.0.0 a 3.0.3 por un fallo en los modificadores. |

Cada artefacto solo carga en la edición, versión, Go y libc exactos de su manifiesto. Las versiones de Go y libc de cada imagen de compilación coinciden con las del binario de destino.

## A2A

| Versión del protocolo | Binding | Estado |
|---|---|---|
| 1.0 | JSON-RPC | Completo: las once operaciones |
| 0.3 | JSON-RPC | Completo: los diez métodos de 0.3 |
| 1.0 y 0.3 | HTTP+JSON, gRPC | No soportados |

## Interoperabilidad verificada

| Componente | Versión |
|---|---|
| SDK oficial de Go, como cliente y como agente | `github.com/a2aproject/a2a-go/v2` v2.8.0 |
| Compatibilidad 0.3 del SDK | `a2acompat/a2av0` del mismo SDK |
| Firma de Agent Cards del SDK | `a2acrypto`, ES256 |
| Redis para la propiedad entre nodos | 7.4 |

## Pruebas que respaldan esta matriz

| Prueba | Community 2.13.11 | Enterprise 3.0.4 |
|---|---|---|
| Pruebas unitarias con el Go de la edición | Superadas | Superadas |
| `check-artifact.sh` y rechazo del artefacto de la otra edición | Superadas | Superadas |
| Comprobación de compatibilidad y carga de KrakenD | Superadas | Superadas, con licencia |
| Plantillas renderizadas con `krakend check` | Superadas | Superadas |
| Extremo a extremo con el SDK oficial, 1.0 y 0.3 | Superadas | Superadas |
| Control de acceso y propiedad entre dos nodos | Superadas | Superadas |
| Configuración inválida impide arrancar | Superada | Superada |
| Carga de 30 s, 16 clientes, dos nodos | 77 058 peticiones, todas correctas | 2 941 peticiones, todas correctas |

La diferencia de volumen en la prueba de carga no la causa el plugin: un Enterprise 3.0.4 sin el plugin rinde igual en la misma máquina. Ver `enterprise.md`.

## Pendiente de verificar

- **Carga de plugins sin el módulo AI Gateway (supuesto S7).** El plugin carga y funciona con la licencia disponible, pero no se comprobó qué módulos incluye esa licencia.
- **ARM64.** Solo se ha verificado amd64. Las imágenes de compilación admiten compilación cruzada.
