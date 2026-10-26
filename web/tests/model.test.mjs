import test from "node:test";
import assert from "node:assert/strict";
import { summarize, format, peak } from "../src/model.ts";
test("independent repeats preserve mixed read/write and ignore failed trials", () => {
  const trial = (repeat, status, read, write) => ({
    point: { id: `r${repeat}`, numjobs: 2, iodepth: 4, repeat },
    status,
    metrics: [
      { operation: "read", name: "throughput", unit: "bytes/s", value: read },
      { operation: "write", name: "throughput", unit: "bytes/s", value: write },
    ],
  });
  const run = {
    plan: { spec: { kind: "data" } },
    trials: [
      trial(1, "succeeded", 10, 100),
      trial(2, "succeeded", 30, 200),
      trial(3, "succeeded", 20, 300),
      trial(4, "failed", 9999, 9999),
    ],
  };
  const data = summarize(run);
  assert.equal(data.length, 2);
  assert.equal(data[0].median, 20);
  assert.equal(data[0].min, 10);
  assert.equal(data[0].max, 30);
  assert.equal(data[0].count, 3);
  assert.equal(data[1].median, 200);
  assert.equal(peak(run).operation, "write");
});
test("missing values remain missing, while measured zero stays zero", () => {
  assert.equal(format(undefined, "ops/s"), "未采集");
  assert.equal(format(0, "ops/s"), "0 ops/s");
  assert.equal(format(1048576, "bytes/s"), "1 MiB/s");
  assert.equal(format(1000, "ns"), "1 μs");
});
test("scenario workloads with identical concurrency stay separate",()=>{
 const trials=["reader-a","reader-b"].flatMap((id,i)=>[1,2].map(repeat=>({point:{id:`${id}-${repeat}`,stage_id:"mix",workload_id:id,numjobs:1,iodepth:1,repeat},status:"succeeded",metrics:[{operation:"read",name:"throughput",unit:"bytes/s",value:10+i*90}]})));
 const summary=summarize({trials});assert.equal(summary.length,2);assert.deepEqual(summary.map(s=>s.count),[2,2]);assert.deepEqual(summary.map(s=>s.median),[10,100]);assert.equal(summary[0].stage_id,"mix");
});
