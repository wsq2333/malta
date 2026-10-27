package store

import (
	"github.com/wsq2333/malta/internal/artifact"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/engine/metadata"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sample(t *testing.T) artifact.Bundle {
	t.Helper()
	var spec domain.Spec
	var env domain.Environment
	var ver domain.SoftwareVersion
	for _, x := range []struct {
		p string
		v any
	}{{"../../benchmarks/metadata/mdtest-files.yaml", &spec}, {"../../environments/local.yaml", &env}, {"../../environments/version-example.yaml", &ver}} {
		if e := domain.Read(x.p, x.v); e != nil {
			t.Fatal(e)
		}
	}
	p, e := domain.NewPlan(spec, env, ver, "/tmp/mdtest", "metadata")
	if e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile("../../testdata/mdtest/mdtest-4.0.0.txt")
	if e != nil {
		t.Fatal(e)
	}
	b, e := metadata.Import(p, [][]byte{raw})
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestImportRoundtripAndConflicts(t *testing.T) {
	s, e := Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	b := sample(t)
	if e = s.Import(b); e != nil {
		t.Fatal(e)
	}
	if e = s.Import(b); e != nil {
		t.Fatal(e)
	}
	runs, e := s.Runs(Filter{})
	if e != nil || len(runs) != 1 {
		t.Fatal(runs, e)
	}
	baseline, e := s.SetBaseline("mdtest", b.Run.ID)
	if e != nil || baseline.RunID != b.Run.ID {
		t.Fatal(e)
	}
	out, e := s.Bundle(b.Run.ID)
	if e != nil || artifact.Digest(out) != artifact.Digest(b) {
		t.Fatal("bundle changed", e)
	}
	dir := filepath.Join(t.TempDir(), "export")
	if e = artifact.Write(dir, out); e != nil {
		t.Fatal(e)
	}
	copyB, e := artifact.Read(dir)
	if e != nil || artifact.Digest(copyB) != artifact.Digest(b) {
		t.Fatal("export changed", e)
	}
	b.Run.Error = "changed"
	if e = s.Import(b); e == nil {
		t.Fatal("accepted conflicting run")
	}
	env := b.Run.Plan.Environment
	env.Name = "changed"
	if e = s.Register("environment", env.ID, env); e == nil {
		t.Fatal("accepted mutable snapshot")
	}
	if _, e = s.Runs(Filter{From: "bad-date"}); e == nil {
		t.Fatal("accepted invalid date")
	}
}
func TestCompareConditionsAndZero(t *testing.T) {
	a := sample(t).Run
	b := sample(t).Run
	b.Plan.Version.ID = "candidate"
	b.Plan.Version.Label = "candidate"
	b.Plan.Hash = b.Plan.Digest()
	c := domain.Compare(a, b)
	if !c.Comparable || len(c.Metrics) != 3 || len(c.Warnings) == 0 {
		t.Fatalf("%+v", c)
	}
	a.Trials[0].Metrics[0].Value = 0
	c = domain.Compare(a, b)
	if c.Metrics[0].Percent != nil {
		t.Fatal("zero baseline produced a percentage")
	}
	b.Plan.Spec.Metadata = map[string]string{"source": "mdtest-4.0.0", "mode": "files", "tasks": "1", "files_per_task": "16"}
	if domain.Compare(a, b).Comparable {
		t.Fatal("different workload marked comparable")
	}
	b = sample(t).Run
	b.Status = "failed"
	if domain.Compare(a, b).Comparable {
		t.Fatal("failed run marked comparable")
	}
	b = sample(t).Run
	b.Plan.Environment.Machines[0].Model = "other"
	if domain.Compare(a, b).Comparable {
		t.Fatal("hardware mismatch accepted")
	}
	b = sample(t).Run
	b.CreatedAt = time.Now().Add(-time.Hour)
	if !domain.Compare(sample(t).Run, b).Comparable {
		t.Fatal("date alone blocked comparison")
	}
}

func TestRecoverDurableBundleBeforeDatabaseCommit(t *testing.T) {
	s, e := Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	original := sample(t)
	r, e := s.CreateManaged(original.Run.Plan, "crash-window", "ci", "kubernetes")
	if e != nil {
		t.Fatal(e)
	}
	original.Run.ID = r.ID
	original.Run.RequestID = r.RequestID
	original.Run.Source = r.Source
	original.Run.Attempt.ID = r.Attempt.ID
	original.Run.Attempt.Executor = "kubernetes"
	if e = artifact.Write(filepath.Join(s.Dir, "artifacts", r.ID), original); e != nil {
		t.Fatal(e)
	}
	if e = s.Recover(); e != nil {
		t.Fatal(e)
	}
	recovered, e := s.Run(r.ID)
	if e != nil || recovered.Status != "succeeded" {
		t.Fatalf("%s %v", recovered.Status, e)
	}
	if _, e = s.Bundle(r.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.Recover(); e != nil {
		t.Fatal(e)
	}
}
