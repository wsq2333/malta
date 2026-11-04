package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/wsq2333/malta/internal/backend"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/store"
)

func TestParameterizedToolsCase(t *testing.T) {
	var q SaveRecord
	if err := domain.Read("../../docs/examples/workflow/oss-fuse-fio.argo-case.example.json", &q); err != nil {
		t.Fatal(err)
	}
	var d Definition
	if err := json.Unmarshal(q.Body, &d); err != nil {
		t.Fatal(err)
	}
	if err := validateDefinition(d); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"oss-fuse-fio.argo-parameters.example.json"} {
		var p map[string]any
		if err := domain.Read("../../docs/examples/workflow/"+name, &p); err != nil {
			t.Fatal(err)
		}
		if err := schemaCheck(d.Parameters, p); err != nil {
			t.Fatal(err)
		}
	}
	if len(d.Results) != 3 {
		t.Fatal("all three stages must be verified")
	}
}

// Optional integration: consumes reports produced by the real SDK/fio development-host smoke test.
func TestToolsGeneratedEvidence(t *testing.T) {
	root := os.Getenv("MALTA_TOOLS_EVIDENCE_DIR")
	if root == "" {
		t.Skip("set MALTA_TOOLS_EVIDENCE_DIR to the tools smoke export")
	}
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m := &Manager{Store: s}
	var q SaveRecord
	if err = domain.Read("../../docs/examples/workflow/oss-fuse-fio.argo-case.example.json", &q); err != nil {
		t.Fatal(err)
	}
	var d Definition
	json.Unmarshal(q.Body, &d)
	var used int64
	for i, action := range []string{"upload", "warmup", "fio-read"} {
		var ev Evidence
		file := filepath.Join(root, "workflow-test.uid-test", action+"-attempt", "evidence.json")
		if err = domain.Read(file, &ev); err != nil {
			t.Fatal(err)
		}
		r := Run{ID: "workflow-tools-proof", ExternalRunID: ev.ExternalRunID, ExternalVersionID: ev.ExternalVersionID}
		r.Snapshot.Hash = ev.SnapshotHash
		r.Snapshot.Environment.ID = ev.EnvironmentID
		r.Snapshot.Version.ID = ev.SoftwareVersionID
		rule := d.Results[i]
		if rule.NodeID != ev.NodeID {
			t.Fatal("logical Argo node mismatch")
		}
		if action == "fio-read" {
			rule.ExpectedOptions = map[string]string{"rw": "read", "bs": "1048576", "numjobs": "2", "iodepth": "1", "ioengine": "psync", "direct": "1", "runtime": "1", "allow_file_create": "0", "time_based": "1"}
		}
		result, err := m.verify(&r, rule, backend.Stage{ID: ev.StageID, NodeID: ev.NodeID, Attempt: ev.Attempt, EvictionCount: ev.EvictionCount}, "/results/workflow-test.uid-test/"+action+"-attempt/evidence.json", backend.Config{RemoteResultsRoot: "/results", LocalResultsRoot: root, ArtifactBytes: 32 << 20}, &used)
		if err != nil || result.State != "complete" || result.Verdict != "passed" {
			t.Fatal(action, result, err)
		}
		if action == "fio-read" && len(result.Metrics) == 0 {
			t.Fatal("fio metrics missing")
		}
	}
}
