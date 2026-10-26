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
