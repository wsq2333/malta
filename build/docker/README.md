# 容器镜像规划

状态：仅预留目录，尚无 Dockerfile 或可拉取的 Malta 镜像。

- `Dockerfile.server`：规划构建 Go 服务与前端静态资源，运行 `malta serve`。
- `Dockerfile.runner`：规划包含 Malta CLI 和固定版本 fio；后续按需要增加元数据工具、MPI 等依赖。

镜像运行时不包含模型推理服务。记录 Malta、工具与基础镜像版本，以 digest 固定实验环境；CPU 架构不同的镜像分别验证。Runner 用前台进程正确接收终止信号，结果通过挂载目录保存。

构建验收覆盖：启动、工具版本、最小测试、终止与结果导出；实际支持架构和发布仓库待实现时确定。

关联：[Kubernetes 运行规划](../../deploy/kubernetes/README.md)。
