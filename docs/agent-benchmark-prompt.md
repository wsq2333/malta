# Malta 性能测试 Agent 提示词（填空模板）

请通过 Malta 完成以下性能测试。将【…】替换为实际值；不适用项填写“无”。本模板是给 Agent 的任务说明，不是可直接提交的 Plan JSON。

## 1. 任务与范围
- 服务地址：{{MALTA_SERVER}}（必须能从 Agent 执行环境访问）。
- 任务目的：【例如验证 Curvine 新版本的 4 MiB 文件顺序读性能】。
- 执行方式：【核验通过后直接执行 / 仅生成计划】。
- 执行器：【kubernetes / local】，Request ID：【本次实验唯一且重试不变的 ID】。
- 计划、会话记录和结果保存目录：【Agent 工作目录中的绝对路径】。
- 准备数据授权：【允许 / 不允许】在指定测试目录创建和写入测试文件。只读负载也可能先写入准备数据。
- 用户预算：准备数据总量上限【字节】，从提交开始的总等待/执行时限【例如 10m】，最多新实验次数【默认 1】；超时取消本任务并确认终态。

## 2. 计划来源（二选一，删除另一项）
A. 使用已有用例：
- Case ID：【用例 ID】，固定 revision：【正整数，不使用“最新”】。
- 预期软件 version_id：【ID / 以固定用例版本为准】。
- 预期环境 environment_id：【ID / 以固定用例版本为准】。
- 原样使用该修订中的目标路径和负载；发现版本或环境不符时，先报告差异，不偷偷修改旧用例。

B. 创建新计划：
- 用例名称/描述：【名称与目的】。
- 文件系统：【名称】，软件 version_id：【已登记 ID】，environment_id：【已登记的执行快照 ID】。
- target：【执行器中的专用测试目录；Kubernetes 通常位于 /bench】，resource_group：【稳定的被测资源标识】。
- 普通 I/O：operation【read/write/randread/randwrite/randrw】，file_size_bytes【每文件字节数】，files_per_job【每 worker 文件数】，block_size_bytes【每次 I/O 字节数】。
- matrix.numjobs【整数数组】，matrix.iodepth【整数数组】，repeats【独立重复次数】。
- runtime_seconds【每次测量秒数】，warmup_seconds【预热秒数】，ioengine【引擎】，direct【true/false】，cache_state【经核实的缓存声明】，read_percent【读比例】，fsync_every【次数，0 表示不按次数同步】，seed【整数】。
- 如使用混合场景：用 ScenarioSpec 文件【路径】替代上面的普通 I/O 参数，明确阶段、子负载和配比；不要将完整 Plan 当作 ScenarioSpec。
- 用例保存：【不保存 / 新建用例 / 为指定 Case ID 保存新 revision】；若更新，Case ID【ID】、base_revision【当前修订】、change_note【说明】。

## 3. 对比与问题跟踪
- 对比参照：【已有 baseline Run ID / 无】；这是运行 ID，不是基线名称。
- 设为基线：【否 / 是，名称为…】；仅在完整成功后执行。
- 发现问题时自动登记 Issue：【否 / 是】，自定义 type：【功能/性能/稳定性/其他】，reporter：【名称】。
- 性能异常判定：【指标、方向与阈值 / 只报告变化，不自动判定回归】。

