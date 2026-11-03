# Argo 工作流验收范围

公开版本只保留 Argo 执行后端。测试使用开发机的模拟 OAuth、Argo Server REST 接口及临时共享结果目录，不创建真实集群工作负载。

验证范围：原生模板与不可变版本、显式后端校验、参数/预算/环境绑定、认证刷新、注册/提交/查询/取消、丢失响应恢复、Workflow UID 防串接、证据 SHA-256 和身份校验、JUnit/fio/断言解析、Issue 发现与修复验证关联。UI 构建和现有前端逻辑测试单独执行。

真实接入仍需验收 Argo 权限、admission/defaulting、展开的 node status、CSI、共享目录及运行身份。严格内容哈希可能拒绝被 admission 改写的模板，不应绕过校验。默认示例关闭执行入口；配置指南见[部署说明](../deploy/kubernetes/overlays/workflow/README.md)。

## 2026-11-03 开发机验证

在独立临时目录完成：

- `go test -p 1 ./...`：全量通过。
- `go vet ./...`：通过。
- `go test -race ./internal/backend/... ./internal/workflow ./internal/api`：通过；补充非 Argo 历史运行保护后，重新运行 workflow/API race 测试通过。
- `tsc --noEmit`、Vite 生产构建、12 项前端测试：通过。现有 Chart chunk 超过 500 kB 的构建提示仍在。
- 隔离服务页面确认工作流用例显示 Argo 模板入口；连接示例默认关闭，没有提交集群工作负载。
- 新增用例覆盖缺省/未知后端、混入不支持的 implementation、无效原生模板及历史运行恢复保护；API 端到端测试覆盖 Argo 提交、证据回收、幂等性及 Issue 发现/验证关联。

本次未更新集群镜像。公开源码和示例不包含内部执行适配器；Git 历史不在此次代码修改范围。
