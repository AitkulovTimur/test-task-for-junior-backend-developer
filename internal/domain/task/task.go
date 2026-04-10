package task

import "time"

type Status string

const (
	StatusNew        Status = "new"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
)

type DeleteMode string

const (
	DeleteModeSingle       DeleteMode = "single"
	DeleteModeFuture       DeleteMode = "future"
	DeleteModeEntireSeries DeleteMode = "entire_series"
)

func (d DeleteMode) Valid() bool {
	switch d {
	case DeleteModeSingle, DeleteModeFuture, DeleteModeEntireSeries:
		return true
	default:
		return false
	}
}

type Task struct {
	ID           int64      `json:"id"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Status       Status     `json:"status"`
	ScheduledAt  *time.Time `json:"scheduled_at,omitempty"`
	ParentRuleID *int64     `json:"parent_rule_id,omitempty"`
	IsModified   bool       `json:"is_modified"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (s Status) Valid() bool {
	switch s {
	case StatusNew, StatusInProgress, StatusDone:
		return true
	default:
		return false
	}
}