## 4. Agent 执行约定
1. 先读取 `malta version` 和服务 `capabilities`，确认执行器、功能和预算。如可访问项目仓库，遵循 `skills/malta-benchmark/SKILL.md`。CLI 使用 `--server URL --format json --non-interactive`；认证从 `MALTA_TOKEN` 获取，凭证不写入模板、计划或报告。
2. 用 `case get CASE_ID REVISION`、`environment show ID`、`version show ID` 核对对应信息。软件升级不能只改标签：使用真实 commit/build 与新的不可变版本快照；必要时保存新用例修订。已有信息先查询，只有执行所需信息仍缺失或有冲突时才提问。
3. 新计划由 `malta plan --spec SPEC --environment ENV --version VERSION --target TARGET --resource-group GROUP --out PLAN` 生成；混合场景以 `--scenario FILE` 替换 `--spec SPEC`。不手工伪造 trials/hash。注册快照发生冲突时不覆盖旧记录。
4. 核对展开后的实验数、准备字节数、测量/预热时间，并为准备与结果回收留出时间。普通 I/O 按所有 trial 的 file_size_bytes × files_per_job × numjobs 累加准备预算；混合场景遵循 Malta 的 Plan.Budget 规则。用户预算比服务上限更小时采用较小值。`run start` 没有单次预算覆盖参数，不能虚构 `--timeout` 或 `--max-bytes` 来绕过服务配置。
5. Kubernetes 核对 `cluster`、`node`、`benchmark_pvc` 与执行器一致，Runner 与 Server 版本匹配，并核实实际 UID/GID、CPU/内存和卷可写性。有已授权的执行器配置与集群访问时运行 `malta kubernetes check --config EXECUTOR --plan PLAN`；该检查只读，不证明卷可写。缺少这些访问时明确未独立验证的项目，依赖服务检查；不能声称已通过独立预检查。用例 JSON 中随意增加 uid/gid 不会改变 Runner 身份。
6. “仅生成计划”时输出文件与核验结果后停止，不写入服务。“直接执行”时，在范围与预算核验通过后提交，不重复索要已明确给出的执行授权。先持久化 server、executor、request_id、计划 hash 和固定 case_ref，再提交：
   - 固定用例：`malta --server URL --format json --non-interactive run start --case CASE_ID --case-revision REVISION --request-id REQUEST_ID --source agent --executor EXECUTOR`
   - 完整计划：同上，以 `--plan PLAN` 替换 `--case` 与 `--case-revision`。两种来源不能同时传。
7. 记录返回的 Run ID。提交响应丢失时先 `run lookup --request-id REQUEST_ID`，核对计划/用例、source 与 executor；确认不存在后才用原 ID 重试。冲突时检查原因，不换 ID 重复发起。每个新增实验使用独立 ID，且不超出授权次数。
8. 用 `run get`、`run logs`、`run wait RUN_ID --timeout 5m` 跟踪；单次等待时间不得超过用户总时限的剩余时间；等待超时不代表失败，也不会自动取消。达到用户总时限时执行 `run cancel RUN_ID` 并确认终态。仅 `status=succeeded` 且 `attempt.result_state=complete` 算完整测量；不要把提交成功当成测试成功。
9. 当前独立 mdtest 元数据负载仅支持规定格式的结果导入，不能当作托管执行提交；混合场景只使用 capabilities 和实现支持的操作。不擅自改部署、权限、挂载、缓存策略或扩大测试范围。
10. 完成后 `result export RUN_ID --out NEW_DIRECTORY` 保存证据；按已填写授权进行 `compare --baseline BASELINE_RUN --candidate RUN_ID`、`baseline set --name NAME --run RUN_ID`。先报告可比性与 warnings，再解释指标变化；最高已测点不等于系统绝对极限。
11. 若授权登记问题，先查询去重，再用 `issue save --file ISSUE_JSON`。包含稳定 issue_id、base_revision、title、type、description、software、hardware、occurred_at（含时区）、resolved=false、resolution=""、reporter，并关联 run_id、version_id、environment_id。区分事实与推测；只有验证修复后才标记解决，更新使用最新 revision，冲突先重新读取。

## 5. 最终请返回
- Run ID、状态、result_state、固定用例修订、软件/环境 ID、计划 hash、工具及 Malta 版本。
- 读写类型、文件/块大小、并发/深度、重复次数，以及带单位的指标中位数与 Min–Max。
- 对比的可比性、差异、warnings 和服务返回的变化率；不可比时不归因于软件升级。
- 原始证据目录、运行页面链接；若有问题，附 Issue ID、状态与证据。无法取得的指标明确写“未采集”。
- 已确认事实、待验证原因和建议的下一步；建议不自动触发新测试。
