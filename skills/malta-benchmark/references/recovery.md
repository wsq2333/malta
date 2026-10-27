# Recovery and failure decisions

| Observation | Action |
| --- | --- |
| Missing environment/version/budget | Query registered objects and capabilities; ask for the missing consequential input before I/O. |
| Start response lost | Lookup the original request; retry identical start only if necessary. |
| Session file exists, run ID absent | `session.py resume` looks up the saved request. It never creates work. |
| Request conflict | Compare plan hash/source/executor; preserve evidence and resolve intent. |
| Wait timeout / agent context ends | Save session, resume the same run; no new request ID. |
| Server unavailable | Report unavailable, retry observation within the monitoring budget; do not submit to another server. |
| Local interrupted | Inspect target and evidence; a replacement experiment needs explicit intent and a new identity. |
| Kubernetes service restarted | Query existing run; server reconciles original Job. Do not run `worker` or recreate Job manually. |
| Job complete but result_state failed | Treat as incomplete evidence; export available diagnostics and involve operator. |
| Partial trials or failed run | Report completed subset as diagnostic data, not a comparable baseline. |
| Non-comparable result | List differences; suppress attribution and percentage conclusions. |
| Node termination unconfirmed | Report uncertainty and retained Job identity; stop additional load on that target until inspected. |

Session helper requires Python 3.9+ on Linux/macOS. It uses a local file lock and
atomic, private session writes. A session file contains configuration and IDs but
no token. Preserve it across agent restarts; never share it across unrelated tests.
