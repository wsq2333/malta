package runner

import (
	"github.com/wsq2333/malta/internal/domain"
	"os"
	"path/filepath"
)

// ClaimAttempt requires an existing controller handshake. The claim is never removed,
// including after failure, so replacement Pods cannot replay an attempt.
func ClaimAttempt(results string, r domain.Run, podUID string) (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	base := filepath.Join(results, r.Attempt.ID)
	var handshake string
	if err := domain.Read(filepath.Join(base, "handshake.json"), &handshake); err != nil {
		return "", err
	}
	if handshake != r.Plan.Hash {
		return "", domain.E("conflict", "shared results handshake mismatch")
	}
	if err := os.Mkdir(filepath.Join(base, "claimed"), 0755); err != nil {
		return "", domain.E("conflict", "attempt already claimed or results unwritable; automatic re-execution prohibited")
	}
	if err := domain.Write(filepath.Join(base, "claimed", "pod.json"), map[string]string{"uid": podUID}); err != nil {
		return "", err
	}
	return filepath.Join(base, "output"), nil
}
