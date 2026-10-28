import { useEffect, useRef } from "react";
import { init, use, type EChartsCoreOption } from "echarts/core";
import { LineChart } from "echarts/charts";
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
} from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";
use([
  LineChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  CanvasRenderer,
]);
export function Chart({
  option,
  label,
}: {
  option: EChartsCoreOption;
  label: string;
}) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!ref.current) return;
    const chart = init(ref.current);
    chart.setOption(option);
    const applyTheme = () => {
      const styles = getComputedStyle(document.documentElement);
      const color = (name: string) => styles.getPropertyValue(name).trim();
      const axis = {
        axisLine: { lineStyle: { color: color("--line") } },
        axisLabel: { color: color("--text-muted") },
        nameTextStyle: { color: color("--text-secondary") },
        splitLine: { lineStyle: { color: color("--line") } },
      };
      chart.setOption({
        color: ["--accent", "--chart-2", "--chart-3", "--chart-4"].map(color),
        textStyle: { color: color("--text-secondary") },
        legend: { textStyle: { color: color("--text-secondary") } },
        tooltip: {
          backgroundColor: color("--surface-raised"),
          borderColor: color("--line"),
          textStyle: { color: color("--text") },
        },
        xAxis: axis,
        yAxis: axis,
      });
    };
    applyTheme();
    const themeObserver = new MutationObserver(applyTheme);
    themeObserver.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["data-theme"],
    });
    const resize = new ResizeObserver(() => chart.resize());
    resize.observe(ref.current);
    return () => {
      resize.disconnect();
      themeObserver.disconnect();
      chart.dispose();
    };
  }, [option]);
  return <div ref={ref} className="chart" role="img" aria-label={label} />;
}
