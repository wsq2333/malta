import { useEffect, useState } from "react";
import { api, date, type Run, type Version } from "./model";
import { Empty, Badge } from "./components";
import { BackToList, RecordCard, Facts, TechnicalInfo } from "./RecordViews";

export function Versions({
  versions,
  detail,
  onDetail,
  onRun,
}: {
  versions: Version[];
  detail: string;
  onDetail: (id: string) => void;
  onRun: (id: string) => void;
}) {
  const [runs, setRuns] = useState<Run[]>([]),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(false);
  const selected = versions.find((v) => v.id === detail);
  useEffect(() => {
    let live = true;
    setRuns([]);
    setError("");
    if (!detail) return;
    setLoading(true);
    api<Run[]>(`/runs?version=${encodeURIComponent(detail)}&limit=500`)
      .then((v) => {
        if (live) setRuns(v);
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
  }, [detail]);
  if (detail)
    return (
      <>
        <BackToList onClick={() => onDetail("")} label="返回软件版本" />
        {selected ? (
          <section className="panel record-detail">
            <h2>{selected.label}</h2>
            <Facts
              items={[
                ["文件系统", selected.filesystem],
                ["已加载测试", `${runs.length} 次（最多 500 条）`],
                ["最近测试", runs[0] ? date(runs[0].created_at) : "暂无"],
              ]}
            />
            <h3>关联测试</h3>
            {loading ? (
              <p>正在加载测试…</p>
            ) : error ? (
              <p role="alert">{error}</p>
            ) : !runs.length ? (
              <Empty title="该版本暂无测试" />
            ) : (
              <div className="record-grid">
                {runs.map((r) => (
                  <RecordCard
                    key={r.id}
                    title={r.plan.spec.name}
                    subtitle={<Badge status={r.status} />}
                    onOpen={() => onRun(r.id)}
                  >
                    <p>{date(r.created_at)}</p>
                  </RecordCard>
                ))}
              </div>
            )}
            <TechnicalInfo value={selected} />
          </section>
        ) : (
          <Empty title="正在加载或未找到该软件版本" />
        )}
      </>
    );
  return (
    <div className="record-grid">
      {versions.length ? (
        versions.map((v) => (
          <RecordCard
            key={v.id}
            title={v.label}
            subtitle={v.filesystem}
            onOpen={() => onDetail(v.id)}
          >
            <p className="muted">查看构建信息与关联测试</p>
          </RecordCard>
        ))
      ) : (
        <Empty title="还没有登记版本" />
      )}
    </div>
  );
}
