# Development guide

[Documentation index](README.md) · [Leer en español](../desarrollo.md)

Use Docker and Go 1.26 or later for local work. Enterprise requires a runtime license for plugin loading. Its configuration check runs without a license.

## Target versions

`versions.env` pins the default KrakenD release for each edition. Build and render with the exact version to test:

```bash
KRAKEND_EE_VERSION=3.2.0 bash scripts/ci.sh
KRAKEND_EE_VERSION=3.2.0 bash config/templates/render.sh enterprise
```

An exact builder image must exist for the requested release. To adopt it, update `versions.env`, align the version references reported by `config/conformance`, run CI and end-to-end tests, and add a verified row to the [compatibility matrix](compatibility.md).

## License and demo credentials

Keep the Enterprise license outside the repository and mount it read-only at runtime. Compose takes its path from `KRAKEND_EE_LICENSE_FILE` and sets `KRAKEND_LICENSE_PATH` inside the container. Filter licensing lines from startup logs before sharing them.

The local Compose demo generates signing keys, trust material, and mock identity-provider client credentials under `deploy/.secrets/`. Its scripts and end-to-end tests read those local credentials. Set `A2A_CLIENTS_FILE` to point them at an alternative client file when needed.

## Test commands

Run these from `a2a-plugin`.

| Target | Command |
|---|---|
| Core and plugin adapter | `(cd core && go test ./...)` and `(cd plugin && go test ./...)` |
| Catalog assembly | `(cd config/catalog && go test ./...)` |
| Schema and conformance | `(cd config/conformance && go test ./...)` |
| Render Community and Enterprise | `bash config/templates/render.sh community` and `bash config/templates/render.sh enterprise` |
| Redis store | `A2A_TEST_REDIS=host:port go test -run TestRedisStore ./ownership/` from `core` |
| End to end | `A2A_EDITION=ce go test ./...` from `e2e` after starting the demo |
| Load | `bash e2e/load.sh ce 30s` against a running demo |
| Pydantic AI demo | `docker compose run --rm pruebas` from `demos/pydantic-ai` |
| Full CI | `bash scripts/ci.sh` |

The plugin adapter owns the edition-specific registration and wrapper interfaces. The core module uses the Go standard library. Static analysis runs against a local SonarQube instance using `SONAR_TOKEN` from the environment.

## Security analysis

Besides SonarQube, the compiled code (`core`, `plugin`, and `config/catalog`) and the images are reviewed with these tools. Versions are those of the latest review, on November 24, 2026. Run the commands from `a2a-plugin`.

| Tool | What it checks | Command |
|---|---|---|
| gosec 2.30.0 | Security mistakes in Go code | `go run github.com/securego/gosec/v2/cmd/gosec@v2.30.0 ./...` in `core`, `plugin`, and `config/catalog` |
| govulncheck 1.10.0 | Known vulnerabilities the code reaches, with each edition's Go | `docker run --rm -v "$PWD:/src:ro" -w /src/core --entrypoint sh krakend/builder:2.13.11 -c 'go run golang.org/x/vuln/cmd/govulncheck@v1.10.0 ./...'`, also in `plugin` and `config/catalog` and with `krakend/builder-ee:3.0.4` |
| Trivy 0.76.2 | Vulnerable dependencies, Dockerfiles, and secrets | `docker run --rm -v "$PWD:/src:ro" aquasec/trivy:0.76.2 fs --scanners vuln,misconfig,secret --skip-dirs /src/deploy/.secrets --skip-dirs /src/out /src` |
| Semgrep 1.185.0 | Go, OWASP Top 10, Dockerfile, and secrets rules | `docker run --rm -v "$PWD:/src:ro" -w /src semgrep/semgrep:1.185.0 semgrep scan --metrics=off --config p/golang --config p/owasp-top-ten --config p/dockerfile --config p/secrets --exclude .secrets --exclude out` |

None of them reports issues in the compiled code. The remaining findings are known:

- **govulncheck on Community.** It reports the Go 1.26.8 standard library vulnerabilities described in the [Community guide](community.md). In `config/catalog` it also flags GO-2026-6604, which only affects Windows; the catalog runs in the Linux image. Enterprise has none.
- **Trivy, `HEALTHCHECK`.** The images do not declare one. It depends on each deployment's port and configuration, so Compose or the orchestrator defines it.
- **Semgrep, servers without TLS.** They are the demo's mock IdP and agents, which only talk inside the Docker network.

## Documentation site

From `a2a-plugin/site`, run `npm ci`, `npm run build`, and `npm run preview`. Update the Spanish and English Markdown pages together. Publication uses the generated static files in `a2a-plugin/out/docs-site/`.
