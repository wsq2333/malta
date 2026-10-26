package fio

import (
	"encoding/json"
	"os"
	"testing"
)

func TestFixture(t *testing.T) {
	b, e := os.ReadFile("../../../testdata/fio/fio-3.33.json")
	if e != nil {
		t.Fatal(e)
	}
	metrics, e := Parse(b)
	if e != nil {
		t.Fatal(e)
	}
	if len(metrics) != 6 {
		t.Fatalf("expected independent read and write metrics: %v", metrics)
	}
	var raw struct {
		Jobs []struct {
			Read struct {
				BW   float64 `json:"bw_bytes"`
				IOPS float64 `json:"iops"`
			} `json:"read"`
		} `json:"jobs"`
	}
	_ = json.Unmarshal(b, &raw)
	if metrics[0].Value != raw.Jobs[0].Read.BW || metrics[1].Value != raw.Jobs[0].Read.IOPS {
		t.Fatal("unit conversion corrupted aggregate")
	}
}
func TestRejectFailedAndUnaggregated(t *testing.T) {
	for _, s := range []string{`{"fio version":"fio-3.33","jobs":[{"error":5}]}`, `{"fio version":"fio-3.33","jobs":[{},{}]}`, `{"fio version":"fio-3.40","jobs":[{}]}`} {
		if _, e := Parse([]byte(s)); e == nil {
			t.Fatal("accepted invalid result")
		}
	}
}
