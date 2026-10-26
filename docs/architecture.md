# 架构与语言选型

状态：S0–S7 已实现本地与 Kubernetes 单客户端闭环；高级分析、多客户端和生产环境认证仍属后续范围。实际命令与边界以 [CLI 文档](cli.md) 和 [实现记录](implementation.md) 为准。Malta 定位为 Agent 增强与管理的分布式文件系统性能对照平台；CLI 与 Agent 操作契约、Kubernetes 单 Pod 执行纳入首版设计，多客户端与高级分析逐步扩展。

## 1. 语言选择

| 方案 | 适合的部分 | 本项目取舍 |
| --- | --- | --- |
| Go | CLI、外部进程控制、采集、API、数据处理 | 作为核心实现语言，CLI 和服务共用领域模型与解析器 |
| TypeScript | 交互 UI、类型化 API 客户端 | 配合 React 构建比较页面，业务判定仍放服务端 |
| Python | 统计探索、Notebook、Agent 原型 | 后续独立使用，首版不额外维护 Python 服务 |
| Rust / C++ | 自研高性能 I/O 引擎、底层集成 | 首版由外部工具生成负载，暂不增加这些语言 |

这是基于项目职责和部署方式的选择，不是对语言性能的排名。若以后开发自有压测工具，应以计时精度、I/O 接口和运行时开销实测重新选型。

