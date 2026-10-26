import { useEffect, useMemo, useState, type FormEvent } from "react";
import {
  Activity,
  ArrowDownToLine,
  ArrowRight,
  Check,
  ChevronRight,
  Database,
  GitBranch,
  Layers3,
  Leaf,
  Play,
  Plus,
  RefreshCw,
  Search,
  Server,
  Settings2,
  ShieldCheck,
  Terminal,
  TrendingUp,
  X,
} from "lucide-react";
import { Chart } from "./LazyChart";
import {
  Badge,
  Detail,
  Dialog,
  Empty,
  PanelTitle,
  RunMetrics,
  Select,
  Stat,
} from "./components";
import {
  api,
  date,
  downloadBundle,
  format,
  metricLabel,
  operationLabel,
  peak,
  size,
  summarize,
  terminal,
  type Baseline,
  type Comparison,
  type Environment,
  type Plan,
  type Run,
  type Trend,
  type Version,
} from "./model";

type Page = "baselines" | "runs" | "versions" | "environments" | "trends";
const pages: { id: Page; label: string; en: string; icon: typeof Activity }[] =
  [
    {
      id: "baselines",
      label: "性能基线",
      en: "Performance baselines",
      icon: Layers3,
    },
    { id: "runs", label: "测试运行", en: "Benchmark runs", icon: Activity },
    {
      id: "versions",
      label: "软件版本",
      en: "Software versions",
      icon: GitBranch,
    },
    {
      id: "environments",
      label: "硬件环境",
      en: "Hardware environments",
      icon: Server,
    },
    {
      id: "trends",
      label: "对比与趋势",
      en: "Compare & trends",
      icon: TrendingUp,
    },
  ];
