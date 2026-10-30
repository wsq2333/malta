package store

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/wsq2333/malta/internal/domain"
)

// Catalog queries the entire matching history, independent of Runs' page limit.
// Aggregation is streamed in a read transaction; only representatives and the requested
// history page are retained. No evidence or immutable run bodies are rewritten.
type CatalogFilter struct {
	Operation                                                         string
	FileSize, BlockSize                                               int64
	Environments                                                      []string
	Version, Kind, Status, From, To, Search, Project, Condition, Mode string
	Saved                                                             bool
	Limit, Offset                                                     int
}
type CatalogRun struct {
	ID              string                 `json:"id"`
	Name            string                 `json:"name"`
	Status          string                 `json:"status"`
	ResultState     string                 `json:"result_state"`
	CreatedAt       time.Time              `json:"created_at"`
	Version         domain.SoftwareVersion `json:"version"`
	EnvironmentID   string                 `json:"environment_id"`
	EnvironmentName string                 `json:"environment_name"`
	Spec            domain.Spec            `json:"spec"`
	Metrics         []domain.Summary       `json:"metrics"`
}
type CatalogGroup struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Count         int         `json:"count"`
	Conditions    int         `json:"conditions"`
	Latest        CatalogRun  `json:"latest"`
	LatestSuccess *CatalogRun `json:"latest_success"`
}
type CatalogPage struct {
	Mode      string         `json:"mode"`
	Total     int            `json:"total"`
	RunCount  int            `json:"run_count"`
	Succeeded int            `json:"succeeded"`
	Active    int            `json:"active"`
	Saved     int            `json:"saved"`
	Limit     int            `json:"limit"`
	Offset    int            `json:"offset"`
	Groups    []CatalogGroup `json:"groups"`
	Runs      []CatalogRun   `json:"runs"`
}

