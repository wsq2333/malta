package metadata

import (
	"fmt"
	"github.com/wsq2333/malta/internal/artifact"
	"github.com/wsq2333/malta/internal/domain"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var rate = regexp.MustCompile(`(?m)^\s*File (creation|stat|removal)\s+([0-9.]+)\s+([0-9.]+)\s+([0-9.]+)\s+([0-9.]+)\s*$`)

func Parse(raw []byte, conditions map[string]string) ([]domain.Metric, error) {
	s := string(raw)
	validCommand := false
	for _, line := range strings.Split(s, "\n") {
		if !strings.HasPrefix(line, "Command line used: ") {
			continue
		}
		args := strings.Fields(strings.TrimPrefix(line, "Command line used: "))
		flags := map[string]string{}
		validCommand = true
		for i := 1; i < len(args); i++ {
			key := strings.Trim(args[i], "'\"")
			if _, exists := flags[key]; exists {
				validCommand = false
				break
			}
			switch key {
			case "-F":
				flags[key] = "true"
			case "-d", "-n", "-i":
				i++
				if i >= len(args) {
					validCommand = false
					break
				}
				flags[key] = strings.Trim(args[i], "'\"")
			default:
				validCommand = false
			}
			if !validCommand {
				break
			}
		}
		validCommand = validCommand && flags["-F"] == "true" && flags["-i"] == "1" && flags["-n"] == conditions["files_per_task"] && flags["-d"] != ""
		break
	}
	if !validCommand {
		return nil, domain.E("unsupported", "supported mdtest command is -d PATH -n FILES -i 1 -F, without other options")
	}
	if !strings.Contains(s, "mdtest-4.0.0 was launched with 1 total task(s)") || strings.Count(s, "SUMMARY rate: (of 1 iterations)") != 1 || !strings.Contains(s, "1 tasks, "+conditions["files_per_task"]+" files") || !strings.Contains(s, "-- finished at") {
		return nil, domain.E("unsupported", "requires complete mdtest 4.0.0 one-task, single-iteration file summary matching declared file count")
	}
	out := []domain.Metric{}
	seen := map[string]bool{}
	names := map[string]string{"creation": "create", "stat": "stat", "removal": "remove"}
	for _, m := range rate.FindAllStringSubmatch(s, -1) {
		if seen[m[1]] {
			return nil, domain.E("invalid", "duplicate mdtest summary")
		}
		seen[m[1]] = true
		max, e := strconv.ParseFloat(m[2], 64)
		if e != nil {
			return nil, e
		}
		min, _ := strconv.ParseFloat(m[3], 64)
		mean, _ := strconv.ParseFloat(m[4], 64)
		std, _ := strconv.ParseFloat(m[5], 64)
		if max != min || min != mean || std != 0 {
			return nil, domain.E("invalid", "mdtest summary is not a single independent iteration")
		}
		out = append(out, domain.Metric{Operation: names[m[1]], Name: "metadata_rate", Unit: "ops/s", Value: mean, Source: "mdtest"})
	}
	if len(out) != 3 {
		return nil, domain.E("invalid", "missing create/stat/remove rates")
	}
	return out, nil
}
func Import(p domain.Plan, raws [][]byte) (artifact.Bundle, error) {
	b := artifact.Bundle{SchemaVersion: domain.Schema, Run: domain.NewRun(p, "import"), Files: []artifact.File{}}
	if e := p.Validate(); e != nil {
		return b, e
	}
	if p.Spec.Kind != "metadata" || len(raws) != p.Spec.Repeats {
		return b, domain.E("invalid", "one raw single-iteration mdtest output is required per planned repeat")
	}
	now := time.Now().UTC()
	b.Run.ToolVersion = "mdtest-4.0.0"
	b.Run.Status = "succeeded"
	b.Run.Attempt.Status = "succeeded"
	b.Run.Attempt.StartedAt = now
	b.Run.Attempt.FinishedAt = now
	b.Run.Attempt.ResultState = "complete"
	for i, raw := range raws {
		metrics, e := Parse(raw, p.Spec.Metadata)
		if e != nil {
			return b, e
		}
		b.Run.Trials = append(b.Run.Trials, domain.Trial{Point: p.Trials[i], Status: "succeeded", StartedAt: now, FinishedAt: now, Metrics: metrics})
		b.Files = append(b.Files, artifact.File{Path: fmt.Sprintf("metadata/iteration-%d.txt", i+1), SHA256: domain.BytesHash(raw), Data: raw})
	}
	return b, b.Validate()
}
