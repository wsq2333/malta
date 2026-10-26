package native

import (
	"context"
	"github.com/wsq2333/malta/internal/domain"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWeightedScheduler(t *testing.T) {
	s := NewScheduler([]int{70, 20, 10}, 42)
	counts := make([]int, 3)
	for i := 0; i < 1000; i++ {
		counts[s.Next()]++
	}
	if counts[0] != 700 || counts[1] != 200 || counts[2] != 100 {
		t.Fatal(counts)
	}
}
func TestRealWeightedIO(t *testing.T) {
	var sc domain.ScenarioSpec
	var env domain.Environment
	var ver domain.SoftwareVersion
	for _, x := range []struct {
		path string
		out  any
	}{{"../../../benchmarks/scenarios/weighted-data.yaml", &sc}, {"../../../environments/local.yaml", &env}, {"../../../environments/version-example.yaml", &ver}} {
		if err := domain.Read(x.path, x.out); err != nil {
			t.Fatal(err)
		}
	}
	sc.Repeats = 1
	stage := &sc.Stages[0]
	stage.WarmupSeconds = 0
	stage.RuntimeSeconds = 1
	for i := range stage.Workloads {
		stage.Workloads[i].Workload.FileSizeBytes = 4096
		stage.Workloads[i].Workload.BlockSizeBytes = 4096
	}
	target := t.TempDir()
	out := t.TempDir()
	p, err := domain.NewScenarioPlan(sc, env, ver, target, "weighted")
	if err != nil {
		t.Fatal(err)
	}
	if err = Execute(context.Background(), Request{Plan: p, Repeat: 1, StageID: stage.ID, DataDir: target, OutDir: out}); err != nil {
		t.Fatal(err)
	}
	counts := []uint64{}
	var total uint64
	for _, point := range p.Trials {
		var e Evidence
		if err = domain.Read(filepath.Join(out, point.ID, "native.json"), &e); err != nil {
			t.Fatal(err)
		}
		trial, err := Parse(e, *stage, point)
		if err != nil {
			t.Fatal(err)
		}
		counts = append(counts, trial.Activity.Submitted)
		total += trial.Activity.Submitted
		e.Counters[0].Bytes++
		if _, err = Parse(e, *stage, point); err == nil {
			t.Fatal("tampered bytes accepted")
		}
		if _, err = os.Stat(filepath.Join(target, point.ID, "worker-0-file-0")); err != nil {
			t.Fatal(err)
		}
	}
	if total < 100 || math.Abs(float64(counts[0])/float64(total)-0.7) > 0.02 {
		t.Fatalf("unexpected submission mix: %v", counts)
	}
}

func TestMetadataAndPipeline(t *testing.T) {
	for _, name := range []string{"weighted-metadata", "pipeline"} {
		t.Run(name, func(t *testing.T) {
			var sc domain.ScenarioSpec
			var env domain.Environment
			var ver domain.SoftwareVersion
			for _, x := range []struct {
				path string
				out  any
			}{{"../../../benchmarks/scenarios/" + name + ".yaml", &sc}, {"../../../environments/local.yaml", &env}, {"../../../environments/version-example.yaml", &ver}} {
				if err := domain.Read(x.path, x.out); err != nil {
					t.Fatal(err)
				}
			}
			sc.Repeats = 1
			sc.Stages[0].RuntimeSeconds = 1
			sc.Stages[0].WarmupSeconds = 0
			for i := range sc.Stages[0].Workloads {
				sc.Stages[0].Workloads[i].Workload.FileSizeBytes = 4096
				sc.Stages[0].Workloads[i].Workload.BlockSizeBytes = 4096
			}
			target := t.TempDir()
			out := t.TempDir()
			p, err := domain.NewScenarioPlan(sc, env, ver, target, "native")
			if err != nil {
				t.Fatal(err)
			}
			if err = Execute(context.Background(), Request{Plan: p, Repeat: 1, StageID: sc.Stages[0].ID, DataDir: target, OutDir: out}); err != nil {
				t.Fatal(err)
			}
			for _, point := range p.Trials {
				var e Evidence
				if err = domain.Read(filepath.Join(out, point.ID, "native.json"), &e); err != nil {
					t.Fatal(err)
				}
				trial, err := Parse(e, sc.Stages[0], point)
				if err != nil {
					t.Fatal(err)
				}
				if trial.Activity.Submitted == 0 {
					t.Fatal("no operations")
				}
				if name == "pipeline" {
					if len(e.Counters) != 6 {
						t.Fatal("missing lifecycle steps")
					}
					for _, c := range e.Counters {
						if c.Succeeded == 0 {
							t.Fatal("unexecuted step", c.Operation)
						}
					}
					files, err := os.ReadDir(filepath.Join(target, point.ID))
					if err != nil || len(files) != 0 {
						t.Fatal("pipeline failed to unlink completed files", err)
					}
				}
			}
		})
	}
}
func TestPipelineStopsAfterCreateFailure(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "exists")
	os.WriteFile(file, []byte("keep"), 0600)
	f := &fileset{path: file, buffer: make([]byte, 4096)}
	defer f.close()
	seen := []string{}
	err := f.perform(domain.ScenarioWorkload{Kind: "pipeline", Steps: []string{"create", "write", "unlink"}}, func(op string, _ int, _ error, _, _ time.Time) { seen = append(seen, op) })
	if err == nil || len(seen) != 1 || seen[0] != "create" {
		t.Fatal("pipeline continued after failure", seen)
	}
	b, _ := os.ReadFile(file)
	if string(b) != "keep" {
		t.Fatal("existing file changed")
	}
}
