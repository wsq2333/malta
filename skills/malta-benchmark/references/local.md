# Local workflow

Requires a pre-authorized existing target, configured server URL, spec/environment/
version files and a budget. The server must advertise `local` in capabilities.
A 3 × 8 MiB, one-second smoke fixture is shipped in `benchmarks/data/smoke-read.yaml`;
it is not a production performance baseline.

```sh
malta --server "$MALTA_SERVER" capabilities
malta plan --spec benchmarks/data/smoke-read.yaml --environment ENV.json \
  --version VERSION.json --target /approved/benchmark --out plan.json
python3 skills/malta-benchmark/scripts/session.py start \
  --session experiment.json --server "$MALTA_SERVER" --plan plan.json \
  --request-id experiment-001 --executor local
# A later agent conversation needs only the CLI, skill and this session file.
python3 skills/malta-benchmark/scripts/session.py resume --session experiment.json
malta --server "$MALTA_SERVER" run wait RUN_ID --timeout 2m
malta --server "$MALTA_SERVER" result export RUN_ID --out evidence-001
malta --server "$MALTA_SERVER" baseline set --name first-reference --run RUN_ID
python3 skills/malta-benchmark/scripts/session.py compare \
  --session candidate.json --baseline BASELINE_RUN_ID
```

Foreground/offline execution uses `malta exec --plan PLAN --out NEW_DIRECTORY
--fio FIO --timeout 1m --max-bytes BYTES`, then `result import --bundle DIRECTORY`.
Do not assume a managed run can be resumed with `exec`: that would start new I/O.
