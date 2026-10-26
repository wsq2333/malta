# Implementation evidence

## S0 — contracts and scaffold

Implemented Go CLI entry point, strict versioned domain models and generated JSON schemas, initial OpenAPI, pinned React/TypeScript build, CI, smoke definitions and genuine fio 3.33 / mdtest 4.0.0 fixtures.

Validation: local `go test ./...`, `go vet ./...`, frontend production build; Linux amd64 cross-compiled domain test binary and `malta version` passed via SSH on the designated development machine. Hardware example deliberately says unknown and is for functional smoke tests only.

## S1 — foreground fio execution

Implemented plan expansion, five fio operations, per-trial preparation/measurement, file/block dimensions, repeats, budgets, local resource locks, process-group cancellation, deadlines and SHA-256 evidence bundles. Added parser, artifact integrity, budget and timeout tests.

Validation: local Go tests and vet; designated Linux host passed genuine fio 3.33 sequential read/write, random read/write and 70/30 mixed I/O, parser fixtures and timeout evidence checks. These are functional tests on temporary local storage, not distributed filesystem performance baselines.

S2–S4 are in progress. Kubernetes Job execution remains S5.
