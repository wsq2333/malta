# 原生工作流托管：Argo

Malta 保存完整原生模板的不可变版本、参数与执行快照，通过后端适配器提交、查询、取消和恢复任务。**原生 Argo Workflow 或 WorkflowTemplate 的语义由 Argo 负责**；Malta 不维护自己的 DAG 语言、不转换工作流模板、不限制节点必须采用某种算子/依赖/重试形式。

结果解析可选。只保存模板时，Malta 记录后端执行状态和原始详情；`succeeded` 表示执行成功，`result_state=not_configured`、`verdict=not_evaluated`，不会将其算作测试通过或修复验证。配置 fio/JUnit/assertions 解析后才进入证据校验、指标对比与修复验证链路。旧 Plan、混合场景、Kubernetes Job 与历史记录保留。

## 页面与使用顺序

测试中心的“工作流测试”可按 **后端 / 执行入口** 筛选分页历史。入口展示未启用、待配置 Secret 或凭据文件已就绪；文件就绪不代表业务权限已经验证。没有可用入口时不能提交，但仍可管理原生模板。

发起顺序为 **用例版本 → 被测系统 → 执行配置 → 软件版本 → 参数 → 预检查 → 提交**。执行配置步骤选择 Argo 入口和匹配被测系统的配置。切换配置清除预检查；已提交运行固定后端、连接指纹和模板版本，不能迁移运行中的任务。

用例管理可导入Argo 原生 JSON，自动包裹为 Malta 用例；也可导入完整用例 JSON。Argo YAML 可先转换为等价 JSON，保存的仍是原生对象。当前只支持 Argo；保留 `implementations` 扩展结构，但仅接受 `implementations.argo`。

## 用例结构

最简用例 body：

```json
{
  "schema_version": "workflow/v1alpha1",
  "backend": "argo",
  "template": {"apiVersion": "argoproj.io/v1alpha1", "kind": "WorkflowTemplate", "spec": {}},
  "timeout_seconds": 600
}
```

以上 `spec` 需填成有效原生模板；空 spec 不代表可执行。完整示例：

- [Argo 原生模板](examples/workflow/native-argo.example.json)
- [带客户端/PVC 绑定的 Argo 用例](examples/workflow/case.example.json)
- [Argo 可选 JUnit 结果解析](examples/workflow/argo-evidence.example.json)
- [被测系统](examples/workflow/system.example.json)与[执行配置](examples/workflow/profile.example.json)

共同字段：`parameters`（可选的封闭 JSON Schema）、`defaults`、`dataset`（可选的数据预算声明）、`timeout_seconds`。未声明 Schema 时，参数使用原生 JSON 对象，由后端校验；未指定 timeout 时采用连接预算与 600 秒中的较小值。

推荐使用 `backend: argo` 配合 `template/bindings/results`；也可使用 `implementations.argo` 包含 `template`、可选 `bindings/results`，不能同时填写两种形式。执行配置选择具体实现，解析后的定义进入不可变快照。

可选 `bindings` 只替换已有的原生 workload spec JSON Pointer，值来自版本化 system/profile settings；不做字符串模板拼接。绑定后的模板按内容登记，原始用例版本保留不变。输入名 `malta_context` 保留，只有模板显式声明它时才注入；启用结果解析必须声明该输入。

Malta 只校验基本模板封装、backend/namespace 与已选执行配置一致、参数 Schema（如有）、预算和结果规则。原生模板中的节点调度、PVC、Secret、重试、嵌套 DAG、循环等由模板作者与后端管理；Malta 不会伪称预检查已验收现场挂载或固定客户端。Profile 的 node 可省略；提供时应与已登记环境元数据一致。

## 服务端入口与认证

当前通用配置见 [workflow-backends.example.yaml](../config/parameter/workflow-backends.example.yaml)，ConfigMap/Secret 挂载见[部署说明](../deploy/kubernetes/overlays/workflow/README.md)。真实值不提交 Git。

```sh
bin/malta serve --listen 0.0.0.0:8080 --data-dir /var/lib/malta \
  --workflow-config /etc/malta-workflows/workflow.yaml
```

每个 connection 包含 `id`、显式 `backend: argo`、`enabled`、URL、`auth`、执行范围和预算。缺省或不支持的 backend 明确报错；`token_file` 仍支持，但不能与 `auth` 同时设置。未启用连接可提前登记，不要求文件已存在或路由已填完。只有启用连接才创建执行适配器。

`auth.type` 支持文件 Bearer Token、OAuth2 账号密码和 Client Credentials。账号密码/client secret 由 Secret 挂载文件读取；Token/Refresh Token 留在内存，到期前续期，同一入口串行刷新。刷新令牌失效时重新认证；Secret 轮换使缓存失效；重启重新登录。明确 401 才刷新并重试一次，保持请求体与幂等身份；网络故障交由运行恢复逻辑处理。

