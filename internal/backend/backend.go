// Package backend is the framework boundary. Only UCE is implemented today.
package backend

import (
	"context"
	"encoding/json"
)

type Stage struct {
	EvictionCount int    `json:"eviction_count"`
	ID            string `json:"stage_run_id"`
	NodeID        string `json:"definition_node_id"`
	Attempt       int    `json:"attempt"`
	Status        string `json:"status"`
}
type Observation struct {
	RunID               string          `json:"run_id"`
	VersionID           string          `json:"workflow_version_id"`
	Status              string          `json:"status"`
	CancellationPending bool            `json:"cancellation_pending"`
	Stages              []Stage         `json:"stages"`
	Raw                 json.RawMessage `json:"raw"`
}
type Output struct {
	RunID   string `json:"run_id"`
	StageID string `json:"stage_run_id"`
	Attempt int    `json:"attempt"`
	Name    string `json:"output_name"`
	Path    string `json:"output_path"`
}
type Submission struct {
	VersionID      string
	Key            string
	Name           string
	Inputs         map[string]any
	Queue          string
	Subtype        string
	TimeoutSeconds int
}
type Backend interface {
	Register(context.Context, json.RawMessage) (string, error)
	Submit(context.Context, Submission) (string, error)
	Lookup(context.Context, string) (string, error)
	Observe(context.Context, string) (Observation, error)
	Cancel(context.Context, string) error
	CancellationComplete(context.Context, string) (bool, error)
	Outputs(context.Context, string, Stage) ([]Output, error)
}
