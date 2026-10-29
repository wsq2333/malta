import type { ReactNode } from "react";
import { ArrowLeft, ArrowRight } from "lucide-react";
import { operationLabel, size, type Plan } from "./model";

export function BackToList({
  onClick,
  label = "返回列表",
}: {
  onClick: () => void;
  label?: string;
}) {
  return (
    <button className="secondary record-back" onClick={onClick}>
      <ArrowLeft size={16} />
      {label}
    </button>
  );
}
export function RecordCard({
  title,
  subtitle,
  children,
  onOpen,
  action = "查看详情",
}: {
  title: string;
  subtitle?: ReactNode;
  children?: ReactNode;
  onOpen: () => void;
  action?: string;
}) {
  return (
    <article className="panel record-card">
      {subtitle && <div className="record-meta">{subtitle}</div>}
      <h2>{title}</h2>
      <div className="record-card-body">{children}</div>
      <button className="secondary" onClick={onOpen}>
        {action}
        <ArrowRight size={15} />
      </button>
    </article>
  );
}
export function Facts({ items }: { items: [string, ReactNode][] }) {
  return (
    <dl className="readable-facts">
      {items.map(([label, value], index) => (
        <div key={`${label}-${index}`}>
          <dt>{label}</dt>
          <dd>
            {value === null || value === undefined || value === ""
              ? "未登记"
              : typeof value === "boolean"
                ? value
                  ? "是"
                  : "否"
                : value}
          </dd>
        </div>
      ))}
    </dl>
  );
}
export function TechnicalInfo({
  value,
  children,
}: {
  value?: unknown;
  children?: ReactNode;
}) {
  return (
    <details className="technical-info">
      <summary>技术信息 / 原始 JSON</summary>
      {children}
      {value !== undefined && <pre>{JSON.stringify(value, null, 2)}</pre>}
    </details>
  );
}
export function WorkloadFacts({ plan }: { plan: Plan }) {
  const s = plan.spec;
  if (s.kind === "scenario")
    return (
      <div className="scenario-summary">
        <h3>混合负载流程</h3>
        {s.scenario?.stages.map((stage) => (
          <section key={stage.id}>
            <h4>
              {stage.id} · {stage.mode}
            </h4>
            <p>
              预热 {stage.warmup_seconds}s · 测量 {stage.runtime_seconds}s
            </p>
            {stage.workloads.map((w) => (
              <p key={w.id}>
                {w.id} · {w.kind} · {w.numjobs} 并发 × 深度 {w.iodepth}
              </p>
            ))}
          </section>
        ))}
      </div>
    );
  if (s.kind === "metadata")
    return (
      <Facts items={Object.entries(s.metadata || {}).map(([k, v]) => [k, v])} />
    );
  const w = s.workload;
  return (
    <Facts
      items={[
        ["读写类型", operationLabel[w.operation] || w.operation],
        ["文件大小", size(w.file_size_bytes)],
        ["I/O 块大小", size(w.block_size_bytes)],
        ["每并发文件数", String(w.files_per_job)],
        [
          "并发 / 队列深度",
          `${s.matrix.numjobs.join("、")} / ${s.matrix.iodepth.join("、")}`,
        ],
        ["测量 / 预热", `${w.runtime_seconds}s / ${w.warmup_seconds}s`],
        ["重复次数", String(s.repeats)],
        ["缓存 / I/O 引擎", `${w.cache_state} / ${w.ioengine}`],
        ["访问方式", w.direct ? "直接 I/O" : "缓冲 I/O"],
      ]}
    />
  );
}
