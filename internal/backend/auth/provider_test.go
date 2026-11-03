package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func secret(t *testing.T, s string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "value")
	if e := os.WriteFile(p, []byte(s), 0600); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestPasswordRefreshRotationAndSingleFlight(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		n := calls.Add(1)
		if n == 1 || n == 4 {
			if r.Form.Get("grant_type") != "password" || r.Form.Get("username") != "test-user" {
				t.Error("missing login")
			}
		}
		if n == 2 || n == 3 {
			if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "refresh-1" {
				t.Error("missing refresh")
			}
		}
		if n == 3 {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"invalid_grant","error_description":"never log secret-password"}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": "token", "refresh_token": "refresh-1", "token_type": "Bearer", "expires_in": 300})
	}))
	defer server.Close()
	p, e := New(Config{Type: "oauth2_password", TokenURL: server.URL, ClientID: "client", UsernameFile: secret(t, "test-user"), PasswordFile: secret(t, "secret-password")})
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := p.Token(context.Background()); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("login stampede", calls.Load())
	}
	p.renew = time.Now().Add(-time.Second)
	if _, e = p.Token(context.Background()); e != nil {
		t.Fatal(e)
	}
	if calls.Load() != 2 {
		t.Fatal("not refreshed")
	}
	p.renew = time.Now().Add(-time.Second)
	if _, e = p.Token(context.Background()); e != nil {
		t.Fatal(e)
	}
	if calls.Load() != 4 {
		t.Fatal("invalid refresh did not reauthenticate")
	}
}
func TestClientCredentialsRotationAndErrorsRedacted(t *testing.T) {
	path := secret(t, "one")
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		calls++
		if r.Form.Get("grant_type") != "client_credentials" {
			t.Error("wrong grant")
		}
		if calls == 2 && r.Form.Get("client_secret") != "two" {
			t.Error("Secret rotation ignored")
		}
		if calls > 2 {
			w.WriteHeader(401)
			w.Write([]byte(`{"error":"invalid_client","error_description":"two"}`))
			return
		}
		w.Write([]byte(`{"access_token":"a","token_type":"Bearer","expires_in":100}`))
	}))
	defer server.Close()
	p, e := New(Config{Type: "oauth2_client_credentials", TokenURL: server.URL, ClientID: "c", ClientSecretFile: path})
	if e != nil {
		t.Fatal(e)
	}
	p.Token(context.Background())
	os.WriteFile(path, []byte("two"), 0600)
	if _, e = p.Token(context.Background()); e != nil {
		t.Fatal(e)
	}
	if calls != 2 {
		t.Fatal("rotation did not login")
	}
	p.Invalidate("a")
	_, e = p.Token(context.Background())
	if e == nil || strings.Contains(e.Error(), "two") {
		t.Fatal("unredacted error", e)
	}
	if _, e = New(Config{Type: "oauth2_password", TokenURL: "http://example.com/token", ClientID: "c", UsernameFile: path, PasswordFile: path}); e == nil {
		t.Fatal("accepted insecure token endpoint")
	}
}
func TestTokenRedirectDenied(t *testing.T) {
	var hits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer server.Close()
	p, _ := New(Config{Type: "oauth2_client_credentials", TokenURL: server.URL, ClientID: "c", ClientSecretFile: secret(t, "s")})
	if _, e := p.Token(context.Background()); e == nil {
		t.Fatal("accepted redirect")
	}
	if hits.Load() != 0 {
		t.Fatal("leaked credentials")
	}
}
