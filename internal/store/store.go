package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wsq2333/malta/internal/artifact"
	"github.com/wsq2333/malta/internal/domain"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Store struct {
	DB  *sql.DB
	Dir string
	mu  sync.Mutex
}

func Open(dir string) (*Store, error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	db, e := sql.Open("sqlite", filepath.Join(dir, "malta.db"))
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	s := &Store{DB: db, Dir: dir}
	_, e = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; CREATE TABLE IF NOT EXISTS objects(kind TEXT NOT NULL,id TEXT NOT NULL,body TEXT NOT NULL,PRIMARY KEY(kind,id)); CREATE TABLE IF NOT EXISTS runs(id TEXT PRIMARY KEY,request_id TEXT UNIQUE,body TEXT NOT NULL,digest TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL); PRAGMA user_version=1;`)
	if e != nil {
		db.Close()
		return nil, e
	}
	return s, nil
}
func (s *Store) Close() error { return s.DB.Close() }
func encode(v any) string     { b, _ := json.Marshal(v); return string(b) }
func (s *Store) Register(kind, id string, v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.register(s.DB, kind, id, v)
}

type queryExec interface {
	QueryRow(string, ...any) *sql.Row
	Exec(string, ...any) (sql.Result, error)
}

func (s *Store) register(db queryExec, kind, id string, v any) error {
	var body string
	e := db.QueryRow("SELECT body FROM objects WHERE kind=? AND id=?", kind, id).Scan(&body)
	if e == nil {
		if body == encode(v) {
			return nil
		}
		return domain.E("conflict", "immutable "+kind+" ID has different content")
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	_, e = db.Exec("INSERT INTO objects(kind,id,body) VALUES(?,?,?)", kind, id, encode(v))
	return e
}
func (s *Store) Object(kind, id string, v any) error {
	var b string
	e := s.DB.QueryRow("SELECT body FROM objects WHERE kind=? AND id=?", kind, id).Scan(&b)
	if errors.Is(e, sql.ErrNoRows) {
		return domain.E("not_found", kind+" not found")
	}
	if e != nil {
		return e
	}
	return json.Unmarshal([]byte(b), v)
}
func (s *Store) Objects(kind string) ([]json.RawMessage, error) {
	rows, e := s.DB.Query("SELECT body FROM objects WHERE kind=? ORDER BY id", kind)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var b string
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		out = append(out, json.RawMessage(b))
	}
	return out, rows.Err()
}
func (s *Store) Run(id string) (domain.Run, error) {
	var r domain.Run
	var b string
	e := s.DB.QueryRow("SELECT body FROM runs WHERE id=?", id).Scan(&b)
	if errors.Is(e, sql.ErrNoRows) {
		return r, domain.E("not_found", "run not found")
	}
	if e != nil {
		return r, e
	}
	e = json.Unmarshal([]byte(b), &r)
	return r, e
}

type Filter struct {
	Environment string
	Version     string
	Kind        string
	Status      string
	From        string
	To          string
	Limit       int
	Offset      int
}

func (s *Store) Runs(f Filter) ([]domain.Run, error) {
	if f.Limit == 0 {
		f.Limit = 100
	}
	if f.Limit < 1 || f.Limit > 500 || f.Offset < 0 {
		return nil, domain.E("invalid", "limit must be 1..500 and offset nonnegative")
	}
	for _, v := range []string{f.From, f.To} {
		if v != "" {
			if _, e := time.Parse(time.RFC3339, v); e != nil {
				return nil, domain.E("invalid", "dates must be RFC3339")
			}
		}
	}
	q := "SELECT body FROM runs WHERE 1=1"
	args := []any{}
	for _, v := range []struct{ k, v string }{{"$.plan.environment.id", f.Environment}, {"$.plan.version.id", f.Version}, {"$.plan.spec.kind", f.Kind}, {"$.status", f.Status}} {
		if v.v != "" {
			q += " AND json_extract(body,?)=?"
			args = append(args, v.k, v.v)
		}
	}
	if f.From != "" {
		q += " AND julianday(created_at)>=julianday(?)"
		args = append(args, f.From)
	}
	if f.To != "" {
		q += " AND julianday(created_at)<=julianday(?)"
		args = append(args, f.To)
	}
	q += " ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?"
	args = append(args, f.Limit, f.Offset)
	rows, e := s.DB.Query(q, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.Run{}
	for rows.Next() {
		var b string
		var r domain.Run
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(b), &r); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Store) Import(b artifact.Bundle) error {
	if e := b.Validate(); e != nil {
		return e
	}
	if e := VerifyEvidence(b); e != nil {
		return e
	}
	if !domain.Terminal(b.Run.Status) {
		return domain.E("invalid", "only terminal result bundles can be imported")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	digest := artifact.Digest(b)
	var existing string
	e := s.DB.QueryRow("SELECT digest FROM runs WHERE id=?", b.Run.ID).Scan(&existing)
	if e == nil {
		if existing == digest {
			return nil
		}
		return domain.E("conflict", "run ID already exists with different contents")
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	tx, e := s.DB.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = s.register(tx, "environment", b.Run.Plan.Environment.ID, b.Run.Plan.Environment); e != nil {
		return e
	}
	if e = s.register(tx, "version", b.Run.Plan.Version.ID, b.Run.Plan.Version); e != nil {
		return e
	}
	dest := filepath.Join(s.Dir, "artifacts", b.Run.ID)
	if _, e = os.Stat(dest); e == nil {
		old, err := artifact.Read(dest)
		if err != nil || artifact.Digest(old) != digest {
			return domain.E("conflict", "existing artifact directory differs")
		}
	} else if e = artifact.Write(dest, b); e != nil {
		return e
	}
	_, e = tx.Exec("INSERT INTO runs(id,request_id,body,digest,created_at) VALUES(?,NULL,?,?,?)", b.Run.ID, encode(b.Run), digest, b.Run.CreatedAt.Format(time.RFC3339Nano))
	if e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) Bundle(id string) (artifact.Bundle, error) {
	if e := domain.CheckID(id); e != nil {
		return artifact.Bundle{}, e
	}
	if _, e := s.Run(id); e != nil {
		return artifact.Bundle{}, e
	}
	b, e := artifact.Read(filepath.Join(s.Dir, "artifacts", id))
	if os.IsNotExist(e) {
		return b, domain.E("not_found", "result evidence is not available yet")
	}
	return b, e
}
func (s *Store) SetBaseline(name, id string) (domain.Baseline, error) {
	b := domain.Baseline{ID: domain.ID("baseline"), Name: name, RunID: id, CreatedAt: time.Now().UTC()}
	if name == "" {
		return b, domain.E("invalid", "baseline name required")
	}
	r, e := s.Run(id)
	if e != nil {
		return b, e
	}
	if r.Status != "succeeded" {
		return b, domain.E("invalid", "baseline run must have succeeded")
	}
	if c := domain.Compare(r, r); !c.Comparable {
		return b, domain.E("invalid", fmt.Sprint(c.Differences))
	}
	if _, e = s.Bundle(id); e != nil {
		return b, e
	}
	return b, s.Register("baseline", b.ID, b)
}
func (s *Store) Compare(a, b string) (domain.Comparison, error) {
	x, e := s.Run(a)
	if e != nil {
		return domain.Comparison{}, e
	}
	y, e := s.Run(b)
	if e != nil {
		return domain.Comparison{}, e
	}
	return domain.Compare(x, y), nil
}
