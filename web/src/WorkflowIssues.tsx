import { useEffect, useState } from "react";
import { api, date } from "./model";

type Association = {
  link: {
    run_id: string;
    issue_id: string;
    role: string;
    note: string;
    created_at: string;
    issue_revision: number;
  };
  issue: {
    issue_id: string;
    title: string;
    resolved: boolean;
    resolution: string;
  };
};
export function WorkflowIssues({
  run,
}: {
  run: {
    id: string;
    verdict: string;
    status: string;
    result_state: string;
    snapshot: {
      case: { name: string };
      system: { name: string };
      version: { id: string; label: string };
      environment: { id: string };
    };
  };
}) {
  const [items, setItems] = useState<Association[]>([]),
    [id, setID] = useState(""),
    [role, setRole] = useState("discovery"),
    [note, setNote] = useState(""),
    [error, setError] = useState(""),
    [tick, setTick] = useState(0),
    [busy, setBusy] = useState(false),
    [draft, setDraft] = useState("");
  useEffect(() => {
    let live = true;
    api<Association[]>(`/workflow/runs/${run.id}/issues`)
      .then((v) => {
        if (live) setItems(v);
      })
      .catch((e) => {
        if (live) setError(e.message);
      });
    return () => {
      live = false;
    };
  }, [run.id, tick]);
  function newIssue() {
    setDraft(
      JSON.stringify(
        {
          issue_id: `issue-${crypto.randomUUID()}`,
          base_revision: 0,
          title: `${run.snapshot.case.name}：待描述问题`,
          type: "功能",
          description: `测试运行 ${run.id} 的问题，请补充实际现象和复现信息。`,
          software: run.snapshot.version.label,
          hardware: run.snapshot.system.name,
          version_id: run.snapshot.version.id,
          environment_id: run.snapshot.environment.id,
          occurred_at: new Date().toISOString(),
          resolved: false,
          resolution: "",
          reporter: "user",
          change_note: "从工作流运行登记",
        },
        null,
        2,
      ),
    );
    setRole("discovery");
  }
  async function link() {
    setBusy(true);
    setError("");
    try {
      let issueID = id;
      if (draft) {
        const issue = await api<{ issue_id: string }>(
          "/issues",
          JSON.parse(draft),
        );
        issueID = issue.issue_id;
        setID(issueID);
      }
      await api(`/workflow/runs/${run.id}/issues`, {
        issue_id: issueID,
        role: draft ? "discovery" : role,
        note,
      });
      setDraft("");
      setTick((v) => v + 1);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="wf-card">
      <h3>问题追踪</h3>
      <p>发现记录与修复验证分别关联。验证通过后仍需在问题页面填写解决方式。</p>
      <Associations items={items} />
      <details>
        <summary>登记问题或关联已有问题</summary>
        <div className="wf-editor">
          <button onClick={newIssue}>用本次运行生成问题 JSON</button>
          {draft ? (
            <label>
              问题 JSON
              <textarea
                className="wf-code"
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
              />
              <button onClick={() => setDraft("")}>改为关联已有问题</button>
            </label>
          ) : (
            <>
              <label>
                Issue ID
                <input value={id} onChange={(e) => setID(e.target.value)} />
              </label>
              <label>
                关联类型
                <select value={role} onChange={(e) => setRole(e.target.value)}>
                  <option value="discovery">发现问题</option>
                  <option
                    value="verification"
                    disabled={
                      run.status !== "succeeded" ||
                      run.verdict !== "passed" ||
                      run.result_state !== "complete"
                    }
                  >
                    修复验证（需通过）
                  </option>
                </select>
              </label>
            </>
          )}
          <label>
            关联说明
            <input value={note} onChange={(e) => setNote(e.target.value)} />
          </label>
          {error && (
            <p role="alert" className="alert error">
              {error}
            </p>
          )}
          <button disabled={busy || (!id && !draft)} onClick={link}>
            {busy ? "保存中…" : "保存关联"}
          </button>
        </div>
      </details>
    </section>
  );
}
function Associations({ items }: { items: Association[] }) {
  return (
    <ul>
      {items.map((v) => (
        <li key={`${v.link.run_id}-${v.link.issue_id}-${v.link.role}`}>
          <a
            href={`?page=issues&detail=${encodeURIComponent(v.issue.issue_id)}`}
          >
            {v.issue.title}
          </a>{" "}
          · {v.link.role === "discovery" ? "发现问题" : "修复验证"} ·{" "}
          {v.issue.resolved ? "已解决" : "待解决"}
          <small>
            {" "}
            {date(v.link.created_at)} · 关联时问题版本 v{v.link.issue_revision}
          </small>
          {v.link.note && <p>{v.link.note}</p>}
          {v.issue.resolution && <p>{v.issue.resolution}</p>}
        </li>
      ))}
    </ul>
  );
}
export function WorkflowIssueHistory({ issueID }: { issueID: string }) {
  const [items, setItems] = useState<Association[]>([]),
    [error, setError] = useState("");
  useEffect(() => {
    let live = true;
    api<Association[]>(`/workflow/issues/${issueID}/runs`)
      .then((v) => {
        if (live) {
          setItems(v);
          setError("");
        }
      })
      .catch((e) => {
        if (live) setError(e.message);
      });
    return () => {
      live = false;
    };
  }, [issueID]);
  return (
    <section>
      <h3>关联工作流</h3>
      {error && <p role="alert">{error}</p>}
      {items.length === 0 && !error && <p>暂无关联工作流。</p>}
      {items.map((v) => (
        <p key={`${v.link.run_id}-${v.link.role}`}>
          <a
            href={`?page=automation&workflow_run=${encodeURIComponent(v.link.run_id)}`}
          >
            {v.link.run_id}
          </a>{" "}
          · {v.link.role === "discovery" ? "发现问题" : "修复验证"} ·{" "}
          {date(v.link.created_at)}
        </p>
      ))}
    </section>
  );
}
