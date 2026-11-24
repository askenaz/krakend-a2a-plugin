# Plugin A2A para KrakenD

[English](en/README.md)

Publica agentes Agent2Agent detrás de KrakenD con descubrimiento, autenticación, control de acceso por operación y streaming.

## Índice de guías

| Tema | Guías |
|---|---|
| Instalación | [Community](community.md) · [Enterprise](enterprise.md) |
| Configuración | [Catálogo de agentes](agentes.md) · [Referencia de campos](configuracion.md) |
| Operación | [Seguridad](seguridad.md) · [Compatibilidad](compatibilidad.md) |
| Contribución | [Desarrollo y pruebas](desarrollo.md) · [Validación técnica](validacion-supuestos.md) |

## Empezar

1. Construye la imagen para [Community](community.md) o [Enterprise](enterprise.md).
2. Registra los agentes en el [catálogo de agentes](agentes.md), con un archivo o una carpeta por agente.
3. Renderiza la configuración, arranca el gateway y prueba la Agent Card.

```bash
curl https://gw.example.com/a2a/billing/.well-known/agent-card.json
```

El ejemplo usa el agente ficticio `billing`. Usa la URL de tu gateway y el ID del agente que hayas configurado. La card publicada anuncia el endpoint público del gateway.

## Enviar un mensaje

El ejemplo usa A2A 1.0 y un token emitido por el proveedor de identidad que valida KrakenD.

```bash
curl -X POST https://gw.example.com/a2a/billing \
  -H "Authorization: Bearer ${TOKEN}" \
  -H 'A2A-Version: 1.0' -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"SendMessage","params":{"message":{"messageId":"example-1","role":"ROLE_USER","parts":[{"text":"Hola"}]}}}'
```

Usa `SendStreamingMessage` y `curl -N` para recibir eventos SSE. Los clientes 0.3 usan los métodos de esa versión, como `message/send`. Consulta la [matriz de compatibilidad](compatibilidad.md) para conocer las versiones verificadas.

## Seguridad y configuración

KrakenD valida el token y propaga la identidad. El plugin aplica scopes por operación y controla la propiedad de tareas y contextos. Consulta el [modelo de seguridad](seguridad.md) y la [referencia de configuración](configuracion.md).
