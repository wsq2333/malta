# CLI (implemented S1)

Requires Go 1.26.5 to build, Linux to benchmark, and fio **3.33**. Pinning the tested parser version avoids silently accepting changed tool semantics. Commands emit a schema-versioned JSON envelope and never prompt.

```sh
go build -o bin/malta ./cmd/malta
mkdir -p /tmp/malta-data
bin/malta plan --spec benchmarks/data/smoke-read.yaml \
  --environment environments/local.yaml --version environments/version-example.yaml \
  --target /tmp/malta-data --resource-group smoke --out /tmp/malta-plan.json
bin/malta exec --plan /tmp/malta-plan.json --out /tmp/malta-result \
  --fio fio --timeout 2m --max-bytes 1073741824 --non-interactive
```

`plan` validates and expands; it performs no benchmark I/O. `exec` creates a unique run/attempt directory beneath an existing target, prepares each trial's files, then runs timed measurement with optional fio ramp time. Preparation is excluded from the measured metrics. It retains data for inspection: `data-paths.json` identifies the directory to remove. The tool never removes pre-existing target data. Use a dedicated target and the same resource group for tests sharing a device; the local advisory lock is held throughout a run. Cross-host exclusion belongs to the later Kubernetes executor.

`bundle.json` contains the run and SHA-256-checked raw files. Other evidence includes plan, tool version, exact argument arrays, preparation/measurement JSON and stderr. File size and block size are distinct; mixed read/write metrics remain separate. Percentiles are per trial and cannot be averaged into a global percentile.

SIGINT/SIGTERM cancels the run and kills its process group. A total deadline includes preparation and warmup. Failed/cancelled runs retain available evidence. `cold-requested` requests fio invalidation; it does not prove a cold distributed cache. `cold-verified` is rejected by this executor. The default total prepared-data budget is 1 GiB; large lab profiles require an explicit budget.

Exit codes: 0 success, 2 invalid/unsupported input, 3 execution/storage/server error, 4 timeout, 5 cancellation, 6 conflict, 7 authentication. Test with `MALTA_TEST_FIO=/absolute/path/to/fio go test ./internal/runner`; this runs five small real workloads in temporary directories.
