package task

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	log.Printf("Service: creating task with title '%s'", input.Title)
	normalized, err := validateCreateInput(input)
	if err != nil {
		log.Printf("Service: validation failed for task creation: %v", err)
		return nil, err
	}

	if input.Recurrence != nil {
		// Validate recurrence configuration
		if err := input.Recurrence.ValidateFieldsFilling(input.RecurrenceType); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}

		// Generate recurrence dates
		start := s.now()
		if input.ScheduledAt != nil {
			start = *input.ScheduledAt
		}

		dates, err := s.generator.GenerateDates(start, &input, s.planningCounts[input.RecurrenceType])
		if err != nil {
			return nil, fmt.Errorf("failed to generate dates: %w", err)
		}

		// Build task instances
		tasks := make([]taskdomain.Task, len(dates))
		now := s.now()
		for i, d := range dates {

			tasks[i] = taskdomain.Task{
				Title:       normalized.Title,
				Description: normalized.Description,
				Status:      taskdomain.StatusNew,
				ScheduledAt: &d,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
		}

		// First task inherits the requested status, others remain new
		tasks[0].Status = normalized.Status

		// Serialize recurrence parameters
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

		log.Printf("Service: creating task series with %d tasks", len(tasks))
		return s.repo.CreateSeries(ctx, rule, tasks)
	}

	log.Printf("Service: creating single task")
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
		log.Printf("Service: failed to create task: %v", err)
		return nil, err
	}

	log.Printf("Service: successfully created task with ID %d", created.ID)
	return created, nil
}

func (s *Service) GetByID(ctx context.Context, id int64) (*taskdomain.Task, error) {
	log.Printf("Service: getting task by ID %d", id)
	if id <= 0 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}

	return s.repo.GetByID(ctx, id)
}

func (s *Service) Update(ctx context.Context, id int64, input UpdateInput) (*taskdomain.Task, error) {
	log.Printf("Service: updating task ID %d", id)
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
		log.Printf("Service: failed to get task %d for update: %v", id, err)
		return nil, err
	}

	// Single task update mode
	if !normalized.ApplyToAll {
		log.Printf("Service: performing single task update for ID %d", id)
		return s.updateSingleTask(ctx, id, &normalized)
	}

	// Series-wide update - check for recurrence changes
	if currentTask.ParentRuleID == nil {
		// Isolated task - apply single update
		return s.updateSingleTask(ctx, id, &normalized)
	}

	// Detect recurrence parameter changes
	recurrenceChanged, err := RecurrenceChanged(ctx, s.repo, currentTask, &normalized)
	if err != nil {
		return nil, fmt.Errorf("failed to check recurrence changes: %w", err)
	}

	if !recurrenceChanged {
		// Content-only update - preserve recurrence pattern
		// Apply content changes to series
		log.Printf("Service: updating series content for rule ID %d", *currentTask.ParentRuleID)
		updatedCurrentTask, err := s.repo.UpdateSeriesTaskAndCurrent(ctx, currentTask.ID, *currentTask.ParentRuleID, &normalized)
		if err != nil {
			log.Printf("Service: failed to update series content: %v", err)
			return nil, fmt.Errorf("failed to update series content: %w", err)
		}

		return updatedCurrentTask, nil
	}

	// series full update with updating recurrence rule
	log.Printf("Service: rescheduling series and full update for rule ID %d", *currentTask.ParentRuleID)
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
	log.Printf("Service: deleting task ID %d with mode %s", id, mode)
	if id <= 0 {
		return fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}

	if !mode.Valid() {
		return fmt.Errorf("%w: invalid delete mode", ErrInvalidInput)
	}

	// Fetch current task to determine series context
	currentTask, err := s.repo.GetByID(ctx, id)
	if err != nil {
		log.Printf("Service: failed to get task %d for deletion: %v", id, err)
		return err
	}

	switch mode {
	case taskdomain.DeleteModeSingle:
		log.Printf("Service: performing single task deletion for ID %d", id)
		return s.repo.Delete(ctx, id)

	case taskdomain.DeleteModeFuture:
		if currentTask.ParentRuleID == nil {
			// Isolated task - simple deletion
			log.Printf("Service: task %d is not part of series, treating as single deletion", id)
			return s.repo.Delete(ctx, id)
		}
		log.Printf("Service: deleting future tasks for rule ID %d from task %d", *currentTask.ParentRuleID, id)
		return s.repo.DeleteFutureTasksTx(ctx, *currentTask.ParentRuleID, id, currentTask.ScheduledAt)

	case taskdomain.DeleteModeEntireSeries:
		if currentTask.ParentRuleID == nil {
			// Isolated task - simple deletion
			log.Printf("Service: task %d is not part of series, treating as single deletion", id)
			return s.repo.Delete(ctx, id)
		}
		log.Printf("Service: deleting entire series for rule ID %d", *currentTask.ParentRuleID)
		return s.repo.DeleteEntireSeriesTx(ctx, *currentTask.ParentRuleID, deleteModified)

	default:
		return fmt.Errorf("%w: unsupported delete mode", ErrInvalidInput)
	}
}

