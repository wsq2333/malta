package store

import (
	"database/sql"
	"errors"
	"github.com/wsq2333/malta/internal/domain"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func issueInput() domain.SaveIssue {
	return domain.SaveIssue{IssueID: "issue-reopen", IssueFields: domain.IssueFields{Title: "reopen denied", Type: "功能 / 文件权限", Description: "created by UID 10001, reopen denied", Software: "Curvine 0.2.2", Hardware: "ecs.g8i.2xlarge", OccurredAt: time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC), Reporter: "agent:test"}}
}
func issueError(t *testing.T, e error, code string) {
	t.Helper()
	var d *domain.Error
	if !errors.As(e, &d) || d.Code != code {
		t.Fatalf("want %s, got %v", code, e)
	}
}
func TestIssueLifecycleAndConcurrency(t *testing.T) {
	dir := t.TempDir()
	s, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	q := issueInput()
	first, e := s.SaveIssue(q)
	if e != nil {
		t.Fatal(e)
	}
	replay, e := s.SaveIssue(q)
	if e != nil || domain.Hash(replay) != domain.Hash(first) {
		t.Fatal("create retry", e)
	}
	changed := q
	changed.Description = "different"
	_, e = s.SaveIssue(changed)
	issueError(t, e, "conflict")
	q.BaseRevision = 1
	q.Resolved = true
	q.Resolution = "fixed ownership and verified close/reopen as UID 10001"
	q.ChangeNote = "verified"
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { _, err := s.SaveIssue(q); results <- err })
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		} else {
			issueError(t, err, "conflict")
		}
	}
	if wins != 1 {
		t.Fatal(wins)
	}
	latest, e := s.Issue(q.IssueID)
	if e != nil || latest.Revision != 2 || latest.ResolvedAt == nil || !latest.CreatedAt.Equal(first.CreatedAt) {
		t.Fatal(latest, e)
	}
	q.BaseRevision = 2
	q.Resolved = false
	q.Resolution = ""
	q.ChangeNote = "regression reproduced"
	if _, e = s.SaveIssue(q); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	h, e := s.IssueHistory(q.IssueID)
	if e != nil || len(h) != 3 {
		t.Fatal(h, e)
	}
	if domain.Hash(h[2]) != domain.Hash(first) || h[0].ResolvedAt != nil || !h[1].Resolved || h[1].Resolution == "" {
		t.Fatal("history mutated", h)
	}
	list, e := s.Issues()
	if e != nil || len(list) != 1 || list[0].Revision != 3 {
		t.Fatal(list, e)
	}
}
func TestIssueValidationAndReferences(t *testing.T) {
	s, e := Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for _, edit := range []func(*domain.SaveIssue){
		func(q *domain.SaveIssue) { q.Type = " " }, func(q *domain.SaveIssue) { q.Resolved = true },
		func(q *domain.SaveIssue) { q.Resolution = "not closed" }, func(q *domain.SaveIssue) { q.OccurredAt = time.Time{} },
		func(q *domain.SaveIssue) { q.RunID = "missing" }, func(q *domain.SaveIssue) { q.VersionID = "missing" },
		func(q *domain.SaveIssue) { q.EnvironmentID = "missing" }, func(q *domain.SaveIssue) { q.IssueID = "../bad" },
		func(q *domain.SaveIssue) { q.BaseRevision = -1 },
	} {
		q := issueInput()
		edit(&q)
		if _, e = s.SaveIssue(q); e == nil {
			t.Fatalf("accepted %+v", q)
		}
	}
	list, _ := s.Issues()
	if len(list) != 0 {
		t.Fatal("partial invalid issue saved")
	}
	b := sample(t)
	if e = s.Import(b); e != nil {
		t.Fatal(e)
	}
	q := issueInput()
	q.RunID = b.Run.ID
	q.VersionID = b.Run.Plan.Version.ID
	q.EnvironmentID = b.Run.Plan.Environment.ID
	if _, e = s.SaveIssue(q); e != nil {
		t.Fatal(e)
	}
	other := b.Run.Plan.Version
	other.ID = "other-version"
	if e = s.Register("version", other.ID, other); e != nil {
		t.Fatal(e)
	}
	q.IssueID = "issue-mismatch"
	q.VersionID = other.ID
	_, e = s.SaveIssue(q)
	issueError(t, e, "invalid")
	q = issueInput()
	q.IssueID = ""
	v, e := s.SaveIssue(q)
	if e != nil || v.IssueID == "" {
		t.Fatal(v, e)
	}
	_, e = s.Issue("absent")
	issueError(t, e, "not_found")
	_, e = s.IssueHistory("absent")
	issueError(t, e, "not_found")
}
func TestIssueMigrationPreservesV2Data(t *testing.T) {
	dir := t.TempDir()
	s, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	v, e := s.SaveCase("", domain.SaveCase{Name: "before upgrade", Plan: sample(t).Run.Plan})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec("DROP TABLE issue_history; DROP TABLE issues; PRAGMA user_version=2;"); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	old, e := s.CaseVersion(v.CaseID, 1)
	if e != nil || domain.Hash(old) != domain.Hash(v) {
		t.Fatal("case migration lost data", e)
	}
	if _, e = s.SaveIssue(issueInput()); e != nil {
		t.Fatal(e)
	}
	var version int
	s.DB.QueryRow("PRAGMA user_version").Scan(&version)
	if version != 3 {
		t.Fatal(version)
	}
	s.Close()
	s, e = Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	issues, e := s.Issues()
	if e != nil || len(issues) != 1 {
		t.Fatal(e, issues)
	}
	s.Close()
	db, e := sql.Open("sqlite", filepath.Join(dir, "malta.db"))
	if e != nil {
		t.Fatal(e)
	}
	db.Exec("PRAGMA user_version=4")
	db.Close()
	if s, e = Open(dir); e == nil {
		s.Close()
		t.Fatal("future schema accepted")
	}
}
func TestIssueExample(t *testing.T) {
	var q domain.SaveIssue
	if e := domain.Read("../../web/src/issue-example.json", &q); e != nil {
		t.Fatal(e)
	}
	if e := q.Validate(); e != nil {
		t.Fatal(e)
	}

}
