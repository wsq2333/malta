import {
  workspaceSection,
  workspaceNavigation,
  workspaceTabs,
} from "./workspace";
import { AutomationPreview } from "./AutomationPreview";
import { useEffect, useMemo, useState, type FormEvent } from "react";
import {
  Activity,
  BookOpen,
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
  Sun,
  Moon,
  Server,
  Settings2,
  ShieldCheck,
  Terminal,
  TrendingUp,
  X,
} from "lucide-react";
import { RunCatalog } from "./RunCatalog";
import { Versions } from "./Versions";
import {
  BackToList,
  WorkloadFacts,
  TechnicalInfo,
  RecordCard,
  Facts,
} from "./RecordViews";
import { catalogRoute } from "./catalog";
import { Issues } from "./Issues";
import { HardwareEnvironments } from "./HardwareEnvironments";
import { groupHardware, hardwareLabel } from "./hardware";
import { Guide } from "./Guide";
import { Chart } from "./LazyChart";
import { Cases, CasePicker, type CaseVersion } from "./Cases";
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

type Page =
  | "automation"
  | "suites"
  | "guide"
  | "cases"
  | "issues"
  | "baselines"
  | "runs"
  | "versions"
  | "environments"
  | "trends";
const pages: { id: Page; label: string; en: string; icon: typeof Activity }[] =
  [
    { id: "automation", label: "测试中心", en: "Test center", icon: Activity },
    { id: "suites", label: "用例管理", en: "Case library", icon: Layers3 },
    {
      id: "baselines",
      label: "测试中心",
      en: "Test center",
      icon: Layers3,
    },
    {
      id: "issues",
      label: "问题管理",
      en: "Issue tracking",
      icon: ShieldCheck,
    },
    { id: "cases", label: "用例管理", en: "Benchmark cases", icon: Layers3 },
    {
      id: "versions",
      label: "环境与版本",
      en: "Environments & versions",
      icon: GitBranch,
    },
    {
      id: "environments",
      label: "环境与版本",
      en: "Environments & versions",
      icon: Server,
    },
    {
      id: "trends",
      label: "测试中心",
      en: "Test center",
      icon: TrendingUp,
    },
    { id: "guide", label: "使用指南", en: "User guide", icon: BookOpen },
  ];
