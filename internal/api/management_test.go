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

func TestManagementOnlyCasesWithoutTokenAndNoExecution(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	plan := testPlan(t, t.TempDir())
	pending, err := s.CreateManaged(plan, "existing-pending", "ci", "local")
	if err != nil {
		t.Fatal(err)
	}
	m, err := service.New(s, service.Config{ManagementOnly: true, MaxBytes: 1 << 30, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	server := httptest.NewServer(Handler(m, "", nil))
	defer server.Close()
	status, data := call(t, server.URL, "GET", "/api/v1/capabilities", "", nil)
	var caps domain.Capabilities
	if status != 200 || json.Unmarshal(data, &caps) != nil || caps.Features["managed_runs"] || !caps.Features["management_only"] || len(caps.Executors) != 0 {
		t.Fatal(status, string(data))
	}
	q := domain.SaveCase{Name: "management case", Plan: plan}
	status, data = call(t, server.URL, "POST", "/api/v1/cases", "", q)
	if status != 201 {
		t.Fatal(status, string(data))
	}
	var v domain.CaseVersion
	if err = json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	q.BaseRevision = 1
	q.Description = "new revision"
	if status, data = call(t, server.URL, "POST", "/api/v1/cases/"+v.CaseID+"/versions", "", q); status != 201 {
		t.Fatal(status, string(data))
	}
	start := service.StartRequest{Plan: plan, RequestID: "must-not-run", Source: "ci", Executor: "local"}
	if status, data = call(t, server.URL, "POST", "/api/v1/runs", "", start); status < 400 {
		t.Fatal(status, string(data))
	}
	if _, err = m.Cancel(pending.ID); err == nil {
		t.Fatal("management-only cancelled an existing run")
	}
	// Allow the normal dispatch loop several ticks: pending work must remain untouched.
	time.Sleep(250 * time.Millisecond)
	got, err := s.Run(pending.ID)
	if err != nil || domain.Hash(got) != domain.Hash(pending) {
		t.Fatal("management-only recovered or dispatched existing work", err, got)
	}
	if _, err = s.ByRequest(start.RequestID); err == nil {
		t.Fatal("disabled execution persisted a request")
	}
}

func TestManagementOnlyRejectsExecutionBackend(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = service.New(s, service.Config{ManagementOnly: true, AllowedRoots: []string{t.TempDir()}, MaxBytes: 1 << 30, Timeout: time.Minute}); err == nil {
		t.Fatal("conflicting management-only backend accepted")
	}
}
