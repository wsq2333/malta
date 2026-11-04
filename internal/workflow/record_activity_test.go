package workflow

import (
	"encoding/json"
	"github.com/wsq2333/malta/internal/domain"
	"testing"
)

func TestCaseListActivityDoesNotChangeSnapshots(t *testing.T) {
	m, q, _ := fixture(t)
	before, err := m.Record("cases", q.Case)
	if err != nil {
		t.Fatal(err)
	}
	records, err := m.Records("cases", "")
	if err != nil || len(records) != 1 || records[0].LastRunAt != "" {
		t.Fatal(records, err)
	}
	// Whole-second and fractional timestamps must sort chronologically, across case revisions.
	for i, ts := range []string{"2026-01-01T00:00:00Z", "2026-01-01T00:00:00.5Z"} {
		raw, _ := json.Marshal(map[string]any{"created_at": ts, "snapshot": map[string]any{"case": map[string]any{"id": q.Case.ID, "revision": i + 1}}})
		if _, err = m.Store.DB.Exec("INSERT INTO workflow_runs(id,request_id,body,updated_at) VALUES(?,?,?,?)", domain.ID("run"), domain.ID("request"), string(raw), ts); err != nil {
			t.Fatal(err)
		}
	}
	records, err = m.Records("cases", "")
	if err != nil || records[0].LastRunAt != "2026-01-01T00:00:00.5Z" {
		t.Fatal(records, err)
	}
	systems, err := m.Records("systems", "")
	if err != nil || systems[0].LastRunAt != "" {
		t.Fatal(systems, err)
	}
	after, _ := m.Record("cases", q.Case)
	if domain.Hash(before) != domain.Hash(after) {
		t.Fatal("listing changed immutable record")
	}
}