export function App() {
  const initial = new URLSearchParams(location.search);
  const [page, setPage] = useState<Page>(
    pages.find((p) => p.id === initial.get("page"))?.id || "baselines",
  );
  const [kind, setKind] = useState(initial.get("kind") || "data"),
    [env, setEnv] = useState(initial.get("environment") || ""),
    [version, setVersion] = useState(initial.get("version") || ""),
    [operation, setOperation] = useState(initial.get("operation") || ""),
    [fileSize, setFileSize] = useState(initial.get("file_size") || ""),
    [blockSize, setBlockSize] = useState(initial.get("block_size") || ""),
    [from, setFrom] = useState(initial.get("from") || ""),
    [to, setTo] = useState(initial.get("to") || "");
  const [runs, setRuns] = useState<Run[]>([]),
    [environments, setEnvironments] = useState<Environment[]>([]),
    [versions, setVersions] = useState<Version[]>([]),
    [baselines, setBaselines] = useState<Baseline[]>([]);
  const [loading, setLoading] = useState(true),
    [error, setError] = useState(""),
    [tick, setTick] = useState(0),
    [selected, setSelected] = useState<Run | null>(null),
    [modal, setModal] = useState(""),
    [toast, setToast] = useState(""),
    [busy, setBusy] = useState(false);
  const [baseline, setBaseline] = useState(initial.get("baseline") || ""),
    [candidate, setCandidate] = useState(initial.get("candidate") || "");
  const [comparison, setComparison] = useState<Comparison | null>(null),
    [trend, setTrend] = useState<Trend | null>(null),
    [compareError, setCompareError] = useState(""),
    [compareLoading, setCompareLoading] = useState(false),
    [chartKey, setChartKey] = useState("");
  const [search, setSearch] = useState(""),
    [token, setToken] = useState(sessionStorage.getItem("malta-token") || ""),
    [jsonInput, setJSONInput] = useState(""),
    [requestId, setRequestId] = useState(""),
    [executor, setExecutor] = useState("local"),
    [executors, setExecutors] = useState<string[]>([]),
    [baselineName, setBaselineName] = useState(""),
    [modalError, setModalError] = useState(""),
    [logs, setLogs] = useState<unknown>(null);
  const refresh = () => setTick((t) => t + 1);
  useEffect(() => {
    const q = new URLSearchParams();
    Object.entries({
      page,
      kind,
      environment: env,
      version,
      operation,
      file_size: fileSize,
      block_size: blockSize,
      from,
      to,
      baseline,
      candidate,
    }).forEach(([k, v]) => {
      if (v) q.set(k, v);
    });
    history.replaceState(null, "", `?${q}`);
  }, [
    page,
    kind,
    env,
    version,
    operation,
    fileSize,
    blockSize,
    from,
    to,
    baseline,
    candidate,
  ]);
  useEffect(() => {
    let valid = true;
    setLoading(true);
    setError("");
    const q = new URLSearchParams({ limit: "500" });
    if (env) q.set("environment", env);
    if (version) q.set("version", version);
    if (from && !Number.isNaN(Date.parse(from)))
      q.set("from", new Date(from).toISOString());
    if (to && !Number.isNaN(Date.parse(to)))
      q.set("to", new Date(to + "T23:59:59.999").toISOString());
    Promise.all([
      api<Run[]>("/runs?" + q),
      api<Environment[]>("/environments"),
      api<Version[]>("/versions"),
      api<Baseline[]>("/baselines"),
      api<{ executors: string[] }>("/capabilities"),
    ])
      .then(([r, e, v, b, caps]) => {
        if (valid) {
          setRuns(r);
          setEnvironments(e);
          setVersions(v);
          setBaselines(b);
          setExecutors(caps.executors);
          setExecutor((old) =>
            caps.executors.includes(old) ? old : caps.executors[0] || "",
          );
          setSelected((old) =>
            old ? r.find((x) => x.id === old.id) || null : null,
          );
        }
      })
      .catch((e) => {
        if (valid) {
          setError(e.message);
          setRuns([]);
        }
      })
      .finally(() => {
        if (valid) setLoading(false);
      });
    return () => {
      valid = false;
    };
  }, [tick, env, version, from, to]);
  useEffect(() => {
    if (
      !runs.some(
        (r) => !terminal(r.status) || r.attempt.result_state === "collecting",
      )
    )
      return;
    const timer = setInterval(refresh, 2000);
    return () => clearInterval(timer);
  }, [runs]);
  useEffect(() => {
    if (!toast) return;
    const timer = setTimeout(() => setToast(""), 4000);
    return () => clearTimeout(timer);
  }, [toast]);
  useEffect(() => {
    let valid = true;
    setComparison(null);
    setTrend(null);
    setCompareError("");
    if (!baseline) {
      setCompareLoading(false);
      return;
    }
    setCompareLoading(true);
    Promise.all([
      api<Trend>("/trends?run_id=" + encodeURIComponent(baseline)),
      candidate
        ? api<Comparison>("/comparisons", { baseline, candidate })
        : Promise.resolve(null),
    ])
      .then(([t, c]) => {
        if (valid) {
          setTrend(t);
          setComparison(c);
        }
      })
      .catch((e) => {
        if (valid) setCompareError(e.message);
      })
      .finally(() => {
        if (valid) setCompareLoading(false);
      });
    return () => {
      valid = false;
    };
  }, [baseline, candidate, tick]);
  const scoped = runs.filter((r) => r.plan.spec.kind === kind),
    visible = scoped.filter(
      (r) =>
        (!operation || r.plan.spec.workload.operation === operation) &&
        (!fileSize ||
          String(r.plan.spec.workload.file_size_bytes) === fileSize) &&
        (!blockSize ||
          String(r.plan.spec.workload.block_size_bytes) === blockSize) &&
        (!search ||
          JSON.stringify([
            r.id,
            r.plan.version.label,
            r.plan.spec.name,
            r.plan.environment.name,
          ])
            .toLowerCase()
            .includes(search.toLowerCase())),
    );
  const successful = visible.filter(
      (r) => r.status === "succeeded" && r.attempt.result_state === "complete",
    ),
    matchingBaselines = baselines.filter((b) =>
      successful.some((r) => r.id === b.run_id),
    ),
    activeCount = runs.filter((r) => !terminal(r.status)).length;
  const currentPage = pages.find((p) => p.id === page)!;
  const notifyAction = async (fn: () => Promise<unknown>, message: string) => {
    setBusy(true);
    try {
      await fn();
      setToast(message);
      refresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  function openModal(name: string) {
    setModalError("");
    setJSONInput("");
    setRequestId("");
    setBaselineName("");
    setModal(name);
  }
  function chooseEnvironment(value: string) {
    setEnv(value);
    setBaseline("");
    setCandidate("");
    setSelected(null);
    setRuns([]);
  }
  async function submitModal(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setModalError("");
    try {
      if (modal === "token") {
        sessionStorage.setItem("malta-token", token);
        refresh();
      } else if (modal === "start") {
        const plan = JSON.parse(jsonInput) as Plan;
        const run = await api<Run>("/runs", {
          plan,
          request_id: requestId,
          source: "human",
          executor,
        });
        setSelected(run);
        setPage("runs");
      } else if (modal === "baseline") {
        if (!selected) throw new Error("请先选择运行");
        await api("/baselines", { name: baselineName, run_id: selected.id });
        setBaseline(selected.id);
      } else {
        await api(
          modal === "environment" ? "/environments" : "/versions",
          JSON.parse(jsonInput),
        );
      }
      setModal("");
      setToast("已保存");
      refresh();
    } catch (e) {
      setModalError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  const trendKeys = trend?.points[0]?.metrics || [];
  const activeChartKey = trendKeys.some((k) => k.key === chartKey)
    ? chartKey
    : trendKeys[0]?.key || "";
  const trendOption = useMemo(() => {
    const points = [...(trend?.points || [])].sort((a, b) =>
      a.run.created_at.localeCompare(b.run.created_at),
    );
    const metric = points[0]?.metrics.find((m) => m.key === activeChartKey);
    const scale =
      metric?.unit === "bytes/s" ? 1048576 : metric?.unit === "ns" ? 1000 : 1;
    const unit =
      metric?.unit === "bytes/s"
        ? "MiB/s"
        : metric?.unit === "ns"
          ? "μs"
          : metric?.unit || "";
    return {
      color: ["#169d77"],
      tooltip: {
        trigger: "axis",
        valueFormatter: (v: number) => `${Number(v).toFixed(2)} ${unit}`,
      },
      grid: { left: 65, right: 28, bottom: 55, top: 25 },
      xAxis: {
        type: "category",
        data: points.map(
          (p) => `${date(p.run.created_at)}\n${p.run.plan.version.label}`,
        ),
        axisLine: { lineStyle: { color: "#dce7e0" } },
        axisLabel: { color: "#718078" },
      },
      yAxis: {
        type: "value",
        name: unit,
        splitLine: { lineStyle: { color: "#edf3ef" } },
      },
      series: [
        {
          name: "独立重复中位数",
          type: "line",
          symbol: "circle",
          symbolSize: 8,
          connectNulls: false,
          data: points.map((p) => {
            const m = p.metrics.find((m) => m.key === activeChartKey);
            return m ? m.median / scale : null;
          }),
          lineStyle: { width: 3 },
          areaStyle: { color: "rgba(37,178,130,.08)" },
        },
      ],
    };
  }, [trend, activeChartKey]);
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <a className="brand" href="?page=baselines">
          <span className="brand-mark">
            <Layers3 size={25} />
          </span>
          <span>
            malta<span className="brand-dot">.</span>
          </span>
        </a>
        <div className="workspace-label">BENCHMARK WORKSPACE</div>
        <nav>
          {pages.map((p) => (
            <button
              key={p.id}
              aria-label={p.label}
              title={p.label}
              onClick={() => setPage(p.id)}
              className={page === p.id ? "active" : ""}
            >
              <p.icon size={19} />
              <span>{p.label}</span>
              {p.id === "runs" && activeCount > 0 && <b>{activeCount}</b>}
              {page === p.id && <ChevronRight size={16} />}
            </button>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <div className="agent-note">
            <span>
              <Terminal size={17} /> AGENT READY
            </span>
            <p>
              可复现的测试
              <br />
              可追溯的证据
            </p>
            <small>CLI · API · Skill</small>
          </div>
          <button className="connection" onClick={() => openModal("token")}>
            <span className="connection-dot" />
            <span>
              可用执行器
              <small>{executors.join(" / ") || "读取中"} · 串行队列</small>
            </span>
            <Settings2 size={16} />
          </button>
        </div>
      </aside>
      <main>
        <div className="topbar">
          <span>
            工作空间 <ChevronRight size={14} />{" "}
            <strong>{currentPage.label}</strong>
          </span>
          <div>
            <span className="subtle">
              <ShieldCheck size={14} /> Evidence first
            </span>
            <button
              className="icon-button"
              aria-label="刷新数据"
              onClick={refresh}
            >
              <RefreshCw size={17} className={loading ? "spin" : ""} />
            </button>
            <button
              className="avatar"
              aria-label="API 连接设置"
              onClick={() => openModal("token")}
            >
              M
            </button>
          </div>
        </div>
        <div className="content">
          <header className="page-header">
            <div>
              <div className="eyebrow">{currentPage.en}</div>
              <h1>
                {currentPage.label}
                <span className="live-tag">
                  <i />
                  真实实验数据
                </span>
              </h1>
              <p>
                {page === "baselines"
                  ? "让每一次性能变化，都有可靠的参照。"
                  : page === "runs"
                    ? "管理测试生命周期，追溯每一份原始证据。"
                    : page === "versions"
                      ? "以不可变的版本身份，串联每一次测试。"
                      : page === "environments"
                        ? "记录云实例与物理硬件，固定公平对照的条件。"
                        : "在相同测试条件下，观察版本与时间带来的变化。"}
              </p>
            </div>
            <button
              className="primary"
              onClick={() =>
                openModal(
                  page === "versions"
                    ? "version"
                    : page === "environments"
                      ? "environment"
                      : "start",
                )
              }
            >
              <Plus size={17} />
              {page === "versions"
                ? "登记版本"
                : page === "environments"
                  ? "登记环境"
                  : "发起测试"}
            </button>
          </header>
          {error && (
            <div className="alert error" role="alert">
              {error}
              <button onClick={() => openModal("token")}>连接设置</button>
              <button onClick={refresh}>重试</button>
            </div>
          )}
          <section className="stats">
            <Stat
              icon={Layers3}
              label="已设基线"
              value={matchingBaselines.length}
              note="当前筛选条件"
            />
            <Stat
              icon={Check}
              label="成功实验"
              value={successful.length}
              note="证据已持久化"
            />
            <Stat
              icon={Server}
              label="硬件环境"
              value={environments.length}
              note="不可变配置快照"
            />
            <Stat
              icon={Activity}
              label="进行中"
              value={activeCount}
              note="准备 / 运行 / 排队"
            />
          </section>
          <section className="panel filters">
            <div className="filters-title">
              <Settings2 size={16} /> 实验条件
            </div>
            <Select label="硬件环境" value={env} onChange={chooseEnvironment}>
              <option value="">全部环境</option>
              {environments.map((e) => (
                <option key={e.id} value={e.id}>
                  {e.name} · {e.kind}
                </option>
              ))}
            </Select>
            <Select
              label="软件版本"
              value={version}
              onChange={(v) => {
                setVersion(v);
                setBaseline("");
                setCandidate("");
                setRuns([]);
                setSelected(null);
              }}
            >
              <option value="">全部版本</option>
              {versions.map((v) => (
                <option key={v.id} value={v.id}>
                  {v.filesystem} {v.label}
                </option>
              ))}
            </Select>
            <label className="select-label">
              <span>起始日期</span>
              <input
                aria-label="起始日期"
                type="date"
                value={from}
                onChange={(e) => setFrom(e.target.value)}
              />
            </label>
            <label className="select-label">
              <span>结束日期</span>
              <input
                aria-label="结束日期"
                type="date"
                value={to}
                onChange={(e) => setTo(e.target.value)}
              />
            </label>
          </section>
          {(page === "baselines" || page === "runs") && (
            <section className="panel results-panel">
              <div className="panel-top">
                <div className="tabs">
                  <button
                    className={kind === "data" ? "selected" : ""}
                    onClick={() => {
                      setKind("data");
                      setOperation("");
                      setFileSize("");
                      setBlockSize("");
                      setSelected(null);
                    }}
                  >
                    <Database size={16} />
                    数据 I/O
                  </button>
                  <button
                    className={kind === "metadata" ? "selected" : ""}
                    onClick={() => {
                      setKind("metadata");
                      setOperation("");
                      setFileSize("");
                      setBlockSize("");
                      setSelected(null);
                    }}
                  >
                    <Layers3 size={16} />
                    元数据
                  </button>
                  <button className={kind === "scenario" ? "selected" : ""} onClick={()=>{setKind("scenario");setOperation("");setFileSize("");setBlockSize("");setSelected(null);}}><Layers3 size={16}/>混合场景</button>
                </div>
                <span className="muted">
                  {visible.length} 次实验 · 最多加载 500 条
                </span>
              </div>
              <div className="table-controls">
                {kind === "data" && (
                  <>
                    <Select
                      label="读写类型"
                      value={operation}
                      onChange={setOperation}
                    >
                      <option value="">全部类型</option>
                      {Object.entries(operationLabel).map(([v, l]) => (
                        <option key={v} value={v}>
                          {l}
                        </option>
                      ))}
                    </Select>
                    <Select
                      label="文件大小"
                      value={fileSize}
                      onChange={setFileSize}
                    >
                      <option value="">全部大小</option>
                      {[
                        ...new Set(
                          scoped.map(
                            (r) => r.plan.spec.workload.file_size_bytes,
                          ),
                        ),
                      ]
                        .sort((a, b) => a - b)
                        .map((n) => (
                          <option key={n} value={n}>
                            {size(n)}
                          </option>
                        ))}
                    </Select>
                    <Select
                      label="I/O 块大小"
                      value={blockSize}
                      onChange={setBlockSize}
                    >
                      <option value="">全部大小</option>
                      {[
                        ...new Set(
                          scoped.map(
                            (r) => r.plan.spec.workload.block_size_bytes,
                          ),
                        ),
                      ]
                        .sort((a, b) => a - b)
                        .map((n) => (
                          <option key={n} value={n}>
                            {size(n)}
                          </option>
                        ))}
                    </Select>
                  </>
                )}
                <label className="search">
                  <Search size={16} />
                  <input
                    aria-label="搜索运行"
                    placeholder="搜索版本、负载或 Run ID"
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                  />
                </label>
              </div>
              {loading && !runs.length ? (
                <div className="empty loading">
                  <RefreshCw className="spin" />
                  正在加载实验…
                </div>
              ) : visible.length === 0 ? (
                <Empty />
              ) : (
                <div className="table-scroll">
                  <table>
                    <thead>
                      <tr>
                        <th>版本 / 测试负载</th>
                        <th>环境 / 日期</th>
                        {kind === "data" ? (
                          <>
                            <th>文件 / I/O 块</th>
                            <th>最高已测吞吐</th>
                          </>
                        ) : kind === "scenario" ? (<><th>阶段 / 子负载</th><th>完成情况</th></>) : (
                          <>
                            <th>对象 / 工具</th>
                            <th>Create / Stat / Remove</th>
                          </>
                        )}
                        <th>状态</th>
                        <th>对照</th>
                        <th />
                      </tr>
                    </thead>
                    <tbody>
                      {visible.map((r) => {
                        const p = peak(r),
                          isBaseline = baselines.some((b) => b.run_id === r.id),
                          summaries = summarize(r);
                        return (
                          <tr
                            key={r.id}
                            className={selected?.id === r.id ? "highlight" : ""}
                            onClick={() => {
                              setSelected(r);
                              setLogs(null);
                            }}
                          >
                            <td>
                              <strong>{r.plan.version.label}</strong>
                              <span>
                                {r.plan.spec.kind === "data"
                                  ? operationLabel[
                                      r.plan.spec.workload.operation
                                    ]
                                  : r.plan.spec.name}
                                <span className="tiny-dot">·</span>
                                {r.plan.version.filesystem}
                              </span>
                            </td>
                            <td>
                              <strong>{r.plan.environment.name}</strong>
                              <span>{date(r.created_at)}</span>
                            </td>
                            {kind === "data" ? (
                              <>
                                <td>
                                  <strong>
                                    {size(r.plan.spec.workload.file_size_bytes)}{" "}
                                    <span className="inline-muted">
                                      /{" "}
                                      {size(
                                        r.plan.spec.workload.block_size_bytes,
                                      )}
                                    </span>
                                  </strong>
                                  <span>
                                    {r.plan.spec.workload.direct
                                      ? "Direct"
                                      : "Buffered"}{" "}
                                    · {r.plan.spec.workload.cache_state}
                                  </span>
                                </td>
                                <td>
                                  <strong className="metric-number">
                                    {format(p?.median, "bytes/s")}
                                  </strong>
                                  <span>
                                    {p
                                      ? `${p.operation} · ${p.numjobs} jobs × depth ${p.iodepth}`
                                      : "无完整测量"}
                                  </span>
                                </td>
                              </>
                            ) : kind === "scenario" ? (<><td><strong>{r.plan.spec.scenario?.stages.length} 个阶段</strong><span>{r.plan.spec.scenario?.stages.map(s=>s.mode).join(" → ")}</span></td><td><strong>{r.trials.filter(t=>t.status==="succeeded").length} / {r.plan.trials.length} 子负载实验</strong><span>展开查看分组指标与测量时间线</span></td></>) : (
                              <>
                                <td>
                                  <strong>
                                    {r.plan.spec.metadata?.files_per_task ||
                                      "—"}{" "}
                                    文件 / task
                                  </strong>
                                  <span>{r.tool_version || "—"}</span>
                                </td>
                                <td>
                                  <strong className="metadata-rates">
                                    {["create", "stat", "remove"]
                                      .map((op) =>
                                        format(
                                          summaries.find(
                                            (s) => s.operation === op,
                                          )?.median,
                                          "ops/s",
                                        ),
                                      )
                                      .join(" / ")}
                                  </strong>
                                  <span>延迟未采集</span>
                                </td>
                              </>
                            )}
                            <td>
                              <Badge status={r.status} />
                              <span>
                                {r.plan.spec.repeats} 次重复 · {r.source}
                              </span>
                            </td>
                            <td>
                              {isBaseline ? (
                                <span className="baseline-tag">
                                  <ShieldCheck size={13} />
                                  基线
                                </span>
                              ) : (
                                <button
                                  className="text-button"
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    setCandidate(r.id);
                                    setPage("trends");
                                  }}
                                >
                                  加入对比
                                </button>
                              )}
                            </td>
                            <td>
                              <button
                                className="icon-button"
                                aria-label={`查看运行 ${r.id}`}
                              >
                                <ChevronRight size={17} />
                              </button>
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              )}
              <div className="panel-foot">
                <Leaf size={14} />
                {kind === "data"
                  ? "最高值仅代表已测矩阵中的重复中位数峰值，不代表系统绝对极限。"
                  : "元数据当前支持 mdtest 4.0.0 的文件 create、stat、remove；未采集指标保留为空。"}
              </div>
            </section>
          )}
          {page === "versions" && (
            <section className="panel">
              <PanelTitle
                title="软件版本登记"
                subtitle="同一 ID 的内容不可覆盖；更新构建时请登记新版本。"
              />
              {versions.length === 0 ? (
                <Empty
                  title="还没有登记版本"
                  text="登记文件系统、版本标签、commit 和构建身份。"
                />
              ) : (
                <div className="table-scroll">
                  <table>
                    <thead>
                      <tr>
                        <th>文件系统 / 版本</th>
                        <th>版本 ID</th>
                        <th>Commit</th>
                        <th>Build / Image</th>
                        <th>筛选内测试</th>
                        <th />
                      </tr>
                    </thead>
                    <tbody>
                      {versions.map((v) => (
                        <tr key={v.id}>
                          <td>
                            <strong>{v.label}</strong>
                            <span>{v.filesystem}</span>
                          </td>
                          <td className="mono">{v.id}</td>
                          <td className="mono">{v.commit || "未登记"}</td>
                          <td className="mono wrap">{v.build || "未登记"}</td>
                          <td>
                            {
                              runs.filter((r) => r.plan.version.id === v.id)
                                .length
                            }{" "}
                            次
                          </td>
                          <td>
                            <button
                              className="text-button"
                              onClick={() => {
                                setVersion(v.id);
                                setPage("baselines");
                              }}
                            >
                              查看测试 <ArrowRight size={14} />
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </section>
          )}
          {page === "environments" && (
            <div className="environment-grid">
              {environments.length === 0 ? (
                <section className="panel">
                  <Empty
                    title="还没有硬件环境"
                    text="记录云环境与实例类型，或物理机器型号及客户端 / 服务端拓扑。"
                  />
                </section>
              ) : (
                environments.map((e) => (
                  <section className="panel environment-card" key={e.id}>
                    <header>
                      <span className="environment-icon">
                        <Server size={22} />
                      </span>
                      <span className="pill">{e.kind}</span>
                    </header>
                    <h2>{e.name}</h2>
                    <p className="mono muted">{e.id}</p>
                    <div className="environment-location">
                      {[e.provider, e.region].filter(Boolean).join(" / ") ||
                        "未登记云区域"}
                      <span>{e.os || "OS 未登记"}</span>
                    </div>
                    {e.machines.map((m, i) => (
                      <div className="machine" key={i}>
                        <div>
                          <strong>{m.role}</strong>
                          <span>{m.count} 台</span>
                        </div>
                        <b>{m.model}</b>
                        <p>
                          {m.cpu} ·{" "}
                          {m.memory_bytes ? size(m.memory_bytes) : "内存未登记"}
                        </p>
                        <p>
                          {m.storage} · {m.network}
                        </p>
                      </div>
                    ))}
                    <details>
                      <summary>配置快照</summary>
                      <pre>{JSON.stringify(e.config, null, 2)}</pre>
                    </details>
                    <button
                      className="secondary full"
                      onClick={() => {
                        chooseEnvironment(e.id);
                        setPage("baselines");
                      }}
                    >
                      查看环境基线 <ArrowRight size={15} />
                    </button>
                  </section>
                ))
              )}
            </div>
          )}
          {page === "trends" && (
            <>
              <section className="panel comparison-selector">
                <PanelTitle
                  title="选择对照实验"
                  subtitle="服务端核对硬件、负载与工具版本后，才计算变化百分比。"
                />
                <div className="compare-picks">
                  <Select
                    label="基线运行"
                    value={baseline}
                    onChange={setBaseline}
                  >
                    <option value="">选择一份基线或成功运行</option>
                    {runs
                      .filter((r) => r.status === "succeeded")
                      .map((r) => (
                        <option key={r.id} value={r.id}>
                          {baselines.find((b) => b.run_id === r.id)?.name ||
                            r.plan.version.label}{" "}
                          · {r.plan.environment.name} · {date(r.created_at)} ·{" "}
                          {r.id.slice(-6)}
                        </option>
                      ))}
                  </Select>
                  <ArrowRight size={20} />
                  <Select
                    label="候选运行"
                    value={candidate}
                    onChange={setCandidate}
                  >
                    <option value="">选择待比较运行</option>
                    {runs.map((r) => (
                      <option key={r.id} value={r.id}>
                        {r.plan.version.label} · {r.plan.environment.name} ·{" "}
                        {date(r.created_at)} · {r.id.slice(-6)}
                      </option>
                    ))}
                  </Select>
                </div>
                {compareError && (
                  <div className="alert error">{compareError}</div>
                )}
                {compareLoading && <p className="muted">正在核对实验条件…</p>}
                {comparison && (
                  <div className="comparison-body">
                    <div
                      className={`alert ${comparison.comparable ? "success" : "warning"}`}
                    >
                      <ShieldCheck size={18} />
                      <strong>
                        {comparison.comparable
                          ? "条件一致，可进行对照"
                          : "条件不一致，不计算性能变化"}
                      </strong>
                    </div>
                    {comparison.differences.map((d, i) => (
                      <p key={i} className="difference">
                        <b>{d.field}</b> {d.reason}
                      </p>
                    ))}
                    {comparison.warnings.map((w, i) => (
                      <p className="warning-text" key={i}>
                        {w}
                      </p>
                    ))}
                    {comparison.comparable && (
                      <div className="table-scroll">
                        <table>
                          <thead>
                            <tr>
                              <th>指标 / 并发</th>
                              <th>基线中位数</th>
                              <th>候选中位数</th>
                              <th>数值变化</th>
                              <th>候选 Min – Max</th>
                              <th>重复</th>
                            </tr>
                          </thead>
                          <tbody>
                            {comparison.metrics.map((m) => (
                              <tr key={m.baseline.key}>
                                <td>
                                  <strong>
                                    {m.baseline.stage_id && <small>{m.baseline.stage_id} / {m.baseline.workload_id}<br/></small>}{m.baseline.operation} ·{" "}
                                    {metricLabel[m.baseline.metric]}
                                  </strong>
                                  <span>
                                    {m.baseline.numjobs} jobs × depth{" "}
                                    {m.baseline.iodepth}
                                  </span>
                                </td>
                                <td>
                                  {format(m.baseline.median, m.baseline.unit)}
                                </td>
                                <td>
                                  <strong>
                                    {format(
                                      m.candidate.median,
                                      m.candidate.unit,
                                    )}
                                  </strong>
                                </td>
                                <td>
                                  <span
                                    className={`delta ${m.percent === null ? "" : m.percent >= 0 === (m.direction === "higher-is-better") ? "good" : "bad"}`}
                                  >
                                    {m.percent === null
                                      ? "零基线 / 无百分比"
                                      : `${m.percent > 0 ? "+" : ""}${m.percent.toFixed(2)}%`}
                                  </span>
                                  <span>
                                    {m.direction === "lower-is-better"
                                      ? "越低越好"
                                      : "越高越好"}
                                  </span>
                                </td>
                                <td>
                                  {format(m.candidate.min, m.candidate.unit)} –{" "}
                                  {format(m.candidate.max, m.candidate.unit)}
                                </td>
                                <td>{m.candidate.count} 次</td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    )}
                  </div>
                )}
                {!baseline && (
                  <Empty
                    title="选择一个可追溯的参照点"
                    text="比较结果会同时展示可比性、重复实验波动和每项指标的变化。"
                  />
                )}
              </section>
              {trend && (
                <section className="panel">
                  <div className="panel-top">
                    <div>
                      <h2>同条件日期趋势</h2>
                      <p className="muted">
                        固定环境、负载、工具版本；每个点对应一次完整运行。
                      </p>
                    </div>
                    <Select
                      label="观察指标"
                      value={activeChartKey}
                      onChange={setChartKey}
                    >
                      {trendKeys.map((s) => (
                        <option key={s.key} value={s.key}>
                          {s.stage_id ? `${s.stage_id}/${s.workload_id} · ` : ""}{s.operation} · {metricLabel[s.metric]} · {s.numjobs}j
                          / {s.iodepth}d
                        </option>
                      ))}
                    </Select>
                  </div>
                  {trend.points.length ? (
                    <Chart
                      option={trendOption}
                      label="同条件运行的指标中位数时间趋势"
                    />
                  ) : (
                    <Empty title="暂无满足条件的趋势数据" />
                  )}
                  <div className="panel-foot">
                    最多核对该环境最近 500 次成功运行 · 当前{" "}
                    {trend.points.length} 个可比点 · 日期按浏览器时区展示
                  </div>
                </section>
              )}
            </>
          )}
          {selected && (page === "baselines" || page === "runs") && (
            <section className="panel run-detail">
              <div className="panel-top">
                <div>
                  <div className="eyebrow">RUN DETAIL</div>
                  <h2>{selected.plan.spec.name}</h2>
                  <p className="mono muted">{selected.id}</p>
                </div>
                <button
                  className="icon-button"
                  aria-label="关闭运行详情"
                  onClick={() => setSelected(null)}
                >
                  <X size={20} />
                </button>
              </div>
              <div className="detail-actions">
                <Badge status={selected.status} />
                <span className="pill">{selected.attempt.executor}</span>
                <span className="muted">
                  证据: {selected.attempt.result_state}
                </span>
                <div className="spacer" />
                {selected.status === "succeeded" &&
                  selected.attempt.result_state === "complete" && (
                    <button
                      className="secondary"
                      disabled={busy}
                      onClick={() => openModal("baseline")}
                    >
                      <ShieldCheck size={15} />
                      设为基线
                    </button>
                  )}
                <button
                  className="secondary"
                  disabled={
                    busy || selected.attempt.result_state !== "complete"
                  }
                  onClick={() =>
                    notifyAction(
                      () => downloadBundle(selected.id),
                      "结果包已下载",
                    )
                  }
                >
                  <ArrowDownToLine size={15} />
                  下载证据
                </button>
                {!terminal(selected.status) && (
                  <button
                    className="danger"
                    disabled={busy}
                    onClick={() =>
                      notifyAction(
                        () => api(`/runs/${selected.id}/cancel`, {}),
                        "已请求取消",
                      )
                    }
                  >
                    取消测试
                  </button>
                )}
              </div>
              {selected.error && (
                <div className="alert error">{selected.error}</div>
              )}
              <div className="detail-grid">
                <Detail
                  label="版本 / 环境"
                  value={`${selected.plan.version.label} / ${selected.plan.environment.name}`}
                />
                <Detail
                  label="调用来源 / 工具"
                  value={`${selected.source} / ${selected.tool_version || "尚未检测"}`}
                />
                <Detail
                  label="资源组 / 目标"
                  value={`${selected.plan.resource_group} / ${selected.plan.target}`}
                />
                <Detail
                  label="Request ID"
                  value={selected.request_id || "离线导入"}
                />
              </div>
              <RunMetrics run={selected} />
              <div className="detail-bottom">
                <details>
                  <summary>执行 Attempt 与集群状态</summary>
                  <pre>{JSON.stringify(selected.attempt, null, 2)}</pre>
                </details>
                <details>
                  <summary>完整执行计划与不可变快照</summary>
                  <pre>{JSON.stringify(selected.plan, null, 2)}</pre>
                </details>
                <button
                  className="secondary"
                  onClick={async () => {
                    try {
                      setLogs(await api(`/runs/${selected.id}/logs`));
                    } catch (e) {
                      setError((e as Error).message);
                    }
                  }}
                >
                  <Terminal size={15} />
                  读取日志
                </button>
              </div>
              {logs !== null && (
                <pre className="logs">{JSON.stringify(logs, null, 2)}</pre>
              )}
            </section>
          )}
          <footer className="page-footer">
            <span>
              malta<span className="brand-dot">.</span>{" "}
              <span>Agent 增强的性能对照平台</span>
            </span>
            <span>服务与测试 Pod 分离 · 统一性能证据与基线</span>
          </footer>
        </div>
      </main>
      {toast && (
        <div className="toast" role="status">
          <Check size={17} />
          {toast}
        </div>
      )}
      {modal && (
        <Dialog
          title={
            modal === "token"
              ? "API 连接设置"
              : modal === "start"
                ? "发起性能测试"
                : modal === "baseline"
                  ? "保存性能基线"
                  : modal === "environment"
                    ? "登记硬件环境"
                    : "登记软件版本"
          }
          onClose={() => setModal("")}
        >
          <form onSubmit={submitModal}>
            {modal === "token" ? (
              <>
                <p>凭证仅保存在当前浏览器会话中。服务未配置 Token 时可留空。</p>
                <label>
                  Bearer Token
                  <input
                    type="password"
                    autoComplete="off"
                    value={token}
                    onChange={(e) => setToken(e.target.value)}
                  />
                </label>
              </>
            ) : modal === "baseline" ? (
              <>
                <p>
                  基线将固定关联到这次成功运行：<code>{selected?.id}</code>。
                </p>
                <label>
                  基线名称
                  <input
                    required
                    autoFocus
                    value={baselineName}
                    onChange={(e) => setBaselineName(e.target.value)}
                    placeholder="例如 release-1.0 / 云环境随机读"
                  />
                </label>
              </>
            ) : (
              <>
                <p>
                  {modal === "start"
                    ? "粘贴 malta plan 生成的完整 JSON。选择 local 在服务节点执行，或 kubernetes 在已配置的独立 Job 中执行真实 I/O。"
                    : "粘贴与版本化 schema 一致的 JSON。相同 ID 不能覆盖不同内容；请为变更登记新 ID。"}
                </p>
                {modal === "start" && (
                  <label>
                    执行器
                    <select
                      value={executor}
                      onChange={(e) => setExecutor(e.target.value)}
                    >
                      {executors.map((name) => (
                        <option key={name} value={name}>
                          {name === "local"
                            ? "Local · 服务节点"
                            : "Kubernetes · 独立 Runner Job"}
                        </option>
                      ))}
                    </select>
                  </label>
                )}
                {modal === "start" && (
                  <label>
                    Request ID
                    <input
                      required
                      pattern="[a-zA-Z0-9][a-zA-Z0-9._-]{0,99}"
                      value={requestId}
                      onChange={(e) => setRequestId(e.target.value)}
                      placeholder="同一次实验重试时保持不变"
                    />
                  </label>
                )}
                <label>
                  {modal === "start" ? "执行计划 JSON" : "登记内容 JSON"}
                  <textarea
                    required
                    rows={14}
                    spellCheck={false}
                    value={jsonInput}
                    onChange={(e) => setJSONInput(e.target.value)}
                    placeholder={
                      modal === "environment"
                        ? "参考 environments/local.yaml 对应的 JSON；包含云厂商、区域、机器型号和 client/server 角色。"
                        : modal === "version"
                          ? '{"schema_version":"1.0","id":"...","filesystem":"...","label":"...","commit":"...","build":"..."}'
                          : "粘贴完整 plan JSON；计划哈希由服务端重新校验。"
                    }
                  />
                </label>
                {modal === "start" && (
                  <div className="alert success">
                    <Play size={16} />
                    执行后端：{executor} · 串行队列 · 服务端校验资源预算
                  </div>
                )}
              </>
            )}
            {modalError && (
              <div className="alert error" role="alert">
                {modalError}
              </div>
            )}
            <div className="dialog-actions">
              <button
                type="button"
                className="secondary"
                onClick={() => setModal("")}
              >
                取消
              </button>
              <button className="primary" disabled={busy}>
                {busy ? "正在提交…" : modal === "start" ? "提交测试" : "保存"}
              </button>
            </div>
          </form>
        </Dialog>
      )}
    </div>
  );
}
