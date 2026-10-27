# S5–S7 validation record

Validation date: 2026-10-26. Functional smoke results are not filesystem performance
claims. Exact supported combinations and remaining lab gates are recorded here.

## S5

- Go race tests: passed, including PVC rejection, ownership mismatch, missing
  evidence, durable submission intent, cancellation and restart reconciliation.
- Frontend TypeScript/build: passed; added explicit local/Kubernetes executor choice.
- Isolated cluster: kind 0.31.0, Kubernetes 1.35.0, Linux arm64, containerd.
- Images: Debian bookworm-slim pinned by digest; fio package 3.33-3; kubectl 1.35.0.
- Storage fixture: three static hostPath PVs on one isolated node. Results is
  declared RWX for the fixture; this is **not** a multi-node RWX/CSI certification.
- Real Job lifecycle: passed successful fio runs, durable evidence after Job cleanup,
  stable request IDs, cancellation after Pod start, service Pod restart preserving
  Job UID, Pod deletion with no successful replay, and artifact permission failure
  retaining the Job. Test output: `/private/tmp/malta-s5-integration-3`.
- Remote Linux amd64: all ten test packages passed over SSH, including five real
  fio operations. Managed CLI smoke passed three repeats, comparison, export,
  idempotency, timeout and cancellation at `/tmp/malta-s5-s7-20261026/s5-smoke`.
- Real integration exposed an in-cluster authentication regression when kubectl
  request-timeout is overridden. Fixed by bounding the subprocess context while
  leaving kubectl's in-cluster config fallback intact.
- Node-loss termination cannot be proven by API state alone; failure is reported
  and Jobs retained. A production node-unavailability drill remains a lab gate.

Production acceptance still needs the actual CSI, benchmark PVC, node/hardware
inventory, network topology and fixed workload budgets. No CSI driver or distributed
filesystem has been certified by these functional checks.

## S6

- Full Go race regression passed; OpenAPI parsed and executor enums checked.
- Skill validator passed; session helper syntax/help checked.
- Public CLI black-box acceptance passed on the SSH development host with real
  fio: lost response/run-ID recovery, duplicate start, changed-executor conflict,
  wait timeout, comparison, and resume of an absent request without starting I/O.
  Evidence: `/tmp/malta-s5-s7-20261026/s6-agent-acceptance/acceptance.json`.
- This exercises the interface an external agent uses. No third-party language
  model was invoked; provider-specific reasoning and behavior acceptance is still
  required before unattended use with that provider.

## S7

- Apache-2.0 selected by the project owner; official license, notice and dependency
  notices included. Packaging collects licenses for linked Go modules and shipped
  frontend runtime packages.
- Go 1.26.5, Node 24.19.0, pnpm 11.25.0: repeated frontend builds and each of
  Linux amd64, Linux arm64 and Darwin arm64 binaries matched byte-for-byte.
  Deterministic tar/gzip metadata and SHA-256 checksums are generated. Container
  base images are pinned but apt dependency resolution is not claimed reproducible.
- Fresh archive extraction on the SSH host verified checksums and ran the real-fio
  CLI smoke and public Agent session contract successfully. Evidence is retained
  at `/tmp/malta-s5-s7-20261026/rc-smoke` and `rc-agent`.
- RC Server/Runner images passed the same real Kubernetes lifecycle and fault suite
  as S5; artifacts are at `/private/tmp/malta-rc-integration`; final version checks and the Kubernetes
  Agent session helper are covered in `malta-rc-final-integration`.
- Browser acceptance on the existing real-data preview confirmed capability-based
  executor choices and preserved baseline/trend rendering. Stale local-only labels
  were removed during acceptance.
- Final Linux amd64 test binaries (all ten packages) passed again over SSH,
  including atomic claim, bundle/SQLite crash-window recovery, capabilities and
  worker version rejection tests.
- Version mismatch between controller and worker now fails before claiming an
  attempt or performing I/O; covered by a focused CLI regression test.
- CI builds/test/packages artifacts but does not publish a GitHub release or push
  container images. Local validation is not a claim that remote GitHub CI ran.

Remaining external gates: a real CSI/multi-node RWX environment, node-loss fencing,
fixed-hardware performance qualification, and provider-specific LLM behavior
acceptance. RC code and functional tests are complete within the tested scope.
