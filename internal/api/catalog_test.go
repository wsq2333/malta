package api

import (
	"encoding/json"
	"github.com/wsq2333/malta/internal/service"
	"github.com/wsq2333/malta/internal/store"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCatalogRouteValidationAndAuthentication(t *testing.T) {
	s, e := store.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	m, e := service.New(s, service.Config{ManagementOnly: true, MaxBytes: 1 << 30, Timeout: time.Minute})
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	server := httptest.NewServer(Handler(m, "secret", nil))
	defer server.Close()
	if status, _ := call(t, server.URL, "GET", "/api/v1/run-catalog", "", nil); status != 401 {
		t.Fatal(status)
	}
	for _, q := range []string{"limit=0", "offset=-1", "limit=no", "mode=conditions", "saved=maybe", "from=wrong", "kind=unknown", "operation=invalid", "file_size=-1", "block_size=1.2"} {
		if status, _ := call(t, server.URL, "GET", "/api/v1/run-catalog?"+q, "secret", nil); status != 400 {
			t.Fatal(q, status)
		}
	}
	status, body := call(t, server.URL, "GET", "/api/v1/run-catalog?mode=history&environment=env-a&environment=env-b&limit=20", "secret", nil)
	var page store.CatalogPage
	if status != 200 {
		t.Fatal(status, string(body))
	}
	if e = json.Unmarshal(body, &page); e != nil || page.Total != 0 || page.Limit != 20 || page.Runs == nil {
		t.Fatal(page, e)
	}
}
