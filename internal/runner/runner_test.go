package runner

import (
	"context"
	"github.com/wsq2333/malta/internal/artifact"
	"github.com/wsq2333/malta/internal/domain"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func plan(t *testing.T) domain.Plan {
	t.Helper()
	var s domain.Spec
	var e domain.Environment
	var v domain.SoftwareVersion
	for _, x := range []struct {
		p string
		v any
	}{{"../../benchmarks/data/smoke-read.yaml", &s}, {"../../environments/local.yaml", &e}, {"../../environments/version-example.yaml", &v}} {
		if err := domain.Read(x.p, x.v); err != nil {
			t.Fatal(err)
		}
	}
	s.Repeats = 1
	p, err := domain.NewPlan(s, e, v, t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func TestTimeoutPreservesEvidence(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "fio")
	if e := os.WriteFile(bin, []byte("#!/bin/sh\nif [ \"$1\" = --version ]; then echo fio-3.33; exit; fi\nsleep 30 &\nwait\n"), 0700); e != nil {
		t.Fatal(e)
	}
	start := time.Now()
	r, e := Execute(context.Background(), domain.NewRun(plan(t), "human"), filepath.Join(dir, "out"), Options{Fio: bin, Timeout: time.Second})
	if e == nil || time.Since(start) > 5*time.Second || r.Status != "failed" {
		t.Fatalf("timeout result %s %v", r.Status, e)
	}
	b, e := artifact.Read(filepath.Join(dir, "out"))
	if e != nil {
		t.Fatal(e)
	}
	if b.Run.Status != "failed" || len(b.Files) < 4 {
		t.Fatalf("missing failure evidence: status=%s files=%d error=%s", b.Run.Status, len(b.Files), b.Run.Error)
	}
}
func TestBudgetBeforeIO(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	_, e := Execute(context.Background(), domain.NewRun(plan(t), "human"), out, Options{MaxBytes: 1})
	if e == nil {
		t.Fatal("expected budget rejection")
	}
	if _, e = os.Stat(out); !os.IsNotExist(e) {
		t.Fatal("budget rejection wrote output")
	}
}
func TestRealFio(t *testing.T) {
	bin := os.Getenv("MALTA_TEST_FIO")
	if bin == "" {
		t.Skip("set MALTA_TEST_FIO to enable actual fio smoke")
	}
	for _, op := range []string{"read", "write", "randread", "randwrite", "randrw"} {
		t.Run(op, func(t *testing.T) {
			p := plan(t)
			p.Spec.Workload.Operation = op
			p.Spec.Workload.ReadPercent = 70
			p.Hash = p.Digest()
			r, e := Execute(context.Background(), domain.NewRun(p, "ci"), filepath.Join(t.TempDir(), "out"), Options{Fio: bin, Timeout: 30 * time.Second})
			if e != nil {
				t.Fatal(e)
			}
			if r.Status != "succeeded" || len(r.Trials) != 1 {
				t.Fatalf("bad result: %+v", r)
			}
			expected := 3
			if op == "randrw" {
				expected = 6
			}
			if len(r.Trials[0].Metrics) != expected {
				t.Fatal(r.Trials[0].Metrics)
			}
		})
	}
}
