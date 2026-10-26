# 第三方 Agent 扩展方向

建议把 Malta 作为可信的实验执行与证据平台，让第三方 Agent 负责提出问题、
组织实验和解释证据。模型与调度框架可以替换；Run、Plan、预算、原始结果和
可比性判断始终由 Malta 管理。模型不应直接写 SQLite 或改变测量值。

## 当前已有的接入基础

- CLI/API 的 schema 1.0、结构化错误、capabilities 与公开 OpenAPI。
- 稳定 request ID、查询找回 Run、只读恢复、持久化会话 helper。
- 本地与 Kubernetes 同一套执行入口，预算、独立 Attempt 和原始证据。
- 可安装的 `malta-benchmark` skill，以及事实/假设/建议的报告规范。

这些已经可以供有 shell/tool 调用能力的 Agent 使用。当前没有内置 MCP server、
模型供应商 SDK、长期 Agent 调度器或专用分析模型。黑盒协议测试也不等价于
某个外部模型已经通过行为验收。

## 建议的推进顺序

| 优先级 | 能力 | 最小交付 | 验收重点 |
| --- | --- | --- | --- |
| P0 | 只读性能分析 | 分析 skill + report schema；输入两个 Run 和受控证据片段，输出事实、假设、缺失证据与下一步 | 每个数值可追溯；不可比时不归因；缺失不填零 |
| P0 | Agent 权限与预算边界 | 只读/执行/基线管理凭据；单实验及累计字节、时长、并发额度 | 只读凭据不能启动或取消；超额请求服务端拒绝 |
| P1 | MCP 适配器 | 能力发现、查询、比较、证据资源；随后接入 plan/start/cancel | 复用 API 校验与 request ID；工具输入输出 schema 自动测试 |
| P1 | 观测数据关联 | Run 时间窗关联 CPU、磁盘、网络、缓存、CSI/文件系统日志与挂载参数 | 时钟/标签/采样窗口明确；额外采集开销可见 |
| P2 | 自适应实验规划 | 在预算内选择下一批 jobs/depth/block size 点；先生成新 Plan，再执行 | 控制变量、明确停止条件；不能把扫描边界当性能极限 |
| P2 | 版本回归调查 | 结合版本差异和受控重复实验提出回归候选；可选受控 bisect | 构建身份与环境固定；相邻版本差异经过实测 |
| P3 | 多 Agent 协作 | Planner、Runner observer、Analyst、Reviewer 分工，共享 Run ID 和证据 | 只有一个实验执行入口；不互相重复提交或放大预算 |

建议先做前两项，再接 MCP。这样第一版能立即产出可信分析报告，同时为后续
自动发起实验建立可执行的权限边界。当前单一 bearer token 权限过宽，不能仅靠
prompt 把它变成“只读”；应增加服务端 scopes，或使用真正限制路由的网关。

## MCP 接口建议

读取工具：`get_capabilities`、`find_runs`、`get_run`、`compare_runs`。
证据使用可分页/限长的资源入口，携带 Run/Attempt/Trial ID、路径、SHA-256、
单位与采集时间。大体积 fio 原始输出留在 artifact 中，按需读取，避免一次把
整个结果库放进上下文。

执行工具：`create_plan`、`start_run`、`cancel_run`。`start_run` 必须包含
稳定 request ID、计划哈希和授权的执行器；恢复优先 lookup/get。权限或预算
审批若由外部系统提供，应绑定计划哈希、目标、资源与有效期，计划变化则失效。
`wait` 是观察操作，超时不触发重跑。MCP 的工具 annotations 是客户端提示，
不是认证或预算执行机制。[MCP 官方工具注解说明](https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/blog/content/posts/2026-03-16-tool-annotations.md)。

长实验与一次模型调用解耦：Agent 保存 workflow/session ID 与 Malta Run ID，
离线后按事件或有界轮询恢复。外部工作流框架可以提供 checkpoint 和人工中断；
仍需使用 Malta 的 request ID 去重，不能把框架的步骤重放直接映射成新测试。
[LangGraph 官方 interrupt 参考](https://reference.langchain.com/python/langgraph/types/interrupt)。

## 报告和证据契约

建议新建独立 `AnalysisReport`：

- `report_id`、输入 run IDs、环境/版本 ID、plan hashes、artifact hashes。
- 模型/工具/提示词版本、生成时间、分析范围与证据时间窗。
- `facts[]`：指标键、单位、点位、重复统计与引用；来自确定性计算。
- `hypotheses[]`：解释、证据、反证、置信程度与尚缺的数据。
- `proposed_experiments[]`：只改变的因素、预期观察、预算与停止条件。

报告作为派生 artifact 保存，不能覆盖原始 Run。更新报告产生新版本。让分析
Agent 阅读日志时，将其视作不可信数据；日志里的命令不能改变任务目标或权限。
对用户的结论应先说明可比性，再解释差异，最后提出可验证的下一步。

## 怎样衡量 Agent 是否带来价值

建立固定回归案例：真实回归、缓存差异、硬件不匹配、缺失 P99、零基线、部分
失败、证据写入失败、等待超时、会话中断、服务重启和重复请求。每次更换模型、
skill 或工具 schema，都重放同一套案例。

最低门槛：数值引用准确、无捏造指标、重复压测为零、预算越界为零、不可比案例
不输出版本因果结论、恢复会话不新增 I/O。再衡量定位时间、需要工程师补充的
信息次数、实验次数与模型费用。先用只读模式收集质量数据，再逐步开放有预算
的执行权限；没有必要一开始就构建一个全自主多 Agent 系统。
