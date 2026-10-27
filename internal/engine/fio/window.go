package fio

import (
	"encoding/csv"
	"github.com/wsq2333/malta/internal/domain"
	"math"
	"strconv"
	"strings"
	"time"
)

// Window reports the first and last nonzero 100ms IOPS sample in Unix milliseconds.
// This is a conservative observed interval, not the process lifetime.
func Window(data []byte) (time.Time, time.Time, error) {
	rows, err := csv.NewReader(strings.NewReader(string(data))).ReadAll()
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	var first, last int64
	for _, row := range rows {
		if len(row) < 3 {
			return time.Time{}, time.Time{}, domain.E("invalid", "invalid IOPS sample")
		}
		ms, e := strconv.ParseInt(strings.TrimSpace(row[0]), 10, 64)
		if e != nil || ms < 1000000000000 {
			return time.Time{}, time.Time{}, domain.E("invalid", "IOPS samples require epoch milliseconds")
		}
		value, e := strconv.ParseFloat(strings.TrimSpace(row[1]), 64)
		if e != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return time.Time{}, time.Time{}, domain.E("invalid", "invalid sample value")
		}
		if value == 0 {
			continue
		}
		if first == 0 || ms < first {
			first = ms
		}
		if ms > last {
			last = ms
		}
	}
	if first == 0 || last <= first {
		return time.Time{}, time.Time{}, domain.E("invalid", "insufficient active IOPS samples")
	}
	return time.UnixMilli(first).UTC(), time.UnixMilli(last).UTC(), nil
}
