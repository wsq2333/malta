export interface Machine {
  role: string;
  model: string;
  count: number;
  cpu: string;
  memory_bytes: number;
  storage: string;
  network: string;
}
export interface Environment {
  schema_version: string;
  id: string;
  name: string;
  kind: string;
  provider: string;
  region: string;
  machines: Machine[];
  os: string;
  config: Record<string, string>;
}
export interface Version {
  schema_version: string;
  id: string;
  filesystem: string;
  label: string;
  commit: string;
  build: string;
}
export interface Workload {
  operation: string;
  file_size_bytes: number;
  files_per_job: number;
  block_size_bytes: number;
  read_percent: number;
  ioengine: string;
  direct: boolean;
  cache_state: string;
  fsync_every: number;
  runtime_seconds: number;
  warmup_seconds: number;
  seed: number;
}
export interface ScenarioWorkload {
  steps?: string[];
  id: string;
  kind: string;
  numjobs: number;
  iodepth: number;
  workload: Workload;
}
export interface ScenarioStage {
  mix?: { basis: string; weights: Record<string, number>; seed: number };
  id: string;
  mode: string;
  warmup_seconds: number;
  runtime_seconds: number;
  workloads: ScenarioWorkload[];
}
export interface Scenario {
  name: string;
  stages: ScenarioStage[];
  repeats: number;
}
export interface Spec {
  scenario?: Scenario;
  schema_version: string;
  name: string;
  kind: string;
  workload: Workload;
  matrix: { numjobs: number[]; iodepth: number[] };
  repeats: number;
  metadata?: Record<string, string>;
}
export interface Metric {
  operation: string;
  name: string;
  unit: string;
  value: number;
  source: string;
}
export interface Trial {
  point: {
    id: string;
    numjobs: number;
    iodepth: number;
    repeat: number;
    stage_id?: string;
    workload_id?: string;
  };
  activity?: {
    submitted: number;
    succeeded: number;
    errors: number;
    late: number;
  };
  started_at?: string;
  finished_at?: string;
  status: string;
  metrics: Metric[];
  error?: string;
}
export interface Plan {
  schema_version: string;
  spec: Spec;
  environment: Environment;
  version: Version;
  target: string;
  resource_group: string;
  trials: Trial["point"][];
  hash: string;
}
export interface Run {
  case_ref?: { id: string; revision: number };
  id: string;
  request_id: string;
  source: string;
  status: string;
  created_at: string;
  updated_at: string;
  plan: Plan;
  attempt: {
    id: string;
    status: string;
    executor: string;
    result_state: string;
  };
  tool_version: string;
  malta_version: string;
  trials: Trial[];
  error?: string;
}
export interface Baseline {
  id: string;
  name: string;
  run_id: string;
  created_at: string;
}
export interface Summary {
  stage_id?: string;
  workload_id?: string;
  key: string;
  numjobs: number;
  iodepth: number;
  operation: string;
  metric: string;
  unit: string;
  count: number;
  median: number;
  min: number;
  max: number;
}
export interface Comparison {
  baseline: string;
  candidate: string;
  comparable: boolean;
  differences: { field: string; reason: string }[];
  warnings: string[];
  metrics: {
    baseline: Summary;
    candidate: Summary;
    percent: number | null;
    direction: string;
  }[];
}
export interface Trend {
  points: { run: Run; metrics: Summary[] }[];
  limit: number;
  scope: string;
}
export const terminal = (s: string) =>
  ["succeeded", "failed", "cancelled", "interrupted"].includes(s);
export const statusLabel: Record<string, string> = {
  pending: "排队中",
  preparing: "准备数据",
  running: "测试中",
  cancelling: "取消中",
  succeeded: "已完成",
  failed: "失败",
  cancelled: "已取消",
  interrupted: "已中断",
};
export const operationLabel: Record<string, string> = {
  read: "顺序读",
  write: "顺序写",
  randread: "随机读",
  randwrite: "随机写",
  randrw: "混合读写",
};
export const metricLabel: Record<string, string> = {
  throughput: "吞吐",
  iops: "IOPS",
  operation_rate: "操作速率",
  latency_p99_upper: "原生操作 P99 上界",
  clat_p99: "每次测试 P99",
  metadata_rate: "操作速率",
};
export function format(value: number | undefined, unit: string): string {
  if (value === undefined || !Number.isFinite(value)) return "未采集";
  if (unit === "bytes/s")
    return `${(value / 1048576).toLocaleString("en-US", { maximumFractionDigits: 2 })} MiB/s`;
  if (unit === "ns")
    return `${(value / 1000).toLocaleString("en-US", { maximumFractionDigits: 2 })} μs`;
  return `${value.toLocaleString("en-US", { maximumFractionDigits: 1 })} ${unit}`;
}
export function size(n: number): string {
  if (!n) return "—";
  for (const [u, v] of [
    ["GiB", 1073741824],
    ["MiB", 1048576],
    ["KiB", 1024],
  ] as const)
    if (n >= v) return `${+(n / v).toFixed(2)} ${u}`;
  return `${n} B`;
}
export function date(s: string): string {
  return new Date(s).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}
export function summarize(r: Run): Summary[] {
  const groups = new Map<string, { s: Summary; v: number[] }>();
  for (const t of r.trials) {
    if (t.status !== "succeeded") continue;
    for (const m of t.metrics) {
      const prefix = t.point.stage_id
        ? `${t.point.stage_id}/${t.point.workload_id}/`
        : "";
      const key = `${prefix}n${t.point.numjobs}-d${t.point.iodepth}/${m.operation}/${m.name}`;
      let g = groups.get(key);
      if (!g) {
        g = {
          s: {
            key,
            stage_id: t.point.stage_id,
            workload_id: t.point.workload_id,
            numjobs: t.point.numjobs,
            iodepth: t.point.iodepth,
            operation: m.operation,
            metric: m.name,
            unit: m.unit,
            count: 0,
            median: 0,
            min: 0,
            max: 0,
          },
          v: [],
        };
        groups.set(key, g);
      }
      g.v.push(m.value);
    }
  }
  return [...groups.values()]
    .map(({ s, v }) => {
      v.sort((a, b) => a - b);
      const n = v.length;
      return {
        ...s,
        count: n,
        min: v[0],
        max: v[n - 1],
        median: n % 2 ? v[Math.floor(n / 2)] : (v[n / 2 - 1] + v[n / 2]) / 2,
      };
    })
    .sort((a, b) => a.key.localeCompare(b.key));
}
export function peak(r: Run): Summary | undefined {
  return summarize(r)
    .filter(
      (s) =>
        s.metric ===
        (r.plan.spec.kind === "data" ? "throughput" : "metadata_rate"),
    )
    .sort((a, b) => b.median - a.median)[0];
}
export async function api<T>(path: string, body?: unknown): Promise<T> {
  const token = sessionStorage.getItem("malta-token") || "";
  const response = await fetch("/api/v1" + path, {
    method: body === undefined ? "GET" : "POST",
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const envelope = await response.json();
  if (!response.ok || envelope.error)
    throw new Error(envelope.error?.message || `HTTP ${response.status}`);
  if (envelope.schema_version !== "1.0")
    throw new Error("服务 schema 版本不兼容");
  return envelope.data as T;
}
export async function downloadBundle(id: string) {
  const bundle = await api<unknown>(`/runs/${id}/bundle`);
  const url = URL.createObjectURL(
    new Blob([JSON.stringify(bundle, null, 2)], { type: "application/json" }),
  );
  const link = document.createElement("a");
  link.href = url;
  link.download = `${id}-bundle.json`;
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
