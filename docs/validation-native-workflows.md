# 原生工作流与多后端认证验收

日期：2026-11-03。本轮只修改 Malta 代码、示例和文档；在指定开发机的独立目录 `/tmp/malta-multibackend-20261103` 验证，没有更新 sample dev 集群，也没有创建真实认证 Secret。

## 自动化验证

开发机执行并通过：

- `go test -p 1 ./...`：全量 Go 测试。
- `go vet ./...`：静态检查。
- `go test -race ./internal/backend/... ./internal/workflow`：认证、HTTP、UCE、Argo、运行生命周期并发检查。
- `pnpm build`：TypeScript 检查和生产构建。
- `node --test tests/*.test.mjs`：12 项现有前端逻辑测试。

最后补充原生参数保留、外置节点状态测试以及参数编辑器返回步骤时的状态修复后，重新运行 Argo/workflow race 测试、前端构建及 12 项前端测试，全部通过。构建仍提示现有图表 chunk 超过 500 kB，不影响构建完成。

覆盖范围：OAuth 密码授权、Client Credentials、提前刷新、refresh token 失效重新登录、Secret 轮换、并发刷新合并、认证错误脱敏、重定向拒绝、401 仅重试一次且保留幂等键；原生模板保存、多后端实现选择、连接筛选分页、历史连接指纹兼容；Argo 模板不可覆盖、Workflow UID 防串接、提交重试、取消、证据解析；保留原生默认参数/artifact 输入和更短时限；节点状态外置时保持执行可观察但拒绝伪造证据。

初次并行全量测试中，未修改的 native I/O pipeline 测试出现一次测量窗口不匹配；本机单独连续运行三次通过，开发机串行全量复核通过。未修改该测量断言。

## 隔离服务与 UI

开发机本地启动模拟 UCE/Argo HTTP 服务及独立 Malta 实例，仅监听 loopback；使用占位凭据，没有请求真实执行后端或发起 Kubernetes Pod。

- UCE 模拟运行：`workflow-087f35549b0b2545578e7879`。
- Argo 模拟运行：`workflow-4266f4dc207b40f28f6ea49d`。
- 两者均验证为 `succeeded / not_configured / not_evaluated`，没有生成虚假的测试通过结论。
- 浏览器确认列表分别显示后端/入口、执行/证据/验证状态；详情明确“执行成功不等于测试通过”；向导可在 UCE/Argo 间切换，并过滤对应执行配置、显示入口和认证状态。
- 原始模拟验证摘要在开发机 `ui-preview/validation.json`。这些运行不是性能基线。

## 真实接入仍需验收

示例配置默认禁用。部署新版本后，需提供 Secret、填写实际连接及执行范围，确认 Keycloak client/grant 和 UCE/Argo API 权限，再做真实冒烟测试。ConfigMap/Secret 挂载方法见 [部署说明](../deploy/kubernetes/overlays/workflow/README.md)。

未在真实 Argo 控制器上验证 admission/defaulting、节点状态外置展开、CSI 与产物目录权限。当前 Argo 模板内容校验采用严格哈希；若现场 admission 修改 spec，会拒绝提交，需要根据实际变换制定兼容规则，不应绕过校验。启用结果解析仍要求批准的共享结果目录与可验证的 evidence manifest；原生-only 模板不要求 Malta 参与产物生成。
