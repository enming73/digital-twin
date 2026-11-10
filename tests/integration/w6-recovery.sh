#!/usr/bin/env bash
# W6 offline-tail acceptance: the simulator completes while the backend has
# never run, then the backend must recover the whole run from the durable
# source. Isolated project name, unique volumes; volumes are NOT deleted.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
cd "$ROOT"
PROJECT=${W6_PROJECT:-w6-e2e-$(date +%s)}
FILES=${W6_COMPOSE_FILES:-compose.w2.yml,compose.w3.yml,compose.w6.yml,compose.w6-recovery.yml}
read -r -a FILES <<<"${FILES//,/ }"
COMPOSE=(docker compose -p "$PROJECT")
for file in "${FILES[@]}"; do COMPOSE+=(-f "$file"); done
BOARDS=${W6_BOARDS:-100}
TIMEOUT=${W6_TIMEOUT:-300}

say() { printf '\n[w6] %s\n' "$*"; }
fail() { printf '\n[w6] FAIL: %s\n' "$*" >&2; exit 1; }

trap 'printf "\n[w6] stopping project %s (volumes retained)\n" "$PROJECT"; "${COMPOSE[@]}" stop >/dev/null 2>&1 || true' EXIT

say "project $PROJECT"
"${COMPOSE[@]}" up -d --build postgres mosquitto

if [ "${W6_LIVE:-0}" = "1" ]; then
  say "starting every service together (live burst mode)"
  "${COMPOSE[@]}" up -d --build
else
  say "starting simulator without a backend (no-deps)"
  "${COMPOSE[@]}" up -d --no-deps simulator
fi

say "waiting for the simulator terminal boundary"
deadline=$((SECONDS + TIMEOUT))
while :; do
  manifest=$("${COMPOSE[@]}" exec -T simulator wget -qO- http://127.0.0.1:8081/v1/runs/demo-run/manifest 2>/dev/null || true)
  if printf '%s' "$manifest" | grep -q '"terminal":{'; then break; fi
  [ $SECONDS -ge $deadline ] && fail "simulator did not reach a terminal boundary"
  sleep 2
done
head=$(printf '%s' "$manifest" | python3 -c 'import json,sys;print(json.load(sys.stdin)["head"])')
event_count=$(printf '%s' "$manifest" | python3 -c 'import json,sys;print(json.load(sys.stdin)["terminal"]["eventCount"])')
say "source terminal head=$head eventCount=$event_count"

if [ "${W6_LIVE:-0}" = "1" ]; then
  say "backend was online throughout; waiting for verified recovery"
else
  say "starting backend after completion"
  "${COMPOSE[@]}" up -d backend
fi

say "waiting for verified recovery"
deadline=$((SECONDS + TIMEOUT))
while :; do
  snapshot=$(curl -sf http://127.0.0.1:8080/api/runs/demo-run/snapshot || true)
  if printf '%s' "$snapshot" | python3 -c '
import json,sys
s=json.load(sys.stdin)
assert s["integrity"]=="complete" and s.get("terminalVerified") is True
assert s["lastSequence"]==int(sys.argv[1])
' "$head" 2>/dev/null; then break; fi
  [ $SECONDS -ge $deadline ] && fail "backend did not reach a verified complete projection"
  sleep 2
done

say "checking unique boards and work order totals"
printf '%s' "$snapshot" | python3 -c '
import json,sys
s=json.load(sys.stdin)
boards=s["boards"]
want=int(sys.argv[1])
assert len(boards)==want, len(boards)
assert all(b["status"]=="completed" for b in boards.values())
order=s["workOrders"]["work-order-1"]
assert order["status"]=="completed" and order["quantity"]==want
assert order["passed"]+order["failed"]==want
print("boards=%d passed=%d failed=%d" % (len(boards), order["passed"], order["failed"]))
' "$BOARDS"

say "checking PostgreSQL log completeness"
counts=$("${COMPOSE[@]}" exec -T postgres psql -U smt_demo -d smt_twin -tAc \
  "select count(*), min(sequence), max(sequence), count(distinct event_id) from events where run_id='demo-run'")
IFS='|' read -r rows first last distinct <<<"$counts"
[ "$rows" = "$head" ] || fail "database rows $rows != source head $head"
[ "$first" = "1" ] && [ "$last" = "$head" ] && [ "$distinct" = "$head" ] || fail "sequence/identity mismatch: $counts"
say "database rows=$rows sequences=1..$last distinct=$distinct"

say "PASS: offline tail recovered and verified (project $PROJECT)"
