#!/usr/bin/env bash
set -euo pipefail

if [[ ${1:-} == --help ]]; then
  cat <<'HELP'
Usage: bash scripts/w6-load.sh [new-compose-project] [run-id]
Runs the synthetic 1000-board paced scenario in a new local Compose project.
The script stops its own containers but preserves the PostgreSQL volume.
Ports 1883 and 8080 must be free. Inspect data before intentionally removing it.
Use a fresh run-id for each production run; reusing one with a new publisher
freezes the stored run as a conflict.
HELP
  exit 0
fi

project=${1:-w6-load-$(date +%s)-$$}
run_id=${2:-demo-run}
if [[ ! $project =~ ^[a-z0-9][a-z0-9_-]*$ ]]; then
  printf 'Invalid Compose project name: %s\n' "$project" >&2
  exit 2
fi
if [[ ! $run_id =~ ^[A-Za-z0-9][A-Za-z0-9_-]*$ ]]; then
  printf 'Invalid run ID: %s\n' "$run_id" >&2
  exit 2
fi
cd "$(dirname "$0")/.."
export W6_RUN_ID=$run_id
compose=(docker compose -p "$project" -f compose.w2.yml -f compose.w3.yml -f compose.w6.yml -f compose.w6-load.yml)
if [[ -n $("${compose[@]}" ps -aq --all) ]] || docker volume inspect "${project}_w6_postgres" >/dev/null 2>&1; then
  printf 'Compose project or volume already exists: %s\n' "$project" >&2
  exit 2
fi

started=0
cleanup() {
  if [[ $started == 1 ]]; then "${compose[@]}" stop || true; fi
  printf 'Preserved project data: %s_w6_postgres\n' "$project" >&2
}
trap cleanup EXIT
started=1
"${compose[@]}" up -d --build
container=$("${compose[@]}" ps -aq simulator)
result=$(docker wait "$container")
if [[ $result != 0 ]]; then printf 'Simulator failed: exit %s\n' "$result" >&2; exit 1; fi

completed=0
for ((i=0; i<120; i++)); do
  data=$(curl -fsS --max-time 3 "http://127.0.0.1:8080/api/runs/${run_id}/workorders/work-order-1" 2>/dev/null || true)
  if [[ -n $data ]] && printf '%s' "$data" | python3 -c 'import json,sys; d=json.load(sys.stdin); sys.exit(0 if d["progress"]["completed"] == 1000 else 1)' 2>/dev/null; then
    printf '%s\n' "$data"
    completed=1
    break
  fi
  sleep 1
done
if [[ $completed != 1 ]]; then printf 'Backend did not report 1000 completed within 120s; inspect logs and integrity.\n' >&2; exit 1; fi

"${compose[@]}" exec -T postgres psql -U smt_demo -d smt_twin -c "SELECT count(*) AS events, min(sequence) AS first_sequence, max(sequence) AS last_sequence, percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM ingested_at-emitted_at)*1000) AS p50_ms, percentile_cont(0.95) WITHIN GROUP (ORDER BY extract(epoch FROM ingested_at-emitted_at)*1000) AS p95_ms, percentile_cont(0.99) WITHIN GROUP (ORDER BY extract(epoch FROM ingested_at-emitted_at)*1000) AS p99_ms FROM events WHERE run_id='${run_id}'"
docker stats --no-stream "${project}-backend-1"
printf 'A matched count does not prove missing-tail coverage; inspect integrity and broker logs.\n'
