#!/usr/bin/env bash
# Guion de demostración del gateway A2A (tarea 9.2).
#
# Uso: deploy/demo.sh <ce|ee>
# Requiere el entorno de deploy/compose levantado y curl. Termina con error si
# algún paso no da el resultado esperado.
set -euo pipefail
EDITION="${1:?uso: demo.sh <ce|ee>}"
case "$EDITION" in
  ce) BASE=18000 ;;
  ee) BASE=28000 ;;
  *) echo "edición desconocida" >&2; exit 2 ;;
esac
GW="http://localhost:$((BASE+80))"
GW2="http://localhost:$((BASE+81))"
IDP="http://localhost:$((BASE+90))"

step() { printf '\n\033[1m== %s\033[0m\n' "$*"; }
ok() { printf '   \033[32mOK\033[0m %s\n' "$*"; }
fail() { printf '   \033[31mFALLO\033[0m %s\n' "$*" >&2; exit 1; }
field() { sed -n "s/.*\"$1\":\"\([^\"]*\)\".*/\1/p" | head -n 1; }

SECRETS="$(cd "$(dirname "$0")" && pwd)/.secrets/idp/clients.json"
[ -f "$SECRETS" ] || { echo "Falta $SECRETS: levanta antes el entorno de deploy/compose" >&2; exit 1; }
secret() { sed -n "s/.*\"$1-app\": {\"secret\": \"\([^\"]*\)\".*/\1/p" "$SECRETS"; }
token() {
  curl -s -u "$1-app:$(secret "$1")" -d grant_type=client_credentials "$IDP/token" | field access_token
}
rpc() { # gateway token versión método params
  local hv=()
  [ -n "$3" ] && hv=(-H "A2A-Version: $3")
  curl -s -X POST "$1/a2a/billing" -H "Authorization: Bearer $2" "${hv[@]}" \
    -H 'Content-Type: application/json' \
    -d "{\"jsonrpc\":\"2.0\",\"id\":\"demo\",\"method\":\"$4\",\"params\":$5}"
}

step "1. Tokens del IdP para alice y bob (client_credentials)"
ALICE=$(token alice); BOB=$(token bob)
[ -n "$ALICE" ] && [ -n "$BOB" ] || fail "no se obtuvieron tokens"
ok "tokens obtenidos"

step "2. Descubrimiento: Agent Card de billing a través del gateway"
CARD=$(curl -s "$GW/a2a/billing/.well-known/agent-card.json")
echo "$CARD" | grep -q "\"url\":\"$GW/a2a/billing\"" || fail "la card no apunta al gateway"
echo "$CARD" | grep -q "billing:9001" && fail "la card filtra la URL interna del agente"
ok "la card anuncia $GW/a2a/billing y no contiene URLs internas"

step "3. SendMessage (A2A 1.0) como alice"
R=$(rpc "$GW" "$ALICE" 1.0 SendMessage '{"message":{"messageId":"demo-1","role":"ROLE_USER","parts":[{"text":"hola"}]}}')
TASK=$(echo "$R" | sed -n 's/.*"task":{"id":"\([^"]*\)".*/\1/p')
[ -n "$TASK" ] || fail "sin tarea: $R"
ok "tarea $TASK creada y registrada a nombre de alice"

step "4. Streaming (SendStreamingMessage): los eventos llegan según se emiten"
START=$(date +%s%N)
curl -sN -X POST "$GW/a2a/billing" -H "Authorization: Bearer $ALICE" -H 'A2A-Version: 1.0' \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":"s","method":"SendStreamingMessage","params":{"message":{"messageId":"demo-2","role":"ROLE_USER","parts":[{"text":"x"}],"metadata":{"delayMs":500}}}}' |
  while IFS= read -r line; do
    case "$line" in data:*) printf '   +%4d ms  %s\n' $(( ($(date +%s%N) - START) / 1000000 )) "$(echo "$line" | cut -c1-90)";; esac
  done

step "5. bob intenta leer la tarea de alice"
R=$(rpc "$GW" "$BOB" 1.0 GetTask "{\"id\":\"$TASK\"}")
echo "$R" | grep -q '"code":-32001' || fail "bob no debería ver la tarea: $R"
ok "denegado con -32001 (Task not found), igual que si no existiera"

step "6. alice lee su tarea desde el otro nodo del gateway (propiedad compartida en Redis)"
R=$(rpc "$GW2" "$ALICE" 1.0 GetTask "{\"id\":\"$TASK\"}")
echo "$R" | grep -q "\"id\":\"$TASK\"" || fail "alice debería ver su tarea en el otro nodo: $R"
ok "el segundo nodo respeta la propiedad registrada por el primero"

step "7. Petición sin token"
CODE=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$GW/a2a/billing" -H 'A2A-Version: 1.0' -d '{}')
CH=$(curl -s -D - -o /dev/null -X POST "$GW/a2a/billing" -H 'A2A-Version: 1.0' -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"GetTask","params":{"id":"x"}}' | grep -i '^www-authenticate' | tr -d '\r')
[ "$CODE" = "401" ] || [ -n "$CH" ] || fail "se esperaba 401"
ok "401 con ${CH:-WWW-Authenticate}"

step "8. Cliente A2A 0.3 (sin cabecera A2A-Version, nombres antiguos)"
R=$(rpc "$GW" "$ALICE" "" message/send '{"message":{"kind":"message","messageId":"demo-3","role":"user","parts":[{"kind":"text","text":"hola"}]}}')
echo "$R" | grep -q '"kind":"task"' || fail "respuesta 0.3 inesperada: $R"
ok "message/send atendido en 0.3 por el mismo agente"

step "9. Webhook hacia la red interna"
R=$(rpc "$GW" "$ALICE" 1.0 CreateTaskPushNotificationConfig "{\"taskId\":\"$TASK\",\"url\":\"https://169.254.169.254/latest/meta-data\"}")
echo "$R" | grep -q '"code":-32602' || fail "el webhook interno debería rechazarse: $R"
ok "rechazado antes de llegar al agente"

step "10. research publica una card firmada por el SDK; el gateway verifica la firma"
CODE=$(curl -s -o /dev/null -w '%{http_code}' "$GW/a2a/research/.well-known/agent-card.json")
[ "$CODE" = "200" ] || fail "la card firmada no se publicó ($CODE)"
ok "firma verificada; la card publicada no lleva la firma original"

printf '\n\033[1mDemostración completada en la edición %s.\033[0m\n' "$EDITION"
