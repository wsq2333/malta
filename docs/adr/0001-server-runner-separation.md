# ADR 0001：服务 Pod 与测试 Pod 分离

状态：接受。适用于 Kubernetes 执行模式；local executor 保留用于本地开发及专用测试节点。

## 决策

`malta-server` 常驻运行，承载 UI、API、运行管理与结果查询，由 Deployment 管理；首版单副本。测试由独立 Job 创建 `malta-runner` Pod，执行 `malta exec` 与 fio/元数据工具，完成后在结果持久化确认之后回收。

服务 Pod 不同时充当压测 Pod，不将压测工具作为其 sidecar。Agent 在服务外通过 CLI/API 管理任务。服务重启后根据持久化状态恢复管理，不能据此自动重跑写入测试。

## 角色与存储

| 角色 | 存储 | 权限与资源 |
| --- | --- | --- |
| Server | 独立数据库、结果索引与 artifact | 目标 namespace 的必要 Job/Pod 管理权限；稳定资源额度 |
| Runner | 被测 PVC、只读计划、独立结果路径 | 默认仅操作挂载目录；按测试定义配置节点与资源 |

数据库不位于被测文件系统。结果可以通过独立结果卷或后续上传机制交付，具体回传方案按访问模式与拓扑验证。测试成功与结果保存成功分别记录，不能在结果未确认时清理唯一副本。

## 原因与影响

- 隔离 UI/API 查询、数据库写入与测试的资源争用。
- 取消、超时或测试崩溃不停止平台服务，服务升级不要求中断独立的测试 Pod。
- Runner 可以调度到指定机型，服务留在管理节点；后续可管理多个环境及多客户端任务。
- 分 Pod 不等于硬件隔离。正式测量应尽量使用不同节点，并记录网络、存储及背景负载等共享资源。
- Job 不能作为恰好一次执行保证；Run/Attempt 身份、资源组协调、独立目录与幂等导入仍是必须实现的机制。

S0–S4 实现通用领域模型、local executor、API 与 UI。Kubernetes executor 和可部署 manifest 在 S5 实现，界面不能将尚未实现的后端显示为可用。

参考：[Deployment](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/)、[Job](https://kubernetes.io/docs/concepts/workloads/controllers/job/)。
