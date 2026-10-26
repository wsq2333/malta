package cli

import (
	"bytes"
	"encoding/json"
	"github.com/wsq2333/malta/internal/domain"
	"os"
	"path/filepath"
	"testing"
)

func TestJSONContract(t *testing.T) {
	for _, tt := range []struct {
		args []string
		exit int
		code string
	}{{[]string{"version"}, 0, ""}, {[]string{"exec"}, 2, "invalid"}, {[]string{"exec", "--plan", "/no-such-malta-plan", "--out", "/tmp/unused"}, 3, "execution"}, {[]string{"--format", "text", "version"}, 2, "invalid"}} {
		var out, errOut bytes.Buffer
		if code := Execute(tt.args, &out, &errOut); code != tt.exit {
			t.Fatalf("%v: exit %d, %s", tt.args, code, out.String())
		}
		var e Envelope
		if err := json.Unmarshal(out.Bytes(), &e); err != nil {
			t.Fatal(err, out.String())
		}
		if e.SchemaVersion != "1.0" {
			t.Fatal(e)
		}
		if tt.code != "" && (e.Error == nil || e.Error.Code != tt.code) {
			t.Fatal(e)
		}
	}
}

func TestWorkerRejectsMismatchedVersionBeforeClaim(t *testing.T) {
	var spec domain.Spec
	var env domain.Environment
	var version domain.SoftwareVersion
	for _, input := range []struct {
		path  string
		value any
	}{{"../../benchmarks/data/smoke-read.yaml", &spec}, {"../../environments/local.yaml", &env}, {"../../environments/version-example.yaml", &version}} {
		if err := domain.Read(input.path, input.value); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := domain.NewPlan(spec, env, version, "/bench", "test")
	if err != nil {
		t.Fatal(err)
	}
	r := domain.NewRun(plan, "ci")
	r.Attempt.Executor = "kubernetes"
	r.MaltaVersion = "incompatible"
	dir := t.TempDir()
	input := filepath.Join(dir, "run.json")
	if err = domain.Write(input, r); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := Execute([]string{"worker", "--run", input, "--results", dir}, &out, &stderr); code != 2 {
		t.Fatalf("%d %s", code, out.String())
	}
	if _, err = os.Stat(filepath.Join(dir, r.Attempt.ID)); !os.IsNotExist(err) {
		t.Fatal("worker touched results before checking version")
	}
}
