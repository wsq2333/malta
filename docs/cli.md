# CLI

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

## Results and comparisons (S2)

```sh
bin/malta environment register --file environments/local.yaml
bin/malta version register --file environments/version-example.yaml
bin/malta result import --bundle /tmp/malta-result
bin/malta result list --kind data --limit 100
bin/malta baseline set --name release-reference --run RUN_ID
bin/malta baseline list
bin/malta compare --baseline RUN_ID --candidate OTHER_RUN_ID
bin/malta result export RUN_ID --out /tmp/malta-export
```

Local commands use `--data-dir .malta`. Snapshot IDs are immutable; create a new ID for changed hardware or build identity. Importing identical content is idempotent; conflicting content is rejected. Successful trial metrics are re-parsed from the attached raw files during import. Baseline records are append-only and always point to a specific successful run. Comparison uses median/min/max of independent repeats; latency is the median of trial P99 values, never a merged P99. A zero baseline yields a null percentage. Incompatible conditions produce explicit reasons and no percentage deltas. Unknown environment/cache fields and fewer than three repeats produce warnings.

Metadata imports support **mdtest 4.0.0**, one task, `-d PATH -n FILES -i 1 -F`, with no additional switches. Each raw file must be a complete independent iteration; the declared file count must match. Only create/stat/remove rates are imported. Latency was not collected and remains absent. The run date is the import date; original tool timestamps remain in raw evidence.

```sh
bin/malta plan --spec benchmarks/metadata/mdtest-files.yaml \
  --environment environments/local.yaml --version environments/version-example.yaml \
  --target /tmp/malta-data --out /tmp/mdtest-plan.json
bin/malta result import-mdtest --plan /tmp/mdtest-plan.json \
  --raw testdata/mdtest/mdtest-4.0.0.txt
```

The checked-in fixture is parser demonstration data, not a new experiment. For repeat counts greater than one, pass one actual output per repeat with comma-separated `--raw` paths.
