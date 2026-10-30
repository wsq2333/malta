import { useEffect, useMemo, useState } from "react";
import {
  api,
  date,
  format,
  operationLabel,
  size,
  type Environment,
  type Version,
  type Spec,
} from "./model";
import { groupHardware, hardwareLabel } from "./hardware";
import { Badge } from "./components";

type Ref = {
  id: string;
  name: string;
  status: string;
  result_state: string;
  created_at: string;
  version: Version;
  environment_id: string;
  environment_name: string;
  spec: Spec;
  metrics: {
    key: string;
    numjobs: number;
    iodepth: number;
    operation: string;
    metric: string;
    unit: string;
    median: number;
    count: number;
  }[];
};
type Group = {
  id: string;
  name: string;
  count: number;
  conditions: number;
  latest: Ref;
  latest_success: Ref | null;
};
type Page = {
  mode: string;
  total: number;
  run_count: number;
  succeeded: number;
  active: number;
  saved: number;
  limit: number;
  offset: number;
  groups: Group[];
  runs: Ref[];
};
const defaults = {
  operation: "",
  file_size: "",
  block_size: "",
  hw: "",
  site: "",
  snapshot: "",
  version: "",
  kind: "data",
  status: "",
  q: "",
  from: "",
  to: "",
  saved: "false",
  mode: "overview",
  project: "",
  condition: "",
  offset: "0",
};
type Filters = typeof defaults;
function readFilters(): Filters {
  const q = new URLSearchParams(location.search);
  const f = { ...defaults };
  for (const k of Object.keys(f) as (keyof Filters)[])
    f[k] = q.get("catalog_" + k) ?? f[k];
  if (!q.has("catalog_snapshot")) f.snapshot = q.get("environment") || "";
  if (!q.has("catalog_version")) f.version = q.get("version") || "";
  if (!q.has("catalog_saved"))
    f.saved = q.get("view") === "saved" ? "true" : "false";
  if (!q.has("catalog_kind")) f.kind = q.get("kind") || "data";
  if (!q.has("catalog_from")) f.from = q.get("from") || "";
  if (!q.has("catalog_to")) f.to = q.get("to") || "";
  if (!["overview", "conditions", "history"].includes(f.mode))
    f.mode = "overview";
  for (const key of ["operation", "file_size", "block_size"] as const)
    if (!q.has("catalog_" + key)) f[key] = q.get(key) || "";
  if (!/^\d+$/.test(f.offset)) f.offset = "0";
  return f;
}
function siteKey(e: Environment) {
  return e.config.cluster
    ? JSON.stringify([e.config.cluster, e.region])
    : "snapshot:" + e.id;
}
function siteLabel(e: Environment) {
  return e.config.cluster
    ? `${e.config.cluster} · ${e.region || "区域未登记"}`
    : `${e.name}（集群未登记，按快照隔离）`;
}
function workload(r: Ref) {
  const w = r.spec.workload;
  return r.spec.kind === "data"
    ? `${operationLabel[w.operation] || w.operation} · 文件 ${size(w.file_size_bytes)} · 块 ${size(w.block_size_bytes)}`
    : r.spec.kind === "scenario"
      ? "混合场景"
      : "元数据";
}
function Metric({
  r,
  onOpen,
}: {
  r: Ref | null;
  onOpen: (id: string) => void;
}) {
  if (!r) return <span className="muted">暂无完整成功结果</span>;
  const metrics = r.metrics.filter(
    (m) =>
      m.metric === "throughput" ||
      m.metric === "metadata_rate" ||
      m.metric === "operation_rate",
  );
  return (
    <button className="metric-link" onClick={() => onOpen(r.id)}>
      {metrics.length === 1 ? (
        <>
          <strong>{format(metrics[0].median, metrics[0].unit)}</strong>
          <small>
            {metrics[0].numjobs} jobs × {metrics[0].iodepth} depth ·{" "}
            {metrics[0].operation}
          </small>
        </>
      ) : (
        <strong>{r.metrics.length} 项指标 · 查看详情</strong>
      )}
      <small>
        {r.version.label} · {date(r.created_at)}
      </small>
    </button>
  );
}
export function PerformanceCatalog({
  environments,
  versions,
  tick,
  onOpen,
  onCompare,
}: {
  environments: Environment[];
  versions: Version[];
  tick: number;
  onOpen: (id: string) => void;
  onCompare: (a: string, b: string, kind: string) => void;
}) {
  const [f, setF] = useState(readFilters),
    [data, setData] = useState<Page | null>(null),
    [loading, setLoading] = useState(true),
    [error, setError] = useState(""),
    [selection, setSelection] = useState<Ref[]>([]),
    [poll, setPoll] = useState(0);
  const groups = useMemo(() => groupHardware(environments), [environments]);
  const scope = f.hw
    ? groups.find((g) => g.key === f.hw)?.snapshots || []
    : environments;
  const sites = [...new Map(scope.map((e) => [siteKey(e), e])).entries()];
  const scoped = scope.filter((e) => !f.site || siteKey(e) === f.site);
  const snapshots = scoped.filter((e) => !f.snapshot || e.id === f.snapshot);
  function change(p: Partial<Filters>, reset = true) {
    if (reset) setSelection([]);
    setF((old) => {
      const next = { ...old, ...(reset ? { offset: "0" } : {}), ...p };
      const q = new URLSearchParams(location.search);
      for (const [k, v] of Object.entries(next)) q.set("catalog_" + k, v);
      history.pushState(null, "", `?${q}`);
      return next;
    });
  }
  useEffect(() => {
    const restore = () => {
      setF(readFilters());
      setSelection([]);
    };
    window.addEventListener("popstate", restore);
    return () => window.removeEventListener("popstate", restore);
  }, []);
  const query = new URLSearchParams({
    mode: f.mode,
    limit: "20",
    offset: f.offset,
    kind: f.kind,
    version: f.version,
    status: f.status,
    search: f.q,
    project: f.project,
    condition: f.condition,
    saved: f.saved,
  });
  const hasScope = Boolean(f.hw || f.site || f.snapshot);
  if (hasScope) for (const e of snapshots) query.append("environment", e.id);
  if (f.kind === "data") {
    for (const key of ["operation", "file_size", "block_size"] as const)
      if (f[key]) query.set(key, f[key]);
  }
  if (f.from)
    query.set(
      "from",
      Number.isNaN(Date.parse(f.from + "T00:00:00"))
        ? f.from
        : new Date(f.from + "T00:00:00").toISOString(),
    );
  if (f.to)
    query.set(
      "to",
      Number.isNaN(Date.parse(f.to + "T23:59:59.999"))
        ? f.to
        : new Date(f.to + "T23:59:59.999").toISOString(),
    );
  const queryString = query.toString(),
    emptyScope = hasScope && snapshots.length === 0;
  useEffect(() => {
    let live = true;
    setLoading(true);
    setError("");
    setData(null);
    const timer = setTimeout(() => {
      if (emptyScope) {
        if (live) {
          setLoading(false);
          setError("该硬件范围没有匹配快照，请调整范围。");
        }
        return;
      }
      api<Page>("/run-catalog?" + queryString)
        .then((v) => {
          if (live) {
            setData(v);
            if (v.total > 0 && v.offset >= v.total)
              change(
                { offset: String(Math.floor((v.total - 1) / 20) * 20) },
                false,
              );
          }
        })
        .catch((e) => {
          if (live) setError(e.message);
        })
        .finally(() => {
          if (live) setLoading(false);
        });
    }, 200);
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [queryString, emptyScope, tick, poll]);
  useEffect(() => {
    if (!data?.active) return;
    const timer = setInterval(() => setPoll((n) => n + 1), 5000);
    return () => clearInterval(timer);
  }, [data?.active]);
  function toggle(r: Ref) {
    setSelection((old) =>
      old.some((v) => v.id === r.id)
        ? old.filter((v) => v.id !== r.id)
        : old.length < 2
          ? [...old, r]
          : old,
    );
  }
  return (
    <section className="performance-catalog">
      <div className="panel catalog-scope">
        <label>
          硬件范围
          <select
            aria-label="硬件范围"
            value={f.hw}
            onChange={(e) =>
              change({
                hw: e.target.value,
                site: "",
                snapshot: "",
                project: "",
                condition: "",
                mode: "overview",
              })
            }
          >
            <option value="">全部硬件</option>
            {groups.map((g) => (
              <option key={g.key} value={g.key}>
                {hardwareLabel(g)}
              </option>
            ))}
          </select>
        </label>
        <label>
          实际环境
          <select
            aria-label="实际环境"
            value={f.site}
            onChange={(e) =>
              change({
                site: e.target.value,
                snapshot: "",
                project: "",
                condition: "",
                mode: "overview",
              })
            }
          >
            <option value="">全部环境 / 区域</option>
            {sites.map(([id, e]) => (
              <option key={id} value={id}>
                {siteLabel(e)}
              </option>
            ))}
          </select>
        </label>
        <label>
          软件版本
          <select
            aria-label="概览软件版本"
            value={f.version}
            onChange={(e) => change({ version: e.target.value })}
          >
            <option value="">全部版本</option>
            {versions.map((v) => (
              <option key={v.id} value={v.id}>
                {v.label}
              </option>
            ))}
          </select>
        </label>
      </div>
      <div className="catalog-toolbar">
        <div className="tabs">
          <button
            className={f.mode !== "history" ? "selected" : ""}
            onClick={() =>
              change({
                mode: f.project ? "conditions" : "overview",
                condition: "",
              })
            }
          >
            测试概览
          </button>
          <button
            className={f.mode === "history" ? "selected" : ""}
            onClick={() => change({ mode: "history" })}
          >
            运行历史
          </button>
        </div>
        <select
          aria-label="记录范围"
          value={f.saved}
          onChange={(e) => change({ saved: e.target.value })}
        >
          <option value="false">全部记录</option>
          <option value="true">已设基线</option>
        </select>
      </div>
      <div className="catalog-controls">
        <select
          aria-label="负载分类"
          value={f.kind}
          onChange={(e) =>
            change({
              kind: e.target.value,
              project: "",
              condition: "",
              mode: f.mode === "history" ? "history" : "overview",
            })
          }
        >
          <option value="data">数据 I/O</option>
          <option value="metadata">元数据</option>
          <option value="scenario">混合场景</option>
        </select>
        <select
          aria-label="运行状态"
          value={f.status}
          onChange={(e) => change({ status: e.target.value })}
        >
          <option value="">全部状态</option>
          {[
            ["pending", "排队"],
            ["preparing", "准备中"],
            ["running", "运行中"],
            ["succeeded", "成功"],
            ["failed", "失败"],
            ["cancelled", "已取消"],
            ["interrupted", "已中断"],
          ].map(([id, label]) => (
            <option key={id} value={id}>
              {label}
            </option>
          ))}
        </select>
        <input
          aria-label="搜索测试记录"
          placeholder="搜索用例、版本或 Run ID"
          value={f.q}
          onChange={(e) => change({ q: e.target.value })}
        />
        <details>
          <summary>
            更多筛选
            {f.snapshot ||
            f.from ||
            f.to ||
            (f.kind === "data" && (f.operation || f.file_size || f.block_size))
              ? " · 已启用"
              : ""}
          </summary>
          <div className="catalog-extra">
            {f.kind === "data" && (
              <>
                <label>
                  读写类型
                  <select
                    aria-label="概览读写类型"
                    value={f.operation}
                    onChange={(e) => change({ operation: e.target.value })}
                  >
                    <option value="">全部类型</option>
                    {Object.entries(operationLabel).map(([value, label]) => (
                      <option key={value} value={value}>
                        {label}
                      </option>
                    ))}
                  </select>
                </label>
                {(
                  [
                    ["file_size", "文件大小"],
                    ["block_size", "I/O 块大小"],
                  ] as const
                ).map(([key, label]) => (
                  <label key={key}>
                    {label}（字节）
                    <input
                      type="number"
                      min="1"
                      step="1"
                      aria-label={label + "（字节）"}
                      placeholder="不限"
                      value={f[key]}
                      onChange={(e) => change({ [key]: e.target.value })}
                    />
                    {Number(f[key]) > 0 && (
                      <small>{size(Number(f[key]))}</small>
                    )}
                  </label>
                ))}
              </>
            )}

            <label>
              执行快照
              <select
                aria-label="概览执行快照"
                value={f.snapshot}
                onChange={(e) => change({ snapshot: e.target.value })}
              >
                <option value="">全部快照</option>
                {scoped.map((e) => (
                  <option key={e.id} value={e.id}>
                    {e.name}
                  </option>
                ))}
              </select>
            </label>
            <label>
              起始日期
              <input
                type="date"
                value={f.from}
                onChange={(e) => change({ from: e.target.value })}
              />
            </label>
            <label>
              结束日期
              <input
                type="date"
                value={f.to}
                onChange={(e) => change({ to: e.target.value })}
              />
            </label>
          </div>
        </details>
      </div>
      {f.project && (
        <div className="catalog-context">
          <button
            className="secondary"
            onClick={() =>
              change({ project: "", condition: "", mode: "overview" })
            }
          >
            ← 全部测试项目
          </button>
          <span>
            {f.condition
              ? "当前仅展示所选测试条件的历史"
              : "当前仅展示所选测试项目"}
          </span>
        </div>
      )}
      <div className="workspace-summary" aria-live="polite">
        {loading
          ? "正在查询完整历史…"
          : data && (
              <>
                <span>
                  <b>{data.run_count}</b> 匹配运行
                </span>
                <span>
                  <b>{data.succeeded}</b> 完整成功
                </span>
                <span>
                  <b>{data.active}</b> 进行中
                </span>
                <span>
                  <b>{data.saved}</b> 基线运行
                </span>
              </>
            )}
      </div>
      {error && (
        <div className="alert error" role="alert">
          {error}
          <button onClick={() => setPoll((n) => n + 1)}>重试</button>
        </div>
      )}
      {selection.length > 0 && (
        <div className="catalog-selection">
          <span>已选 {selection.length}/2 · 第一条作为参照</span>
          <button
            className="primary"
            disabled={selection.length !== 2 || loading}
            onClick={() => onCompare(selection[0].id, selection[1].id, f.kind)}
          >
            对比所选
          </button>
          <button onClick={() => setSelection([])}>清空</button>
        </div>
      )}
      {data && !loading && (
        <>
          <div className="panel table-scroll">
            <table className="catalog-table">
              <thead>
                <tr>
                  {f.mode === "history" ? (
                    <>
                      <th>选择</th>
                      <th>运行 / 时间</th>
                      <th>版本 / 环境</th>
                      <th>结果</th>
                      <th>状态</th>
                      <th>操作</th>
                    </>
                  ) : (
                    <>
                      <th>
                        {f.mode === "conditions" ? "测试条件" : "测试项目"}
                      </th>
                      <th>最近成功结果</th>
                      <th>最近运行</th>
                      <th>历史</th>
                    </>
                  )}
                </tr>
              </thead>
              <tbody>
                {f.mode === "history"
                  ? data.runs.map((r) => (
                      <tr key={r.id}>
                        <td>
                          <input
                            type="checkbox"
                            aria-label={`选择 ${r.id}`}
                            checked={selection.some((v) => v.id === r.id)}
                            disabled={
                              r.status !== "succeeded" ||
                              r.result_state !== "complete" ||
                              (selection.length === 2 &&
                                !selection.some((v) => v.id === r.id))
                            }
                            onChange={() => toggle(r)}
                          />
                        </td>
                        <td>
                          <strong>{r.name}</strong>
                          <small>{date(r.created_at)}</small>
                          <small>{r.id}</small>
                        </td>
                        <td>
                          {r.version.label}
                          <small>{r.environment_name}</small>
                        </td>
                        <td>
                          <Metric
                            r={
                              r.status === "succeeded" &&
                              r.result_state === "complete"
                                ? r
                                : null
                            }
                            onOpen={onOpen}
                          />
                        </td>
                        <td>
                          <Badge status={r.status} />
                          {r.status === "succeeded" &&
                            r.result_state !== "complete" && (
                              <small>证据未完整回收</small>
                            )}
                        </td>
                        <td>
                          <button
                            className="secondary"
                            onClick={() => onOpen(r.id)}
                          >
                            查看
                          </button>
                        </td>
                      </tr>
                    ))
                  : data.groups.map((g) => (
                      <tr key={g.id}>
                        <td>
                          <strong>{g.name}</strong>
                          <small>{workload(g.latest)}</small>
                          {f.mode === "overview" ? (
                            <button
                              className="text-action"
                              onClick={() =>
                                change({
                                  project: g.id,
                                  condition: "",
                                  mode: "conditions",
                                })
                              }
                            >
                              {g.conditions} 套测试条件 →
                            </button>
                          ) : (
                            <>
                              <small>{g.latest.environment_name}</small>
                              <small>
                                {g.latest.spec.kind === "data"
                                  ? `${g.latest.spec.matrix.numjobs.join("/")} jobs × ${g.latest.spec.matrix.iodepth.join("/")} depth · ${g.latest.spec.workload.cache_state} · ${g.latest.spec.workload.direct ? "直接" : "缓冲"} I/O`
                                  : "独立负载与环境快照"}
                              </small>
                              <button
                                className="text-action"
                                onClick={() => onOpen(g.latest.id)}
                              >
                                查看完整条件 →
                              </button>
                            </>
                          )}
                        </td>
                        <td>
                          {g.conditions > 1 ? (
                            <span className="muted">多套条件，请展开查看</span>
                          ) : (
                            <Metric r={g.latest_success} onOpen={onOpen} />
                          )}
                        </td>
                        <td>
                          <Badge status={g.latest.status} />
                          <small>{g.latest.version.label}</small>
                          <small>{date(g.latest.created_at)}</small>
                        </td>
                        <td>
                          <button
                            className="secondary"
                            onClick={() =>
                              change({
                                mode: "history",
                                project:
                                  f.mode === "overview" ? g.id : f.project,
                                condition: f.mode === "conditions" ? g.id : "",
                              })
                            }
                          >
                            {g.count} 次 →
                          </button>
                        </td>
                      </tr>
                    ))}
                {data.total === 0 && (
                  <tr>
                    <td
                      colSpan={f.mode === "history" ? 6 : 4}
                      className="catalog-empty"
                    >
                      暂无匹配记录，请调整硬件范围或筛选条件。
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
          <div className="catalog-pagination">
            <span>
              共 {data.total}{" "}
              {f.mode === "history"
                ? "次运行"
                : f.mode === "conditions"
                  ? "套条件"
                  : "个项目"}{" "}
              · 每页 20 条 · 第 {Math.floor(data.offset / 20) + 1}/
              {Math.max(1, Math.ceil(data.total / 20))} 页
            </span>
            <button
              className="secondary"
              disabled={data.offset === 0}
              onClick={() =>
                change({ offset: String(Math.max(0, data.offset - 20)) }, false)
              }
            >
              上一页
            </button>
            <button
              className="secondary"
              disabled={data.offset + 20 >= data.total}
              onClick={() =>
                change({ offset: String(data.offset + 20) }, false)
              }
            >
              下一页
            </button>
          </div>
        </>
      )}
      <p className="catalog-note">
        统计覆盖当前筛选下的完整历史。指标来自最近完整成功运行的重复中位数；多并发/混合指标分别查看。硬件归组仅用于浏览，条件与可比性仍以原始记录为准。未声明集群身份的快照单独列出。
      </p>
    </section>
  );
}
