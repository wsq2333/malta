package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/wsq2333/malta/internal/domain"
	"time"
)

func (s *Store) SaveIssue(q domain.SaveIssue) (domain.Issue, error) {
	var v domain.Issue
	if e := q.Validate(); e != nil {
		return v, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, e := s.DB.Begin()
	if e != nil {
		return v, e
	}
	defer tx.Rollback()
	if q.IssueID == "" {
		q.IssueID = domain.ID("issue")
	}
	var body string
	e = tx.QueryRow("SELECT body FROM issues WHERE issue_id=?", q.IssueID).Scan(&body)
	var old domain.Issue
	switch {
	case errors.Is(e, sql.ErrNoRows):
		if q.BaseRevision != 0 {
			return v, domain.E("not_found", "issue not found")
		}
	case e != nil:
		return v, e
	default:
		if e = json.Unmarshal([]byte(body), &old); e != nil {
			return v, e
		}
		// A stable caller-supplied ID makes an identical initial submission retry safe.
		if q.BaseRevision == 0 && old.Revision == 1 && domain.Hash(old.IssueFields) == domain.Hash(q.IssueFields) {
			return old, nil
		}
		if q.BaseRevision != old.Revision {
			return v, domain.E("conflict", "issue changed; reload latest revision before saving")
		}
	}
	for _, ref := range []struct{ kind, id string }{{"version", q.VersionID}, {"environment", q.EnvironmentID}} {
		if ref.id == "" {
			continue
		}
		var one int
		if e = tx.QueryRow("SELECT 1 FROM objects WHERE kind=? AND id=?", ref.kind, ref.id).Scan(&one); errors.Is(e, sql.ErrNoRows) {
			return v, domain.E("invalid", ref.kind+" reference does not exist")
		} else if e != nil {
			return v, e
		}
	}
	if q.RunID != "" {
		var b string
		if e = tx.QueryRow("SELECT body FROM runs WHERE id=?", q.RunID).Scan(&b); errors.Is(e, sql.ErrNoRows) {
			return v, domain.E("invalid", "run reference does not exist")
		} else if e != nil {
			return v, e
		}
		var run domain.Run
		if e = json.Unmarshal([]byte(b), &run); e != nil {
			return v, e
		}
		if (q.VersionID != "" && q.VersionID != run.Plan.Version.ID) || (q.EnvironmentID != "" && q.EnvironmentID != run.Plan.Environment.ID) {
			return v, domain.E("invalid", "version/environment references must match the linked run")
		}
	}
	now := time.Now().UTC()
	v = domain.Issue{IssueID: q.IssueID, Revision: q.BaseRevision + 1, IssueFields: q.IssueFields, CreatedAt: now, UpdatedAt: now}
	if old.Revision > 0 {
		v.CreatedAt = old.CreatedAt
	}
	if q.Resolved {
		v.ResolvedAt = &now
		if old.Resolved {
			v.ResolvedAt = old.ResolvedAt
		}
	}
	if old.Revision == 0 {
		_, e = tx.Exec("INSERT INTO issues(issue_id,revision,body) VALUES(?,?,?)", v.IssueID, v.Revision, encode(v))
	} else {
		var result sql.Result
		result, e = tx.Exec("UPDATE issues SET revision=?,body=? WHERE issue_id=? AND revision=?", v.Revision, encode(v), v.IssueID, q.BaseRevision)
		if e == nil {
			n, err := result.RowsAffected()
			if err != nil {
				return v, err
			}
			if n != 1 {
				return v, domain.E("conflict", "issue changed")
			}
		}
	}
	if e != nil {
		return v, e
	}
	if _, e = tx.Exec("INSERT INTO issue_history(issue_id,revision,body) VALUES(?,?,?)", v.IssueID, v.Revision, encode(v)); e != nil {
		return v, e
	}
	return v, tx.Commit()
}

func (s *Store) Issue(id string) (domain.Issue, error) {
	var v domain.Issue
	if e := domain.CheckID(id); e != nil {
		return v, e
	}
	var b string
	e := s.DB.QueryRow("SELECT body FROM issues WHERE issue_id=?", id).Scan(&b)
	if errors.Is(e, sql.ErrNoRows) {
		return v, domain.E("not_found", "issue not found")
	}
	if e != nil {
		return v, e
	}
	e = json.Unmarshal([]byte(b), &v)
	return v, e
}

func (s *Store) Issues() ([]domain.Issue, error) {
	return s.issueRows("SELECT body FROM issues ORDER BY json_extract(body,'$.updated_at') DESC,issue_id", nil)
}
func (s *Store) IssueHistory(id string) ([]domain.Issue, error) {
	if e := domain.CheckID(id); e != nil {
		return nil, e
	}
	out, e := s.issueRows("SELECT body FROM issue_history WHERE issue_id=? ORDER BY revision DESC", []any{id})
	if e == nil && len(out) == 0 {
		e = domain.E("not_found", "issue not found")
	}
	return out, e
}
func (s *Store) issueRows(query string, args []any) ([]domain.Issue, error) {
	rows, e := s.DB.Query(query, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.Issue{}
	for rows.Next() {
		var b string
		var v domain.Issue
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(b), &v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