func catalogKeys(r domain.Run) (string, string) {
	identity := "name:" + r.Plan.Spec.Name
	if r.CaseRef != nil {
		identity = "case:" + r.CaseRef.ID
	}
	project := domain.Hash([]string{r.Plan.Version.Filesystem, r.Plan.Spec.Kind, identity})
	spec := r.Plan.Spec
	spec.Name = ""
	if spec.Scenario != nil {
		cp := *spec.Scenario
		cp.Name = ""
		spec.Scenario = &cp
	}
	env := r.Plan.Environment
	env.ID = ""
	env.Name = ""
	condition := domain.Hash(struct {
		Spec                domain.Spec
		Env                 domain.Environment
		Tool, Malta, Target string
	}{spec, env, r.ToolVersion, r.MaltaVersion, r.Plan.Target})
	return project, condition
}
func catalogRef(r domain.Run) CatalogRun {
	ref := CatalogRun{ID: r.ID, Name: r.Plan.Spec.Name, Status: r.Status, ResultState: r.Attempt.ResultState, CreatedAt: r.CreatedAt, Version: r.Plan.Version, EnvironmentID: r.Plan.Environment.ID, EnvironmentName: r.Plan.Environment.Name, Spec: r.Plan.Spec, Metrics: []domain.Summary{}}
	if r.Status == "succeeded" && r.Attempt.ResultState == "complete" {
		ref.Metrics = domain.Summarize(r)
	}
	return ref
}
func (s *Store) Catalog(ctx context.Context, f CatalogFilter) (CatalogPage, error) {
	out := CatalogPage{Mode: f.Mode, Limit: f.Limit, Offset: f.Offset, Groups: []CatalogGroup{}, Runs: []CatalogRun{}}
	if f.Mode != "overview" && f.Mode != "conditions" && f.Mode != "history" {
		return out, domain.E("invalid", "mode must be overview, conditions or history")
	}
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 || len(f.Environments) > 1000 || len(f.Search) > 200 {
		return out, domain.E("invalid", "invalid catalog pagination or filter size")
	}
	if f.Mode == "conditions" && f.Project == "" || f.Condition != "" && f.Project == "" {
		return out, domain.E("invalid", "project required")
	}
	for _, d := range []string{f.From, f.To} {
		if d != "" {
			if _, e := time.Parse(time.RFC3339, d); e != nil {
				return out, domain.E("invalid", "dates must be RFC3339")
			}
		}
	}
	if f.From != "" && f.To != "" {
		a, _ := time.Parse(time.RFC3339, f.From)
		b, _ := time.Parse(time.RFC3339, f.To)
		if a.After(b) {
			return out, domain.E("invalid", "start date must precede end date")
		}
	}
	if f.Kind != "" && f.Kind != "data" && f.Kind != "metadata" && f.Kind != "scenario" {
		return out, domain.E("invalid", "invalid workload kind")
	}
	switch f.Status {
	case "", "pending", "queued", "preparing", "running", "cancelling", "succeeded", "failed", "cancelled", "interrupted":
	default:
		return out, domain.E("invalid", "invalid status")
	}
	if f.FileSize < 0 || f.BlockSize < 0 {
		return out, domain.E("invalid", "sizes must be non-negative bytes")
	}
	switch f.Operation {
	case "", "read", "write", "randread", "randwrite", "rw", "randrw":
	default:
		return out, domain.E("invalid", "invalid I/O operation")
	}
	q := "SELECT body FROM runs WHERE 1=1"
	args := []any{}
	for _, pair := range [][2]string{{"$.plan.version.id", f.Version}, {"$.plan.spec.kind", f.Kind}, {"$.status", f.Status}} {
		if pair[1] != "" {
			q += " AND json_extract(body,?)=?"
			args = append(args, pair[0], pair[1])
		}
	}
	if f.Operation != "" {
		q += " AND json_extract(body,'$.plan.spec.workload.operation')=?"
		args = append(args, f.Operation)
	}
	for _, field := range []struct {
		path  string
		value int64
	}{{"$.plan.spec.workload.file_size_bytes", f.FileSize}, {"$.plan.spec.workload.block_size_bytes", f.BlockSize}} {
		if field.value > 0 {
			q += " AND json_extract(body,?)=?"
			args = append(args, field.path, field.value)
		}
	}
	if len(f.Environments) > 0 {
		qs := []string{}
		for _, id := range f.Environments {
			if e := domain.CheckID(id); e != nil {
				return out, e
			}
			qs = append(qs, "?")
			args = append(args, id)
		}
		q += " AND json_extract(body,'$.plan.environment.id') IN (" + strings.Join(qs, ",") + ")"
	}
	if f.From != "" {
		q += " AND julianday(created_at)>=julianday(?)"
		args = append(args, f.From)
	}
	if f.To != "" {
		q += " AND julianday(created_at)<=julianday(?)"
		args = append(args, f.To)
	}
	q += " ORDER BY julianday(created_at) DESC,id DESC"
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	// Baselines and runs are read from one snapshot, so counts agree with the page.
	br, e := tx.QueryContext(ctx, "SELECT body FROM objects WHERE kind='baseline'")
	if e != nil {
		return out, e
	}
	saved := map[string]bool{}
	for br.Next() {
		var b string
		if e = br.Scan(&b); e != nil {
			br.Close()
			return out, e
		}
		var v domain.Baseline
		if e = json.Unmarshal([]byte(b), &v); e != nil {
			br.Close()
			return out, e
		}
		saved[v.RunID] = true
	}
	e = br.Err()
	br.Close()
	if e != nil {
		return out, e
	}
	rows, e := tx.QueryContext(ctx, q, args...)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	groups := map[string]*CatalogGroup{}
	conditions := map[string]map[string]bool{}
	for rows.Next() {
		if e = ctx.Err(); e != nil {
			return out, e
		}
		var b string
		var r domain.Run
		if e = rows.Scan(&b); e != nil {
			return out, e
		}
		if e = json.Unmarshal([]byte(b), &r); e != nil {
			return out, e
		}
		if f.Saved && !saved[r.ID] {
			continue
		}
		if f.Search != "" && !strings.Contains(strings.ToLower(strings.Join([]string{r.ID, r.Plan.Spec.Name, r.Plan.Version.Label, r.Plan.Environment.Name}, " ")), strings.ToLower(f.Search)) {
			continue
		}
		project, condition := catalogKeys(r)
		if f.Project != "" && project != f.Project || f.Condition != "" && condition != f.Condition {
			continue
		}
		out.RunCount++
		if saved[r.ID] {
			out.Saved++
		}
		if !domain.Terminal(r.Status) {
			out.Active++
		}
		success := r.Status == "succeeded" && r.Attempt.ResultState == "complete"
		if success {
			out.Succeeded++
		}
		if f.Mode == "history" {
			if out.Total >= f.Offset && len(out.Runs) < f.Limit {
				out.Runs = append(out.Runs, catalogRef(r))
			}
			out.Total++
			continue
		}
		key := project
		if f.Mode == "conditions" {
			key = condition
		}
		g := groups[key]
		if g == nil {
			g = &CatalogGroup{ID: key, Name: r.Plan.Spec.Name, Latest: catalogRef(r)}
			groups[key] = g
			conditions[key] = map[string]bool{}
		}
		g.Count++
		conditions[key][condition] = true
		if success && g.LatestSuccess == nil {
			ref := catalogRef(r)
			g.LatestSuccess = &ref
		}
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	rows.Close()
	if f.Mode != "history" {
		list := []CatalogGroup{}
		for id, g := range groups {
			g.Conditions = len(conditions[id])
			if g.Conditions > 1 {
				g.LatestSuccess = nil
			}
			list = append(list, *g)
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].Latest.CreatedAt.Equal(list[j].Latest.CreatedAt) {
				return list[i].ID < list[j].ID
			}
			return list[i].Latest.CreatedAt.After(list[j].Latest.CreatedAt)
		})
		out.Total = len(list)
		if f.Offset < len(list) {
			end := f.Offset + f.Limit
			if end > len(list) {
				end = len(list)
			}
			out.Groups = list[f.Offset:end]
		}
	}
	return out, tx.Commit()
}
