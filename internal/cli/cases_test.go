package cli

import (
	"bytes"
	"encoding/json"
	"github.com/wsq2333/malta/internal/domain"
	"path/filepath"
	"testing"
)

func TestCaseCLI(t *testing.T) {
	dir := t.TempDir()
	var spec domain.Spec
	var env domain.Environment
	var ver domain.SoftwareVersion
	for _, f := range []struct {
		p string
		v any
	}{{"../../benchmarks/data/smoke-read.yaml", &spec}, {"../../environments/local.yaml", &env}, {"../../environments/version-example.yaml", &ver}} {
		if e := domain.Read(f.p, f.v); e != nil {
			t.Fatal(e)
		}
	}
	p, e := domain.NewPlan(spec, env, ver, "/tmp/case-cli", "case")
	if e != nil {
		t.Fatal(e)
	}
	q := domain.SaveCase{Name: "CLI case", Plan: p}
	file := filepath.Join(dir, "case.json")
	if e = domain.Write(file, q); e != nil {
		t.Fatal(e)
	}
	run := func(args ...string) json.RawMessage {
		t.Helper()
		var out, errs bytes.Buffer
		if code := Execute(append([]string{"--data-dir", filepath.Join(dir, "db")}, args...), &out, &errs); code != 0 {
			t.Fatalf("%v: %s", args, out.String())
		}
		var env struct {
			Data json.RawMessage `json:"data"`
		}
		if e := json.Unmarshal(out.Bytes(), &env); e != nil {
			t.Fatal(e)
		}
		return env.Data
	}
	var v domain.CaseVersion
	if e = json.Unmarshal(run("case", "save", "--file", file), &v); e != nil {
		t.Fatal(e)
	}
	q.BaseRevision = 1
	q.Description = "edited"
	if e = domain.Write(file, q); e != nil {
		t.Fatal(e)
	}
	run("case", "save", "--id", v.CaseID, "--file", file)
	var old domain.CaseVersion
	if e = json.Unmarshal(run("case", "get", v.CaseID, "1"), &old); e != nil || domain.Hash(old) != domain.Hash(v) {
		t.Fatal("old revision changed", e)
	}
	var history []domain.CaseVersion
	if e = json.Unmarshal(run("case", "history", v.CaseID), &history); e != nil || len(history) != 2 {
		t.Fatal(e, len(history))
	}
	var cases []domain.Case
	if e = json.Unmarshal(run("case", "list"), &cases); e != nil || len(cases) != 1 || cases[0].Latest.Revision != 2 {
		t.Fatal(e, cases)
	}
	for _, args := range [][]string{{"case", "get", v.CaseID, "0"}, {"--server", "http://127.0.0.1:1", "run", "start", "--case", v.CaseID, "--request-id", "test"}, {"--server", "http://127.0.0.1:1", "run", "start", "--case", v.CaseID, "--plan", file, "--request-id", "test"}} {
		var out, errs bytes.Buffer
		if code := Execute(args, &out, &errs); code != 2 {
			t.Fatal("invalid arguments not rejected", code, out.String())
		}
	}
}
