// Package workflow manages native workflow definitions and verified test evidence.
// Execution scheduling belongs to the backend, never to this package.
package workflow

import (
	"encoding/json"
	"github.com/wsq2333/malta/internal/domain"
	"time"
)

const Schema = "workflow/v1alpha1"

type Ref struct {
	ID       string `json:"id"`
	Revision int    `json:"revision"`
}
type Record struct {
	ID          string          `json:"id"`
	Revision    int             `json:"revision"`
	Kind        string          `json:"kind"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	ChangeNote  string          `json:"change_note"`
	CreatedAt   time.Time       `json:"created_at"`
	Body        json.RawMessage `json:"body"`
}
type SaveRecord struct {
	ID           string          `json:"id,omitempty"`
	BaseRevision int             `json:"base_revision"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	ChangeNote   string          `json:"change_note"`
	Body         json.RawMessage `json:"body"`
}
type Implementation struct {
	Template json.RawMessage `json:"template,omitempty"`
	Bindings []Binding       `json:"bindings"`
	Results  []ResultRule    `json:"results"`
}
type Definition struct {
	Implementations map[string]Implementation `json:"implementations,omitempty"`
	SchemaVersion   string                    `json:"schema_version"`
	Backend         string                    `json:"backend"`
	Template        json.RawMessage           `json:"template,omitempty"`
	Parameters      json.RawMessage           `json:"parameters,omitempty"`
	Defaults        map[string]any            `json:"defaults"`
	Bindings        []Binding                 `json:"bindings"`
	Results         []ResultRule              `json:"results"`
	// Dataset budget counts the unique prepared dataset, not each consuming stage.
	Dataset        Dataset `json:"dataset"`
	TimeoutSeconds int     `json:"timeout_seconds"`
}
type Dataset struct {
	FileSizeParameter  string `json:"file_size_parameter"`
	FileCountParameter string `json:"file_count_parameter"`
}

// Binding replaces an existing JSON pointer in the EP; no string interpolation.
// Values come from versioned system/profile settings, never from arbitrary patches.
type Binding struct {
	Pointer string `json:"pointer"`
	Source  string `json:"source"`
	Key     string `json:"key"`
}
type ResultRule struct {
	NodeID           string            `json:"node_id"`
	Output           string            `json:"output"`
	Parser           string            `json:"parser"`
	Required         bool              `json:"required"`
	Assertions       []string          `json:"assertions"`
	ExpectedOptions  map[string]string `json:"expected_options,omitempty"`
	OptionParameters map[string]string `json:"option_parameters,omitempty"`
}
type System struct {
	EnvironmentID     string         `json:"environment_id"`
	SoftwareVersionID string         `json:"software_version_id"`
	Settings          map[string]any `json:"settings"`
}
type Profile struct {
	Backend    string         `json:"backend"`
	Connection string         `json:"connection"`
	SystemIDs  []string       `json:"system_ids"`
	Cluster    string         `json:"cluster"`
	Namespace  string         `json:"namespace"`
	Node       string         `json:"node"`
	Queue      string         `json:"queue"`
	Subtype    string         `json:"subtype"`
	Settings   map[string]any `json:"settings"`
}
type Selection struct {
	Case       Ref            `json:"case"`
	System     Ref            `json:"system"`
	Profile    Ref            `json:"profile"`
	VersionID  string         `json:"version_id"`
	Parameters map[string]any `json:"parameters,omitempty"`
}
type Snapshot struct {
	Selection      Selection              `json:"selection"`
	Case           Record                 `json:"case"`
	System         Record                 `json:"system"`
	Profile        Record                 `json:"profile"`
	Environment    domain.Environment     `json:"environment"`
	Version        domain.SoftwareVersion `json:"version"`
	Definition     Definition             `json:"definition"`
	EP             json.RawMessage        `json:"ep"`
	Inputs         map[string]any         `json:"inputs"`
	TemplateHash   string                 `json:"template_hash"`
	EPHash         string                 `json:"ep_hash"`
	Hash           string                 `json:"hash"`
	PreparedBytes  int64                  `json:"prepared_bytes"`
	ConnectionHash string                 `json:"connection_hash"`
}
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}
type Preflight struct {
	ID        string    `json:"id"`
	ExpiresAt time.Time `json:"expires_at"`
	Ready     bool      `json:"ready"`
	Checks    []Check   `json:"checks"`
	Snapshot  Snapshot  `json:"snapshot"`
}
type Start struct {
	PreflightID  string `json:"preflight_id"`
	SnapshotHash string `json:"snapshot_hash"`
	RequestID    string `json:"request_id"`
}
type Step struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}
type Run struct {
	SchemaVersion      string          `json:"schema_version"`
	ID                 string          `json:"id"`
	RequestID          string          `json:"request_id"`
	Key                string          `json:"idempotency_key"`
	Snapshot           Snapshot        `json:"snapshot"`
	ExecutionState     string          `json:"execution_state"`
	Status             string          `json:"status"`
	ResultState        string          `json:"result_state"`
	Verdict            string          `json:"verdict"`
	ExternalVersionID  string          `json:"external_version_id"`
	ExternalRunID      string          `json:"external_run_id"`
	CreatedAt          time.Time       `json:"created_at"`
	Deadline           time.Time       `json:"deadline"`
	UpdatedAt          time.Time       `json:"updated_at"`
	CancelRequested    bool            `json:"cancel_requested"`
	SubmissionStarted  bool            `json:"submission_started"`
	CancelConfirmed    bool            `json:"cancel_confirmed"`
	Steps              []Step          `json:"steps"`
	Observations       json.RawMessage `json:"observations,omitempty"`
	Results            []Result        `json:"results"`
	Error              string          `json:"error,omitempty"`
	CollectionDeadline *time.Time      `json:"collection_deadline,omitempty"`
}
type Evidence struct {
	EvictionCount     int             `json:"eviction_count"`
	SnapshotHash      string          `json:"snapshot_hash"`
	SchemaVersion     string          `json:"schema_version"`
	UCEVersionID      string          `json:"workflow_version_id"`
	UCERunID          string          `json:"run_id"`
	NodeID            string          `json:"node_id"`
	StageID           string          `json:"stage_run_id"`
	Attempt           int             `json:"attempt"`
	EnvironmentID     string          `json:"environment_id"`
	SoftwareVersionID string          `json:"software_version_id"`
	Parser            string          `json:"parser"`
	Assertions        map[string]bool `json:"assertions"`
	Files             []EvidenceFile  `json:"files"`
	Metrics           []domain.Metric `json:"metrics,omitempty"`
}
type EvidenceFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type Result struct {
	NodeID     string          `json:"node_id"`
	StageID    string          `json:"stage_run_id"`
	Attempt    int             `json:"attempt"`
	Parser     string          `json:"parser"`
	State      string          `json:"state"`
	Verdict    string          `json:"verdict"`
	Metrics    []domain.Metric `json:"metrics"`
	Assertions map[string]bool `json:"assertions"`
	Files      []EvidenceFile  `json:"files"`
	Error      string          `json:"error,omitempty"`
}
