# A2A plugin configuration

[Documentation index](README.md) · [Leer en español](../configuracion.md)

The templates combine shared settings and the [agent catalog](agents.md) into the JSON block read by the plugin. The block has the same structure in both editions.

## Placement in KrakenD

| Edition | Root version | Plugin block |
|---|---|---|
| Community 2.13 | `3` | `extra_config["plugin/http-server"]["a2a-gateway"]` |
| Enterprise 3.0 | `4` | The `config` of the `a2a-gateway` entry in `extra_config["plugin/http-server"].plugins` |

The JSON schema is `a2a-plugin/config/schema/a2a-gateway.schema.json`. The example below shows the plugin block. The gateway also needs internal routes, which the templates generate from each agent's `gateway` settings.

```json
{
  "public_url": "https://gw.example.com",
  "agents": [
    {
      "id": "billing",
      "card": { "source": "upstream", "url": "https://billing.example.com/.well-known/agent-card.json" }
    }
  ]
}
```

## Root fields

| Field | Default | Purpose |
|---|---|---|
| `public_url` | Required | Public origin of the gateway, including scheme and optional port. |
| `base_path` | `/a2a` | Public path prefix for each agent. |
| `internal_prefix` | `/__a2a` | Internal route prefix. Must be disjoint from the public prefix. |
| `route_granularity` | `class` | `class` routes to `unary` and `stream`. `operation` uses a route per operation. |
| `default_agent` | None | Agent whose card is also served at `/.well-known/agent-card.json`. |
| `limits` | See below | Body and response inspection limits. |
| `identity` | See below | Caller identity propagation. |
| `authorization` | See below | Required scopes per operation. |
| `ownership` | See below | Task and context ownership. |
| `push_notifications` | See below | Webhook validation. |
| `challenge` | See below | Authentication challenge and protected resource metadata. |
| `agents` | Required | At least one agent definition. |

## Limits, identity, and authorization

| Field | Default | Purpose |
|---|---|---|
| `limits.max_body_bytes` | 4194304 | Maximum request body. Range 1 KiB–64 MiB. Larger requests get 413. |
| `limits.max_inspect_bytes` | 8388608 | Maximum response size inspected for ownership. Range 1 KiB–256 MiB. |
| `identity.required` | `true` | Requires propagated identity. Missing identity yields 401. |
| `identity.headers` | `issuer: X-A2A-Iss`, `subject: X-A2A-Sub`, `scopes: X-A2A-Scope` | Maps claim names to headers. An explicit object replaces these defaults. The subject is required. Scopes are space-separated. |
| `identity.owner_scope` | `issuer` and `subject` where present | Claim names that form the ownership key. |
| `authorization.operation_scopes` | Empty | Required scopes keyed by the A2A 1.0 operation name. All listed scopes are required. |

Use distinct identity headers and avoid standard HTTP, A2A protocol, and plugin headers. Include a `scopes` header and require identity when enabling operation scopes. `owner_scope` references identity header keys and excludes `scopes`.

## Ownership and Redis

| Field | Default | Purpose |
|---|---|---|
| `ownership.enabled` | `true` | Requires verified identity. |
| `ownership.store` | `memory` | Use `redis` to share ownership across nodes. |
| `ownership.ttl` | `168h` | Expiration since last use, between one minute and one year. |
| `ownership.unknown_task` | `deny` | Can be `passthrough` during a migration. |
| `ownership.list_tasks` | `owned_context` | Requires a context owned by the caller. Also supports `deny` and `passthrough`. |
| `ownership.on_store_error` | `deny` | Policy when the store is unavailable. Also supports `passthrough`. |
| `ownership.memory.max_entries` | 100000 | Maximum number of records in memory. |
| `ownership.redis.address` | Required for Redis | Server address, including port. |
| `ownership.redis.db` | 0 | Redis database number. |
| `ownership.redis.tls` | `false` | Uses TLS for Redis. |
| `ownership.redis.username_env` | None | Name of the environment variable holding the username. |
| `ownership.redis.password_env` | None | Name of the environment variable holding the password. |
| `ownership.redis.timeout` | `2s` | Per-operation timeout, between 100 ms and 30 s. |

