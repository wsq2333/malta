package store

import (
	"fmt"
	"github.com/wsq2333/malta/internal/artifact"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/engine/fio"
	"github.com/wsq2333/malta/internal/engine/metadata"
)

func VerifyEvidence(b artifact.Bundle) error {
	for _, t := range b.Run.Trials {
		if t.Status != "succeeded" {
			continue
		}
		var metrics []domain.Metric
		var e error
		if b.Run.Plan.Spec.Kind == "data" {
			if b.Run.ToolVersion != "fio-3.33" {
				return domain.E("unsupported", "unvalidated fio version")
			}
			raw, err := artifact.Get(b, t.Point.ID+"/measure.json")
			if err != nil {
				return err
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
	return nil
}
