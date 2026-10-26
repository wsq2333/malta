package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

type Baseline struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	RunID     string    `json:"run_id"`
	CreatedAt time.Time `json:"created_at"`
}
type Summary struct {
	StageID    string  `json:"stage_id,omitempty"`
	WorkloadID string  `json:"workload_id,omitempty"`
	Key        string  `json:"key"`
	NumJobs    int     `json:"numjobs"`
	IODepth    int     `json:"iodepth"`
	Operation  string  `json:"operation"`
	Metric     string  `json:"metric"`
	Unit       string  `json:"unit"`
	Count      int     `json:"count"`
	Median     float64 `json:"median"`
	Min        float64 `json:"min"`
	Max        float64 `json:"max"`
}
type Difference struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}
type Delta struct {
	Baseline  Summary  `json:"baseline"`
	Candidate Summary  `json:"candidate"`
	Percent   *float64 `json:"percent"`
	Direction string   `json:"direction"`
}
type Comparison struct {
	Baseline    string       `json:"baseline"`
	Candidate   string       `json:"candidate"`
	Comparable  bool         `json:"comparable"`
	Differences []Difference `json:"differences"`
	Warnings    []string     `json:"warnings"`
	Metrics     []Delta      `json:"metrics"`
}

func Summarize(r Run) []Summary {
	groups := map[string][]float64{}
	info := map[string]Summary{}
	for _, t := range r.Trials {
		if t.Status != "succeeded" {
			continue
		}
		for _, m := range t.Metrics {
			k := fmt.Sprintf("n%d-d%d/%s/%s", t.Point.NumJobs, t.Point.IODepth, m.Operation, m.Name)
			if t.Point.StageID != "" {
				k = t.Point.StageID + "/" + t.Point.WorkloadID + "/" + k
			}
			groups[k] = append(groups[k], m.Value)
			info[k] = Summary{StageID: t.Point.StageID, WorkloadID: t.Point.WorkloadID, Key: k, NumJobs: t.Point.NumJobs, IODepth: t.Point.IODepth, Operation: m.Operation, Metric: m.Name, Unit: m.Unit}
		}
	}
	out := []Summary{}
	for k, v := range groups {
		sort.Float64s(v)
		s := info[k]
		s.Count = len(v)
		s.Min = v[0]
		s.Max = v[len(v)-1]
		s.Median = v[len(v)/2]
		if len(v)%2 == 0 {
			s.Median = (v[len(v)/2-1] + v[len(v)/2]) / 2
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
func Compare(a, b Run) Comparison {
	c := Comparison{Baseline: a.ID, Candidate: b.ID, Comparable: true, Differences: []Difference{}, Warnings: []string{}, Metrics: []Delta{}}
	diff := func(field, reason string) {
		c.Comparable = false
		c.Differences = append(c.Differences, Difference{field, reason})
	}
	if a.Status != "succeeded" || b.Status != "succeeded" {
		diff("status", "both runs must succeed")
	}
	if a.Plan.Spec.Kind != b.Plan.Spec.Kind {
		diff("kind", "different workload kinds cannot be compared")
	}
	sa, sb := a.Plan.Spec, b.Plan.Spec
	if sa.Scenario != nil {
		aCopy := *sa.Scenario
		aCopy.Name = ""
		sa.Scenario = &aCopy
	}
	if sb.Scenario != nil {
		bCopy := *sb.Scenario
		bCopy.Name = ""
		sb.Scenario = &bCopy
	}
	sa.Name = ""
	sb.Name = ""
	if Hash(sa) != Hash(sb) {
		diff("spec", "workload, matrix, repeat count or metadata conditions differ")
	}
	ea, eb := a.Plan.Environment, b.Plan.Environment
	ea.ID = ""
	ea.Name = ""
	eb.ID = ""
	eb.Name = ""
	if Hash(ea) != Hash(eb) {
		diff("environment", "hardware, topology, OS or configuration differs")
	}
	if a.Plan.Version.Filesystem != b.Plan.Version.Filesystem {
		diff("filesystem", "filesystem identities differ")
	}
	if a.ToolVersion == "" || a.ToolVersion != b.ToolVersion {
		diff("tool_version", "tool versions missing or different")
	}
	if a.MaltaVersion != b.MaltaVersion {
		diff("malta_version", "normalizer versions differ")
	}
	if strings.Contains(strings.ToLower(string(mustJSON(ea))), "unknown") {
		c.Warnings = append(c.Warnings, "Hardware snapshot contains unknown fields; verify environment before interpreting changes.")
	}
	if a.Plan.Spec.Kind == "data" && (a.Plan.Spec.Workload.CacheState == "unknown" || a.Plan.Spec.Workload.CacheState == "cold-requested") {
		c.Warnings = append(c.Warnings, "Cache state has not been verified.")
	}
	if a.Plan.Spec.Repeats < 3 || b.Plan.Spec.Repeats < 3 {
		c.Warnings = append(c.Warnings, "Fewer than three independent repeats; this is insufficient for a stable baseline.")
	}
	am, bm := Summarize(a), Summarize(b)
	lookup := map[string]Summary{}
	for _, s := range bm {
		lookup[s.Key] = s
	}
	if len(am) != len(bm) {
		diff("metrics", "metric sets differ")
	}
	for _, s := range am {
		v, ok := lookup[s.Key]
		if !ok || s.Unit != v.Unit || s.Count != a.Plan.Spec.Repeats || v.Count != b.Plan.Spec.Repeats {
			diff("metrics", s.Key+" missing or incomplete")
		}
	}
	if !c.Comparable {
		return c
	}
	for _, s := range am {
		v := lookup[s.Key]
		d := Delta{Baseline: s, Candidate: v, Direction: "higher-is-better"}
		if s.Metric == "clat_p99" {
			d.Direction = "lower-is-better"
		}
		if s.Median != 0 {
			x := (v.Median - s.Median) / s.Median * 100
			d.Percent = &x
		}
		c.Metrics = append(c.Metrics, d)
	}
	return c
}
