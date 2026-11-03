package workflow

import (
	"encoding/json"
	"github.com/wsq2333/malta/internal/backend"
	"github.com/wsq2333/malta/internal/backend/argo"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/store"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeTemplatesWithoutResultContract(t *testing.T) {
	m, q, _ := fixture(t)
	record, _ := m.Record("cases", q.Case)
	var d Definition
	json.Unmarshal(record.Body, &d)
	d.Results = nil
	d.Bindings = nil
	d.Parameters = nil
	d.Defaults = nil
	// Backend-specific graph/retry features are not interpreted by Malta.
	var ep map[string]any
	json.Unmarshal(d.Template, &ep)
	ep["custom_backend_field"] = map[string]any{"native": true}
	object(ep["spec"])["arguments"] = map[string]any{}
	d.Template, _ = json.Marshal(ep)
	record.Body, _ = json.Marshal(d)
	saved, e := m.Save("cases", SaveRecord{ID: record.ID, BaseRevision: 1, Name: record.Name, Body: record.Body})
	if e != nil {
		t.Fatal(e)
	}
	q.Case.Revision = saved.Revision
	p, e := m.Preflight(q)
	if e != nil || !p.Ready {
		t.Fatal(p, e)
	}
	r, e := m.Start(Start{p.ID, p.Snapshot.Hash, "native"})
	if e != nil {
		t.Fatal(e)
	}
	if _, ok := submissionInputs(r)["malta_context"]; ok {
		t.Fatal("injected undeclared input")
	}
	m.advance(&r)
	m.advance(&r)
	f := m.backends["test"].(*fakeBackend)
	f.obs = backend.Observation{RunID: r.ExternalRunID, VersionID: r.ExternalVersionID, Status: "Succeeded"}
	m.advance(&r)
	if r.Status != "succeeded" || r.Verdict != "not_evaluated" || r.ResultState != "not_configured" {
		t.Fatal("native success misreported as test pass", r)
	}
}
func TestArgoImplementationSelectionAndServerFiltering(t *testing.T) {
	m, q, cfg := fixture(t)
	argo := json.RawMessage(`{"apiVersion":"argoproj.io/v1alpha1","kind":"WorkflowTemplate","spec":{"entrypoint":"native-steps","retryStrategy":{"limit":2},"templates":[{"name":"native-steps","steps":[[{"name":"hello","template":"hello"}]]},{"name":"hello","container":{"image":"alpine","command":["echo","hello"]}}]}}`)
	d := Definition{SchemaVersion: Schema, TimeoutSeconds: 60, Implementations: map[string]Implementation{"argo": {Template: argo}}}
	body, _ := json.Marshal(d)
	saved, e := m.Save("cases", SaveRecord{Name: "native variants", Body: body})
	if e != nil {
		t.Fatal(e)
	}
	q.Case = Ref{saved.ID, 1}
	cfg.Backend = "argo"
	cfg.ID = "argo-test"
	m.connections[cfg.ID] = cfg
	m.backends[cfg.ID] = &fakeBackend{}
	profileRecord, _ := m.Record("profiles", q.Profile)
	var profile Profile
	json.Unmarshal(profileRecord.Body, &profile)
	profile.Backend = "argo"
	profile.Connection = cfg.ID
	body, _ = json.Marshal(profile)
	pr, e := m.Save("profiles", SaveRecord{Name: "Argo", Body: body})
	if e != nil {
		t.Fatal(e)
	}
	q.Profile = Ref{pr.ID, 1}
	p, e := m.Preflight(q)
	if e != nil || !p.Ready {
		t.Fatal(p, e)
	}
	if p.Snapshot.Definition.Backend != "argo" || string(p.Snapshot.EP) == "" {
		t.Fatal("wrong variant")
	}
	r, e := m.Start(Start{p.ID, p.Snapshot.Hash, "argo-native"})
	if e != nil {
		t.Fatal(e)
	}
	filtered, e := m.FilteredRuns(20, 0, "argo", "another-connection")
	if e != nil || len(filtered) != 0 {
		t.Fatal(filtered, e)
	}
	filtered, e = m.FilteredRuns(20, 0, "argo", cfg.ID)
	if e != nil || len(filtered) != 1 || filtered[0].ID != r.ID {
		t.Fatal(filtered, e)
	}
	cfg.Backend = "unsupported"
	m.connections[cfg.ID] = cfg
	if _, e = m.compile(q); e == nil {
		t.Fatal("accepted backend mismatch")
	}
}
func TestDisabledConnections(t *testing.T) {
	m, _, cfg := fixture(t)
	_ = m
	disabled := false
	cfg.Enabled = &disabled
	cfg.Cluster = ""
	cfg.Namespace = ""
	cfg.RemoteResultsRoot = ""
	cfg.LocalResultsRoot = ""
	s, e := store.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	manager, e := New(s, Config{Connections: []backend.Config{cfg}}, true)
	if e != nil {
		t.Fatal(e)
	}
	defer manager.Close()
	if manager.Enabled() || manager.Connections()[0]["auth_status"] != "disabled" {
		t.Fatal(manager.Connections())
	}
}
func TestArgoNativeLifecycleThroughREST(t *testing.T) {
	m, q, cfg := fixture(t)
	var tpl, run map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/workflow-templates/bench" && r.Method == "POST" {
			var b map[string]any
			json.NewDecoder(r.Body).Decode(&b)
			tpl = object(b["template"])
			json.NewEncoder(w).Encode(tpl)
			return
		}
		if len(r.URL.Path) > len("/api/v1/workflow-templates/") && r.URL.Path[:len("/api/v1/workflow-templates/")] == "/api/v1/workflow-templates/" {
			if tpl == nil {
				w.WriteHeader(404)
				return
			}
			json.NewEncoder(w).Encode(tpl)
			return
		}
		if r.Method == "POST" {
			var b map[string]any
			json.NewDecoder(r.Body).Decode(&b)
			run = object(b["workflow"])
			object(run["metadata"])["uid"] = "test-uid"
			run["status"] = map[string]any{"phase": "Succeeded"}
			json.NewEncoder(w).Encode(run)
			return
		}
		json.NewEncoder(w).Encode(run)
	}))
	defer server.Close()
	cfg.URL = server.URL
	cfg.Backend = "argo"
	m.connections["test"] = cfg
	// Exercise the Argo adapter through the shared manager lifecycle.
	client, e := argo.New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	m.backends["test"] = client
	cr, _ := m.Record("cases", q.Case)
	body := json.RawMessage(`{"schema_version":"workflow/v1alpha1","backend":"argo","timeout_seconds":60,"template":{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow","spec":{"entrypoint":"hello","templates":[{"name":"hello","container":{"image":"alpine","command":["echo","hello"]}}]}}}`)
	saved, e := m.Save("cases", SaveRecord{ID: cr.ID, BaseRevision: 1, Name: cr.Name, Body: body})
	if e != nil {
		t.Fatal(e)
	}
	q.Case.Revision = saved.Revision
	pr, _ := m.Record("profiles", q.Profile)
	var p Profile
	json.Unmarshal(pr.Body, &p)
	p.Backend = "argo"
	pr.Body, _ = json.Marshal(p)
	saved, e = m.Save("profiles", SaveRecord{ID: pr.ID, BaseRevision: 1, Name: pr.Name, Body: pr.Body})
	if e != nil {
		t.Fatal(e)
	}
	q.Profile.Revision = saved.Revision
	check, e := m.Preflight(q)
	if e != nil || !check.Ready {
		t.Fatal(check, e)
	}
	r, e := m.Start(Start{check.ID, check.Snapshot.Hash, "native-rest"})
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 3; i++ {
		m.advance(&r)
		m.persist(r)
	}
	if r.Status != "succeeded" || r.Verdict != "not_evaluated" {
		t.Fatal(r.Error, r.Status, r.Verdict)
	}
}

