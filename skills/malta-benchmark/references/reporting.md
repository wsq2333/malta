# Evidence-based reports

Use this compact structure (free-form prose or JSON is acceptable):

- **Identity:** baseline/candidate run IDs, version IDs and labels, environment
  IDs, plan hashes, tool and Malta versions, dates and executor.
- **Validity:** terminal status, result_state, comparable flag, differences and
  warnings, repeats and missing measurements.
- **Facts:** exact operation/metric/unit/numjobs/iodepth, baseline and candidate
  medians, Min/Max and delta returned by `compare`; relevant raw artifact path
  such as `n1-d1-r1/measure.json`. Link `/api/v1/runs/RUN/artifacts/PATH` when useful.
- **Hypotheses:** possible explanation, confidence and specific missing evidence.
  A throughput delta alone cannot identify cache, network or filesystem causality.
- **Next experiment:** one controlled change, expected observation, approved
  capacity/time budget and stop condition. Proposal does not authorize execution.

P99 here is a median of per-trial P99s, not pooled P99. Mixed reads and writes are
separate metrics. The highest matrix point is a measured peak, not an absolute
hardware limit. Unknown cache state remains unknown. Metadata imports provide
create/stat/remove operation rates; absent latency cannot be inferred.

Do not alter normalized measurements, baseline records or existing artifacts to
fit a narrative. Model-generated summaries are derived reports and should retain
model/prompt version plus evidence references when stored by an external system.
