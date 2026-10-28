package api

import (
	"encoding/json"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/service"
	"github.com/wsq2333/malta/internal/store"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIssueAPIManagementOnly(t *testing.T) {
	s, e := store.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	m, e := service.New(s, service.Config{ManagementOnly: true, MaxBytes: 1 << 30, Timeout: time.Minute})
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	server := httptest.NewServer(Handler(m, "secret", nil))
	defer server.Close()
	q := domain.SaveIssue{IssueID: "issue-api", IssueFields: domain.IssueFields{Title: "latency", Type: "性能", Description: "regression", Software: "fs v1", Hardware: "cloud vm", OccurredAt: time.Now().UTC(), Reporter: "agent"}}
	if status, _ := call(t, server.URL, "POST", "/api/v1/issues", "", q); status != 401 {
		t.Fatal(status)
	}
	status, data := call(t, server.URL, "POST", "/api/v1/issues", "secret", q)
	if status != 201 {
		t.Fatal(status, string(data))
	}
	var v domain.Issue
	if e = json.Unmarshal(data, &v); e != nil || v.Revision != 1 {
		t.Fatal(e, v)
	}
	q.BaseRevision = 1
	q.Resolved = true
	q.Resolution = "fixed and retested"
	status, data = call(t, server.URL, "POST", "/api/v1/issues", "secret", q)
	if status != 201 {
		t.Fatal(status, string(data))
	}
	if status, _ = call(t, server.URL, "POST", "/api/v1/issues", "secret", q); status != 409 {
		t.Fatal(status)
	}
	for _, path := range []string{"/api/v1/issues", "/api/v1/issues/issue-api", "/api/v1/issues/issue-api/history"} {
		if status, data = call(t, server.URL, "GET", path, "secret", nil); status != 200 {
			t.Fatal(status, string(data))
		}
	}
	if status, _ = call(t, server.URL, "GET", "/api/v1/issues/missing", "secret", nil); status != 404 {
		t.Fatal(status)
	}
	for _, bad := range []any{map[string]any{"unknown": true}, map[string]any{"resolved": "false"}, map[string]any{"description": strings.Repeat("x", 129<<10)}} {
		if status, _ = call(t, server.URL, "POST", "/api/v1/issues", "secret", bad); status != 400 {
			t.Fatal(status)
		}
	}
}
