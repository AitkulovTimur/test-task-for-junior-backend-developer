package task

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
)

type Service struct {
	repo           Repository
	generator      Generator
	planningCounts map[taskdomain.RecurrenceType]int
	now            func() time.Time
}

func NewService(repo Repository, gen Generator, planningCounts map[taskdomain.RecurrenceType]int) *Service {
	return &Service{
		repo:           repo,
		generator:      gen,
		planningCounts: planningCounts,
		now:            func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*taskdomain.Task, error) {
	normalized, err := validateCreateInput(input)
	if err != nil {
		return nil, err
	}

	if input.Recurrence != nil {
		// 1. Validate recurrence
		if err := input.Recurrence.ValidateFieldsFilling(input.RecurrenceType); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}

		// 2. Generate dates for recurrence
		start := s.now()
		if input.ScheduledAt != nil {
			start = *input.ScheduledAt
		}

		dates, err := s.generator.GenerateDates(start, &input, s.planningCounts[input.RecurrenceType])
		if err != nil {
			return nil, fmt.Errorf("failed to generate dates: %w", err)
		}

		// 3. Collect tasks
		tasks := make([]taskdomain.Task, len(dates))
		now := s.now()
		for i, d := range dates {

			tasks[i] = taskdomain.Task{
				Title:       normalized.Title,
				Description: normalized.Description,
				//TODO: добавить в README: полагаю, что в иных статусах может быть только первая задача. Другие не могут быть, так как они в будущем
				Status:      taskdomain.StatusNew,
				ScheduledAt: &d,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
		}

		//TODO:Only the first task can have his own created status (README)
		tasks[0].Status = normalized.Status

		// 4. Serialization for DB JSONB
		paramsJSON, err := json.Marshal(input.Recurrence)
		if err != nil {
			return nil, fmt.Errorf("marshal recurrence params: %w", err)
		}

		rule := &taskdomain.RecurrenceRule{
			Type:        input.RecurrenceType,
			Params:      paramsJSON,
			ScheduledAt: input.ScheduledAt,
			CreatedAt:   s.now(),
		}

		return s.repo.CreateSeries(ctx, rule, tasks)
	}

	model := &taskdomain.Task{
		Title:       normalized.Title,
		Description: normalized.Description,
		Status:      normalized.Status,
		ScheduledAt: normalized.ScheduledAt,
	}
	now := s.now()
	model.CreatedAt = now
	model.UpdatedAt = now

	created, err := s.repo.Create(ctx, model)
	if err != nil {
		return nil, err
	}

	return created, nil
}

func (s *Service) GetByID(ctx context.Context, id int64) (*taskdomain.Task, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}

	return s.repo.GetByID(ctx, id)
}

func (s *Service) Update(ctx context.Context, id int64, input UpdateInput) (*taskdomain.Task, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}

	normalized, err := validateUpdateInput(input)
	if err != nil {
		return nil, err
	}

	// Get current task to check if it's part of a series
	currentTask, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Case 1: Single update (ApplyToAll == false)
	if !normalized.ApplyToAll {
		return s.updateSingleTask(ctx, id, &normalized)
	}

	// Case 2 & 3: Apply to all - need to check if recurrence parameters changed
	if currentTask.ParentRuleID == nil {
		// Not a series task, treat as single update
		return s.updateSingleTask(ctx, id, &normalized)
	}

	// Check if recurrence parameters changed
	recurrenceChanged, err := RecurrenceChanged(ctx, s.repo, currentTask, &normalized)
	if err != nil {
		return nil, fmt.Errorf("failed to check recurrence changes: %w", err)
	}

	if !recurrenceChanged {
		// Case 2: Bulk content update - recurrence didn't change
		// Update series content
		updatedCurrentTask, err := s.repo.UpdateSeriesTaskAndCurrent(ctx, currentTask.ID, *currentTask.ParentRuleID, &normalized)
		if err != nil {
			return nil, fmt.Errorf("failed to update series content: %w", err)
		}

		return updatedCurrentTask, nil
	}

	// Case 3: Re-scheduling - recurrence parameters changed
	return s.rescheduleSeries(ctx, id, currentTask.ParentRuleID, &normalized)
}

func (s *Service) updateSingleTask(ctx context.Context, id int64, input *UpdateInput) (*taskdomain.Task, error) {
	model := &taskdomain.Task{
		ID:          id,
		Title:       input.Title,
		Description: input.Description,
		Status:      input.Status,
		ScheduledAt: input.ScheduledAt,
		UpdatedAt:   s.now(),
	}
	return s.repo.Update(ctx, model)
}

