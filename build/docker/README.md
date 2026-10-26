# Server and Runner images

Build the embedded frontend, then Linux binaries into `build/output/`:

```sh
(cd web && pnpm install --frozen-lockfile && pnpm build)
for arch in amd64 arm64; do
  CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -trimpath \
    -o build/output/malta-linux-$arch ./cmd/malta
done
```

The Server additionally needs `kubectl-linux-<arch>` (1.35.0). Download from
`https://dl.k8s.io/release/v1.35.0/bin/linux/<arch>/kubectl`, verify against
`kubectl-checksums.json`, and set executable permissions. S7's release script
will automate these steps.

```sh
docker build -f build/docker/Dockerfile.server -t malta-server:local .
docker build -f build/docker/Dockerfile.runner -t malta-runner:local .
```

`BASE_IMAGE` can point to a mirror of the **same digest**. The base manifest is
pinned; fio is fixed to Debian `3.33-3`. OS dependency updates can change image
bytes; capture each produced image digest and use that digest in executor configs.
Do not claim that apt-based images are byte-for-byte reproducible across dates.

The Server includes UI and kubectl; Runner includes fio and the same Malta binary.
Both run as UID/GID 10001 with Malta as PID 1 to receive SIGTERM. No model runtime,
Kubernetes credentials, mounting privileges or workload data is baked into images.
