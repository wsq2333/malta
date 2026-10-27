# 多负载场景

已实现版本 `scenario/v1alpha1`：顺序/并行 fio、按操作次数加权的原生 I/O 与 stat，以及逐文件生命周期流水线。通过 `plan --scenario` 生成计划，继续使用原有 exec、run、result、baseline、compare 命令。旧单负载 schema 1.0 的计划哈希与导入保持兼容。

设计参考 EMC Mongoose 的 [Scenario](https://emc-mongoose.github.io/mongoose/howto.html#scenario)、[Weighted Load](https://github.com/emc-mongoose/mongoose-load-step-weighted) 和 [Pipeline](https://github.com/emc-mongoose/mongoose-load-step-pipeline) 组合方式。Malta 使用自己的声明式契约，不直接执行 Mongoose 配置或脚本。

## 支持范围

| 模式 | 引擎与行为 | 示例 |
| --- | --- | --- |
| sequential | fio 子负载依次测量，统一在阶段测量前完成准备 | 修改 mixed-data 的 mode |
| parallel | 同阶段子负载并行；阶段内全部使用 fio 或全部 native | mixed-data.yaml |
| weighted | native 共享调度器按提交操作次数控制权重 | weighted-data.yaml、weighted-metadata.yaml |
| pipeline | native；每个 worker 反复执行同一个文件的完整生命周期，各 worker 可并发 | pipeline.yaml |

阶段列表始终依次执行。原生阶段需要顺序执行不同负载时，将其分成多个阶段。当前不支持同一阶段混用 fio 和 native，不支持多 Pod 同步。整个场景作为一个 Run，占用一次资源锁；独立 Run 继续串行排队。

原生引擎 `malta-native-1` 的首版边界：buffered psync、iodepth=1、cache_state=warm、不使用 fsync_every、块大小不超过 1 MiB、每个子负载最多 64 个文件。单阶段最多 256 workers、16 个子负载；最多 16 阶段、20 次重复、128 个展开后的子负载实验。不支持的选项会明确拒绝。

原生数据操作支持 read/write/randread/randwrite/randrw；独立元数据操作支持 stat。创建和删除通过 pipeline 表达，避免未统计的隐含清理改变压测负载。pipeline 当前每个 worker 使用一个单块文件，文件大小等于 I/O 块大小；支持 create、write、fsync、stat、read、unlink，每种步骤至多一次。必须以 create 开始、unlink 结束并包含 write，read/fsync 必须在 write 后。前序失败即停止后续步骤。暂不提供操作间延迟、共享文件竞争或任意依赖图。

## 定义与启动

```yaml
schema_version: scenario/v1alpha1
name: small-random-read-with-large-write
repeats: 3
failure_policy: stop_all
stages:
  - id: mixed-data
    mode: parallel
    warmup_seconds: 2
    runtime_seconds: 10
    dataset_policy: isolated_per_workload
    workloads:
      - id: small-read
        kind: data
        numjobs: 2
        iodepth: 1
        workload:
          operation: randread
          file_size_bytes: 8388608
          files_per_job: 1
          block_size_bytes: 4096
          read_percent: 100
          ioengine: psync
          direct: false
          cache_state: warm
          fsync_every: 0
          seed: 42
      - id: large-write
        kind: data
        numjobs: 1
        iodepth: 1
        workload:
          operation: write
          file_size_bytes: 33554432
          files_per_job: 1
          block_size_bytes: 1048576
          read_percent: 0
          ioengine: psync
          direct: false
          cache_state: warm
          fsync_every: 0
          seed: 43
```

时长在阶段定义，子负载内不允许重复指定。repeats 重复整个场景，每次创建新的专用目录。parallel 固定组合共同施压，不对多个子负载做隐式矩阵笛卡尔积。子负载共用被测挂载、各用独立文件目录；不改变已有业务文件。

```sh
mkdir -p /tmp/malta-data
bin/malta plan --scenario benchmarks/scenarios/mixed-data.yaml \
  --environment environments/local.yaml --version environments/version-example.yaml \
  --target /tmp/malta-data --resource-group shared-device --out /tmp/mixed-plan.json
bin/malta exec --plan /tmp/mixed-plan.json --out /tmp/mixed-result \
  --fio /path/to/fio --timeout 2m --max-bytes 1073741824

# 或交给已启动、允许访问目标目录的服务：
bin/malta --server http://127.0.0.1:8080 run start \
  --plan /tmp/mixed-plan.json --request-id mixed-001 --source agent
bin/malta --server http://127.0.0.1:8080 run wait RUN_ID --timeout 2m
```

全原生场景不调用 fio；含 fio 阶段时需要 fio 3.33。Kubernetes 使用相同计划与 `--executor kubernetes`，目标为 `/bench`，环境快照须匹配执行器；Server 与 Runner 都必须升级到支持 scenario/v1alpha1 的二进制/镜像。

以上示例每次准备 2 × 8 MiB + 1 × 32 MiB = 48 MiB，三次保留数据共 144 MiB，不含日志。预算按所有重复的准备量计算；pipeline 采用保守容量上界，即使正常完成时文件已 unlink。测量预算按顺序累加、并行取阶段时长；总 deadline 还包括准备和证据收集。

## 权重与测量口径

weighted 使用如下字段，并给每个子负载显式设置 `engine: native`：

```yaml
mix:
  basis: operations
  weights:
    small-read: 70
    large-write: 20
    file-stat: 10
  seed: 42
```

权重为 1..10000 的正整数，按总和归一化；每个子负载必须恰好有一个权重。共享 smooth weighted round-robin 调度器按操作提交分配机会，seed 固定相同权重时的顺序；被选负载的 worker 繁忙时施加背压，不跳过它来悄悄改变配比。worker 数量比例不代表操作数比例。权重也不代表字节比例：一次数据操作是一次指定块大小的 ReadAt/WriteAt，一次 stat 是一次路径查询。

每个子负载保留 submitted、succeeded、errors、late；UI 按重复显示目标/实际提交比例。预热和准备不计入这些计数。以固定测量窗口内提交的操作为分母；在窗口之后完成的操作标为 late，不计入吞吐或延迟直方图。有限窗口和在途操作会产生配比偏差，当前展示实测偏差，不自动设置配比达标阈值。

原生 I/O 在独立子进程执行，便于超时和取消时终止整个进程组。prepare 不计入测量。pipeline 一旦开始处理一个文件就按依赖完成其生命周期，窗口结束后不启动新的文件周期；窗口之后开始的收尾步骤不计入测量，仍受整个 Run 的 deadline 约束。

fio 子负载保存 100ms、Unix 时间戳的 IOPS 采样，以非零样本的首尾作为保守观测区间。原生子负载保存固定计数窗口及实际完成 I/O 的首尾。并行、weighted 和 pipeline 阶段的共同观测区间必须达到配置测量时间的 50%，否则整个场景失败，不能固定为成功基线。此规则随当前场景协议固定。

## 结果、证据与比较

数据库仍使用 objects/runs。场景 Run、Plan 和 Bundle 明确标记 scenario/v1alpha1，API envelope 仍为 1.0。完整定义位于 `plan.spec.scenario`，展开后的 Trial 通过 `point.stage_id`、`point.workload_id`、`point.repeat` 定位；没有新建独立子 Run，也没有把子负载作为重复样本。

结果包保留 `plan.json`、fio 命令/输出/IOPS 日志，或原生 `native.json` 计数与直方图，以及失败日志。导入重新检查子负载参数、窗口、工具身份和从原始证据推导的指标。原生直方图使用纳秒的二次幂区间，展示 `latency_p99_upper`（桶上界），不冒充精确 P99 或 fio clat P99。其计时区间是对应文件操作调用，排除调度等待与文件选择；调度开销仍影响总体吞吐。

UI “混合场景”页签可按版本、环境查看场景，展开后展示阶段、子负载、各重复时间线、权重计数及独立指标。对照与趋势的指标键包含阶段/子负载，避免同并发配置合并。原生与 fio 使用不同工具身份。跨版本比较必须固定完整场景拓扑、引擎、负载参数、权重、共享策略、时间窗口、重复和硬件；名称变化不影响可比性。

不计算混合场景的整体 P99，也不直接相加不同窗口的平均吞吐。创建/关闭文件的辅助开销、缓存环境和采样开销均属于解释结果时的条件；这些功能验收不代表分布式文件系统的性能基线。

## 验收

```sh
go test -race ./...
go vet ./...
pnpm --dir web test
pnpm --dir web build
python3 scripts/scenario-smoke.py --malta /absolute/path/to/malta \
  --fio /absolute/path/to/fio --out /tmp/malta-scenarios-new
```

smoke 脚本在新建目录启动临时 loopback 服务，用小规模真实 I/O 验证五类场景、幂等提交、导入导出、基线比较、70:30/70:20:10 配比和取消后的子进程清理。结果保留在输出目录，服务自动退出。生产 CSI、固定硬件性能与多客户端同步需要独立验收。

本次执行结果见 [场景验收记录](validation-scenarios.md)。
