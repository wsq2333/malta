package workflow

import (
	"encoding/json"
	"github.com/wsq2333/malta/internal/domain"
	"time"
)

type IssueLinkRequest struct {
	IssueID string `json:"issue_id"`
	Role    string `json:"role"`
	NodeID  string `json:"node_id,omitempty"`
	Note    string `json:"note"`
}
type IssueLink struct {
	IssueLinkRequest
	RunID         string    `json:"run_id"`
	IssueRevision int       `json:"issue_revision"`
	CreatedAt     time.Time `json:"created_at"`
}
type LinkedIssue struct {
	Link  IssueLink    `json:"link"`
	Issue domain.Issue `json:"issue"`
}

// LinkIssue preserves the issue revision at the time of association. A passing
// verification is evidence for a human decision; it never closes an issue itself.
func (m *Manager) LinkIssue(runID string, q IssueLinkRequest) (IssueLink, error) {
	var link IssueLink
	if q.Role != "discovery" && q.Role != "verification" || len(q.Note) > 2000 {
		return link, domain.E("invalid", "role must be discovery or verification; note max 2000 bytes")
	}
	r, e := m.Get(runID)
	if e != nil {
		return link, e
	}
	issue, e := m.Store.Issue(q.IssueID)
	if e != nil {
		return link, e
	}
	if q.NodeID != "" {
		found := false
		for _, rule := range r.Snapshot.Definition.Results {
			if rule.NodeID == q.NodeID {
				found = true
			}
		}
		if !found {
			return link, domain.E("invalid", "node must be a result-bearing node in this run")
		}
	}
	if q.Role == "discovery" && (issue.VersionID != "" && issue.VersionID != r.Snapshot.Version.ID || issue.EnvironmentID != "" && issue.EnvironmentID != r.Snapshot.Environment.ID) {
		return link, domain.E("invalid", "discovery must match issue version/environment")
	}
	if q.Role == "verification" {
		if r.Status != "succeeded" || r.ResultState != "complete" || r.Verdict != "passed" {
			return link, domain.E("invalid", "verification requires a completed and verified passing run")
		}
		var count int
		e = m.Store.DB.QueryRow(`SELECT count(*) FROM workflow_issue_links l JOIN workflow_runs r ON r.id=l.run_id WHERE l.issue_id=? AND l.role='discovery' AND json_extract(r.body,'$.snapshot.case.id')=? AND json_extract(r.body,'$.snapshot.environment.id')=?`, q.IssueID, r.Snapshot.Case.ID, r.Snapshot.Environment.ID).Scan(&count)
		if e != nil {
			return link, e
		}
		if count == 0 {
			return link, domain.E("invalid", "verification needs a discovery run from the same case and environment")
		}
	}
	link = IssueLink{q, r.ID, issue.Revision, time.Now().UTC()}
	raw, _ := json.Marshal(link)
	_, e = m.Store.DB.Exec(`INSERT INTO workflow_issue_links(run_id,issue_id,role,body) VALUES(?,?,?,?) ON CONFLICT(run_id,issue_id,role) DO NOTHING`, r.ID, q.IssueID, q.Role, string(raw))
	if e != nil {
		return link, e
	}
	var saved string
	if e = m.Store.DB.QueryRow(`SELECT body FROM workflow_issue_links WHERE run_id=? AND issue_id=? AND role=?`, r.ID, q.IssueID, q.Role).Scan(&saved); e != nil {
		return link, e
	}
	_ = json.Unmarshal([]byte(saved), &link)
	if domain.Hash(link.IssueLinkRequest) != domain.Hash(q) {
		return link, domain.E("conflict", "issue association already exists with different content")
	}
	return link, nil
}
func (m *Manager) IssueLinks(runID, issueID string) ([]LinkedIssue, error) {
	query := `SELECT l.body,i.body FROM workflow_issue_links l JOIN issues i ON i.issue_id=l.issue_id WHERE `
	value := runID
	if runID != "" {
		if _, e := m.Get(runID); e != nil {
			return nil, e
		}
		query += "l.run_id=?"
	} else {
		if _, e := m.Store.Issue(issueID); e != nil {
			return nil, e
		}
		query += "l.issue_id=?"
		value = issueID
	}
	rows, e := m.Store.DB.Query(query+" ORDER BY json_extract(l.body,'$.created_at'),l.run_id", value)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []LinkedIssue{}
	for rows.Next() {
		var lb, ib string
		var v LinkedIssue
		if e = rows.Scan(&lb, &ib); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(lb), &v.Link); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(ib), &v.Issue); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
