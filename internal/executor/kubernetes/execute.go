package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/wsq2333/malta/internal/artifact"
	"github.com/wsq2333/malta/internal/domain"
)

var ErrDetached = errors.New("Kubernetes run detached; reconcile the same attempt on restart")

type Options struct {
	Timeout   time.Duration
	MaxBytes  int64
	Update    func(domain.Run) error
	Cancelled func() bool
}
type meta struct {
	UID    string            `json:"uid"`
	Labels map[string]string `json:"labels"`
}
type jobState struct {
	Metadata meta `json:"metadata"`
	Status   struct {
		Conditions []struct{ Type, Status, Reason, Message string }
	}
}
type podState struct {
	Metadata meta
	Spec     struct {
		NodeName string `json:"nodeName"`
	}
	Status struct {
		Phase             string
		ContainerStatuses []struct {
			RestartCount int    `json:"restartCount"`
			ImageID      string `json:"imageID"`
			State        struct {
				Terminated *struct {
					ExitCode int `json:"exitCode"`
					Reason   string
				}
			}
		} `json:"containerStatuses"`
	}
}

func (e *Executor) owned(ctx context.Context, kind string, r domain.Run) error {
	var o struct{ Metadata meta }
	if err := e.get(ctx, kind, Name(r), &o); err != nil {
		return err
	}
	if o.Metadata.Labels["malta.attempt"] != r.Attempt.ID || o.Metadata.Labels["malta.run"] != r.ID || o.Metadata.Labels["malta.plan"] != r.Plan.Hash[:32] {
		return domain.E("conflict", "existing Kubernetes object has different ownership")
	}
	return nil
}
func (e *Executor) create(ctx context.Context, obj object, kind string, r domain.Run) error {
	if _, err := e.Transport.Do(ctx, obj, "create", "-f", "-", "-o", "json"); err != nil {
		if check := e.owned(ctx, kind, r); check != nil {
			return fmt.Errorf("create %s: %v; reconcile: %w", kind, err, check)
		}
	}
	return nil
}
func (e *Executor) Execute(ctx context.Context, r domain.Run, out string, o Options) (domain.Run, error) {
	if err := e.Validate(r.Plan); err != nil {
		return r, err
	}
	if r.Attempt.BackendHash != "" && r.Attempt.BackendHash != domain.Hash(e.Config) {
		return r, domain.E("conflict", "executor configuration changed; restore original configuration to reconcile attempt")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		return r, err
	}
	shared := filepath.Join(e.Config.ResultsDir, r.Attempt.ID)
	if r.Attempt.JobName == "" {
		snapshot, err := e.Preflight(ctx)
		if err != nil {
			return r, err
		}
		if err = domain.Write(filepath.Join(out, "environment-observed.json"), snapshot); err != nil {
			return r, err
		}
		if err = os.MkdirAll(shared, 0755); err != nil {
			return r, err
		}
		if err = domain.Write(filepath.Join(shared, "handshake.json"), r.Plan.Hash); err != nil {
			return r, err
		}
		r.Attempt.JobName = Name(r)
		r.Attempt.Namespace = e.Config.Namespace
		r.Attempt.BackendHash = domain.Hash(e.Config)
		r.Attempt.StartedAt = time.Now().UTC()
		// Persist intent before creating anything; reconciliation always uses these deterministic names.
		if err = o.Update(r); err != nil {
			return r, err
		}
	}
	cm, job := e.Objects(r, o.Timeout, o.MaxBytes)
	// A missing Job is created only if no Job UID has ever been recorded. The durable claim also prevents replacement Pods running fio twice.
	if r.Attempt.JobUID == "" {
		if err := e.create(ctx, cm, "configmap", r); err != nil {
			return r, err
		}
		if err := e.create(ctx, job, "job", r); err != nil {
			return r, err
		}
	}
	cancelled := r.Status == "cancelling"
	stopping := false
	stopDeadline := time.Time{}
	deadline := r.Attempt.StartedAt.Add(o.Timeout + 90*time.Second)
	for {
		if ctx.Err() != nil {
			return r, ErrDetached
		}
		if o.Cancelled != nil && o.Cancelled() {
			cancelled = true
		}
		if (cancelled || time.Now().After(deadline)) && !stopping {
			patch := `{"spec":{"suspend":true}}`
			if _, err := e.Transport.Do(ctx, nil, "patch", "job", Name(r), "--type=merge", "-p", patch); err != nil {
				return r, err
			}
			stopping = true
			stopDeadline = time.Now().Add(60 * time.Second)
		}
		var state jobState
		if err := e.get(ctx, "job", Name(r), &state); err != nil {
			return r, err
		}
		if state.Metadata.Labels["malta.attempt"] != r.Attempt.ID || (r.Attempt.JobUID != "" && r.Attempt.JobUID != state.Metadata.UID) {
			return r, domain.E("conflict", "Job identity changed")
		}
		r.Attempt.JobUID = state.Metadata.UID
		var list struct{ Items []podState }
		raw, err := e.Transport.Do(ctx, nil, "get", "pods", "-l", "job-name="+Name(r), "-o", "json")
		if err != nil {
			return r, err
		}
		if err = json.Unmarshal(raw, &list); err != nil {
			return r, err
		}
		active := false
		for _, pod := range list.Items {
			if pod.Metadata.Labels["malta.attempt"] != r.Attempt.ID {
				return r, domain.E("conflict", "Pod identity changed")
			}
			found := false
			for _, uid := range r.Attempt.PodUIDs {
				if uid == pod.Metadata.UID {
					found = true
				}
			}
			if !found {
				r.Attempt.PodUIDs = append(r.Attempt.PodUIDs, pod.Metadata.UID)
			}
			if pod.Spec.NodeName != "" && pod.Spec.NodeName != e.Config.Node {
				return r, domain.E("conflict", "Pod scheduled on unexpected node")
			}
			if pod.Status.Phase == "Running" || pod.Status.Phase == "Pending" || pod.Status.Phase == "Unknown" {
				active = true
			}
			for _, c := range pod.Status.ContainerStatuses {
				r.Attempt.Restarts = c.RestartCount
				r.Attempt.ImageID = c.ImageID
				if c.State.Terminated != nil {
					code := c.State.Terminated.ExitCode
					r.Attempt.ExitCode = &code
				}
			}
		}
		if err = domain.Write(filepath.Join(out, "job.json"), state); err != nil {
			return r, err
		}
		if err = domain.Write(filepath.Join(out, "pods.json"), list); err != nil {
			return r, err
		}
		if logs, logErr := e.Transport.Do(ctx, nil, "logs", "job/"+Name(r), "--all-containers=true", "--tail=200", "--limit-bytes=65536"); logErr == nil {
			if err = os.WriteFile(filepath.Join(out, "runner.stderr"), logs, 0644); err != nil {
				return r, err
			}
		}
		complete, failed := false, false
		for _, condition := range state.Status.Conditions {
			if condition.Status == "True" {
				if condition.Type == "Complete" {
					complete = true
				}
				if condition.Type == "Failed" {
					failed = true
					r.Error = condition.Reason + ": " + condition.Message
				}
			}
		}
		if (complete || failed || stopping) && !active {
			if !complete && !failed && !stopping {
				continue
			}
			bundle, bundleErr := artifact.Read(filepath.Join(shared, "output"))
			if bundleErr == nil {
				if bundle.Run.ID != r.ID || bundle.Run.Attempt.ID != r.Attempt.ID || bundle.Run.Plan.Hash != r.Plan.Hash {
					return r, domain.E("conflict", "result identity differs from submitted run")
				}
				r.Trials = bundle.Run.Trials
				r.ToolVersion = bundle.Run.ToolVersion
				for _, file := range bundle.Files {
					path := filepath.Join(out, filepath.FromSlash(file.Path))
					if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
						return r, err
					}
					if err = os.WriteFile(path, file.Data, 0644); err != nil {
						return r, err
					}
				}
			}
			r.Status = "failed"
			r.Attempt.ResultState = "complete"
			if cancelled {
				r.Status = "cancelled"
				r.Error = "cancel requested; Job suspended and Pods stopped"
			} else if complete && bundleErr == nil && bundle.Run.Status == "succeeded" {
				r.Status = "succeeded"
				r.Error = ""
			} else if bundleErr != nil {
				r.Error = "runner evidence unavailable: " + bundleErr.Error()
				r.Attempt.ResultState = "failed"
			} else if r.Error == "" {
				r.Error = bundle.Run.Error
				if r.Error == "" {
					r.Error = "Job stopped before successful completion"
				}
			}
			r.Attempt.Status = r.Status
			r.Attempt.FinishedAt = time.Now().UTC()
			return r, nil
		}
		if stopping && time.Now().After(stopDeadline) {
			r.Attempt.ResultState = "failed"
			return r, domain.E("execution", "Pod termination unconfirmed; Job retained; inspect cluster before scheduling further load")
		}
		r.Status = "running"
		if cancelled {
			r.Status = "cancelling"
		}
		r.Attempt.Status = r.Status
		r.UpdatedAt = time.Now().UTC()
		if err = o.Update(r); err != nil {
			return r, err
		}
		select {
		case <-ctx.Done():
			return r, ErrDetached
		case <-time.After(time.Second):
		}
	}
}

// Cleanup is called only after Store.Finish confirms evidence persistence.
func (e *Executor) Cleanup(ctx context.Context, r domain.Run) error {
	if r.Attempt.ResultState != "complete" {
		return nil
	}
	if err := e.owned(ctx, "job", r); err != nil {
		return err
	}
	if _, err := e.Transport.Do(ctx, nil, "delete", "job", Name(r), "--wait=false"); err != nil {
		return err
	}
	_, err := e.Transport.Do(ctx, nil, "delete", "configmap", Name(r), "--ignore-not-found=true")
	return err
}
