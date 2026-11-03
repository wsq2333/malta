package workflow

import (
	"context"
	"encoding/json"
	"github.com/wsq2333/malta/internal/backend"
	"github.com/wsq2333/malta/internal/backend/uce"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/store"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Manager, Selection, uce.Config) {
	t.Helper()
	s, e := store.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cfg := uce.Config{ID: "test", URL: "http://example.invalid", TokenFile: filepath.Join(t.TempDir(), "token"), RemoteResultsRoot: "/shared", LocalResultsRoot: t.TempDir(), MaxBytes: 1 << 30, TimeoutSeconds: 120, ArtifactBytes: 1 << 20, Cluster: "sample", Namespace: "bench", Subtypes: []string{"malta"}}
	if e = os.WriteFile(cfg.TokenFile, []byte("test-token"), 0600); e != nil {
		t.Fatal(e)
	}
	m := &Manager{Store: s, enabled: true, ctx: ctx, connections: map[string]uce.Config{"test": cfg}, backends: map[string]backend.Backend{"test": &fakeBackend{}}}
	if e = migrate(s); e != nil {
		t.Fatal(e)
	}
	var plan domain.Plan
	if e = domain.Read("../../environments/local.yaml", &plan.Environment); e != nil {
		t.Fatal(e)
	}
	if e = domain.Read("../../environments/version-example.yaml", &plan.Version); e != nil {
		t.Fatal(e)
	}
	plan.Environment.Config = map[string]string{"cluster": "sample", "node": "node"}
	if e = s.Register("environment", plan.Environment.ID, plan.Environment); e != nil {
		t.Fatal(e)
	}
	if e = s.Register("version", plan.Version.ID, plan.Version); e != nil {
		t.Fatal(e)
	}
	var q SaveRecord
	if e = domain.Read("../../docs/examples/workflow/case.example.json", &q); e != nil {
		t.Fatal(e)
	}
	c, e := m.Save("cases", q)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(System{EnvironmentID: plan.Environment.ID, SoftwareVersionID: plan.Version.ID, Settings: map[string]any{}})
	sys, e := m.Save("systems", SaveRecord{Name: "test system", Body: raw})
	if e != nil {
		t.Fatal(e)
	}
	raw, _ = json.Marshal(Profile{Backend: "uce", Connection: "test", Cluster: "sample", Namespace: "bench", Node: "node", Queue: "default", Subtype: "malta", SystemIDs: []string{sys.ID}, Settings: map[string]any{"results_pvc": "shared-nas", "results_mount": "/shared", "node": "node"}})
	p, e := m.Save("profiles", SaveRecord{Name: "test profile", Body: raw})
	if e != nil {
		t.Fatal(e)
	}
	return m, Selection{Case: Ref{c.ID, 1}, System: Ref{sys.ID, 1}, Profile: Ref{p.ID, 1}, VersionID: plan.Version.ID, Parameters: map[string]any{}}, cfg
}

type fakeBackend struct {
	registers, submits, lookups, cancels int
	lost                                 bool
	outputs                              []backend.Output
	obs                                  backend.Observation
}

