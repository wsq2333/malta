package runner

import (
	"context"
	"errors"
	"fmt"
	"github.com/wsq2333/malta/internal/artifact"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/engine/fio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Options struct {
	Native   string
	Fio      string
	Timeout  time.Duration
	MaxBytes int64
	OnUpdate func(domain.Run) error
}

func Execute(ctx context.Context, r domain.Run, out string, o Options) (result domain.Run, retErr error) {
	result = r
	if e := r.Plan.Validate(); e != nil {
		return r, e
	}
	if r.Plan.Spec.Kind != "data" && r.Plan.Spec.Kind != "scenario" {
		return r, domain.E("unsupported", "exec supports fio data plans only")
	}
	if r.Plan.Spec.Workload.CacheState == "cold-verified" {
		return r, domain.E("unsupported", "executor cannot verify cold caches; use cold-requested or unknown")
	}
	if o.Fio == "" {
		o.Fio = "fio"
	}
	if o.Timeout <= 0 {
		o.Timeout = time.Hour
	}
	if o.MaxBytes <= 0 {
		o.MaxBytes = 1 << 30
	}
	bytes, seconds := r.Plan.Budget()
	if bytes < 0 || bytes > o.MaxBytes {
		return r, domain.E("invalid", "planned data exceeds --max-bytes budget")
	}
	if r.Plan.Spec.Scenario != nil && time.Duration(seconds)*time.Second >= o.Timeout {
		return r, domain.E("invalid", "measurement leaves no preparation time within timeout")
	}

	target, e := filepath.EvalSymlinks(r.Plan.Target)
	if e != nil {
		return r, domain.E("invalid", "target must already exist: "+e.Error())
	}
	if target == "/" {
		return r, domain.E("invalid", "root target rejected")
	}
	info, e := os.Stat(target)
	if e != nil || !info.IsDir() {
		return r, domain.E("invalid", "target must be a directory")
	}
	unlock, e := Lock(filepath.Join(target, ".malta-"+r.Plan.ResourceGroup+".lock"))
	if e != nil {
		return r, e
	}
	defer unlock()
	if e = os.MkdirAll(filepath.Dir(out), 0755); e != nil {
		return r, e
	}
	if e = os.Mkdir(out, 0755); e != nil {
		return r, domain.E("conflict", "output directory must not exist")
	}
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	result.Status = "preparing"
	result.Attempt.Status = "preparing"
	result.Attempt.StartedAt = time.Now().UTC()
	update := func() error {
		result.UpdatedAt = time.Now().UTC()
		if e := domain.Write(filepath.Join(out, "run.json"), result); e != nil {
			return e
		}
		if o.OnUpdate != nil {
			return o.OnUpdate(result)
		}
		return nil
	}
	defer func() {
		if retErr != nil {
			result.Status = "failed"
			result.Error = retErr.Error()
			if errors.Is(ctx.Err(), context.Canceled) {
				result.Status = "cancelled"
				retErr = domain.E("cancelled", "execution cancelled")
			} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				retErr = domain.E("timeout", "execution timeout")
			}
			result.Error = retErr.Error()
		} else {
			result.Status = "succeeded"
		}
		result.Attempt.Status = result.Status
		result.Attempt.FinishedAt = time.Now().UTC()
		result.Attempt.ResultState = "complete"
		if e := update(); e != nil && retErr == nil {
			retErr = e
		}
		b, e := artifact.Collect(out, result)
		if e == nil {
			e = domain.Write(filepath.Join(out, "bundle.json"), b)
		}
		if e != nil && retErr == nil {
			retErr = e
		}
	}()
	if e = update(); e != nil {
		return result, e
	}
	if result.Plan.Spec.Scenario == nil || domain.ScenarioUsesFio(result.Plan.Spec.Scenario) {
		v, e := exec.CommandContext(ctx, o.Fio, "--version").CombinedOutput()
		_ = os.WriteFile(filepath.Join(out, "tool-version.txt"), v, 0644)
		if e != nil {
			return result, domain.E("execution", "fio version check failed: "+e.Error())
		}
		result.ToolVersion = strings.TrimSpace(string(v))
		if e = fio.CheckVersion(result.ToolVersion); e != nil {
			return result, e
		}
	}
	if result.Plan.Spec.Scenario != nil {
		result.ToolVersion = domain.ScenarioTools(result.Plan.Spec.Scenario)
	}
	if e = domain.Write(filepath.Join(out, "plan.json"), r.Plan); e != nil {
		return result, e
	}
	dataDir := filepath.Join(target, result.ID+"-"+result.Attempt.ID)
	if e = os.Mkdir(dataDir, 0700); e != nil {
		return result, e
	}
	if e = domain.Write(filepath.Join(out, "data-paths.json"), map[string]string{"created_directory": dataDir, "cleanup": "retained; remove this dedicated directory after inspection"}); e != nil {
		return result, e
	}
	if result.Plan.Spec.Scenario != nil {
		err := executeScenario(ctx, &result, out, dataDir, o, update)
		return result, err
	}
	for _, p := range result.Plan.Trials {
		t := domain.Trial{Point: p, Status: "preparing", StartedAt: time.Now().UTC(), Metrics: []domain.Metric{}}
		dir := filepath.Join(dataDir, p.ID)
		if e = os.Mkdir(dir, 0700); e != nil {
			return result, e
		}
		evidence := filepath.Join(out, p.ID)
		if e = os.Mkdir(evidence, 0755); e != nil {
			return result, e
		}
		for _, phase := range []string{"prepare", "measure"} {
			if phase == "measure" {
				result.Status = "running"
				result.Attempt.Status = "running"
			}
			if e = update(); e != nil {
				return result, e
			}
			args := fio.Args(result.Plan, p, dir, phase == "prepare")
			if e = domain.Write(filepath.Join(evidence, phase+"-command.json"), append([]string{o.Fio}, args...)); e != nil {
				return result, e
			}
			stdout, e := os.Create(filepath.Join(evidence, phase+".json"))
			if e != nil {
				return result, e
			}
			stderr, e := os.Create(filepath.Join(evidence, phase+".stderr"))
			if e != nil {
				stdout.Close()
				return result, e
			}
			e = process(ctx, o.Fio, args, stdout, stderr)
			stdout.Close()
			stderr.Close()
			if e != nil {
				t.Status = "failed"
				t.Error = e.Error()
				t.FinishedAt = time.Now().UTC()
				result.Trials = append(result.Trials, t)
				return result, fmt.Errorf("%s %s: %w", p.ID, phase, e)
			}
		}
		data, e := os.ReadFile(filepath.Join(evidence, "measure.json"))
		if e == nil {
			t.Metrics, e = fio.Parse(data)
		}
		t.FinishedAt = time.Now().UTC()
		t.Status = "succeeded"
		if e != nil {
			t.Status = "failed"
			t.Error = e.Error()
		}
		result.Trials = append(result.Trials, t)
		if e != nil {
			return result, e
		}
		if e = update(); e != nil {
			return result, e
		}
	}
	return result, nil
}
