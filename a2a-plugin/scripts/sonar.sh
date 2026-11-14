#!/usr/bin/env bash
# Análisis local con SonarQube.
#
# Uso:
#   SONAR_TOKEN=<token> a2a-plugin/scripts/sonar.sh [contenedor de SonarQube]
#
# Genera la cobertura de Go y ejecuta el escáner en Docker, en la misma red que
# el contenedor de SonarQube (por defecto, a2a-sonarqube). El token nunca se
# escribe en el repositorio.
set -euo pipefail
: "${SONAR_TOKEN:?Define SONAR_TOKEN con un token de análisis de tu SonarQube local}"
SERVER="${1:-a2a-sonarqube}"
REPO=$(cd "$(dirname "$0")/../.." && pwd)
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) export MSYS_NO_PATHCONV=1; HREPO=$(cygpath -w "$REPO") ;;
  *) HREPO=$REPO ;;
esac

mkdir -p "$REPO/a2a-plugin/out/coverage"
(cd "$REPO/a2a-plugin/core" && go test -count=1 -coverprofile=../out/coverage/core.out ./... >/dev/null)
(cd "$REPO/a2a-plugin/plugin" && go test -count=1 -coverprofile=../out/coverage/plugin.out ./... >/dev/null)

docker network inspect sonar-net >/dev/null 2>&1 || docker network create sonar-net >/dev/null
docker network connect sonar-net "$SERVER" 2>/dev/null || true

docker run --rm --network sonar-net \
  -e SONAR_HOST_URL="http://$SERVER:9000" -e SONAR_TOKEN \
  -v "$HREPO:/usr/src" \
  sonarsource/sonar-scanner-cli:latest -Dsonar.qualitygate.wait=true
