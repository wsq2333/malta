import { Badge, Empty } from "./components";
import { RecordCard, Facts } from "./RecordViews";
import { baselineEntries } from "./catalog";
import {
  date,
  format,
  peak,
  operationLabel,
  size,
  type Run,
  type Baseline,
} from "./model";

export function RunCatalog({
  runs,
  allRuns,
  baselines,
  view,
  filtered,
  onOpen,
}: {
  runs: Run[];
  allRuns: Run[];
  baselines: Baseline[];
  view: string;
  filtered: boolean;
  onOpen: (id: string) => void;
}) {
  const entries =
    view === "saved"
      ? baselineEntries(baselines, runs, filtered, allRuns).map(
          ({ baseline, run }) => ({
            key: baseline.id,
            id: baseline.run_id,
            title: baseline.name,
            run,
            at: baseline.created_at,
          }),
        )
      : runs.map((run) => ({
          key: run.id,
          id: run.id,
          title: run.plan.spec.name,
          run,
          at: run.created_at,
        }));
  if (!entries.length)
    return (
      <Empty
        title={view === "saved" ? "暂无已设基线" : "暂无匹配的测试"}
        text={
          view === "saved"
            ? "在“全部测试”中打开成功记录，点击“设为基线”。"
            : "发起测试，或调整筛选条件。"
        }
      />
    );
  return (
    <div className="record-grid">
      {entries.map(({ key, id, title, run, at }) => (
        <RecordCard
          key={key}
          title={title}
          subtitle={
            <>
              {run && <Badge status={run.status} />}
              <span>{date(at)}</span>
            </>
          }
          onOpen={() => onOpen(id)}
        >
          {run ? (
            <Facts
              items={[
                ["软件版本", run.plan.version.label],
                [
                  "负载",
                  run.plan.spec.kind === "data"
                    ? `${operationLabel[run.plan.spec.workload.operation]} · ${size(run.plan.spec.workload.file_size_bytes)}`
                    : run.plan.spec.kind === "scenario"
                      ? "混合场景"
                      : "元数据",
                ],
                [
                  "云环境",
                  [
                    run.plan.environment.provider || run.plan.environment.kind,
                    run.plan.environment.region,
                  ]
                    .filter(Boolean)
                    .join(" / "),
                ],
                [
                  "主要指标",
                  run.plan.spec.kind === "data"
                    ? format(peak(run)?.median, "bytes/s")
                    : `${run.trials.filter((t) => t.status === "succeeded").length} / ${run.plan.trials.length} 项成功`,
                ],
              ]}
            />
          ) : (
            <p className="muted">关联记录未在当前加载范围内，打开详情查看。</p>
          )}
        </RecordCard>
      ))}
    </div>
  );
}
