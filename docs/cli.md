# CLI

Requires Go 1.26.5 to build, Linux to benchmark, and fio **3.33**. Pinning the tested parser version avoids silently accepting changed tool semantics. Commands emit a schema-versioned JSON envelope and never prompt.

```sh
go build -o bin/malta ./cmd/malta
mkdir -p /tmp/malta-data
bin/malta plan --spec benchmarks/data/smoke-read.yaml \
  --environment environments/local.yaml --version environments/version-example.yaml \
  --target /tmp/malta-data --resource-group smoke --out /tmp/malta-plan.json
bin/malta exec --plan /tmp/malta-plan.json --out /tmp/malta-result \
  --fio fio --timeout 2m --max-bytes 1073741824 --non-interactive
```

`plan` validates and expands; it performs no benchmark I/O. `exec` creates a unique run/attempt directory beneath an existing target, prepares each trial's files, then runs timed measurement with optional fio ramp time. Preparation is excluded from the measured metrics. It retains data for inspection: `data-paths.json` identifies the directory to remove. The tool never removes pre-existing target data. Use a dedicated target and the same resource group for tests sharing a device; the local advisory lock is held throughout a run. Cross-host exclusion belongs to the later Kubernetes executor.

`bundle.json` contains the run and SHA-256-checked raw files. Other evidence includes plan, tool version, exact argument arrays, preparation/measurement JSON and stderr. File size and block size are distinct; mixed read/write metrics remain separate. Percentiles are per trial and cannot be averaged into a global percentile.

SIGINT/SIGTERM cancels the run and kills its process group. A total deadline includes preparation and warmup. Failed/cancelled runs retain available evidence. `cold-requested` requests fio invalidation; it does not prove a cold distributed cache. `cold-verified` is rejected by this executor. The default total prepared-data budget is 1 GiB; large lab profiles require an explicit budget.

Exit codes: 0 success, 2 invalid/unsupported input, 3 execution/storage/server error, 4 timeout, 5 cancellation, 6 conflict, 7 authentication. Test with `MALTA_TEST_FIO=/absolute/path/to/fio go test ./internal/runner`; this runs five small real workloads in temporary directories.

## Results and comparisons (S2)

```sh
bin/malta environment register --file environments/local.yaml
bin/malta version register --file environments/version-example.yaml
bin/malta result import --bundle /tmp/malta-result
bin/malta result list --kind data --limit 100
bin/malta baseline set --name release-reference --run RUN_ID
bin/malta baseline list
bin/malta compare --baseline RUN_ID --candidate OTHER_RUN_ID
bin/malta result export RUN_ID --out /tmp/malta-export
```

Local commands use `--data-dir .malta`. Snapshot IDs are immutable; create a new ID for changed hardware or build identity. Importing identical content is idempotent; conflicting content is rejected. Successful trial metrics are re-parsed from the attached raw files during import. Baseline records are append-only and always point to a specific successful run. Comparison uses median/min/max of independent repeats; latency is the median of trial P99 values, never a merged P99. A zero baseline yields a null percentage. Incompatible conditions produce explicit reasons and no percentage deltas. Unknown environment/cache fields and fewer than three repeats produce warnings.

Metadata imports support **mdtest 4.0.0**, one task, `-d PATH -n FILES -i 1 -F`, with no additional switches. Each raw file must be a complete independent iteration; the declared file count must match. Only create/stat/remove rates are imported. Latency was not collected and remains absent. The run date is the import date; original tool timestamps remain in raw evidence.

```sh
bin/malta plan --spec benchmarks/metadata/mdtest-files.yaml \
  --environment environments/local.yaml --version environments/version-example.yaml \
  --target /tmp/malta-data --out /tmp/mdtest-plan.json
bin/malta result import-mdtest --plan /tmp/mdtest-plan.json \
  --raw testdata/mdtest/mdtest-4.0.0.txt
```

The checked-in fixture is parser demonstration data, not a new experiment. For repeat counts greater than one, pass one actual output per repeat with comma-separated `--raw` paths.

## Managed service and Agent workflow (S3)

```sh
bin/malta serve --listen 127.0.0.1:8080 --data-dir /tmp/malta-store \
  --allow-target /tmp/malta-data --fio /path/to/fio --run-timeout 10m
bin/malta --server http://127.0.0.1:8080 run start \
  --plan /tmp/malta-plan.json --request-id experiment-001 --source agent
bin/malta --server http://127.0.0.1:8080 run wait RUN_ID --timeout 2m
bin/malta --server http://127.0.0.1:8080 run get RUN_ID
bin/malta --server http://127.0.0.1:8080 run logs RUN_ID --follow
bin/malta --server http://127.0.0.1:8080 run cancel RUN_ID
```

`--server` also routes registry, import/export, history, baseline and comparison commands through the API. Use `MALTA_TOKEN` for both server and client. Binding beyond loopback requires a token; remote deployments must provide TLS through a proxy or SSH tunnel. A browser cannot make cross-origin mutations. OpenAPI is in `api/openapi.yaml`.

A single queue serializes both local and Kubernetes runs. This conservative first implementation prevents concurrent tests on the server from interfering. A resource lock also excludes competing foreground runs using the same target and group. The server revalidates canonical target paths and byte/duration budgets. Use dedicated directories owned by the benchmark operator; concurrently replacing directories or mounts is outside the local executor's trust boundary.

The server durably records request IDs before queueing. Identical retries return the original run; a changed plan/source/executor conflicts. `wait` timeout or disconnection leaves the run running. `logs --follow` is NDJSON of complete, bounded snapshots (64 KiB per stderr file), not incremental log lines. Terminal execution can briefly have `result_state=collecting`; evidence is durable only at `complete`. Persistence failures remain visible as `failed`.

The data directory allows only one manager. On restart, unfinished local queued/running/collecting work becomes `interrupted`, with no automatic replay and no PID-based killing of unrelated processes. Graceful shutdown cancels the active process group; Linux children additionally receive a parent-death signal. Keep the service and fio in a process supervisor/cgroup for forced-stop containment. Raw work files are retained for interrupted-run diagnosis. New attempts require a new, intentionally chosen request ID.

Install or copy the scoped [malta-benchmark skill](../skills/malta-benchmark/SKILL.md) into your Agent's skill directory. It covers both executors and session recovery. Kubernetes configuration and deployment are described in [the deployment guide](../deploy/kubernetes/README.md).

## Discovery and request recovery (S6)

```sh
malta version
malta --server "$MALTA_SERVER" capabilities
malta --server "$MALTA_SERVER" run lookup --request-id experiment-001
python3 skills/malta-benchmark/scripts/session.py resume --session experiment.json
```

Capabilities reports the configured executors, byte/deadline limits and feature
availability. Without `--server` it describes offline CLI capabilities. Request
lookup is read-only and returns `not_found` for unknown requests. On a lost start
response, lookup first; retries use the same plan, source, executor and request ID.
Kubernetes server restart reconciles the original Job and does not replay I/O.
The skill session helper checkpoints before submission and locks its session file;
new conversations can resume without knowing any database paths or tables.
