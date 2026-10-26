package store

import (
	"fmt"
	"github.com/wsq2333/malta/internal/artifact"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/engine/fio"
	"github.com/wsq2333/malta/internal/engine/metadata"
	"time"
)

func VerifyEvidence(b artifact.Bundle) error {
	for _, t := range b.Run.Trials {
		if t.Status != "succeeded" {
			continue
		}
		var metrics []domain.Metric
		var e error
		if b.Run.Plan.Spec.Kind == "data" || b.Run.Plan.Spec.Kind == "scenario" {
			child, err := b.Run.Plan.WorkloadPlan(t.Point)
			if err != nil {
				return err
			}
			if b.Run.Plan.Spec.Kind == "scenario" {
				log, err := artifact.Get(b, t.Point.ID+"/io-window.log")
				if err != nil {
					return err
				}
				start, end, err := fio.Window(log)
				if err != nil {
					return err
				}
				if !start.Equal(t.StartedAt) || !end.Equal(t.FinishedAt) {
					return domain.E("invalid", "trial window differs from raw samples")
				}
				if end.Sub(start) > time.Duration(child.Spec.Workload.RuntimeSeconds+2)*time.Second {
					return domain.E("invalid", "trial window exceeds requested duration")
				}
			}
			if b.Run.ToolVersion != "fio-3.33" {
				return domain.E("unsupported", "unvalidated fio version")
			}
			raw, err := artifact.Get(b, t.Point.ID+"/measure.json")
			if err != nil {
				return err
			}
			if e = fio.VerifyPlan(raw, child, t.Point); e != nil {
				return e
			}
			metrics, e = fio.Parse(raw)
		} else {
			if b.Run.ToolVersion != "mdtest-4.0.0" {
				return domain.E("unsupported", "unvalidated metadata version")
			}
			raw, err := artifact.Get(b, fmt.Sprintf("metadata/iteration-%d.txt", t.Point.Repeat))
			if err != nil {
				return err
			}
			metrics, e = metadata.Parse(raw, b.Run.Plan.Spec.Metadata)
		}
		if e != nil {
			return e
		}
		if domain.Hash(metrics) != domain.Hash(t.Metrics) {
			return domain.E("invalid", "normalized metrics differ from raw evidence")
		}
	}
	if b.Run.Status == "succeeded" && b.Run.Plan.Spec.Scenario != nil {
		for repeat := 1; repeat <= b.Run.Plan.Spec.Repeats; repeat++ {
			for _, stage := range b.Run.Plan.Spec.Scenario.Stages {
				if stage.Mode != "parallel" {
					continue
				}
				trials := []domain.Trial{}
				for _, t := range b.Run.Trials {
					if t.Point.Repeat == repeat && t.Point.StageID == stage.ID {
						trials = append(trials, t)
					}
				}
				if err := domain.ValidateStageWindow(stage, trials); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
