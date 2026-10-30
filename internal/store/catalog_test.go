package store

import (
	"context"
	"fmt"
	"github.com/wsq2333/malta/internal/domain"
	"testing"
	"time"
)

func TestCatalogFullHistoryAndConditions(t *testing.T) {
	s, e := Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	base := sample(t).Run
	base.CaseRef = &domain.CaseRef{ID: "case-catalog", Revision: 1}
	base.Attempt.ResultState = "complete"
	tx, e := s.DB.Begin()
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 525; i++ {
		r := base
		r.ID = fmt.Sprintf("run-%04d", i)
		r.CreatedAt = time.Date(2026, 1, 1, 0, i, 0, 0, time.UTC)
		r.Plan.Version.ID = fmt.Sprintf("v-%d", i%2)
		r.Plan.Version.Label = r.Plan.Version.ID
		if i == 524 {
			r.Status = "failed"
		}
		if _, e = tx.Exec("INSERT INTO runs(id,body,created_at) VALUES(?,?,?)", r.ID, encode(r), r.CreatedAt.Format(time.RFC3339Nano)); e != nil {
			t.Fatal(e)
		}
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	f := CatalogFilter{Mode: "overview", Limit: 20}
	out, e := s.Catalog(context.Background(), f)
	if e != nil {
		t.Fatal(e)
	}
	if out.Total != 1 || out.RunCount != 525 || out.Succeeded != 524 || out.Groups[0].Conditions != 1 || out.Groups[0].Latest.ID != "run-0524" || out.Groups[0].LatestSuccess.ID != "run-0523" {
		t.Fatalf("wrong aggregation: %+v", out)
	}
	project := out.Groups[0].ID
	f.Mode = "history"
	f.Offset = 520
	out, e = s.Catalog(context.Background(), f)
	if e != nil || out.Total != 525 || len(out.Runs) != 5 || out.Runs[0].ID != "run-0004" {
		t.Fatalf("pagination: %+v %v", out, e)
	}
	f.Project = project
	f.Mode = "conditions"
	f.Offset = 0
	out, e = s.Catalog(context.Background(), f)
	if e != nil || len(out.Groups) != 1 {
		t.Fatal(out, e)
	}
	f.Condition = out.Groups[0].ID
	f.Mode = "history"
	f.Search = "run-0000"
	out, e = s.Catalog(context.Background(), f)
	if e != nil || out.Total != 1 || out.Runs[0].ID != "run-0000" {
		t.Fatal(out, e)
	}
	f.Search = ""
	f.Environments = []string{"missing-env"}
	out, e = s.Catalog(context.Background(), f)
	if e != nil || out.Total != 0 {
		t.Fatal(out, e)
	}
	f.Environments = nil
	f.Status = "failed"
	out, e = s.Catalog(context.Background(), f)
	if e != nil || out.Total != 1 {
		t.Fatal(out, e)
	}
	f.Status = ""
	f.Saved = true
	out, e = s.Catalog(context.Background(), f)
	if e != nil || out.Total != 0 {
		t.Fatal(out, e)
	}
	if e = s.Register("baseline", "baseline-test", domain.Baseline{ID: "baseline-test", Name: "test", RunID: "run-0000"}); e != nil {
		t.Fatal(e)
	}
	out, e = s.Catalog(context.Background(), f)
	if e != nil || out.Total != 1 || out.Saved != 1 {
		t.Fatal(out, e)
	}
	f.Saved = false
	f.From = "2026-01-01T08:40:00Z"
	out, e = s.Catalog(context.Background(), f)
	if e != nil || out.Total != 5 {
		t.Fatal(out, e)
	}
}
func TestCatalogKeyIsolation(t *testing.T) {
	a := sample(t).Run
	a.CaseRef = &domain.CaseRef{ID: "case-a", Revision: 1}
	p, c := catalogKeys(a)
	b := a
	b.Plan.Version.ID = "new-version"
	b.Plan.Version.Label = "new"
	b.CaseRef = &domain.CaseRef{ID: "case-a", Revision: 2}
	p2, c2 := catalogKeys(b)
	if p != p2 || c != c2 {
		t.Fatal("software/revision should not split conditions")
	}
	b.Plan.Environment.Config = map[string]string{"uid": "0"}
	p2, c2 = catalogKeys(b)
	if p != p2 || c == c2 {
		t.Fatal("identity must split conditions")
	}
	b = a
	b.Plan.Spec.Repeats++
	_, c2 = catalogKeys(b)
	if c == c2 {
		t.Fatal("repeat conditions merged")
	}
	b = a
	b.MaltaVersion = "other"
	_, c2 = catalogKeys(b)
	if c == c2 {
		t.Fatal("normalizer conditions merged")
	}
	b = a
	b.CaseRef = &domain.CaseRef{ID: "case-b", Revision: 1}
	p2, _ = catalogKeys(b)
	if p == p2 {
		t.Fatal("different cases merged")
	}
}
func TestCatalogRejectsInvalidQueryAndCancellation(t *testing.T) {
	s, e := Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for _, f := range []CatalogFilter{{Mode: "history", Limit: 101}, {Mode: "history", Limit: 20, Offset: -1}, {Mode: "bad", Limit: 20}, {Mode: "conditions", Limit: 20}, {Mode: "history", Limit: 20, From: "yesterday"}, {Mode: "history", Limit: 20, Status: "not-real"}} {
		if _, e = s.Catalog(context.Background(), f); e == nil {
			t.Fatalf("accepted %+v", f)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = s.Catalog(ctx, CatalogFilter{Mode: "overview", Limit: 20}); e == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestCatalogMixedConditionsAndIncompleteResults(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	base := sample(t).Run
	base.Attempt.ResultState = "complete"
	for i := 0; i < 4; i++ {
		r := base
		r.ID = fmt.Sprintf("run-mixed-%d", i)
		r.CreatedAt = time.Date(2026, 1, 1, 0, i, 0, 0, time.UTC)
		if i == 2 {
			r.Attempt.ResultState = "missing"
		}
		if i == 3 {
			r.Plan.Spec.Repeats++
		}
		if _, err = s.DB.Exec("INSERT INTO runs(id,body,created_at) VALUES(?,?,?)", r.ID, encode(r), r.CreatedAt.Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
	}
	page, err := s.Catalog(context.Background(), CatalogFilter{Mode: "overview", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Succeeded != 3 || page.Groups[0].Conditions != 2 || page.Groups[0].LatestSuccess != nil {
		t.Fatalf("mixed conditions merged: %+v", page)
	}
	page, err = s.Catalog(context.Background(), CatalogFilter{Mode: "conditions", Project: page.Groups[0].ID, Limit: 20})
	if err != nil || len(page.Groups) != 2 {
		t.Fatal(page, err)
	}
	for _, group := range page.Groups {
		if group.Count == 3 && (group.Latest.ID != "run-mixed-2" || group.LatestSuccess == nil || group.LatestSuccess.ID != "run-mixed-1" || len(group.Latest.Metrics) != 0) {
			t.Fatalf("incomplete evidence treated as successful: %+v", group)
		}
	}
}

func TestCatalogWorkloadFilters(t *testing.T) {
	s, e := Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	base := sample(t).Run
	var spec domain.Spec
	if e = domain.Read("../../benchmarks/data/smoke-read.yaml", &spec); e != nil {
		t.Fatal(e)
	}
	base.Plan.Spec = spec
	for i := 0; i < 3; i++ {
		r := base
		r.ID = fmt.Sprintf("run-filter-%d", i)
		if i == 1 {
			r.Plan.Spec.Workload.Operation = "write"
		}
		if i == 2 {
			r.Plan.Spec.Workload.FileSizeBytes *= 2
			r.Plan.Spec.Workload.BlockSizeBytes *= 2
		}
		if _, e = s.DB.Exec("INSERT INTO runs(id,body,created_at) VALUES(?,?,?)", r.ID, encode(r), r.CreatedAt.Format(time.RFC3339Nano)); e != nil {
			t.Fatal(e)
		}
	}
	for _, tt := range []struct {
		f    CatalogFilter
		want int
	}{
		{CatalogFilter{Operation: "read"}, 2},
		{CatalogFilter{Operation: "write"}, 1},
		{CatalogFilter{FileSize: spec.Workload.FileSizeBytes}, 2},
		{CatalogFilter{BlockSize: spec.Workload.BlockSizeBytes * 2}, 1},
		{CatalogFilter{Operation: "read", FileSize: spec.Workload.FileSizeBytes, BlockSize: spec.Workload.BlockSizeBytes}, 1},
	} {
		tt.f.Mode = "history"
		tt.f.Limit = 20
		page, e := s.Catalog(context.Background(), tt.f)
		if e != nil || page.Total != tt.want {
			t.Fatal(page, e, tt)
		}
	}
}
