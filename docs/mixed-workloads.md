# 多负载场景定义草案

状态：设计提案，尚未实现。本文 YAML 不可直接传给当前 `malta plan --spec`。现有 schema 1.0、单负载执行、旧结果导入与基线比较保持原有语义。

## 参考与目标

按 EMC Mongoose 存储压测工具理解本次参考：其 [Scenario 文档](https://emc-mongoose.github.io/mongoose/howto.html#scenario) 提供顺序、并行及配置继承；[Weighted Load](https://github.com/emc-mongoose/mongoose-load-step-weighted) 通过共享权重节流协调不同操作生成器；[Pipeline](https://github.com/emc-mongoose/mongoose-load-step-pipeline) 将同一对象依次传给不同操作，并支持操作间延迟。

Malta 借鉴组合方式，使用声明式 JSON/YAML，保留负载、版本、硬件、证据和可比性契约。这里是 Malta 自己的设计，不承诺直接兼容 Mongoose 配置或执行其脚本。

## 四种明确的组合语义

| 模式 | 含义 | 示例 |
| --- | --- | --- |
| sequential | 一个完整子负载结束后开始下一个 | 大文件写入结束，再执行小文件随机读 |
| parallel | 多种子负载在同一测量窗口内共同施压，各自配置 worker 和深度 | 4 KiB 随机读与 1 MiB 顺序写同时运行 |
| weighted | 共享调度器按声明单位分配操作，记录目标与实际比例 | 70% 随机读、20% 顺序写、10% stat，按操作次数计 |
| pipeline | 同一文件按依赖顺序完成一组操作，多文件之间可并发 | create → write → fsync → stat → read → unlink |

`parallel` 的 worker 数量比例不等于实际 IOPS 或字节比例。`weighted` 首先定义操作次数权重；按字节权重属于独立扩展，不能与无数据字节的 stat 混用。pipeline 是逐文件依赖，sequential 是整个阶段依赖。

## 第一阶段定义示例

采用独立的 `ScenarioSpec`，复用现有 `Workload` 字段；阶段为顺序列表，阶段内部选择组合模式。首阶段不支持递归场景，以限制展开规模。以下数值是功能验证模板，不是推荐性能测试规模。

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

重复整个场景三次，每次使用新的数据目录。单阶段容量为 2 × 8 MiB + 1 × 32 MiB = 48 MiB，保留三次数据需要 144 MiB，不包含日志与证据。时长由阶段统一控制，展开到执行计划时填入每个子负载，不允许子负载覆盖阶段窗口。parallel 固定组合同时执行；第一阶段不对多个子负载矩阵做隐式笛卡尔积。

同一被测挂载下给每个子负载独立数据目录，使其共享存储资源但不互相改写文件。所有准备工作完成后才能开始并行测量，记录每个子负载的实际起止时间和窗口重叠情况；不能只凭配置认定同步成功。整个场景持有一次资源锁，内部子负载不能分别争抢同一把现有运行锁。无关 Run 仍按现有队列串行。

## 权重与文件依赖的后续定义

weighted 阶段拟增加：

```yaml
mix:
  basis: operations
  weights:
    small-read: 70
    large-write: 20
    file-stat: 10
  seed: 42
```

此片段需配合三个对应子负载定义，不是完整可执行配置。每个 data 操作计一次指定块大小的 I/O，stat 计一次系统操作；不得混淆为读取整个文件的次数。准备、预热、隐含文件打开等辅助操作不纳入目标配比，但需说明测量口径。

权重作用于测量窗口中提交的逻辑操作，比例按权重总和归一化。记录各类提交数、成功数、错误数、实际字节数、目标比例和实际比例；失败的操作仍计入提交分母。记录有限窗口下的偏差及停止时未完成操作，不承诺成功完成的操作比例严格等于目标。任何偏差验收阈值必须显式进入计划，不能事后用归一化隐藏偏差。

独立 fio 进程的 `numjobs` 或 IOPS 上限不能替代跨操作共享权重调度。通用 weighted 需要可按操作调度的引擎适配层，或经固定版本验证的引擎原生能力；不能把多个独立限速负载标为精确加权。

pipeline 需要记录文件身份、步骤依赖、文件状态与有界队列；create 失败的文件不能继续执行 read。共享文件的写入、删除与读取竞争必须显式定义，第一阶段只接受 isolated_per_workload。当前 mdtest 仅支持结果导入，数据与元数据实时混合需要先实现元数据执行器及相关证据校验。

## 执行、结果与展示

场景作为一个顶层 Run，避免把并行子负载作为独立队列任务而意外串行化。建议新增独立的场景模型与版本分派，保留既有 Run 1.0 的反序列化与哈希；不要仅在旧结构里增加默认字段而悄悄改变旧计划摘要。

```text
ScenarioRun
  scenario_snapshot + environment + version + plan_hash
  attempt
  repetitions[]
    stages[]
      workloads[]
        actual_start/end + status + metrics + evidence
```

各子负载指标由 stage_id、workload_id、参数点、操作和指标名共同定位。重复摘要以整个场景的一次重复为单位，不能将不同子负载作为重复样本。证据路径按 repeat/stage/workload 分隔，保存展开配置、工具命令、原始输出与校验和。

结果/UI 展示阶段时间线、并行子负载列表、实际窗口、独立吞吐/IOPS/P99，以及 weighted 的目标/实际操作比例。整体吞吐必须基于共同窗口内的真实字节数和持续时间计算，不能直接相加不同窗口的平均速率。不能平均子负载 P99 得到整体 P99；只有具有一致口径的原始直方图时才另行计算。

基线比较固定整个场景的拓扑、负载配置、权重口径、数据共享策略、预热/测量窗口、重复数、工具版本和硬件条件，再逐个对齐子负载。子负载缺失、失败或同步不满足约定时，不产生成功的混合场景基线。比较可保留完整子负载诊断，但不将部分成功当作整体成功。

SQLite 可先延续 JSON 快照存储，不必立即拆出多张关系表；需要显式区分场景记录与旧单负载记录，并扩展查询、导入、比较和 UI 的版本处理。Agent 能力发现应公布可支持的组合模式与上限，未实现的模式直接拒绝。

## 实现顺序与验收

1. **定义与计划**：新增版本化 ScenarioSpec/ScenarioPlan、schema、校验与预算展开，保留旧 plan 哈希和导入；验证唯一 ID、总容量、累计时间和最大并发。
2. **文件 I/O 组合执行**：实现 sequential/parallel，统一准备、同步测量、取消和失败回收。评估 fio 多 job 分组；当前解析器只接受一个 group，必须同时升级分组解析与原始选项核验。可参考 [fio 分组报告文档](https://fio.readthedocs.io/en/latest/fio_doc.html#measurements-and-reporting)，实际实现仍需在项目固定的 fio 3.33 验证。
3. **持久化与 UI**：接入托管运行、本地/Kubernetes 单 Runner Pod、结果包导入导出、分组比较、场景页面和 Agent 能力发现。任何新增类型都必须经过原始证据验证，不能绕开现有导入保证。
4. **按操作加权**：实现共享调度、固定随机种子、目标/实际比例与计数证据；在不同操作延迟和失败条件下验证比重与统计口径。
5. **元数据与流水线**：增加元数据原生执行、文件状态机、共享数据策略和生命周期压测；多 Pod 同步作为后续独立阶段。

每步单独 commit。运行层验收使用指定 Linux 开发机的临时数据目录；验证并行确实重叠、准备不混入测量、取消无残留、失败保留证据、Kubernetes 恢复不重放，以及旧单负载结果仍能导入和比较。设计文档提交不代表上述执行验收已完成。