func (f *fakeBackend) Register(context.Context, json.RawMessage) (string, error) {
	f.registers++
	return "external-version", nil
}
func (f *fakeBackend) Submit(context.Context, backend.Submission) (string, error) {
	f.submits++
	if f.lost {
		return "", context.DeadlineExceeded
	}
	return "external-run", nil
}
func (f *fakeBackend) Lookup(context.Context, string) (string, error) {
	f.lookups++
	return "external-run", nil
}
func (f *fakeBackend) Observe(context.Context, string) (backend.Observation, error) {
	return f.obs, nil
}
func (f *fakeBackend) Cancel(context.Context, string) error                       { f.cancels++; return nil }
func (f *fakeBackend) CancellationComplete(context.Context, string) (bool, error) { return true, nil }
func (f *fakeBackend) Outputs(context.Context, string, backend.Stage) ([]backend.Output, error) {
	return f.outputs, nil
}
func TestVersionAndPreflightIntegrity(t *testing.T) {
	m, q, _ := fixture(t)
	c, _ := m.Record("cases", q.Case)
	v, e := m.Save("cases", SaveRecord{ID: c.ID, BaseRevision: 1, Name: c.Name, Body: c.Body})
	if e != nil || v.Revision != 2 {
		t.Fatal(e)
	}
	if _, e = m.Save("cases", SaveRecord{ID: c.ID, BaseRevision: 1, Name: c.Name, Body: c.Body}); e == nil {
		t.Fatal("accepted stale edit")
	}
	a, e := m.Preflight(q)
	if e != nil || !a.Ready {
		t.Fatal(e, a)
	}
	b, e := m.Preflight(q)
	if e != nil || a.Snapshot.Hash != b.Snapshot.Hash {
		t.Fatal("non-deterministic compilation", e)
	}
	r, e := m.Start(Start{a.ID, a.Snapshot.Hash, "stable-request"})
	if e != nil {
		t.Fatal(e)
	}
	same, e := m.Start(Start{a.ID, a.Snapshot.Hash, "stable-request"})
	if e != nil || same.ID != r.ID {
		t.Fatal("duplicate submission", e)
	}
	if _, e = m.Start(Start{b.ID, b.Snapshot.Hash, "other-request"}); e == nil {
		t.Fatal("cluster test isolation bypassed")
	}
	if _, e = m.Start(Start{a.ID, "changed", "stable-request"}); e == nil {
		t.Fatal("request drift accepted")
	}

}
func TestLostSubmissionRecovery(t *testing.T) {
	m, q, _ := fixture(t)
	f := &fakeBackend{lost: true}
	m.backends["test"] = f
	p, _ := m.Preflight(q)
	r, e := m.Start(Start{p.ID, p.Snapshot.Hash, "recovery"})
	if e != nil {
		t.Fatal(e)
	}
	m.advance(&r)
	_ = m.persist(r)
	m.advance(&r)
	_ = m.persist(r)
	if !r.SubmissionStarted || f.submits != 1 {
		t.Fatal("missing persisted submission intent")
	}
	// Re-read durable state to model a process restart after a lost POST response.
	r, e = m.Get(r.ID)
	if e != nil {
		t.Fatal(e)
	}
	m.advance(&r)
	if f.submits != 1 || f.lookups != 1 || r.ExternalRunID != "external-run" {
		t.Fatal("resubmitted uncertain run", f, r)
	}
}
func TestEvidenceMissingStaleAndValid(t *testing.T) {
	m, q, cfg := fixture(t)
	p, _ := m.Preflight(q)
	r, e := m.Start(Start{p.ID, p.Snapshot.Hash, "evidence"})
	if e != nil {
		t.Fatal(e)
	}
	r.ExternalRunID = "external-run"
	r.ExternalVersionID = "external-version"
	rule := r.Snapshot.Definition.Results[0]
	stage := backend.Stage{ID: "stage", NodeID: rule.NodeID, Status: "succeeded", Attempt: 0}
	path := "/shared/external-run/stage/0/result.json"
	if _, e = m.verify(&r, rule, stage, path, cfg, new(int64)); e == nil {
		t.Fatal("missing result passed")
	}
	root := filepath.Join(cfg.LocalResultsRoot, "external-run/stage/0")
	_ = os.MkdirAll(root, 0700)
	report := []byte(`<testsuite><testcase name="test" classname="suite"/></testsuite>`)
	_ = os.WriteFile(filepath.Join(root, "junit.xml"), report, 0600)
	manifest := Evidence{SchemaVersion: "malta.evidence/v1", UCERunID: r.ExternalRunID, UCEVersionID: r.ExternalVersionID, SnapshotHash: r.Snapshot.Hash, NodeID: rule.NodeID, StageID: stage.ID, EnvironmentID: r.Snapshot.Environment.ID, SoftwareVersionID: r.Snapshot.Version.ID, Parser: "junit", Assertions: map[string]bool{}, Files: []EvidenceFile{{"junit.xml", domain.BytesHash(report), int64(len(report))}}}
	save := func() {
		raw, _ := json.Marshal(manifest)
		_ = os.WriteFile(filepath.Join(root, "result.json"), raw, 0600)
	}
	save()
	result, e := m.verify(&r, rule, stage, path, cfg, new(int64))
	if e != nil || result.Verdict != "passed" {
		t.Fatal(result, e)
	}
	manifest.Attempt = 1
	save()
	if _, e = m.verify(&r, rule, stage, path, cfg, new(int64)); e == nil {
		t.Fatal("stale attempt accepted")
	}
	manifest.Attempt = 0
	manifest.Files[0].SHA256 = strings.Repeat("0", 64)
	save()
	if _, e = m.verify(&r, rule, stage, path, cfg, new(int64)); e == nil {
		t.Fatal("corrupt file accepted")
	}
}
func TestPathAndJUnitValidation(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret")
	_ = os.WriteFile(outside, []byte("secret"), 0600)
	_ = os.Symlink(outside, filepath.Join(root, "link"))
	for _, p := range []string{"../secret", "link", "/secret"} {
		if _, e := readEvidence(root, p, 1024); e == nil {
			t.Fatal("unsafe path accepted", p)
		}
	}
	for _, raw := range []string{`<testsuite/>`, `<testsuite tests="2"><testcase name="one"/></testsuite>`, `<testsuite><testcase name="x"/><testcase name="x"/></testsuite>`, `<!DOCTYPE x><testsuite><testcase name="x"/></testsuite>`} {
		if _, _, _, e := parseJUnit([]byte(raw)); e == nil {
			t.Fatal("invalid report accepted")
		}
	}
	n, f, s, e := parseJUnit([]byte(`<testsuite><testcase name="x"><failure/></testcase><testcase name="y"><skipped/></testcase></testsuite>`))
	if e != nil || n != 2 || f != 1 || s != 1 {
		t.Fatal(n, f, s, e)
	}
}
func TestCancelBeforeSubmissionAndDeadline(t *testing.T) {
	m, q, _ := fixture(t)
	p, _ := m.Preflight(q)
	r, _ := m.Start(Start{p.ID, p.Snapshot.Hash, "cancel"})
	r.Deadline = time.Now().Add(-time.Second)
	m.advance(&r)
	if r.Status != "cancelled" || !r.CancelConfirmed || r.SubmissionStarted {
		t.Fatal(r)
	}
}

