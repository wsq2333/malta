# 工作流测试：UCE 适配

Malta 管理用例、执行快照、结果证据和问题；UCE/Yarlung 调度 DAG。当前只实现 UCE，`internal/backend.Backend` 保留其他框架的接入边界。`backend: argo` 会明确拒绝，不会悄悄降级为其他执行方式。旧 Plan、场景、Kubernetes Job、基线和导入格式继续保留。

## 页面入口

保持五项侧栏，不增加一级模块：

| 页面 | 入口与用途 |
| --- | --- |
| 用例管理 | **工作流用例**：保存完整 EP、说明、参数 Schema、默认值、环境绑定和结果契约；编辑生成新修订 |
| 环境与版本 | 硬件环境下 **被测系统与执行配置**：引用已登记硬件快照、软件版本；保存允许使用的 UCE 路由、客户端节点和 PVC 配置 |
| 测试中心 | **工作流测试**：七步发起向导、分页运行历史、执行/证据/验证三个状态、结果和原始文件、同条件对比 |
| 问题管理 | 保留现有问题生命周期；详情显示关联工作流的发现与修复验证记录 |

原自动化页面的虚构数据已被真实工作流页面替换。性能 DAG 与自动化 DAG 共用工作流入口，以结果解析器区分；工作流比较暂为显式选择两次运行，不进入旧 Plan 基线目录的统计。

发起顺序：**用例版本 → 被测系统 → 执行配置 → 软件版本 → 参数 → 预检查 → 提交**。前三项都固定到不可变修订；软件版本必须与选中的系统修订一致。升级集群后，应登记新版本并保存系统的新修订，再发起测试。选择版本不会更改集群。

列表摘要显示名称、版本和状态；模板、完整快照和原始后端状态默认折叠。用例与环境编辑器支持 JSON 导入；参数向导根据 JSON Schema 提供基础类型、枚举、JSON 对象/数组输入，服务端执行最终校验。

## 服务端配置与边界

从 [`config/parameter/workflow.example.yaml`](../config/parameter/workflow.example.yaml) 复制真实配置。只有示例文件可提交 Git。真实 Token 通过文件引用，每次 HTTP 请求重新读取，支持 Secret 轮换；Token 不写入执行快照或响应。连接 URL、结果路径映射和预算由管理员配置，用户提交的 Profile 只能选择连接 ID，不能提供 URL 或凭据。

```sh
bin/malta serve --listen 127.0.0.1:8080 --data-dir .malta \
  --workflow-config config/parameter/workflow.yaml
```

启用此参数时不可使用 `--management-only`。不需要同时配置旧 Job 执行器或本地 fio。单项/场景测试可以继续使用旧执行器，但旧执行器与 UCE 的集群互斥锁彼此独立；请不要让它们同时压测同一集群。

配置必须对应已验收的 UCE 路由、执行 namespace、subtype、客户端、结果 NAS PVC。`remote_results_root` 是 UCE 输出根目录，`local_results_root` 是同一目录在 Malta Server 中的挂载路径。当前实现通过共享文件系统回收，**不提供跨集群下载器或 OSS 产物读取器**。

预检查验证完整 EP、依赖、参数类型、预算、节点选择和结果 PVC 挂载、已登记版本与环境、服务端结果目录与 Token 文件可访问性。它不创建探测 Pod，不验证现场 CSI/挂载权限、UCE 路由是否实际落到声明 namespace、客户端软件是否与登记一致。管理员需先完成这些验收；页面保留明确提示。模板中的其他 PVC、OSS Secret、权限、工具镜像和业务命令由用例作者负责。

同一连接范围内提交模板具备执行容器的能力，配置连接与编辑模板应仅授予可信用户；现有 API Token 仍是统一的访问控制，没有新增多租户角色权限。

## 用例与模板

示例文件：

- [`case.example.json`](examples/workflow/case.example.json)：完整单节点 UCE EP、JUnit 和原子发布证据示例。仅验证算术断言与回收协议，**不是文件系统正确性或性能测试**。
- [`system.example.json`](examples/workflow/system.example.json)：已登记的环境和软件引用。
- [`profile.example.json`](examples/workflow/profile.example.json)：UCE 路由、允许的系统和结构化绑定值。
- [`selection.example.json`](examples/workflow/selection.example.json)：提交预检查的选择。

执行定义 `workflow/v1alpha1` 保存：

- `template`：完整 EP。UCE Schema 副本固定到 API Server 源码 `0506eff`，注册时仍由目标 API Server 做权威校验。
- `parameters`：封闭对象 JSON Schema，`additionalProperties: false`；不支持 `$ref` 或 `$dynamicRef`。
- `defaults`：用例默认输入。EP 中声明的默认值也应显式放到此处，避免快照漏记实际输入。
- `bindings`：将系统/执行配置中的字段写入 EP 已存在的 JSON Pointer。只允许替换 operator manifest 的 spec 字段，不执行任意文本插值。
- `results`：按 `node_id + output` 指定解析器、必要断言及 fio 的实际选项校验。
- `dataset`：`file_size_parameter × file_count_parameter` 是唯一准备数据量，不对每个读取阶段重复累计；不含副本、缓存和元数据开销，不是磁盘配额强制器。
- `timeout_seconds`：从 Malta 接受提交开始计算执行期限；后端终态后证据回收另有最多 120 秒宽限期。

