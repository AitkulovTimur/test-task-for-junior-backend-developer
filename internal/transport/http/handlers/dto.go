package handlers

import (
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
)

type taskCreateDTO struct {
	Title          string                       `json:"title"`
	Description    string                       `json:"description"`
	Status         taskdomain.Status            `json:"status"`
	ScheduledAt    *time.Time                   `json:"scheduled_at,omitempty"`
	RecurrenceType taskdomain.RecurrenceType    `json:"recurrence_type,omitempty"`
	Recurrence     *taskdomain.RecurrenceParams `json:"recurrence,omitempty"`
}

type taskUpdateDTO struct {
	Title          string                       `json:"title"`
	Description    string                       `json:"description"`
	Status         taskdomain.Status            `json:"status"`
	ScheduledAt    *time.Time                   `json:"scheduled_at"`
	ApplyToAll     bool                         `json:"apply_to_all"`
	RecurrenceType taskdomain.RecurrenceType    `json:"recurrence_type,omitempty"`
	Recurrence     *taskdomain.RecurrenceParams `json:"recurrence,omitempty"`
}

type taskDTO struct {
	ID           int64             `json:"id"`
	Title        string            `json:"title"`
	Description  string            `json:"description"`
	Status       taskdomain.Status `json:"status"`
	ScheduledAt  *time.Time        `json:"scheduled_at,omitempty"`
	ParentRuleID *int64            `json:"parent_rule_id,omitempty"`
	IsModified   bool              `json:"is_modified"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

func newTaskDTO(task *taskdomain.Task) taskDTO {
	return taskDTO{
		ID:           task.ID,
		Title:        task.Title,
		Description:  task.Description,
		Status:       task.Status,
		ScheduledAt:  task.ScheduledAt,
		ParentRuleID: task.ParentRuleID,
		IsModified:   task.IsModified,
		CreatedAt:    task.CreatedAt,
		UpdatedAt:    task.UpdatedAt,
	}
}
