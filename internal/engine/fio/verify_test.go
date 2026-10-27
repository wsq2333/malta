package fio

import (
	"encoding/json"
	"github.com/wsq2333/malta/internal/domain"
	"strings"
	"testing"
)

func TestRejectMislabeledPlan(t *testing.T) {
	p := domain.Plan{Spec: domain.Spec{Workload: domain.Workload{Operation: "read", FileSizeBytes: 8192, BlockSizeBytes: 4096, FilesPerJob: 1, RuntimeSeconds: 1, IOEngine: "psync", CacheState: "warm"}}}
	point := domain.Point{NumJobs: 1, IODepth: 1}
	options := map[string]string{}
	for _, arg := range Args(p, point, "unused", false) {
		k, v, _ := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		options[k] = v
	}
	raw, _ := json.Marshal(map[string]any{"jobs": []any{map[string]any{"job options": options}}})
	if e := VerifyPlan(raw, p, point); e != nil {
		t.Fatal(e)
	}
	p.Spec.Workload.Operation = "write"
	if e := VerifyPlan(raw, p, point); e == nil {
		t.Fatal("raw read mislabeled as write")
	}
}
