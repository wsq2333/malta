# Argo 镜像构建与工具验证

本文记录初次构建阶段的验证范围，不代表后续部署状态。当时开发机完成 Linux/amd64 的两份本地镜像归档，尚未推送仓库或部署集群。服务使用提交 706b21e，工具改动位于 codex/argo-test-tools 分支，未提交版本通过源码清单摘要标记。

| 镜像 | OCI manifest digest |
| --- | --- |
| `malta:dev-706b21e` | `sha256:980fc67f9028b7c8703c7f872bfd6eb2dde2aabfbb2552b9bd6e96892724d9ff` |
| `malta-test-tools:dev-706b21e-dirty-4f401b399fab` | `sha256:39c99d1acd459f4e94f8e65cc61cb2a457c8e1c586e2aabb0e2f0fe1cd5ef43d` |

开发机没有运行 Docker daemon，使用已校验的固定 Debian 基础层和 Docker 配方对应文件系统生成 OCI layout 与 Docker archive。没有叠加包含旧应用的镜像层。校验 archive/config/layer 摘要后，从最终归档还原文件系统，以 UID/GID 10001:10001 执行验证：

- Malta 0.1.0-dev.706b21e、kubectl v1.35.0、健康接口、UI 静态资源及仅 Argo 工作流能力字段正常；二进制 SHA-256 与发布清单一致。
- 工具版本 0.2.0、Python 3.11.2、OSS SDK 2.19.1、fio 3.33 正常；镜像不包含 Malta 二进制、测试代码或测试数据。
- 6 项 Python 测试通过。使用真实 OSS SDK 对隔离的 HTTP 模拟 OSS 进行条件上传/HEAD，并用真实 fio 完成 16 MiB 文件集全读和短时测量；不是实际 OSS 或 Curvine 测试。
- `TestParameterizedToolsCase` 和 `TestToolsGeneratedEvidence` 通过，Malta 读取实际工具生成的三个节点证据并解析 fio 指标。
- 服务发布构建完成两次 UI/二进制一致性检查及 12 项前端测试；kubectl 缓存按仓库固定摘要核验。

Argo 3.5.13 官方 CLI 下载无进展后停止，原生 `argo template lint --offline` 未执行。Malta 格式检查不能代替 Argo 控制器校验。现场仍需绑定镜像 digest、ServiceAccount、Secret、节点和 PVC，确认认证/TLS 与共享目录，再做真实运行验收。

工具预热为 FUSE 完整读取，未验证内容 SHA-256、缓存命中、缓存块大小或副本数；不能将这些项报告为通过。详见[工具说明](../../tools/malta-test-tools/README.md)。机器路径与完整构建清单只保存在被 Git 忽略的 build/output 下。
