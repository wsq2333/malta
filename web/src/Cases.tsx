import { useEffect, useRef, useState, type FormEvent } from "react";
import { BookOpen, Plus, Play, Pencil, Download, Search } from "lucide-react";
import { api, date, type Plan, type Run } from "./model";
import { Dialog, Empty } from "./components";
export interface CaseVersion {
  case_id: string;
  revision: number;
  name: string;
  description: string;
  change_note: string;
  created_at: string;
  plan: Plan;
}
interface TestCase {
  id: string;
  created_at: string;
  updated_at: string;
  latest: CaseVersion;
}
const pretty = (v: unknown) => JSON.stringify(v, null, 2);
const kindLabel = (kind: string) =>
  ({ data: "数据 I/O", metadata: "元数据", scenario: "混合负载" })[kind] ||
  kind;
function exportPlan(v: CaseVersion) {
  const url = URL.createObjectURL(
    new Blob([pretty(v.plan)], { type: "application/json" }),
  );
  const a = document.createElement("a");
  a.href = url;
  a.download = `${v.case_id}-v${v.revision}.json`;
  a.click();
  URL.revokeObjectURL(url);
}
export function CasePicker({
  onSelect,
  initial,
}: {
  initial?: CaseVersion | null;
  onSelect: (v: CaseVersion | null) => void;
}) {
  const [cases, setCases] = useState<TestCase[]>([]),
    [id, setID] = useState(initial?.case_id || ""),
    [versions, setVersions] = useState<CaseVersion[]>([]),
    [revision, setRevision] = useState(initial ? String(initial.revision) : ""),
    [error, setError] = useState("");
  useEffect(() => {
    let live = true;
    api<TestCase[]>("/cases")
      .then((v) => {
        if (live) setCases(v);
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
    setVersions([]);
    setError("");
    if (id)
      api<CaseVersion[]>(`/cases/${id}/versions`)
        .then((v) => {
          if (live) setVersions(v);
        })
        .catch((e) => {
          if (live) setError(e.message);
        });
    return () => {
      live = false;
    };
  }, [id]);
  return (
    <div className="case-picker">
      <label>
        已保存的用例
        <select
          value={id}
          onChange={(e) => {
            setID(e.target.value);
            setRevision("");
            onSelect(null);
          }}
        >
          <option value="">请选择用例</option>
          {cases.map((c) => (
            <option key={c.id} value={c.id}>
              {c.latest.name} · v{c.latest.revision}
            </option>
          ))}
        </select>
      </label>
      <label>
        固定用例版本
        <select
          value={revision}
          disabled={!versions.length}
          onChange={(e) => {
            setRevision(e.target.value);
            onSelect(
              versions.find((v) => v.revision === Number(e.target.value)) ||
                null,
            );
          }}
        >
          <option value="">请选择版本</option>
          {versions.map((v) => (
            <option key={v.revision} value={v.revision}>
              v{v.revision} · {date(v.created_at)} · {v.change_note || v.name}
            </option>
          ))}
        </select>
      </label>
      {error && (
        <div className="alert error" role="alert">
          {error}
        </div>
      )}
    </div>
  );
}
export function Cases({
  tick,
  createSignal,
  runs,
  onStart,
}: {
  tick: number;
  createSignal: number;
  runs: Run[];
  onStart: (v: CaseVersion) => void;
}) {
  const [cases, setCases] = useState<TestCase[]>([]),
    [history, setHistory] = useState<CaseVersion[]>([]),
    [selected, setSelected] = useState<CaseVersion | null>(null),
    [activeID, setActiveID] = useState(""),
    [query, setQuery] = useState(""),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(true),
    [reload, setReload] = useState(0);
  const [editing, setEditing] = useState(false),
    [editID, setEditID] = useState(""),
    [baseRevision, setBaseRevision] = useState(0),
    [name, setName] = useState(""),
    [description, setDescription] = useState(""),
    [note, setNote] = useState(""),
    [json, setJSON] = useState(""),
    [saveError, setSaveError] = useState(""),
    [busy, setBusy] = useState(false);
  function create() {
    setEditID("");
    setBaseRevision(0);
    setName("");
    setDescription("");
    setNote("");
    setJSON("");
    setSaveError("");
    setEditing(true);
  }
  const seenCreate = useRef(createSignal);
  useEffect(() => {
    if (createSignal !== seenCreate.current) {
      seenCreate.current = createSignal;
      create();
    }
  }, [createSignal]);
  useEffect(() => {
    let live = true;
    setLoading(true);
    setError("");
    api<TestCase[]>("/cases")
      .then((v) => {
        if (live) setCases(v);
      })
      .catch((e) => {
        if (live) setError(e.message);
      })
      .finally(() => {
        if (live) setLoading(false);
      });
    return () => {
      live = false;
    };
  }, [tick, reload]);
  useEffect(() => {
    let live = true;
    setHistory([]);
    setSelected(null);
    if (activeID)
      api<CaseVersion[]>(`/cases/${activeID}/versions`)
        .then((v) => {
          if (live) {
            setHistory(v);
            setSelected(v[0]);
          }
        })
        .catch((e) => {
          if (live) setError(e.message);
        });
    return () => {
      live = false;
    };
  }, [activeID, reload, tick]);
  function edit(v: CaseVersion) {
    setEditID(v.case_id);
    setBaseRevision(history[0].revision);
    setName(v.name);
    setDescription(v.description);
    setNote(`基于 v${v.revision} 编辑`);
    setJSON(pretty(v.plan));
    setSaveError("");
    setEditing(true);
  }
  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setSaveError("");
    try {
      const v = await api<CaseVersion>(
        editID ? `/cases/${editID}/versions` : "/cases",
        {
          name,
          description,
          change_note: note,
          base_revision: baseRevision,
          plan: JSON.parse(json),
        },
      );
      setActiveID(v.case_id);
      setReload((x) => x + 1);
      setEditing(false);
    } catch (e) {
      setSaveError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  const visible = cases.filter((c) =>
    `${c.latest.name} ${c.latest.description} ${c.id}`
      .toLowerCase()
      .includes(query.toLowerCase()),
  );
  return (
    <>
      <section className="panel case-library">
        <div className="panel-top">
          <div>
            <div className="eyebrow">CASE LIBRARY</div>
            <h2>
              用例库 <span className="pill">{cases.length} 个用例</span>
            </h2>
            <p className="muted">完整计划 · 版本快照 · 可重复执行</p>
          </div>
          <BookOpen size={28} />
        </div>
        <div className="case-toolbar">
          <Search size={17} />
          <input
            aria-label="搜索用例"
            placeholder="搜索名称、描述或 ID"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          <button className="secondary" onClick={create}>
            <Plus size={15} />
            新建用例
          </button>
        </div>
        {error && (
          <div className="alert error" role="alert">
            {error}
            <button onClick={() => setReload((x) => x + 1)}>重试</button>
          </div>
        )}
        {loading ? (
          <p className="case-padding muted">正在加载用例…</p>
        ) : visible.length ? (
          <div className="table-scroll">
            <table>
              <thead>
                <tr>
                  <th>用例 / 描述</th>
                  <th>类型</th>
                  <th>最新版本</th>
                  <th>创建时间</th>
                  <th>更新时间</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {visible.map((c) => (
                  <tr
                    key={c.id}
                    className={activeID === c.id ? "case-active" : ""}
                  >
                    <td>
                      <button
                        className="case-title"
                        onClick={() => setActiveID(c.id)}
                      >
                        {c.latest.name}
                      </button>
                      <p className="case-description muted">
                        {c.latest.description || "暂无描述"}
                      </p>
                    </td>
                    <td>{kindLabel(c.latest.plan.spec.kind)}</td>
                    <td>
                      <span className="pill">v{c.latest.revision}</span>
                    </td>
                    <td>{date(c.created_at)}</td>
                    <td>{date(c.updated_at)}</td>
                    <td>
                      <button
                        className="secondary"
                        onClick={() => setActiveID(c.id)}
                      >
                        查看版本
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <Empty
            title={query ? "没有匹配的用例" : "建立你的第一个用例"}
            text="保存计划 JSON，也可以从已有测试运行中载入。每次编辑都会生成新版本。"
          />
        )}
      </section>
      {selected && (
        <section className="panel case-library">
          <div className="panel-top">
            <div>
              <div className="eyebrow">VERSION SNAPSHOT</div>
              <h2>
                {selected.name}{" "}
                <span className="pill">v{selected.revision}</span>
              </h2>
              <p className="mono muted">{selected.case_id}</p>
            </div>
          </div>
          <div className="case-padding">
            <div className="case-actions">
              <label>
                历史版本{" "}
                <select
                  aria-label="查看历史版本"
                  value={selected.revision}
                  onChange={(e) =>
                    setSelected(
                      history.find(
                        (v) => v.revision === Number(e.target.value),
                      ) || null,
                    )
                  }
                >
                  {history.map((v) => (
                    <option key={v.revision} value={v.revision}>
                      v{v.revision} · {date(v.created_at)}
                    </option>
                  ))}
                </select>
              </label>
              <button className="secondary" onClick={() => edit(selected)}>
                <Pencil size={15} />
                编辑为新版本
              </button>
              <button
                className="secondary"
                onClick={() => exportPlan(selected)}
              >
                <Download size={15} />
                导出 JSON
              </button>
              <button className="primary" onClick={() => onStart(selected)}>
                <Play size={15} />
                使用此版本测试
              </button>
            </div>
            <p className="case-description">
              {selected.description || "暂无描述"}
            </p>
            <p className="muted">
              版本创建于 {date(selected.created_at)} ·{" "}
              {selected.change_note || "未填写变更说明"}
            </p>
            <div className="case-facts">
              <span>环境：{selected.plan.environment.name}</span>
              <span>软件：{selected.plan.version.label}</span>
              <span>目标：{selected.plan.target}</span>
            </div>
            <label className="case-json-label">
              执行计划 JSON（只读快照）
              <textarea
                aria-label="用例版本 JSON"
                readOnly
                spellCheck={false}
                rows={16}
                value={pretty(selected.plan)}
              />
            </label>
          </div>
        </section>
      )}
      {editing && (
        <Dialog
          title={editID ? `保存为新版本 v${baseRevision + 1}` : "新建用例"}
          onClose={() => {
            if (!busy) setEditing(false);
          }}
        >
          <form onSubmit={save}>
            <p>
              保存完整 Plan JSON。保存时重新生成 trials 与 hash；场景参数取自
              spec.scenario。保存不会执行测试。请勿在 JSON 中保存密钥。
            </p>
            <label>
              用例名称
              <input
                required
                maxLength={200}
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="例如：混合读写 / 4 KiB / 云盘"
              />
            </label>
            <label>
              用例描述
              <textarea
                rows={3}
                maxLength={10000}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="测试目标、适用环境、预期与注意事项"
              />
            </label>
            <label>
              变更说明
              <input
                maxLength={2000}
                value={note}
                onChange={(e) => setNote(e.target.value)}
                placeholder="例如：并发数从 4 调整为 8"
              />
            </label>
            <label>
              从已有测试载入
              <select
                defaultValue=""
                onChange={(e) => {
                  const r = runs.find((r) => r.id === e.target.value);
                  if (r) {
                    setJSON(pretty(r.plan));
                    if (!name) setName(r.plan.spec.name);
                  }
                }}
              >
                <option value="">选择运行以载入计划</option>
                {runs.map((r) => (
                  <option key={r.id} value={r.id}>
                    {r.plan.spec.name} · {r.id}
                  </option>
                ))}
              </select>
            </label>
            <label>
              导入计划 JSON 文件
              <input
                type="file"
                accept=".json,application/json"
                onChange={async (e) => {
                  const f = e.target.files?.[0];
                  if (!f) return;
                  if (f.size > 4 * 1024 * 1024) {
                    setSaveError("文件不能超过 4 MiB");
                    return;
                  }
                  try {
                    setJSON(pretty(JSON.parse(await f.text())));
                    setSaveError("");
                  } catch (err) {
                    setSaveError((err as Error).message);
                  }
                }}
              />
            </label>
            <label>
              执行计划 JSON
              <textarea
                required
                rows={16}
                spellCheck={false}
                value={json}
                onChange={(e) => setJSON(e.target.value)}
                placeholder="粘贴 malta plan 生成的完整 JSON"
              />
            </label>
            {saveError && (
              <div className="alert error" role="alert">
                {saveError}
              </div>
            )}
            <div className="dialog-actions">
              <button
                type="button"
                className="secondary"
                disabled={busy}
                onClick={() => setEditing(false)}
              >
                取消
              </button>
              <button type="submit" className="primary" disabled={busy}>
                {busy ? "正在保存…" : editID ? "保存新版本" : "创建用例 v1"}
              </button>
            </div>
          </form>
        </Dialog>
      )}
    </>
  );
}
