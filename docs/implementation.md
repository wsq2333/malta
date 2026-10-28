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

## S6 — provider-neutral Agent workflow

Added capabilities and request lookup to the public CLI/API, versioned capability
schema, a durable session helper, and scoped local/Kubernetes/recovery/reporting
skill references. Session recovery does not submit work. Black-box acceptance
uses only the published CLI and skill assets, with no database access.

## S7 — release candidate and installation

Added deterministic RC packaging for Linux amd64/arm64 and macOS arm64, embedded
UI, dependency notices/checksums, pinned kubectl fetching, container license inputs,
CI artifacts, installation/upgrade instructions and an explicit support matrix.
Apache-2.0 was selected by the owner. The RC archive passed a fresh-directory
installation, real fio smoke and Agent protocol acceptance on the SSH host; RC
images passed the isolated Kubernetes lifecycle suite. Browser acceptance now
shows only configured executors. Worker/controller version mismatch is rejected
before I/O. Third-party Agent evolution is described in its own design document.

## Remaining scope

Production CSI/node-failure and external model acceptance remain environment-specific gates. The server retains a serial queue and SQLite. Supported adapters are intentionally pinned to fio 3.33 and the documented mdtest 4.0.0 single-task file mode. UI history/trend windows are capped at 500 rows; CLI/API history supports pagination.

## Mixed workload scenarios

Added scenario/v1alpha1 planning, sequential/parallel fio, native weighted data/stat, and per-file pipelines. Managed local/Kubernetes execution, portable evidence, baseline/trend grouping, and the scenario UI share the same plan and result contract. Legacy plan hashes remain unchanged. See [usage and boundaries](mixed-workloads.md) and [acceptance evidence](validation-scenarios.md).

## Workspace visual refresh — 2026-10-28

The UI now defaults to a charcoal theme inspired by the visual hierarchy of
[Entire](https://entire.com/about): neutral surfaces, fine borders, larger headings,
white primary actions and restrained green accents. It keeps Malta's navigation,
benchmark tables and JSON workflows. No Entire fonts, logos or imagery are bundled.
The top bar switches between dark and light themes; the preference is stored locally
under `malta-theme`, with a dark fallback if browser storage is unavailable.

Semantic CSS variables in `web/src/style.css` cover surfaces, text, status and chart
colors. ECharts reads the same palette and updates when the theme changes. Long case
descriptions use a three-line list summary; the version view retains the full text.

Validation: TypeScript, the production frontend build and all three existing model
tests passed on the designated SSH development host. Browser checks used a separate
local management-only service and a local copy of the registered Curvine case:
case list/version/editor, guide, light/dark switching and reload persistence.
At 390 px, the case table scrolls within its container without whole-page overflow.
This change does not deploy a new image or modify the sample dev cluster.


## Issue tracking (2026-10-28)

Added a shared UI/API/CLI issue register with user-defined type labels, JSON file
upload, software/hardware descriptions, optional validated registry/run links,
occurrence/creation/update/resolution timestamps, resolution evidence and immutable
revision history. Optimistic revision checks prevent stale edits; stable IDs allow
identical v1 create retries. SQLite v2 upgrades transactionally to v3 while keeping
existing cases and runs. See [Issue management](issue-management.md).

Validation on the SSH development host: full `go test ./...`, `go vet ./...`,
frontend tests and production build passed. An isolated real management-only service
also passed CLI-over-HTTP creation, retry, custom type, resolve, conflict, reopen,
history and capabilities checks. Initial API test budget and nullable-schema defects
were reproduced/fixed locally before the successful development-host rerun.

Browser checks used the local preview only: form registration, custom type filtering,
resolution update, old-version inspection and JSON file upload/save. Fixed the Vite
proxy to preserve the browser-facing Host for the API's existing same-origin checks;
normal-origin requests reach validation while cross-origin writes remain rejected.
Local preview records are explicitly marked as validation examples. No cluster upgrade,
real issue registration, image push or benchmark execution was performed for this feature.
The existing ECharts chunk-size warning remains; the production build succeeds.