`--management-only` 与 `--workflow-config` 互斥。旧执行器与工作流执行器的锁独立，不能据此保证跨执行器互斥；使用时需安排资源隔离，避免同时压测同一系统。URL 和认证配置由管理员控制，Profile 只能引用 connection ID。登录成功不证明 Argo 业务权限完整。

只有配置结果解析时才要求 `remote_results_root/local_results_root`：它们必须是执行 Pod 与 Malta Server 可见的同一 NAS 目录。当前不提供跨集群下载器或 OSS 产物解析器；native-only 模板可以自行使用 OSS 等存储，后端原始输出引用保留在运行详情中。

## 后端生命周期与恢复

| 操作 | Argo Server REST API |
| --- | --- |
| 准备模板 | 创建按内容寻址的 WorkflowTemplate，同名不覆盖 |
| 提交 | 确定性 Workflow 名称，使用已核验 spec 创建 Workflow |
| 恢复 | 按名称及 annotation 查询并校验 Workflow UID |
| 状态 | Workflow phase / node status |
| 取消 | terminate 后等待 Workflow 和 Pod node 终态 |
| 可选结果路径 | node outputs.parameters 的值 |

Argo 注册/提交使用原生 API 的校验；不在 Malta 重新实现 Argo schema。Workflow 使用登记模板的已核验内容副本，避免运行中模板被修改。提交参数只覆盖同名输入，保留模板默认参数和 artifact 输入；模板自身更短的执行时限也保留。root labels/annotations 纳入模板内容并传给 Workflow；保留前缀 `malta.dev/`。返回身份包含 `Workflow name.UID`，同名资源被删除重建时拒绝串接。若 Argo API 只返回压缩/外置 node status，需提供已展开的节点响应才可解析证据；原生运行状态仍可查询。

任何后端提交前先持久化意图。响应丢失后先查相同身份，不更换 request_id 重提；执行连接/路由变化时暂停恢复。过期任务请求取消；缺少后端确认不释放执行槽位。已启用解析时 `collect` 只回收缺失证据，不重跑 DAG。

Argo 原生重试/循环可运行。解析规则的 `node_id` 对应 node.name 去掉 Workflow 名称前缀后的逻辑路径；重试末尾 `(N)` 映射为 attempt=N。嵌套或展开任务需针对实际逻辑路径配置规则，不自动将多个实例聚合为一次性能结果；同一逻辑路径/attempt 存在歧义时拒绝解析。简单 DAG 例子使用任务名 `check`。

## 正确结果的约定

后端的输出登记 只证明“路径已登记”，不能证明文件真实存在或测试通过。每个结果输出 JSON 应使用 `malta.evidence/v1`，包含：

```json
{
  "schema_version": "malta.evidence/v1",
  "workflow_version_id": "从 malta_context 获取",
  "run_id": "Argo Workflow name.UID",
  "node_id": "Argo 逻辑节点路径",
  "stage_run_id": "实际 stage ID",
  "attempt": 0,
  "eviction_count": 0,
  "snapshot_hash": "从 malta_context 获取",
  "environment_id": "从 malta_context 获取",
  "software_version_id": "从 malta_context 获取",
  "parser": "junit",
  "assertions": {},
  "files": [{"path": "junit.xml", "sha256": "原始文件 SHA-256", "size": 123}]
}
```

Argo 从 Workflow 名称、UID 和 Pod 的 node-id annotation 构造身份（见 Argo 证据示例）。不能写死或使用上次值。先完成原始报告写入，再在同一目录用临时文件 + rename 原子发布 manifest。完整示例包括这段逻辑。

验证内容包括运行、版本、节点、attempt、驱逐次数、快照和环境身份；路径必须位于批准的共享根目录并包含当前 run ID；拒绝路径穿越和符号链接；校验所有原始文件大小与 SHA-256，按内容哈希归档。总证据上限 32 MiB（连接可收紧），最多 64 个文件。超限不截断为成功。

| 解析器 | 输入与判定 |
| --- | --- |
| `fio-3.33` | `fio.json`；仅接受一个汇总 job，校验 fio 版本、错误及实际 rw/bs/numjobs/iodepth/ioengine/direct/runtime/allow_file_create 等选项；从原始数据计算指标 |
| `junit` | `junit.xml`；逐 testcase 统计，空报告、重复标识、损坏 XML 无效；failure/error 或 skipped 不判通过 |
| `assertions-v1` | 必须列出并返回命名布尔断言；缺少断言无效、false 为失败；不接收自报性能指标 |

对于测试断言失败，建议算子包装器先完整写入报告与 manifest，并确保后端完成输出登记；Malta 会依据报告给出失败结论。阶段执行失败时也会尝试收集已登记证据；未登记或未写完的报告仍视为缺失，不能用后端失败状态伪造测试结果。

这些校验防止串结果、错版本和报告损坏，但无法证明不可信算子的自报断言是真实现场事实；业务验证命令仍需用例审查。三种状态分别展示：后端执行状态、证据 `complete/missing/invalid`、结论 `passed/failed/inconclusive`。已配置结果规则但缺少证据时结论为无法判定；未配置结果规则时显示 `not_configured / not_evaluated`。

