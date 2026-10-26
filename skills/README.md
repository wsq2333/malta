# Agent skills

状态：目录预留与契约规划，尚无可安装的 skill；CLI 实现后再添加 `SKILL.md` 和经过验证的命令示例。

Malta 的 skill 随项目版本发布，面向外部 Agent，复用标准 CLI。它不承载另一套 Runner，也不在测试 Pod 中运行模型推理。

## 规划结构

```text
skills/
├── README.md
├── malta-benchmark/
│   ├── SKILL.md
│   └── references/       # 负载选择、退出码、JSON 与失败恢复
└── malta-analysis/
    ├── SKILL.md
    └── references/       # 可比性、统计与证据引用
```

## 执行流程

`需求 → environment 查询 → plan → run start → get/logs/wait → 结果检查 → compare → 报告`。

- 计划必须确定目标环境、测试子目录、文件与 I/O 大小、读写类型、资源预算、测量窗口及重复次数。
- 用 `--format json` 和 `--non-interactive` 调用；按明确字段判断状态，不解析彩色终端表格。
- 提交重试复用 request-id；等待超时先查询现有 Run，不重新发起压测。
- 在已授权的环境与额度内自动执行；不因为一般性的流程提示重复询问。越界操作返回配置错误，不能静默扩大测试目录或资源。
- 取消后核对 attempt 终态和进程清理。测试失败保留日志，不自动把失败当成低性能。
- 比较前读取可比性原因；基线变更按项目策略显式记录，新结果不自动覆盖历史引用。
- 报告必须指向 run_id、attempt/trial、具体指标和 artifact；分析结论不能改写原始数据。

## 发布验收

skill 声明支持的 Malta CLI 与 schema 版本。示例须覆盖一次小规模成功运行、重复提交、超时查询、取消、结果缺失及不可比场景。CLI 不支持的能力不得写成可执行步骤。

关联：[README](../README.md)、[架构](../docs/architecture.md)。
