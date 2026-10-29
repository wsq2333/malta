import { useEffect, useState } from "react";
import { Empty, Badge } from "./components";
import { api, date, size, type Run } from "./model";
import type { HardwareGroup } from "./hardware";
import { BackToList, RecordCard, Facts, TechnicalInfo } from "./RecordViews";

export function HardwareEnvironments({
  groups,
  detail,
  onDetail,
  onRun,
  onSelectSnapshot,
}: {
  groups: HardwareGroup[];
  detail: string;
  onDetail: (id: string) => void;
  onRun: (id: string) => void;
  onSelectSnapshot: (id: string) => void;
}) {
  const group = groups.find((g) => g.snapshots.some((s) => s.id === detail));
  const [tab, setTab] = useState("hardware"),
    [snapshotID, setSnapshotID] = useState("");
  const [runs, setRuns] = useState<Run[]>([]),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(false);
  const snapshot =
    group?.snapshots.find((s) => s.id === (snapshotID || detail)) ||
    group?.snapshots[0];
  const ids =
    group?.snapshots
      .map((s) => s.id)
      .sort()
      .join(",") || "";
  useEffect(() => {
    setTab("hardware");
    setSnapshotID("");
  }, [detail]);
  useEffect(() => {
    let live = true;
    setRuns([]);
    setError("");
    if (!ids || tab !== "runs") return;
    setLoading(true);
    Promise.all(
      ids
        .split(",")
        .map((id) =>
          api<Run[]>(`/runs?environment=${encodeURIComponent(id)}&limit=500`),
        ),
    )
      .then((results) => {
        if (live)
          setRuns(
            results
              .flat()
              .sort((a, b) => b.created_at.localeCompare(a.created_at)),
          );
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
  }, [ids, tab]);
  if (!detail)
    return (
      <div className="record-grid">
        {groups.length ? (
          groups.map((g) => (
            <RecordCard
              key={g.key}
              title={g.provider === "Alibaba Cloud" ? "阿里云" : g.provider}
              onOpen={() => onDetail(g.snapshots[0].id)}
            >
              <Facts
                items={g.machines.map((m) => [
                  m.role,
                  `${m.counts[0]} 台 × ${m.model}`,
                ])}
              />
              {!g.machines.some((m) => m.role === "测试客户端") && (
                <p className="muted">客户端尚未登记</p>
              )}
            </RecordCard>
          ))
        ) : (
          <Empty title="还没有硬件环境" />
        )}
      </div>
    );
  if (!group || !snapshot)
    return (
      <>
        <BackToList onClick={() => onDetail("")} />
        <Empty title="正在加载或未找到该硬件配置" />
      </>
    );
  const config = snapshot.config;
  return (
    <>
      <BackToList label="返回硬件环境" onClick={() => onDetail("")} />
      <section className="panel record-detail">
        <h2>
          {group.provider === "Alibaba Cloud" ? "阿里云" : group.provider}
        </h2>
        <div className="tabs record-tabs">
          {[
            ["hardware", "硬件配置"],
            ["snapshots", "测试配置"],
            ["runs", "关联测试"],
          ].map(([id, label]) => (
            <button
              key={id}
              className={tab === id ? "selected" : ""}
              onClick={() => setTab(id)}
            >
              {label}
            </button>
          ))}
        </div>
        {tab !== "runs" && (
          <label className="snapshot-picker">
            配置采集记录
            <select
              value={snapshot.id}
              onChange={(e) => setSnapshotID(e.target.value)}
            >
              {group.snapshots.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.config.observed_at
                    ? date(s.config.observed_at)
                    : "时间未登记"}{" "}
                  · {s.name}
                </option>
              ))}
            </select>
          </label>
        )}
        {tab === "hardware" && (
          <>
            <Facts
              items={[
                ["云类型", snapshot.provider || snapshot.kind],
                ["区域", snapshot.region],
                ["操作系统", snapshot.os],
                [
                  "客户端状态",
                  group.incompleteSnapshotIds.includes(snapshot.id)
                    ? "此历史记录未登记客户端"
                    : "已登记",
                ],
              ]}
            />
            <div className="table-scroll">
              <table>
                <thead>
                  <tr>
                    <th>角色</th>
                    <th>数量 / 机型</th>
                    <th>CPU / 内存</th>
                    <th>存储 / 网络</th>
                  </tr>
                </thead>
                <tbody>
                  {snapshot.machines.map((m, i) => (
                    <tr key={i}>
                      <td>
                        {m.role === "server (co-located master+worker)"
                          ? "服务端 / Worker（共置）"
                          : m.role.startsWith("benchmark client")
                            ? "测试客户端"
                            : m.role}
                      </td>
                      <td>
                        {m.count} 台 × {m.model}
                      </td>
                      <td>
                        {m.cpu}
                        <br />
                        {size(m.memory_bytes)}
                      </td>
                      <td className="wrap">
                        {m.storage}
                        <br />
                        {m.network}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <p className="muted">
              按物理机器计数，共置的 Master 和 Worker
              不重复计算。不同区域可属于同类硬件，比较时仍核对实际快照。
            </p>
          </>
        )}
        {tab === "snapshots" && (
          <>
            <Facts
              items={[
                ["运行身份 UID / GID", config.runner_uid_gid],
                [
                  "CPU 请求 / 上限",
                  `${config.runner_cpu_request_millis || config.runner_cpu_millis || "未登记"}m / ${config.runner_cpu_limit_millis || config.runner_cpu_millis || "未登记"}m`,
                ],
                [
                  "Runner 内存",
                  config.runner_memory_bytes
                    ? size(Number(config.runner_memory_bytes))
                    : "未登记",
                ],
                ["Malta 版本", config.runner_malta_version],
                ["测试节点", config.node],
                [
                  "测试卷 / 结果卷",
                  `${config.benchmark_pvc || "未登记"} / ${config.results_pvc || "未登记"}`,
                ],
                ["配置说明", config.client_status],
              ]}
            />
            <button
              className="secondary"
              onClick={() => onSelectSnapshot(snapshot.id)}
            >
              查看此配置的测试
            </button>
          </>
        )}
        {tab === "runs" && (
          <>
            {loading ? (
              <p>正在加载测试…</p>
            ) : error ? (
              <p role="alert">{error}</p>
            ) : !runs.length ? (
              <Empty title="暂无关联测试" />
            ) : (
              <div className="record-grid">
                {runs.map((r) => (
                  <RecordCard
                    key={r.id}
                    title={r.plan.spec.name}
                    subtitle={<Badge status={r.status} />}
                    onOpen={() => onRun(r.id)}
                  >
                    <p>
                      {r.plan.version.label} · {date(r.created_at)}
                    </p>
                  </RecordCard>
                ))}
              </div>
            )}
            <p className="muted">每份配置最多加载最近 500 条测试。</p>
          </>
        )}
        {tab !== "runs" && <TechnicalInfo value={snapshot} />}
      </section>
    </>
  );
}