Go 标准库提供外部进程调用和静态资源嵌入能力。设计上使用参数数组执行工具，并实现 Linux 进程组级取消与清理；不能假设取消父进程就会清理所有后代进程。参考 [os/exec](https://pkg.go.dev/os/exec) 与 [embed](https://pkg.go.dev/embed)。

前端使用 React + TypeScript，由 [Vite](https://vite.dev/guide/) 构建静态资源；图表候选为 [Apache ECharts](https://echarts.apache.org/handbook/en/get-started/)。首版不需要服务端渲染或额外的 Node.js 生产服务。

## 2. 进程与模块边界

采用单仓库和模块化单体，一个二进制包含不同子命令。Runner 与 API 是逻辑模块，不要求一开始部署成多个服务。

| 模块 | 负责 | 边界 |
| --- | --- | --- |
| domain | 负载、运行、指标和环境类型 | 不依赖 HTTP、数据库或具体工具 |
| runner | 准备、执行、计时、取消、收集、完成状态 | 不直接决定性能是否回退 |
| cli | 参数校验、命令组织、结构化输出与退出码 | 人和 Agent 共用；不复制服务端业务规则 |
| executor | local/kubernetes 提交、状态协调、取消和回收 | 与前台 Runner 分开，不依赖 CLI 会话存活 |
| engine/fio | 能力检查、参数生成、输出解析 | 保留工具语义，不将未报告的指标补为零 |
| compare | 条件匹配、摘要统计、差异与可比性原因 | CLI、UI 和 Agent 使用相同结果 |
| store | 查询、导入事务、schema migrations | 首版 SQLite；通过明确查询接口隔离存储实现 |
| artifact | 文件布局、内容哈希、读取与导出 | 原始内容不可被派生分析覆盖 |
| api | 输入验证、分页、筛选和错误响应 | 不承载压测工具专有解析规则 |
| web | 页面、图表与交互 | 不自行计算另一套回退判定 |

首版 UI 侧重结果查询和比较，压测通过 CLI 发起。同一测试资源组默认串行执行，避免多个负载互相干扰。资源组是运行配置中的显式标识；它不能自动隔离其他机器上手工发起的测试。

`plan` 固定负载、目标、预算和环境快照，`run start` 将其交给共享服务管理。local executor 在服务所在测试节点执行；kubernetes executor 创建 Job。两者均使用前台 Runner 写出可移植结果包，完成后由 `result import` 或服务导入。独立的 `exec` 支持当前机器/Pod 中离线执行，不提供脱离监督进程的后台管理承诺。

托管运行先持久化 Run 和 request-id，再提交执行。相同 request-id/计划返回同一 Run，不同内容报冲突；实际执行可能包含多个 Attempt，记录调度原因和状态。取消先停止后端继续启动，再终止进程/Pod，保留已收集证据。仅凭取消请求返回不能判定进程已经停止。

## 3. 数据模型

| 对象 | 主要字段与含义 |
| --- | --- |
| TestDefinition | schema_version、负载、参数矩阵、数据集、预热/测量/重复策略、约束 |
| ExecutionPlan | 完全展开的定义、环境引用、目标路径、执行后端、资源与时长预算、计划哈希 |
| EnvironmentSnapshot | 客户端/服务端硬件、拓扑、版本、挂载与配置、已知背景负载、缺失字段 |
| Run | run_id、完整定义快照、环境快照、开始/结束时间、工具及 Malta 版本、状态 |
| Attempt | attempt_id、所属 Run、执行后端、Pod/进程身份、开始/结束、失败/重试原因、结果持久化状态 |
| Trial | trial_id、所属 Run、参数点、重复序号、实测时间、状态、指标及 artifact 引用 |
| Metric | 名称、操作类型、单位、范围、延迟定义、统计方法、值或缺失原因 |
| Artifact | 相对路径、类型、大小、SHA-256、来源 Trial；原始输出及可选时序文件 |
| Baseline | 名称、版本、显式选中的已完成 Run/Trial、选择条件和创建时间 |
| Comparison | 基线与候选引用、允许变化的维度、差异、可比性、统计方法版本 |

Run 表示一次逻辑测试计划；Attempt 表示一次实际启动；Trial 表示一个参数点的一次计划内独立重复。重试产生新的 Attempt，不冒充新重复实验。多客户端阶段在 Trial 下增加 client 维度，不能把每个客户端误当作一次独立重复。

EnvironmentSnapshot 分别描述云厂商/区域/实例类型或物理机厂商/型号，记录客户端与各服务端角色的拓扑。Kubernetes 运行增加 namespace、Job/Pod UID、节点、资源限制、PVC/CSI 与镜像 digest。环境更改形成新快照，不能通过编辑环境名称重写历史条件。

基线以新版本更新，不悄悄改写旧引用。运行原始结果保持不可变；重新解析时记录解析器版本并形成新的派生结果。对比报告包含所用数据版本，避免日后算法变化导致历史报告含义漂移。

区分三类标识：完整配置哈希用于追溯，负载哈希用于寻找相同负载，比较策略决定哪些差异被允许。文件系统版本属于版本对比的允许变化项，不能因为完整哈希不同而拒绝所有版本对比。

结果包计划采用以下布局：

```text
<run-id>/
├── manifest.json          # schema 版本、状态、文件索引及内容哈希
├── definition.json        # 完全展开后的负载定义
├── environment.json       # 环境快照，明确标记未知项
├── summary.json           # 规范化 Trial 指标与解析器版本
└── trials/<trial-id>/
    ├── command.json       # 工具版本、结构化参数和实际执行条件
    ├── fio.json           # 原始 JSON/JSON+ 输出
    ├── stdout.log
    └── stderr.log
```

原始输出优先保留文件；不要在 manifest 中采集任意环境变量或凭证。schema 不支持的重大版本应明确拒绝导入。

## 4. 执行与导入一致性

运行状态为 `pending → preparing → running → succeeded / failed / cancelled / interrupted`。任意阶段的失败都保留诊断信息；启动恢复时，本地未完成运行标记为 interrupted；Kubernetes 接管原 Job/Attempt。两者均不自动重放压测。

Runner 记录工具退出码、超时、实际时长和每个 Trial 的完整性。read 测试的数据准备与预热单独计时，不混入测量窗口。每次运行使用目标路径下独立子目录，只清理自己创建且记录在案的文件。

导入先校验 schema、路径和文件哈希，在临时目录准备 artifact，再发布不可变文件，最后在数据库事务中登记。数据库提交前的中断可能留下未引用文件，由后续清理处理；数据库不能指向尚未发布的文件。

同一 run_id 与相同包内容重复导入视为幂等操作；同一 ID 内容不同应报冲突，不能覆盖旧证据。导入服务对包内相对路径做约束，禁止越出 artifact 根目录。

## 5. 存储与部署

首版使用 SQLite 保存索引、摘要、基线和比较记录，大型日志、时序数据和直方图保存在 artifact 文件中。数据库启用事务、外键、版本化 migrations 和有界写入重试。

SQLite WAL 允许读写并行，但同时只有一个 writer，且不适用于网络文件系统。因此数据库放在服务所在机器的本地磁盘，不能放到被测分布式文件系统上；多客户端通过服务访问数据。[SQLite WAL 文档](https://www.sqlite.org/wal.html)

生产目标是 Linux，开发环境可使用 macOS。提供 server/runner 两类容器镜像，Agent 运行时与模型推理不放进压测进程。Kubernetes 用 Job 创建 Runner Pod，复用已有 PVC/CSI；测试路径、只读计划和结果路径分离。服务和图表查询尽量与压测隔离，无法隔离时记录其资源占用。

单 Pod 路径进入 MVP。记录资源 requests/limits、实际节点和镜像版本；结果确认持久化之后才安排 Pod 清理。多 Pod 同步与聚合单独实现。目录职责见 [Kubernetes 部署](../deploy/kubernetes/README.md) 和 [镜像构建](../build/docker/README.md)。

当出现多个服务实例、持续并发导入或 SQLite 写入瓶颈时再引入 PostgreSQL；原始文件超出本地磁盘管理能力时引入对象存储。迁移需要数据和事务语义验证，不承诺只换连接串即可完成。

首版默认仅监听 loopback。供团队远程使用前补齐认证、权限、传输保护和部署说明。MVP 不提供任意 shell 命令执行的 HTTP 入口。

## 6. API 与 Agent 扩展

API 使用 `/api/v1` 命名空间。先在 OpenAPI 中定义契约，再生成前端类型；结果包的 JSON Schema 是离线交换格式，两者通过契约测试保持一致。

| 规划接口 | 用途 |
| --- | --- |
| `GET /api/v1/runs` | 分页筛选历史运行 |
| `POST /api/v1/runs` | 接收验证后的计划与 request-id，提交托管执行 |
| `GET /api/v1/runs/{id}` | 环境、定义、Trial 和状态 |
| `POST /api/v1/runs/{id}/cancel` | 幂等请求取消，返回当前状态，待后端确认终态 |
| `GET /api/v1/runs/{id}/artifacts/{artifact_id}` | 获取已登记的原始证据 |
| `POST /api/v1/baselines` | 创建指向已有结果的基线版本 |
| `POST /api/v1/comparisons` | 指定基线、候选、允许差异，生成比较结果 |
| `GET /api/v1/trends` | 按明确可比条件返回历史序列 |

Agent 从首版通过项目 skill 调用 CLI，完成计划、提交、查询、等待、取消和比较。`--format json` 与 `--non-interactive` 是稳定契约，stdout 不混入诊断日志。执行受项目配置的目标环境、目录与额度约束；已授权范围内自动运行，越界返回可识别错误。API 提供指标定义、单位、缺失和可比性原因，不能只返回图表数组。

测试管理 skill 已在 `skills/malta-benchmark` 实现，含会话恢复与证据报告规则；专用分析 skill 与模型适配层按 [第三方 Agent 方向](third-party-agents.md)扩展。流程与验收见 [skills](../skills/README.md)。

分析报告独立于测量结果，记录模型/工具版本、输入运行 ID、证据引用、观测事实、推断和待验证建议。后续接入 MCP 或其他 Agent 协议时复用查询服务，具体 SDK 与模型供应商暂不绑定。

## 7. 验证顺序

1. 解析器：用真实脱敏 fio fixture 验证单位、读写分组、直方图、缺失字段、错误和不同受支持版本。
2. 比较器：验证环境差异、允许版本变化、零基线、指标方向和不完整重复实验。
3. 存储：验证重复导入、ID 冲突、中断恢复和 migrations。
4. Linux 集成：在专用临时测试目录运行小数据量 fio，验证执行、取消、导入、查询闭环；CI 不做性能阈值验收。
5. UI：端到端验证筛选、对比、缺失值、单位和证据下载，核对图表与 API 数值一致。
6. 实验室验收：固定环境执行多轮真实测试，验证重复性和采集开销，再决定回退阈值。
7. CLI/Agent：验证无交互模式、JSON 契约、幂等提交、断线后的状态查询和超时不重提任务。
8. Kubernetes：验证 PVC 挂载、资源快照、取消、重复 Attempt、Pod 丢失和结果持久化后清理；单 Pod 通过不视为多客户端测量验收。

## 8. 首次实现前需固定的参数

第一版支持的 fio 版本及 I/O 引擎、SQLite 驱动与发布方式、开发依赖版本、负载模板默认参数和项目许可证，需要在相关功能落地时明确。当前设计不以未验证的版本或虚构测试数据作为发布承诺。
