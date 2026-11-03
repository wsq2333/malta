package uce

import (
	"context"
	"encoding/json"
	"github.com/wsq2333/malta/internal/backend"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSubmissionContractAndTokenRotation(t *testing.T) {
	token := filepath.Join(t.TempDir(), "token")
	_ = os.WriteFile(token, []byte("first"), 0600)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		want := "first"
		if calls == 2 {
			want = "second"
		}
		if r.Header.Get("Authorization") != "Bearer "+want {
			t.Error("token not read per request")
		}
		if r.URL.Path != "/v2/workflow-runs" || r.Header.Get("timeout") != "120" || r.Header.Get("Idempotency-Key") != "stable" {
			t.Error("submission contract mismatch")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["execution_backend"] != "yarlung" || body["inputs"].(map[string]any)["count"].(map[string]any)["value"] != float64(4) {
			t.Error("wrong input envelope")
		}
		_, _ = w.Write([]byte(`{"run_id":"run-example"}`))
	}))
	defer server.Close()
	c, e := New(Config{URL: server.URL, TokenFile: token})
	if e != nil {
		t.Fatal(e)
	}
	s := backend.Submission{VersionID: "ep", Key: "stable", TimeoutSeconds: 120, Inputs: map[string]any{"count": 4}}
	for _, value := range []string{"first", "second"} {
		_ = os.WriteFile(token, []byte(value), 0600)
		if _, e = c.Submit(context.Background(), s); e != nil {
			t.Fatal(e)
		}
	}
}
func TestNoRedirectCredentialLeak(t *testing.T) {
	hit := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer server.Close()
	token := filepath.Join(t.TempDir(), "token")
	_ = os.WriteFile(token, []byte("secret"), 0600)
	c, _ := New(Config{URL: server.URL, TokenFile: token})
	if _, e := c.Lookup(context.Background(), "key"); e == nil {
		t.Fatal("accepted redirect")
	}
	if hit {
		t.Fatal("followed credential redirect")
	}
}
func TestCancellationRequiresAllPages(t *testing.T) {
	n := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 1 {
			_, _ = w.Write([]byte(`{"tasks":[{"status":"Succeeded"}],"next_cursor":"next"}`))
		} else {
			_, _ = w.Write([]byte(`{"tasks":[{"status":"Retrying"}],"next_cursor":""}`))
		}
	}))
	defer server.Close()
	token := filepath.Join(t.TempDir(), "token")
	_ = os.WriteFile(token, []byte("secret"), 0600)
	c, _ := New(Config{URL: server.URL, TokenFile: token})
	ok, e := c.CancellationComplete(context.Background(), "r")
	if e != nil || ok || n != 2 {
		t.Fatal(ok, e, n)
	}
}
