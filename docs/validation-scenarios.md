# Mixed scenario acceptance — 2026-10-26

Implemented in separate commits for definition/planning, fio execution, managed storage/UI, weighted native I/O, and metadata/pipeline execution. The final follow-up covers preparation-state reporting, source-template JSON Schema validation, UI polish, and this acceptance record.

## Verified

- Local `go test -race ./...` and `go vet ./...` passed. The legacy 1.0 fixture plan retains its exact SHA-256. Scenario source YAML validates against the generated scenario schema without redundant workload-level timing fields.
- Frontend tests, TypeScript checks and production build passed. The existing ECharts chunk-size advisory remains; it is not a build failure.
- On the designated Linux amd64 development host, real fio 3.33 sequential and parallel scenarios passed, including per-child raw option checks and active IOPS-window overlap.
- The managed CLI/API acceptance script passed all five profiles: sequential, parallel, weighted data, weighted data/stat, and pipeline. It verifies idempotent requests, durable results, export/import roundtrips, baseline comparison, submission ratios and cancellation with no remaining native child process.
- A separate composite test passed two repeats of fio followed by native weighted I/O, including native warmup, separate data directories, combined tool identity and raw-evidence verification.
- The weighted 70:30 standalone acceptance observed 50,341 / 21,574 submitted operations (70.0007% reads). These counts only validate scheduling; they are not performance reference numbers. The 70:20:10 managed test also passed its 2-percentage-point functional test tolerance. The product itself does not impose an implicit ratio acceptance threshold.
- Native tests execute real filesystem I/O, reject modified byte counters, verify per-operation histogram counts, and prove that a failed create does not execute later pipeline steps or overwrite an existing file.
- In isolated kind 0.31.0 / Kubernetes 1.35.0 on Linux arm64, Server Deployment and separate Runner Jobs passed fio parallel, native data/stat and pipeline scenarios. Evidence remained available after successful Job cleanup. Native Job cancellation passed. Restarting the Server preserved the original Job UID and completed the same native run.
- Browser inspection confirmed the scenario tab, stage/workload details, timeline, target/actual 70/20/10 shares, separate stat/data metrics and native P99 upper-bound labels. The existing preview database and its old runs were preserved. New UI samples are explicitly named local functional acceptance; they are not imported remote performance results.

## Evidence locations

- Development host: `/tmp/malta-scenarios-20261026/acceptance-1/summary.json` and exported bundles beneath the same directory. Full remote evidence remains on that host.
- Local Kubernetes fixture: `/private/tmp/malta-mixed-acceptance/summary.json`, scenario run/bundle JSON and test scripts. The temporary `malta-mixed` cluster is removed after acceptance.
- Local browser samples: `/private/tmp/malta-local-scenario-preview/`; UI records are in the existing `/private/tmp/malta-ui-store` database.
- Reusable CLI/API acceptance entry point: `scripts/scenario-smoke.py`.

The Kubernetes fixture uses isolated single-node hostPath PVs, not a certified CSI deployment. Native v1 uses buffered synchronous calls and a bounded logarithmic latency histogram. Multi-Pod synchronization, cross-engine concurrency within one stage, arbitrary shared-file races, byte-weighted mixes, operation delays and production performance baselines remain outside this implementation. No LLM provider was invoked.
