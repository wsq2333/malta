//go:build linux || darwin

package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCancellationKillsChildGroup(t *testing.T) {
	dir := t.TempDir()
	out, e := os.Create(filepath.Join(dir, "stdout"))
	if e != nil {
		t.Fatal(e)
	}
	defer out.Close()
	errFile, e := os.Create(filepath.Join(dir, "stderr"))
	if e != nil {
		t.Fatal(e)
	}
	defer errFile.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- process(ctx, "/bin/sh", []string{"-c", "sleep 30 & child=$!; echo $child; wait"}, out, errFile)
	}()
	var pid int
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(out.Name())
		pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		if pid > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("child did not start")
	}
	cancel()
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("cancelled process succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("process group did not terminate")
	}
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state, e := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
		if e != nil || strings.HasPrefix(strings.TrimSpace(string(state)), "Z") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("benchmark child still running after cancellation")
}
