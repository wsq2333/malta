import test from "node:test";
import assert from "node:assert/strict";
import {
  workflowSummary,
  verifiedPerformanceResults,
} from "../src/workflowPresentation.ts";
const success = {
  status: "succeeded",
  execution_state: "succeeded",
  result_state: "complete",
  verdict: "passed",
};
test("execution success does not mask failed or missing evidence", () => {
  for (const result_state of ["invalid", "missing"])
    assert.equal(
      workflowSummary({
        ...success,
        status: "failed",
        result_state,
        verdict: "inconclusive",
      }).label,
      "失败",
    );
  assert.equal(workflowSummary(success).label, "成功");
  assert.equal(
    workflowSummary({ ...success, verdict: "failed" }).label,
    "失败",
  );
  assert.equal(
    workflowSummary({ ...success, status: "running", result_state: "pending" })
      .label,
    "运行中",
  );
  assert.equal(
    workflowSummary({ ...success, status: "cancelled" }).label,
    "已取消",
  );
  const native = workflowSummary({
    ...success,
    result_state: "not_configured",
    verdict: "not_evaluated",
  });
  assert.equal(native.label, "成功");
  assert.match(native.description, /未配置/);
});
test("overview shows only independently identified, verified performance stages", () => {
  const fio = {
    node_id: "read",
    parser: "fio-3.33",
    state: "complete",
    verdict: "passed",
    metrics: [{ value: 123 }],
  };
  const input = {
    ...success,
    results: [
      { ...fio, parser: "assertions-v1" },
      fio,
      { ...fio, node_id: "write" },
      { ...fio, state: "invalid" },
      { ...fio, metrics: [] },
      { ...fio, metrics: [{ value: NaN }] },
    ],
  };
  assert.deepEqual(
    verifiedPerformanceResults(input).map((r) => r.node_id),
    ["read", "write"],
  );
  assert.deepEqual(
    verifiedPerformanceResults({ ...input, status: "failed" }),
    [],
  );
  assert.deepEqual(
    verifiedPerformanceResults({ ...input, result_state: "invalid" }),
    [],
  );
  assert.deepEqual(
    verifiedPerformanceResults({ ...input, verdict: "failed" }),
    [],
  );
});
