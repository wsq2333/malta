package domain

import "time"

// CaseVersion is an immutable, runnable plan snapshot. Revisions start at one.
type CaseVersion struct {
	CaseID      string    `json:"case_id"`
	Revision    int       `json:"revision"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	ChangeNote  string    `json:"change_note"`
	CreatedAt   time.Time `json:"created_at"`
	Plan        Plan      `json:"plan"`
}
type Case struct {
	ID        string      `json:"id"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
	Latest    CaseVersion `json:"latest"`
}
type SaveCase struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	ChangeNote   string `json:"change_note"`
	BaseRevision int    `json:"base_revision"`
	Plan         Plan   `json:"plan"`
}
type CaseRef struct {
	ID       string `json:"id"`
	Revision int    `json:"revision"`
}
