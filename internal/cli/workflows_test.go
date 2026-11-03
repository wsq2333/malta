package cli

import (
	"bytes"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/workflow"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkflowCLIAndConfig(t *testing.T) {
	var config workflow.Config
	if e := domain.Read("../../config/parameter/workflow.example.yaml", &config); e != nil || len(config.Connections) != 1 || config.Connections[0].MaxBytes != 256<<30 {
		t.Fatal(config, e)
	}
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schema_version":"1.0","data":[]}`))
	}))
	defer server.Close()
	for _, tc := range []struct {
		args []string
		path string
	}{
		{[]string{"list", "--backend", "argo", "--connection", "dev"}, "/api/v1/workflow/runs?limit=20&offset=0&backend=argo&connection=dev"},
		{[]string{"list", "--limit", "5", "--offset", "10"}, "/api/v1/workflow/runs?limit=5&offset=10"},
		{[]string{"records", "cases", "--id", "test-case"}, "/api/v1/workflow/records/cases?id=test-case"},
		{[]string{"issues", "test-run"}, "/api/v1/workflow/runs/test-run/issues"},
	} {
		var out bytes.Buffer
		r := NewRoot(&out, &out)
		r.SetArgs(append([]string{"--server", server.URL, "workflow"}, tc.args...))
		if e := r.Execute(); e != nil {
			t.Fatal(e)
		}
		if path != tc.path || !strings.Contains(out.String(), `"data":[]`) {
			t.Fatal(path, out.String())
		}
	}
}
