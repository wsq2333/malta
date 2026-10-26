// Package native implements explicitly bounded synchronous operations for weighted scenarios.
package native

import (
	"fmt"
	"github.com/wsq2333/malta/internal/domain"
	"math"
	"slices"
	"time"
)

const Version = "malta-native-1"

type Counter struct {
	Operation string `json:"operation"`
	domain.Activity
	Bytes     uint64     `json:"bytes"`
	Histogram [65]uint64 `json:"histogram"`
}
type Evidence struct {
	Version     string       `json:"version"`
	StageHash   string       `json:"stage_hash"`
	Point       domain.Point `json:"point"`
	WindowStart time.Time    `json:"window_start"`
	WindowEnd   time.Time    `json:"window_end"`
	FirstIO     time.Time    `json:"first_io"`
	LastIO      time.Time    `json:"last_io"`
	Counters    []Counter    `json:"counters"`
}

func Parse(e Evidence, stage domain.Stage, p domain.Point) (domain.Trial, error) {
	t := domain.Trial{Point: p, Status: "succeeded", StartedAt: e.FirstIO, FinishedAt: e.LastIO, Activity: &domain.Activity{}, Metrics: []domain.Metric{}}
	if e.Version != Version || e.StageHash != domain.Hash(stage) || e.Point != p || e.WindowEnd.Sub(e.WindowStart) != time.Duration(stage.RuntimeSeconds)*time.Second || e.FirstIO.Before(e.WindowStart) || e.LastIO.After(e.WindowEnd) || !e.LastIO.After(e.FirstIO) {
		return t, domain.E("invalid", "native evidence identity or measurement window mismatch")
	}
	var w domain.ScenarioWorkload
	for _, v := range stage.Workloads {
		if v.ID == p.WorkloadID {
			w = v
		}
	}
	allowed := domain.NativeOperations(w)
	seen := map[string]bool{}
	secs := float64(stage.RuntimeSeconds)
	for _, c := range e.Counters {
		if seen[c.Operation] || !slices.Contains(allowed, c.Operation) || c.Submitted != c.Succeeded+c.Errors || c.Late > c.Succeeded || c.Errors != 0 {
			return t, domain.E("invalid", "native operation counters inconsistent")
		}
		seen[c.Operation] = true
		measured := c.Succeeded - c.Late
		var samples uint64
		for _, n := range c.Histogram {
			samples += n
		}
		if samples != measured {
			return t, domain.E("invalid", "native histogram sample count mismatch")
		}
		expectedBytes := uint64(0)
		if c.Operation == "read" || c.Operation == "write" {
			expectedBytes = measured * uint64(w.Workload.BlockSizeBytes)
		}
		if c.Bytes != expectedBytes {
			return t, domain.E("invalid", "native byte counter mismatch")
		}
		t.Activity.Submitted += c.Submitted
		t.Activity.Succeeded += c.Succeeded
		t.Activity.Errors += c.Errors
		t.Activity.Late += c.Late
		if measured == 0 && w.Kind == "pipeline" {
			return t, domain.E("invalid", "pipeline has an unmeasured step")
		}
		if measured == 0 {
			continue
		}
		add := func(name, unit string, value float64) {
			t.Metrics = append(t.Metrics, domain.Metric{Operation: c.Operation, Name: name, Unit: unit, Value: value, Source: Version})
		}
		if c.Operation == "read" || c.Operation == "write" {
			add("throughput", "bytes/s", float64(c.Bytes)/secs)
			add("iops", "ops/s", float64(measured)/secs)
		} else {
			metric := "metadata_rate"
			if c.Operation == "fsync" {
				metric = "operation_rate"
			}
			add(metric, "ops/s", float64(measured)/secs)
		}
		threshold := (measured*99 + 99) / 100
		var count uint64
		for bucket, n := range c.Histogram {
			count += n
			if count >= threshold {
				add("latency_p99_upper", "ns", math.Pow(2, float64(bucket))-1)
				break
			}
		}
	}
	if len(seen) != len(allowed) {
		return t, domain.E("invalid", "native operation counters missing")
	}
	if len(t.Metrics) == 0 {
		return t, domain.E("execution", fmt.Sprintf("no completed measurement operations for %s", p.WorkloadID))
	}
	return t, nil
}
