import test from "node:test";
import assert from "node:assert/strict";
import {
  workspaceNavigation,
  workspaceSection,
  workspaceTabs,
} from "../src/workspace.ts";
test("all existing and prototype routes belong to five workspace sections", () => {
  assert.equal(workspaceNavigation.length, 5);
  const roots = new Set(workspaceNavigation.map((p) => p.page));
  for (const page of [
    "baselines",
    "runs",
    "trends",
    "automation",
    "cases",
    "suites",
    "issues",
    "environments",
    "versions",
    "guide",
  ])
    assert.ok(roots.has(workspaceSection(page)), page);
  assert.equal(workspaceSection("runs"), "baselines");
  assert.equal(workspaceSection("trends"), "baselines");
  assert.equal(workspaceSection("versions"), "environments");
});
test("comparison remains within performance and issues have no extra navigation layer", () => {
  assert.deepEqual(workspaceTabs("trends"), workspaceTabs("baselines"));
  assert.deepEqual(workspaceTabs("issues"), []);
  assert.deepEqual(
    workspaceTabs("suites").map((p) => p.page),
    ["cases", "suites"],
  );
});
