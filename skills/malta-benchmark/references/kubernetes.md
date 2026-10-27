# Kubernetes workflow

The server must advertise `kubernetes`. Its administrator configures cluster,
namespace, fixed node, digest-pinned runner, PVCs and CPU/memory limits. The plan
uses `/bench` and a `kubernetes` environment whose config contains matching
`cluster`, `node`, `benchmark_pvc`. Reuse the immutable registered identity.
The caller does not pass arbitrary Pod specs or change cluster RBAC.

```sh
malta --server "$MALTA_SERVER" capabilities
malta plan --spec SPEC --environment ENV --version VERSION --target /bench --out plan.json
# Optional for operators with their own cluster credentials:
malta kubernetes check --config EXECUTOR.json --plan plan.json
python3 skills/malta-benchmark/scripts/session.py start \
  --session cluster-experiment.json --server "$MALTA_SERVER" --plan plan.json \
  --request-id cluster-experiment-001 --executor kubernetes
malta --server "$MALTA_SERVER" run get RUN_ID
malta --server "$MALTA_SERVER" run logs RUN_ID
malta --server "$MALTA_SERVER" run wait RUN_ID --timeout 5m
```

A successful submission only queues work. Inspect `attempt.job_uid`, `pod_uids`,
`exit_code` and `result_state`. Evidence includes observed environment, Job/Pod
snapshots and raw fio files. Results PVC must be independent and RWX; a handshake
and permanent claim prevent execution on an unshared mount or duplicate attempt.
The fixture's hostPath validation does not establish compatibility with your CSI.

`run cancel` suspends the Job and observes termination. Unconfirmed node termination
is a failure requiring operator inspection; never treat force deletion as proof
that remote I/O stopped. Do not delete Jobs, reset claims or modify PVCs through
this skill. The service cleans Jobs only after durable complete evidence.
