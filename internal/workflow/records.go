package workflow

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/store"
	"strings"
	"time"
)

func migrate(s *store.Store) error {
	_, e := s.DB.Exec(`CREATE TABLE IF NOT EXISTS workflow_records(kind TEXT NOT NULL,id TEXT NOT NULL,revision INTEGER NOT NULL,body TEXT NOT NULL,PRIMARY KEY(kind,id,revision));
 CREATE TABLE IF NOT EXISTS workflow_preflights(id TEXT PRIMARY KEY,body TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS workflow_issue_links(run_id TEXT NOT NULL,issue_id TEXT NOT NULL,role TEXT NOT NULL,body TEXT NOT NULL,PRIMARY KEY(run_id,issue_id,role));
 CREATE INDEX IF NOT EXISTS workflow_issue_by_issue ON workflow_issue_links(issue_id);
 CREATE TABLE IF NOT EXISTS workflow_runs(id TEXT PRIMARY KEY,request_id TEXT UNIQUE NOT NULL,body TEXT NOT NULL,updated_at TEXT NOT NULL);
 CREATE INDEX IF NOT EXISTS workflow_runs_by_case_time ON workflow_runs(json_extract(body,'$.snapshot.case.id'),julianday(json_extract(body,'$.created_at')) DESC,id DESC);`)
	return e
}
func validKind(k string) bool { return k == "cases" || k == "systems" || k == "profiles" }
func (m *Manager) Save(kind string, q SaveRecord) (Record, error) {
	var r Record
	if !validKind(kind) || strings.TrimSpace(q.Name) == "" || len(q.Name) > 200 || len(q.Description) > 10000 || len(q.ChangeNote) > 2000 || len(q.Body) > 4<<20 {
		return r, domain.E("invalid", "invalid workflow record")
	}
	switch kind {
	case "cases":
		var d Definition
		if e := domain.DecodeJSON(strings.NewReader(string(q.Body)), &d); e != nil {
			return r, domain.E("invalid", e.Error())
		}
		if e := validateDefinition(d); e != nil {
			return r, e
		}
	case "systems":
		var v System
		if e := domain.DecodeJSON(strings.NewReader(string(q.Body)), &v); e != nil {
			return r, domain.E("invalid", e.Error())
		}
		var env domain.Environment
		var version domain.SoftwareVersion
		if e := m.Store.Object("environment", v.EnvironmentID, &env); e != nil {
			return r, e
		}
		if e := m.Store.Object("version", v.SoftwareVersionID, &version); e != nil {
			return r, e
		}
	case "profiles":
		var p Profile
		if e := domain.DecodeJSON(strings.NewReader(string(q.Body)), &p); e != nil {
			return r, domain.E("invalid", e.Error())
		}
		if p.Backend != "argo" || p.Connection == "" || p.Cluster == "" || p.Namespace == "" || len(p.SystemIDs) == 0 {
			return r, domain.E("invalid", "workflow profile requires connection, cluster, namespace, node and allowed systems")
		}
	}
	if q.ID == "" {
		if q.BaseRevision != 0 {
			return r, domain.E("invalid", "new record requires base_revision 0")
		}
		q.ID = domain.ID("wf")
	} else if domain.CheckID(q.ID) != nil || q.BaseRevision < 1 {
		return r, domain.E("invalid", "edit requires ID and current revision")
	}
	tx, e := m.Store.DB.Begin()
	if e != nil {
		return r, e
	}
	defer tx.Rollback()
	var rev int
	if e = tx.QueryRow("SELECT COALESCE(MAX(revision),0) FROM workflow_records WHERE kind=? AND id=?", kind, q.ID).Scan(&rev); e != nil {
		return r, e
	}
	if rev != q.BaseRevision {
		return r, domain.E("conflict", "record changed; reload before saving")
	}
	r = Record{q.ID, rev + 1, kind, q.Name, q.Description, q.ChangeNote, time.Now().UTC(), q.Body}
	raw, _ := json.Marshal(r)
	if _, e = tx.Exec("INSERT INTO workflow_records VALUES(?,?,?,?)", kind, r.ID, r.Revision, string(raw)); e != nil {
		return r, e
	}
	return r, tx.Commit()
}
func (m *Manager) Record(kind string, ref Ref) (Record, error) {
	var r Record
	var raw string
	e := m.Store.DB.QueryRow("SELECT body FROM workflow_records WHERE kind=? AND id=? AND revision=?", kind, ref.ID, ref.Revision).Scan(&raw)
	if errors.Is(e, sql.ErrNoRows) {
		return r, domain.E("not_found", "workflow record not found")
	}
	if e != nil {
		return r, e
	}
	e = json.Unmarshal([]byte(raw), &r)
	return r, e
}

