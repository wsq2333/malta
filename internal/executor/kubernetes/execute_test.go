package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wsq2333/malta/internal/domain"
)

type fakeTransport struct {
	run        domain.Run
	calls      []string
	resultMode string
	foreign    bool
	rwx        bool
}

func (f *fakeTransport) Do(ctx context.Context, input any, args ...string) ([]byte, error) {
	f.calls = append(f.calls, strings.Join(args, " "))
	out := any(object{})
	switch args[0] + " " + args[1] {
	case "get pvc":
		modes := []string{"ReadWriteMany"}
		if !f.rwx {
			modes = []string{"ReadWriteOnce"}
		}
		out = object{"spec": object{"accessModes": modes, "volumeName": "pv", "volumeMode": "Filesystem"}, "status": object{"phase": "Bound"}}
	case "get node":
		out = object{"metadata": object{"uid": "nodeuid", "labels": object{"kubernetes.io/hostname": "node"}}, "status": object{"conditions": []object{{"type": "Ready", "status": "True"}}}}
	case "get job":
		lab := labels(f.run)
		if f.foreign {
			lab["malta.attempt"] = "other"
		}
		conditions := []object{}
		if f.resultMode == "complete" {
			conditions = append(conditions, object{"type": "Complete", "status": "True"})
		}
		out = object{"metadata": object{"uid": "jobuid", "labels": lab}, "status": object{"conditions": conditions}}
	case "get pods":
		out = object{"items": []any{}}
	case "logs job/" + Name(f.run):
		return []byte("test log"), nil
	}
	return json.Marshal(out)
}
func fixture(t *testing.T) (*Executor, domain.Run, *fakeTransport) {
	t.Helper()
	var spec domain.Spec
	var env domain.Environment
	var version domain.SoftwareVersion
	for _, v := range []struct {
		path  string
		value any
	}{{"../../../benchmarks/data/smoke-read.yaml", &spec}, {"../../../environments/local.yaml", &env}, {"../../../environments/version-example.yaml", &version}} {
		if err := domain.Read(v.path, v.value); err != nil {
			t.Fatal(err)
		}
	}
	env.Kind = "kubernetes"
	env.Config = map[string]string{"cluster": "test", "node": "node", "benchmark_pvc": "bench"}
	p, err := domain.NewPlan(spec, env, version, "/bench", "test")
	if err != nil {
		t.Fatal(err)
	}
	r := domain.NewRun(p, "ci")
	r.Attempt.Executor = "kubernetes"
	c := Config{Namespace: "test", Cluster: "test", Node: "node", BenchmarkPVC: "bench", ResultsPVC: "results", ResultsDir: t.TempDir(), Image: "malta@sha256:" + strings.Repeat("a", 64), ServiceAccount: "runner", CPUMillis: 1000, MemoryBytes: 256 << 20}
	f := &fakeTransport{run: r, rwx: true, resultMode: "complete"}
	e := &Executor{c, f}
	return e, r, f
}
func TestRejectUnsupportedPVCBeforeSubmit(t *testing.T) {
	e, _, f := fixture(t)
	f.rwx = false
	if _, err := e.Preflight(context.Background()); err == nil {
		t.Fatal("RWO result PVC accepted")
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "create ") {
			t.Fatal("submitted")
		}
	}
}
func TestJobSafetyAndIdentity(t *testing.T) {
	e, r, _ := fixture(t)
	_, job := e.Objects(r, time.Minute, 1<<20)
	b, _ := json.Marshal(job)
	s := string(b)
	for _, want := range []string{`"backoffLimit":0`, `"restartPolicy":"Never"`, `"activeDeadlineSeconds":105`, `"automountServiceAccountToken":false`, `"readOnly":true`, `"podReplacementPolicy":"Failed"`} {
		if !strings.Contains(s, want) {
			t.Fatal(want)
		}
	}
	if strings.Contains(s, "ttlSecondsAfterFinished") {
		t.Fatal("TTL before persistence")
	}
	c := e.Config
	c.Image = "malta:latest"
	if c.Validate() == nil {
		t.Fatal("mutable image accepted")
	}
}
func TestCompleteWithoutResultsFailsAndRetainsJob(t *testing.T) {
	e, r, f := fixture(t)
	r, err := e.Execute(context.Background(), r, t.TempDir(), Options{Timeout: time.Minute, MaxBytes: 1 << 20, Update: func(domain.Run) error { return nil }})
	if err != nil || r.Status != "failed" || r.Attempt.ResultState != "failed" {
		t.Fatalf("%+v %v", r, err)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "delete") {
			t.Fatal("deleted before durability")
		}
	}
	if err = e.Cleanup(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "delete") {
			t.Fatal("deleted incomplete evidence")
		}
	}
}
func TestResumeDoesNotCreateAndCancelStopsJob(t *testing.T) {
	e, r, f := fixture(t)
	f.resultMode = ""
	r.Attempt.JobName = Name(r)
	r.Attempt.JobUID = "jobuid"
	r.Attempt.BackendHash = domain.Hash(e.Config)
	r.Attempt.StartedAt = time.Now()
	r.Status = "cancelling"
	r, err := e.Execute(context.Background(), r, t.TempDir(), Options{Timeout: time.Minute, Update: func(domain.Run) error { return nil }})
	if err != nil || r.Status != "cancelled" {
		t.Fatalf("%s %v", r.Status, err)
	}
	patch := false
	for _, c := range f.calls {
		if strings.HasPrefix(c, "create") {
			t.Fatal("recreated resumed job")
		}
		if strings.HasPrefix(c, "patch job") {
			patch = true
		}
	}
	if !patch {
		t.Fatal("did not suspend")
	}
}
func TestForeignJobAndChangedConfigRejected(t *testing.T) {
	e, r, f := fixture(t)
	f.foreign = true
	if _, err := e.Execute(context.Background(), r, t.TempDir(), Options{Timeout: time.Minute, Update: func(domain.Run) error { return nil }}); err == nil {
		t.Fatal("foreign job accepted")
	}
	r.Attempt.BackendHash = "other"
	if _, err := e.Execute(context.Background(), r, t.TempDir(), Options{}); err == nil {
		t.Fatal("changed backend accepted")
	}
}
func TestShutdownDetaches(t *testing.T) {
	e, r, f := fixture(t)
	f.resultMode = ""
	r.Attempt.JobName = Name(r)
	r.Attempt.JobUID = "jobuid"
	r.Attempt.BackendHash = domain.Hash(e.Config)
	r.Attempt.StartedAt = time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := e.Execute(ctx, r, t.TempDir(), Options{})
	if !errors.Is(err, ErrDetached) {
		t.Fatal(err)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "delete") || strings.HasPrefix(c, "patch") {
			t.Fatal("shutdown stopped job")
		}
	}
}
func TestSubmissionIntentPersistedBeforeCreate(t *testing.T) {
	e, r, f := fixture(t)
	out := t.TempDir()
	sentinel := errors.New("database unavailable")
	_, err := e.Execute(context.Background(), r, out, Options{Update: func(domain.Run) error { return sentinel }})
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "create") {
			t.Fatal("created before durable intent")
		}
	}
	if _, err = os.Stat(filepath.Join(out, "environment-observed.json")); err != nil {
		t.Fatal(err)
	}
}
