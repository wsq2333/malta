# Malta test tools for Argo

独立工具镜像，执行 OSS 上传 → FUSE 全量读取 → fio 3.33 顺序读。运行时为 Python 3.11、OSS SDK 2.19.1 和 fio 3.33，默认 UID/GID 为 10001；不包含 Malta 服务二进制、集群配置或凭据。

## 用例与参数

运行 `python3 tools/malta-test-tools/render_examples.py` 生成以下公开占位示例：

- [Argo WorkflowTemplate](../../docs/examples/workflow/oss-fuse-fio.argo.example.json)
- [Malta 用例](../../docs/examples/workflow/oss-fuse-fio.argo-case.example.json)
- [1 GiB 参数](../../docs/examples/workflow/oss-fuse-fio.argo-parameters.example.json)

默认 256 个文件 × 4 MiB = 1 GiB，上传并发 4；顺序读使用 psync、iodepth=1、numjobs=1、direct=0、runtime=60 秒。file_size_bytes/file_count 控制唯一数据量，fio_bs_bytes 控制 I/O 大小。多个 fio job 读取同一文件集，并不是互不重叠分区；并发不放大准备数据量。最大数据量 256 GiB，仍须服从 Malta 连接的更小预算。

参数作为独立环境变量 `MALTA_PARAM_<大写参数名>` 传入，不拼接 shell。Malta 注入的 `malta_context` 单独传给 `MALTA_CONTEXT`。示例不配置重试或循环，attempt 固定为 0；若需要重试，必须同时扩展身份及证据映射，不能直接给模板添加 retryStrategy。

## 部署前绑定

将示例复制到被 Git 忽略的 `config/workflow/`，再填写实际值：

1. 镜像替换为已推送的 digest，配置 imagePullSecrets。
2. 填写执行 namespace 内的 ServiceAccount、固定节点、结果 RWX PVC 和只读测试 PVC。ServiceAccount 需满足 Argo executor 的任务结果权限。
3. `/bench` 必须暴露所填 OSS bucket 的根目录 FUSE 视图；结果卷挂到 `/results`，与 Malta 的 remote/local_results_root 对齐。SQLite 状态盘不能代替结果卷。
4. OSS endpoint/bucket 使用现场配置；AK/SK 和可选 Session Token 只通过 Secret 的键引用注入。需要 PutObject 和 HeadObject 权限，不覆盖其他实验对象。
5. 确认 10001:10001 可读测试卷、可写结果卷。临时卷容量 1 GiB，模板限制为单工作流 1200 秒；实际连接和用户总时限取更小值。

对象前缀为 `<dataset_prefix>/<Workflow name>.<Workflow UID>/part-N.bin`。条件写入禁止覆盖，仅允许同一运行且大小/摘要元数据一致的对象重用。不会自动删除数据或缓存。

## 结果契约与限制

上传生成 dataset.json；预热消费该清单并生成 warmup.json；读取消费 warmup.json，核对清单摘要，再发布 fio 报告。共享结果路径是 `/results/<Workflow name>.<UID>/<Argo node ID>/`。节点 ID 从 Pod annotation 获取，各阶段发布 `malta.evidence/v1` 清单，包含身份、文件大小和 SHA-256。

工具最后写入 `/tmp/malta-<输出名>-path`，由 Argo `outputs.parameters.valueFrom.path` 读取；下游节点通过 `tasks.<名称>.outputs.parameters` 消费路径，Malta 回收同一共享卷上的证据。参考 [Argo 输出参数](https://argo-workflows.readthedocs.io/en/release-3.5/walk-through/output-parameters/) 和 [运行变量](https://argo-workflows.readthedocs.io/en/release-3.5/variables/)。

预热仅证明完整读过文件集，**不证明缓存命中、单副本或缓存块大小**。上传 SHA-256 存在清单和对象元数据中；HEAD 校验不是下载后重算，当前工具不执行 FUSE 内容 SHA-256 验证。fio 禁止创建文件，不修改只读测试卷。报告不得将这些未验证项写成通过。

## 构建与验证

在 Linux/amd64、Python 3.11 上准备锁定依赖，再构建：

```sh
python3 tools/malta-test-tools/prepare_dependencies.py \
  --output build/output/malta-test-tools \
  --ca-bundle /etc/ssl/certs/ca-certificates.crt
docker build -f build/docker/Dockerfile.test-tools \
  -t malta-test-tools:<source-version> .
docker run --rm malta-test-tools:<source-version> version
```

依赖准备须在与运行镜像兼容的 **Debian bookworm、Linux/amd64、Python 3.11** 构建环境执行，并安装 `build-essential`、`python3-dev` 和 `python3-pip`（推荐使用 venv）。`prepare_dependencies.py` 强制从源码构建 `crcmod==1.7`，禁用其 wheel 缓存，并验证原生扩展确实加载；缺少编译器或头文件导致的纯 Python 回退会使准备失败。编译工具只留在构建环境，不复制进运行镜像。

镜像构建会以 UID/GID 10001 执行 `malta-test-tools version`，预期包含 `"crc64_backend": "native"`。上传入口再次检查原生扩展并显式保留 OSS SDK 的 CRC64 校验；不会通过关闭 CRC 来提速。`dependencies.json` 同时记录原生后端、Python 版本和扩展文件摘要。原生 CRC 的计算速度不代表真实 OSS 上传吞吐，实际速度仍取决于网络、服务响应和其他处理开销。

生产镜像仅包含运行脚本、锁定 SDK 依赖、许可证和 OS 工具；没有测试代码与配置。开发机无 Docker daemon 时，可按相同文件清单生成 OCI layout / Docker archive，并验证每层摘要及最终 rootfs；这不等价于在真实 Kubernetes 集群验收。

`tests/test_tasks.py` 使用真实 OSS SDK、模拟 loopback OSS 和真实 fio，覆盖条件上传重用、三阶段数据衔接、报告摘要、Argo 身份/输出路径、错误脱敏、数据缺失、原生/纯 Python CRC 结果一致性、原生扩展缺失阻断，以及错误 OSS CRC 响应拒绝。测试只能对临时目录运行，结果不是 Curvine 性能基线。
