# Agent catalog

[Documentation index](README.md) · [Leer en español](../agentes.md)

Keep shared settings separate from individual agent definitions. The same catalog generates the A2A block and internal routes in KrakenD Community and Enterprise.

## File layout

```text
settings/
  a2a.json
  agents/
    research.json
    billing/
      agent.json
      gateway.json
      security.json
```

`a2a.json` holds the service, JWT validator, claims, routes, and global plugin settings. In `agents/`, use an `<id>.json` file for a complete definition or an `<id>/` directory with a required `agent.json` manifest. The declared `id` must match the file or directory name.

## Split an agent into sections

`agents/billing/agent.json` holds the main agent fields.

```json
{
  "id": "billing",
  "versions": ["1.0"],
  "card": {
    "source": "upstream",
    "url": "https://billing.example.com/.well-known/agent-card.json"
  }
}
```

Each additional file contains a section's value. For example, `agents/billing/gateway.json` holds the backend object with its host, path, and timeouts.

```json
{
  "host": "https://billing.example.com",
  "path": "/",
  "timeout": "10s",
  "stream_timeout": "120s"
}
```

Only the templates use `gateway`; it never reaches the plugin. It accepts these fields:

| Field | Purpose |
|---|---|
| `host`, `path` | Agent address |
| `timeout`, `stream_timeout` | Timeouts of the `unary` and `stream` internal endpoints |
| `extra_config` | KrakenD components on the agent's internal endpoints, such as a rate limit |
| `auth` | `auth/validator` keys that replace those in `a2a.json` for this agent only. Use it to require scopes or roles per agent |
| `backend_extra_config` | KrakenD components on the call to the agent. Use it so the gateway authenticates to the agent, with `auth/client-credentials` or a fixed `modifier/martian` header |

For example, to let only clients with `a2a.write` use an agent, and have the gateway present its own OAuth token to it:

```json
{
  "host": "https://research.example.com",
  "path": "/",
  "timeout": "10s",
  "stream_timeout": "120s",
  "auth": { "scopes_key": "scope", "scopes": ["a2a.write"], "scopes_matcher": "all" },
  "backend_extra_config": {
    "auth/client-credentials": {
      "client_id": "gateway",
      "client_secret": "<secret>",
      "token_url": "https://idp.example.com/token"
    }
  }
}
```

Both credential forms work in both editions, including streaming. The agent receives the gateway's credential, never the client's token.

You can split out `card`, `versions`, `security`, `capabilities`, `disabled_operations`, `ownership`, and `gateway`. If a section appears twice, the assembler reports both files and stops. Each agent needs the applicable configuration fields and a `gateway` section for the templates to generate its backend. [Configuration reference](configuration.md)

For a static Agent Card, store the document in a subdirectory such as `assets/`, mount that directory in the container, and set `card.file` to the mounted path. The `card.json` section file contains the `source` and `file` options. KrakenD reads the Agent Card and the JWKS referenced by `card.signature.jwks_file` from its filesystem.

## Render and deploy

From `a2a-plugin`, run the command for your edition. The optional third argument selects your own `settings` directory.

```bash
bash config/templates/render.sh community https://gw.example.com ./config/templates/settings
bash config/templates/render.sh enterprise https://gw.example.com ./config/templates/settings
```

The commands write `out/rendered-community.json` and `out/rendered-enterprise.json`. Deploy the file for your edition. The Compose examples assemble and render on startup using `/usr/local/bin/a2a-config run -c /etc/krakend/krakend.tmpl`. To register an agent, add its file or directory and redeploy.

## Migrate from the inline list

The `agents` array in `a2a.json` remains supported. Move agents to `agents/` one at a time and remove each original entry as you move it. The assembler rejects duplicate IDs across both sources.
