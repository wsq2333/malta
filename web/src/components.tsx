import { useEffect, useMemo, type ReactNode } from "react";
import { Activity, BarChart3, Database, X } from "lucide-react";
import { ScenarioView } from "./ScenarioView";
import { Chart } from "./LazyChart";
import { format, metricLabel, statusLabel, summarize, type Run } from "./model";
export function Badge({ status }: { status: string }) {
  return (
    <span className={`badge ${status}`}>
      <i />
      {statusLabel[status] || status}
    </span>
  );
}
export function Empty({
  title = "还没有测试结果",
  text = "通过 CLI 或“发起测试”提交计划，完成后即可在这里查看真实数据。",
}: {
  title?: string;
  text?: string;
}) {
  return (
    <div className="empty">
      <Database size={30} />
      <h3>{title}</h3>
      <p>{text}</p>
    </div>
  );
}
export function Select({
  label,
  value,
  onChange,
  children,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  children: ReactNode;
}) {
  return (
    <label className="select-label">
      <span>{label}</span>
      <select value={value} onChange={(e) => onChange(e.target.value)}>
        {children}
      </select>
    </label>
  );
}
export function Dialog({
  title,
  onClose,
  children,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
}) {
  useEffect(() => {
    const fn = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", fn);
    return () => document.removeEventListener("keydown", fn);
  }, [onClose]);
  return (
    <div className="scrim" onClick={onClose}>
      <section
        className="dialog"
        role="dialog"
        aria-modal="true"
        aria-label={title}
        onClick={(e) => e.stopPropagation()}
      >
        <header>
          <h2>{title}</h2>
          <button aria-label="关闭" className="icon-button" onClick={onClose}>
            <X size={20} />
          </button>
        </header>
        {children}
      </section>
    </div>
  );
}
export function Stat({
  icon: Icon,
  label,
  value,
  note,
}: {
  icon: typeof Activity;
  label: string;
  value: number;
  note: string;
}) {
  return (
    <div className="stat">
      <div>
        <span>{label}</span>
        <Icon size={19} />
      </div>
      <strong>{value.toString().padStart(2, "0")}</strong>
      <small>{note}</small>
    </div>
  );
}
export function PanelTitle({
  title,
  subtitle,
}: {
  title: string;
  subtitle: string;
}) {
  return (
    <div className="panel-heading">
      <h2>{title}</h2>
      <p>{subtitle}</p>
    </div>
  );
}
export function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}
export function RunMetrics({ run }: { run: Run }) {
  const summaries = summarize(run);
  const throughput = summaries.filter((s) => s.metric === "throughput");
  const option = useMemo(() => {
    const jobs = [...new Set(throughput.map((s) => s.numjobs))].sort(
      (a, b) => a - b,
    );
    const series = [
      ...new Set(throughput.map((s) => `${s.operation}/${s.iodepth}`)),
    ].map((key) => {
      const [op, depth] = key.split("/");
      return {
        name: `${op} · depth ${depth}`,
        type: "line",
        symbolSize: 7,
        connectNulls: false,
        data: jobs.map((j) => {
          const s = throughput.find(
            (s) =>
              s.numjobs === j &&
              s.operation === op &&
              s.iodepth === Number(depth),
          );
          return s ? s.median / 1048576 : null;
        }),
      };
    });
    return {
      color: ["#169d77", "#65b7bd", "#a3c653", "#829ddb"],
      tooltip: { trigger: "axis" },
      legend: { bottom: 5 },
      grid: { left: 65, right: 25, top: 35, bottom: 65 },
      xAxis: { type: "category", name: "Jobs", data: jobs },
      yAxis: {
        type: "value",
        name: "MiB/s",
        splitLine: { lineStyle: { color: "#edf3ef" } },
      },
      series,
    };
  }, [run]);
  return (
    <>
      <ScenarioView run={run} />
      <div className="metric-heading">
        <BarChart3 size={17} />
        <h3>重复实验摘要</h3>
        <span>中位数 / Min / Max，按并发与操作分别统计</span>
      </div>
      {summaries.length ? (
        <div className="table-scroll">
          <table>
            <thead>
              <tr>
                <th>操作 / 指标</th>
                <th>Jobs × Depth</th>
                <th>中位数</th>
                <th>Min</th>
                <th>Max</th>
                <th>已完成重复</th>
              </tr>
            </thead>
            <tbody>
              {summaries.map((s) => (
                <tr key={s.key}>
                  <td>
                    {s.stage_id && <small>{s.stage_id} / {s.workload_id}<br /></small>}{s.operation} · {metricLabel[s.metric]}
                  </td>
                  <td>
                    {s.numjobs} × {s.iodepth}
                  </td>
                  <td>
                    <strong>{format(s.median, s.unit)}</strong>
                  </td>
                  <td>{format(s.min, s.unit)}</td>
                  <td>{format(s.max, s.unit)}</td>
                  <td>
                    {s.count} / {run.plan.spec.repeats}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <Empty
          title="尚无完整测量"
          text="准备阶段、失败与未完成试验不会被当作性能零值。"
        />
      )}
      {!run.plan.spec.scenario && throughput.length > 0 && (
        <>
          <h3 className="chart-title">已测并发曲线</h3>
          <Chart option={option} label="吞吐随 jobs 和队列深度变化的实测曲线" />
        </>
      )}
      {run.plan.spec.kind === "metadata" && (
        <div className="panel-foot">
          元数据延迟：未采集。当前仅展示实际导入的操作速率。
        </div>
      )}
    </>
  );
}
