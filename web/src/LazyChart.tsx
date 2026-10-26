import { lazy, Suspense } from "react";
import type { EChartsCoreOption } from "echarts/core";
const Loaded = lazy(() =>
  import("./Chart").then((m) => ({ default: m.Chart })),
);
export function Chart(props: { option: EChartsCoreOption; label: string }) {
  return (
    <Suspense fallback={<div className="chart empty">正在加载图表…</div>}>
      <Loaded {...props} />
    </Suspense>
  );
}
