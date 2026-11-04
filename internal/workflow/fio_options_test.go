package workflow

import (
	"encoding/json"
	"github.com/wsq2333/malta/internal/domain"
	"testing"
)

func TestFioParameterCompilationPreservesDecimal(t *testing.T) {
	m, selection, cfg := fixture(t)
	cfg.TimeoutSeconds = 1200
	m.connections["test"] = cfg
	var q SaveRecord
	if err := domain.Read("../../docs/examples/workflow/oss-fuse-fio.argo-case.example.json", &q); err != nil {
		t.Fatal(err)
	}
	c, err := m.Save("cases", q)
	if err != nil {
		t.Fatal(err)
	}
	selection.Case = Ref{c.ID, c.Revision}
	// Exercise the real API JSON decoding path, not an integer-valued Go map.
	if err = json.Unmarshal([]byte(`{"fio_bs_bytes":1048576,"fio_numjobs":4}`), &selection.Parameters); err != nil {
		t.Fatal(err)
	}
	s, err := m.compile(selection)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range s.Definition.Results {
		if r.Parser == "fio-3.33" && (r.ExpectedOptions["bs"] != "1048576" || r.ExpectedOptions["numjobs"] != "4") {
			t.Fatalf("incorrect fio expectations: %#v", r.ExpectedOptions)
		}
	}
}

func TestFioLegacyNumericOptions(t *testing.T) {
	for _, tt := range []struct {
		key, actual, expected string
		match                 bool
	}{
		{"bs", "1048576", "1.048576e+06", true},
		{"bs", "1048575", "1.048576e+06", false},
		{"bs", "9007199254740992", "9007199254740993", false},
		{"bs", "1M", "1048576", false},
		{"bs", "", "1048576", false},
		{"bs", "2/2", "1", false},
		{"bs", "1e99999999", "1", false},
		{"bs", "1.5", "1.50", false},
		{"ioengine", "psync", "libaio", false},
		{"ioengine", "1", "1e0", false},
		{"direct", "0", "0", true},
		{"numjobs", "4", "4e0", true},
	} {
		if got := fioOptionMatches(tt.key, tt.actual, tt.expected); got != tt.match {
			t.Errorf("%+v got %v", tt, got)
		}
	}
}

func TestRetryInvalidEvidenceDoesNotResubmit(t *testing.T) {
	m, q, _ := fixture(t)
	pre, err := m.Preflight(q)
	if err != nil {
		t.Fatal(err)
	}
	r, err := m.Start(Start{pre.ID, pre.Snapshot.Hash, "invalid-evidence"})
	if err != nil {
		t.Fatal(err)
	}
	r.ExternalRunID = "external-run"
	r.ExternalVersionID = "external-version"
	r.ExecutionState = "succeeded"
	r.Status = "failed"
	r.ResultState = "invalid"
	hash := r.Snapshot.Hash
	if err = m.persist(r); err != nil {
		t.Fatal(err)
	}
	next, err := m.RetryCollection(r.ID)
	if err != nil || next.Status != "collecting" || next.CollectionDeadline == nil || next.Snapshot.Hash != hash || next.ExternalRunID != r.ExternalRunID {
		t.Fatal(next, err)
	}
	if _, err = m.RetryCollection(r.ID); err == nil {
		t.Fatal("accepted concurrent collection")
	}
	f := m.backends["test"].(*fakeBackend)
	if f.submits != 0 || f.registers != 0 {
		t.Fatal("recollection submitted new workflow")
	}
}