func TestArgoEvidenceIsVerifiedAfterNativeExecution(t *testing.T) {
	m, q, cfg := fixture(t)
	var template, run map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "workflow-templates") {
			if r.Method == "POST" {
				var b map[string]any
				json.NewDecoder(r.Body).Decode(&b)
				template = object(b["template"])
			}
			if template == nil {
				w.WriteHeader(404)
				return
			}
			json.NewEncoder(w).Encode(template)
			return
		}
		if r.Method == "POST" {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			run = object(body["workflow"])
			metadata := object(run["metadata"])
			metadata["uid"] = "proof-uid"
			name := metadata["name"].(string)
			id := name + ".proof-uid"
			stage := name + "-stage"
			var ctx map[string]string
			for _, v := range array(object(object(run["spec"])["arguments"])["parameters"]) {
				p := object(v)
				if p["name"] == "malta_context" {
					json.Unmarshal([]byte(p["value"].(string)), &ctx)
				}
			}
			dir := filepath.Join(cfg.LocalResultsRoot, id, stage)
			if e := os.MkdirAll(dir, 0700); e != nil {
				t.Error(e)
			}
			raw := []byte(`<testsuite tests="1"><testcase classname="argo" name="real-report"/></testsuite>`)
			os.WriteFile(filepath.Join(dir, "junit.xml"), raw, 0600)
			manifest := Evidence{SchemaVersion: "malta.evidence/v1", ExternalVersionID: ctx["workflow_version_id"], ExternalRunID: id, NodeID: "check", StageID: stage, SnapshotHash: ctx["snapshot_hash"], EnvironmentID: ctx["environment_id"], SoftwareVersionID: ctx["software_version_id"], Parser: "junit", Assertions: map[string]bool{}, Files: []EvidenceFile{{Path: "junit.xml", SHA256: domain.BytesHash(raw), Size: int64(len(raw))}}}
			data, _ := json.Marshal(manifest)
			os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0600)
			run["status"] = map[string]any{"phase": "Succeeded", "nodes": map[string]any{stage: map[string]any{"id": stage, "name": name + ".check", "displayName": "check", "type": "Pod", "phase": "Succeeded", "outputs": map[string]any{"parameters": []any{map[string]any{"name": "evidence", "value": "/shared/" + id + "/" + stage + "/manifest.json"}}, "artifacts": []any{map[string]any{"name": "native-artifact-reference"}}}}}}
		}
		json.NewEncoder(w).Encode(run)
	}))
	defer server.Close()
	cfg.TimeoutSeconds = 600
	cfg.Backend = "argo"
	cfg.URL = server.URL
	m.connections["test"] = cfg
	client, e := argo.New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	m.backends["test"] = client
	var save SaveRecord
	if e = domain.Read("../../docs/examples/workflow/argo-evidence.example.json", &save); e != nil {
		t.Fatal(e)
	}
	rec, e := m.Save("cases", save)
	if e != nil {
		t.Fatal(e)
	}
	q.Case = Ref{rec.ID, rec.Revision}
	old, _ := m.Record("profiles", q.Profile)
	var profile Profile
	json.Unmarshal(old.Body, &profile)
	profile.Backend = "argo"
	body, _ := json.Marshal(profile)
	rec, e = m.Save("profiles", SaveRecord{Name: "Argo evidence", Body: body})
	if e != nil {
		t.Fatal(e)
	}
	q.Profile = Ref{rec.ID, rec.Revision}
	check, e := m.Preflight(q)
	if e != nil || !check.Ready {
		t.Fatal(check, e)
	}
	r, e := m.Start(Start{check.ID, check.Snapshot.Hash, "argo-proof"})
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 3; i++ {
		m.advance(&r)
		m.persist(r)
	}
	if r.Status != "succeeded" || r.Verdict != "passed" || len(r.Results) != 1 || len(r.Results[0].Metrics) == 0 {
		t.Fatal(r.Error, r.Status, r.Results)
	}
	if !strings.Contains(string(r.Observations), "native-artifact-reference") {
		t.Fatal("native output metadata was discarded")
	}
	// Native workflow with no parser cannot be used as a verification result (existing issue rule).
}
func TestNativeExamplesAndConfiguration(t *testing.T) {
	for _, name := range []string{"native-argo.example.json", "argo-evidence.example.json"} {
		var q SaveRecord
		if e := domain.Read("../../docs/examples/workflow/"+name, &q); e != nil {
			t.Fatal(name, e)
		}
		var d Definition
		json.Unmarshal(q.Body, &d)
		if e := validateDefinition(d); e != nil {
			t.Fatal(name, e)
		}
	}
	var c Config
	if e := domain.Read("../../config/parameter/workflow-backends.example.yaml", &c); e != nil {
		t.Fatal(e)
	}
	s, e := store.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	manager, e := New(s, c, true)
	if e != nil {
		t.Fatal(e)
	}
	defer manager.Close()
	if manager.Enabled() || len(manager.Connections()) != 1 {
		t.Fatal("disabled sample unexpectedly active")
	}
}

