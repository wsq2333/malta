import test from "node:test";
import assert from "node:assert/strict";
import { catalogRoute, baselineEntries } from "../src/catalog.ts";

test("legacy run links resolve to all tests, while the landing defaults to all", () => {
  assert.deepEqual(catalogRoute(new URLSearchParams("page=runs&view=saved")), {
    page: "baselines",
    view: "all",
  });
  assert.deepEqual(catalogRoute(new URLSearchParams("page=baselines")), {
    page: "baselines",
    view: "all",
  });
  assert.equal(
    catalogRoute(new URLSearchParams("page=baselines&view=all")).view,
    "all",
  );
});
test("named baselines keep distinct names even when they share one run", () => {
  const b = [
    { id: "b1", name: "release", run_id: "r1" },
    { id: "b2", name: "weekly", run_id: "r1" },
  ];
  assert.equal(baselineEntries(b, [{ id: "r1" }], false).length, 2);
});
test("baseline cards distinguish an unloaded record from a known record excluded by workload filters", () => {
  const b = [
    { id: "b1", run_id: "r1" },
    { id: "b2", run_id: "r2" },
    { id: "b3", run_id: "r3" },
  ];
  assert.deepEqual(
    baselineEntries(b, [{ id: "r1" }], false, [{ id: "r1" }, { id: "r2" }]).map(
      (e) => e.baseline.id,
    ),
    ["b1", "b3"],
  );
  assert.deepEqual(
    baselineEntries(b, [{ id: "r1" }], true, [{ id: "r1" }, { id: "r2" }]).map(
      (e) => e.baseline.id,
    ),
    ["b1"],
  );
});
