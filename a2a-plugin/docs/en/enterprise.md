# Install on KrakenD Enterprise

[Documentation index](README.md) · [Leer en español](../enterprise.md)

Build the plugin against the exact Enterprise release you deploy. The default target is 3.0.4.

## Build

From `a2a-plugin`:

```bash
docker build -f build/enterprise/Dockerfile -t krakend-a2a:ee-3.0.4 .
docker build -f build/enterprise/Dockerfile --target artifacts -o type=local,dest=out/enterprise .
```

The builder runs tests with the target release's Go compiler. The artifact directory contains the `.so`, a manifest with the edition, release, Go, libc, architecture and checksum, and `check-artifact.sh`. To build another verified version, pass the exact release in `--build-arg KRAKEND_VERSION=<version>`.

## Check before deployment

```bash
docker run --rm --entrypoint sh -v "$PWD/out/enterprise:/p:ro" krakend/krakend-ee:3.0.4 /p/check-artifact.sh /p/krakend-a2a-ee-3.0.4.so
```

For a plugin load test, provide the license from outside the repository:

```bash
KRAKEND_EE_LICENSE_FILE=/path/outside/repo/LICENSE bash build/enterprise/plugin-test.sh krakend-a2a:ee-3.0.4
```

## Configure

Define your [agent catalog](agents.md) and render it:

```bash
bash config/templates/render.sh enterprise https://gw.example.com
```

The output is `out/rendered-enterprise.json`. The Enterprise Compose example assembles the catalog and renders the template on startup.

For a hand-written configuration, use `version: 4`. Declare the plugin loader under root `extra_config`. Both `plugin/http-server` and `plugin/req-resp-modifier` use a `plugins` array with `name` and `config` per entry. The edge uses `a2a-gateway` and the modifier uses `a2a-authz` with `{"agent": "<id>"}`. [Configuration reference](configuration.md)

## Run

```bash
docker run -p 8080:8080 \
  -e KRAKEND_LICENSE_PATH=/run/krakend-license/LICENSE \
  -v "${KRAKEND_EE_LICENSE_FILE:?}:/run/krakend-license/LICENSE:ro" \
  -v "$PWD/out/rendered-enterprise.json:/etc/krakend/krakend.json:ro" \
  krakend-a2a:ee-3.0.4 run -c /etc/krakend/krakend.json
```

## Upgrade Enterprise

Build for the target release, check the artifact on that image, and verify plugin loading with the license mounted. Run the end-to-end tests against the target deployment before rolling it out. [Development guide](development.md)

## Go SDK clients

Pass the full Agent Card URL to the resolver:

```go
card, err := agentcard.DefaultResolver.Resolve(ctx, "https://gw.example.com/a2a/billing/.well-known/agent-card.json")
```

## Performance

The compatibility matrix reports results from a synthetic load test of the supplied demo topology. Measure deployment capacity with your own agents, policies, and traffic mix. [Compatibility matrix](compatibility.md)
