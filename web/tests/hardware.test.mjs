import test from "node:test";
import assert from "node:assert/strict";
import { groupHardware } from "../src/hardware.ts";

const server = {
  role: "server (co-located master+worker)",
  model: "ecs.g8i.2xlarge",
  count: 1,
};
const client = {
  role: "benchmark client (separate from Curvine servers)",
  model: "ecs.g8i.2xlarge",
  count: 1,
};
const snapshot = (
  id,
  config = {},
  machines = [server, server, server, client],
) => ({
  schema_version: "1.0",
  id,
  name: id,
  kind: "kubernetes",
  provider: "Alibaba Cloud",
  region: "cn-wulanchabu",
  machines,
  os: "Linux",
  config,
});

test("runtime identity, quota and image changes stay in one hardware category without changing snapshots", () => {
  const input = [
    snapshot("non-root", { runner_uid_gid: "10001:10001" }),
    snapshot("root", { runner_uid_gid: "0:0" }),
    snapshot("quota", {
      runner_cpu_request_millis: "950",
      runner_image: "new-image",
    }),
    snapshot("initial", {}, [server, server, server]),
  ];
  const original = JSON.stringify(input);
  const groups = groupHardware(input);
  assert.equal(groups.length, 1);
  assert.equal(groups[0].snapshots.length, 4);
  assert.deepEqual(groups[0].incompleteSnapshotIds, ["initial"]);
  assert.deepEqual(
    groups[0].machines.find((m) => m.role.includes("共置")).counts,
    [3],
  );
  assert.deepEqual(
    groups[0].machines.find((m) => m.role === "测试客户端").counts,
    [1],
  );
  assert.equal(JSON.stringify(input), original);
  assert.equal(
    JSON.stringify(groupHardware(input.toReversed())),
    JSON.stringify(groups),
  );
});

test("different client models stay distinct and an ambiguous server-only snapshot stays unassigned", () => {
  const groups = groupHardware([
    snapshot("a"),
    snapshot("b", {}, [
      server,
      server,
      server,
      { ...client, model: "ecs.g8i.4xlarge" },
    ]),
    snapshot("initial", {}, [server, server, server]),
  ]);
  assert.equal(groups.length, 3);
  assert.equal(
    groups.find((g) => g.incompleteSnapshotIds.includes("initial")).snapshots
      .length,
    1,
  );
});

test("regions share a hardware category, while provider, roles and unknown cloud identity remain distinct", () => {
  const groups = groupHardware([
    snapshot("a"),
    { ...snapshot("region"), region: "another-region" },
    { ...snapshot("provider"), provider: "other-cloud" },
    snapshot("role", {}, [{ ...server, role: "worker" }, client]),
    { ...snapshot("unknown-a"), provider: "" },
    { ...snapshot("unknown-b"), provider: "" },
  ]);
  assert.equal(groups.length, 5);
});

test("node counts define separate hardware categories, while row ordering does not multiply machines", () => {
  const groups = groupHardware([
    snapshot("a", {}, [{ ...server, count: 3 }, client]),
    snapshot("b", {}, [client, server, server, server]),
    snapshot("c", {}, [{ ...server, count: 6 }, client]),
  ]);
  assert.equal(groups.length, 2);
  assert.equal(groups[0].machines.length, 2);
  assert.deepEqual(
    groups
      .find((g) => g.snapshots.some((s) => s.id === "a"))
      .machines.find((m) => m.role.includes("共置")).counts,
    [3],
  );
});