## Webhooks and authentication challenges

| Field | Default | Purpose |
|---|---|---|
| `push_notifications.require_https` | `true` | Requires HTTPS webhook URLs. |
| `push_notifications.deny_private_networks` | `true` | Rejects private and reserved destinations, including after DNS resolution. |
| `push_notifications.allowed_hosts` | Empty | Optional hostname allowlist with lowercase names and optional leading `*.`. |
| `challenge.scheme` | `Bearer` | Authentication scheme in `WWW-Authenticate`. |
| `challenge.realm` | None | Realm in the challenge. |
| `challenge.resource_metadata` | `false` | Publishes per-agent protected resource metadata (RFC 9728). |
| `challenge.authorization_servers` | Empty | Authorization server URLs. Required when resource metadata is enabled. |
| `challenge.scopes_supported` | Empty | Scopes advertised in resource metadata. |

## Agent fields

| Field | Default | Purpose |
|---|---|---|
| `id` | Required | Unique lowercase agent ID, up to 63 letters, digits, and hyphens. |
| `versions` | `["1.0"]` | Enables A2A 1.0 or 0.3. Requests without `A2A-Version` use 0.3. |
| `card` | Required | Agent Card source and cache settings. |
| `security` | None | Gateway security schemes advertised in the card. |
| `capabilities.streaming` | `true` | Enables streaming and task subscriptions. |
| `capabilities.push_notifications` | `false` | Enables push notification configuration. |
| `disabled_operations` | Empty | Rejects listed A2A 1.0 operation names for this agent. |
| `ownership.enabled`, `ownership.unknown_task`, `ownership.list_tasks` | Inherit | Overrides global ownership policies for this agent. |

The gateway advertises a capability when the agent declares it and the gateway enables it.

### Agent Card

| Field inside `card` | Default | Purpose |
|---|---|---|
| `source` | Required | `static` or `upstream`. |
| `inline` | None | Inline static card. Use either this field or `file` for static cards. |
| `file` | None | Path to a mounted static card. |
| `url` | Required for upstream | Agent discovery URL. |
| `ttl` | `5m` | How long to reuse a cached upstream card. |
| `max_stale` | `1h` | How long the last valid card may be served after failures. Must be at least `ttl`. |
| `max_bytes` | 1048576 | Maximum size of an upstream card. |
| `http_max_age` | `5m` | Client cache lifetime. Use `0s` to disable caching. |
| `signature.verify` | `false` | Requires a valid signature on the upstream card. |
| `signature.jwks_file` | Required when verifying | Mounted JWKS containing trusted keys. |

### Published security schemes

`security.schemes` maps names to A2A 1.0 security schemes. Each entry contains exactly one of `apiKeySecurityScheme`, `httpAuthSecurityScheme`, `oauth2SecurityScheme`, `openIdConnectSecurityScheme`, or `mtlsSecurityScheme`. `security.requirements` is an optional list whose `schemes` objects refer to named schemes. The plugin translates these to the Agent Card's protocol version.

## Validation

The plugin rejects invalid configuration at startup. It reports unknown fields, nulls, invalid ranges, and conflicting settings. Agent IDs must be unique and `default_agent` must exist. Public and internal prefixes must be disjoint and cannot contain dot segments. Ownership and operation scopes require verified identity. Write durations in Go format, such as `500ms` or `168h`.

Startup warnings flag plain HTTP public URLs, disabled identity checks, in-memory ownership, passthrough policies, and relaxed webhook settings. Run the configuration schema and conformance tests from the repository:

```bash
cd a2a-plugin/config/conformance && go test ./...
```