当前支持 paths-v1、单层 DAG、最多 64 个节点。用 `input_path` 声明阶段依赖；拒绝 `node_output` 混用。所有阶段应固定到 Profile 中声明的客户端，并挂载指定 RWX 结果卷。准备/预热/读取业务脚本仍由完整 EP 的容器定义承担；Malta 不把任意业务脚本自动转成 DAG。

例如 `上传 OSS → 预热 Curvine → fio 读取`：三个节点分别声明输出 JSON，后续节点用 `input_path` 引用前序输出；每个必要阶段设置结果规则。准备阶段可使用 `assertions-v1` 校验文件数量/字节数/缓存就绪等业务条件；fio 节点使用 `fio-3.33`，只读已准备数据并设置 `allow_file_create=0`。**本次不内置 OSS 上传或 Curvine 预热实现**；需要将经验证的工具镜像、脚本、凭据引用和卷配置写入 EP。

模板中的 `malta_context` 必须是声明的 JSON 根输入，并传给需要发布证据的算子。用户参数不能覆盖它。服务注入 Malta run ID、UCE workflow version ID、快照哈希、环境 ID 和软件版本 ID。

## 控制步骤、恢复和取消

```text
保存运行与请求身份
  → 准备执行定义：POST /v1/workflow-versions（完整 EP）
  → 提交运行：POST /v2/workflow-runs（yarlung、参数、timeout、幂等键）
  → UCE 执行原生 DAG
  → Malta 回收、归档并验证每个必要阶段结果
```

EP 注册是 Malta 的第一个控制步骤，不是放在待注册 DAG 中的 Pod 算子。最终 EP 的根版本身份由内容派生，配置绑定变化会得到新的版本身份。

提交前先持久化请求与快照；HTTP 响应丢失后按相同幂等键查询，必要时以相同内容重试。`request_id` 不得换成新值来“重试”未明确失败的请求。服务重启后恢复未结束运行，不重新生成请求。连接配置变化会暂停恢复并提示还原，不把旧任务转投新集群。

同一集群的工作流运行串行占用执行槽位，后端不可达时不会释放。取消必须等后端终态、取消标志和取消任务分页均确认；提交结果不明的任务保留待确认，不贸然释放。`collect` 只重试缺失证据的回收，不重跑 DAG。

当前模板要求 `maxRetry/retryLimit=0`。这不保证 Pod 永不因基础设施驱逐而重启：证据还必须匹配 `eviction_count`。如果业务不能安全重复，应在算子中以独立测试目录和业务幂等键限制副作用。

## 正确结果的约定

UCE 的 output registration 只证明“路径已登记”，不能证明文件真实存在或测试通过。每个结果输出 JSON 应使用 `malta.evidence/v1`，包含：

```json
{
  "schema_version": "malta.evidence/v1",
  "workflow_version_id": "从 malta_context 获取",
  "run_id": "UCE 运行 ID",
  "node_id": "模板中的 definition_node_id",
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

在实际 UCE stage 渲染上下文中获取 run/stage/attempt/eviction_count，不能写死或使用上次值。先完成原始报告写入，再在同一目录用临时文件 + rename 原子发布 manifest。完整示例包括这段逻辑。

验证内容包括运行、版本、节点、attempt、驱逐次数、快照和环境身份；路径必须位于批准的共享根目录并包含当前 run ID；拒绝路径穿越和符号链接；校验所有原始文件大小与 SHA-256，按内容哈希归档。总证据上限 32 MiB（连接可收紧），最多 64 个文件。超限不截断为成功。

| 解析器 | 输入与判定 |
| --- | --- |
| `fio-3.33` | `fio.json`；仅接受一个汇总 job，校验 fio 版本、错误及实际 rw/bs/numjobs/iodepth/ioengine/direct/runtime/allow_file_create 等选项；从原始数据计算指标 |
| `junit` | `junit.xml`；逐 testcase 统计，空报告、重复标识、损坏 XML 无效；failure/error 或 skipped 不判通过 |
| `assertions-v1` | 必须列出并返回命名布尔断言；缺少断言无效、false 为失败；不接收自报性能指标 |

对于测试断言失败，建议算子包装器先完整写入报告与 manifest，并确保 UCE 完成输出登记；Malta 会依据报告给出失败结论。阶段执行失败时也会尝试收集已登记证据；未登记或未写完的报告仍视为缺失，不能用后端失败状态伪造测试结果。

这些校验防止串结果、错版本和报告损坏，但无法证明不可信算子的自报断言是真实现场事实；业务验证命令仍需用例审查。三种状态分别展示：后端执行状态、证据 `complete/missing/invalid`、结论 `passed/failed/inconclusive`。后端成功但无证据时结论是无法判定。

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

## 存储与后续扩展

新增独立表 `workflow_records`、`workflow_preflights`、`workflow_runs`、`workflow_issue_links`，启动时幂等创建，不改旧 Plan/Run 记录。用例/系统/执行配置按 kind + id + revision 存不可变版本；run 保存完整快照；原始证据归档在数据目录的 `workflow-artifacts/`。

Argo 后续实现新的 Backend，以及原生模板校验/绑定和输出映射；不将 UCE EP 强行翻译成 Argo 模板。共享运行身份、版本、参数、证据与问题关联可以复用。

当前验收通过模拟 UCE 的 HTTP 契约和本地共享目录完成；真实 sample dev 的认证、路由、挂载和生产者模板仍需单独联调。此功能代码不自动更新集群部署。
