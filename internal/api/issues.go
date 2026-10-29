package api

import (
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/store"
	"net/http"
)

func issueRoutes(mux *http.ServeMux, s *store.Store) {
	mux.HandleFunc("GET /api/v1/issues", func(w http.ResponseWriter, r *http.Request) { v, e := s.Issues(); send(w, 200, v, e) })
	mux.HandleFunc("POST /api/v1/issues", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
		var q domain.SaveIssue
		e := decode(w, r, &q)
		var v domain.Issue
		if e == nil {
			v, e = s.SaveIssue(q)
		}
		send(w, 201, v, e)
	})
	mux.HandleFunc("GET /api/v1/issues/{id}", func(w http.ResponseWriter, r *http.Request) { v, e := s.Issue(r.PathValue("id")); send(w, 200, v, e) })
	mux.HandleFunc("GET /api/v1/issues/{id}/history", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.IssueHistory(r.PathValue("id"))
		send(w, 200, v, e)
	})
}
