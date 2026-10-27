package web

import (
	"io/fs"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFrontend(t *testing.T) {
	handler := Handler()
	_, built := fs.Stat(assets, "dist/index.html")
	for _, path := range []string{"/", "/runs"} {
		req := httptest.NewRequest("GET", "http://localhost"+path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if built == nil {
			if w.Code != 200 || !strings.Contains(w.Body.String(), "id=\"root\"") {
				t.Fatal(path, w.Code, w.Body.String())
			}
		} else if w.Code != 503 {
			t.Fatal("missing assets should give actionable response", w.Code)
		}
	}
	for _, path := range []string{"/.gitkeep", "/missing.js"} {
		if built != nil {
			continue
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "http://localhost"+path, nil))
		if w.Code != 404 {
			t.Fatal(path, w.Code)
		}
	}
}
