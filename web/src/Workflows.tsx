import { useEffect, useState, type ReactNode } from "react";
import { api, date } from "./model";
import { Dialog, Empty } from "./components";
import example from "./workflow-example.json";
import "./workflow.css";
import { WorkflowIssues } from "./WorkflowIssues";

type Ref = { id: string; revision: number };
type RecordValue = {
  id: string;
  revision: number;
  name: string;
  description: string;
  change_note: string;
  created_at: string;
  body: Record<string, any>;
};
type Check = { name: string; status: string; detail: string };
type Preflight = {
  id: string;
  ready: boolean;
  expires_at: string;
  checks: Check[];
  snapshot: {
    hash: string;
    ep: unknown;
    inputs: unknown;
    prepared_bytes: number;
  };
};
type WorkflowRun = {
  id: string;
  request_id: string;
  external_run_id: string;
  status: string;
  execution_state: string;
  result_state: string;
  verdict: string;
  error?: string;
  created_at: string;
  deadline: string;
  cancel_confirmed: boolean;
  steps: { name: string; state: string; detail: string }[];
  snapshot: {
    case: RecordValue;
    system: RecordValue;
    profile: RecordValue;
    version: { id: string; label: string };
    environment: { id: string };
    ep: unknown;
    inputs: unknown;
  };
  observations?: unknown;
  results: {
    node_id: string;
    attempt: number;
    parser: string;
    state: string;
    verdict: string;
    error?: string;
    metrics: { name: string; value: number; unit: string }[];
    assertions: Record<string, boolean>;
    files: { path: string; sha256: string; size: number }[];
  }[];
};
const pretty = (v: unknown) => JSON.stringify(v, null, 2);
const labels: Record<string, string> = {
  pending: "待处理",
  preparing: "准备中",
  submitting: "提交中",
  running: "运行中",
  collecting: "回收中",
  succeeded: "执行成功",
  failed: "失败",
  cancelled: "已取消",
  cancelling: "取消中",
  passed: "通过",
  inconclusive: "无法判定",
  complete: "完整",
  missing: "缺失",
  invalid: "无效",
  skipped: "跳过",
};
const label = (s: string) => labels[s] || s;
const ref = (r: RecordValue): Ref => ({ id: r.id, revision: r.revision });
function download(name: string, v: unknown) {
  const u = URL.createObjectURL(
    new Blob([pretty(v)], { type: "application/json" }),
  );
  const a = document.createElement("a");
  a.href = u;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(u), 1000);
}
export function WorkflowSection({
  mode,
  children,
}: {
  mode: "cases" | "runs" | "settings";
  children: ReactNode;
}) {
  const [active, setActive] = useState(
    new URLSearchParams(location.search).get("workflow") === "1",
  );
  return (
    <>
      <div className="wf-tabs" role="tablist" aria-label="测试类型">
        <button
          role="tab"
          aria-selected={!active}
          onClick={() => setActive(false)}
        >
          {mode === "settings" ? "硬件环境" : "单项 / 混合负载"}
        </button>
        <button
          role="tab"
          aria-selected={active}
          onClick={() => setActive(true)}
        >
          {mode === "settings" ? "被测系统与执行配置" : "工作流测试 · UCE"}
        </button>
      </div>
      {active ? <Workflows mode={mode} /> : children}
    </>
  );
}
export function Workflows({ mode }: { mode: "cases" | "runs" | "settings" }) {
  const [records, setRecords] = useState<RecordValue[]>([]),
    [kind, setKind] = useState(mode === "settings" ? "systems" : "cases"),
    [error, setError] = useState(""),
    [tick, setTick] = useState(0),
    [editing, setEditing] = useState<RecordValue | null | undefined>(undefined),
    [selected, setSelected] = useState<RecordValue | null>(null),
    [history, setHistory] = useState<RecordValue[]>([]),
    [wizard, setWizard] = useState<RecordValue | null | undefined>(undefined);
  const [runs, setRuns] = useState<WorkflowRun[]>([]),
    [offset, setOffset] = useState(0),
    [run, setRun] = useState<WorkflowRun | null>(null),
    [baseline, setBaseline] = useState(""),
    [comparison, setComparison] = useState<Comparison | null>(null);
  useEffect(() => {
    const id = new URLSearchParams(location.search).get("workflow_run");
    if (!id) return;
    let live = true;
    api<WorkflowRun>(`/workflow/runs/${encodeURIComponent(id)}`)
      .then((r) => {
        if (live) setRun(r);
      })
      .catch((e) => {
        if (live) setError(e.message);
      });
    return () => {
      live = false;
    };
  }, []);
  useEffect(() => {
    let live = true;
    setError("");
    const p =
      mode === "runs"
        ? api<WorkflowRun[]>(`/workflow/runs?limit=20&offset=${offset}`).then(
            (v) => {
              if (live) setRuns(v);
            },
          )
        : api<RecordValue[]>(`/workflow/records/${kind}`).then((v) => {
            if (live) setRecords(v);
          });
    p.catch((e) => {
      if (live) setError(e.message);
    });
    return () => {
      live = false;
    };
  }, [mode, kind, tick, offset]);
  useEffect(() => {
    if (mode !== "runs") return;
    const t = setInterval(() => setTick((x) => x + 1), 5000);
    return () => clearInterval(t);
  }, [mode]);
  useEffect(() => {
    if (!run) return;
    let live = true;
    const load = () =>
      api<WorkflowRun>(`/workflow/runs/${run.id}`)
        .then((v) => {
          if (live) setRun(v);
        })
        .catch((e) => {
          if (live) setError(e.message);
        });
    const t = setInterval(load, 4000);
    return () => {
      live = false;
      clearInterval(t);
    };
  }, [run?.id]);
  async function detail(r: RecordValue) {
    setSelected(r);
    try {
      setHistory(
        await api<RecordValue[]>(`/workflow/records/${kind}?id=${r.id}`),
      );
    } catch (e) {
      setError((e as Error).message);
    }
  }
  async function action(verb: string) {
    if (!run) return;
    try {
      setRun(await api<WorkflowRun>(`/workflow/runs/${run.id}/${verb}`, {}));
    } catch (e) {
      setError((e as Error).message);
    }
  }
  return (
    <section className="wf-workspace">
      <div className="wf-heading">
        <div>
          <span className="eyebrow">WORKFLOW / UCE</span>
          <h2>
            {mode === "cases"
              ? "可复用的工作流用例"
              : mode === "runs"
                ? "工作流测试"
                : "执行环境管理"}
          </h2>
          <p>
            {mode === "cases"
              ? "完整 EP、参数和验收标准随版本保存。"
              : mode === "runs"
                ? "分别查看执行状态、证据完整性和验证结论。"
                : "引用已登记的硬件与软件版本；连接凭据由服务端管理。"}
          </p>
        </div>
        <button
          className="primary"
          onClick={() => (mode === "runs" ? setWizard(null) : setEditing(null))}
        >
          {mode === "runs"
            ? "发起工作流测试"
            : kind === "cases"
              ? "新建工作流用例"
              : "新建配置"}
        </button>
      </div>
      {mode === "settings" && (
        <div className="wf-tabs">
          <button
            aria-pressed={kind === "systems"}
            onClick={() => {
              setKind("systems");
              setSelected(null);
            }}
          >
            被测系统
          </button>
          <button
            aria-pressed={kind === "profiles"}
            onClick={() => {
              setKind("profiles");
              setSelected(null);
            }}
          >
            执行配置
          </button>
        </div>
      )}
      {error && (
        <div className="alert error" role="alert">
          {error}
        </div>
      )}
      {mode !== "runs" && !selected && (
        <div className="wf-grid">
          {records.map((r) => (
            <article className="wf-card" key={r.id}>
              <span className="wf-tag">
                {kind === "cases"
                  ? "UCE EP"
                  : kind === "systems"
                    ? "被测系统"
                    : "执行配置"}{" "}
                · v{r.revision}
              </span>
              <h3>{r.name}</h3>
              <p>{r.description || "暂无描述"}</p>
              <small>{date(r.created_at)}</small>
              <div className="wf-actions">
                <button onClick={() => detail(r)}>查看详情</button>
                {kind === "cases" && (
                  <button onClick={() => setWizard(r)}>使用此版本</button>
                )}
              </div>
            </article>
          ))}
          {records.length === 0 && (
            <Empty
              title="还没有工作流配置"
              text="从完整 EP 示例开始，保存为第一个版本。"
            />
          )}
        </div>
      )}
      {selected && (
        <article className="wf-card">
          <div className="wf-actions">
            <button onClick={() => setSelected(null)}>返回列表</button>
            <select
              aria-label="工作流用例版本"
              value={selected.revision}
              onChange={(e) =>
                setSelected(
                  history.find((v) => v.revision === Number(e.target.value)) ||
                    selected,
                )
              }
            >
              {history.map((r) => (
                <option key={r.revision} value={r.revision}>
                  v{r.revision} · {r.change_note || date(r.created_at)}
                </option>
              ))}
            </select>
            <button onClick={() => setEditing(history[0] || selected)}>
              编辑为新版本
            </button>
            <button
              onClick={() =>
                download(`${selected.id}-v${selected.revision}.json`, selected)
              }
            >
              导出定义
            </button>
            {kind === "cases" && (
              <button className="primary" onClick={() => setWizard(selected)}>
                使用此版本测试
              </button>
            )}
          </div>
          <h3>{selected.name}</h3>
          <p>{selected.description}</p>
          {kind === "cases" && <DAG template={selected.body.template} />}
          <details>
            <summary>完整定义、模板与结果契约</summary>
            <pre>{pretty(selected.body)}</pre>
          </details>
        </article>
      )}
      {mode === "runs" && (
        <>
          <div className="wf-table-wrap">
            <table className="wf-table">
              <thead>
                <tr>
                  <th>测试 / 环境</th>
                  <th>时间</th>
                  <th>执行</th>
                  <th>证据</th>
                  <th>验证</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {runs.map((r) => (
                  <tr key={r.id}>
                    <td>
                      <strong>{r.snapshot.case.name}</strong>
                      <small>
                        {r.snapshot.system.name} · {r.snapshot.version.label}
                      </small>
                    </td>
                    <td>{date(r.created_at)}</td>
                    <td>
                      {label(
                        r.execution_state === "not_submitted"
                          ? r.status
                          : r.execution_state || r.status,
                      )}
                    </td>
                    <td>{label(r.result_state)}</td>
                    <td>{label(r.verdict)}</td>
                    <td>
                      <button onClick={() => setRun(r)}>详情</button>
                      {r.verdict === "passed" &&
                        r.result_state === "complete" && (
                          <button
                            onClick={() => {
                              if (!baseline) setBaseline(r.id);
                              else
                                api<Comparison>("/workflow/compare", {
                                  baseline,
                                  candidate: r.id,
                                })
                                  .then(setComparison)
                                  .catch((e) => setError(e.message));
                            }}
                          >
                            {baseline === r.id
                              ? "已选参照"
                              : baseline
                                ? "与参照对比"
                                : "选作对比参照"}
                          </button>
                        )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {runs.length === 0 && <Empty title="还没有工作流运行" />}
          <div className="wf-actions">
            <button
              disabled={!offset}
              onClick={() => setOffset(Math.max(0, offset - 20))}
            >
              上一页
            </button>
            <span>第 {offset / 20 + 1} 页</span>
            <button
              disabled={runs.length < 20}
              onClick={() => setOffset(offset + 20)}
            >
              下一页
            </button>
            {baseline && (
              <button
                onClick={() => {
                  setBaseline("");
                  setComparison(null);
                }}
              >
                清除参照
              </button>
            )}
          </div>
          {comparison !== null && <ComparisonView value={comparison} />}
        </>
      )}
      {editing !== undefined && (
        <RecordEditor
          kind={kind}
          initial={editing}
          onClose={() => setEditing(undefined)}
          onSaved={(r) => {
            setEditing(undefined);
            setSelected(r);
            detail(r);
            setTick((x) => x + 1);
          }}
        />
      )}
      {wizard !== undefined && (
        <Wizard
          initial={wizard}
          onClose={() => setWizard(undefined)}
          onStarted={(r) => {
            setWizard(undefined);
            setRun(r);
            setTick((x) => x + 1);
          }}
        />
      )}
      {run && (
        <Dialog title={run.snapshot.case.name} onClose={() => setRun(null)}>
          <div className="wf-detail">
            <p>
              {run.snapshot.system.name} · {run.snapshot.version.label}
            </p>
            <div className="wf-facts">
              <span>
                执行：
                {label(
                  run.execution_state === "not_submitted"
                    ? run.status
                    : run.execution_state || run.status,
                )}
              </span>
              <span>证据：{label(run.result_state)}</span>
              <span>验证：{label(run.verdict)}</span>
            </div>
            <ol className="wf-progress">
              {run.steps.map((s) => (
                <li key={s.name} data-state={s.state}>
                  <strong>{s.name}</strong>
                  <span>{label(s.state)}</span>
                  <small>{s.detail}</small>
                </li>
              ))}
            </ol>
            {run.error && <div className="alert error">{run.error}</div>}
            <DAG template={run.snapshot.ep} />
            <WorkflowIssues key={run.id} run={run} />
            {run.results.map((r, i) => (
              <article className="wf-card" key={i}>
                <h4>
                  {r.node_id} · {r.parser}
                </h4>
                <p>
                  {label(r.state)} / {label(r.verdict)} · attempt {r.attempt}
                </p>
                {r.error && <p className="alert error">{r.error}</p>}
                <ul>
                  {r.metrics.map((m) => (
                    <li key={m.name}>
                      {m.name}：{m.value} {m.unit}
                    </li>
                  ))}
                </ul>
                <dl>
                  {Object.entries(r.assertions || {}).map(([k, v]) => (
                    <div key={k}>
                      {k}：{v ? "通过" : "失败"}
                    </div>
                  ))}
                </dl>
                {r.files.map((f) => (
                  <button
                    key={f.path}
                    onClick={() =>
                      downloadArtifact(run.id, f.path).catch((e) =>
                        setError(e.message),
                      )
                    }
                  >
                    {f.path.slice(-45)} · {f.size} B
                  </button>
                ))}
              </article>
            ))}
            <details>
              <summary>运行身份、快照与原始状态</summary>
              <pre>{pretty(run)}</pre>
            </details>
            <div className="wf-actions">
              <button onClick={() => download(`${run.id}.json`, run)}>
                导出运行记录
              </button>
              {!["succeeded", "failed", "cancelled"].includes(run.status) && (
                <button onClick={() => action("cancel")}>取消并等待确认</button>
              )}
              {run.result_state === "missing" && (
                <button onClick={() => action("collect")}>
                  仅重试证据回收
                </button>
              )}
            </div>
          </div>
        </Dialog>
      )}
    </section>
  );
}
async function downloadArtifact(id: string, name: string) {
  const token = sessionStorage.getItem("malta-token") || "";
  const r = await fetch(
    `/api/v1/workflow/runs/${id}/artifacts/${encodeURIComponent(name)}`,
    { headers: token ? { Authorization: `Bearer ${token}` } : {} },
  );
  if (!r.ok) throw new Error("证据下载失败");
  const u = URL.createObjectURL(await r.blob());
  const a = document.createElement("a");
  a.href = u;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(u), 1000);
}
function DAG({ template }: { template: any }) {
  const nodes = template?.dag_definitions?.[0]?.nodes || [];
  const names = new Map(nodes.map((n: any) => [n.definition_node_id, n.name]));
  return (
    <div className="wf-dag" aria-label="工作流依赖">
      {nodes.map((n: any) => (
        <div className="wf-node" key={n.definition_node_id}>
          <small>
            {(n.input_path || [])
              .map((p: any) => names.get(p.from_node) || p.from_node)
              .join(" + ") || "起点"}{" "}
            →
          </small>
          <strong>{n.name}</strong>
        </div>
      ))}
    </div>
  );
}
function RecordEditor({
  kind,
  initial,
  onClose,
  onSaved,
}: {
  kind: string;
  initial: RecordValue | null;
  onClose: () => void;
  onSaved: (v: RecordValue) => void;
}) {
  const defaults =
    kind === "cases"
      ? example.body
      : kind === "systems"
        ? { environment_id: "", software_version_id: "", settings: {} }
        : {
            backend: "uce",
            connection: "",
            system_ids: [],
            cluster: "",
            namespace: "",
            node: "",
            queue: "default",
            subtype: "malta",
            settings: { results_pvc: "", results_mount: "", node: "" },
          };
  const [name, setName] = useState(initial?.name || ""),
    [description, setDescription] = useState(initial?.description || ""),
    [note, setNote] = useState(""),
    [json, setJSON] = useState(pretty(initial?.body || defaults)),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const [references, setReferences] = useState<unknown>(null);
  useEffect(() => {
    let live = true;
    const paths =
      kind === "profiles"
        ? ["/workflow/connections", "/workflow/records/systems"]
        : kind === "systems"
          ? ["/environments", "/versions"]
          : [];
    if (paths.length)
      Promise.all(paths.map((p) => api(p)))
        .then((values) => {
          if (live)
            setReferences(
              Object.fromEntries(paths.map((p, i) => [p, values[i]])),
            );
        })
        .catch((e) => {
          if (live) setError(e.message);
        });
    return () => {
      live = false;
    };
  }, [kind]);
  async function save() {
    setBusy(true);
    setError("");
    try {
      const v = await api<RecordValue>(`/workflow/records/${kind}`, {
        id: initial?.id,
        base_revision: initial?.revision || 0,
        name,
        description,
        change_note: note,
        body: JSON.parse(json),
      });
      onSaved(v);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog
      title={initial ? "编辑为新版本" : "新建工作流配置"}
      onClose={onClose}
    >
      <div className="wf-editor">
        {references !== null && (
          <details>
            <summary>查看可引用的已登记配置（ID）</summary>
            <pre className="wf-reference">{pretty(references)}</pre>
          </details>
        )}

        <label>
          名称
          <input value={name} onChange={(e) => setName(e.target.value)} />
        </label>
        <label>
          描述
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </label>
        <label>
          修订说明
          <input value={note} onChange={(e) => setNote(e.target.value)} />
        </label>
        <label>
          导入 JSON
          <input
            type="file"
            accept=".json"
            onChange={async (e) => {
              try {
                const f = e.target.files?.[0];
                if (!f) return;
                const v = JSON.parse(await f.text());
                setJSON(pretty(v.body || v));
                if (v.name) setName(v.name);
                if (v.description) setDescription(v.description);
              } catch (e) {
                setError((e as Error).message);
              }
            }}
          />
        </label>
        <label>
          {kind === "cases"
            ? "完整模板、参数与结果契约"
            : "配置 JSON（不要填写凭据）"}
          <textarea
            className="wf-code"
            spellCheck={false}
            value={json}
            onChange={(e) => setJSON(e.target.value)}
          />
        </label>
        {kind === "cases" && (
          <p>
            示例仅验证算术断言和证据回收，不是性能测试。运行前配置工具镜像、真实路由和结果卷。
          </p>
        )}
        {error && (
          <div className="alert error" role="alert">
            {error}
          </div>
        )}
        <button className="primary" disabled={busy || !name} onClick={save}>
          {busy ? "保存中…" : "保存不可变版本"}
        </button>
      </div>
    </Dialog>
  );
}
function Wizard({
  initial,
  onClose,
  onStarted,
}: {
  initial: RecordValue | null;
  onClose: () => void;
  onStarted: (r: WorkflowRun) => void;
}) {
  const [step, setStep] = useState(0),
    [cases, setCases] = useState<RecordValue[]>([]),
    [systems, setSystems] = useState<RecordValue[]>([]),
    [profiles, setProfiles] = useState<RecordValue[]>([]),
    [versions, setVersions] = useState<{ id: string; label: string }[]>([]),
    [history, setHistory] = useState<RecordValue[]>(initial ? [initial] : []);
  const [chosen, setChosen] = useState<RecordValue | null>(initial),
    [system, setSystem] = useState<RecordValue | null>(null),
    [profile, setProfile] = useState<RecordValue | null>(null),
    [version, setVersion] = useState(""),
    [params, setParams] = useState<Record<string, any>>(
      initial?.body.defaults || {},
    ),
    [check, setCheck] = useState<Preflight | null>(null),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [request, setRequest] = useState(() => `workflow-${crypto.randomUUID()}`);
  useEffect(() => {
    let live = true;
    Promise.all([
      api<RecordValue[]>("/workflow/records/cases"),
      api<RecordValue[]>("/workflow/records/systems"),
      api<RecordValue[]>("/workflow/records/profiles"),
      api<{ id: string; label: string }[]>("/versions"),
    ])
      .then(([c, s, p, v]) => {
        if (live) {
          setCases(c);
          setSystems(s);
          setProfiles(p);
          setVersions(v);
        }
      })
      .catch((e) => {
        if (live) setError(e.message);
      });
    return () => {
      live = false;
    };
  }, []);
  useEffect(() => {
    setCheck(null);
  }, [chosen, system, profile, version, params]);
  useEffect(() => {
    if (!initial) return;
    let live = true;
    api<RecordValue[]>(`/workflow/records/cases?id=${initial.id}`)
      .then((v) => {
        if (live) setHistory(v);
      })
      .catch((e) => {
        if (live) setError(e.message);
      });
    return () => {
      live = false;
    };
  }, [initial]);
  const steps = [
    "用例版本",
    "被测系统",
    "执行配置",
    "软件版本",
    "参数",
    "预检查",
    "提交",
  ];
  const valid = [
    !!chosen,
    !!system,
    !!profile,
    !!version,
    true,
    !!check?.ready,
    true,
  ][step];
  async function preflight() {
    if (!chosen || !system || !profile) return;
    setBusy(true);
    setError("");
    try {
      setCheck(
        await api<Preflight>("/workflow/preflight", {
          case: ref(chosen),
          system: ref(system),
          profile: ref(profile),
          version_id: version,
          parameters: params,
        }),
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function submit() {
    if (!check) return;
    setBusy(true);
    setError("");
    try {
      onStarted(
        await api<WorkflowRun>("/workflow/runs", {
          preflight_id: check.id,
          snapshot_hash: check.snapshot.hash,
          request_id: request,
        }),
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog title="发起工作流测试" onClose={onClose}>
      <div className="wf-wizard">
        <ol className="wf-stepper">
          {steps.map((s, i) => (
            <li key={s} aria-current={i === step ? "step" : undefined}>
              {i + 1}. {s}
            </li>
          ))}
        </ol>
        <h3>{steps[step]}</h3>
        {step === 0 && (
          <>
            <label>
              用例
              <select
                value={chosen?.id || ""}
                onChange={async (e) => {
                  const v = cases.find((c) => c.id === e.target.value) || null;
                  setChosen(v);
                  setParams(v?.body.defaults || {});
                  if (v)
                    try {
                      setHistory(
                        await api<RecordValue[]>(
                          `/workflow/records/cases?id=${v.id}`,
                        ),
                      );
                    } catch (e) {
                      setError((e as Error).message);
                    }
                }}
              >
                <option value="">选择用例</option>
                {cases.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name}
                  </option>
                ))}
              </select>
            </label>
            <label>
              固定版本
              <select
                value={chosen?.revision || ""}
                onChange={(e) => {
                  const v =
                    history.find(
                      (c) => c.revision === Number(e.target.value),
                    ) || null;
                  setChosen(v);
                  setParams(v?.body.defaults || {});
                }}
              >
                {history.map((c) => (
                  <option key={c.revision} value={c.revision}>
                    v{c.revision}
                  </option>
                ))}
              </select>
            </label>
            {chosen && <DAG template={chosen.body.template} />}
          </>
        )}
        {step === 1 && (
          <label>
            被测系统
            <select
              value={system?.id || ""}
              onChange={(e) => {
                const v = systems.find((s) => s.id === e.target.value) || null;
                setSystem(v);
                setProfile(null);
                setVersion(v?.body.software_version_id || "");
              }}
            >
              <option value="">选择系统</option>
              {systems.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name} · v{s.revision}
                </option>
              ))}
            </select>
          </label>
        )}
        {step === 2 && (
          <label>
            兼容执行配置
            <select
              value={profile?.id || ""}
              onChange={(e) =>
                setProfile(
                  profiles.find((p) => p.id === e.target.value) || null,
                )
              }
            >
              <option value="">选择配置</option>
              {profiles
                .filter((p) => p.body.system_ids?.includes(system?.id))
                .map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name} · {p.body.cluster} / {p.body.node}
                  </option>
                ))}
            </select>
          </label>
        )}
        {step === 3 && (
          <>
            <label>
              已登记的软件版本
              <select
                value={version}
                onChange={(e) => setVersion(e.target.value)}
              >
                <option value="">选择版本</option>
                {versions.map((v) => (
                  <option key={v.id} value={v.id}>
                    {v.label} · {v.id}
                  </option>
                ))}
              </select>
            </label>
            <p>应与被测系统配置中的版本一致；此选择不会升级被测集群。</p>
          </>
        )}
        {step === 4 && (
          <>
            <ParameterForm
              schema={chosen?.body.parameters}
              values={params}
              onChange={setParams}
            />
            <p>
              未在参数 Schema 中声明的字段不能提交。malta_context 由服务注入。
            </p>
          </>
        )}
        {step === 5 && (
          <>
            <button disabled={busy} onClick={preflight}>
              {busy ? "检查中…" : "执行预检查"}
            </button>
            {check && (
              <>
                <ul className="wf-checks">
                  {check.checks.map((c) => (
                    <li key={c.name} data-state={c.status}>
                      <strong>
                        {c.name} · {label(c.status)}
                      </strong>
                      <p>{c.detail}</p>
                    </li>
                  ))}
                </ul>
                <p>
                  准备正文：{check.snapshot.prepared_bytes.toLocaleString()}{" "}
                  字节 · 有效至 {date(check.expires_at)}
                </p>
                <details>
                  <summary>查看最终 EP 与参数</summary>
                  <pre>
                    {pretty({
                      ep: check.snapshot.ep,
                      inputs: check.snapshot.inputs,
                    })}
                  </pre>
                </details>
              </>
            )}
          </>
        )}
        {step === 6 && (
          <>
            <p>
              {chosen?.name} v{chosen?.revision} → {system?.name} →{" "}
              {profile?.name}
            </p>
            <label>
              Request ID
              <input
                value={request}
                onChange={(e) => setRequest(e.target.value)}
              />
            </label>
            <p>网络重试保持此 ID。提交后先注册 EP，再创建 UCE 工作流。</p>
            <button
              className="primary"
              disabled={busy || !check?.ready || !request}
              onClick={submit}
            >
              {busy ? "提交中…" : "提交测试"}
            </button>
          </>
        )}
        {error && (
          <div className="alert error" role="alert">
            {error}
          </div>
        )}
        <div className="wf-actions">
          <button
            disabled={step === 0 || busy}
            onClick={() => setStep((s) => s - 1)}
          >
            上一步
          </button>
          {step < 6 && (
            <button
              className="primary"
              disabled={!valid || busy}
              onClick={() => setStep((s) => s + 1)}
            >
              下一步
            </button>
          )}
        </div>
      </div>
    </Dialog>
  );
}
function ParameterForm({
  schema,
  values,
  onChange,
}: {
  schema: any;
  values: Record<string, any>;
  onChange: (v: Record<string, any>) => void;
}) {
  return (
    <div className="wf-parameters">
      {Object.entries(schema?.properties || {}).map(([key, value]) => {
        const p = value as any;
        return (
          <label key={key}>
            {p.title || key}
            {schema.required?.includes(key) ? " *" : ""}
            {p.enum ? (
              <select
                value={values[key] ?? ""}
                onChange={(e) =>
                  onChange({
                    ...values,
                    [key]: p.enum.find(
                      (x: any) => String(x) === e.target.value,
                    ),
                  })
                }
              >
                <option value="">请选择</option>
                {p.enum.map((x: any) => (
                  <option key={String(x)} value={String(x)}>
                    {String(x)}
                  </option>
                ))}
              </select>
            ) : p.type === "boolean" ? (
              <input
                type="checkbox"
                checked={!!values[key]}
                onChange={(e) =>
                  onChange({ ...values, [key]: e.target.checked })
                }
              />
            ) : p.type === "object" || p.type === "array" ? (
              <JSONParameter
                value={values[key]}
                onChange={(v) => onChange({ ...values, [key]: v })}
              />
            ) : (
              <input
                type={
                  p.type === "integer" || p.type === "number"
                    ? "number"
                    : "text"
                }
                min={p.minimum}
                max={p.maximum}
                step={p.type === "integer" ? 1 : "any"}
                value={values[key] ?? ""}
                onChange={(e) => {
                  const v = { ...values };
                  if (e.target.value === "") delete v[key];
                  else
                    v[key] =
                      p.type === "integer" || p.type === "number"
                        ? Number(e.target.value)
                        : e.target.value;
                  onChange(v);
                }}
              />
            )}
            <small>{p.description}</small>
          </label>
        );
      })}
      {!Object.keys(schema?.properties || {}).length && (
        <p>此用例没有需要填写的业务参数。</p>
      )}
    </div>
  );
}
function JSONParameter({
  value,
  onChange,
}: {
  value: unknown;
  onChange: (v: unknown) => void;
}) {
  const [text, setText] = useState(pretty(value ?? {}));
  const [error, setError] = useState("");
  return (
    <>
      <textarea
        value={text}
        onChange={(e) => {
          setText(e.target.value);
          try {
            onChange(JSON.parse(e.target.value));
            setError("");
          } catch {
            setError("JSON 尚未完成，将阻止使用旧值");
            onChange(null);
          }
        }}
      />
      {error && <small role="alert">{error}</small>}
    </>
  );
}

type Comparison = {
  comparable: boolean;
  reason?: string;
  warning?: string;
  metrics?: {
    metric: string;
    baseline: number;
    candidate: number;
    delta: number;
    percent?: number;
  }[];
};
function ComparisonView({ value }: { value: Comparison }) {
  return (
    <section className="wf-card">
      <h3>{value.comparable ? "同条件结果对比" : "测试条件不可比"}</h3>
      <p>{value.reason || value.warning}</p>
      {value.comparable && (
        <div className="wf-table-wrap">
          <table className="wf-table">
            <thead>
              <tr>
                <th>节点 / 指标 / 单位</th>
                <th>参照</th>
                <th>候选</th>
                <th>变化</th>
              </tr>
            </thead>
            <tbody>
              {value.metrics?.map((v) => (
                <tr key={v.metric}>
                  <td>{v.metric}</td>
                  <td>{v.baseline.toLocaleString()}</td>
                  <td>{v.candidate.toLocaleString()}</td>
                  <td>
                    {v.percent === undefined ? "—" : `${v.percent.toFixed(2)}%`}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
