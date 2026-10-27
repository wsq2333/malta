package fio

import (
	"encoding/json"
	"github.com/wsq2333/malta/internal/domain"
	"strings"
)

// VerifyPlan checks the actual tool-reported options instead of trusting an import's labels.
func VerifyPlan(raw []byte, p domain.Plan, point domain.Point) error {
	var v struct {
		Jobs []struct {
			Options map[string]string `json:"job options"`
		} `json:"jobs"`
	}
	if e := json.Unmarshal(raw, &v); e != nil {
		return e
	}
	if len(v.Jobs) != 1 {
		return domain.E("invalid", "fio aggregate missing")
	}
	actual := v.Jobs[0].Options
	for _, arg := range Args(p, point, "unused", false) {
		key, value, ok := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		if !ok || key == "directory" || key == "write_iops_log" || key == "output-format" || key == "eta" {
			continue
		}
		if actual[key] != value {
			return domain.E("invalid", "fio raw option does not match plan: "+key)
		}
	}
	return nil
}