// RecordSummary carries live activity only in listing responses, never immutable snapshots.
type RecordSummary struct {
	Record
	LastRunAt string `json:"last_run_at,omitempty"`
}

func (m *Manager) Records(kind, id string) ([]RecordSummary, error) {
	if !validKind(kind) {
		return nil, domain.E("invalid", "unknown record kind")
	}
	query := "SELECT r.body,COALESCE((SELECT json_extract(w.body,'$.created_at') FROM workflow_runs w WHERE r.kind='cases' AND json_extract(w.body,'$.snapshot.case.id')=r.id ORDER BY julianday(json_extract(w.body,'$.created_at')) DESC,w.id DESC LIMIT 1),'') FROM workflow_records r WHERE kind=?"
	args := []any{kind}
	if id != "" {
		query += " AND id=?"
		args = append(args, id)
	} else {
		query += " AND revision=(SELECT MAX(revision) FROM workflow_records WHERE kind=r.kind AND id=r.id)"
	}
	query += " ORDER BY id,revision DESC LIMIT 500"
	rows, e := m.Store.DB.Query(query, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []RecordSummary{}
	for rows.Next() {
		var raw string
		var r RecordSummary
		if e = rows.Scan(&raw, &r.LastRunAt); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(raw), &r); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (m *Manager) Get(id string) (Run, error)         { return m.getBy("id", id) }
func (m *Manager) Lookup(request string) (Run, error) { return m.getBy("request_id", request) }
func (m *Manager) getBy(column, value string) (Run, error) {
	var r Run
	var raw string
	e := m.Store.DB.QueryRow("SELECT body FROM workflow_runs WHERE "+column+"=?", value).Scan(&raw)
	if errors.Is(e, sql.ErrNoRows) {
		return r, domain.E("not_found", "workflow run not found")
	}
	if e != nil {
		return r, e
	}
	e = json.Unmarshal([]byte(raw), &r)
	return r, e
}
func (m *Manager) Runs(limit, offset int) ([]Run, error) {
	return m.FilteredRuns(limit, offset, "", "")
}
func (m *Manager) FilteredRuns(limit, offset int, kind, connection string) ([]Run, error) {
	if kind != "" && kind != "argo" {
		return nil, domain.E("invalid", "unknown backend filter")
	}
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, domain.E("invalid", "limit 1..100, offset nonnegative")
	}
	rows, e := m.Store.DB.Query("SELECT body FROM workflow_runs WHERE (?='' OR json_extract(body,'$.snapshot.profile.body.backend')=?) AND (?='' OR json_extract(body,'$.snapshot.profile.body.connection')=?) ORDER BY json_extract(body,'$.created_at') DESC,id DESC LIMIT ? OFFSET ?", kind, kind, connection, connection, limit, offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		var raw string
		var r Run
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(raw), &r); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (m *Manager) persist(r Run) error {
	r.UpdatedAt = time.Now().UTC()
	raw, _ := json.Marshal(r)
	// Cancellation is stored inside the same serialized manager critical section.
	_, e := m.Store.DB.Exec("UPDATE workflow_runs SET body=json_set(?, '$.cancel_requested', json(CASE WHEN json_extract(body,'$.cancel_requested')=1 OR json_extract(?,'$.cancel_requested')=1 THEN 'true' ELSE 'false' END)),updated_at=? WHERE id=?", string(raw), string(raw), r.UpdatedAt.Format(time.RFC3339Nano), r.ID)
	return e
}
