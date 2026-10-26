package fio

import (
	"encoding/json"
	"fmt"
	"github.com/wsq2333/malta/internal/domain"
	"path/filepath"
	"strconv"
	"strings"
)

func CheckVersion(s string) error {
	if strings.TrimSpace(s) != "fio-3.33" {
		return domain.E("unsupported", "supported fio version is fio-3.33; received "+strings.TrimSpace(s))
	}
	return nil
}
func Args(p domain.Plan, point domain.Point, dir string, prepare bool) []string {
	w := p.Spec.Workload
	a := []string{"--name=malta", "--directory=" + dir, "--filename_format=job.$jobnum.$filenum", "--filesize=" + strconv.FormatInt(w.FileSizeBytes, 10), "--size=" + strconv.FormatInt(w.FileSizeBytes*int64(w.FilesPerJob), 10), fmt.Sprintf("--nrfiles=%d", w.FilesPerJob), fmt.Sprintf("--numjobs=%d", point.NumJobs), "--group_reporting=1", "--output-format=json+", "--eta=never", "--randrepeat=1", fmt.Sprintf("--randseed=%d", w.Seed)}
	if prepare {
		return append(a, "--rw=write", fmt.Sprintf("--bs=%d", w.BlockSizeBytes), "--ioengine=psync", "--iodepth=1", "--end_fsync=1")
	}
	if point.StageID != "" {
		a = append(a, "--write_iops_log="+filepath.Join(dir, "measure"), "--per_job_logs=0", "--log_avg_msec=100", "--log_unix_epoch=1")
	}
	a = append(a, "--rw="+w.Operation, fmt.Sprintf("--bs=%d", w.BlockSizeBytes), "--ioengine="+w.IOEngine, fmt.Sprintf("--iodepth=%d", point.IODepth), fmt.Sprintf("--runtime=%d", w.RuntimeSeconds), fmt.Sprintf("--ramp_time=%d", w.WarmupSeconds), "--time_based=1", "--allow_file_create=0", fmt.Sprintf("--direct=%d", boolInt(w.Direct)), fmt.Sprintf("--fsync=%d", w.FsyncEvery))
	if w.Operation == "randrw" {
		a = append(a, fmt.Sprintf("--rwmixread=%d", w.ReadPercent))
	}
	if w.CacheState == "warm" {
		a = append(a, "--invalidate=0")
	} else {
		a = append(a, "--invalidate=1")
	}
	return a
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

type stats struct {
	Bytes float64  `json:"io_bytes"`
	BW    *float64 `json:"bw_bytes"`
	BWKiB float64  `json:"bw"`
	IOPS  float64  `json:"iops"`
	Lat   struct {
		Percentile map[string]float64 `json:"percentile"`
	} `json:"clat_ns"`
}

func Parse(data []byte) ([]domain.Metric, error) {
	var v struct {
		Version string `json:"fio version"`
		Jobs    []struct {
			Error int   `json:"error"`
			Read  stats `json:"read"`
			Write stats `json:"write"`
		} `json:"jobs"`
	}
	if e := json.Unmarshal(data, &v); e != nil {
		return nil, domain.E("invalid", "invalid fio JSON: "+e.Error())
	}
	if e := CheckVersion(v.Version); e != nil {
		return nil, e
	}
	if len(v.Jobs) != 1 {
		return nil, domain.E("invalid", "expected one fio group_reporting aggregate")
	}
	j := v.Jobs[0]
	if j.Error != 0 {
		return nil, domain.E("execution", fmt.Sprintf("fio job error %d", j.Error))
	}
	out := []domain.Metric{}
	for _, item := range []struct {
		op string
		s  stats
	}{{"read", j.Read}, {"write", j.Write}} {
		s := item.s
		if s.Bytes <= 0 {
			continue
		}
		bw := s.BWKiB * 1024
		if s.BW != nil {
			bw = *s.BW
		}
		out = append(out, domain.Metric{Operation: item.op, Name: "throughput", Unit: "bytes/s", Value: bw, Source: "fio"}, domain.Metric{Operation: item.op, Name: "iops", Unit: "ops/s", Value: s.IOPS, Source: "fio"})
		for k, v := range s.Lat.Percentile {
			q, _ := strconv.ParseFloat(k, 64)
			if q == 99 {
				out = append(out, domain.Metric{Operation: item.op, Name: "clat_p99", Unit: "ns", Value: v, Source: "fio"})
			}
		}
	}
	if len(out) == 0 {
		return nil, domain.E("execution", "fio completed no I/O")
	}
	for _, m := range out {
		if e := m.Validate(); e != nil {
			return nil, e
		}
	}
	return out, nil
}
