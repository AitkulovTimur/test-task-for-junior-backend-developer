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
		for i, d := range dates {
			now := s.now()

			tasks[i] = taskdomain.Task{
				Title:       normalized.Title,
				Description: normalized.Description,
				Status:      normalized.Status,
				ScheduledAt: &d,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
		}

		// 4. Serialization for DB JSONB
		paramsJSON, err := json.Marshal(input.Recurrence)
		if err != nil {
			return nil, fmt.Errorf("marshal recurrence params: %w", err)
		}

		rule := &taskdomain.RecurrenceRule{
			Type:      input.RecurrenceType,
			Params:    paramsJSON,
			CreatedAt: s.now(),
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

	model := &taskdomain.Task{
		ID:          id,
		Title:       normalized.Title,
		Description: normalized.Description,
		Status:      normalized.Status,
		UpdatedAt:   s.now(),
	}

	updated, err := s.repo.Update(ctx, model)
	if err != nil {
		return nil, err
	}

	return updated, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}

	return s.repo.Delete(ctx, id)
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

	return input, nil
}

func validateUpdateInput(input UpdateInput) (UpdateInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)

	if input.Title == "" {
		return UpdateInput{}, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}

	if !input.Status.Valid() {
		return UpdateInput{}, fmt.Errorf("%w: invalid status", ErrInvalidInput)
	}

	return input, nil
}
