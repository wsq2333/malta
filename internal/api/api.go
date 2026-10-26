package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"github.com/wsq2333/malta/internal/artifact"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/service"
	"github.com/wsq2333/malta/internal/store"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Envelope struct {
	SchemaVersion string        `json:"schema_version"`
	Data          any           `json:"data,omitempty"`
	Error         *domain.Error `json:"error,omitempty"`
}

func send(w http.ResponseWriter, status int, v any, e error) {
	w.Header().Set("Content-Type", "application/json")
	env := Envelope{SchemaVersion: domain.Schema, Data: v}
	if e != nil {
		var d *domain.Error
		if !errors.As(e, &d) {
			d = &domain.Error{Code: "internal", Message: e.Error()}
		}
		env.Data = nil
		env.Error = d
		switch d.Code {
		case "invalid", "unsupported":
			status = 400
		case "conflict":
			status = 409
		case "not_found":
			status = 404
		case "unauthorized":
			status = 401
		default:
			status = 500
		}
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(env)
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, artifact.MaxBytes*2)
	if e := domain.DecodeJSON(r.Body, v); e != nil {
		return domain.E("invalid", e.Error())
	}
	return nil
}
func Handler(m *service.Manager, token string, frontend http.Handler) http.Handler {
	mux := http.NewServeMux()
	s := m.Store
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		send(w, 200, map[string]string{"status": "ok", "version": domain.Version}, nil)
	})
	for _, kind := range []string{"environment", "version"} {
		kind := kind
		base := "/api/v1/" + kind + "s"
		mux.HandleFunc("GET "+base, func(w http.ResponseWriter, r *http.Request) { v, e := s.Objects(kind); send(w, 200, v, e) })
		mux.HandleFunc("GET "+base+"/{id}", func(w http.ResponseWriter, r *http.Request) {
			var v json.RawMessage
			e := s.Object(kind, r.PathValue("id"), &v)
			send(w, 200, v, e)
		})
		mux.HandleFunc("POST "+base, func(w http.ResponseWriter, r *http.Request) {
			var v any
			var id string
			var e error
			if kind == "environment" {
				var value domain.Environment
				e = decode(w, r, &value)
				if e == nil {
					e = value.Validate()
				}
				v = value
				id = value.ID
			} else {
				var value domain.SoftwareVersion
				e = decode(w, r, &value)
				if e == nil {
					e = value.Validate()
				}
				v = value
				id = value.ID
			}
			if e == nil {
				e = s.Register(kind, id, v)
			}
			send(w, 201, v, e)
		})
	}
	mux.HandleFunc("GET /api/v1/runs", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit, offset := 100, 0
		var e error
		if q.Get("limit") != "" {
			limit, e = strconv.Atoi(q.Get("limit"))
		}
		if e == nil && q.Get("offset") != "" {
			offset, e = strconv.Atoi(q.Get("offset"))
		}
		if e != nil {
			send(w, 400, nil, domain.E("invalid", "pagination must be integers"))
			return
		}
		v, e := s.Runs(store.Filter{Environment: q.Get("environment"), Version: q.Get("version"), Kind: q.Get("kind"), Status: q.Get("status"), From: q.Get("from"), To: q.Get("to"), Limit: limit, Offset: offset})
		send(w, 200, v, e)
	})
	mux.HandleFunc("POST /api/v1/runs", func(w http.ResponseWriter, r *http.Request) {
		var q service.StartRequest
		e := decode(w, r, &q)
		var v domain.Run
		if e == nil {
			v, e = m.Start(q)
		}
		send(w, 202, v, e)
	})
	mux.HandleFunc("GET /api/v1/runs/{id}", func(w http.ResponseWriter, r *http.Request) { v, e := s.Run(r.PathValue("id")); send(w, 200, v, e) })
	mux.HandleFunc("POST /api/v1/runs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) { v, e := m.Cancel(r.PathValue("id")); send(w, 200, v, e) })
	mux.HandleFunc("GET /api/v1/runs/{id}/bundle", func(w http.ResponseWriter, r *http.Request) { v, e := s.Bundle(r.PathValue("id")); send(w, 200, v, e) })
	mux.HandleFunc("GET /api/v1/runs/{id}/artifacts/{path...}", func(w http.ResponseWriter, r *http.Request) {
		b, e := s.Bundle(r.PathValue("id"))
		var data []byte
		if e == nil {
			data, e = artifact.Get(b, r.PathValue("path"))
		}
		if e != nil {
			send(w, 400, nil, e)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(data)
	})
	mux.HandleFunc("POST /api/v1/imports", func(w http.ResponseWriter, r *http.Request) {
		var b artifact.Bundle
		e := decode(w, r, &b)
		if e == nil {
			e = s.Import(b)
		}
		send(w, 201, b.Run, e)
	})
	mux.HandleFunc("GET /api/v1/baselines", func(w http.ResponseWriter, r *http.Request) { v, e := s.Objects("baseline"); send(w, 200, v, e) })
	mux.HandleFunc("POST /api/v1/baselines", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Name  string `json:"name"`
			RunID string `json:"run_id"`
		}
		e := decode(w, r, &q)
		var v domain.Baseline
		if e == nil {
			v, e = s.SetBaseline(q.Name, q.RunID)
		}
		send(w, 201, v, e)
	})
	mux.HandleFunc("POST /api/v1/comparisons", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Baseline  string `json:"baseline"`
			Candidate string `json:"candidate"`
		}
		e := decode(w, r, &q)
		var v domain.Comparison
		if e == nil {
			v, e = s.Compare(q.Baseline, q.Candidate)
		}
		send(w, 200, v, e)
	})
	mux.HandleFunc("GET /api/v1/trends", func(w http.ResponseWriter, r *http.Request) {
		anchor, e := s.Run(r.URL.Query().Get("run_id"))
		if e != nil {
			send(w, 404, nil, e)
			return
		}
		runs, e := s.Runs(store.Filter{Environment: anchor.Plan.Environment.ID, Kind: anchor.Plan.Spec.Kind, Status: "succeeded", Limit: 500})
		type point struct {
			Run     domain.Run       `json:"run"`
			Metrics []domain.Summary `json:"metrics"`
		}
		points := []point{}
		for _, v := range runs {
			if domain.Compare(anchor, v).Comparable {
				points = append(points, point{v, domain.Summarize(v)})
			}
		}
		send(w, 200, map[string]any{"points": points, "limit": 500, "scope": "latest 500 successful runs in the selected environment; matching conditions only"}, e)
	})
	mux.HandleFunc("GET /api/v1/runs/{id}/logs", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if e := domain.CheckID(id); e != nil {
			send(w, 400, nil, e)
			return
		}
		run, e := s.Run(id)
		if e != nil {
			send(w, 404, nil, e)
			return
		}
		type entry struct {
			Path      string `json:"path"`
			Text      string `json:"text"`
			Truncated bool   `json:"truncated"`
		}
		logs := []entry{}
		b, e := s.Bundle(id)
		if e == nil {
			for _, f := range b.Files {
				if strings.HasSuffix(f.Path, ".stderr") {
					data := f.Data
					truncated := len(data) > 65536
					if truncated {
						data = data[len(data)-65536:]
					}
					logs = append(logs, entry{f.Path, string(data), truncated})
				}
			}
		} else {
			for _, p := range run.Plan.Trials {
				for _, phase := range []string{"prepare", "measure"} {
					rel := p.ID + "/" + phase + ".stderr"
					file, e := os.Open(filepath.Join(s.Dir, "work", id, rel))
					if e != nil {
						continue
					}
					info, e := file.Stat()
					truncated := false
					if e == nil && info.Size() > 65536 {
						_, _ = file.Seek(-65536, io.SeekEnd)
						truncated = true
					}
					data, _ := io.ReadAll(io.LimitReader(file, 65536))
					file.Close()
					logs = append(logs, entry{rel, string(data), truncated})
				}
			}
		}
		if run.Attempt.Executor == "kubernetes" && !domain.Terminal(run.Status) {
			if f, e := os.Open(filepath.Join(s.Dir, "work", id, "runner.stderr")); e == nil {
				data, _ := io.ReadAll(io.LimitReader(f, 65537))
				f.Close()
				truncated := len(data) > 65536
				if truncated {
					data = data[:65536]
				}
				logs = append(logs, entry{"runner.stderr", string(data), truncated})
			}
		}
		sort.Slice(logs, func(i, j int) bool { return logs[i].Path < logs[j].Path })
		send(w, 200, map[string]any{"status": run.Status, "entries": logs, "format": "snapshot"}, nil)
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		send(w, 404, nil, domain.E("not_found", "API route not found"))
	})
	if frontend != nil {
		mux.Handle("/", frontend)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if token == "" {
				host, _, e := net.SplitHostPort(r.Host)
				if e != nil {
					host = r.Host
				}
				ip := net.ParseIP(host)
				if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
					send(w, 403, nil, domain.E("unauthorized", "unauthenticated API requires a loopback Host"))
					return
				}
			}
			if r.Method != "GET" && r.Method != "HEAD" {
				if origin := r.Header.Get("Origin"); origin != "" {
					u, e := url.Parse(origin)
					if e != nil || u.Host != r.Host {
						send(w, 403, nil, domain.E("unauthorized", "cross-origin mutations are not allowed"))
						return
					}
				}
				if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
					send(w, 400, nil, domain.E("invalid", "application/json is required"))
					return
				}
			}
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/v1/health" && token != "" {
			value := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(value), []byte(token)) != 1 {
				send(w, 401, nil, domain.E("unauthorized", "valid bearer token required"))
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
