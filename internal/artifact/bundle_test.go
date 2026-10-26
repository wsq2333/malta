package artifact

import (
	"github.com/wsq2333/malta/internal/domain"
	"testing"
)

func TestPathAndChecksum(t *testing.T) { // Validate the path check after constructing a valid run from repository examples.
	var s domain.Spec
	var e domain.Environment
	var v domain.SoftwareVersion
	for _, x := range []struct {
		p string
		v any
	}{{"../../benchmarks/data/smoke-read.yaml", &s}, {"../../environments/local.yaml", &e}, {"../../environments/version-example.yaml", &v}} {
		if err := domain.Read(x.p, x.v); err != nil {
			t.Fatal(err)
		}
	}
	p, err := domain.NewPlan(s, e, v, "/tmp/target", "test")
	if err != nil {
		t.Fatal(err)
	}
	b := Bundle{SchemaVersion: domain.Schema, Run: domain.NewRun(p, "ci"), Files: []File{{Path: "../escape", SHA256: domain.BytesHash([]byte("x")), Data: []byte("x")}}}
	if b.Validate() == nil {
		t.Fatal("accepted traversal")
	}
	b.Files[0].Path = "raw.json"
	b.Files[0].Data = []byte("tampered")
	if b.Validate() == nil {
		t.Fatal("accepted checksum mismatch")
	}
}
