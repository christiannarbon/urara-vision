#!/usr/bin/env bash
# Records the Python chat service's responses as golden files. No model is called.
# Needs the compose stack up; see README.md.
set -euo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../../../../.." && pwd)
GOLDEN="$HERE/golden"
CHAT=${CHAT_URL:-http://localhost:8090}
BACKEND=${BACKEND_URL:-http://localhost:8080}
AUTH="Authorization: Bearer ${BACKEND_API_TOKEN:-relviz-dev-token-not-for-production}"
ADMIN_USER=${BOOTSTRAP_ADMIN_USERNAME:-admin}
ADMIN_PASSWORD=${BOOTSTRAP_ADMIN_PASSWORD:-relviz-dev-admin-password}
LABEL=golden-capture
UNKNOWN_ID=00000000-0000-4000-8000-000000000000
COMPOSE=(docker compose -f "$ROOT/docker-compose.yml")

TMP=$(mktemp -d)

# Placeholders for values that change per run.
NORMALISE='walk(
  if type == "string" then
    gsub("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"; "<uuid>")
    | if test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}") then "<time>" else . end
  elif type == "object" then
    with_entries(
      if .key == "requestId" then .value = "<request-id>"
      elif (.key | test("latency|duration"; "i")) and (.value | type) == "number" then .value = "<number>"
      else . end)
  else . end)'

backend() { curl -sSf -H "$AUTH" -H 'Content-Type: application/json' "$@"; }

# The settings switch needs an admin session; the API token is not enough.
set_chat() {
  curl -sSf -b "$TMP/jar" -H 'X-Requested-With: urara' -H 'Content-Type: application/json' \
    -X PATCH -d "{\"chatEnabled\":$1}" "$BACKEND/api/v1/settings" >/dev/null
}

cleanup_snapshots() {
  backend "$BACKEND/api/v1/snapshots" |
    jq -r --arg l "$LABEL" '.snapshots[]? | select(.sourceLabel == $l) | .id' |
    while read -r sid; do backend -X DELETE "$BACKEND/api/v1/snapshots/$sid" >/dev/null || true; done
}

finish() {
  "${COMPOSE[@]}" start backend >/dev/null 2>&1 || true
  wait_for "$BACKEND/healthz" 200
  set_chat true || true
  cleanup_snapshots || true
  rm -rf "$TMP"
}

wait_for() { # url status [curl args...]
  local url=$1 want=$2; shift 2
  for _ in $(seq 60); do
    [ "$(curl -s -o /dev/null -w '%{http_code}' "$@" "$url")" = "$want" ] && return 0
    sleep 1
  done
  echo "timed out waiting for $want from $url" >&2
  return 1
}

# A version can be ingested once, so an existing one is reused rather than replaced.
ingest() {
  local dir="$ROOT/docs/demo/$1" status
  (cd "$dir" && find . -type f \( -name '*.md' -o -name '*.toml' \) ! -path ./README.md | sed 's|^\./||' | LC_ALL=C sort) |
    while read -r path; do jq -n --arg path "$path" --rawfile content "$dir/$path" '{$path, $content}'; done |
    jq -s --arg name "$1" --arg label "$LABEL" '{name: $name, sourceLabel: $label, files: .}' >"$TMP/ingest"
  status=$(curl -sS -o "$TMP/ingested" -w '%{http_code}' -H "$AUTH" -H 'Content-Type: application/json' \
    --data-binary @"$TMP/ingest" "$BACKEND/api/v1/ingest")
  case $status in
    201 | 200) jq -r .snapshot.id "$TMP/ingested" ;;
    409) backend "$BACKEND/api/v1/projects/$(jq -r .project "$TMP/ingested")/versions/$(jq -r .version "$TMP/ingested")" | jq -r .id ;;
    *) echo "ingest of $1 failed with $status" >&2; return 1 ;;
  esac
}

probe() { # file method path [curl args...]
  local out="$GOLDEN/$1" method=$2 path=$3; shift 3
  mkdir -p "$(dirname "$out")"
  local status
  status=$(curl -sS -o "$TMP/body" -D "$TMP/headers" -w '%{http_code}' -X "$method" "$@" "$CHAT$path")
  if ! jq -e . "$TMP/body" >/dev/null 2>&1; then echo null >"$TMP/body"; fi
  jq --argjson status "$status" \
    --argjson retry "$(grep -qi '^retry-after:' "$TMP/headers" && echo true || echo false)" \
    --argjson rid "$(grep -qi '^x-request-id:' "$TMP/headers" && echo true || echo false)" \
    "{status: \$status, headers: {\"Retry-After\": \$retry, \"X-Request-Id\": \$rid}, body: (. | $NORMALISE)}" \
    "$TMP/body" >"$out"
}

tool() { # file tool args
  probe "debug/$1" POST /debug/tool -H 'Content-Type: application/json' \
    -d "$(jq -nc --arg s "$JAFFLE" --arg t "$2" --argjson a "$3" '{snapshotId: $s, tool: $t, args: $a}')"
}

chat_py() { "${COMPOSE[@]}" exec -T chat python -c "$1"; }

trap finish EXIT

