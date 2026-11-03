# Agent 工作流测试提示词（Argo）

填入【…】后交给 Agent。实际环境值只存入被 Git 忽略的 `config/workflow/`；凭据通过 Secret 提供。这是任务提示词，不是可直接提交的 JSON。

```text
请通过 Malta 发起一次 Argo 工作流测试。

服务地址：【MALTA_URL】
目的与验收条件：【测试内容、正确性要求、指标和证据】
用例 ID / 修订：【CASE_ID / REVISION】
被测系统 ID / 修订：【SYSTEM_ID / REVISION】
执行配置 ID / 修订：【PROFILE_ID / REVISION】
软件版本 ID：【VERSION_ID，核对实际部署后填写】
参数：【参数 JSON，须符合用例 Schema】
Request ID：【唯一 ID；提交重试保持不变】
允许写入范围：【专用目录/对象前缀】
最大准备数据量：【字节数】
总等待与执行时限：【秒】
是否允许直接提交：【是/否】
清理授权：【无/精确范围】
是否关联 Issue：【无/Issue ID 与 discovery 或 verification】
计划和结果保存目录：【本地忽略目录】

1. 查询 Malta capabilities、workflow connections 和指定记录。确认后端为 argo、版本及实际执行环境一致；缺失信息先查询，不猜测 ID。
2. 核验完整 Workflow/WorkflowTemplate、节点依赖、参数、镜像、Secret 引用、PVC、读写范围及已有文件读取方式。不得暗中替换测试语义。
3. 配置结果解析时，确认执行 Pod 和 Malta 的结果目录为同一共享存储；模板声明 malta_context，输出匹配 fio-3.33、junit 或 assertions-v1 的 evidence manifest。只返回后端 Succeeded 不能代替测试通过。
4. 生成 selection JSON（case/system/profile 的 id 与 revision、version_id、parameters），执行 workflow preflight --file。预检查不能代替现场权限与挂载验证；列出未验证项，阻塞项未解决不提交。
5. 先 workflow lookup REQUEST_ID 防止重复。授权且检查通过后，以返回的 preflight_id 和 snapshot.hash 生成 start JSON，再 workflow start --file。响应丢失仍先查询同一 Request ID，不新增实验。
6. 使用 workflow get 跟踪；到总时限执行 workflow cancel 并确认终态，无法确认时明确报告。workflow collect 只重试证据回收，不重跑 DAG。
7. 报告 Malta/Argo Run ID、固定快照、参数、后端状态、result_state、verdict、原始证据和未验证项。有结果契约时仅在证据完整且验收通过后报告测试成功；没有解析契约时报告“执行成功，未评估测试结论”。
```

CLI 使用 `--server URL --format json --non-interactive`，Malta 认证通过 `MALTA_TOKEN` 提供。不要虚构 `workflow wait`、超时或数据量命令参数。示例参见[工作流说明](workflows.md)。公开示例中的 JUnit 检查只演示回收协议，不是实际文件系统性能测试。
