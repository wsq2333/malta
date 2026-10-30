package api

import (
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/store"
	"net/http"
	"strconv"
)

func catalogRoutes(mux *http.ServeMux, s *store.Store) {
	mux.HandleFunc("GET /api/v1/run-catalog", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		f := store.CatalogFilter{Operation: q.Get("operation"), Environments: q["environment"], Version: q.Get("version"), Kind: q.Get("kind"), Status: q.Get("status"), From: q.Get("from"), To: q.Get("to"), Search: q.Get("search"), Project: q.Get("project"), Condition: q.Get("condition"), Mode: q.Get("mode"), Saved: q.Get("saved") == "true", Limit: 20}
		if f.Mode == "" {
			f.Mode = "overview"
		}
		var e error
		if q.Has("limit") {
			f.Limit, e = strconv.Atoi(q.Get("limit"))
		}
		if e == nil && q.Has("offset") {
			f.Offset, e = strconv.Atoi(q.Get("offset"))
		}
		if e == nil && q.Has("file_size") {
			f.FileSize, e = strconv.ParseInt(q.Get("file_size"), 10, 64)
		}
		if e == nil && q.Has("block_size") {
			f.BlockSize, e = strconv.ParseInt(q.Get("block_size"), 10, 64)
		}
		if e != nil || (q.Has("saved") && q.Get("saved") != "true" && q.Get("saved") != "false") {
			send(w, 400, nil, domain.E("invalid", "invalid catalog query"))
			return
		}
		result, e := s.Catalog(r.Context(), f)
		send(w, 200, result, e)
	})
}
