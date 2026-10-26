package runner

import (
	"github.com/wsq2333/malta/internal/domain"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestConcurrentPodsClaimAttemptOnlyOnce(t *testing.T) {
	r := domain.NewRun(plan(t), "ci")
	root := t.TempDir()
	base := filepath.Join(root, r.Attempt.ID)
	if e := os.Mkdir(base, 0755); e != nil {
		t.Fatal(e)
	}
	if _, e := ClaimAttempt(root, r, "wrong-mount"); e == nil {
		t.Fatal("missing handshake accepted")
	}
	if e := domain.Write(filepath.Join(base, "handshake.json"), r.Plan.Hash); e != nil {
		t.Fatal(e)
	}
	var winners atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			if _, e := ClaimAttempt(root, r, "pod"); e == nil {
				winners.Add(1)
			}
		})
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("%d Pods won", winners.Load())
	}
	if _, e := ClaimAttempt(root, r, "replacement"); e == nil {
		t.Fatal("replacement replayed")
	}
}
