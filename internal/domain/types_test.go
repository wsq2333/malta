package domain

import (
	"testing"
)

func FixturePlan(t *testing.T) Plan {
	t.Helper()
	var s Spec
	var e Environment
	var v SoftwareVersion
	for _, x := range []struct {
		p string
		v any
	}{{"../../benchmarks/data/smoke-read.yaml", &s}, {"../../environments/local.yaml", &e}, {"../../environments/version-example.yaml", &v}} {
		if err := Read(x.p, x.v); err != nil {
			t.Fatal(err)
		}
	}
	p, err := NewPlan(s, e, v, "/tmp/malta-target", "smoke")
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func TestPlanValidation(t *testing.T) {
	p := FixturePlan(t)
	p.Spec.Workload.BlockSizeBytes = 8193
	p.Hash = p.Digest()
	if p.Validate() == nil {
		t.Fatal("unaligned block accepted")
	}
	p = FixturePlan(t)
	p.Trials = nil
	p.Hash = p.Digest()
	if p.Validate() == nil {
		t.Fatal("inconsistent matrix accepted")
	}
	p = FixturePlan(t)
	p.SchemaVersion = "2"
	p.Hash = p.Digest()
	if p.Validate() == nil {
		t.Fatal("unknown schema accepted")
	}
}
func TestStrictDecode(t *testing.T) {
	var s Spec
	if Decode([]byte("unknown: true"), &s) == nil {
		t.Fatal("unknown property accepted")
	}
}
func TestMetricUnits(t *testing.T) {
	if (Metric{"read", "throughput", "MiB/s", 1, "fio"}).Validate() == nil {
		t.Fatal("noncanonical unit accepted")
	}
	if (Metric{"read", "clat_p99", "ns", -1, "fio"}).Validate() == nil {
		t.Fatal("negative metric accepted")
	}
}
