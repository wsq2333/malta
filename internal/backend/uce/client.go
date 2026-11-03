package uce

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/wsq2333/malta/internal/backend"
	"github.com/wsq2333/malta/internal/domain"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	TokenFile string `json:"token_file"`
	// Roots belong to trusted server configuration, never user-submitted profiles.
	RemoteResultsRoot string   `json:"remote_results_root"`
	LocalResultsRoot  string   `json:"local_results_root"`
	MaxBytes          int64    `json:"max_bytes"`
	TimeoutSeconds    int      `json:"timeout_seconds"`
	ArtifactBytes     int64    `json:"artifact_bytes"`
	Cluster           string   `json:"cluster"`
	Namespace         string   `json:"namespace"`
	Subtypes          []string `json:"subtypes"`
}
type Client struct {
	Config Config
	HTTP   *http.Client
}
type HTTPError struct {
	Status int
	Code   string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("UCE HTTP %d (%s)", e.Status, e.Code) }
func New(c Config) (*Client, error) {
	u, e := url.Parse(c.URL)
	if e != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return nil, domain.E("invalid", "UCE URL must be an origin without credentials")
	}
	if c.TokenFile == "" {
		return nil, domain.E("invalid", "UCE token_file is required")
	}
	return &Client{c, &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) call(ctx context.Context, method, path, key string, timeout int, body any, out any) error {
	raw, e := json.Marshal(body)
	if e != nil {
		return e
	}
	req, e := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.Config.URL, "/")+path, bytes.NewReader(raw))
	if e != nil {
		return e
	}
	token, e := os.ReadFile(c.Config.TokenFile)
	if e != nil {
		return fmt.Errorf("UCE credential file unavailable")
	}
	if strings.TrimSpace(string(token)) == "" {
		return fmt.Errorf("UCE credential file is empty")
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	if timeout > 0 {
		req.Header.Set("timeout", strconv.Itoa(timeout))
	}
	resp, e := c.HTTP.Do(req)
	if e != nil {
		return fmt.Errorf("UCE request failed: %T", e)
	}
	defer resp.Body.Close()
	data, e := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if e != nil {
		return fmt.Errorf("UCE response read failed")
	}
	if len(data) > 8<<20 {
		return fmt.Errorf("UCE response exceeds limit")
	}
	if resp.StatusCode >= 300 {
		var x struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(data, &x)
		if len(x.Code) > 100 {
			x.Code = "remote_error"
		}
		return &HTTPError{resp.StatusCode, x.Code}
	}
	if out != nil {
		if e = json.Unmarshal(data, out); e != nil {
			return fmt.Errorf("invalid UCE response")
		}
	}
	return nil
}
func (c *Client) Register(ctx context.Context, ep json.RawMessage) (string, error) {
	var v struct {
		ID string `json:"workflow_version_id"`
	}
	e := c.call(ctx, "POST", "/v1/workflow-versions", "", 0, ep, &v)
	if e == nil && domain.CheckID(v.ID) != nil {
		e = fmt.Errorf("UCE registration omitted version identity")
	}
	return v.ID, e
}
func (c *Client) Submit(ctx context.Context, s backend.Submission) (string, error) {
	inputs := map[string]any{}
	for k, v := range s.Inputs {
		inputs[k] = map[string]any{"value": v}
	}
	body := map[string]any{"workflow_version_id": s.VersionID, "workflow_name": s.Name, "inputs": inputs, "execution_backend": "yarlung", "queue": s.Queue, "subtype": s.Subtype}
	var v struct {
		ID string `json:"run_id"`
	}
	e := c.call(ctx, "POST", "/v2/workflow-runs", s.Key, s.TimeoutSeconds, body, &v)
	if e == nil && domain.CheckID(v.ID) != nil {
		e = fmt.Errorf("UCE submission omitted run identity")
	}
	return v.ID, e
}
func (c *Client) Lookup(ctx context.Context, key string) (string, error) {
	var v struct {
		ID string `json:"run_id"`
	}
	e := c.call(ctx, "GET", "/v1/workflow-runs/by-idempotency-key/"+url.PathEscape(key), "", 0, nil, &v)
	if e == nil && domain.CheckID(v.ID) != nil {
		e = fmt.Errorf("UCE lookup omitted run identity")
	}
	return v.ID, e
}
func (c *Client) Observe(ctx context.Context, id string) (backend.Observation, error) {
	var o backend.Observation
	var raw json.RawMessage
	e := c.call(ctx, "GET", "/v1/workflow-runs/"+url.PathEscape(id), "", 0, nil, &raw)
	if e != nil {
		return o, e
	}
	if e = json.Unmarshal(raw, &o); e != nil {
		return o, e
	}
	o.Raw = raw
	e = c.call(ctx, "GET", "/v1/workflow-runs/"+url.PathEscape(id)+"/stages", "", 0, nil, &o.Stages)
	return o, e
}
func (c *Client) Cancel(ctx context.Context, id string) error {
	return c.call(ctx, "POST", "/v1/workflow-runs/"+url.PathEscape(id)+":cancel", "", 0, nil, nil)
}
func (c *Client) CancellationComplete(ctx context.Context, id string) (bool, error) {
	cursor := ""
	for page := 0; page < 20; page++ {
		var v struct {
			Tasks []struct {
				Status string `json:"status"`
			} `json:"tasks"`
			Next string `json:"next_cursor"`
		}
		p := "/v1/workflow-runs/" + url.PathEscape(id) + "/cancellations"
		if cursor != "" {
			p += "?after=" + url.QueryEscape(cursor)
		}
		if e := c.call(ctx, "GET", p, "", 0, nil, &v); e != nil {
			return false, e
		}
		for _, t := range v.Tasks {
			if strings.ToLower(t.Status) != "succeeded" {
				return false, nil
			}
		}
		if v.Next == "" {
			return true, nil
		}
		cursor = v.Next
	}
	return false, fmt.Errorf("cancellation pagination exceeds bound")
}
func (c *Client) Outputs(ctx context.Context, id string, s backend.Stage) ([]backend.Output, error) {
	var v struct {
		State   string           `json:"registration_status"`
		Outputs []backend.Output `json:"outputs"`
	}
	e := c.call(ctx, "GET", fmt.Sprintf("/v1/workflow-runs/%s/stages/%s/attempts/%d/outputs", url.PathEscape(id), url.PathEscape(s.ID), s.Attempt), "", 0, nil, &v)
	if e == nil && v.State != "registered" {
		e = fmt.Errorf("stage output registration pending")
	}
	return v.Outputs, e
}
