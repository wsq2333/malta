# Kubernetes: Server Deployment + Runner Job

S5 implements one serial client Job per server. Kubernetes 1.35+ and kubectl
1.35.0 are the initial integration target. Both images must be pinned by digest.
The service's SQLite directory is independent of benchmark and results storage.

## Prerequisites

- Existing namespace and three **different** PVCs: state (SQLite-safe local locks),
  benchmark (Bound filesystem RWO or RWX), results (Bound filesystem **RWX**).
- Results filesystem must support atomic mkdir and be mounted at `results_dir`
  on the server and `/results` on the worker. A per-attempt handshake verifies
  the shared mount before any benchmark I/O; a durable claim prevents replay.
- An explicit Ready, schedulable benchmark node whose hostname label equals its
  name. Kubernetes scheduling checks PVC topology; unschedulable Jobs terminate
  at the deadline. Populate cloud, model, storage and network in the environment.
- UID/GID 10001 must be able to write all three volumes. If the CSI ignores fsGroup,
  provision ownership separately; no privileged init container is supplied.
- Benchmark and results CSI performance/isolation, node affinity and capacity
  must be validated in the target cluster. Metadata claims cannot prove these.
  Server and runner resource limits are explicit; keep service load off benchmark
  nodes when possible. SQLite server replicas must remain 1 with Recreate updates.

## Render and install

1. Build images with `build/docker/Dockerfile.server` and `Dockerfile.runner`;
   push to your chosen registry, record their resulting digests.
2. Copy `overlays/example/executor.json`; set cluster/node/PVC/image/resources.
   Keep `context` empty inside the service Pod. For a host-run server, set the
   explicit kubeconfig context and mount the same results filesystem locally.
3. Create a Secret named `malta-api-token` with key `token` using your secret
   management process. The server token must not be committed.
4. Render without making cluster changes:

```sh
python3 scripts/render-kubernetes.py --config executor.json \
  --server-image YOUR_SERVER_IMAGE_AT_SHA256 \
  --state-pvc malta-state --token-secret malta-api-token > deployment.json
kubectl --context YOUR_CONTEXT apply --dry-run=server -f deployment.json
kubectl --context YOUR_CONTEXT apply -f deployment.json
```

`base/*.json` are parameterized manifests, not directly applicable files. The
renderer resolves all placeholders and adds the executor ConfigMap. Namespace,
PVCs and API Secret intentionally remain operator-owned. RBAC lets the server
manage Jobs/config maps and read Pod logs/PVCs plus one named Node. The separate
Runner service account has no roles and no mounted API token.

## Plan and run

Update `environments/kubernetes.yaml` with immutable hardware identity. Its
`config.cluster`, `config.node`, and `config.benchmark_pvc` must match the executor.

```sh
malta plan --spec benchmarks/data/smoke-read.yaml \
  --environment environments/kubernetes.yaml --version environments/version-example.yaml \
  --target /bench --resource-group lab --out plan.json
malta kubernetes check --config executor.json --plan plan.json
malta --server http://127.0.0.1:8080 run start --executor kubernetes \
  --plan plan.json --request-id experiment-001 --source human
malta --server http://127.0.0.1:8080 run wait RUN_ID --timeout 5m
malta --server http://127.0.0.1:8080 run logs RUN_ID
malta --server http://127.0.0.1:8080 run cancel RUN_ID
```

Use port-forward or authenticated TLS ingress; `MALTA_TOKEN` supplies API auth.
`kubernetes check` is a read-only prerequisite check, not an I/O permission test.

## Recovery and evidence

The run records Job UID, all observed Pod UIDs, restart count, exit code and image
ID. `environment-observed.json`, `job.json`, `pods.json`, logs, and fio outputs
remain in the downloadable bundle after cleanup. The run's declared environment
is immutable; the observed snapshot is separate evidence, not an automatic
hardware certification.

Service shutdown detaches from Kubernetes work. Restart with the same database,
results mount and executor configuration reconciles the same Job/Attempt.
A request ID reused with a different plan/source/executor is rejected.
Cancellation suspends the Job and waits for Pods to stop before finalization.
An unreachable node may prevent termination confirmation: the run reports a
failure, retains the Job and evidence, and requires operator inspection. Never
force-delete a Pod as proof that I/O on an unreachable node has stopped.

No TTL is set. Only confirmed durable, complete evidence allows Job cleanup.
Missing/invalid results and persistence failures retain objects for investigation.
An API/identity failure can leave a Job running until its active deadline; inspect
it before starting further load. After fixing a persistence issue, export the
retained worker bundle and import it into a separate inspection store; do not
reset claims or reuse an attempt ID. `/results/<attempt>` and benchmark data are
retained for explicit cleanup; record retention is not automated in v0.1.

Actual tested combinations and limitations are tracked in
[the validation record](../../docs/validation-s5-s7.md). No production CSI support
is implied by the functional test fixture.

For the real sample dev Curvine rollout, see
[deployment and acceptance findings](../../docs/validation-sample-dev.md).
That rollout exposed a regular-file ownership issue: a storage preflight must
verify the UID/GID of a newly created file **after closing and reopening it**, in
addition to mkdir/rename and cross-node result sharing. Successful scheduling and
result collection alone do not establish a valid performance baseline.
