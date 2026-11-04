type State = {
  status: string;
  execution_state: string;
  result_state: string;
  verdict: string;
};
export function workflowSummary(run: State) {
  if (run.status === "cancelled")
    return {
      label: "已取消",
      tone: "neutral",
      description: "本次测试已取消。",
    };
  if (!["succeeded", "failed"].includes(run.status))
    return {
      label: "运行中",
      tone: "active",
      description: "测试尚未结束，完成后将自动更新结果。",
    };
  if (
    run.status === "failed" ||
    run.result_state === "invalid" ||
    run.result_state === "missing" ||
    run.verdict === "failed"
  )
    return {
      label: "失败",
      tone: "failure",
      description:
        run.execution_state === "succeeded"
          ? "执行已完成，但结果未通过核验。可在详情中查看原因并重新核验。"
          : "测试未完成，请在详情中查看失败原因。",
    };
  if (run.result_state === "not_configured")
    return {
      label: "成功",
      tone: "success",
      description: "工作流执行成功；此用例未配置测试结果校验。",
    };
  if (run.result_state === "complete" && run.verdict === "passed")
    return {
      label: "成功",
      tone: "success",
      description: "测试已完成，结果核验通过。",
    };
  return {
    label: "失败",
    tone: "failure",
    description: "未获得完整、通过核验的测试结果。请查看详情。",
  };
}

export function verifiedPerformanceResults<
  T extends {
    parser: string;
    state: string;
    verdict: string;
    metrics: { value: number }[];
  },
>(run: State & { results: T[] }) {
  if (
    run.status !== "succeeded" ||
    run.result_state !== "complete" ||
    run.verdict !== "passed"
  )
    return [];
  return run.results.filter(
    (r) =>
      r.parser === "fio-3.33" &&
      r.state === "complete" &&
      r.verdict === "passed" &&
      r.metrics.length > 0 &&
      r.metrics.every((m) => Number.isFinite(m.value)),
  );
}
