package api

import (
	"encoding/json"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/workflow"
	"net/http"
	"strconv"
)

func workflowRoutes(mux *http.ServeMux, m *workflow.Manager) {
	mux.HandleFunc("GET /api/v1/workflow/runs/{id}/issues", func(w http.ResponseWriter, r *http.Request) {
		v, e := m.IssueLinks(r.PathValue("id"), "")
		send(w, 200, v, e)
	})
	mux.HandleFunc("GET /api/v1/workflow/issues/{id}/runs", func(w http.ResponseWriter, r *http.Request) {
		v, e := m.IssueLinks("", r.PathValue("id"))
		send(w, 200, v, e)
	})
	mux.HandleFunc("POST /api/v1/workflow/runs/{id}/issues", func(w http.ResponseWriter, r *http.Request) {
		var q workflow.IssueLinkRequest
		e := decode(w, r, &q)
		var v workflow.IssueLink
		if e == nil {
			v, e = m.LinkIssue(r.PathValue("id"), q)
		}
		send(w, 201, v, e)
	})
	mux.HandleFunc("GET /api/v1/workflow/connections", func(w http.ResponseWriter, r *http.Request) { send(w, 200, m.Connections(), nil) })
	mux.HandleFunc("GET /api/v1/workflow/records/{kind}", func(w http.ResponseWriter, r *http.Request) {
		v, e := m.Records(r.PathValue("kind"), r.URL.Query().Get("id"))
		send(w, 200, v, e)
	})
	mux.HandleFunc("POST /api/v1/workflow/records/{kind}", func(w http.ResponseWriter, r *http.Request) {
		var q workflow.SaveRecord
		e := decode(w, r, &q)
		var v workflow.Record
		if e == nil {
			v, e = m.Save(r.PathValue("kind"), q)
		}
		send(w, 201, v, e)
	})
	mux.HandleFunc("POST /api/v1/workflow/preflight", func(w http.ResponseWriter, r *http.Request) {
		var q workflow.Selection
		e := decode(w, r, &q)
		var v workflow.Preflight
		if e == nil {
			v, e = m.Preflight(q)
		}
		send(w, 200, v, e)
	})
	mux.HandleFunc("POST /api/v1/workflow/runs", func(w http.ResponseWriter, r *http.Request) {
		var q workflow.Start
		e := decode(w, r, &q)
		var v workflow.Run
		if e == nil {
			v, e = m.Start(q)
		}
		send(w, 202, v, e)
	})
	mux.HandleFunc("GET /api/v1/workflow/runs", func(w http.ResponseWriter, r *http.Request) {
		limit, offset := 20, 0
		var e error
		if q := r.URL.Query().Get("limit"); q != "" {
			limit, e = strconv.Atoi(q)
		}
		if q := r.URL.Query().Get("offset"); q != "" && e == nil {
			offset, e = strconv.Atoi(q)
		}
		if e != nil {
			send(w, 400, nil, domain.E("invalid", "invalid pagination"))
			return
		}
		v, e := m.FilteredRuns(limit, offset, r.URL.Query().Get("backend"), r.URL.Query().Get("connection"))
		send(w, 200, v, e)
	})
	mux.HandleFunc("GET /api/v1/workflow/requests/{id}", func(w http.ResponseWriter, r *http.Request) { v, e := m.Lookup(r.PathValue("id")); send(w, 200, v, e) })
	mux.HandleFunc("GET /api/v1/workflow/runs/{id}", func(w http.ResponseWriter, r *http.Request) { v, e := m.Get(r.PathValue("id")); send(w, 200, v, e) })
	mux.HandleFunc("POST /api/v1/workflow/runs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) { v, e := m.Cancel(r.PathValue("id")); send(w, 200, v, e) })
	mux.HandleFunc("POST /api/v1/workflow/runs/{id}/collect", func(w http.ResponseWriter, r *http.Request) {
		v, e := m.RetryCollection(r.PathValue("id"))
		send(w, 200, v, e)
	})
	mux.HandleFunc("GET /api/v1/workflow/runs/{id}/export", func(w http.ResponseWriter, r *http.Request) {
		v, e := m.Get(r.PathValue("id"))
		if e != nil {
			send(w, 400, nil, e)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename="+v.ID+".json")
		_ = json.NewEncoder(w).Encode(v)
	})
	mux.HandleFunc("GET /api/v1/workflow/runs/{id}/artifacts/{name}", func(w http.ResponseWriter, r *http.Request) {
		v, e := m.Artifact(r.PathValue("id"), r.PathValue("name"))
		if e != nil {
			send(w, 400, nil, e)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(v)
	})
	mux.HandleFunc("POST /api/v1/workflow/compare", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Baseline  string `json:"baseline"`
			Candidate string `json:"candidate"`
		}
		e := decode(w, r, &q)
		var a, b workflow.Run
		var v map[string]any
		if e == nil {
			a, e = m.Get(q.Baseline)
		}
		if e == nil {
			b, e = m.Get(q.Candidate)
		}
		if e == nil {
			v, e = workflow.Compare(a, b)
		}
		send(w, 200, v, e)
	})
}