cp "$ROOT"/chat/tests/unit/fixtures/*.json "$HERE/fixtures/"
rm -rf "$GOLDEN"

curl -sSf -c "$TMP/jar" -H 'Content-Type: application/json' -H 'X-Requested-With: urara' \
  -d "$(jq -nc --arg u "$ADMIN_USER" --arg p "$ADMIN_PASSWORD" '{username: $u, password: $p}')" \
  "$BACKEND/api/v1/auth/login" >/dev/null
USER_ID=$(curl -sSf -b "$TMP/jar" -H 'X-Requested-With: urara' "$BACKEND/api/v1/auth/me" | jq -r .user.id)
set_chat true
cleanup_snapshots
JAFFLE=$(ingest jaffle-shop-ddd)
ingest sakila-oltp-ddd >/dev/null

AS_USER=(-H "X-User-Id: $USER_ID" -H 'Content-Type: application/json')
wait_for "$CHAT/api/chat/stats?snapshot=$JAFFLE" 200 "${AS_USER[@]}"

probe probes/healthz.json GET /healthz
probe probes/readyz.json GET /readyz

probe debug/tools.json GET /debug/tools
tool tool-list_domains.json list_domains '{}'
tool tool-list_tables.json list_tables '{"domain":"ordering"}'
tool tool-get_tables.json get_tables '{"ids":["ordering/fact_orders"]}'
tool tool-search_model.json search_model '{"query":"customer","limit":5}'
tool tool-get_neighbourhood.json get_neighbourhood '{"table_id":"ordering/fact_orders"}'
tool tool-find_join_paths.json find_join_paths \
  '{"from_table":"ordering/fact_order_items","to_table":"customer_identity/dim_customers"}'
tool tool-get_lineage.json get_lineage '{"table_id":"ordering/fact_orders"}'
tool tool-list_diagnostics.json list_diagnostics '{}'
tool tool-list_source_models.json list_source_models '{}'
tool tool-get_tables-missing.json get_tables '{"ids":["ordering/fact_orders","ordering/nope"]}'
tool tool-unknown.json nope '{}'
tool tool-bad-args.json search_model '{"query":"customer","limit":500}'

answer() { # file body
  probe "errors/$1" POST /api/chat/answer "${AS_USER[@]}" -d "$2"
}
probe errors/anonymous.json GET '/api/chat/conversations?snapshot=latest'
answer validation-missing.json '{}'
answer validation-extra.json "{\"snapshotId\":\"$JAFFLE\",\"question\":\"hi\",\"bogus\":1}"
answer question-empty.json "{\"snapshotId\":\"$JAFFLE\",\"question\":\"   \"}"
answer question-long.json "{\"snapshotId\":\"$JAFFLE\",\"question\":\"$(head -c 4001 /dev/zero | tr '\0' a)\"}"
head -c 1048577 /dev/zero | tr '\0' a >"$TMP/large"
probe errors/too-large.json POST /api/chat/answer "${AS_USER[@]}" --data-binary @"$TMP/large"
probe errors/conversation-unknown.json GET "/api/chat/conversations/$UNKNOWN_ID" "${AS_USER[@]}"
answer snapshot-unknown.json "{\"snapshotId\":\"$UNKNOWN_ID\",\"question\":\"hi\"}"

probe stats/empty.json GET "/api/chat/stats?snapshot=$JAFFLE" "${AS_USER[@]}"

probe conversations/create.json POST /api/chat/conversations "${AS_USER[@]}" \
  -d "{\"snapshotId\":\"$JAFFLE\",\"title\":\"golden\"}"
CID=$(curl -sSf "${AS_USER[@]}" "$CHAT/api/chat/conversations?snapshot=$JAFFLE" | jq -r '.conversations[0].id')
probe conversations/list.json GET "/api/chat/conversations?snapshot=$JAFFLE" "${AS_USER[@]}"
probe conversations/get.json GET "/api/chat/conversations/$CID" "${AS_USER[@]}"
probe conversations/delete.json DELETE "/api/chat/conversations/$CID" "${AS_USER[@]}"

# The context card and system prompts, rendered by the Python code itself.
mkdir -p "$GOLDEN/card" "$GOLDEN/prompt"
# Fixed but parseable, so Go can decode this file as well as compare against it.
backend "$BACKEND/api/v1/snapshots/$JAFFLE/context" |
  jq '.snapshot.id = "00000000-0000-0000-0000-000000000000" | .snapshot.createdAt = "2000-01-01T00:00:00Z"' \
    >"$HERE/fixtures/jaffle-context.json"
CARD='import sys
from urara_chat.agent.context_card import render_context_card
from urara_chat.backend.models import SnapshotContext
sys.stdout.write(render_context_card(SnapshotContext.model_validate_json(sys.stdin.read())))'
chat_py "$CARD" <"$HERE/fixtures/context.json" >"$GOLDEN/card/fixture.txt"
chat_py "$CARD" <"$HERE/fixtures/jaffle-context.json" >"$GOLDEN/card/jaffle.txt"
for lang in EN JA; do
  chat_py "import sys
from urara_chat.agent.prompts import build_system_prompt
sys.stdout.write(build_system_prompt(sys.stdin.read(), '$lang'))" \
    <"$GOLDEN/card/fixture.txt" >"$GOLDEN/prompt/$lang.txt"
done

# Chat off: the gate caches the backend's answer, so wait for it to expire.
set_chat false
wait_for "$CHAT/api/chat/stats?snapshot=$JAFFLE" 503 "${AS_USER[@]}"
probe errors/chat-off.json GET "/api/chat/conversations?snapshot=$JAFFLE" "${AS_USER[@]}"
set_chat true

"${COMPOSE[@]}" stop backend >/dev/null 2>&1
probe probes/readyz-backend-down.json GET /readyz
echo "captured $(find "$GOLDEN" -type f | wc -l | tr -d ' ') files under $GOLDEN"
