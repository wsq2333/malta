import { ArrowRight, Server } from "lucide-react";
import { Empty } from "./components";
import { size } from "./model";
import type { HardwareGroup } from "./hardware";

export function HardwareEnvironments({
  groups,
  onSelectSnapshot,
}: {
  groups: HardwareGroup[];
  onSelectSnapshot: (id: string) => void;
}) {
  return (
    <>
      <section className="panel hardware-explanation">
        <strong>
          {groups.length} 类硬件 ·{" "}
          {groups.reduce((n, g) => n + g.snapshots.length, 0)} 份执行快照
        </strong>
        <p>
          按云厂商、区域和各角色机型归组。UID/GID、资源额度、镜像和挂载参数保存在执行快照中；同类硬件的结果仍需核对测试条件后才能比较。
        </p>
      </section>
      <div className="environment-grid hardware-grid">
        {!groups.length ? (
          <section className="panel">
            <Empty
              title="还没有硬件环境"
              text="登记云环境及服务端、Worker、测试客户端的机型。"
            />
          </section>
        ) : (
          groups.map((group) => (
            <section className="panel environment-card" key={group.key}>
              <header>
                <span className="environment-icon">
                  <Server size={22} />
                </span>
                <span className="pill">
                  {group.snapshots.length} 份执行快照
                </span>
              </header>
              <h2>{group.location}</h2>
              {group.machines.map((machine) => (
                <div
                  className="machine"
                  key={JSON.stringify([machine.role, machine.model])}
                >
                  <div>
                    <strong>{machine.role}</strong>
                    <span>
                      {machine.counts.length === 1
                        ? `${machine.counts[0]} 台`
                        : `${machine.counts.join(" / ")} 台（依快照）`}
                    </span>
                  </div>
                  <b>{machine.model}</b>
                </div>
              ))}
              {!group.machines.some((m) => m.role === "测试客户端") && (
                <p className="muted">测试客户端机型尚未登记</p>
              )}
              <details className="hardware-snapshots">
                <summary>执行快照（{group.snapshots.length}）</summary>
                <p className="muted">
                  保留每次运行的真实配置；早期未登记的客户端信息不会补写到历史快照。
                </p>
                <div className="hardware-snapshot-list">
                  {group.snapshots.map((snapshot) => {
                    const config = snapshot.config;
                    const request =
                      config.runner_cpu_request_millis ||
                      config.runner_cpu_millis;
                    const limit =
                      config.runner_cpu_limit_millis ||
                      config.runner_cpu_millis;
                    return (
                      <article className="hardware-snapshot" key={snapshot.id}>
                        <h3>{snapshot.name}</h3>
                        <p className="mono muted">{snapshot.id}</p>
                        {group.incompleteSnapshotIds.includes(snapshot.id) && (
                          <span className="pill">客户端未登记</span>
                        )}
                        <dl>
                          <dt>UID / GID</dt>
                          <dd>{config.runner_uid_gid || "未登记"}</dd>
                          <dt>CPU 请求 / 上限</dt>
                          <dd>
                            {request ? `${request}m` : "未登记"} /{" "}
                            {limit ? `${limit}m` : "未登记"}
                          </dd>
                          <dt>Runner 内存</dt>
                          <dd>
                            {config.runner_memory_bytes
                              ? size(Number(config.runner_memory_bytes))
                              : "未登记"}
                          </dd>
                          <dt>Malta 版本</dt>
                          <dd>{config.runner_malta_version || "未登记"}</dd>
                          <dt>采集时间</dt>
                          <dd>{config.observed_at || "未登记"}</dd>
                        </dl>
                        <details>
                          <summary>
                            完整配置（系统、存储、网络、挂载等）
                          </summary>
                          <pre>{JSON.stringify(snapshot, null, 2)}</pre>
                        </details>
                        <button
                          className="secondary full"
                          onClick={() => onSelectSnapshot(snapshot.id)}
                        >
                          查看此快照基线 <ArrowRight size={15} />
                        </button>
                      </article>
                    );
                  })}
                </div>
              </details>
            </section>
          ))
        )}
      </div>
    </>
  );
}
