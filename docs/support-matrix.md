# RC support and acceptance boundaries

| Area | Implemented / tested | Not claimed |
| --- | --- | --- |
| Linux amd64 | Go test binaries, five real fio operations, managed CLI, Agent session recovery over SSH on Linux 5.10 | Other kernels/architectures automatically certified |
| Linux arm64 | Server + Runner images, kind 0.31.0 / Kubernetes 1.35.0, successful/cancelled/restarted/failed Jobs | Production distributed filesystem performance |
| macOS arm64 | CLI/API development, race tests, embedded UI build/browser | fio performance equivalence with Linux |
| Tool adapters | fio 3.33; mdtest 4.0.0 single-task single-iteration file-mode imports | Arbitrary tool versions, native metadata execution, MPI/multi-client |
| Kubernetes storage | Three single-node static hostPath PVs in an isolated functional fixture; results marked RWX | Real RWX multi-node semantics or any CSI-driver certification |
| Results recovery | Durable bundle, original Job/Attempt identity, permanent claim, cancel observation, retained failed evidence | Exactly-once completion, network partition fencing, automatic retry after data loss |
| Agent interface | Versioned CLI/API, capabilities, request lookup, durable helper, skill, black-box driver | Third-party LLM reasoning quality or a shipped MCP server |
| Release | Embedded UI, three binary targets, dependency notices, checksums, deterministic repeated builds | Signed public releases, published image registry, reproducible apt resolution |

Server is a single instance with SQLite and a serial queue. One shared bearer token
covers all API functions; multi-tenant roles/read-only agent credentials are future
work. No model inference runs in the benchmark process. No automatic performance
regression gate is derived from these functional runs.

Before a production deployment: validate the specific CSI/access modes/atomic mkdir,
permissions and PV topology; document node failure fencing and manual recovery;
set measured workload budgets on isolated hardware; execute the skill against the
chosen external model with recorded tool calls. These are environmental acceptance
gates, not capabilities silently inferred from kind.
