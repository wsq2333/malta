# Third-party notices

Malta's original code is Apache-2.0. Dependencies retain their own terms.
Release archives include a `licenses/` directory collected from the pinned Go
modules, Go runtime, and frontend packages. This includes dependency notices where
present. `go.mod`, `go.sum`, `web/package.json` and `web/pnpm-lock.yaml` identify
versions; `build-manifest.json` records release inputs.

The embedded UI includes React/React DOM (MIT), ECharts/zrender (Apache-2.0),
lucide-react (ISC) and their runtime dependencies. The release script copies the
actual installed license files rather than replacing them with this summary.

Runner images install fio 3.33 from Debian; fio is a separate executable with its
own GPL licensing, and Debian package copyright/license files remain under
`/usr/share/doc`. Server images contain the separately distributed kubectl 1.35.0
(Apache-2.0). These programs are not linked into Malta. Container base packages
have their own notices, retained in the images. Rebuilding or redistributing an
image must preserve applicable notices and source-availability obligations.
