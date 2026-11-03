package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wsq2333/malta/internal/backend"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/service"
	"github.com/wsq2333/malta/internal/store"
	"github.com/wsq2333/malta/internal/workflow"
)

// Run an entire Malta API -> Argo HTTP -> shared evidence -> issue linkage cycle.
// The backend is a contract fixture; it never creates cluster workloads.
func TestWorkflowAPIEndToEnd(t *testing.T) {
	root := t.TempDir()
	token := filepath.Join(t.TempDir(), "token")
	if e := os.WriteFile(token, []byte("backend-token"), 0600); e != nil {
		t.Fatal(e)
	}
	var definition workflow.SaveRecord
	if e := domain.Read("../../docs/examples/workflow/case.example.json", &definition); e != nil {
		t.Fatal(e)
	}
	var def workflow.Definition
	_ = json.Unmarshal(definition.Body, &def)
	node := def.Results[0].NodeID
	var posts atomic.Int32
	var template, remoteRun map[string]any
	obj := func(v any) map[string]any { m, _ := v.(map[string]any); return m }
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer backend-token" {
			w.WriteHeader(401)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/v1/workflow-templates/bench") {
			if r.Method == "POST" {
				var b map[string]any
				json.NewDecoder(r.Body).Decode(&b)
				template = obj(b["template"])
			}
			if template == nil {
				w.WriteHeader(404)
				return
			}
			json.NewEncoder(w).Encode(template)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/api/v1/workflows/bench" {
			posts.Add(1)
			var b map[string]any
			json.NewDecoder(r.Body).Decode(&b)
			remoteRun = obj(b["workflow"])
			meta := obj(remoteRun["metadata"])
			meta["uid"] = "fixture-uid"
			name := meta["name"].(string)
			runID := name + ".fixture-uid"
			var ctx map[string]string
			params := obj(obj(remoteRun["spec"])["arguments"])["parameters"].([]any)
			for _, v := range params {
				p := obj(v)
				if p["name"] == "malta_context" {
					json.Unmarshal([]byte(p["value"].(string)), &ctx)
				}
			}
			report := []byte(`<testsuite tests="1"><testcase classname="contract" name="evidence"/></testsuite>`)
			dir := filepath.Join(root, runID, "stage", "0")
			if e := os.MkdirAll(dir, 0700); e != nil {
				t.Error(e)
			}
			os.WriteFile(filepath.Join(dir, "junit.xml"), report, 0600)
			ev := workflow.Evidence{SchemaVersion: "malta.evidence/v1", ExternalVersionID: ctx["workflow_version_id"], ExternalRunID: runID, NodeID: node, StageID: "stage", SnapshotHash: ctx["snapshot_hash"], EnvironmentID: ctx["environment_id"], SoftwareVersionID: ctx["software_version_id"], Parser: "junit", Assertions: map[string]bool{}, Files: []workflow.EvidenceFile{{Path: "junit.xml", SHA256: domain.BytesHash(report), Size: int64(len(report))}}}
			raw, _ := json.Marshal(ev)
			os.WriteFile(filepath.Join(dir, "result.json"), raw, 0600)
			remoteRun["status"] = map[string]any{"phase": "Succeeded", "nodes": map[string]any{"stage": map[string]any{"id": "stage", "name": name + "." + node, "type": "Pod", "phase": "Succeeded", "outputs": map[string]any{"parameters": []any{map[string]any{"name": def.Results[0].Output, "value": "/shared/" + runID + "/stage/0/result.json"}}}}}}
			json.NewEncoder(w).Encode(remoteRun)
			return
		}
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/v1/workflows/bench/") && remoteRun != nil {
			json.NewEncoder(w).Encode(remoteRun)
			return
		}
		t.Errorf("unexpected backend request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(404)
	}))
	defer remote.Close()
	s, e := store.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	m, e := service.New(s, service.Config{MaxBytes: 1 << 30, Timeout: time.Minute, Workflows: workflow.Config{Connections: []backend.Config{{Backend: "argo", ID: "test", URL: remote.URL, TokenFile: token, RemoteResultsRoot: "/shared", LocalResultsRoot: root, MaxBytes: 1 << 30, TimeoutSeconds: 120, ArtifactBytes: 1 << 20, Cluster: "sample", Namespace: "bench"}}}})
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	server := httptest.NewServer(Handler(m, "test-token", nil))
	defer server.Close()
	for _, path := range []string{"/api/v1/workflow/connections", "/api/v1/workflow/runs", "/api/v1/workflow/records/cases"} {
		if status, _ := call(t, server.URL, "GET", path, "", nil); status != 401 {
			t.Fatal("unauthenticated", path, status)
		}
	}
	plan := testPlan(t, t.TempDir())
	plan.Environment.Config = map[string]string{"cluster": "sample", "node": "node"}
	if e = s.Register("environment", plan.Environment.ID, plan.Environment); e != nil {
		t.Fatal(e)
	}
	if e = s.Register("version", plan.Version.ID, plan.Version); e != nil {
		t.Fatal(e)
	}
	save := func(kind string, q workflow.SaveRecord) workflow.Record {
		status, data := call(t, server.URL, "POST", "/api/v1/workflow/records/"+kind, "test-token", q)
		if status != 201 {
			t.Fatalf("%s: %d %s", kind, status, data)
		}
		var v workflow.Record
		_ = json.Unmarshal(data, &v)
		return v
	}
	c := save("cases", definition)
	raw, _ := json.Marshal(workflow.System{EnvironmentID: plan.Environment.ID, SoftwareVersionID: plan.Version.ID, Settings: map[string]any{}})
	sys := save("systems", workflow.SaveRecord{Name: "test system", Body: raw})
	raw, _ = json.Marshal(workflow.Profile{Backend: "argo", Connection: "test", Cluster: "sample", Namespace: "bench", Node: "node", SystemIDs: []string{sys.ID}, Settings: map[string]any{"results_pvc": "shared-nas", "results_mount": "/shared", "node": "node"}})
	prof := save("profiles", workflow.SaveRecord{Name: "test profile", Body: raw})
	q := workflow.Selection{Case: workflow.Ref{ID: c.ID, Revision: 1}, System: workflow.Ref{ID: sys.ID, Revision: 1}, Profile: workflow.Ref{ID: prof.ID, Revision: 1}, VersionID: plan.Version.ID, Parameters: map[string]any{}}
	status, data := call(t, server.URL, "POST", "/api/v1/workflow/preflight", "test-token", q)
	var pre workflow.Preflight
	_ = json.Unmarshal(data, &pre)
	if status != 200 || !pre.Ready {
		t.Fatalf("preflight %d %s", status, data)
	}
	start := workflow.Start{PreflightID: pre.ID, SnapshotHash: pre.Snapshot.Hash, RequestID: "http-end-to-end"}
	status, data = call(t, server.URL, "POST", "/api/v1/workflow/runs", "test-token", start)
	if status != 202 {
		t.Fatalf("start %d %s", status, data)
	}
	var run workflow.Run
	_ = json.Unmarshal(data, &run)
	id := run.ID
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		_, data = call(t, server.URL, "GET", "/api/v1/workflow/runs/"+id, "test-token", nil)
		_ = json.Unmarshal(data, &run)
		if run.Status == "succeeded" || run.Status == "failed" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if run.Status != "succeeded" || run.Verdict != "passed" || run.ResultState != "complete" {
		t.Fatalf("run not verified: %s", data)
	}
	if posts.Load() != 1 {
		t.Fatal("submission duplicated")
	}
	_, data = call(t, server.URL, "POST", "/api/v1/workflow/runs", "test-token", start)
	var same workflow.Run
	_ = json.Unmarshal(data, &same)
	if same.ID != id {
		t.Fatal("idempotency lost")
	}
	for _, file := range run.Results[0].Files {
		if status, _ := call(t, server.URL, "GET", "/api/v1/workflow/runs/"+id+"/artifacts/"+file.Path, "test-token", nil); status != 200 {
			t.Fatal("artifact", status)
		}
	}
	issue, e := s.SaveIssue(domain.SaveIssue{IssueFields: domain.IssueFields{Title: "test issue", Type: "功能", Description: "contract fixture", Software: "fixture", Hardware: "fixture", Reporter: "test", OccurredAt: time.Now()}})
	if e != nil {
		t.Fatal(e)
	}
	for _, role := range []string{"discovery", "verification"} {
		if status, data = call(t, server.URL, "POST", "/api/v1/workflow/runs/"+id+"/issues", "test-token", workflow.IssueLinkRequest{IssueID: issue.IssueID, Role: role}); status != 201 {
			t.Fatalf("link %d %s", status, data)
		}
	}
	status, data = call(t, server.URL, "GET", "/api/v1/workflow/issues/"+issue.IssueID+"/runs", "test-token", nil)
	var links []workflow.LinkedIssue
	_ = json.Unmarshal(data, &links)
	if status != 200 || len(links) != 2 || links[0].Issue.Resolved {
		t.Fatal("links or issue lifecycle", status, string(data))
	}
}
