import { useEffect, useRef, useState, type FormEvent } from "react";
import { Download, Pencil, Search } from "lucide-react";
import { api, date } from "./model";
import { Dialog, Empty } from "./components";
import example from "./issue-example.json";

type IssueInput = typeof example & {
  version_id?: string;
  environment_id?: string;
  run_id?: string;
};
type Issue = Omit<IssueInput, "base_revision"> & {
  revision: number;
  created_at: string;
  updated_at: string;
  resolved_at: string | null;
};
const pretty = (v: unknown) => JSON.stringify(v, null, 2);
function editable(v: Issue): IssueInput {
  const {
    revision,
    created_at: _c,
    updated_at: _u,
    resolved_at: _r,
    ...fields
  } = v;
  return { ...fields, base_revision: revision, change_note: "" };
}
function download(value: unknown, filename: string) {
  const url = URL.createObjectURL(
    new Blob([pretty(value) + "\n"], { type: "application/json" }),
  );
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
function parseInput(raw: string): IssueInput {
  const v = JSON.parse(raw);
  if (!v || Array.isArray(v) || typeof v !== "object")
    throw new Error("请上传单个问题对象，不包含 API 的 data 外层。");
  for (const key of Object.keys(example)) {
    if (typeof v[key] !== typeof example[key as keyof typeof example])
      throw new Error(`字段 ${key} 的类型不正确或缺失。`);
  }
  for (const key of ["version_id", "environment_id", "run_id"]) {
    if (v[key] !== undefined && typeof v[key] !== "string")
      throw new Error(`${key} 必须是字符串。`);
  }
  return v as IssueInput;
}
export function Issues({
  tick,
  createSignal,
}: {
  tick: number;
  createSignal: number;
}) {
  const [items, setItems] = useState<Issue[]>([]),
    [history, setHistory] = useState<Issue[]>([]);
  const [active, setActive] = useState(""),
    [revision, setRevision] = useState(0);
  const [query, setQuery] = useState(""),
    [type, setType] = useState(""),
    [status, setStatus] = useState("");
  const [error, setError] = useState(""),
    [loading, setLoading] = useState(true),
    [reload, setReload] = useState(0);
  const [editing, setEditing] = useState(false),
    [jsonMode, setJSONMode] = useState(false);
  const [draft, setDraft] = useState<IssueInput>(example),
    [raw, setRaw] = useState("");
  const [busy, setBusy] = useState(false),
    [saveError, setSaveError] = useState("");
  function open(v?: Issue) {
    const next = v
      ? editable(v)
      : {
          ...example,
          issue_id: `issue-${crypto.randomUUID()}`,
          title: "",
          description: "",
          software: "",
          hardware: "",
          occurred_at: new Date().toISOString(),
          reporter: "user",
        };
    setDraft(next);
    setRaw(pretty(next));
    setJSONMode(false);
    setSaveError("");
    setEditing(true);
  }
  const seenCreate = useRef(createSignal);
  useEffect(() => {
    if (seenCreate.current !== createSignal) {
      seenCreate.current = createSignal;
      open();
    }
  }, [createSignal]);
  useEffect(() => {
    let live = true;
    setLoading(true);
    setError("");
    api<Issue[]>("/issues")
      .then((v) => {
        if (live) setItems(v);
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
    setRevision(0);
    if (active)
      api<Issue[]>(`/issues/${encodeURIComponent(active)}/history`)
        .then((v) => {
          if (live) {
            setHistory(v);
            setRevision(v[0].revision);
          }
        })
        .catch((e) => {
          if (live) setError(e.message);
        });
    return () => {
      live = false;
    };
  }, [active, tick, reload]);
  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setSaveError("");
    try {
      const q = jsonMode ? parseInput(raw) : draft;
      const v = await api<Issue>("/issues", q);
      setActive(v.issue_id);
      setEditing(false);
      setReload((n) => n + 1);
    } catch (e) {
      setSaveError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  const selected = history.find((v) => v.revision === revision);
  const types = [
    ...new Set(["功能", "性能", "稳定性", ...items.map((v) => v.type)]),
  ].sort();
  const visible = items.filter(
    (v) =>
      (!type || v.type === type) &&
      (!status || String(v.resolved) === status) &&
      [
        v.issue_id,
        v.title,
        v.description,
        v.software,
        v.hardware,
        v.reporter,
        v.run_id,
        v.version_id,
        v.environment_id,
      ]
        .join(" ")
        .toLowerCase()
        .includes(query.toLowerCase()),
  );
  function textField(
    key: keyof IssueInput,
    label: string,
    required = true,
    multiline = false,
  ) {
    const common = {
      value: String(draft[key] ?? ""),
      required,
      onChange: (
        e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>,
      ) => setDraft({ ...draft, [key]: e.target.value }),
    };
    return (
      <label key={key}>
        {label}
        {multiline ? (
          <textarea {...common} rows={3} />
        ) : (
          <input
            {...common}
            list={key === "type" ? "issue-types" : undefined}
          />
        )}
      </label>
    );
  }
  return (
    <>
      <section className="panel">
        <div className="case-toolbar">
          <Search size={16} />
          <input
            aria-label="搜索问题"
            placeholder="搜索 ID、描述、软件、硬件或运行…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          <select
            aria-label="问题类型筛选"
            value={type}
            onChange={(e) => setType(e.target.value)}
          >
            <option value="">全部类型</option>
            {types.map((t) => (
              <option key={t}>{t}</option>
            ))}
          </select>
          <select
            aria-label="解决状态筛选"
            value={status}
            onChange={(e) => setStatus(e.target.value)}
          >
            <option value="">全部状态</option>
            <option value="false">未解决</option>
            <option value="true">已解决</option>
          </select>
          <button
            className="secondary"
            onClick={() => download(example, "issue.example.json")}
          >
            <Download size={15} />
            JSON 模板
          </button>
        </div>
        <p className="case-padding muted">
          共 {items.length} 个问题 · {items.filter((v) => !v.resolved).length}{" "}
          个未解决 · 当前显示 {visible.length} 个
        </p>
        {error && (
          <div className="alert error" role="alert">
            {error}
            <button onClick={() => setReload((n) => n + 1)}>重试</button>
          </div>
        )}
        {loading ? (
          <p className="case-padding">正在加载问题…</p>
        ) : (
          !error &&
          (visible.length ? (
            <div className="table-scroll">
              <table>
                <thead>
                  <tr>
                    <th>问题 / ID</th>
                    <th>类型</th>
                    <th>状态</th>
                    <th>软件 / 硬件</th>
                    <th>发现时间</th>
                    <th>更新</th>
                  </tr>
                </thead>
                <tbody>
                  {visible.map((v) => (
                    <tr
                      key={v.issue_id}
                      className={active === v.issue_id ? "selected" : ""}
                    >
                      <td>
                        <button
                          className="issue-title"
                          onClick={() => setActive(v.issue_id)}
                        >
                          {v.title}
                        </button>
                        <small className="muted">{v.issue_id}</small>
                      </td>
                      <td>
                        <span className="pill">{v.type}</span>
                      </td>
                      <td>
                        <span
                          className={`badge ${v.resolved ? "succeeded" : "pending"}`}
                        >
                          {v.resolved ? "已解决" : "未解决"}
                        </span>
                      </td>
                      <td>
                        <span className="issue-summary">{v.software}</span>
                        <small className="issue-summary muted">
                          {v.hardware}
                        </small>
                      </td>
                      <td>{date(v.occurred_at)}</td>
                      <td>
                        v{v.revision}
                        <small className="muted">{date(v.updated_at)}</small>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <Empty
              title={items.length ? "没有匹配的问题" : "还没有登记问题"}
              text="点击“登记问题”填写表单，或上传用户 / agent 生成的 JSON。"
            />
          ))
        )}
      </section>
      {selected && (
        <section className="panel case-padding issue-detail">
          <div className="case-actions">
            <h2>{selected.title}</h2>
            <select
              aria-label="问题历史版本"
              value={revision}
              onChange={(e) => setRevision(Number(e.target.value))}
            >
              {history.map((v) => (
                <option key={v.revision} value={v.revision}>
                  v{v.revision} · {date(v.updated_at)}
                </option>
              ))}
            </select>
            <button
              className="secondary"
              onClick={() =>
                download(
                  selected,
                  `${selected.issue_id}-v${selected.revision}.json`,
                )
              }
            >
              导出快照
            </button>
            <button className="primary" onClick={() => open(history[0])}>
              <Pencil size={15} />
              编辑最新版本 / 解决
            </button>
          </div>
          <p className="muted">
            {selected.issue_id} · {selected.type} ·{" "}
            {selected.resolved ? "已解决" : "未解决"} · 报告人{" "}
            {selected.reporter}
          </p>
          <p className="case-description">{selected.description}</p>
          <dl className="issue-facts">
            <dt>软件</dt>
            <dd>
              {selected.software}
              {selected.version_id && (
                <small>版本 ID：{selected.version_id}</small>
              )}
            </dd>
            <dt>硬件</dt>
            <dd>
              {selected.hardware}
              {selected.environment_id && (
                <small>环境 ID：{selected.environment_id}</small>
              )}
            </dd>
            <dt>测试运行</dt>
            <dd>{selected.run_id || "未关联"}</dd>
            <dt>发现 / 创建</dt>
            <dd>
              {date(selected.occurred_at)} / {date(selected.created_at)}
            </dd>
            <dt>解决时间</dt>
            <dd>{selected.resolved_at ? date(selected.resolved_at) : "—"}</dd>
            <dt>解决方式</dt>
            <dd className="case-description">
              {selected.resolution || "尚未解决"}
            </dd>
            <dt>变更说明</dt>
            <dd className="case-description">{selected.change_note || "—"}</dd>
          </dl>
          <details>
            <summary>查看此版本 JSON</summary>
            <pre className="issue-json">{pretty(selected)}</pre>
          </details>
        </section>
      )}
      {editing && (
        <Dialog
          title={draft.base_revision ? "编辑问题 · 保存新版本" : "登记问题"}
          onClose={() => {
            if (!busy) setEditing(false);
          }}
        >
          <form onSubmit={save}>
            <p>
              问题类型可以自定义，如“功能”“性能”。保存不会发起测试。每次更新保留历史；请勿上传密钥。
            </p>
            <fieldset disabled={busy} className="issue-fields">
              <div className="case-actions">
                <button
                  type="button"
                  className="secondary"
                  onClick={() => {
                    try {
                      if (jsonMode) setDraft(parseInput(raw));
                      else setRaw(pretty(draft));
                      setJSONMode(!jsonMode);
                      setSaveError("");
                    } catch (e) {
                      setSaveError((e as Error).message);
                    }
                  }}
                >
                  {jsonMode ? "切换表单" : "编辑 JSON"}
                </button>
                <button
                  type="button"
                  className="secondary"
                  onClick={() => {
                    try {
                      download(
                        jsonMode ? parseInput(raw) : draft,
                        "issue-input.json",
                      );
                    } catch (e) {
                      setSaveError((e as Error).message);
                    }
                  }}
                >
                  下载提交 JSON
                </button>
              </div>
              <label>
                上传问题 JSON（最大 128 KiB）
                <input
                  type="file"
                  accept=".json,application/json"
                  onChange={async (e) => {
                    const f = e.target.files?.[0];
                    if (!f) return;
                    try {
                      if (f.size > 128 * 1024)
                        throw new Error("文件不能超过 128 KiB");
                      const text = await f.text();
                      JSON.parse(text);
                      setRaw(text);
                      setJSONMode(true);
                      setSaveError("");
                    } catch (e) {
                      setSaveError((e as Error).message);
                    }
                  }}
                />
              </label>
              {jsonMode ? (
                <label>
                  问题 JSON
                  <textarea
                    required
                    rows={21}
                    spellCheck={false}
                    value={raw}
                    onChange={(e) => setRaw(e.target.value)}
                  />
                </label>
              ) : (
                <>
                  <p className="muted">
                    {draft.issue_id} · 基于 v{draft.base_revision} 保存
                  </p>
                  {textField("title", "问题标题")}
                  {textField("type", "问题类型（可自定义）")}
                  <datalist id="issue-types">
                    {types.map((t) => (
                      <option key={t} value={t} />
                    ))}
                  </datalist>
                  {textField(
                    "description",
                    "描述 / 复现步骤 / 证据",
                    true,
                    true,
                  )}
                  {textField("software", "对应软件及版本")}
                  {textField("hardware", "对应硬件 / 环境")}
                  {textField("occurred_at", "发现时间（RFC3339，含时区）")}
                  {textField("reporter", "报告人 / agent 名称")}
                  <details>
                    <summary>关联已登记的数据（可选）</summary>
                    {textField("version_id", "软件版本 ID", false)}
                    {textField("environment_id", "硬件环境 ID", false)}
                    {textField("run_id", "运行 ID", false)}
                  </details>
                  <label className="issue-resolved">
                    <input
                      type="checkbox"
                      checked={draft.resolved}
                      onChange={(e) =>
                        setDraft({
                          ...draft,
                          resolved: e.target.checked,
                          resolution: e.target.checked ? draft.resolution : "",
                        })
                      }
                    />
                    已解决
                  </label>
                  {draft.resolved &&
                    textField("resolution", "解决方式及验证结果", true, true)}
                  {textField(
                    "change_note",
                    "变更说明（调查进展 / 重新打开原因）",
                    false,
                    true,
                  )}
                </>
              )}
            </fieldset>
            {saveError && (
              <div className="alert error" role="alert">
                {saveError}（若版本冲突，请保留当前内容并重新载入最新问题。）
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
                {busy ? "保存中…" : "保存问题"}
              </button>
            </div>
          </form>
        </Dialog>
      )}
    </>
  );
}
