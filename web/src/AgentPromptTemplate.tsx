import { useRef, useState } from "react";
import { Copy, Download } from "lucide-react";
import template from "../../docs/agent-benchmark-prompt.md?raw";

export function AgentPromptTemplate() {
  const [text, setText] = useState(() =>
    template.replaceAll("{{MALTA_SERVER}}", location.origin),
  );
  const [message, setMessage] = useState("");
  const field = useRef<HTMLTextAreaElement>(null);
  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      setMessage("已复制提示词，可粘贴给 Agent 填写并执行。");
    } catch {
      // Internal HTTP deployments may not expose the Clipboard API.
      field.current?.focus();
      field.current?.select();
      try {
        if (document.execCommand("copy")) {
          setMessage("已复制提示词，可粘贴给 Agent 填写并执行。");
          return;
        }
      } catch {
        /* Keep the text selected for manual copying. */
      }
      setMessage("内容已全选，请按 Ctrl+C 或 ⌘C 复制。");
    }
  }
  function download() {
    const url = URL.createObjectURL(
      new Blob([text], { type: "text/markdown;charset=utf-8" }),
    );
    const a = document.createElement("a");
    a.href = url;
    a.download = "malta-agent-benchmark-prompt.md";
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    setMessage("已下载提示词 Markdown 文件。");
  }
  return (
    <section className="panel guide-section" id="agent-prompt">
      <div className="guide-heading">
        <span>08</span>
        <h2>Agent 提示词填空模板</h2>
      </div>
      <p>
        复制给
        Agent，填写【…】后使用。支持按固定用例版本执行或创建新计划，包含预算、提交恢复、对比和问题跟踪约定。
      </p>
      <p>
        服务地址已填入当前页面地址，请确认 Agent
        能访问。可以直接在下方填空；编辑仅保留在当前页面，离开前请复制或下载。此处不会发起测试，请勿填写凭证。
      </p>
      <div className="guide-code-header">
        <strong>发起性能测试 · Markdown</strong>
        <div>
          <button className="secondary" onClick={copy}>
            <Copy size={14} />
            复制提示词
          </button>
          <button className="secondary" onClick={download}>
            <Download size={14} />
            下载提示词
          </button>
        </div>
      </div>
      <textarea
        ref={field}
        className="agent-prompt-editor"
        aria-label="Agent 性能测试提示词"
        spellCheck={false}
        value={text}
        onChange={(e) => {
          setText(e.target.value);
          setMessage("");
        }}
      />
      <span className="guide-copy-status" role="status">
        {message}
      </span>
    </section>
  );
}
