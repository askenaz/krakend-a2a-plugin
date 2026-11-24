# Compatibility matrix

[Documentation index](README.md) · [Leer en español](../compatibilidad.md)

The plugin is compiled per KrakenD edition and exact release. The artifact manifest records the edition, release, Go version, libc, architecture, and checksum.

## KrakenD

| Edition | Verified release | Go | libc | Builder |
|---|---|---|---|---|
| Community | 2.13.11 | 1.26.8 | musl 1.2.5 (Alpine 3.23.5) | `krakend/builder:2.13.11` |
| Enterprise | 3.0.4 | 1.27.4 | musl 1.2.5 (Alpine 3.23.8) | `krakend/builder-ee:3.0.4` |
| Enterprise | 3.0.5 | 1.27.4 | musl 1.2.5 (Alpine 3.23.8) | Build with `KRAKEND_EE_VERSION=3.0.5` |

Community 3.x removed Go plugin support. Community 2.13 has stopped receiving security fixes. Enterprise 3.0.0 through 3.0.3 have a request modifier issue.

## A2A and interoperability

| Component | Verified support |
|---|---|
| A2A 1.0 | All eleven JSON-RPC operations |
| A2A 0.3 | All ten JSON-RPC methods |
| HTTP+JSON and gRPC bindings | Unsupported |
| Official Go SDK | v2.8.0, including its 0.3 compatibility package |
| Agent Card signing | ES256 with the Go SDK |
| Shared task ownership | Redis 7.4 |

## Verification

The project checks version and artifact compatibility, plugin loading, rendered templates, invalid startup configuration, end-to-end requests in both protocol versions, and ownership across two nodes. The synthetic 30-second load runs used 16 clients and two gateway nodes.

| Release | Successful requests |
|---|---:|
| Community 2.13.11 | 77,058 |
| Enterprise 3.0.4 | 2,941 |
| Enterprise 3.0.5 | 4,052 |

## Verification gaps

The tested architecture is amd64. The license module combinations needed for Enterprise plugin loading have yet to be isolated. See [technical validation](validation.md) for the prototype results.
