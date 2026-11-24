# Technical validation · phase 0

[Documentation index](README.md) · [Leer en español](../validacion-supuestos.md)

The prototype used one plugin source for Community 2.13.11 and Enterprise 3.0.4. The probe ran the same requests against both gateways and a simulated agent.

## Verified assumptions

| Assumption | Community | Enterprise |
|---|---|---|
| The edge rewrites the path and the router honors it | Confirmed | Confirmed |
| The wrapped writer can flush SSE events incrementally | Confirmed | Confirmed |
| Both symbols from one plugin share process state | Confirmed | Confirmed |
| The request modifier sees propagated identity and can stop routing | Confirmed | Confirmed |
| One source works with both editions and their configuration shapes | Confirmed | Confirmed |
| The generated configuration passes `krakend check` | Confirmed | Confirmed |
| Plugin loading is independent of the AI Gateway license module | Not applicable | Module combinations unverified |

## Scenarios

The probe exercised valid requests, propagated identity, client-token removal, reserved headers, internal route hiding, missing authentication, task authorization, invalid JSON, streaming, a non-A2A route, and correlation cleanup.

## Streaming

| Edition | Event arrival with plugin flushing | Without plugin flushing |
|---|---|---|
| Community 2.13.11 | 0 s · 1.5 s · 3 s | 3 s · 3 s · 3 s |
| Enterprise 3.0.4 | 0 s · 1.4 s · 2.8 s | 0 s · 1.5 s · 3 s |

## Implementation implications

- Community provides the namespace map to the plugin, with settings under the plugin name. Enterprise provides the plugin's `config` directly.
- The wrong edition's `.so` causes a Go runtime error. Check the manifest before loading it.
- Enterprise requires a license for `krakend plugin test`. `krakend check` runs without that license.
- KrakenD instantiates the request modifier per endpoint. Edge correlation state is shared across instances.

To reproduce the prototype checks, follow `a2a-plugin/prototype/README.md` in the source checkout.
