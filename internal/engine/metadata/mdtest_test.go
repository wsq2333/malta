package metadata

import (
	"os"
	"strings"
	"testing"
)

func TestRealFixture(t *testing.T) {
	b, e := os.ReadFile("../../../testdata/mdtest/mdtest-4.0.0.txt")
	if e != nil {
		t.Fatal(e)
	}
	c := map[string]string{"files_per_task": "8"}
	m, e := Parse(b, c)
	if e != nil || len(m) != 3 {
		t.Fatal(m, e)
	}
	if m[0].Value != 1571.121 || m[0].Operation != "create" {
		t.Fatal(m)
	}
	for _, s := range []string{strings.ReplaceAll(string(b), "of 1 iterations", "of 3 iterations"), strings.ReplaceAll(string(b), "mdtest-4.0.0", "mdtest-3.0"), strings.ReplaceAll(string(b), "File stat", "Other stat")} {
		if _, e = Parse([]byte(s), c); e == nil {
			t.Fatal("accepted unsupported or incomplete data")
		}
	}
	if _, e = Parse(b, map[string]string{"files_per_task": "100"}); e == nil {
		t.Fatal("ignored declared workload")
	}
}
