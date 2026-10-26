---
name: malta-benchmark
description: Plan, submit, monitor and compare filesystem benchmarks using the Malta CLI. Use for Malta data I/O testing, version and hardware baseline comparisons, evidence inspection or cancellation. Supports the implemented local executor and mdtest result imports; Kubernetes execution is a later milestone.
---

# Malta benchmark management

Use the installed `malta` CLI with `--format json --non-interactive`. Read the CLI's `--help` when flags are uncertain. Every stdout record has `schema_version`, `data` or `error`. Put API credentials in `MALTA_TOKEN`, never in specs, result bundles or reports.

1. Establish the requested version, immutable environment snapshot, target filesystem, workload, resource group and byte/runtime budget from the user's instructions or existing configuration. Inspect `environment list/show`, `version list/show`, and `result list` before inventing new identities. Follow existing authorization; ask only for missing consequential parameters.
2. Run `malta plan --spec SPEC --environment ENV --version VERSION --target TARGET --resource-group GROUP --out PLAN`. Inspect the expanded plan and separate file size, block size, cache state, queue depth, jobs and repeat count. Planning performs no benchmark I/O. Do not label unknown hardware or unverified caches as verified.
3. For managed execution, use `malta --server URL run start --plan PLAN --request-id STABLE_ID --source agent --executor local`. Record the returned run ID. Retry a lost response with the **same** request ID and identical payload. A conflict requires inspection, not an automatically generated replacement ID.
4. Query `run get ID`, `run logs ID` or `run logs ID --follow`. The follow form emits NDJSON **snapshots**, not append-only log lines. `run wait ID --timeout 5m` only waits; exit code 4 does not stop or restart the benchmark. Resume querying the same run ID. Use `run cancel ID` when cancellation is requested or required by the agreed budget.
5. A succeeded run must have `attempt.result_state=complete` before treating its evidence as durable. `interrupted` runs are never blindly retried: inspect evidence, target state and the intended experiment first. `result export ID --out NEW_DIRECTORY` retrieves the portable bundle.
6. Set an explicit baseline with `baseline set --name NAME --run ID`, then `compare --baseline BASELINE_RUN --candidate CANDIDATE_RUN`. Report comparability reasons and warnings before percentages. Cite run IDs and raw artifacts. Distinguish median per-trial P99 from a pooled P99, and measured matrix maxima from an absolute system limit. Missing metadata latency is uncollected, not zero.

For offline execution, `exec --plan PLAN --out NEW_DIRECTORY --fio PATH --timeout DURATION --max-bytes BYTES` runs in the foreground; `result import --bundle DIRECTORY` stores it. For metadata, use `result import-mdtest --plan PLAN --raw FILES` only for supported mdtest 4.0.0 one-task, one-iteration `-d PATH -n FILES -i 1 -F` outputs, one real output per repeat.

Exit codes: 0 success; 2 invalid/unsupported; 3 execution/storage/server failure; 4 timeout; 5 cancelled; 6 conflict; 7 unauthorized. Never interpret command failure as a measured zero. The server has one local worker; data is retained in unique directories recorded by `data-paths.json`. The Kubernetes executor is not implemented in S0–S4.
