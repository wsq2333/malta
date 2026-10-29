# Install, configure and upgrade Malta 0.1 RC

The RC is built locally/by CI; no public release or registry publication is implied.
Use archives from your trusted build. Production CSI and external model behavior
still require the environment-specific acceptance in `support-matrix.md`.

## Install an archive

Linux amd64 is validated on the designated development host. Linux arm64 is
validated in the isolated Kubernetes container environment. macOS arm64 supports
CLI/server development; actual fio performance runs target Linux.

```sh
sha256sum --ignore-missing -c SHA256SUMS
# macOS: shasum -a 256 ARCHIVE.tar.gz, then compare with its SHA256SUMS entry
tar -xzf malta-0.1.0-rc.1-linux-amd64.tar.gz
cd malta-0.1.0-rc.1-linux-amd64
./malta version
./malta capabilities
```

The binary contains the UI. Node, Go and SQLite system libraries are unnecessary
at runtime. Optional skill session scripts require Python 3.9+. Local benchmarks
need **fio 3.33**, invoked using its explicit path; check `fio --version` (another
unrelated Python utility also uses the name fio). The Runner image includes it.

## Local functional smoke

Use a new directory on an approved local test filesystem; avoid production data.
The bundled smoke creates 3 × 8 MiB files plus small control artifacts. It exercises
real I/O, keeps its test files, and stops its temporary server automatically.

```sh
MALTA_BIN="$PWD/malta" MALTA_TEST_FIO=/absolute/path/to/fio \
  bash scripts/smoke.sh /tmp/malta-rc-smoke
```

To retain an interactive service afterwards:

```sh
mkdir -p /tmp/malta-target
./malta serve --listen 127.0.0.1:8080 --data-dir "$PWD/state" \
  --allow-target /tmp/malta-target --fio /absolute/path/to/fio
```

Open `http://127.0.0.1:8080`. Generate a plan with the README example, use a fresh
request ID, then set a baseline and compare another completed run. Use immutable,
accurate environment/version identities for actual performance measurements.
Metadata imports use the separately documented mdtest 4.0.0 file-mode contract.

For Agent session acceptance using only public interfaces:

```sh
python3 tests/agent/acceptance.py --malta "$PWD/malta" \
  --fio /absolute/path/to/fio --output /tmp/malta-agent-rc
```

## Kubernetes service Pod

Follow [deployment instructions](../deploy/kubernetes/README.md). Server and Runner
are separate images/identities. Supply existing namespace, SQLite-safe state PVC,
benchmark PVC, independent RWX results PVC and API token Secret. Pin both images
by digest. Use the renderer and server-side dry-run before applying manifests.
The server is single replica, Recreate deployment; the SQLite directory is never
placed on the benchmark PVC. Do not run multiple servers against one database.

`MALTA_TOKEN` is required by default when listening beyond loopback.
An explicitly trusted private deployment may opt in with `--allow-unauthenticated`.
For UI/API without execution, use `--management-only` (mutually exclusive with
local targets and Kubernetes config); see [the management-only profile](../deploy/kubernetes/overlays/management/README.md). Provide TLS through your
existing ingress/reverse proxy or use port-forward/SSH. v0.1 has one shared token,
not tenant-level authorization. Runner has no service-account token or host mounts.

## Build from source

Pin Go 1.26.5, Node 24.19.0 and pnpm 11.25.0; Python 3.9+ drives packaging.

```sh
python3 scripts/release.py --version 0.1.0-rc.1 --with-kubectl
```

This builds UI twice and each binary twice, checks identical bytes, packages
licenses and writes archives plus `SHA256SUMS` and `build-manifest.json` to
`release/`. It never uploads. The revision includes `-dirty` for uncommitted source.
The Go binary is pure Go with embedded assets; timestamps/build IDs are excluded.
Archives normalize ordering, modes, owners and timestamps. Docker OS package
resolution is not byte-reproducible; preserve and deploy the resulting image digest.

Build the two Dockerfiles after release preparation. Their context needs
`build/output/malta-linux-<arch>`, verified kubectl binaries and copied licenses.
Use `--platform linux/amd64` or `linux/arm64` matching the target. The default
base digest can be obtained from a registry mirror using `BASE_IMAGE` with the
same digest. Publishing images/releases is a separate, explicit operation.

## Upgrade and rollback

1. Stop accepting new runs. Let local runs finish or cancel them explicitly.
   Kubernetes runs can continue while the service is stopped, but keep the same
   configuration, result mount and runner digest until those attempts finalize.
2. Stop the service. Back up the **entire state directory** (SQLite DB/WAL/SHM,
   artifacts and work) consistently, and separately snapshot the results PVC.
   Copying a live SQLite file alone is not a backup.
3. Verify checksums and the support matrix, replace binary or server image, then
   restart with the same state directory. Check version, capabilities and pending
   run identities. Open the UI and export a known bundle.
4. For rollback, stop the new service and restore the complete pre-upgrade snapshot
   with its matching binary. Do not blindly open a newer database with an older
   binary. v0.1 uses SQLite `user_version=1` and schema 1.0; later migrations must
   explicitly document compatibility.

Exported bundles are the portable archive format. A compare may become
non-comparable across Malta/tool versions; do not suppress that warning.

## Retention and limits

History/trend UI queries cap at 500 rows; CLI/API history supports pagination.
Artifacts are bounded to 32 MiB per bundle, HTTP envelopes to 64 MiB, plans to
128 trials. Default run deadline is one hour and prepared data budget 1 GiB.
These budgets bound data prepared, not cumulative time-based I/O traffic.
Data, `/results/<attempt>` and failed Jobs are retained for inspection; cleanup
is operator-controlled. Remove only verified dedicated directories and only after
exporting required evidence. Server logs should be collected by the deployment.
