# Implementation evidence

## S0 — contracts and scaffold

Implemented Go CLI entry point, strict versioned domain models and generated JSON schemas, initial OpenAPI, pinned React/TypeScript build, CI, smoke definitions and genuine fio 3.33 / mdtest 4.0.0 fixtures.

Validation: local `go test ./...`, `go vet ./...`, frontend production build; Linux amd64 cross-compiled domain test binary and `malta version` passed via SSH on the designated development machine. Hardware example deliberately says unknown and is for functional smoke tests only.

## S1 — foreground fio execution

Implemented plan expansion, five fio operations, per-trial preparation/measurement, file/block dimensions, repeats, budgets, local resource locks, process-group cancellation, deadlines and SHA-256 evidence bundles. Added parser, artifact integrity, budget and timeout tests.

Validation: local Go tests and vet; designated Linux host passed genuine fio 3.33 sequential read/write, random read/write and 70/30 mixed I/O, parser fixtures and timeout evidence checks. These are functional tests on temporary local storage, not distributed filesystem performance baselines.

## S2 — result storage and baselines

Implemented SQLite persistence, immutable snapshots, idempotent imports with raw-metric verification, bundle export, filtered history, append-only baselines and condition-aware comparisons. Real mdtest 4.0.0 one-task file summaries provide create/stat/remove rates. Missing latency stays absent; zero baseline has no percentage.

Validation: local tests and vet plus Linux test binaries passed SQLite roundtrip/conflict, baseline, changed hardware/workload, failed run, zero baseline and genuine metadata fixture cases. Performance dates for imported metadata are import dates, with original timestamps retained in raw evidence.

## S3 — managed API and Agent CLI

Implemented single-manager SQLite queue, one conservative local worker, server-side target/budget validation, idempotent request IDs, cancellation, restart interruption, authenticated API, bounded log snapshots, wait semantics and local Agent skill. Restart never automatically replays an unfinished write experiment. OpenAPI now links concrete response schemas.

Validation: local API and storage race checks, structured CLI error tests, skill validator; Linux API tests with real fio passed. The Linux end-to-end CLI script verifies identical retries, wait timeout without cancellation, three actual repeats, baseline comparison, evidence export and cancellation. All benchmark data is isolated beneath the designated temporary test workspace.

## S4 — real-data Web UI and final integration

Implemented embedded React/TypeScript UI with white backgrounds and mint-green accents: data/metadata baseline tables, file/block/operation/version/environment/date filters, immutable registry views and registration, managed run submission and cancellation, details/logs/evidence download, server-authoritative comparisons, date trends and measured concurrency curves. Filters are represented in the URL. Charts load on demand; no synthetic benchmark rows are bundled.

Integration tightened imported fio option checks against the declared plan, unknown metric rejection and idempotent retrieval after server budget changes. The CLI/API contracts and README now describe implemented behavior and explicit future scope.

Final validation on 2026-10-26:

- Local `go test -race ./...`, `go vet ./...`, frontend TypeScript/production build and statistical unit tests passed.
- All Go test packages were cross-compiled for Linux amd64 and executed via SSH on the designated development host, whose installed Go 1.22.2 was left unchanged. Domain, artifact, fio, mdtest, runner, SQLite, API, CLI and embedded frontend tests passed.
- Real fio 3.33 passed sequential read/write, random read/write and mixed I/O. Cancellation was tested after a child had actually started and verified that the child stopped. The managed CLI smoke script passed request idempotency, wait timeout without cancellation, three real repeats, baseline comparison, export and cancellation.
- Browser acceptance used real Linux evidence and the real mdtest fixture: values matched, uncollected latency stayed absent, incompatible conditions suppressed deltas, invalid plans displayed an error, and a 390 px viewport had no whole-page horizontal overflow. Details are in `tests/e2e/README.md`.
- Agent skill frontmatter passed the skill validator. Its instructions were exercised through the local managed CLI workflow.

Remote test workspace: `/tmp/malta-s0-s4-20261026`. S3/S4 smoke workspaces retain result bundles and CLI output for inspection; temporary services are stopped by the script. Tool builds were isolated there. Test evidence is functional local-storage evidence, not a measured Curvine or distributed filesystem performance baseline.

## S5 — independent Kubernetes Runner

Implemented digest-pinned images, parameterized deployment/RBAC manifests,
read-only prerequisite checks, per-attempt claims and shared-volume handshake,
Job/PVC execution, identity/status/log evidence, cancellation, restart reconciliation
and cleanup only after persistence. UI supports executor selection and Attempt details.
Remote Linux real-fio tests and an isolated Kubernetes 1.35.0 lifecycle/fault suite
passed. See [validation details](validation-s5-s7.md).

## Remaining scope

S6 completes the Agent interface and workflows; S7 prepares RC delivery. Production CSI/node-failure and external model acceptance remain environment-specific gates. The server retains a serial queue and SQLite. Supported adapters are intentionally pinned to fio 3.33 and the documented mdtest 4.0.0 single-task file mode. UI history/trend windows are capped at 500 rows; CLI/API history supports pagination.
