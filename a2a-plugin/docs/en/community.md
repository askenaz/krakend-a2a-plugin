# Install on KrakenD Community

[Documentation index](README.md) · [Leer en español](../community.md)

The plugin loads into the official KrakenD Community 2.13.11 binary.

## Community 2.13 security status

Community 3.x removed support for Go plugins. Community 2.13.11 is the final Community release that loads them. The 2.13 line no longer receives security fixes. Put a maintained load balancer or WAF in front of it and follow KrakenD's security notices. [Compatibility matrix](compatibility.md)

## Build

From `a2a-plugin`, build the image or export the plugin artifacts:

```bash
docker build -f build/community/Dockerfile -t krakend-a2a:ce-2.13.11 .
docker build -f build/community/Dockerfile --target artifacts -o type=local,dest=out/community .
```

The build tests against the same Go release as the Community binary. The target image verifies the manifest, runs `krakend check-plugin`, and loads both plugin symbols with `krakend test-plugin`. The artifact directory contains the `.so`, its manifest, and `check-artifact.sh`.

## Check before deployment

```bash
docker run --rm --entrypoint sh -v "$PWD/out/community:/p:ro" krakend:2.13.11 /p/check-artifact.sh /p/krakend-a2a-ce-2.13.11.so
```

The script reports any mismatch in edition, KrakenD version, Go, libc, architecture, or checksum.

## Configure

Define your [agent catalog](agents.md) and render it:

```bash
bash config/templates/render.sh community https://gw.example.com
```

For a hand-written configuration, use `version: 3`. Put `plugin` at the root and declare the edge in `extra_config["plugin/http-server"]` with `"name": ["a2a-gateway"]` and a key named `a2a-gateway`. Each internal endpoint declares `plugin/req-resp-modifier` with `"name": ["a2a-authz"]` and `"a2a-authz": {"agent": "<id>"}`. [Configuration reference](configuration.md)

## Run

```bash
docker run -p 8080:8080 -v "$PWD/out/rendered-community.json:/etc/krakend/krakend.json:ro" krakend-a2a:ce-2.13.11 run -c /etc/krakend/krakend.json
```

## Streaming and discovery

The plugin flushes each SSE write on Community, where the default `no-op` pipeline otherwise buffers the stream. Use the full Agent Card URL with the Go SDK resolver. [SDK example](enterprise.md#go-sdk-clients)
