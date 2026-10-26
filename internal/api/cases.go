package api

import (
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/store"
	"net/http"
	"strconv"
)

func caseRoutes(mux *http.ServeMux, s *store.Store) {
	mux.HandleFunc("GET /api/v1/cases", func(w http.ResponseWriter, r *http.Request) { v, e := s.Cases(); send(w, 200, v, e) })
	save := func(w http.ResponseWriter, r *http.Request) {
		var q domain.SaveCase
		e := decode(w, r, &q)
		var v domain.CaseVersion
		if e == nil {
			v, e = s.SaveCase(r.PathValue("id"), q)
		}
		send(w, 201, v, e)
	}
	mux.HandleFunc("POST /api/v1/cases", save)
	mux.HandleFunc("POST /api/v1/cases/{id}/versions", save)
	mux.HandleFunc("GET /api/v1/cases/{id}/versions", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.CaseVersions(r.PathValue("id"))
		send(w, 200, v, e)
	})
	mux.HandleFunc("GET /api/v1/cases/{id}/versions/{revision}", func(w http.ResponseWriter, r *http.Request) {
		n, e := strconv.Atoi(r.PathValue("revision"))
		if e != nil {
			send(w, 400, nil, domain.E("invalid", "revision must be an integer"))
			return
		}
		v, e := s.CaseVersion(r.PathValue("id"), n)
		send(w, 200, v, e)
	})
}