func (s *Service) rescheduleSeries(ctx context.Context, taskID int64, ruleID *int64, input *UpdateInput) (*taskdomain.Task, error) {

	if input.Recurrence == nil {
		return nil, fmt.Errorf("%w: recurrence is required while rescheduling series", ErrInvalidInput)
	}

	paramsJSON, err := json.Marshal(input.Recurrence)
	if err != nil {
		return nil, fmt.Errorf("marshal recurrence params: %w", err)
	}

	start := s.now()
	if input.ScheduledAt != nil {
		start = *input.ScheduledAt
	}

	createInput := CreateInput{
		Title:          input.Title,
		Description:    input.Description,
		Status:         input.Status,
		ScheduledAt:    input.ScheduledAt,
		RecurrenceType: input.RecurrenceType,
		Recurrence:     input.Recurrence,
	}

	dates, err := s.generator.GenerateDates(start, &createInput, s.planningCounts[input.RecurrenceType])
	if err != nil {
		return nil, fmt.Errorf("failed to generate dates: %w", err)
	}

	tasks := make([]taskdomain.Task, len(dates))
	for i, d := range dates {
		tasks[i] = taskdomain.Task{
			Title:        input.Title,
			Description:  input.Description,
			Status:       taskdomain.StatusNew,
			ScheduledAt:  &d,
			ParentRuleID: ruleID,
			CreatedAt:    s.now(),
			UpdatedAt:    s.now(),
		}
	}

	err = s.repo.RescheduleSeriesTx(ctx, *ruleID, taskID, input.RecurrenceType, paramsJSON, input.ScheduledAt, tasks)
	if err != nil {
		return nil, fmt.Errorf("failed to reschedule in db: %w", err)
	}

	return s.updateSingleTask(ctx, taskID, input)
}

func (s *Service) Delete(ctx context.Context, id int64, mode taskdomain.DeleteMode, deleteModified bool) error {
	if id <= 0 {
		return fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}

	if !mode.Valid() {
		return fmt.Errorf("%w: invalid delete mode", ErrInvalidInput)
	}

	// Get current task to check if it's part of a series
	currentTask, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	switch mode {
	case taskdomain.DeleteModeSingle:
		return s.repo.Delete(ctx, id)

	case taskdomain.DeleteModeFuture:
		if currentTask.ParentRuleID == nil {
			// Not a series task, treat as single deletion
			return s.repo.Delete(ctx, id)
		}
		return s.repo.DeleteFutureTasksTx(ctx, *currentTask.ParentRuleID, id, currentTask.ScheduledAt)

	case taskdomain.DeleteModeEntireSeries:
		if currentTask.ParentRuleID == nil {
			// Not a series task, treat as single deletion
			return s.repo.Delete(ctx, id)
		}
		return s.repo.DeleteEntireSeriesTx(ctx, *currentTask.ParentRuleID, deleteModified)

	default:
		return fmt.Errorf("%w: unsupported delete mode", ErrInvalidInput)
	}
}

func (s *Service) List(ctx context.Context) ([]taskdomain.Task, error) {
	return s.repo.List(ctx)
}

func validateCreateInput(input CreateInput) (CreateInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)

	if input.Title == "" {
		return CreateInput{}, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}

	if input.Status == "" {
		input.Status = taskdomain.StatusNew
	}

	if !input.Status.Valid() {
		return CreateInput{}, fmt.Errorf("%w: invalid status", ErrInvalidInput)
	}

	//TODO: добавить в README, это мое решение, так как было на мокапе.
	//Возможно подразумевалась функциональность без ScheduledAt
	if input.ScheduledAt != nil {
		if input.ScheduledAt.UTC().Before(time.Now().UTC()) {
			return CreateInput{}, fmt.Errorf("%w: scheduled date cannot be in the past", ErrInvalidInput)
		}
	}

	hasRecurrence := input.Recurrence != nil
	hasType := input.RecurrenceType != ""

	if hasRecurrence != hasType {
		return CreateInput{},
			fmt.Errorf(
				"%w: recurrence info must be complete (both fields (recurrence_type + recurrence) must be provided)",
				ErrInvalidInput)
	}

	return input, nil
}

func validateUpdateInput(input UpdateInput) (UpdateInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)

	if input.Title == "" {
		return UpdateInput{}, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}

	//TODO: добавить в README: предполагается, что если потерялся статус, то он становится new
	if input.Status == "" {
		input.Status = taskdomain.StatusNew
	}

	if !input.Status.Valid() {
		return UpdateInput{}, fmt.Errorf("%w: invalid status", ErrInvalidInput)
	}

	// Validate recurrence if provided
	if input.Recurrence != nil {
		if err := input.Recurrence.ValidateFieldsFilling(input.RecurrenceType); err != nil {
			return UpdateInput{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
	}

	// Validate scheduled date
	if input.ScheduledAt != nil {
		if input.ScheduledAt.UTC().Before(time.Now().UTC()) {
			return UpdateInput{}, fmt.Errorf("%w: scheduled date cannot be in the past", ErrInvalidInput)
		}
	}

	return input, nil
}
