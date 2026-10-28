package domain

import (
	"strings"
	"time"
)

// IssueFields describes evidence supplied by a user or agent. Registry links are optional.
type IssueFields struct {
	Title         string    `json:"title"`
	Type          string    `json:"type"`
	Description   string    `json:"description"`
	Software      string    `json:"software"`
	Hardware      string    `json:"hardware"`
	VersionID     string    `json:"version_id,omitempty"`
	EnvironmentID string    `json:"environment_id,omitempty"`
	RunID         string    `json:"run_id,omitempty"`
	OccurredAt    time.Time `json:"occurred_at"`
	Resolved      bool      `json:"resolved"`
	Resolution    string    `json:"resolution"`
	Reporter      string    `json:"reporter"`
	ChangeNote    string    `json:"change_note"`
}

type SaveIssue struct {
	IssueID      string `json:"issue_id"`
	BaseRevision int    `json:"base_revision"`
	IssueFields
}

// Issue is an immutable revision. CreatedAt is preserved across all revisions.
type Issue struct {
	IssueID  string `json:"issue_id"`
	Revision int    `json:"revision"`
	IssueFields
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	ResolvedAt *time.Time `json:"resolved_at" jsonschema:"nullable"`
}

func (q SaveIssue) Validate() error {
	if q.IssueID != "" {
		if e := CheckID(q.IssueID); e != nil {
			return e
		}
	}
	if q.BaseRevision < 0 || (q.BaseRevision > 0 && q.IssueID == "") {
		return E("invalid", "updates require issue_id and a nonnegative base_revision")
	}
	for _, f := range []struct {
		name, value string
		max         int
		required    bool
	}{
		{"title", q.Title, 200, true}, {"type", q.Type, 100, true},
		{"description", q.Description, 20000, true}, {"software", q.Software, 2000, true},
		{"hardware", q.Hardware, 2000, true}, {"resolution", q.Resolution, 20000, false},
		{"reporter", q.Reporter, 200, true}, {"change_note", q.ChangeNote, 2000, false},
	} {
		if len(f.value) > f.max || (f.required && strings.TrimSpace(f.value) == "") {
			return E("invalid", f.name+" is required or exceeds its byte limit")
		}
	}
	if q.Type != strings.TrimSpace(q.Type) {
		return E("invalid", "type must not have leading or trailing whitespace")
	}
	if q.OccurredAt.IsZero() {
		return E("invalid", "occurred_at must be an RFC3339 timestamp")
	}
	if q.Resolved && strings.TrimSpace(q.Resolution) == "" {
		return E("invalid", "resolved issues require a resolution")
	}
	if !q.Resolved && q.Resolution != "" {
		return E("invalid", "unresolved issues must have an empty resolution; record investigation in description/change_note")
	}
	for _, id := range []string{q.VersionID, q.EnvironmentID, q.RunID} {
		if id != "" {
			if e := CheckID(id); e != nil {
				return e
			}
		}
	}
	return nil
}
