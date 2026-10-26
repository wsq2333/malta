# UI acceptance — S4

The browser acceptance uses the actual embedded application and API. No fabricated performance rows are shipped with the UI.

Reproduce by running `scripts/smoke.sh` on Linux with fio 3.33, importing its exported bundle into a preview store, importing the documented genuine mdtest fixture for parser demonstration, and serving that store. Use two separate smoke runs for a date comparison; their underlying software is unchanged, so this is functional validation, not a claimed version improvement.

Verified through the browser:

| Scenario | Expected result |
| --- | --- |
| Data baseline / run detail | MiB/s, IOPS, trial P99, Min/Max and repeat counts agree with the API and raw fio |
| Metadata tab | create/stat/remove match the mdtest fixture; latency says uncollected |
| Different workload comparison | Explicit incompatibility reasons; no percentage deltas |
| Identical-condition comparison | Server-provided per-point deltas and warnings; date trend and concurrency chart |
| Empty operation filter | Empty state without synthetic zero metrics |
| Invalid submitted plan | Visible validation error; no run is created |
| Narrow viewport (390 px) | Navigation remains named; tables scroll within their panels; no whole-page horizontal overflow |
| URL state | Page, kind, environment, version, operation, sizes, dates and comparison IDs are represented in the URL |

Frontend statistical unit checks run with `pnpm --dir web test`. Backend integration and embedded-resource tests run with `go test ./...`; set `MALTA_TEST_FIO` to exercise real I/O. The browser checks above are manual acceptance performed through the browser automation interface, not an unattended Playwright test suite.
