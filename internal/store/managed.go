package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/wsq2333/malta/internal/artifact"
	"github.com/wsq2333/malta/internal/domain"
	"path/filepath"
	"time"
)

func (s *Store) ByRequest(id string) (domain.Run, error) {
	var r domain.Run
	var body string
	e := s.DB.QueryRow("SELECT body FROM runs WHERE request_id=?", id).Scan(&body)
	if errors.Is(e, sql.ErrNoRows) {
		return r, domain.E("not_found", "request not found")
	}
	if e != nil {
		return r, e
	}
	e = json.Unmarshal([]byte(body), &r)
	return r, e
}

func (s *Store) CreateManaged(p domain.Plan, request, source string) (domain.Run, error) {
	r := domain.NewRun(p, source)
	r.RequestID = request
	if e := domain.CheckID(request); e != nil {
		return r, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var body string
	e := s.DB.QueryRow("SELECT body FROM runs WHERE request_id=?", request).Scan(&body)
	if e == nil {
		var old domain.Run
		if e = json.Unmarshal([]byte(body), &old); e != nil {
			return r, e
		}
		if old.Plan.Hash != p.Hash || old.Source != source {
			return r, domain.E("conflict", "request_id already refers to another plan or source")
		}
		return old, nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return r, e
	}
	tx, e := s.DB.Begin()
	if e != nil {
		return r, e
	}
	defer tx.Rollback()
	if e = s.register(tx, "environment", p.Environment.ID, p.Environment); e != nil {
		return r, e
	}
	if e = s.register(tx, "version", p.Version.ID, p.Version); e != nil {
		return r, e
	}
	_, e = tx.Exec("INSERT INTO runs(id,request_id,body,created_at) VALUES(?,?,?,?)", r.ID, request, encode(r), r.CreatedAt.Format(time.RFC3339Nano))
	if e != nil {
		return r, e
	}
	return r, tx.Commit()
}
func (s *Store) Update(r domain.Run) error {
	if e := r.Validate(); e != nil {
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res, e := s.DB.Exec("UPDATE runs SET body=? WHERE id=? AND digest=''", encode(r), r.ID)
	if e != nil {
		return e
	}
	n, e := res.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return domain.E("conflict", "run is missing or evidence already finalized")
	}
	return nil
}
func (s *Store) Finish(b artifact.Bundle) error {
	if e := b.Validate(); e != nil {
		return e
	}
	if e := VerifyEvidence(b); e != nil {
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := artifact.Write(filepath.Join(s.Dir, "artifacts", b.Run.ID), b); e != nil {
		return e
	}
	res, e := s.DB.Exec("UPDATE runs SET body=?,digest=? WHERE id=? AND digest=''", encode(b.Run), artifact.Digest(b), b.Run.ID)
	if e != nil {
		return e
	}
	n, e := res.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return domain.E("conflict", "run already finalized")
	}
	return nil
}
func (s *Store) NextPending() (domain.Run, error) {
	var r domain.Run
	var b string
	e := s.DB.QueryRow("SELECT body FROM runs WHERE json_extract(body,'$.status')='pending' AND request_id IS NOT NULL ORDER BY created_at,id LIMIT 1").Scan(&b)
	if errors.Is(e, sql.ErrNoRows) {
		return r, domain.E("not_found", "queue empty")
	}
	if e != nil {
		return r, e
	}
	e = json.Unmarshal([]byte(b), &r)
	return r, e
}
func (s *Store) Recover() error {
	rows, e := s.DB.Query("SELECT body FROM runs WHERE request_id IS NOT NULL AND digest=''")
	if e != nil {
		return e
	}
	var runs []domain.Run
	for rows.Next() {
		var b string
		var r domain.Run
		if e = rows.Scan(&b); e != nil {
			rows.Close()
			return e
		}
		if e = json.Unmarshal([]byte(b), &r); e != nil {
			rows.Close()
			return e
		}
		runs = append(runs, r)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, r := range runs {
		if domain.Terminal(r.Status) && r.Attempt.ResultState == "failed" {
			continue
		}
		r.Status = "interrupted"
		r.Error = "service stopped before evidence was durably finalized; not automatically retried"
		r.Attempt.Status = "interrupted"
		r.Attempt.ResultState = "failed"
		r.Attempt.FinishedAt = time.Now().UTC()
		r.UpdatedAt = time.Now().UTC()
		if e = s.Update(r); e != nil {
			return e
		}
	}
	return nil
}
