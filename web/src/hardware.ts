import type { Environment } from "./model";

export interface HardwareMachine {
  role: string;
  model: string;
  counts: number[];
}
export interface HardwareGroup {
  key: string;
  location: string;
  provider: string;
  machines: HardwareMachine[];
  snapshots: Environment[];
  incompleteSnapshotIds: string[];
}

function role(value: string): string {
  const normalized = value.trim().toLowerCase();
  if (normalized === "server (co-located master+worker)")
    return "服务端 / Worker（共置）";
  if (
    /^(benchmark client\b|client\b|runner\b|测试客户端|客户端)/.test(normalized)
  )
    return "测试客户端";
  const aliases: Record<string, string> = {
    server: "服务端",
    服务端: "服务端",
    worker: "Worker",
    master: "Master",
  };
  return aliases[normalized] || normalized;
}
const configurationKey = (m: { role: string; model: string; count: number }) =>
  JSON.stringify([m.role, m.model, m.count]);
const machineKey = (m: { role: string; model: string }) =>
  JSON.stringify([m.role, m.model]);

/** Display categories only. Immutable snapshots still govern execution and comparison. */
export function groupHardware(environments: Environment[]): HardwareGroup[] {
  const entries = environments.map((environment) => {
    const machines = new Map<
      string,
      { role: string; model: string; count: number }
    >();
    for (const m of environment.machines) {
      const item = {
        role: role(m.role),
        model: m.model.trim(),
        count: m.count,
      };
      const key = machineKey(item);
      const existing = machines.get(key);
      if (existing) existing.count += item.count;
      else machines.set(key, item);
    }
    const models = [...machines.values()].sort((a, b) =>
      machineKey(a).localeCompare(machineKey(b)),
    );
    const provider = environment.provider.trim();
    const region = environment.region.trim();
    // Categories describe cloud + topology + models, not physical cluster identity.
    const scope = JSON.stringify([
      provider || environment.kind,
      provider ? "" : environment.id,
    ]);
    return {
      environment,
      models,
      scope,
      location: [provider || environment.kind, region]
        .filter(Boolean)
        .join(" / "),
      key: JSON.stringify([scope, models.map(configurationKey)]),
      client: models.some((m) => m.role === "测试客户端"),
    };
  });
  const groups = new Map<string, HardwareGroup>();
  for (const entry of entries) {
    let destination = entry;
    if (!entry.client) {
      // Link legacy server-only registrations only when client hardware is unambiguous.
      // Preserve the missing-client label; never invent hardware in the original snapshot.
      const candidates = new Map(
        entries
          .filter(
            (other) =>
              other.client &&
              other.scope === entry.scope &&
              JSON.stringify(
                other.models
                  .filter((m) => m.role !== "测试客户端")
                  .map(configurationKey),
              ) === JSON.stringify(entry.models.map(configurationKey)),
          )
          .map((other) => [other.key, other]),
      );
      if (candidates.size === 1) destination = [...candidates.values()][0];
    }
    let group = groups.get(destination.key);
    if (!group) {
      group = {
        key: destination.key,
        location: destination.location,
        provider:
          destination.environment.provider || destination.environment.kind,
        machines: destination.models.map((m) => ({
          role: m.role,
          model: m.model,
          counts: [],
        })),
        snapshots: [],
        incompleteSnapshotIds: [],
      };
      groups.set(group.key, group);
    }
    group.snapshots.push(entry.environment);
    if (!entry.client) group.incompleteSnapshotIds.push(entry.environment.id);
    for (const m of group.machines) {
      const actual = entry.models.find((x) => machineKey(x) === machineKey(m));
      if (actual && !m.counts.includes(actual.count))
        m.counts.push(actual.count);
    }
  }
  for (const group of groups.values()) {
    group.machines.forEach((m) => m.counts.sort((a, b) => a - b));
    group.snapshots.sort(
      (a, b) =>
        (b.config.observed_at || "").localeCompare(
          a.config.observed_at || "",
        ) || a.id.localeCompare(b.id),
    );
  }
  return [...groups.values()].sort((a, b) => a.key.localeCompare(b.key));
}

export function hardwareLabel(group: HardwareGroup): string {
  return `${group.provider} · ${group.machines.map((m) => `${m.role} ${m.counts[0]} × ${m.model}`).join(" / ")}`;
}