func TestUnsupportedBackendsCannotExecute(t *testing.T) {
	m, q, cfg := fixture(t)
	record, _ := m.Record("cases", q.Case)
	var d Definition
	if e := json.Unmarshal(record.Body, &d); e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"", "private-engine", "ARGO"} {
		t.Run("backend-"+kind, func(t *testing.T) {
			bad := d
			bad.Backend = kind
			raw, _ := json.Marshal(bad)
			if _, e := m.Save("cases", SaveRecord{Name: "unsupported", Body: raw}); e == nil {
				t.Fatal("accepted unsupported case")
			}
			badConfig := cfg
			badConfig.Backend = kind
			disabled := false
			badConfig.Enabled = &disabled
			manager, e := New(m.Store, Config{Connections: []backend.Config{badConfig}}, true)
			if e == nil {
				manager.Close()
				t.Fatal("accepted unsupported connection even when disabled")
			}
			profile := Profile{Backend: kind, Connection: cfg.ID, Cluster: cfg.Cluster, Namespace: cfg.Namespace, SystemIDs: []string{q.System.ID}}
			raw, _ = json.Marshal(profile)
			if _, e := m.Save("profiles", SaveRecord{Name: "unsupported", Body: raw}); e == nil {
				t.Fatal("accepted unsupported profile")
			}
		})
	}
	mixed := Definition{SchemaVersion: Schema, Implementations: map[string]Implementation{"argo": {Template: d.Template}, "private-engine": {Template: d.Template}}}
	raw, _ := json.Marshal(mixed)
	if _, e := m.Save("cases", SaveRecord{Name: "mixed unsupported", Body: raw}); e == nil {
		t.Fatal("accepted hidden unsupported implementation")
	}
	if _, e := m.FilteredRuns(20, 0, "private-engine", ""); e == nil {
		t.Fatal("accepted unsupported filter")
	}
	// A bare object must not be mistaken for a native Argo definition.
	d.Template = json.RawMessage(`{"spec":{}}`)
	if e := validateDefinition(d); e == nil {
		t.Fatal("accepted non-Argo template")
	}
}

func TestUnsupportedHistoricalRunIsNotResumed(t *testing.T) {
	m, q, _ := fixture(t)
	pre, e := m.Preflight(q)
	if e != nil {
		t.Fatal(e)
	}
	r, e := m.Start(Start{pre.ID, pre.Snapshot.Hash, "historical"})
	if e != nil {
		t.Fatal(e)
	}
	var profile Profile
	json.Unmarshal(r.Snapshot.Profile.Body, &profile)
	profile.Backend = "private-engine"
	r.Snapshot.Profile.Body, _ = json.Marshal(profile)
	m.advance(&r)
	fake := m.backends["test"].(*fakeBackend)
	if r.Error == "" || fake.registers != 0 || fake.submits != 0 || fake.lookups != 0 {
		t.Fatal("resumed unsupported historical run", r)
	}
}
