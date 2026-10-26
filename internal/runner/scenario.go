package runner

import (
	"context"
	"fmt"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/engine/fio"
	"os"
	"path/filepath"
	"sync"
	"time"
)

func fioPhase(ctx context.Context, plan domain.Plan, point domain.Point, data, out, bin, phase string) error {
	args := fio.Args(plan, point, data, phase == "prepare")
	if err := domain.Write(filepath.Join(out, phase+"-command.json"), append([]string{bin}, args...)); err != nil {
		return err
	}
	stdout, err := os.Create(filepath.Join(out, phase+".json"))
	if err != nil {
		return err
	}
	defer stdout.Close()
	stderr, err := os.Create(filepath.Join(out, phase+".stderr"))
	if err != nil {
		return err
	}
	defer stderr.Close()
	return process(ctx, bin, args, stdout, stderr)
}
func executeScenario(ctx context.Context, result *domain.Run, out, data, bin string, update func() error) error {
	sc := result.Plan.Spec.Scenario
	for repeat := 1; repeat <= sc.Repeats; repeat++ {
		for _, stage := range sc.Stages {
			points := []domain.Point{}
			for _, p := range result.Plan.Trials {
				if p.Repeat == repeat && p.StageID == stage.ID {
					points = append(points, p)
				}
			}
			// All preparation completes before any child enters the measurement window.
			result.Status = "preparing"
			result.Attempt.Status = "preparing"
			if err := update(); err != nil {
				return err
			}
			for _, point := range points {
				child, _ := result.Plan.WorkloadPlan(point)
				dir := filepath.Join(data, point.ID)
				ev := filepath.Join(out, point.ID)
				if err := os.Mkdir(dir, 0700); err != nil {
					return err
				}
				if err := os.Mkdir(ev, 0755); err != nil {
					return err
				}
				if err := fioPhase(ctx, child, point, dir, ev, bin, "prepare"); err != nil {
					result.Trials = append(result.Trials, domain.Trial{Point: point, Status: "failed", Error: err.Error(), Metrics: []domain.Metric{}})
					return fmt.Errorf("%s preparation: %w", point.ID, err)
				}
			}
			result.Status = "running"
			result.Attempt.Status = "running"
			if err := update(); err != nil {
				return err
			}
			measured := make([]domain.Trial, len(points))
			errs := make([]error, len(points))
			group, stop := context.WithCancel(ctx)
			measure := func(i int) {
				point := points[i]
				child, _ := result.Plan.WorkloadPlan(point)
				dir := filepath.Join(data, point.ID)
				ev := filepath.Join(out, point.ID)
				t := domain.Trial{Point: point, Status: "failed", StartedAt: time.Now().UTC(), Metrics: []domain.Metric{}}
				err := fioPhase(group, child, point, dir, ev, bin, "measure")
				log, logErr := os.ReadFile(filepath.Join(dir, "measure_iops.log"))
				if logErr == nil {
					logErr = os.WriteFile(filepath.Join(ev, "io-window.log"), log, 0644)
				}
				if err == nil {
					err = logErr
				}
				if err == nil {
					t.StartedAt, t.FinishedAt, err = fio.Window(log)
				}
				if err == nil {
					raw, e := os.ReadFile(filepath.Join(ev, "measure.json"))
					err = e
					if err == nil {
						err = fio.VerifyPlan(raw, child, point)
					}
					if err == nil {
						t.Metrics, err = fio.Parse(raw)
					}
				}
				if err != nil {
					t.Error = err.Error()
					if t.FinishedAt.IsZero() {
						t.FinishedAt = time.Now().UTC()
					}
					stop()
				} else {
					t.Status = "succeeded"
				}
				measured[i] = t
				errs[i] = err
			}
			if stage.Mode == "parallel" {
				var wg sync.WaitGroup
				start := make(chan struct{})
				for i := range points {
					wg.Add(1)
					go func(i int) { defer wg.Done(); <-start; measure(i) }(i)
				}
				close(start)
				wg.Wait()
			} else {
				for i := range points {
					measure(i)
					if errs[i] != nil {
						break
					}
				}
			}
			stop()
			for _, t := range measured {
				if t.Point.ID != "" {
					result.Trials = append(result.Trials, t)
				}
			}
			for _, err := range errs {
				if err != nil {
					return err
				}
			}
			if stage.Mode == "parallel" {
				if err := domain.ValidateStageWindow(stage, measured); err != nil {
					return err
				}
			}
			if err := update(); err != nil {
				return err
			}
		}
	}
	return nil
}