func (s *Service) List(ctx context.Context) ([]taskdomain.Task, error) {
	log.Printf("Service: listing all tasks")
	return s.repo.List(ctx)
}

func (s *Service) ReplenishTasks(ctx context.Context) error {
	log.Printf("Service: starting task replenishment for %d recurrence types", len(s.planningCounts))
	now := s.now()

	totalProcessed := 0

	for recurrenceType, targetCount := range s.planningCounts {
		log.Printf("Service: processing recurrence type '%s' with target count %d", recurrenceType, targetCount)

		// Batch fetch rules requiring replenishment
		infos, err := s.repo.GetRulesWithStatsForReplenish(ctx, recurrenceType, now, targetCount)
		if err != nil {
			log.Printf("Service: ERROR fetching rules for type %s: %v", recurrenceType, err)
			return fmt.Errorf("failed to fetch rules for type %s: %w", recurrenceType, err)
		}

		log.Printf("Service: found %d rules for type '%s' that need replenishment", len(infos), recurrenceType)

		for _, info := range infos {
			err := s.processReplenishment(ctx, info, targetCount)
			if err != nil {
				log.Printf("Service: ERROR processing rule %d: %v", info.Rule.ID, err)
				continue
			}
			totalProcessed++
			log.Printf("Service: successfully processed rule %d", info.Rule.ID)
		}

		if len(infos) == 0 {
			log.Printf("Service: no rules need replenishment for type '%s'", recurrenceType)
		}
	}

	log.Printf("Service: completed replenishment - processed %d rules", totalProcessed)
	return nil

}

func (s *Service) processReplenishment(ctx context.Context, info taskdomain.ReplenishInfo, targetCount int) error {
	needed := targetCount - info.FutureCount
	if needed <= 0 {
		return nil
	}

	var params taskdomain.RecurrenceParams
	if err := json.Unmarshal(info.Rule.Params, &params); err != nil {
		return err
	}

	dates, err := s.generator.GenerateDates(info.LastTaskDate, &CreateInput{
		RecurrenceType: info.Rule.Type,
		Recurrence:     &params,
		ScheduledAt:    info.Rule.ScheduledAt,
	}, needed)

	if err != nil {
		return err
	}

	newTasks := make([]taskdomain.Task, len(dates))
	for i, date := range dates {
		newTasks[i] = taskdomain.Task{
			Title:        info.BaseTitle,
			Description:  info.BaseDescription,
			Status:       taskdomain.StatusNew,
			ScheduledAt:  &date,
			ParentRuleID: &info.Rule.ID,
			CreatedAt:    s.now(),
			UpdatedAt:    s.now(),
		}
	}

	return s.repo.CreateTasks(ctx, newTasks)
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
