package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var assets embed.FS

func Handler() http.Handler {
	root, _ := fs.Sub(assets, "dist")
	files := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			http.Error(w, "method not allowed", 405)
			return
		}
		if _, e := fs.Stat(root, "index.html"); e != nil {
			http.Error(w, "Build web assets with pnpm --dir web build, then rebuild Malta.", 503)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if strings.HasPrefix(path, ".") {
			http.NotFound(w, r)
			return
		}
		if path == "" {
			files.ServeHTTP(w, r)
			return
		}
		if _, e := fs.Stat(root, path); e != nil {
			if strings.Contains(path, ".") {
				http.NotFound(w, r)
				return
			}
			copy := r.Clone(r.Context())
			copy.URL.Path = "/"
			files.ServeHTTP(w, copy)
			return
		}
		files.ServeHTTP(w, r)
	})
}
