package store

import (
	"encoding/json"
	"github.com/wsq2333/malta/internal/domain"
	"testing"
)

func TestCaseListLastTestTime(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	v, err := s.SaveCase("", domain.SaveCase{Name: "read", Plan: sample(t).Run.Plan})
	if err != nil {
		t.Fatal(err)
	}
	cases, err := s.Cases()
	if err != nil || len(cases) != 1 || cases[0].LastRunAt != "" {
		t.Fatal(cases, err)
	}
	for _, ts := range []string{"2026-01-01T00:00:00Z", "2026-01-01T00:00:00.5Z"} {
		raw, _ := json.Marshal(map[string]any{"case_ref": map[string]any{"id": v.CaseID, "revision": 1}})
		if _, err = s.DB.Exec("INSERT INTO runs(id,body,created_at) VALUES(?,?,?)", domain.ID("run"), string(raw), ts); err != nil {
			t.Fatal(err)
		}
	}
	cases, err = s.Cases()
	if err != nil || cases[0].LastRunAt != "2026-01-01T00:00:00.5Z" || !cases[0].UpdatedAt.Equal(v.CreatedAt) {
		t.Fatal(cases, err)
	}
}
