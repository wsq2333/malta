# marea-test-tools 首版构建验收

- 平台：Linux/amd64，工具版本 0.1.0。
- 镜像标签：`marea-test-tools:dev-33efe80-dirty-6c1d868a`。
- OCI manifest：`sha256:c8109637f570939548c5956f36f132dffc52e0a7ef5e937697fc34447b9d6029`。
- Docker 归档 SHA-256：`859841858436ba57d60054a503c5aa9616b3e078a795a22aca4d533049b7ee27`。
- 基础镜像 digest：`sha256:54236516deb9a71a4d689b6e0dd807a6d6caf0337060d3ce745f0ae5219a8911`。
- tasks.py SHA-256：`ce16b2bc54a517d035b728a869c9aadd9c2f9bd187c7c54747653e947cea6b8c`。
- Python 3.11、OSS Python SDK 2.19.1、fio 3.33；运行身份 10001:10001。

开发机没有 Docker daemon，本次在经过核验的 Runner OCI 镜像上增加应用层，生成 OCI layout 和 Docker load 归档。校验各层及配置 digest，从最终归档重新解包，并通过 chroot 运行验收；不是 Kubernetes 或容器运行时部署验收。构建目录保留 image-metadata.json、source-files-sha256.json、dependencies.json 和 validation.json。

## 已通过

1. 使用现有 UCE API Server 的 model 源码，严格解码 EP、归一化、TranslateExecutionPlan 和 ValidateArtifactContract。确认 upload → warmup → fio-read 的依赖与输出路径。
2. Malta 工作流模块测试；新用例定义及 1 GiB、256 GiB 参数示例校验。
3. 从最终镜像归档以非 root 身份执行 5 项 Python 测试：数据预算/参数约束，路径与软链接约束，缺失数据失败，SDK 错误不泄露凭据，以及完整三阶段链路。
4. 完整链路使用真实 OSS SDK 访问隔离的 HTTP OSS 模拟服务，上传 4 × 4 MiB 文件；重复上传检查禁止覆盖与同 Run 对象复用。真实 fio 先全读 16 MiB，再以 numjobs=2、direct=1、psync、1 MiB 块、1 秒计时读取同一数据集；验证实际 job options、清单摘要和证据。
5. `TestToolsGeneratedEvidence` 使用以上 SDK/fio 生成的实际文件，通过 Malta 身份、摘要、断言与 fio 参数校验，三个结果为 passed，读取结果包含性能指标。

## 验收边界

- 未推送镜像、未注册现场 EP、未提交 UCE Run、未更改集群部署或 Secret。
- 模拟 OSS 与本地文件不能证明真实 OSS 权限、Curvine 映射、CSI 挂载、执行侧资源池或共享结果 PVC 可用。
- 预热为 FUSE 完整读取，明确记录 `cache_hit_verified=false`；没有验证 Worker 缓存覆盖率、命中率或内容逐字节正确性。
- 1 GiB / 256 GiB 仅通过参数验证；本次三阶段 smoke 数据量为 16 MiB，不是正式性能基线。
- 模板仍保留镜像、资源池、节点、PVC、OSS endpoint/bucket/Secret 名称占位值。使用方式见 [工具说明](../../tools/marea-test-tools/README.md)。
