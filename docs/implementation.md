# Implementation evidence

## S0 — contracts and scaffold

Implemented Go CLI entry point, strict versioned domain models and generated JSON schemas, initial OpenAPI, pinned React/TypeScript build, CI, smoke definitions and genuine fio 3.33 / mdtest 4.0.0 fixtures.

Validation: local `go test ./...`, `go vet ./...`, frontend production build; Linux amd64 cross-compiled domain test binary and `malta version` passed via SSH on the designated development machine. Hardware example deliberately says unknown and is for functional smoke tests only.

S1–S4 are in progress. Kubernetes Job execution remains S5.
