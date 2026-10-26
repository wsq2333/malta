#!/usr/bin/env bash
set -euo pipefail
# Functional integration only. Run from repository root with a new workspace.
bin=${MALTA_BIN:-./bin/malta}
fio=${MALTA_TEST_FIO:-fio}
work=${1:?provide a new absolute temporary workspace}
port=${MALTA_TEST_PORT:-19481}
mkdir "$work"
mkdir "$work/data"
"$bin" serve --listen "127.0.0.1:$port" --allow-target "$work/data" \
  --data-dir "$work/store" --fio "$fio" --run-timeout 1m >"$work/server.stdout" 2>"$work/server.stderr" &
server_pid=$!
trap 'kill -TERM "$server_pid" 2>/dev/null || true; wait "$server_pid" 2>/dev/null || true' EXIT
url="http://127.0.0.1:$port"
for _ in $(seq 1 100); do
  if curl -fsS "$url/api/v1/health" >"$work/health.json"; then break; fi
  sleep 0.1
done
curl -fsS "$url/api/v1/health" >/dev/null
"$bin" plan --spec benchmarks/data/smoke-read.yaml --environment environments/local.yaml \
  --version environments/version-example.yaml --target "$work/data" --out "$work/plan.json" >"$work/plan-envelope.json"
"$bin" --server "$url" run start --plan "$work/plan.json" --request-id smoke-001 --source agent >"$work/start.json"
run_id=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["data"]["id"])' "$work/start.json")
"$bin" --server "$url" run start --plan "$work/plan.json" --request-id smoke-001 --source agent >"$work/retry.json"
python3 - "$work/start.json" "$work/retry.json" <<'PY'
import json,sys
assert json.load(open(sys.argv[1]))['data']['id'] == json.load(open(sys.argv[2]))['data']['id']
PY
set +e
"$bin" --server "$url" run wait "$run_id" --timeout 100ms >"$work/wait-timeout.json"
status=$?
set -e
test "$status" -eq 4
"$bin" --server "$url" run wait "$run_id" --timeout 50s >"$work/completed.json"
"$bin" --server "$url" baseline set --name smoke-reference --run "$run_id" >"$work/baseline.json"
"$bin" --server "$url" compare --baseline "$run_id" --candidate "$run_id" >"$work/comparison.json"
"$bin" --server "$url" result export "$run_id" --out "$work/export" >"$work/export.json"
"$bin" --server "$url" run logs "$run_id" >"$work/logs.json"
"$bin" --server "$url" run start --plan "$work/plan.json" --request-id smoke-cancel --source agent >"$work/cancel-start.json"
cancel_id=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["data"]["id"])' "$work/cancel-start.json")
"$bin" --server "$url" run cancel "$cancel_id" >"$work/cancel.json"
set +e
"$bin" --server "$url" run wait "$cancel_id" --timeout 10s >"$work/cancel-wait.json"
status=$?
set -e
test "$status" -eq 5
python3 - "$work/comparison.json" "$work/completed.json" <<'PY'
import json,sys
c=json.load(open(sys.argv[1]))['data']
r=json.load(open(sys.argv[2]))['data']
assert c['comparable'] and len(c['metrics']) == 3
assert all(m['percent'] == 0 for m in c['metrics'])
assert r['attempt']['result_state'] == 'complete' and len(r['trials']) == 3
print('PASS: managed CLI, idempotency, wait timeout, three real repeats, baseline, export and cancellation')
PY
