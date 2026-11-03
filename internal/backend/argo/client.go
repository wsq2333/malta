// Package argo uses the Argo Server REST API and immutable, content-addressed templates.
package argo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wsq2333/malta/internal/backend"
	"github.com/wsq2333/malta/internal/backend/httpapi"
	"github.com/wsq2333/malta/internal/domain"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const prefix = "malta.dev/"

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

type Client struct {
	Config backend.Config
	*httpapi.Client
}

func New(c backend.Config) (*Client, error) {
	if !namePattern.MatchString(c.Namespace) {
		return nil, fmt.Errorf("Argo namespace is required")
	}
	h, e := httpapi.New(c)
	if e != nil {
		return nil, e
	}
	return &Client{c, h}, nil
}

type metadata struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	UID         string            `json:"uid"`
	Annotations map[string]string `json:"annotations"`
}
type parameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type node struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Type        string `json:"type"`
	BoundaryID  string `json:"boundaryID"`
	Phase       string `json:"phase"`
	Outputs     struct {
		Parameters []parameter `json:"parameters"`
	} `json:"outputs"`
}
type workflow struct {
	Raw      json.RawMessage `json:"-"`
	Metadata metadata        `json:"metadata"`
	Spec     map[string]any  `json:"spec"`
	Status   struct {
		Phase           string          `json:"phase"`
		Nodes           map[string]node `json:"nodes"`
		CompressedNodes string          `json:"compressedNodes"`
		Offload         string          `json:"offloadNodeStatusVersion"`
	} `json:"status"`
}

