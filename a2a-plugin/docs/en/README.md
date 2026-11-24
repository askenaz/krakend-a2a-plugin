# A2A plugin for KrakenD

[Español](../README.md)

Publish Agent2Agent agents behind KrakenD with discovery, authentication, operation-level access control, and streaming.

## Documentation index

| Topic | Guides |
|---|---|
| Installation | [Community](community.md) · [Enterprise](enterprise.md) |
| Configuration | [Agent catalog](agents.md) · [Field reference](configuration.md) |
| Operations | [Security](security.md) · [Compatibility](compatibility.md) |
| Contributing | [Development and testing](development.md) · [Technical validation](validation.md) |

## Get started

1. Build the image for [Community](community.md) or [Enterprise](enterprise.md).
2. Register agents in the [agent catalog](agents.md), using a file or directory for each agent.
3. Render the configuration, start the gateway, and request the Agent Card.

```bash
curl https://gw.example.com/a2a/billing/.well-known/agent-card.json
```

The example uses a fictional agent named `billing`. Use your gateway URL and configured agent ID. The published card announces the gateway's public endpoint.

## Send a message

This example uses A2A 1.0 and a token from the identity provider configured in KrakenD.

```bash
curl -X POST https://gw.example.com/a2a/billing \
  -H "Authorization: Bearer ${TOKEN}" \
  -H 'A2A-Version: 1.0' -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"SendMessage","params":{"message":{"messageId":"example-1","role":"ROLE_USER","parts":[{"text":"Hello"}]}}}'
```

Use `SendStreamingMessage` and `curl -N` to receive SSE events. A2A 0.3 clients use that version's methods, such as `message/send`. See the [compatibility matrix](compatibility.md) for verified versions.

## Security and configuration

KrakenD validates the token and propagates identity. The plugin enforces operation scopes and controls task and context ownership. See the [security model](security.md) and [configuration reference](configuration.md).
