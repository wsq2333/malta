package store

import (
	"github.com/wsq2333/malta/internal/domain"
	"sync"
	"testing"
)

func TestCasesHistoryConflictPersistence(t *testing.T) {
	dir := t.TempDir()
	s, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	q := domain.SaveCase{Name: "随机读", Description: "原始描述", Plan: sample(t).Run.Plan}
	v1, e := s.SaveCase("", q)
	if e != nil {
		t.Fatal(e)
	}
	q.BaseRevision = 1
	q.Description = "新描述"
	q.Plan.Spec.Repeats = 2
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.SaveCase(v1.CaseID, q); results <- err }()
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("concurrent writes: %d succeeded", wins)
	}
	old, e := s.CaseVersion(v1.CaseID, 1)
	if e != nil || domain.Hash(old) != domain.Hash(v1) {
		t.Fatal("history changed", e)
	}
	latest, e := s.CaseVersion(v1.CaseID, 2)
	if e != nil || latest.Plan.Hash == v1.Plan.Hash || latest.Plan.Validate() != nil || len(latest.Plan.Trials) != 2 {
		t.Fatal("plan not rebuilt", e)
	}
	q.BaseRevision = 2
	q.Plan.Spec.Kind = "invalid"
	if _, e = s.SaveCase(v1.CaseID, q); e == nil {
		t.Fatal("invalid plan saved")
	}
	s.Close()
	s, e = Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	cases, e := s.Cases()
	if e != nil || len(cases) != 1 || cases[0].Latest.Revision != 2 || !cases[0].CreatedAt.Equal(v1.CreatedAt) {
		t.Fatal(cases, e)
	}
	history, e := s.CaseVersions(v1.CaseID)
	if e != nil || len(history) != 2 {
		t.Fatal(history, e)
	}
}
func TestScenarioCase(t *testing.T) {
	s, e := Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	p := sample(t).Run.Plan
	var scenario domain.ScenarioSpec
	if e = domain.Read("../../benchmarks/scenarios/weighted-data.yaml", &scenario); e != nil {
		t.Fatal(e)
	}
	p, e = domain.NewScenarioPlan(scenario, p.Environment, p.Version, p.Target, p.ResourceGroup)
	if e != nil {
		t.Fatal(e)
	}
	p.Spec.Scenario.Repeats = 2
	q := domain.SaveCase{Name: "混合", Plan: p}
	v, e := s.SaveCase("", q)
	if e != nil || v.Plan.Spec.Repeats != 2 || v.Plan.Validate() != nil {
		t.Fatal("scenario changed", e)
	}
}
