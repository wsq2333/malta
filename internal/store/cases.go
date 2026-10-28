package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/wsq2333/malta/internal/domain"
	"strings"
	"time"
)

func (s *Store) migrateCases() error {
	var version int
	if e := s.DB.QueryRow("PRAGMA user_version").Scan(&version); e != nil {
		return e
	}
	if version > 3 {
		return domain.E("unsupported", "database schema is newer than this Malta binary")
	}
	if _, e := s.DB.Exec("PRAGMA foreign_keys=ON"); e != nil {
		return e
	}
	tx, e := s.DB.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	_, e = tx.Exec(`CREATE TABLE IF NOT EXISTS test_cases(id TEXT PRIMARY KEY, created_at TEXT NOT NULL, latest_revision INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS case_versions(case_id TEXT NOT NULL REFERENCES test_cases(id), revision INTEGER NOT NULL CHECK(revision>0), body TEXT NOT NULL, PRIMARY KEY(case_id,revision));
 CREATE TABLE IF NOT EXISTS issues(
 issue_id TEXT PRIMARY KEY,
 revision INTEGER NOT NULL CHECK(revision>0),
 body TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS issue_history(
 issue_id TEXT NOT NULL REFERENCES issues(issue_id),
 revision INTEGER NOT NULL CHECK(revision>0),
 body TEXT NOT NULL,
 PRIMARY KEY(issue_id,revision));
 PRAGMA user_version=3;`)
	if e != nil {
		return e
	}
	return tx.Commit()
}

// SaveCase appends a revision under an optimistic lock. Derived trials/hash are rebuilt.
func (s *Store) SaveCase(id string, q domain.SaveCase) (domain.CaseVersion, error) {
	var out domain.CaseVersion
	q.Name = strings.TrimSpace(q.Name)
	if q.Name == "" || len(q.Name) > 200 || len(q.Description) > 10000 || len(q.ChangeNote) > 2000 {
		return out, domain.E("invalid", "name is required (max 200 bytes); description max 10000; change_note max 2000")
	}
	if q.BaseRevision < 0 || (id == "" && q.BaseRevision != 0) || (id != "" && q.BaseRevision < 1) {
		return out, domain.E("invalid", "create with base_revision 0; edits require the latest base_revision")
	}
	p := q.Plan
	if p.SchemaVersion != p.Spec.SchemaVersion {
		return out, domain.E("invalid", "plan and spec schema differ")
	}
	// Scenario wrapper name/repeats are derived from the editable scenario definition.
	if p.Spec.Kind == "scenario" && p.Spec.Scenario != nil {
		p.Spec.Name = p.Spec.Scenario.Name
		p.Spec.Repeats = p.Spec.Scenario.Repeats
	}
	// Validate bounded input before expanding trials.
	if e := p.Spec.Validate(); e != nil {
		return out, e
	}
	var e error
	if p.Spec.Kind == "scenario" {
		p, e = domain.NewScenarioPlan(*p.Spec.Scenario, p.Environment, p.Version, p.Target, p.ResourceGroup)
	} else {
		p, e = domain.NewPlan(p.Spec, p.Environment, p.Version, p.Target, p.ResourceGroup)
	}
	if e != nil {
		return out, e
	}
	if e = p.Validate(); e != nil {
		return out, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, e := s.DB.Begin()
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	if id == "" {
		id = domain.ID("case")
		_, e = tx.Exec("INSERT INTO test_cases(id,created_at,latest_revision) VALUES(?,?,0)", id, now.Format(time.RFC3339Nano))
		if e != nil {
			return out, e
		}
	} else {
		if e = domain.CheckID(id); e != nil {
			return out, e
		}
		var latest int
		e = tx.QueryRow("SELECT latest_revision FROM test_cases WHERE id=?", id).Scan(&latest)
		if errors.Is(e, sql.ErrNoRows) {
			return out, domain.E("not_found", "case not found")
		}
		if e != nil {
			return out, e
		}
		if latest != q.BaseRevision {
			return out, domain.E("conflict", "case has a newer revision; reload before saving")
		}
	}
	out = domain.CaseVersion{CaseID: id, Revision: q.BaseRevision + 1, Name: q.Name, Description: q.Description, ChangeNote: q.ChangeNote, CreatedAt: now, Plan: p}
	if _, e = tx.Exec("INSERT INTO case_versions(case_id,revision,body) VALUES(?,?,?)", id, out.Revision, encode(out)); e != nil {
		return out, e
	}
	if _, e = tx.Exec("UPDATE test_cases SET latest_revision=? WHERE id=?", out.Revision, id); e != nil {
		return out, e
	}
	return out, tx.Commit()
}
func (s *Store) CaseVersion(id string, revision int) (domain.CaseVersion, error) {
	var out domain.CaseVersion
	var body string
	if revision < 1 {
		return out, domain.E("invalid", "revision must be positive")
	}
	e := s.DB.QueryRow("SELECT body FROM case_versions WHERE case_id=? AND revision=?", id, revision).Scan(&body)
	if errors.Is(e, sql.ErrNoRows) {
		return out, domain.E("not_found", "case revision not found")
	}
	if e != nil {
		return out, e
	}
	e = json.Unmarshal([]byte(body), &out)
	return out, e
}
func (s *Store) Cases() ([]domain.Case, error) {
	rows, e := s.DB.Query("SELECT c.id,c.created_at,v.body FROM test_cases c JOIN case_versions v ON v.case_id=c.id AND v.revision=c.latest_revision ORDER BY json_extract(v.body,'$.created_at') DESC,c.id")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.Case{}
	for rows.Next() {
		var c domain.Case
		var created, body string
		if e = rows.Scan(&c.ID, &created, &body); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(body), &c.Latest); e != nil {
			return nil, e
		}
		if c.CreatedAt, e = time.Parse(time.RFC3339Nano, created); e != nil {
			return nil, e
		}
		c.UpdatedAt = c.Latest.CreatedAt
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Store) CaseVersions(id string) ([]domain.CaseVersion, error) {
	rows, e := s.DB.Query("SELECT body FROM case_versions WHERE case_id=? ORDER BY revision DESC", id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.CaseVersion{}
	for rows.Next() {
		var v domain.CaseVersion
		var body string
		if e = rows.Scan(&body); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(body), &v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if len(out) == 0 {
		return nil, domain.E("not_found", "case not found")
	}
	return out, nil
}
