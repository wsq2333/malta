package uce

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/wsq2333/malta/internal/backend"
	"github.com/wsq2333/malta/internal/backend/httpapi"
	"github.com/wsq2333/malta/internal/domain"
	"net/url"
	"strconv"
	"strings"
)

type Config = backend.Config
type HTTPError = backend.HTTPError
type Client struct {
	Config Config
	*httpapi.Client
}

func New(c Config) (*Client, error) {
	h, e := httpapi.New(c)
	if e != nil {
		return nil, e
	}
	return &Client{c, h}, nil
}
func (c *Client) call(ctx context.Context, method, path, key string, timeout int, body, out any) error {
	headers := map[string]string{}
	if key != "" {
		headers["Idempotency-Key"] = key
	}
	if timeout > 0 {
		headers["timeout"] = strconv.Itoa(timeout)
	}
	return c.Call(ctx, method, path, headers, body, out)
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
	e = c.call(ctx, "GET", "/v1/workflow-runs/"+url.PathEscape(id)+"/stages", "", 0, nil, &o.StageDetails)
	if e == nil {
		e = json.Unmarshal(o.StageDetails, &o.Stages)
	}
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