func TestPreflightRejectsPlacementAndChangedConnection(t *testing.T) {
	m, q, cfg := fixture(t)
	p, e := m.Preflight(q)
	if e != nil {
		t.Fatal(e)
	}
	cfg.Namespace = "another-namespace"
	m.connections["test"] = cfg
	if _, e = m.Start(Start{p.ID, p.Snapshot.Hash, "changed-route"}); e == nil {
		t.Fatal("accepted changed route")
	}
	cfg.Namespace = "bench"
	m.connections["test"] = cfg
	record, _ := m.Record("profiles", q.Profile)
	var profile Profile
	_ = json.Unmarshal(record.Body, &profile)
	profile.Node = "wrong-node"
	record.Body, _ = json.Marshal(profile)
	saved, e := m.Save("profiles", SaveRecord{ID: record.ID, BaseRevision: 1, Name: record.Name, Body: record.Body})
	if e != nil {
		t.Fatal(e)
	}
	q.Profile.Revision = saved.Revision
	if _, e = m.Preflight(q); e == nil {
		t.Fatal("accepted manifest with wrong client")
	}
}
func TestBackendSuccessWithoutEvidenceDoesNotPass(t *testing.T) {
	m, q, _ := fixture(t)
	p, _ := m.Preflight(q)
	r, e := m.Start(Start{p.ID, p.Snapshot.Hash, "missing-evidence"})
	if e != nil {
		t.Fatal(e)
	}
	m.advance(&r)
	m.advance(&r)
	f := m.backends["test"].(*fakeBackend)
	f.obs = backend.Observation{RunID: r.ExternalRunID, VersionID: r.ExternalVersionID, Status: "Succeeded"}
	d := time.Now().Add(-time.Second)
	r.CollectionDeadline = &d
	m.advance(&r)
	if r.Status != "failed" || r.ResultState != "missing" || r.Verdict != "inconclusive" {
		t.Fatal(r)
	}
}
func TestCancelFlagSurvivesWorkerPersistence(t *testing.T) {
	m, q, _ := fixture(t)
	p, _ := m.Preflight(q)
	r, e := m.Start(Start{p.ID, p.Snapshot.Hash, "cancel-race"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = m.Cancel(r.ID); e != nil {
		t.Fatal(e)
	}
	if e = m.persist(r); e != nil {
		t.Fatal(e)
	}
	current, _ := m.Get(r.ID)
	if !current.CancelRequested {
		t.Fatal("worker lost concurrent cancellation")
	}
}

func TestFioEvidenceAndFailedStageReport(t *testing.T) {
	m, q, cfg := fixture(t)
	p, _ := m.Preflight(q)
	r, e := m.Start(Start{p.ID, p.Snapshot.Hash, "parsers"})
	if e != nil {
		t.Fatal(e)
	}
	r.ExternalRunID = "external-run"
	r.ExternalVersionID = "external-version"
	rule := r.Snapshot.Definition.Results[0]
	stage := backend.Stage{ID: "stage", NodeID: rule.NodeID, Status: "Failed"}
	root := filepath.Join(cfg.LocalResultsRoot, "external-run/stage/0")
	_ = os.MkdirAll(root, 0700)
	publish := func(parser, filename string, raw []byte) {
		t.Helper()
		_ = os.WriteFile(filepath.Join(root, filename), raw, 0600)
		manifest := Evidence{SchemaVersion: "malta.evidence/v1", UCERunID: r.ExternalRunID, UCEVersionID: r.ExternalVersionID, SnapshotHash: r.Snapshot.Hash, NodeID: rule.NodeID, StageID: stage.ID, EnvironmentID: r.Snapshot.Environment.ID, SoftwareVersionID: r.Snapshot.Version.ID, Parser: parser, Assertions: map[string]bool{}, Files: []EvidenceFile{{filename, domain.BytesHash(raw), int64(len(raw))}}}
		data, _ := json.Marshal(manifest)
		_ = os.WriteFile(filepath.Join(root, "result.json"), data, 0600)
	}
	publish("junit", "junit.xml", []byte(`<testsuite tests="1"><testcase name="fails"><failure message="regression"/></testcase></testsuite>`))
	f := m.backends["test"].(*fakeBackend)
	f.outputs = []backend.Output{{RunID: r.ExternalRunID, StageID: stage.ID, Name: rule.Output, Path: "/shared/external-run/stage/0/result.json"}}
	results, complete, invalid, passed := m.collect(&r, backend.Observation{Stages: []backend.Stage{stage}}, f, cfg)
	if !complete || invalid || passed || results[0].Verdict != "failed" {
		t.Fatal("failed stage report lost", results)
	}
	raw, e := os.ReadFile("../../testdata/fio/fio-3.33.json")
	if e != nil {
		t.Fatal(e)
	}
	var fio map[string]any
	_ = json.Unmarshal(raw, &fio)
	job := fio["jobs"].([]any)[0].(map[string]any)
	options := job["job options"].(map[string]any)
	options["direct"] = "1"
	options["allow_file_create"] = "0"
	rule.Parser = "fio-3.33"
	rule.ExpectedOptions = map[string]string{}
	for _, k := range []string{"rw", "bs", "numjobs", "iodepth", "ioengine", "direct", "runtime", "allow_file_create"} {
		rule.ExpectedOptions[k] = options[k].(string)
	}
	raw, _ = json.Marshal(fio)
	publish("fio-3.33", "fio.json", raw)
	result, e := m.verify(&r, rule, stage, f.outputs[0].Path, cfg, new(int64))
	if e != nil || len(result.Metrics) == 0 {
		t.Fatal(result, e)
	}
	rule.ExpectedOptions["bs"] = "4m"
	if _, e = m.verify(&r, rule, stage, f.outputs[0].Path, cfg, new(int64)); e == nil {
		t.Fatal("accepted wrong fio block size")
	}
}

func TestMissingCredentialBlocksPreflight(t *testing.T) {
	m, q, cfg := fixture(t)
	if e := os.Remove(cfg.TokenFile); e != nil {
		t.Fatal(e)
	}
	p, e := m.Preflight(q)
	if e != nil || p.Ready {
		t.Fatal("missing credentials permitted", p, e)
	}
}
