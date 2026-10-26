package domain

import (
	"fmt"
	"path/filepath"
	"regexp"
	"time"
)

const ScenarioSchema = "scenario/v1alpha1"

var scenarioID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

type ScenarioSpec struct {
	SchemaVersion string  `json:"schema_version" jsonschema:"enum=scenario/v1alpha1"`
	Name          string  `json:"name"`
	Repeats       int     `json:"repeats"`
	FailurePolicy string  `json:"failure_policy"`
	Stages        []Stage `json:"stages"`
}
type Stage struct {
	ID             string             `json:"id"`
	Mode           string             `json:"mode"`
	WarmupSeconds  int                `json:"warmup_seconds"`
	RuntimeSeconds int                `json:"runtime_seconds"`
	DatasetPolicy  string             `json:"dataset_policy"`
	Workloads      []ScenarioWorkload `json:"workloads"`
}
type ScenarioWorkload struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	NumJobs  int      `json:"numjobs"`
	IODepth  int      `json:"iodepth"`
	Workload Workload `json:"workload"`
}

func (s ScenarioSpec) Validate() error {
	if s.SchemaVersion != ScenarioSchema || s.Name == "" || s.Repeats < 1 || s.Repeats > 20 || s.FailurePolicy != "stop_all" || len(s.Stages) == 0 || len(s.Stages) > 16 {
		return E("invalid", "scenario requires version, name, 1..20 repeats, stop_all and 1..16 stages")
	}
	seen := map[string]bool{}
	count := 0
	for _, stage := range s.Stages {
		if !scenarioID.MatchString(stage.ID) || seen[stage.ID] {
			return E("invalid", "stage IDs must be unique lowercase identifiers (max 32 characters)")
		}
		seen[stage.ID] = true
		if stage.Mode != "sequential" && stage.Mode != "parallel" {
			return E("unsupported", "scenario mode is not supported")
		}
		if stage.DatasetPolicy != "isolated_per_workload" || len(stage.Workloads) == 0 || len(stage.Workloads) > 16 {
			return E("invalid", "require isolated datasets and 1..16 workloads per stage")
		}
		children := map[string]bool{}
		workers := 0
		for _, w := range stage.Workloads {
			if !scenarioID.MatchString(w.ID) || children[w.ID] {
				return E("invalid", "workload IDs must be unique lowercase identifiers within a stage")
			}
			children[w.ID] = true
			if w.Kind != "data" {
				return E("unsupported", "scenario currently supports data workloads")
			}
			if w.Workload.RuntimeSeconds != 0 || w.Workload.WarmupSeconds != 0 {
				return E("invalid", "duration belongs to the stage, not the workload")
			}
			spec := w.Spec(stage)
			if err := spec.Validate(); err != nil {
				return err
			}
			if w.Workload.CacheState == "cold-verified" {
				return E("unsupported", "cold caches cannot be verified")
			}
			workers += w.NumJobs
			count++
		}
		if workers > 256 {
			return E("invalid", "stage exceeds 256 total workers")
		}
	}
	if count*s.Repeats > 128 {
		return E("invalid", "scenario exceeds 128 workload trials")
	}
	return nil
}
func (w ScenarioWorkload) Spec(stage Stage) Spec {
	workload := w.Workload
	workload.RuntimeSeconds = stage.RuntimeSeconds
	workload.WarmupSeconds = stage.WarmupSeconds
	return Spec{SchemaVersion: Schema, Name: w.ID, Kind: w.Kind, Workload: workload, Matrix: Matrix{NumJobs: []int{w.NumJobs}, IODepth: []int{w.IODepth}}, Repeats: 1}
}
func (s ScenarioSpec) Points() []Point {
	points := []Point{}
	for repeat := 1; repeat <= s.Repeats; repeat++ {
		for _, stage := range s.Stages {
			for _, w := range stage.Workloads {
				points = append(points, Point{ID: fmt.Sprintf("r%d-%s-%s", repeat, stage.ID, w.ID), NumJobs: w.NumJobs, IODepth: w.IODepth, Repeat: repeat, StageID: stage.ID, WorkloadID: w.ID})
			}
		}
	}
	return points
}
func NewScenarioPlan(s ScenarioSpec, e Environment, v SoftwareVersion, target, group string) (Plan, error) {
	p := Plan{SchemaVersion: ScenarioSchema, Spec: Spec{SchemaVersion: ScenarioSchema, Name: s.Name, Kind: "scenario", Repeats: s.Repeats, Scenario: &s}, Environment: e, Version: v, Target: filepath.Clean(target), ResourceGroup: group, Trials: s.Points()}
	p.Hash = p.Digest()
	return p, p.Validate()
}

// WorkloadPlan resolves a point into the exact legacy fio parameter contract.
func (p Plan) WorkloadPlan(point Point) (Plan, error) {
	if p.Spec.Scenario == nil {
		return p, nil
	}
	for _, stage := range p.Spec.Scenario.Stages {
		if stage.ID == point.StageID {
			for _, w := range stage.Workloads {
				if w.ID == point.WorkloadID {
					child := p
					child.SchemaVersion = Schema
					child.Spec = w.Spec(stage)
					child.Trials = []Point{point}
					child.Hash = ""
					return child, nil
				}
			}
		}
	}
	return Plan{}, E("invalid", "scenario workload not found")
}

// Budget uses retained data across repeats, and sequential versus overlapping windows.
func (p Plan) Budget() (int64, int64) {
	var bytes, seconds int64
	for _, point := range p.Trials {
		child, _ := p.WorkloadPlan(point)
		w := child.Spec.Workload
		bytes += w.FileSizeBytes * int64(w.FilesPerJob) * int64(point.NumJobs)
	}
	if p.Spec.Scenario == nil {
		seconds = int64(len(p.Trials)) * int64(p.Spec.Workload.RuntimeSeconds+p.Spec.Workload.WarmupSeconds)
	} else {
		for _, stage := range p.Spec.Scenario.Stages {
			n := 1
			if stage.Mode == "sequential" {
				n = len(stage.Workloads)
			}
			seconds += int64(n) * int64(stage.RuntimeSeconds+stage.WarmupSeconds) * int64(p.Spec.Repeats)
		}
	}
	return bytes, seconds
}

// Require a shared observed interval covering at least half of the configured measurement.
func ValidateStageWindow(stage Stage, trials []Trial) error {
	if len(trials) != len(stage.Workloads) {
		return E("invalid", "incomplete scenario stage")
	}
	var start, end time.Time
	for _, t := range trials {
		if t.Status != "succeeded" || !t.FinishedAt.After(t.StartedAt) {
			return E("invalid", "incomplete observed measurement window")
		}
		if start.IsZero() || t.StartedAt.After(start) {
			start = t.StartedAt
		}
		if end.IsZero() || t.FinishedAt.Before(end) {
			end = t.FinishedAt
		}
	}
	if end.Sub(start) < time.Duration(stage.RuntimeSeconds)*time.Second/2 {
		return E("invalid", "parallel measurement windows overlap less than 50% of requested runtime")
	}
	return nil
}
