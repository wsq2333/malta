package runner

import (
	"context"
	"github.com/wsq2333/malta/internal/artifact"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/store"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func scene(t *testing.T, mode string) domain.Plan {
	t.Helper()
	old := plan(t)
	var sc domain.ScenarioSpec
	if e := domain.Read("../../benchmarks/scenarios/mixed-data.yaml", &sc); e != nil {
		t.Fatal(e)
	}
	sc.Repeats = 1
	sc.Stages[0].Mode = mode
	sc.Stages[0].RuntimeSeconds = 2
	sc.Stages[0].WarmupSeconds = 0
	p, e := domain.NewScenarioPlan(sc, old.Environment, old.Version, old.Target, "scene")
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestRealScenario(t *testing.T) {
	bin := os.Getenv("MALTA_TEST_FIO")
	if bin == "" {
		t.Skip("requires fio 3.33")
	}
	for _, mode := range []string{"parallel", "sequential"} {
		t.Run(mode, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "out")
			r, e := Execute(context.Background(), domain.NewRun(scene(t, mode), "ci"), out, Options{Fio: bin, Timeout: 30 * time.Second})
			if e != nil {
				t.Fatal(e)
			}
			if len(r.Trials) != 2 || r.Status != "succeeded" {
				t.Fatal(r.Status)
			}
			b, e := artifact.Read(out)
			if e != nil {
				t.Fatal(e)
			}
			if e = store.VerifyEvidence(b); e != nil {
				t.Fatal(e)
			}
			b.Run.Trials[0].Metrics[0].Value++
			if store.VerifyEvidence(b) == nil {
				t.Fatal("tampered metrics accepted")
			}
		})
	}
}
func TestScenarioPreparationFailure(t *testing.T) {
	p := scene(t, "parallel")
	bin := filepath.Join(t.TempDir(), "fio")
	os.WriteFile(bin, []byte("#!/bin/sh\nif [ \"$1\" = --version ]; then echo fio-3.33; exit; fi\nexit 42\n"), 0700)
	out := filepath.Join(t.TempDir(), "out")
	r, e := Execute(context.Background(), domain.NewRun(p, "ci"), out, Options{Fio: bin, Timeout: 30 * time.Second})
	if e == nil || r.Status != "failed" {
		t.Fatal("failure not propagated")
	}
	b, e := artifact.Read(out)
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range b.Files {
		if filepath.Base(f.Path) == "measure-command.json" {
			t.Fatal("measurement started after failed preparation")
		}
	}
}