export function App() {
  const [theme, setTheme] = useState(
    document.documentElement.dataset.theme || "dark",
  );
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    try {
      localStorage.setItem("malta-theme", theme);
    } catch {
      /* Storage can be disabled. */
    }
  }, [theme]);
  const initial = new URLSearchParams(location.search);
  const route = catalogRoute(initial);
  const [page, setPageState] = useState<Page>(
    pages.find((p) => p.id === route.page)?.id || "baselines",
  );
  const [catalogView, setCatalogView] = useState(route.view);
  const [detail, setDetailState] = useState(initial.get("detail") || "");
  const [detailLoading, setDetailLoading] = useState(false);
  function setDetail(id: string) {
    const q = new URLSearchParams(location.search);
    if (id) q.set("detail", id);
    else q.delete("detail");
    history.pushState(null, "", `?${q}`);
    setDetailState(id);
    setLogs(null);
  }
  function setPage(next: Page) {
    const canonical = next === "runs" ? "baselines" : next;
    const q = new URLSearchParams(location.search);
    q.set("page", canonical);
    q.delete("detail");
    if (next === "runs") q.set("view", "all");
    history.pushState(null, "", `?${q}`);
    setPageState(canonical);
    setDetailState("");
    setSelected(null);
    if (next === "runs") setCatalogView("all");
  }
  function showRun(id: string) {
    if (page !== "baselines") setPage("runs");
    setDetail(id);
  }

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
    [managementOnly, setManagementOnly] = useState(false),
    [baselineName, setBaselineName] = useState(""),
    [modalError, setModalError] = useState(""),
    [logs, setLogs] = useState<unknown>(null);
  const [issueCreate, setIssueCreate] = useState(0);
  const [caseCreate, setCaseCreate] = useState(0),
    [chosenCase, setChosenCase] = useState<CaseVersion | null>(null),
    [startMode, setStartMode] = useState("json");
  const refresh = () => setTick((t) => t + 1);
  useEffect(() => {
    const q = new URLSearchParams();
    Object.entries({
      page,
      view: catalogView,
      detail,
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
    catalogView,
    detail,
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
    const restore = () => {
      const q = new URLSearchParams(location.search),
        route = catalogRoute(q);
      setPageState(pages.find((p) => p.id === route.page)?.id || "baselines");
      setCatalogView(route.view);
      setDetailState(q.get("detail") || "");
      setSelected(null);
      setKind(q.get("kind") || "data");
      setEnv(q.get("environment") || "");
      setVersion(q.get("version") || "");
      setOperation(q.get("operation") || "");
      setFileSize(q.get("file_size") || "");
      setBlockSize(q.get("block_size") || "");
      setFrom(q.get("from") || "");
      setTo(q.get("to") || "");
      setBaseline(q.get("baseline") || "");
      setCandidate(q.get("candidate") || "");
    };
    window.addEventListener("popstate", restore);
    return () => window.removeEventListener("popstate", restore);
  }, []);
  useEffect(() => {
    let live = true;
    if (!detail || page !== "baselines") {
      setSelected(null);
      return;
    }
    setDetailLoading(true);
    setSelected((old) => (old?.id === detail ? old : null));
    api<Run>(`/runs/${encodeURIComponent(detail)}`)
      .then((r) => {
        if (live) setSelected(r);
      })
      .catch((e) => {
        if (live) setError(e.message);
      })
      .finally(() => {
        if (live) setDetailLoading(false);
      });
    return () => {
      live = false;
    };
  }, [detail, page, tick]);
  const hardwareGroups = useMemo(
    () => groupHardware(environments),
    [environments],
  );
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
      api<{ executors: string[]; features: { management_only?: boolean } }>(
        "/capabilities",
      ),
    ])
      .then(([r, e, v, b, caps]) => {
        if (valid) {
          setRuns(r);
          setEnvironments(e);
          setVersions(v);
          setBaselines(b);
          setExecutors(caps.executors);
          setManagementOnly(caps.features.management_only === true);
          setExecutor((old) =>
            caps.executors.includes(old) ? old : caps.executors[0] || "",
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
      ) &&
      (!selected ||
        (terminal(selected.status) &&
          selected.attempt.result_state !== "collecting"))
    )
      return;
    const timer = setInterval(refresh, 2000);
    return () => clearInterval(timer);
  }, [runs, selected]);
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
  const prototype = page === "automation" || page === "suites";
  const tabs = workspaceTabs(page);
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
    if (name === "start" && !executors.length) {
      setToast("当前服务未启用测试执行，可先管理用例与结果。");
      return;
    }
    setModalError("");
    setJSONInput("");
    setRequestId("");
    setBaselineName("");
    setChosenCase(null);
    setStartMode("json");
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
        if (startMode === "case" && !chosenCase)
          throw new Error("请选择用例及固定版本");
        const plan = chosenCase ? undefined : (JSON.parse(jsonInput) as Plan);
        const run = await api<Run>("/runs", {
          plan,
          case_ref: chosenCase
            ? { id: chosenCase.case_id, revision: chosenCase.revision }
            : undefined,
          request_id: requestId,
          source: "human",
          executor,
        });
        setCatalogView("all");
        showRun(run.id);
        setSelected(run);
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
      },
      yAxis: {
        type: "value",
        name: unit,
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
          areaStyle: { opacity: 0.08 },
        },
      ],
    };
  }, [trend, activeChartKey]);
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <a className="brand" href="?page=baselines&view=all">
          <span className="brand-mark">
            <Layers3 size={25} />
          </span>
          <span>
            malta<span className="brand-dot">.</span>
          </span>
        </a>
        <div className="workspace-label">BENCHMARK WORKSPACE</div>
        <nav>
          {workspaceNavigation.map((item) => {
            const p = pages.find((p) => p.id === item.page)!;
            return (
              <button
                key={p.id}
                aria-label={p.label}
                title={p.label}
                onClick={() => setPage(p.id === "baselines" ? "runs" : p.id)}
                className={workspaceSection(page) === p.id ? "active" : ""}
              >
                <p.icon size={19} />
                <span>{p.label}</span>
                {p.id === "baselines" && activeCount > 0 && (
                  <b>{activeCount}</b>
                )}
                {workspaceSection(page) === p.id && <ChevronRight size={16} />}
              </button>
            );
          })}
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
              <small>
                {managementOnly
                  ? "仅管理服务 · 测试未启用"
                  : executors.join(" / ") || "读取中"}
              </small>
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
              className="icon-button theme-toggle"
              aria-label={theme === "dark" ? "切换浅色主题" : "切换深色主题"}
              title={theme === "dark" ? "切换浅色主题" : "切换深色主题"}
              onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
            >
              {theme === "dark" ? <Sun size={17} /> : <Moon size={17} />}
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
                  {prototype
                    ? "交互原型 · 示例数据"
                    : page === "guide"
                      ? "入门与示例"
                      : "真实实验数据"}
                </span>
              </h1>
              <p>
                {prototype
                  ? "预览自动化验证与问题追踪流程，尚未接入执行服务。"
                  : page === "guide"
                    ? "从填写 JSON 到发起测试，逐步建立可靠的性能对照。"
                    : page === "issues"
                      ? "记录测试问题、追溯调查过程与解决方式。"
                      : page === "cases"
                        ? "保存测试定义，以不可变版本组织每一次实验。"
                        : page === "baselines"
                          ? "让每一次性能变化，都有可靠的参照。"
                          : page === "runs"
                            ? "管理测试生命周期，追溯每一份原始证据。"
                            : page === "versions"
                              ? "以不可变的版本身份，串联每一次测试。"
                              : page === "environments"
                                ? "按云环境与各角色机型归组，在执行快照中保留具体测试条件。"
                                : "在相同测试条件下，观察版本与时间带来的变化。"}
              </p>
            </div>
            {!prototype && (
              <button
                className="primary"
                disabled={
                  !executors.length &&
                  ![
                    "guide",
                    "issues",
                    "cases",
                    "versions",
                    "environments",
                  ].includes(page)
                }
                onClick={() =>
                  page === "guide"
                    ? setPage("cases")
                    : page === "issues"
                      ? setIssueCreate((n) => n + 1)
                      : page === "cases"
                        ? setCaseCreate((n) => n + 1)
                        : openModal(
                            page === "versions"
                              ? "version"
                              : page === "environments"
                                ? "environment"
                                : "start",
                          )
                }
              >
                <Plus size={17} />
                {page === "guide"
                  ? "前往用例管理"
                  : page === "issues"
                    ? "登记问题"
                    : page === "cases"
                      ? "新建用例"
                      : page === "versions"
                        ? "登记版本"
                        : page === "environments"
                          ? "登记执行快照"
                          : "发起测试"}
              </button>
            )}
          </header>
          {!detail && tabs.length > 0 && (
            <div
              className="workspace-tabs"
              role="tablist"
              aria-label="工作区分类"
            >
              {tabs.map((t) => (
                <button
                  key={t.page}
                  role="tab"
                  aria-selected={
                    page === t.page ||
                    (page === "trends" && t.page === "baselines")
                  }
                  onClick={() => setPage(t.page as Page)}
                >
                  {t.label}
                </button>
              ))}
              {page === "baselines" && (
                <button
                  className="secondary workspace-compare"
                  onClick={() => setPage("trends")}
                >
                  <TrendingUp size={16} />
                  对比与趋势
                </button>
              )}
            </div>
          )}
          {page === "trends" && (
            <BackToList
              label="返回性能测试"
              onClick={() => setPage("baselines")}
            />
          )}
          {prototype && (
            <AutomationPreview
              mode={page === "suites" ? "suites" : "runs"}
              detail={detail}
              onDetail={setDetail}
              onNavigate={(next) => setPage(next)}
            />
          )}
          {managementOnly && (
            <div className="alert">
              当前为仅管理服务：可管理问题、用例、版本、环境与已有结果；测试执行尚未启用。
            </div>
          )}
          {error && page !== "guide" && (
            <div className="alert error" role="alert">
              {error}
              <button onClick={() => openModal("token")}>连接设置</button>
              <button onClick={refresh}>重试</button>
            </div>
          )}
          {page === "guide" && <Guide onNavigate={setPage} />}
          {page === "issues" && (
            <Issues
              tick={tick}
              createSignal={issueCreate}
              active={detail}
              setActive={setDetail}
            />
          )}
          {!detail && (page === "baselines" || page === "trends") && (
            <>
              <div className="workspace-summary" aria-label="测试概况">
                <span>
                  <b>{matchingBaselines.length}</b> 已设基线
                </span>
                <span>
                  <b>{successful.length}</b> 成功实验
                </span>
                <span>
                  <b>{activeCount}</b> 进行中
                </span>
                <span>
                  <b>{hardwareGroups.length}</b> 类硬件
                </span>
              </div>
              <details
                className="workspace-filters"
                open={env || version || from || to ? true : undefined}
              >
                <summary>
                  版本、环境与日期筛选
                  {env || version || from || to ? " · 已启用" : ""}
                </summary>
                <section className="panel filters">
                  <div className="filters-title">
                    <Settings2 size={16} /> 实验条件
                  </div>
                  <Select
                    label="执行快照"
                    value={env}
                    onChange={chooseEnvironment}
                  >
                    <option value="">全部执行快照</option>
                    {hardwareGroups.map((group) => (
                      <optgroup key={group.key} label={hardwareLabel(group)}>
                        {group.snapshots.map((snapshot) => (
                          <option key={snapshot.id} value={snapshot.id}>
                            {snapshot.name} · {snapshot.id}
                          </option>
                        ))}
                      </optgroup>
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
              </details>
            </>
          )}
          {page === "cases" && (
            <Cases
              tick={tick}
              createSignal={caseCreate}
              activeID={detail}
              setActiveID={setDetail}
              runs={runs}
              canStart={executors.length > 0}
              onStart={(v) => {
                openModal("start");
                setStartMode("case");
                setChosenCase(v);
                setJSONInput(JSON.stringify(v.plan, null, 2));
                setRequestId(`case-${crypto.randomUUID()}`);
              }}
            />
          )}
          {page === "baselines" && !detail && (
            <section className="panel results-panel">
              <div className="catalog-toolbar">
                <label>
                  显示范围{" "}
                  <select
                    aria-label="测试显示范围"
                    value={catalogView}
                    onChange={(e) => setCatalogView(e.target.value)}
                  >
                    <option value="all">全部测试</option>
                    <option value="saved">已设基线</option>
                  </select>
                </label>
                <span className="muted">数据 I/O、元数据与混合场景</span>
              </div>
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
                  <button
                    className={kind === "scenario" ? "selected" : ""}
                    onClick={() => {
                      setKind("scenario");
                      setOperation("");
                      setFileSize("");
                      setBlockSize("");
                      setSelected(null);
                    }}
                  >
                    <Layers3 size={16} />
                    混合场景
                  </button>
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
                <p className="case-padding">正在加载测试…</p>
              ) : (
                <RunCatalog
                  runs={visible}
                  allRuns={runs}
                  baselines={baselines}
                  view={catalogView}
                  filtered={Boolean(
                    env ||
                    version ||
                    from ||
                    to ||
                    operation ||
                    fileSize ||
                    blockSize ||
                    search,
                  )}
                  onOpen={showRun}
                />
              )}
              <div className="panel-foot">
                <Leaf size={14} />
                {kind === "data"
                  ? "最高值仅代表已测矩阵中的重复中位数峰值，不代表系统绝对极限。"
                  : kind === "scenario"
                    ? "混合场景按子负载分别统计；展开查看实际配比、测量时间线与独立指标。"
                    : "元数据当前支持 mdtest 4.0.0 的文件 create、stat、remove；未采集指标保留为空。"}
              </div>
            </section>
          )}
          {page === "versions" && (
            <Versions
              versions={versions}
              detail={detail}
              onDetail={setDetail}
              onRun={showRun}
            />
          )}
          {page === "environments" && (
            <HardwareEnvironments
              groups={hardwareGroups}
              detail={detail}
              onDetail={setDetail}
              onRun={showRun}
              onSelectSnapshot={(id) => {
                chooseEnvironment(id);
                setPage("runs");
              }}
            />
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
                <div className="record-grid">
                  {[
                    [baseline, "基线测试"],
                    [candidate, "候选测试"],
                  ].map(([id, label]) => {
                    const run = runs.find((r) => r.id === id);
                    return run ? (
                      <RecordCard
                        key={label}
                        title={`${label} · ${run.plan.version.label}`}
                        subtitle={<Badge status={run.status} />}
                        onOpen={() => showRun(run.id)}
                      >
                        <Facts
                          items={[
                            ["测试负载", run.plan.spec.name],
                            ["时间", date(run.created_at)],
                            [
                              "云环境",
                              run.plan.environment.provider ||
                                run.plan.environment.kind,
                            ],
                          ]}
                        />
                      </RecordCard>
                    ) : null;
                  })}
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
                      <div className="comparison-summary">
                        <Facts
                          items={comparison.metrics
                            .slice(0, 3)
                            .map((m) => [
                              `${m.baseline.operation} · ${metricLabel[m.baseline.metric] || m.baseline.metric} · ${m.baseline.numjobs}j / ${m.baseline.iodepth}d`,
                              `${format(m.baseline.median, m.baseline.unit)} → ${format(m.candidate.median, m.candidate.unit)}（${m.percent === null ? "无百分比" : `${m.percent.toFixed(2)}%`}）`,
                            ])}
                        />
                        <p className="muted">
                          概览展示前 3 项，展开下方查看全部指标。
                        </p>
                      </div>
                    )}
                    {comparison.comparable && (
                      <details className="comparison-metrics">
                        <summary>指标对照详情</summary>
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
                                      {m.baseline.stage_id && (
                                        <small>
                                          {m.baseline.stage_id} /{" "}
                                          {m.baseline.workload_id}
                                          <br />
                                        </small>
                                      )}
                                      {m.baseline.operation} ·{" "}
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
                                    {format(m.candidate.min, m.candidate.unit)}{" "}
                                    –{" "}
                                    {format(m.candidate.max, m.candidate.unit)}
                                  </td>
                                  <td>{m.candidate.count} 次</td>
                                </tr>
                              ))}
                            </tbody>
                          </table>
                        </div>
                      </details>
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
                          {s.stage_id
                            ? `${s.stage_id}/${s.workload_id} · `
                            : ""}
                          {s.operation} · {metricLabel[s.metric]} · {s.numjobs}j
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
          {detail && page === "baselines" && (
            <BackToList
              label={catalogView === "saved" ? "返回已设基线" : "返回全部测试"}
              onClick={() => {
                setDetail("");
                setSelected(null);
              }}
            />
          )}
          {detail && page === "baselines" && detailLoading && !selected && (
            <p>正在加载测试详情…</p>
          )}
          {detail && selected && page === "baselines" && (
            <section className="panel run-detail">
              <div className="panel-top">
                <div>
                  <div className="eyebrow">RUN DETAIL</div>
                  <h2>{selected.plan.spec.name}</h2>
                  <p className="muted">
                    {date(selected.created_at)} · {selected.plan.version.label}
                  </p>
                  {selected.case_ref && (
                    <p className="muted">
                      用例修订 v{selected.case_ref.revision}
                    </p>
                  )}
                </div>
                <button
                  className="icon-button"
                  aria-label="关闭运行详情"
                  onClick={() => {
                    setDetail("");
                    setSelected(null);
                  }}
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
              <details className="technical-info">
                <summary>测试负载参数</summary>
                <WorkloadFacts plan={selected.plan} />
              </details>
              <RunMetrics run={selected} />
              <button
                className="secondary"
                onClick={() => {
                  setCandidate(selected.id);
                  setPage("trends");
                }}
              >
                与其他测试对比
              </button>
              <TechnicalInfo
                value={{
                  run_id: selected.id,
                  request_id: selected.request_id,
                  case_ref: selected.case_ref,
                }}
              >
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
              </TechnicalInfo>
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
                    ? "选择已保存的用例版本，或粘贴完整计划 JSON。提交前请核对目标目录、环境和软件版本，执行时仍会校验资源预算。"
                    : "粘贴与版本化 schema 一致的 JSON。相同 ID 不能覆盖不同内容；请为变更登记新 ID。"}
                </p>
                {modal === "start" && (
                  <>
                    <label>
                      计划来源
                      <select
                        value={startMode}
                        onChange={(e) => {
                          setStartMode(e.target.value);
                          setChosenCase(null);
                          setJSONInput("");
                        }}
                      >
                        <option value="json">粘贴计划 JSON</option>
                        <option value="case">选择已保存的用例</option>
                      </select>
                    </label>
                    {startMode === "case" && (
                      <>
                        <CasePicker
                          initial={chosenCase}
                          onSelect={(v) => {
                            setChosenCase(v);
                            setJSONInput(
                              v ? JSON.stringify(v.plan, null, 2) : "",
                            );
                          }}
                        />
                        {chosenCase && (
                          <div className="alert success">
                            已固定：{chosenCase.name} · v{chosenCase.revision}
                          </div>
                        )}
                      </>
                    )}
                  </>
                )}
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
                    readOnly={modal === "start" && startMode === "case"}
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