func (w *workflow) UnmarshalJSON(data []byte) error {
	type plain workflow
	var p plain
	if e := json.Unmarshal(data, &p); e != nil {
		return e
	}
	*w = workflow(p)
	w.Raw = append(json.RawMessage(nil), data...)
	return nil
}
func (c *Client) path(resource string) string {
	return "/api/v1/" + resource + "/" + url.PathEscape(c.Config.Namespace)
}
func obj(v any) map[string]any                { m, _ := v.(map[string]any); return m }
func templateHash(spec map[string]any) string { return domain.Hash(spec) }
func templateName(spec map[string]any) string { return "malta-tpl-" + templateHash(spec)[:40] }
func (c *Client) Register(ctx context.Context, raw json.RawMessage) (string, error) {
	var t map[string]any
	if json.Unmarshal(raw, &t) != nil || obj(t["spec"]) == nil {
		return "", fmt.Errorf("invalid Argo template")
	}
	spec := obj(t["spec"])
	// Preserve user workflow labels/annotations as part of immutable template content.
	wm := obj(spec["workflowMetadata"])
	if wm == nil {
		wm = map[string]any{}
	}
	for _, field := range []string{"labels", "annotations"} {
		values := obj(wm[field])
		if values == nil {
			values = map[string]any{}
		}
		for k, v := range obj(obj(t["metadata"])[field]) {
			if strings.HasPrefix(k, prefix) {
				return "", fmt.Errorf("reserved Malta metadata prefix")
			}
			values[k] = v
		}
		for k := range values {
			if strings.HasPrefix(k, prefix) {
				return "", fmt.Errorf("reserved Malta metadata prefix")
			}
		}
		if len(values) > 0 {
			wm[field] = values
		}
	}
	if len(wm) > 0 {
		spec["workflowMetadata"] = wm
	}
	name := templateName(spec)
	// Never overwrite a pre-existing template, even if its name matches.
	var existing workflow
	e := c.Call(ctx, "GET", c.path("workflow-templates")+"/"+name, nil, nil, &existing)
	if e != nil {
		var he *backend.HTTPError
		if !errors.As(e, &he) || he.Status != 404 {
			return "", e
		}
		body := map[string]any{"template": map[string]any{"apiVersion": "argoproj.io/v1alpha1", "kind": "WorkflowTemplate", "metadata": map[string]any{"name": name, "namespace": c.Config.Namespace, "annotations": map[string]string{prefix + "template-hash": templateHash(spec)}}, "spec": spec}}
		e = c.Call(ctx, "POST", c.path("workflow-templates"), nil, body, &existing)
		if e != nil {
			if !errors.As(e, &he) || he.Status != 409 {
				return "", e
			}
			if e = c.Call(ctx, "GET", c.path("workflow-templates")+"/"+name, nil, nil, &existing); e != nil {
				return "", e
			}
		}
	}
	if existing.Metadata.Name != name || existing.Metadata.Namespace != c.Config.Namespace || templateHash(existing.Spec) != templateHash(spec) || existing.Metadata.Annotations[prefix+"template-hash"] != templateHash(spec) {
		return "", fmt.Errorf("Argo template identity or content differs")
	}
	return name, nil
}
func runName(key string) string { return "malta-" + domain.BytesHash([]byte(key))[:40] }
func identity(w workflow) (string, error) {
	if !namePattern.MatchString(w.Metadata.Name) || w.Metadata.UID == "" || strings.ContainsAny(w.Metadata.UID, "/.") {
		return "", fmt.Errorf("Argo omitted workflow identity")
	}
	return w.Metadata.Name + "." + w.Metadata.UID, nil
}
func (c *Client) Submit(ctx context.Context, s backend.Submission) (string, error) {
	if !namePattern.MatchString(s.VersionID) {
		return "", fmt.Errorf("invalid Argo template identity")
	}
	var t workflow
	if e := c.Call(ctx, "GET", c.path("workflow-templates")+"/"+s.VersionID, nil, nil, &t); e != nil {
		return "", e
	}
	if t.Metadata.Name != s.VersionID || t.Metadata.Namespace != c.Config.Namespace || templateName(t.Spec) != s.VersionID || t.Metadata.Annotations[prefix+"template-hash"] != templateHash(t.Spec) {
		return "", fmt.Errorf("Argo template changed before submission")
	}
	// Inline the verified native spec: a later WorkflowTemplate edit cannot alter this run.
	spec := t.Spec
	names := make([]string, 0, len(s.Inputs))
	for n := range s.Inputs {
		names = append(names, n)
	}
	sort.Strings(names)
	// Preserve native artifact inputs and parameter defaults; override supplied values only.
	arguments := obj(spec["arguments"])
	if arguments == nil {
		arguments = map[string]any{}
	}
	args, _ := arguments["parameters"].([]any)
	for _, n := range names {
		v := s.Inputs[n]
		text, ok := v.(string)
		if !ok {
			b, e := json.Marshal(v)
			if e != nil {
				return "", e
			}
			text = string(b)
		}
		replaced := false
		for i, existing := range args {
			if obj(existing)["name"] == n {
				p := obj(existing)
				delete(p, "valueFrom")
				p["value"] = text
				args[i] = p
				replaced = true
				break
			}
		}
		if !replaced {
			args = append(args, map[string]any{"name": n, "value": text})
		}
	}
	if len(args) > 0 {
		arguments["parameters"] = args
	}
	if len(arguments) > 0 {
		spec["arguments"] = arguments
	}
	deadline, _ := spec["activeDeadlineSeconds"].(float64)
	if deadline <= 0 || deadline > float64(s.TimeoutSeconds) {
		spec["activeDeadlineSeconds"] = s.TimeoutSeconds
	}
	annotations := map[string]string{prefix + "idempotency-key": s.Key, prefix + "template": s.VersionID, prefix + "submission-hash": domain.Hash(spec)}
	for k, v := range obj(obj(spec["workflowMetadata"])["annotations"]) {
		if !strings.HasPrefix(k, prefix) {
			if text, ok := v.(string); ok {
				annotations[k] = text
			}
		}
	}
	wf := map[string]any{"apiVersion": "argoproj.io/v1alpha1", "kind": "Workflow", "metadata": map[string]any{"name": runName(s.Key), "namespace": c.Config.Namespace, "annotations": annotations, "labels": obj(obj(spec["workflowMetadata"])["labels"])}, "spec": spec}
	var w workflow
	e := c.Call(ctx, "POST", c.path("workflows"), nil, map[string]any{"workflow": wf}, &w)
	if e != nil {
		var he *backend.HTTPError
		if !errors.As(e, &he) || he.Status != 409 {
			return "", e
		}
		if e = c.Call(ctx, "GET", c.path("workflows")+"/"+runName(s.Key), nil, nil, &w); e != nil {
			return "", e
		}
	}
	if w.Metadata.Namespace != c.Config.Namespace || w.Metadata.Name != runName(s.Key) || w.Metadata.Annotations[prefix+"idempotency-key"] != s.Key || w.Metadata.Annotations[prefix+"template"] != s.VersionID || w.Metadata.Annotations[prefix+"submission-hash"] != domain.Hash(spec) {
		return "", fmt.Errorf("Argo idempotency conflict")
	}
	return identity(w)
}
func (c *Client) Lookup(ctx context.Context, key string) (string, error) {
	var w workflow
	if e := c.Call(ctx, "GET", c.path("workflows")+"/"+runName(key), nil, nil, &w); e != nil {
		return "", e
	}
	if w.Metadata.Name != runName(key) || w.Metadata.Namespace != c.Config.Namespace || w.Metadata.Annotations[prefix+"idempotency-key"] != key {
		return "", fmt.Errorf("Argo idempotency identity mismatch")
	}
	return identity(w)
}
func (c *Client) get(ctx context.Context, id string) (workflow, error) {
	var w workflow
	parts := strings.Split(id, ".")
	if len(parts) != 2 || !namePattern.MatchString(parts[0]) || parts[1] == "" {
		return w, fmt.Errorf("invalid Argo run identity")
	}
	if e := c.Call(ctx, "GET", c.path("workflows")+"/"+parts[0], nil, nil, &w); e != nil {
		return w, e
	}
	actual, e := identity(w)
	if e != nil || actual != id || w.Metadata.Namespace != c.Config.Namespace {
		return w, fmt.Errorf("Argo workflow was replaced or identity changed")
	}
	return w, nil
}
func phase(s string) string {
	switch s {
	case "Succeeded":
		return "succeeded"
	case "Failed", "Error":
		return "failed"
	case "Skipped", "Omitted":
		return "cancelled"
	default:
		return strings.ToLower(s)
	}
}
func stages(w workflow) ([]backend.Stage, error) {
	if len(w.Status.Nodes) == 0 && (w.Status.CompressedNodes != "" || w.Status.Offload != "") {
		return nil, fmt.Errorf("Argo node status unavailable; configure hydrated API responses")
	}
	out := []backend.Stage{}
	for id, n := range w.Status.Nodes {
		if n.Type != "Pod" {
			continue
		}
		if n.ID != id {
			return nil, fmt.Errorf("Argo node identity mismatch")
		}
		logical := strings.TrimPrefix(n.Name, w.Metadata.Name+".")
		if n.Name == w.Metadata.Name {
			logical, _ = w.Spec["entrypoint"].(string)
		} else if logical == n.Name {
			logical = n.DisplayName
		}
		attempt := 0
		if i := strings.LastIndex(logical, "("); i >= 0 && strings.HasSuffix(logical, ")") {
			if a, e := strconv.Atoi(logical[i+1 : len(logical)-1]); e == nil {
				attempt = a
				logical = logical[:i]
			}
		}
		out = append(out, backend.Stage{ID: n.ID, NodeID: logical, Attempt: attempt, Status: phase(n.Phase)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (c *Client) Observe(ctx context.Context, id string) (backend.Observation, error) {
	w, e := c.get(ctx, id)
	if e != nil {
		return backend.Observation{}, e
	}
	ss, stageErr := stages(w)
	evidenceError := ""
	if stageErr != nil {
		evidenceError = stageErr.Error()
	}
	state := phase(w.Status.Phase)
	if w.Spec["shutdown"] == "Terminate" && (state == "succeeded" || state == "failed") {
		state = "cancelled"
	}
	raw := w.Raw
	return backend.Observation{EvidenceError: evidenceError, RunID: id, VersionID: w.Metadata.Annotations[prefix+"template"], Status: state, Stages: ss, Raw: raw}, nil
}
func (c *Client) Cancel(ctx context.Context, id string) error {
	w, e := c.get(ctx, id)
	if e != nil {
		return e
	}
	return c.Call(ctx, "PUT", c.path("workflows")+"/"+w.Metadata.Name+"/terminate", nil, map[string]string{"namespace": c.Config.Namespace, "name": w.Metadata.Name}, nil)
}
func (c *Client) CancellationComplete(ctx context.Context, id string) (bool, error) {
	w, e := c.get(ctx, id)
	if e != nil {
		return false, e
	}
	if w.Status.Phase != "Succeeded" && w.Status.Phase != "Failed" && w.Status.Phase != "Error" {
		return false, nil
	}
	if _, e := stages(w); e != nil {
		return false, e
	}
	for _, n := range w.Status.Nodes {
		if n.Type == "Pod" && (n.Phase == "Pending" || n.Phase == "Running") {
			return false, nil
		}
	}
	return true, nil
}
func (c *Client) Outputs(ctx context.Context, id string, s backend.Stage) ([]backend.Output, error) {
	w, e := c.get(ctx, id)
	if e != nil {
		return nil, e
	}
	ss, e := stages(w)
	if e != nil {
		return nil, e
	}
	found := false
	for _, n := range ss {
		if n == s {
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("Argo stage changed")
	}
	out := []backend.Output{}
	for _, p := range w.Status.Nodes[s.ID].Outputs.Parameters {
		out = append(out, backend.Output{RunID: id, StageID: s.ID, Attempt: s.Attempt, Name: p.Name, Path: p.Value})
	}
	return out, nil
}
