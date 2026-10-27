package api

import (
	"encoding/json"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/service"
	"github.com/wsq2333/malta/internal/store"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCaseAPIAndPinnedRun(t *testing.T) {
	s, e := store.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	target := t.TempDir()
	m, e := service.New(s, service.Config{AllowedRoots: []string{target}, Fio: fakeFio(t, true), MaxBytes: 1 << 30, Timeout: 30 * time.Second})
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	server := httptest.NewServer(Handler(m, "secret", nil))
	defer server.Close()
	q := domain.SaveCase{Name: "case", Plan: testPlan(t, target)}
	if status, _ := call(t, server.URL, "POST", "/api/v1/cases", "", q); status != 401 {
		t.Fatal(status)
	}
	status, data := call(t, server.URL, "POST", "/api/v1/cases", "secret", q)
	if status != 201 {
		t.Fatal(status, string(data))
	}
	var v domain.CaseVersion
	if e = json.Unmarshal(data, &v); e != nil {
		t.Fatal(e)
	}
	path := "/api/v1/cases/" + v.CaseID + "/versions"
	q.BaseRevision = 1
	q.Description = "second"
	if status, data = call(t, server.URL, "POST", path, "secret", q); status != 201 {
		t.Fatal(status, string(data))
	}
	if status, _ = call(t, server.URL, "POST", path, "secret", q); status != 409 {
		t.Fatal(status)
	}
	if status, _ = call(t, server.URL, "GET", path+"/99", "secret", nil); status != 404 {
		t.Fatal(status)
	}
	if status, _ = call(t, server.URL, "GET", path+"/bad", "secret", nil); status != 400 {
		t.Fatal(status)
	}
	start := service.StartRequest{CaseRef: &domain.CaseRef{ID: v.CaseID, Revision: 1}, RequestID: "case-test", Source: "ci", Executor: "local"}
	status, data = call(t, server.URL, "POST", "/api/v1/runs", "secret", start)
	if status != 202 {
		t.Fatal(status, string(data))
	}
	var run domain.Run
	if e = json.Unmarshal(data, &run); e != nil {
		t.Fatal(e)
	}
	if run.CaseRef == nil || run.CaseRef.Revision != 1 || run.Plan.Hash != v.Plan.Hash {
		t.Fatal("wrong revision", run)
	}
	if status, _ = call(t, server.URL, "POST", "/api/v1/runs", "secret", start); status != 202 {
		t.Fatal(status)
	}
	start.CaseRef.Revision = 2
	if status, _ = call(t, server.URL, "POST", "/api/v1/runs", "secret", start); status != 409 {
		t.Fatal(status)
	}
	start.Plan = v.Plan
	if status, _ = call(t, server.URL, "POST", "/api/v1/runs", "secret", start); status != 400 {
		t.Fatal(status)
	}
	if _, e = m.Cancel(run.ID); e != nil {
		t.Fatal(e)
	}
}
