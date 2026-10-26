# Kubernetes 部署与测试运行

状态：目录预留与设计，尚无可直接 apply 的 YAML 或已发布 Runner 镜像。

## 目录职责

- `base/`：规划服务 Deployment/Service、配置和身份；单副本服务起步。
- `jobs/`：规划单客户端 Runner Job，挂载现有 PVC 并执行固定计划。
- `overlays/`：规划按环境设置 namespace、PVC、节点选择、资源和镜像 digest。

使用 Kustomize 风格的目录组织起步，待实际 manifest 实现后验证组合，不同时维护重复的 Helm 方案。

## 三类挂载

| 路径 | 用途 | 生命周期 |
| --- | --- | --- |
| `/bench` | 现有被测 PVC，使用独立 run/attempt 子目录 | 不删除 PVC，只处理本次记录的测试文件 |
| `/etc/malta` | 展开后的 plan/config，只读 | 绑定计划摘要和 Run |
| `/results` | 独立结果存储或上传暂存目录 | 导入/上传确认后才允许清理 Pod |

服务数据库独立存储，不能放在被测卷中。采用 SQLite 时使用支持本地文件锁语义的文件系统；存储可用性和服务单副本约束须在部署时验证。

## 执行与恢复

1. 提交端校验 cluster/context、namespace、PVC、访问模式、拓扑和资源额度，固定计划及镜像。
2. Job Pod 执行 `malta exec`，记录实际节点、Pod UID、attempt_id、环境与挂载。
3. 测试输出结果包；结果持久化或上传成功与 I/O 成功分别记录。
4. 服务导入并确认 artifact 可读后，再启用 Job TTL 清理。

默认规划 `restartPolicy: Never`、`backoffLimit: 0`、显式 deadline。它们不能保证底层任务只启动一次；资源组协调与 attempt 身份仍需处理重复 Pod、抢占、中断和恢复。重试不冒充独立的重复实验。

执行 Pod 仅消费已挂载存储，默认关闭 ServiceAccount token 自动挂载。提交端使用目标 namespace 内的 Job/Pod 查询与管理权限，日志权限单独声明。节点或集群元数据查询按需要由提交端提供，缺失时标为 unknown。特殊 FUSE 自挂载另做适配，默认路径复用 CSI，不要求 privileged。

单 Pod 先验收；多客户端需另行验证共享访问模式、启动同步、时间窗口和聚合算法，增加 Job parallelism 不等于完成分布式测量。

参考：[Jobs](https://kubernetes.io/docs/concepts/workloads/controllers/job/)、[PV/PVC](https://kubernetes.io/docs/concepts/storage/persistent-volumes/)、[ServiceAccount](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/)。
