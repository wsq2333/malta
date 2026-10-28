package cli

import (
	"bytes"
	"encoding/json"
	"github.com/wsq2333/malta/internal/domain"
	"path/filepath"
	"testing"
)

func TestIssueCLI(t *testing.T) {
	dir := t.TempDir()
	var q domain.SaveIssue
	if e := domain.Read("../../web/src/issue-example.json", &q); e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(dir, "issue.json")
	run := func(want int, args ...string) json.RawMessage {
		t.Helper()
		var out, err bytes.Buffer
		code := Execute(append([]string{"--data-dir", filepath.Join(dir, "db")}, args...), &out, &err)
		if code != want {
			t.Fatalf("%v: exit %d %s", args, code, out.String())
		}
		var env struct {
			Data json.RawMessage `json:"data"`
		}
		json.Unmarshal(out.Bytes(), &env)
		return env.Data
	}
	if e := domain.Write(file, q); e != nil {
		t.Fatal(e)
	}
	run(0, "issue", "save", "--file", file)
	q.BaseRevision = 1
	q.Type = "自定义类型"
	q.Resolved = true
	q.Resolution = "verified fix"
	domain.Write(file, q)
	run(0, "issue", "save", "--file", file)
	run(6, "issue", "save", "--file", file)
	var latest domain.Issue
	json.Unmarshal(run(0, "issue", "get", q.IssueID), &latest)
	if latest.Revision != 2 || latest.Type != q.Type {
		t.Fatal(latest)
	}
	var history []domain.Issue
	json.Unmarshal(run(0, "issue", "history", q.IssueID), &history)
	if len(history) != 2 || history[1].Resolved {
		t.Fatal(history)
	}
	var list []domain.Issue
	json.Unmarshal(run(0, "issue", "list"), &list)
	if len(list) != 1 {
		t.Fatal(list)
	}
	run(2, "issue", "get", "../bad")
}
