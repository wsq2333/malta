package httpapi

import (
	"context"
	"github.com/wsq2333/malta/internal/backend"
	"github.com/wsq2333/malta/internal/backend/auth"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func Test401ReplayPreservesKeyAndBody(t *testing.T) {
	count := 0
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if count == 1 {
			w.Write([]byte(`{"access_token":"expired","token_type":"Bearer","expires_in":100}`))
		} else {
			w.Write([]byte(`{"access_token":"fresh","token_type":"Bearer","expires_in":100}`))
		}
	}))
	defer oauth.Close()
	requests := 0
	var first string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		b, _ := io.ReadAll(r.Body)
		if requests == 1 {
			first = string(b)
			w.WriteHeader(401)
			return
		}
		if string(b) != first || r.Header.Get("Idempotency-Key") != "same" || r.Header.Get("Authorization") != "Bearer fresh" {
			t.Error("unsafe replay")
		}
		w.Write([]byte(`{}`))
	}))
	defer api.Close()
	file := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(file, []byte("s"), 0600)
	c, e := New(backend.Config{URL: api.URL, Auth: &auth.Config{Type: "oauth2_client_credentials", TokenURL: oauth.URL, ClientID: "c", ClientSecretFile: file}})
	if e != nil {
		t.Fatal(e)
	}
	if e = c.Call(context.Background(), "POST", "/submit", map[string]string{"Idempotency-Key": "same"}, map[string]string{"input": "value"}, nil); e != nil {
		t.Fatal(e)
	}
	if requests != 2 || count != 2 {
		t.Fatal(requests, count)
	}
}
