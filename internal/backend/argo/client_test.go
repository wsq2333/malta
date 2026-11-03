package argo

import (
	"context"
	"encoding/json"
	"github.com/wsq2333/malta/internal/backend"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeTemplateIdempotencyIdentityAndEvidence(t *testing.T) {
	var template, run map[string]any
	creates := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing auth")
		}
		if strings.Contains(r.URL.Path, "workflow-templates") {
			if r.Method == "POST" {
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				template = obj(body["template"])
				json.NewEncoder(w).Encode(template)
				return
			}
			if template == nil {
				w.WriteHeader(404)
				return
			}
			json.NewEncoder(w).Encode(template)
			return
		}
		if r.Method == "POST" {
			creates++
			if run != nil {
				w.WriteHeader(409)
				return
			}
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			run = obj(body["workflow"])
			obj(run["metadata"])["uid"] = "uid-1"
			json.NewEncoder(w).Encode(run)
			return
		}
		if run == nil {
			w.WriteHeader(404)
			return
		}
		if r.Method == "PUT" {
			obj(run["spec"])["shutdown"] = "Terminate"
			run["status"] = map[string]any{"phase": "Failed"}
			json.NewEncoder(w).Encode(run)
			return
		}
		json.NewEncoder(w).Encode(run)
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	file := filepath.Join(t.TempDir(), "token")
	os.WriteFile(file, []byte("secret"), 0600)
	c, e := New(backend.Config{URL: server.URL, Namespace: "bench", TokenFile: file})
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	raw := json.RawMessage(`{"apiVersion":"argoproj.io/v1alpha1","kind":"WorkflowTemplate","spec":{"entrypoint":"main","activeDeadlineSeconds":30,"arguments":{"parameters":[{"name":"count","value":"1","description":"native input"},{"name":"mode","value":"read"}],"artifacts":[{"name":"input","raw":{"data":"payload"}}]},"templates":[{"name":"main","dag":{"tasks":[]}}]}}`)
	version, e := c.Register(ctx, raw)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Register(ctx, raw); e != nil {
		t.Fatal(e)
	}
	sub := backend.Submission{VersionID: version, Key: "stable-key", Inputs: map[string]any{"count": 2}, TimeoutSeconds: 60}
	id, e := c.Submit(ctx, sub)
	if e != nil {
		t.Fatal(e)
	}
	spec := obj(run["spec"])
	arguments := obj(spec["arguments"])
	params := arguments["parameters"].([]any)
	if len(params) != 2 || obj(params[0])["value"] != "2" || obj(params[1])["value"] != "read" || len(arguments["artifacts"].([]any)) != 1 || spec["activeDeadlineSeconds"] != float64(30) {
		t.Fatal("lost native defaults, artifacts or tighter timeout", spec)
	}
	again, e := c.Submit(ctx, sub)
	if e != nil || again != id {
		t.Fatal(again, e)
	}
	found, e := c.Lookup(ctx, sub.Key)
	if e != nil || found != id || creates != 2 {
		t.Fatal(found, e, creates)
	}
	name := obj(run["metadata"])["name"].(string)
	stage := name + "-123"
	path := "/results/" + id + "/report.json"
	run["status"] = map[string]any{"phase": "Succeeded", "nodes": map[string]any{stage: map[string]any{"id": stage, "name": name + ".read", "displayName": "read", "type": "Pod", "boundaryID": name, "phase": "Succeeded", "outputs": map[string]any{"parameters": []any{map[string]any{"name": "evidence", "value": path}}}}}}
	obs, e := c.Observe(ctx, id)
	if e != nil || obs.VersionID != version || len(obs.Stages) != 1 {
		t.Fatal(obs, e)
	}
	outputs, e := c.Outputs(ctx, id, obs.Stages[0])
	if e != nil || len(outputs) != 1 || outputs[0].Path != path {
		t.Fatal(outputs, e)
	}
	run["status"] = map[string]any{"phase": "Succeeded", "offloadNodeStatusVersion": "external-1"}
	obs, e = c.Observe(ctx, id)
	if e != nil || obs.Status != "succeeded" || obs.EvidenceError == "" {
		t.Fatal("native execution must remain observable without hydrated nodes", obs, e)
	}
	if _, e = c.Outputs(ctx, id, backend.Stage{}); e == nil {
		t.Fatal("accepted unhydrated evidence")
	}
	if done, e := c.CancellationComplete(ctx, id); e == nil || done {
		t.Fatal("confirmed cancellation without hydrated node state", done, e)
	}
	if e = c.Cancel(ctx, id); e != nil {
		t.Fatal(e)
	}
	if done, e := c.CancellationComplete(ctx, id); e != nil || !done {
		t.Fatal(done, e)
	}
	obs, e = c.Observe(ctx, id)
	if e != nil || obs.Status != "cancelled" {
		t.Fatal(obs, e)
	}
	obj(run["metadata"])["uid"] = "uid-2"
	if _, e = c.Observe(ctx, id); e == nil {
		t.Fatal("accepted replacement Workflow")
	}
	obj(template["spec"])["entrypoint"] = "tampered"
	if _, e = c.Submit(ctx, sub); e == nil {
		t.Fatal("accepted mutated template")
	}
}
