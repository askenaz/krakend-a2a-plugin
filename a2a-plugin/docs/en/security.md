# Security model

[Documentation index](README.md) · [Leer en español](../seguridad.md)

KrakenD authenticates each request. The A2A plugin authorizes each operation using the identity that KrakenD verified and propagated with `propagate_claims`.

## Request flow

1. The A2A edge removes incoming reserved and identity headers, validates the JSON-RPC envelope, negotiates the version, and routes the operation to an internal endpoint.
2. The internal KrakenD endpoint validates the token, propagates claims, and applies configured native policies.
3. The `a2a-authz` modifier checks required identity, operation scopes, and ownership.
4. The agent receives the JSON-RPC message and verified identity headers. The client's token is withheld.
5. The edge records ownership before returning newly created resources, rewrites extended cards, and normalizes errors.

## Per-agent access and agent authentication

KrakenD decides which clients may use each agent, with the `auth/validator` on that agent's internal endpoints. The templates adjust it per agent with `gateway.auth`. KrakenD also controls how the gateway calls the agent, through the backend configuration, which the templates adjust per agent with `gateway.backend_extra_config`.

An agent can trust the identity headers only if it is reachable solely through the gateway. If the agent cannot be isolated on the network, make the gateway authenticate to it, for example with `auth/client-credentials` in `gateway.backend_extra_config`, and have the agent reject requests that lack that credential.

## Task and context ownership

The plugin records the principal that creates a task or context. Other principals receive the same `-32001 Task not found` response as for an absent task. Use the memory store for one node or Redis when nodes share ownership.

For tasks created before rollout, start with `ownership.unknown_task: passthrough` so existing tasks can finish. Switch to `deny` after those tasks complete or expire.

## Fail-closed behavior

Required identity missing from a request yields HTTP 401. Direct requests to internal routes are hidden. With `on_store_error: deny`, an ownership store failure stops the operation. If an endpoint lacks `a2a-authz` and its backend replies, the edge withholds the result and returns `-32603`, although the backend may already have acted. The supplied templates attach the modifier to every internal route.

## JSON and webhook validation

The gateway rejects duplicate keys, including case variations, in the envelope and fields used for authorization or webhook validation. Webhook validation covers both protocol versions. With defaults enabled, webhooks must use HTTPS and resolve to public endpoints. `allowed_hosts` can restrict hostnames further. Apply outbound network controls to the agent that makes the webhook request.

## Agent Cards

The gateway replaces agent URLs with public gateway URLs and applies its published security and capabilities. Upstream cards can be verified with trusted JWKS. Rewriting invalidates the original signature, so published cards omit it. Extended cards require an authenticated caller.

## Logs and deployment

Request logs record agent, operation, version, outcome, and a non-reversible principal digest. They omit request bodies, tokens, and raw subject IDs. Filter Enterprise startup logs before sharing because KrakenD may include license information. See the [compatibility matrix](compatibility.md) for the supported deployment environments.