比较要求双方全部必要证据验证通过；模板、环境、执行配置、参数和结果契约一致。指标按节点/操作/名称/单位逐项对应，上传、预热与读取吞吐不混合。单次结果只给变化值与变化率，不自动断言性能回归。

## Agent / CLI

所有命令走相同认证 API；设置 `MALTA_TOKEN`，不要将 Token 放入用例 JSON。

```sh
bin/malta --server http://127.0.0.1:8080 workflow connections
bin/malta --server http://127.0.0.1:8080 workflow save cases --file docs/examples/workflow/case.example.json
# 根据返回 ID 和实际环境填写本地系统、执行配置、selection 文件，再执行：
bin/malta --server http://127.0.0.1:8080 workflow save systems --file /tmp/system.json
bin/malta --server http://127.0.0.1:8080 workflow save profiles --file /tmp/profile.json
bin/malta --server http://127.0.0.1:8080 workflow records cases --id CASE_ID
bin/malta --server http://127.0.0.1:8080 workflow preflight --file /tmp/selection.json
```

预检查返回 `ready`、检查列表、`id`、`snapshot.hash`，有效期 10 分钟。仅在 ready=true 且现场条件已确认时填写提交文件：

```json
{"preflight_id":"检查返回的 id","snapshot_hash":"检查返回的 snapshot.hash","request_id":"本任务固定唯一 ID"}
```

```sh
bin/malta --server http://127.0.0.1:8080 workflow start --file /tmp/start.json
bin/malta --server http://127.0.0.1:8080 workflow lookup REQUEST_ID
bin/malta --server http://127.0.0.1:8080 workflow get RUN_ID
bin/malta --server http://127.0.0.1:8080 workflow list --limit 20 --offset 0
bin/malta --server http://127.0.0.1:8080 workflow cancel RUN_ID
bin/malta --server http://127.0.0.1:8080 workflow collect RUN_ID
```

API 根路径 `/api/v1/workflow`。对应 `connections`、`records/{cases|systems|profiles}`、`preflight`、`runs`、`requests/{request_id}`、`runs/{id}/cancel`、`runs/{id}/collect`、`compare`。GET `runs/{id}/export` 导出记录；GET `runs/{id}/artifacts/{name}` 下载已归档文件。CLI `compare --file` 输入 `{"baseline":"RUN_A","candidate":"RUN_B"}`。

## 问题关联

运行详情可生成带软件/硬件/时间的 Issue JSON，编辑后提交；也可输入已有 issue_id，登记 `discovery` 或 `verification`。一个问题可以关联多次运行，一次运行可以关联多个问题。

```json
{"issue_id":"issue-example","role":"discovery","node_id":"可选的结果节点 ID","note":"实际失败现象"}
```

`workflow link RUN_ID --file /tmp/link.json` 对应 POST `runs/{id}/issues`；`workflow issues RUN_ID` 查询关联。GET `issues/{issue_id}/runs` 提供反向追踪。关联保存当时的问题修订号；展示同时读取问题当前解决状态。关联不可覆盖，同内容重试幂等。

修复验证要求运行已通过证据校验，并存在同用例、同环境的发现记录；允许软件版本升级。它证明这次测试通过，不自动证明全部问题已解决。用户/Agent 仍通过原问题 API 保存新修订、解决方式和 resolved 状态。


## 存储与验收边界

继续使用 `workflow_records`、`workflow_preflights`、`workflow_runs`、`workflow_issue_links`；不迁移或改写既有业务数据。已有非 Argo 配置不能用于本版本提交。更新连接配置后必须重新预检查；已有历史记录保留，不转换或重新执行。保存完整模板、选定实现、参数及执行快照；凭据不进入快照、API 响应或错误正文。

列表 API 支持 `GET /api/v1/workflow/runs?backend=argo&connection=example-argo&limit=20&offset=0`，过滤在分页之前执行。

代码验收使用开发机上的模拟 OAuth 和 Argo REST 接口与共享证据目录。它验证协议、恢复和解析逻辑，不等价于真实集群授权/CSI/控制器验收；本次代码变更不自动更新现有集群镜像，也不创建账号或真实 Secret。

协议依据：[Argo REST API](https://argo-workflows.readthedocs.io/en/latest/swagger/)、[Argo 模板变量](https://argo-workflows.readthedocs.io/en/latest/variables/)、[Keycloak Grant Types](https://github.com/keycloak/keycloak/blob/main/docs/guides/securing-apps/partials/oidc/supported-grant-types.adoc)。


## Agent 提示词

使用[工作流测试提示词](agent-workflow-benchmark-prompt.md)填写 Argo 用例、执行配置和验收条件。公开仓库不提供现成的 OSS/缓存预热/fio 三阶段工具镜像；使用此流程前需要自行提供对应 Argo 模板与工具，并逐项验证其证据输出。
