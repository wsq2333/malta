package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/wsq2333/malta/internal/artifact"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/runner"
	"github.com/wsq2333/malta/internal/store"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Config struct {
	AllowedRoots []string
	Fio          string
	MaxBytes     int64
	Timeout      time.Duration
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
	Plan      domain.Plan `json:"plan"`
	RequestID string      `json:"request_id"`
	Source    string      `json:"source"`
	Executor  string      `json:"executor"`
}

func New(s *store.Store, c Config) (*Manager, error) {
	if len(c.AllowedRoots) == 0 {
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
	if e = s.Recover(); e != nil {
		unlock()
		return nil, e
	}
	ctx, stop := context.WithCancel(context.Background())
	m := &Manager{Store: s, Config: c, ctx: ctx, stop: stop, done: make(chan struct{}), unlock: unlock}
	go m.loop()
	return m, nil
}
func (m *Manager) Validate(p domain.Plan) error {
	if e := p.Validate(); e != nil {
		return e
	}
	if p.Spec.Kind != "data" || p.Spec.Workload.CacheState == "cold-verified" {
		return domain.E("unsupported", "local runner supports data plans without cold-verified claims")
	}
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
	var bytes int64
	for _, pnt := range p.Trials {
		bytes += p.Spec.Workload.FileSizeBytes * int64(p.Spec.Workload.FilesPerJob) * int64(pnt.NumJobs)
		if bytes < 0 || bytes > m.Config.MaxBytes {
			return domain.E("invalid", "plan exceeds server data budget")
		}
	}
	seconds := int64(len(p.Trials)) * int64(p.Spec.Workload.RuntimeSeconds+p.Spec.Workload.WarmupSeconds)
	if time.Duration(seconds)*time.Second >= m.Config.Timeout {
		return domain.E("invalid", "measurement duration leaves no preparation time within server deadline")
	}
	return nil
}
func (m *Manager) Start(q StartRequest) (domain.Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ctx.Err() != nil {
		return domain.Run{}, domain.E("conflict", "server is shutting down")
	}
	if q.Executor != "local" {
		return domain.Run{}, domain.E("unsupported", "only local executor is implemented; Kubernetes is S5")
	}
	if q.Source != "human" && q.Source != "agent" && q.Source != "ci" {
		return domain.Run{}, domain.E("invalid", "source must be human, agent or ci")
	}
	if e := m.Validate(q.Plan); e != nil {
		return domain.Run{}, e
	}
	return m.Store.CreateManaged(q.Plan, q.RequestID, q.Source)
}
func (m *Manager) Cancel(id string) (domain.Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, e := m.Store.Run(id)
	if e != nil {
		return r, e
	}
	if domain.Terminal(r.Status) {
		return r, nil
	}
	if m.active == id {
		m.cancel()
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
		e = m.Store.Finish(artifact.Bundle{SchemaVersion: domain.Schema, Run: r, Files: []artifact.File{}})
	}
	return r, e
}
func (m *Manager) Close() { m.stop(); <-m.done; m.unlock() }
func (m *Manager) loop() {
	defer close(m.done)
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
		if e = m.Validate(r.Plan); e != nil {
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
		final, runErr := runner.Execute(ctx, r, out, runner.Options{Fio: m.Config.Fio, MaxBytes: m.Config.MaxBytes, Timeout: m.Config.Timeout, OnUpdate: func(update domain.Run) error {
			m.mu.Lock()
			defer m.mu.Unlock()
			if domain.Terminal(update.Status) {
				update.Attempt.ResultState = "collecting"
			} else if ctx.Err() != nil {
				update.Status = "cancelling"
			}
			return m.Store.Update(update)
		}})
		cancel()
		m.mu.Lock()
		if !domain.Terminal(final.Status) {
			final.Status = "failed"
			final.Attempt.Status = "failed"
		}
		if runErr != nil {
			final.Error = runErr.Error()
			if final.Status == "succeeded" {
				final.Status = "failed"
				final.Attempt.Status = "failed"
			}
		}
		final.UpdatedAt = time.Now().UTC()
		final.Attempt.FinishedAt = time.Now().UTC()
		final.Attempt.ResultState = "complete"
		b, collectErr := artifact.Collect(out, final)
		if errors.Is(collectErr, os.ErrNotExist) {
			b = artifact.Bundle{SchemaVersion: domain.Schema, Run: final, Files: []artifact.File{}}
			collectErr = nil
		}
		if collectErr == nil {
			collectErr = m.Store.Finish(b)
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
