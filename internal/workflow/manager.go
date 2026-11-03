package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/wsq2333/malta/internal/backend"
	"github.com/wsq2333/malta/internal/backend/argo"
	"github.com/wsq2333/malta/internal/backend/auth"
	"github.com/wsq2333/malta/internal/backend/httpapi"
	"github.com/wsq2333/malta/internal/backend/uce"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/store"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Connections []backend.Config `json:"connections"`
}
type Manager struct {
	Store       *store.Store
	connections map[string]backend.Config
	backends    map[string]backend.Backend
	credentials map[string]*auth.Provider
	enabled     bool
	ctx         context.Context
	stop        context.CancelFunc
	done        chan struct{}
	mu          sync.Mutex
}

func connectionHash(c backend.Config) string {
	c.TokenFile = ""
	if c.Auth != nil {
		a := *c.Auth
		a.TokenFile = ""
		a.UsernameFile = ""
		a.PasswordFile = ""
		a.ClientSecretFile = ""
		c.Auth = &a
	}
	return domain.Hash(c)
}
func New(s *store.Store, c Config, enabled bool) (*Manager, error) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{Store: s, connections: map[string]backend.Config{}, backends: map[string]backend.Backend{}, credentials: map[string]*auth.Provider{}, enabled: enabled, ctx: ctx, stop: cancel, done: make(chan struct{})}
	if e := migrate(s); e != nil {
		cancel()
		return nil, e
	}
	for _, v := range c.Connections {
		if domain.CheckID(v.ID) != nil || m.connections[v.ID].ID != "" || (v.Kind() != "uce" && v.Kind() != "argo") {
			cancel()
			return nil, domain.E("invalid", "invalid/duplicate connection ID or unsupported backend")
		}
		// Disabled entries may reserve an endpoint before routes and credentials exist.
		h, e := httpapi.New(v)
		if e != nil {
			cancel()
			return nil, e
		}
		m.connections[v.ID] = v
		m.credentials[v.ID] = h.Auth
		if !v.Active() {
			continue
		}
		if v.Cluster == "" || v.Namespace == "" || (v.Kind() == "uce" && len(v.Subtypes) == 0) || v.MaxBytes < 1 || v.TimeoutSeconds < 1 || v.TimeoutSeconds > 86400 || v.ArtifactBytes < 1 || v.ArtifactBytes > 32<<20 {
			cancel()
			return nil, domain.E("invalid", "invalid workflow route or budget")
		}
		for _, root := range []string{v.RemoteResultsRoot, v.LocalResultsRoot} {
			if root == "" {
				continue
			}
			if !filepath.IsAbs(root) || filepath.Clean(root) != root || root == "/" {
				cancel()
				return nil, domain.E("invalid", "result roots must be clean absolute non-root paths")
			}
		}
		if v.Kind() == "uce" {
			client, e := uce.New(v)
			if e != nil {
				cancel()
				return nil, e
			}
			m.backends[v.ID] = client
			m.credentials[v.ID] = client.Auth
		} else {
			client, e := argo.New(v)
			if e != nil {
				cancel()
				return nil, e
			}
			m.backends[v.ID] = client
			m.credentials[v.ID] = client.Auth
		}
	}
	go m.loop()
	return m, nil
}
func (m *Manager) Close()        { m.stop(); <-m.done }
func (m *Manager) Enabled() bool { return m.enabled && len(m.backends) > 0 }
func (m *Manager) BackendEnabled(kind string) bool {
	if m == nil || !m.enabled {
		return false
	}
	for id, c := range m.connections {
		if c.Kind() == kind && m.backends[id] != nil {
			return true
		}
	}
	return false
}
func (m *Manager) credential(c backend.Config) (*auth.Provider, error) {
	if p := m.credentials[c.ID]; p != nil {
		return p, nil
	}
	a, e := c.AuthConfig()
	if e != nil {
		return nil, e
	}
	return auth.New(a)
}
func (m *Manager) Connections() []map[string]any {
	out := []map[string]any{}
	for _, c := range m.connections {
		state := "credentials_missing"
		p, e := m.credential(c)
		if e == nil {
			state = p.Status()
		}
		if !c.Active() {
			state = "disabled"
		}
		a, _ := c.AuthConfig()
		kind := a.Type
		if kind == "" {
			kind = "bearer_token_file"
		}
		out = append(out, map[string]any{"id": c.ID, "backend": c.Kind(), "url": c.URL, "cluster": c.Cluster, "namespace": c.Namespace, "subtypes": c.Subtypes, "max_bytes": c.MaxBytes, "timeout_seconds": c.TimeoutSeconds, "enabled": c.Active() && m.enabled, "available": c.Active() && m.enabled && state == "configured", "auth_type": kind, "auth_status": state})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["id"].(string) < out[j]["id"].(string) })
	return out
}
func (m *Manager) Preflight(q Selection) (Preflight, error) {
	s, e := m.compile(q)
	if e != nil {
		return Preflight{}, e
	}
	p := Preflight{ID: domain.ID("check"), ExpiresAt: time.Now().UTC().Add(10 * time.Minute), Ready: m.Enabled(), Snapshot: s, Checks: []Check{
		{"模板与参数", "passed", "模板封装、参数与可选结果规则已校验；原生 DAG 语义由后端校验"},
		{"环境绑定", "passed", "已核对已登记系统、软件版本及执行配置；不是实时集群探测"},
		{"预算", "passed", fmt.Sprintf("准备数据 %d 字节，执行期限 %d 秒；终态后证据回收最多另等 120 秒", s.PreparedBytes, s.Definition.TimeoutSeconds)},
		{"现场事实", "warning", "提交前须由管理员核验 执行路由、PVC、软件实际版本及凭据权限；本步骤不创建探测 Pod"},
	}}
	var profile Profile
	_ = json.Unmarshal(s.Profile.Body, &profile)
	cfg := m.connections[profile.Connection]
	provider, authErr := m.credential(cfg)
	if authErr == nil {
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		_, authErr = provider.Token(ctx)
		cancel()
	}
	if authErr != nil {
		p.Ready = false
		p.Checks = append(p.Checks, Check{"后端认证", "failed", authErr.Error()})
	} else {
		p.Checks = append(p.Checks, Check{"后端认证", "passed", "可取得访问 Token；业务权限由后端接口校验，令牌不写入快照"})
	}
	if len(s.Definition.Results) > 0 {
		if st, statErr := os.Stat(cfg.LocalResultsRoot); statErr != nil || !st.IsDir() {
			p.Ready = false
			p.Checks = append(p.Checks, Check{"结果卷", "failed", "服务端结果根目录不可访问"})
		} else {
			p.Checks = append(p.Checks, Check{"结果卷", "warning", "本地目录可访问；跨 Pod 同卷共享及原子写入需在执行配置验收时确认"})
		}
	} else {
		p.Checks = append(p.Checks, Check{"结果解析", "warning", "未配置解析器：仅保存后端执行状态与原始详情，不生成测试通过结论"})
	}
	if !m.Enabled() {
		p.Checks = append(p.Checks, Check{"执行后端", "failed", "工作流执行未启用"})
	}
	raw, _ := json.Marshal(p)
	_, e = m.Store.DB.Exec("INSERT INTO workflow_preflights VALUES(?,?)", p.ID, string(raw))
	return p, e
}
func (m *Manager) Start(q Start) (Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if domain.CheckID(q.RequestID) != nil {
		return Run{}, domain.E("invalid", "request_id required")
	}
	if old, e := m.Lookup(q.RequestID); e == nil {
		if old.Snapshot.Hash != q.SnapshotHash {
			return old, domain.E("conflict", "request_id already used for another snapshot")
		}
		return old, nil
	} else {
		var de *domain.Error
		if !errors.As(e, &de) || de.Code != "not_found" {
			return Run{}, e
		}
	}
	if !m.Enabled() {
		return Run{}, domain.E("unsupported", "workflow execution disabled")
	}
	var raw string
	if e := m.Store.DB.QueryRow("SELECT body FROM workflow_preflights WHERE id=?", q.PreflightID).Scan(&raw); e != nil {
		return Run{}, domain.E("not_found", "preflight not found")
	}
	var p Preflight
	if e := json.Unmarshal([]byte(raw), &p); e != nil {
		return Run{}, e
	}
	if !p.Ready || time.Now().After(p.ExpiresAt) || p.Snapshot.Hash != q.SnapshotHash {
		return Run{}, domain.E("conflict", "preflight expired, failed or changed")
	}
	current, e := m.compile(p.Snapshot.Selection)
	if e != nil {
		return Run{}, e
	}
	if current.Hash != p.Snapshot.Hash {
		return Run{}, domain.E("conflict", "configuration changed; repeat preflight")
	}
	now := time.Now().UTC()
	r := Run{SchemaVersion: Schema, ID: domain.ID("workflow"), RequestID: q.RequestID, Key: uuid.NewString(), Snapshot: p.Snapshot, Status: "pending", ExecutionState: "not_submitted", ResultState: "pending", Verdict: "inconclusive", CreatedAt: now, UpdatedAt: now, Deadline: now.Add(time.Duration(p.Snapshot.Definition.TimeoutSeconds) * time.Second), Steps: []Step{{"准备执行定义", "pending", ""}, {"提交运行", "pending", ""}, {"执行 DAG", "pending", ""}, {"回收并验证结果", "pending", ""}}, Results: []Result{}}
	// A cluster-wide test lease prevents different templates from interfering.
	// Persisted active runs are the lease; API outages do not release it.
	var active int
	e = m.Store.DB.QueryRow(`SELECT count(*) FROM workflow_runs WHERE json_extract(body,'$.snapshot.profile.body.cluster')=? AND json_extract(body,'$.status') NOT IN ('succeeded','failed','cancelled')`, profileCluster(r)).Scan(&active)
	if e != nil {
		return Run{}, e
	}
	if active > 0 {
		return Run{}, domain.E("conflict", "another workflow holds this execution cluster; wait for terminal state")
	}
	b, _ := json.Marshal(r)
	_, e = m.Store.DB.Exec("INSERT INTO workflow_runs VALUES(?,?,?,?)", r.ID, r.RequestID, string(b), now.Format(time.RFC3339Nano))
	return r, e
}
func profileCluster(r Run) string {
	var p Profile
	_ = json.Unmarshal(r.Snapshot.Profile.Body, &p)
	return p.Cluster
}
func (m *Manager) Cancel(id string) (Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, e := m.Get(id)
	if e != nil {
		return r, e
	}
	if terminal(r.Status) {
		return r, nil
	}
	r.CancelRequested = true
	r.Status = "cancelling"
	e = m.persist(r)
	return r, e
}
func terminal(s string) bool { return s == "succeeded" || s == "failed" || s == "cancelled" }
func (m *Manager) RetryCollection(id string) (Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, e := m.Get(id)
	if e != nil {
		return r, e
	}
	if r.ExternalRunID == "" || r.ResultState != "missing" {
		return r, domain.E("invalid", "only missing evidence can be recollected")
	}
	deadline := time.Now().UTC().Add(2 * time.Minute)
	r.CollectionDeadline = &deadline
	r.ResultState = "collecting"
	r.Status = "collecting"
	r.Error = ""
	e = m.persist(r)
	return r, e
}
func (m *Manager) loop() {
	defer close(m.done)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			if m.enabled {
				m.round()
			}
		}
	}
}
func (m *Manager) round() {
	rows, e := m.Store.DB.Query(`SELECT id FROM workflow_runs WHERE json_extract(body,'$.status') NOT IN ('succeeded','failed','cancelled') ORDER BY updated_at,id LIMIT 20`)
	if e != nil {
		return
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		if m.ctx.Err() != nil {
			return
		}
		r, e := m.Get(id)
		if e != nil {
			continue
		}
		m.advance(&r)
		m.mu.Lock()
		latest, e := m.Get(id)
		if e == nil {
			r.CancelRequested = r.CancelRequested || latest.CancelRequested
			if r.CancelRequested && !terminal(r.Status) {
				r.Status = "cancelling"
			}
			_ = m.persist(r)
		}
		m.mu.Unlock()
	}
}
func (m *Manager) advance(r *Run) {
	var p Profile
	_ = json.Unmarshal(r.Snapshot.Profile.Body, &p)
	b := m.backends[p.Connection]
	cfg, ok := m.connections[p.Connection]
	if !ok || b == nil || connectionHash(cfg) != r.Snapshot.ConnectionHash {
		r.Error = "workflow connection missing or changed; restore the snapshotted connection to resume"
		return
	}
	expired := time.Now().After(r.Deadline) && r.CollectionDeadline == nil
	if expired {
		r.CancelRequested = true
	}
	if r.ExternalRunID == "" && r.SubmissionStarted {
		id, e := b.Lookup(m.ctx, r.Key)
		if e == nil {
			r.ExternalRunID = id
			r.Steps[1].State = "succeeded"
		} else {
			var he *backend.HTTPError
			if !errors.As(e, &he) || he.Status != 404 {
				r.Error = e.Error()
				return
			}
			if r.CancelRequested {
				r.Error = "submission outcome not yet observed; cancellation pending, no resubmission"
				return
			}
		}
	}
	if r.CancelRequested && r.ExternalRunID == "" && !r.SubmissionStarted {
		r.Status = "cancelled"
		r.CancelConfirmed = true
		r.ResultState = "complete"
		r.Steps[1].State = "skipped"
		return
	}
	if r.ExternalVersionID == "" {
		r.Steps[0].State = "running"
		r.Status = "preparing"
		id, e := b.Register(m.ctx, r.Snapshot.EP)
		if e != nil {
			r.Error = e.Error()
			var he *backend.HTTPError
			if errors.As(e, &he) && he.Status >= 400 && he.Status < 500 && he.Status != 429 {
				r.Status = "failed"
				r.Steps[0].State = "failed"
				r.ResultState = "missing"
			}
			return
		}
		r.ExternalVersionID = id
		r.Steps[0] = Step{"准备执行定义", "succeeded", id}
		_ = m.persist(*r)
		return
	}
	if r.ExternalRunID == "" {
		// Save submission intent BEFORE the external side effect. Retries use the same body/key.
		firstSubmission := !r.SubmissionStarted
		r.SubmissionStarted = true
		r.Status = "submitting"
		r.Steps[1].State = "running"
		if e := m.persist(*r); e != nil {
			r.Error = e.Error()
			return
		}
		if current, e := m.Get(r.ID); e != nil || current.CancelRequested {
			r.CancelRequested = true
			if e == nil && firstSubmission {
				r.Status = "cancelled"
				r.CancelConfirmed = true
				r.SubmissionStarted = false
				r.Steps[1].State = "skipped"
			}
			return
		}
		id, e := b.Submit(m.ctx, backend.Submission{VersionID: r.ExternalVersionID, Key: r.Key, Name: r.ID, Inputs: submissionInputs(*r), Queue: p.Queue, Subtype: p.Subtype, TimeoutSeconds: r.Snapshot.Definition.TimeoutSeconds})
		if e != nil {
			r.Error = e.Error()
			var he *backend.HTTPError
			if errors.As(e, &he) && (he.Status == 400 || he.Status == 401 || he.Status == 403 || he.Status == 409 || he.Status == 422) {
				r.Status = "failed"
				r.Steps[1].State = "failed"
				r.ResultState = "missing"
			}
			return
		}
		r.ExternalRunID = id
		r.Steps[1] = Step{"提交运行", "succeeded", id}
		return
	}
	o, e := b.Observe(m.ctx, r.ExternalRunID)
	if e != nil {
		r.Error = e.Error()
		return
	}
	if o.RunID != r.ExternalRunID || o.VersionID != r.ExternalVersionID {
		r.Error = "workflow execution identity mismatch"
		return
	}
	r.Observations, _ = json.Marshal(o)
	status := strings.ToLower(o.Status)
	r.ExecutionState = status
	r.Steps[2] = Step{"执行 DAG", status, r.ExternalRunID}
	r.Error = ""
	if r.CancelRequested && !terminal(status) && status != "timeout" {
		r.Status = "cancelling"
		if e = b.Cancel(m.ctx, r.ExternalRunID); e != nil {
			r.Error = e.Error()
		}
		return
	}
	if !terminal(status) && status != "timeout" {
		r.Status = "running"
		return
	}
	if o.CancellationPending || status == "cancelled" || status == "timeout" {
		done, e := b.CancellationComplete(m.ctx, r.ExternalRunID)
		if e != nil || !done || o.CancellationPending {
			r.Status = "cancelling"
			if e != nil {
				r.Error = e.Error()
			}
			return
		}
		r.CancelConfirmed = true
	}
	if len(r.Snapshot.Definition.Results) == 0 {
		r.ResultState = "not_configured"
		r.Verdict = "not_evaluated"
		r.Status = status
		if status == "timeout" || r.CancelRequested {
			r.Status = "cancelled"
		}
		r.Steps[3] = Step{"回收并验证结果", "skipped", "未配置结果解析；执行成功不代表测试通过"}
		return
	}
	if r.CollectionDeadline == nil {
		d := time.Now().UTC().Add(2 * time.Minute)
		r.CollectionDeadline = &d
	}
	r.Status = "collecting"
	r.ResultState = "collecting"
	r.Steps[3].State = "running"
	results, complete, invalid, passed := m.collect(r, o, b, cfg)
	r.Results = results
	if !complete && !invalid && time.Now().Before(*r.CollectionDeadline) {
		return
	}
	r.ResultState = "complete"
	r.Verdict = "passed"
	r.Status = "succeeded"
	if invalid {
		r.ResultState = "invalid"
	}
	if !complete && !invalid {
		r.ResultState = "missing"
	}
	if !complete || invalid {
		r.Verdict = "inconclusive"
		r.Status = "failed"
	} else if !passed || status != "succeeded" {
		r.Verdict = "failed"
		r.Status = "failed"
	}
	if status == "cancelled" || status == "timeout" || r.CancelRequested {
		r.Status = "cancelled"
		r.Verdict = "inconclusive"
	}
	r.Steps[3] = Step{"回收并验证结果", r.ResultState, r.Verdict}
}

func submissionInputs(r Run) map[string]any {
	v := map[string]any{}
	for k, x := range r.Snapshot.Inputs {
		v[k] = x
	}
	var template map[string]any
	_ = json.Unmarshal(r.Snapshot.EP, &template)
	if declaresContext(template, r.Snapshot.Definition.Backend) {
		v["malta_context"] = map[string]any{"malta_run_id": r.ID, "workflow_version_id": r.ExternalVersionID, "snapshot_hash": r.Snapshot.Hash, "environment_id": r.Snapshot.Environment.ID, "software_version_id": r.Snapshot.Version.ID}
	}
	return v
}
