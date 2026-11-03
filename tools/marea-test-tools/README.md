# marea-test-tools

首版工具镜像：OSS 上传 → Curvine FUSE 完整读取预热 → fio 3.33 顺序读。UCE 负责 DAG 调度；镜像不启动 Malta Worker，也不包含集群凭据。当前预热方式为 `fuse-full-read`，仅证明完整读过数据集，**不证明 Worker 缓存覆盖或命中率**。没有捆绑 Curvine CLI、清缓存或删除 OSS 数据的动作。

## 模板和参数

- `docs/examples/workflow/oss-fuse-fio.ep.example.json`：原生 UCE EP。
- `docs/examples/workflow/oss-fuse-fio.case.example.json`：Malta 用例，包含参数 Schema、默认值、准备数据预算及三阶段结果规则。
- `oss-fuse-fio.parameters.example.json`：4 MiB × 256 = 1 GiB。
- `oss-fuse-fio.parameters-256gib.example.json`：4 MiB × 65,536 = 256 GiB；并发不随数据量放大。

运行 `python3 tools/marea-test-tools/render_examples.py` 重建这四个文件。修改算子实现、模板环境绑定或发布版本时，应按 UCE 不可变定义规则更新相应版本 ID；这些示例 ID 不能反复用于登记不同内容。

所有测试参数声明为 DAG 输入，再通过 `input_bindings` 传给三个算子。`MAREA_TEST_PARAMS={{ toJson .task_params }}` 将其作为 JSON 传给程序，不拼接 shell 命令。Malta 自动注入的 `malta_context` 用于证据身份校验；直接通过 UCE 提交时，也必须显式提供合法的上下文，但不会自动生成 Malta 的运行记录。

| 参数 | 默认值 | 含义 |
| --- | --- | --- |
| file_size_bytes / file_count | 4194304 / 256 | 文件大小、唯一文件数量 |
| dataset_prefix / data_seed | malta-read-cache / 1 | 数据前缀及可复现的数据生成种子 |
| upload_concurrency | 4 | 并行生成及上传数；临时空间需求约为并发 × 文件大小，工具上限 1 GiB |
| warmup_timeout_seconds | 600 | 完整读取预热的执行上限 |
| fio_bs_bytes | 1048576 | I/O 块大小，不是文件大小 |
| fio_numjobs / fio_iodepth | 1 / 1 | 并发和队列深度；psync 强制深度 1 |
| fio_ioengine / fio_direct | psync / 1 | 支持 psync、libaio；direct 不支持时失败，不静默降级 |
| fio_runtime_seconds | 60 | 计时重复读时长；不是完整遍历次数 |

rw 固定 read，禁止 fio 创建文件，所有 job 读取同一份完整数据集。numjobs 增大不会增加准备数据大小，也不是互不重叠的分区读取。fio 使用 group_reporting，生成一个汇总报告；Malta 将参数与实际 job options 对照。预热与计时读分别使用独立 fio 进程，预热固定一个 job 完整遍历。

## 提交前替换的环境信息

先按 [本地配置说明](../../config/README.md) 将示例复制到被忽略的 `config/workflow/`，再填写真实环境信息；认证文件放 `config/secrets/`。不要直接修改仓库中的示例来保存现场配置。

模板故意保留占位值，不能直接提交现场运行：

1. 三个算子的镜像替换为已推送的固定 digest；确认执行侧镜像拉取配置。
2. `resource_pool`、固定 nodeSelector，以及 Run 的 queue/subtype 对齐实际执行集群。EP 不指定管理集群；namespace、ServiceAccount 和 Pod 安全策略由已配置执行侧提供。镜像默认 UID/GID 10001。
3. upload 的 `OSS_ENDPOINT`、`OSS_BUCKET` 填实际值。`OSS_ACCESS_KEY_ID`、`OSS_ACCESS_KEY_SECRET`、可选 `OSS_SESSION_TOKEN` 通过 `secretKeyRef` 从执行 namespace 的 Secret 注入，不放 task_params 或版本化 JSON。需允许 PutObject、HeadObject，且对象不能是禁止条件写入的特殊桶类型。
4. 三个阶段挂载同一份共享结果 PVC 到 `/results`；UCE 的 `${output_root}` 必须落在该挂载内。Malta 的 remote_results_root/local_results_root 指向同一底层目录。数据文件放 OSS/Curvine，NAS 只放清单和报告。
5. warmup/read 的 `/bench` 必须是**该 OSS bucket 根目录在目标 Curvine 集群中的 FUSE 视图**，能看到直接上传的对象。若只挂载 bucket 子目录，需要先调整映射，不能直接套用默认路径。
6. 替换 Curvine PVC、结果 PVC、固定节点。测试卷以只读方式挂载；上传仅访问 OSS。确认 UID/GID 10001 可读 Curvine、可写结果卷；本例不提供 root 绕过。
7. 用例整体超时 12000 秒；连接超时/数据预算需相应放行。预热超时是进程上限，其他阶段仍受 UCE 工作流超时约束。

