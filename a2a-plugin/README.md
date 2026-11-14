# Plugin A2A para KrakenD

Soporte nativo del protocolo Agent2Agent (A2A) en KrakenD Community 2.13.11 y KrakenD Enterprise 3.0.4, con un único código fuente.

- **Descubrimiento.** Publica la Agent Card de cada agente reescrita para que solo anuncie el gateway, con caché, `ETag` y verificación opcional de firmas.
- **Pasarela de protocolo.** Valida JSON-RPC, negocia `A2A-Version`, entiende las once operaciones de A2A 1.0 y los métodos de 0.3, y encamina cada operación por el pipeline nativo de KrakenD, con streaming SSE incremental.
- **Control de acceso.** Reutiliza la autenticación de KrakenD, añade scopes por operación, registra quién es propietario de cada tarea y contexto, y bloquea webhooks hacia redes internas.

## Empezar

Demostración completa con Community, desde este directorio:

```bash
docker compose -f deploy/compose/community.yml up -d --build
```

```bash
bash deploy/demo.sh ce
```

El primer arranque genera las claves y los secretos de la demostración en `deploy/.secrets`, excluido de git. Para rotarlos, borra ese directorio y vuelve a levantar el entorno.

Con Enterprise, la licencia se monta desde fuera del repositorio:

```bash
KRAKEND_EE_LICENSE_FILE=/ruta/fuera/del/repo/LICENSE docker compose -f deploy/compose/enterprise.yml up -d --build
```

```bash
bash deploy/demo.sh ee
```

## Documentación

| Documento | Contenido |
|---|---|
| [docs/configuracion.md](docs/configuracion.md) | Referencia del bloque de configuración |
| [docs/seguridad.md](docs/seguridad.md) | Modelo de seguridad y límites |
| [docs/community.md](docs/community.md) | Instalación en Community 2.13 y aviso de soporte |
| [docs/enterprise.md](docs/enterprise.md) | Instalación en Enterprise 3.0 |
| [docs/compatibilidad.md](docs/compatibilidad.md) | Versiones verificadas |
| [docs/desarrollo.md](docs/desarrollo.md) | Desarrollo, pruebas y licencia de Enterprise |
| [docs/validacion-supuestos.md](docs/validacion-supuestos.md) | Resultados de la fase 0 |

## Estructura

| Ruta | Contenido |
|---|---|
| `core/` | Núcleo independiente de la edición, solo biblioteca estándar |
| `plugin/` | Adaptador de plugin único para ambas ediciones |
| `build/` | Construcción por edición y comprobación de artefactos |
| `config/` | Esquema JSON, ejemplos, plantillas y pruebas de conformidad |
| `deploy/` | IdP simulado, agente de referencia del SDK oficial, Compose y demostración |
| `e2e/` | Pruebas de extremo a extremo con el SDK oficial y prueba de carga |
| `prototype/` | Prototipo desechable de la fase 0 |
| `scripts/ci.sh` | Integración continua completa |
| `scripts/sonar.sh` | Análisis local con SonarQube |

El diseño y las especificaciones están en `openspec/changes/add-a2a-gateway-plugin/`, en la raíz del repositorio.
