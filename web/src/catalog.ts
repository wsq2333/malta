import type { Baseline, Run } from "./model";
export function catalogRoute(params: URLSearchParams) {
  return {
    page:
      params.get("page") === "runs"
        ? "baselines"
        : params.get("page") || "baselines",
    view:
      params.get("page") === "runs" || params.get("view") === "all"
        ? "all"
        : "saved",
  };
}
export function baselineEntries(
  baselines: Baseline[],
  runs: Run[],
  filtered: boolean,
  knownRuns: Run[] = runs,
) {
  const index = new Map(runs.map((r) => [r.id, r]));
  return baselines
    .map((baseline) => ({ baseline, run: index.get(baseline.run_id) }))
    .filter(
      (entry) =>
        entry.run !== undefined ||
        (!filtered && !knownRuns.some((r) => r.id === entry.baseline.run_id)),
    );
}
