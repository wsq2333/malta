package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/wsq2333/malta/internal/artifact"
	"github.com/wsq2333/malta/internal/domain"
	kube "github.com/wsq2333/malta/internal/executor/kubernetes"
	"github.com/wsq2333/malta/internal/runner"
	"github.com/wsq2333/malta/internal/store"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Config struct {
	ManagementOnly bool
	Kubernetes     *kube.Executor
	AllowedRoots   []string
	Fio            string
	MaxBytes       int64
	Timeout        time.Duration
}
type Manager struct {
	Store  *store.Store
	Config Config
	mu     sync.Mutex
	active string
	cancel context.CancelFunc
	ctx    context.Context
	stop   context.CancelFunc
	done   chan struct{}
	unlock func()
}
type StartRequest struct {
	CaseRef   *domain.CaseRef `json:"case_ref,omitempty"`
	Plan      domain.Plan     `json:"plan,omitzero"`
	RequestID string          `json:"request_id"`
	Source    string          `json:"source"`
	Executor  string          `json:"executor"`
}

func New(s *store.Store, c Config) (*Manager, error) {
	if c.ManagementOnly && (len(c.AllowedRoots) > 0 || c.Kubernetes != nil) {
		return nil, domain.E("invalid", "management-only cannot configure execution backends")
	}
	if !c.ManagementOnly && len(c.AllowedRoots) == 0 && c.Kubernetes == nil {
		return nil, domain.E("invalid", "at least one --allow-target is required")
	}
	if c.MaxBytes <= 0 || c.Timeout <= 0 {
		return nil, domain.E("invalid", "positive byte and timeout budgets required")
	}
	for i, p := range c.AllowedRoots {
		abs, e := filepath.Abs(p)
		if e != nil {
			return nil, e
		}
		abs, e = filepath.EvalSymlinks(abs)
		if e != nil {
			return nil, e
		}
		if abs == "/" {
			return nil, domain.E("invalid", "root filesystem cannot be an allowed target")
		}
		c.AllowedRoots[i] = abs
	}
	unlock, e := runner.Lock(filepath.Join(s.Dir, "server.lock"))
	if e != nil {
		return nil, e
	}
	if !c.ManagementOnly {
		if e = s.Recover(); e != nil {
			unlock()
			return nil, e
		}
	}
	ctx, stop := context.WithCancel(context.Background())
	m := &Manager{Store: s, Config: c, ctx: ctx, stop: stop, done: make(chan struct{}), unlock: unlock}
	go m.loop()
	return m, nil
}
func (m *Manager) Validate(p domain.Plan) error { return m.validate(p, "local") }
func (m *Manager) validate(p domain.Plan, executor string) error {
	if m.Config.ManagementOnly {
		return domain.E("unsupported", "run execution is disabled in management-only mode")
	}
	if e := p.Validate(); e != nil {
		return e
	}
	if (p.Spec.Kind != "data" && p.Spec.Kind != "scenario") || p.Spec.Workload.CacheState == "cold-verified" {
		return domain.E("unsupported", "local runner supports data plans without cold-verified claims")
	}
	if executor == "kubernetes" {
		if m.Config.Kubernetes == nil {
			return domain.E("unsupported", "Kubernetes executor is not configured")
		}
		if e := m.Config.Kubernetes.Validate(p); e != nil {
			return e
		}
	} else {
		target, e := filepath.EvalSymlinks(p.Target)
		if e != nil {
			return domain.E("invalid", "target does not exist")
		}
		allowed := false
		for _, root := range m.Config.AllowedRoots {
			rel, e := filepath.Rel(root, target)
			if e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
				allowed = true
			}
		}
		if !allowed {
			return domain.E("invalid", "target is outside configured allowed roots")
		}
	}
	bytes, seconds := p.Budget()
	if bytes < 0 || bytes > m.Config.MaxBytes {
		return domain.E("invalid", "plan exceeds server data budget")
	}

	if time.Duration(seconds)*time.Second >= m.Config.Timeout {
		return domain.E("invalid", "measurement duration leaves no preparation time within server deadline")
	}
	return nil
}
func (m *Manager) Start(q StartRequest) (domain.Run, error) {
	if m.Config.ManagementOnly {
		return domain.Run{}, domain.E("unsupported", "run execution is disabled in management-only mode")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ctx.Err() != nil {
		return domain.Run{}, domain.E("conflict", "server is shutting down")
	}
	if q.Executor != "local" && q.Executor != "kubernetes" {
		return domain.Run{}, domain.E("unsupported", "executor must be local or kubernetes")
	}
	if q.Source != "human" && q.Source != "agent" && q.Source != "ci" {
		return domain.Run{}, domain.E("invalid", "source must be human, agent or ci")
	}
	if q.CaseRef != nil {
		if domain.Hash(q.Plan) != domain.Hash(domain.Plan{}) {
			return domain.Run{}, domain.E("invalid", "provide either plan or case_ref")
		}
		v, e := m.Store.CaseVersion(q.CaseRef.ID, q.CaseRef.Revision)
		if e != nil {
			return domain.Run{}, e
		}
		q.Plan = v.Plan
	}
	if e := q.Plan.Validate(); e != nil {
		return domain.Run{}, e
	}
	if e := domain.CheckID(q.RequestID); e != nil {
		return domain.Run{}, e
	}
	if old, e := m.Store.ByRequest(q.RequestID); e == nil {
		if domain.Hash(old.CaseRef) != domain.Hash(q.CaseRef) || old.Plan.Hash != q.Plan.Hash || old.Source != q.Source || old.Attempt.Executor != q.Executor {
			return domain.Run{}, domain.E("conflict", "request_id already refers to another plan or source")
		}
		return old, nil
	} else {
		var d *domain.Error
		if !errors.As(e, &d) || d.Code != "not_found" {
			return domain.Run{}, e
		}
	}
	if e := m.validate(q.Plan, q.Executor); e != nil {
		return domain.Run{}, e
	}
	return m.Store.CreateManagedFromCase(q.Plan, q.RequestID, q.Source, q.Executor, q.CaseRef)
}
func (m *Manager) Cancel(id string) (domain.Run, error) {
	if m.Config.ManagementOnly {
		return domain.Run{}, domain.E("unsupported", "run cancellation is disabled in management-only mode")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	r, e := m.Store.Run(id)
	if e != nil {
		return r, e
	}
	if domain.Terminal(r.Status) {
		return r, nil
	}
	if m.active == id || (r.Attempt.Executor == "kubernetes" && r.Attempt.JobName != "") {
		if r.Attempt.Executor != "kubernetes" {
			m.cancel()
		}
		r.Status = "cancelling"
	} else {
		r.Status = "cancelled"
		r.Attempt.Status = "cancelled"
		r.Attempt.FinishedAt = time.Now().UTC()
		r.Attempt.ResultState = "complete"
	}
	r.UpdatedAt = time.Now().UTC()
	if e = m.Store.Update(r); e != nil {
		return r, e
	}
	if r.Status == "cancelled" {
		e = m.Store.Finish(artifact.Bundle{SchemaVersion: r.SchemaVersion, Run: r, Files: []artifact.File{}})
	}
	return r, e
}
func (m *Manager) Close() { m.stop(); <-m.done; m.unlock() }
func (m *Manager) loop() {
	defer close(m.done)
	if m.Config.ManagementOnly {
		<-m.ctx.Done()
		return
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
		}
		m.mu.Lock()
		r, e := m.Store.NextPending()
		if e != nil {
			m.mu.Unlock()
			continue
		}
		if e = m.validate(r.Plan, r.Attempt.Executor); e != nil {
			r.Status = "failed"
			r.Error = e.Error()
			r.Attempt.Status = "failed"
			r.Attempt.ResultState = "failed"
			_ = m.Store.Update(r)
			m.mu.Unlock()
			continue
		}
		ctx, cancel := context.WithCancel(m.ctx)
		m.active = r.ID
		m.cancel = cancel
		m.mu.Unlock()
		out := filepath.Join(m.Store.Dir, "work", r.ID)
		var final domain.Run
		var runErr error
		if r.Attempt.Executor == "kubernetes" {
			final, runErr = m.Config.Kubernetes.Execute(ctx, r, out, kube.Options{Timeout: m.Config.Timeout, MaxBytes: m.Config.MaxBytes, Update: func(update domain.Run) error {
				m.mu.Lock()
				defer m.mu.Unlock()
				old, e := m.Store.Run(update.ID)
				if e != nil {
					return e
				}
				if old.Status == "cancelling" {
					update.Status = "cancelling"
				}
				return m.Store.Update(update)
			}, Cancelled: func() bool { old, e := m.Store.Run(r.ID); return e == nil && old.Status == "cancelling" }})
		} else {
			final, runErr = runner.Execute(ctx, r, out, runner.Options{Fio: m.Config.Fio, MaxBytes: m.Config.MaxBytes, Timeout: m.Config.Timeout, OnUpdate: func(update domain.Run) error {
				m.mu.Lock()
				defer m.mu.Unlock()
				if domain.Terminal(update.Status) {
					update.Attempt.ResultState = "collecting"
				} else if ctx.Err() != nil {
					update.Status = "cancelling"
				}
				return m.Store.Update(update)
			}})
		}
		if errors.Is(runErr, kube.ErrDetached) {
			cancel()
			return
		}

		cancel()
		m.mu.Lock()
		if !domain.Terminal(final.Status) {
			final.Status = "failed"
			final.Attempt.Status = "failed"
		}
		if runErr != nil {
			if r.Attempt.Executor == "kubernetes" {
				final.Attempt.ResultState = "failed"
			}
			final.Error = runErr.Error()
			if final.Status == "succeeded" {
				final.Status = "failed"
				final.Attempt.Status = "failed"
			}
		}
		final.UpdatedAt = time.Now().UTC()
		final.Attempt.FinishedAt = time.Now().UTC()
		if final.Attempt.ResultState != "failed" {
			final.Attempt.ResultState = "complete"
		}
		b, collectErr := artifact.Collect(out, final)
		if errors.Is(collectErr, os.ErrNotExist) {
			b = artifact.Bundle{SchemaVersion: final.SchemaVersion, Run: final, Files: []artifact.File{}}
			collectErr = nil
		}
		if collectErr == nil {
			collectErr = m.Store.Finish(b)
		}
		if collectErr == nil && r.Attempt.Executor == "kubernetes" && final.Attempt.ResultState == "complete" {
			if cleanupErr := m.Config.Kubernetes.Cleanup(context.Background(), final); cleanupErr != nil {
				fmt.Fprintln(os.Stderr, "Kubernetes cleanup deferred:", cleanupErr)
			}
		}
		if collectErr != nil {
			final.Status = "failed"
			final.Attempt.Status = "failed"
			final.Attempt.ResultState = "failed"
			final.Error = fmt.Sprintf("evidence persistence failed: %v; execution: %v", collectErr, runErr)
			_ = m.Store.Update(final)
		}
		m.active = ""
		m.cancel = nil
		m.mu.Unlock()
	}
}