数据路径为 `<dataset_prefix>/<UCE run_id>/part-N.bin`，按 Run 隔离。文件数据由种子与序号生成，逐文件 SHA-256 存入清单；SDK 使用禁止覆盖请求，409 时只接受同 Run 且大小及摘要元数据相同的对象。无自动删除；失败后可能保留部分对象，由管理员按 Run 前缀处理。

上传阶段的 HEAD 校验验证大小和上传时写入的摘要元数据，不等价于下载全量内容重新计算校验和；FUSE 阶段检查大小并完成读取，也不证明内容校验或缓存命中。

## 输出契约

- upload：`dataset` → dataset.json；`evidence` → assertions-v1 manifest。
- warmup：消费 dataset；`warmup` → warmup.json，包含数据清单路径/摘要、实际全读字节数、`cache_hit_verified=false`；另发布 evidence。
- fio-read：消费 warmup；核验数据清单未变，发布 fio.json、read.fio 和 evidence。

UCE `input_path` 建立节点依赖。`${output_root}/${run_id}/${stage_name}/${attempt}/${eviction_count}/...json` 由 UCE 解析并通过 MAREA_OUTPUT_PATHS 传入；程序先写报告，再原子发布 JSON。所有证据包含外部运行/节点/attempt/驱逐次数及 Malta 快照身份。默认不重试；失败退出非零，不伪造成功 evidence。

## 构建与验证

Docker 配方为 `build/docker/Dockerfile.marea-test-tools`。基础镜像必须是经过核验的 Malta Runner（fio 3.33、Python 3.11），以 digest 指定。开发机无 Docker daemon 时使用本目录的 OCI 构建脚本，在该基础镜像上添加脚本、固定版本 SDK 和 CA bundle。构建产物包含依赖版本/安装文件摘要、基础镜像 digest、源码摘要及 smoke 记录，不包含私有配置。

依赖以 `requirements.lock` 固定。SDK 采用 OSS Python SDK 2.19.1；测试使用隔离 HTTP 模拟 OSS，真实 SDK 上传/HEAD 和真实 fio I/O。结果不作为 Curvine 性能基线，也不证明现场 CSI、OSS 权限及缓存行为。

在 Linux/amd64、Python 3.11 构建机上，先准备依赖，再构建（输出目录必须不存在）：

```sh
python3 tools/marea-test-tools/prepare_dependencies.py \
  --output build/output/marea-test-tools \
  --ca-bundle /etc/ssl/certs/ca-certificates.crt
docker build -f build/docker/Dockerfile.marea-test-tools \
  --build-arg BASE_IMAGE=<verified-runner-image@sha256:digest> \
  -t marea-test-tools:<source-version> .
```

无 Docker daemon 的开发机也可使用 `build_oci.py --help`。它接收已核验的 Runner OCI 目录、准备好的 Linux rootfs、源码目录和输出目录，生成 OCI layout 与可 `docker load -i` 导入的归档。依赖锁定版本；`dependencies.json` 记录已安装文件 SHA-256，保留 SDK 依赖自带的许可证。

## 在 Malta 中使用

1. 将 `.case.example.json` 中的 `body.template` 按上述清单替换环境占位值，再保存为工作流用例的新版本。独立 `.ep.example.json` 是同一份模板，便于 UCE 校验或审阅。
2. 选择被测系统、UCE 执行配置、软件版本，在参数步骤粘贴参数示例；需要 256 GiB 时使用对应参数文件，不需修改 DAG 结构。
3. 预检查确认数据预算、共享结果卷和结果解析规则，随后提交。Malta 负责 EP 注册与发起 Run；不需要在测试 DAG 中额外增加注册算子。
4. Malta 回收三个节点的 evidence。只有断言、运行身份、文件大小/摘要与 fio 参数验证通过，才形成通过结论和 fio 指标。后端节点成功但证据缺失，不应视为测试通过。

这是一份待现场绑定的模板。模拟 OSS 的验收不能代替真实 UCE、Curvine、Secret、PVC 和执行身份的联调。
