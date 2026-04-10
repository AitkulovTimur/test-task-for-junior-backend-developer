package task

import (
	"context"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
)

type Repository interface {
	Create(ctx context.Context, task *taskdomain.Task) (*taskdomain.Task, error)
	GetByID(ctx context.Context, id int64) (*taskdomain.Task, error)
	GetRuleByID(ctx context.Context, id int64) (*taskdomain.RecurrenceRule, error)
	Update(ctx context.Context, task *taskdomain.Task) (*taskdomain.Task, error)
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context) ([]taskdomain.Task, error)
	CreateSeries(ctx context.Context, rule *taskdomain.RecurrenceRule, tasks []taskdomain.Task) (*taskdomain.Task, error)
	UpdateSeriesTaskAndCurrent(ctx context.Context, taskId int64, ruleID int64, input *UpdateInput) (*taskdomain.Task, error)

	RescheduleSeriesTx(
		ctx context.Context,
		ruleID int64,
		currentTaskID int64,
		ruleType taskdomain.RecurrenceType,
		params []byte,
		scheduledAt *time.Time,
		newTasks []taskdomain.Task,
	) error
}

type Generator interface {
	GenerateDates(startFrom time.Time, input *CreateInput, countOfDatesToGen int) ([]time.Time, error)
}

type Usecase interface {
	Create(ctx context.Context, input CreateInput) (*taskdomain.Task, error)
	GetByID(ctx context.Context, id int64) (*taskdomain.Task, error)
	Update(ctx context.Context, id int64, input UpdateInput) (*taskdomain.Task, error)
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context) ([]taskdomain.Task, error)
}

type CreateInput struct {
	Title       string
	Description string
	Status      taskdomain.Status

	ScheduledAt    *time.Time
	RecurrenceType taskdomain.RecurrenceType
	Recurrence     *taskdomain.RecurrenceParams
}

type UpdateInput struct {
	Title       string
	Description string
	Status      taskdomain.Status
	ScheduledAt *time.Time
	ApplyToAll  bool

	RecurrenceType taskdomain.RecurrenceType
	Recurrence     *taskdomain.RecurrenceParams
}
