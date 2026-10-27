package api

import (
	"bytes"
	"encoding/json"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/engine/fio"
	"github.com/wsq2333/malta/internal/service"
	"github.com/wsq2333/malta/internal/store"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testPlan(t *testing.T, target string) domain.Plan {
	t.Helper()
	var spec domain.Spec
	var env domain.Environment
	var ver domain.SoftwareVersion
	for _, x := range []struct {
		p string
		v any
	}{{"../../benchmarks/data/smoke-read.yaml", &spec}, {"../../environments/local.yaml", &env}, {"../../environments/version-example.yaml", &ver}} {
		if e := domain.Read(x.p, x.v); e != nil {
			t.Fatal(e)
		}
	}
	spec.Repeats = 1
	p, e := domain.NewPlan(spec, env, ver, target, "smoke")
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func fakeFio(t *testing.T, slow bool) string {
	t.Helper()
	dir := t.TempDir()
	raw, e := os.ReadFile("../../testdata/fio/fio-3.33.json")
	if e != nil {
		t.Fatal(e)
	}
	// The fake tool reports options for this test's actual plan, like real fio.
	var fixtureJSON map[string]any
	if e = json.Unmarshal(raw, &fixtureJSON); e != nil {
		t.Fatal(e)
	}
	options := map[string]string{}
	p := testPlan(t, "/unused")
	for _, arg := range fio.Args(p, p.Trials[0], "/unused", false) {
		key, value, ok := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		if ok {
			options[key] = value
		}
	}
	fixtureJSON["jobs"].([]any)[0].(map[string]any)["job options"] = options
	raw, e = json.Marshal(fixtureJSON)
	if e != nil {
		t.Fatal(e)
	}
	fixture := filepath.Join(dir, "fixture.json")
	if e = os.WriteFile(fixture, raw, 0600); e != nil {
		t.Fatal(e)
	}
	bin := filepath.Join(dir, "fio")
	body := "#!/bin/sh\nif [ \"$1\" = --version ]; then echo fio-3.33; exit; fi\ncat '" + fixture + "'\n"
	if slow {
		body = "#!/bin/sh\nif [ \"$1\" = --version ]; then echo fio-3.33; exit; fi\nsleep 30 &\nwait\n"
	}
	if e = os.WriteFile(bin, []byte(body), 0700); e != nil {
		t.Fatal(e)
	}
	return bin
}
func call(t *testing.T, base, method, path, token string, v any) (int, json.RawMessage) {
	t.Helper()
	b, _ := json.Marshal(v)
	req, e := http.NewRequest(method, base+path, bytes.NewReader(b))
	if e != nil {
		t.Fatal(e)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer resp.Body.Close()
	data, e := io.ReadAll(resp.Body)
	if e != nil {
		t.Fatal(e)
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(data, &env)
	if resp.StatusCode >= 400 {
		return resp.StatusCode, data
	}
	return resp.StatusCode, env.Data
}
func TestManagedAPI(t *testing.T) {
	s, e := store.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	target := t.TempDir()
	bin := os.Getenv("MALTA_TEST_FIO")
	if bin == "" {
		bin = fakeFio(t, false)
	}
	m, e := service.New(s, service.Config{AllowedRoots: []string{target}, Fio: bin, MaxBytes: 1 << 30, Timeout: 30 * time.Second})
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	server := httptest.NewServer(Handler(m, "test-token", nil))
	defer server.Close()
	if status, _ := call(t, server.URL, "GET", "/api/v1/runs", "", nil); status != 401 {
		t.Fatal(status)
	}
	q := service.StartRequest{Plan: testPlan(t, target), RequestID: "idempotency", Source: "ci", Executor: "local"}
	status, data := call(t, server.URL, "POST", "/api/v1/runs", "test-token", q)
	if status != 202 {
		t.Fatal(status, string(data))
	}
	var r domain.Run
	_ = json.Unmarshal(data, &r)
	_, data = call(t, server.URL, "POST", "/api/v1/runs", "test-token", q)
	var second domain.Run
	_ = json.Unmarshal(data, &second)
	if r.ID != second.ID {
		t.Fatal("request duplicated")
	}
	if status, found := call(t, server.URL, "GET", "/api/v1/requests/idempotency", "test-token", nil); status != 200 {
		t.Fatal(status, string(found))
	} else {
		var recovered domain.Run
		if e = json.Unmarshal(found, &recovered); e != nil || recovered.ID != r.ID {
			t.Fatal("lookup changed identity")
		}
	}
	if status, caps := call(t, server.URL, "GET", "/api/v1/capabilities", "test-token", nil); status != 200 {
		t.Fatal(status)
	} else {
		var v domain.Capabilities
		if e = json.Unmarshal(caps, &v); e != nil || !v.Features["request_lookup"] || v.Mode != "server" {
			t.Fatal(string(caps))
		}
	}
	if status, _ := call(t, server.URL, "GET", "/api/v1/requests/missing", "test-token", nil); status != 404 {
		t.Fatal(status)
	}

	q.Source = "agent"
	if status, _ = call(t, server.URL, "POST", "/api/v1/runs", "test-token", q); status != 409 {
		t.Fatal("request conflict not rejected", status)
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		_, data = call(t, server.URL, "GET", "/api/v1/runs/"+r.ID, "test-token", nil)
		_ = json.Unmarshal(data, &r)
		if domain.Terminal(r.Status) && r.Attempt.ResultState != "collecting" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if r.Status != "succeeded" || r.Attempt.ResultState != "complete" {
		t.Fatalf("run: %+v", r)
	}
	if status, data = call(t, server.URL, "GET", "/api/v1/runs/"+r.ID+"/bundle", "test-token", nil); status != 200 {
		t.Fatal(status, string(data))
	}
	if status, _ = call(t, server.URL, "POST", "/api/v1/baselines", "test-token", map[string]string{"name": "test", "run_id": r.ID}); status != 201 {
		t.Fatal(status)
	}
	if status, _ = call(t, server.URL, "GET", "/api/v1/trends?run_id="+r.ID, "test-token", nil); status != 200 {
		t.Fatal(status)
	}
	q.RequestID = "outside"
	q.Plan.Target = "/tmp"
	q.Plan.Hash = q.Plan.Digest()
	if status, _ = call(t, server.URL, "POST", "/api/v1/runs", "test-token", q); status != 400 {
		t.Fatal("escaped target", status)
	}
}
func TestCancelAndRestart(t *testing.T) {
	s, e := store.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	target := t.TempDir()
	config := service.Config{AllowedRoots: []string{target}, Fio: fakeFio(t, true), MaxBytes: 1 << 30, Timeout: time.Minute}
	m, e := service.New(s, config)
	if e != nil {
		t.Fatal(e)
	}
	r, e := m.Start(service.StartRequest{Plan: testPlan(t, target), RequestID: "cancel", Source: "ci", Executor: "local"})
	if e != nil {
		t.Fatal(e)
	}
	time.Sleep(200 * time.Millisecond)
	if _, e = m.Cancel(r.ID); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 100; i++ {
		r, e = s.Run(r.ID)
		if e != nil {
			t.Fatal(e)
		}
		if r.Status == "cancelled" && r.Attempt.ResultState == "complete" {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if r.Status != "cancelled" || r.Attempt.ResultState != "complete" {
		t.Fatalf("cancel did not settle: %+v", r)
	}
	m.Close()
	pending, e := s.CreateManaged(testPlan(t, target), "crashed", "ci")
	if e != nil {
		t.Fatal(e)
	}
	m, e = service.New(s, config)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	recovered, e := s.Run(pending.ID)
	if e != nil || recovered.Status != "interrupted" {
		t.Fatal(recovered.Status, e)
	}
	again, e := m.Start(service.StartRequest{Plan: pending.Plan, RequestID: "crashed", Source: "ci", Executor: "local"})
	if e != nil || again.ID != pending.ID || again.Status != "interrupted" {
		t.Fatal("restart resubmitted work", again, e)
	}
	if _, e = service.New(s, config); e == nil {
		t.Fatal("accepted second manager")
	}
	m.Config.MaxBytes = 1
	again, e = m.Start(service.StartRequest{Plan: pending.Plan, RequestID: "crashed", Source: "ci", Executor: "local"})
	if e != nil || again.ID != pending.ID {
		t.Fatal("policy change broke idempotent retrieval", e)
	}
}

func TestRealManagedScenario(t *testing.T) {
	bin := os.Getenv("MALTA_TEST_FIO")
	if bin == "" {
		t.Skip("requires real fio 3.33")
	}
	target := t.TempDir()
	old := testPlan(t, target)
	var sc domain.ScenarioSpec
	if e := domain.Read("../../benchmarks/scenarios/mixed-data.yaml", &sc); e != nil {
		t.Fatal(e)
	}
	sc.Repeats = 1
	sc.Stages[0].RuntimeSeconds = 2
	sc.Stages[0].WarmupSeconds = 0
	// Identical concurrency must still retain distinct workload identity.
	sc.Stages[0].Workloads[0].NumJobs = 1
	p, e := domain.NewScenarioPlan(sc, old.Environment, old.Version, target, "scene")
	if e != nil {
		t.Fatal(e)
	}
	s, e := store.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	m, e := service.New(s, service.Config{AllowedRoots: []string{target}, Fio: bin, MaxBytes: 1 << 30, Timeout: 30 * time.Second})
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	server := httptest.NewServer(Handler(m, "test-token", nil))
	defer server.Close()
	q := service.StartRequest{Plan: p, RequestID: "scenario-e2e", Source: "agent", Executor: "local"}
	status, data := call(t, server.URL, "POST", "/api/v1/runs", "test-token", q)
	if status != 202 {
		t.Fatal(status, string(data))
	}
	var r domain.Run
	json.Unmarshal(data, &r)
	_, data = call(t, server.URL, "POST", "/api/v1/runs", "test-token", q)
	var same domain.Run
	json.Unmarshal(data, &same)
	if same.ID != r.ID {
		t.Fatal("duplicate run")
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		r, e = s.Run(r.ID)
		if e != nil {
			t.Fatal(e)
		}
		if domain.Terminal(r.Status) && r.Attempt.ResultState != "collecting" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if r.Status != "succeeded" || r.Attempt.ResultState != "complete" {
		t.Fatal(r.Status, r.Error)
	}
	b, e := s.Bundle(r.ID)
	if e != nil {
		t.Fatal(e)
	}
	other, e := store.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer other.Close()
	if e = other.Import(b); e != nil {
		t.Fatal(e)
	}
	if _, e = other.SetBaseline("mixed", r.ID); e != nil {
		t.Fatal(e)
	}
	comparison, e := other.Compare(r.ID, r.ID)
	if e != nil || !comparison.Comparable || len(comparison.Metrics) != 6 {
		t.Fatal(comparison, e)
	}
	for _, d := range comparison.Metrics {
		if d.Baseline.StageID == "" || d.Baseline.Count != 1 {
			t.Fatal("workloads merged", d)
		}
	}
}
