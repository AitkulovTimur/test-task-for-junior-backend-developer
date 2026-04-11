package task

import (
	"context"
	"testing"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
)

type mockRepository struct {
	tasks []taskdomain.Task
}

func (m *mockRepository) Create(_ context.Context, task *taskdomain.Task) (*taskdomain.Task, error) {
	task.ID = 1
	m.tasks = append(m.tasks, *task)
	return task, nil
}

func (m *mockRepository) CreateSeries(_ context.Context, _ *taskdomain.RecurrenceRule, tasks []taskdomain.Task) (*taskdomain.Task, error) {
	firstTask := tasks[0]
	firstTask.ID = 1
	m.tasks = append(m.tasks, tasks...)
	return &firstTask, nil
}

func (m *mockRepository) GetByID(_ context.Context, id int64) (*taskdomain.Task, error) {
	for _, task := range m.tasks {
		if task.ID == id {
			return &task, nil
		}
	}
	return nil, taskdomain.ErrNotFound
}

func (m *mockRepository) GetRuleByID(_ context.Context, id int64) (*taskdomain.RecurrenceRule, error) {
	return nil, nil
}

func (m *mockRepository) Update(_ context.Context, task *taskdomain.Task) (*taskdomain.Task, error) {
	for i, t := range m.tasks {
		if t.ID == task.ID {
			m.tasks[i] = *task
			return &m.tasks[i], nil
		}
	}
	return nil, nil
}

func (m *mockRepository) Delete(_ context.Context, id int64) error {
	for i, task := range m.tasks {
		if task.ID == id {
			m.tasks = append(m.tasks[:i], m.tasks[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *mockRepository) List(_ context.Context) ([]taskdomain.Task, error) {
	return m.tasks, nil
}

func (m *mockRepository) UpdateSeriesTaskAndCurrent(_ context.Context, taskId int64, ruleID int64, input *UpdateInput) (*taskdomain.Task, error) {
	for i, task := range m.tasks {
		if task.ID == taskId {
			m.tasks[i].Title = input.Title
			m.tasks[i].Description = input.Description
			m.tasks[i].Status = input.Status
			m.tasks[i].ScheduledAt = input.ScheduledAt
			return &m.tasks[i], nil
		}
	}
	return nil, nil
}

func (m *mockRepository) CreateTasks(_ context.Context, tasks []taskdomain.Task) error {
	m.tasks = append(m.tasks, tasks...)
	return nil
}

func (m *mockRepository) RescheduleSeriesTx(_ context.Context, ruleID int64, currentTaskID int64, ruleType taskdomain.RecurrenceType, params []byte, scheduledAt *time.Time, newTasks []taskdomain.Task) error {
	return nil
}

func (m *mockRepository) DeleteFutureTasksTx(_ context.Context, ruleID int64, taskID int64, scheduledAt *time.Time) error {
	return nil
}

func (m *mockRepository) DeleteEntireSeriesTx(_ context.Context, ruleID int64, deleteModified bool) error {
	return nil
}

func (m *mockRepository) GetRulesWithStatsForReplenish(_ context.Context, recurrenceType taskdomain.RecurrenceType, now time.Time, targetCount int) ([]taskdomain.ReplenishInfo, error) {
	return nil, nil
}

type mockGenerator struct{}

func (m *mockGenerator) GenerateDates(startFrom time.Time, _ *CreateInput, countOfDatesToGen int) ([]time.Time, error) {
	dates := make([]time.Time, countOfDatesToGen)
	for i := 0; i < countOfDatesToGen; i++ {
		dates[i] = startFrom.AddDate(0, 0, i)
	}
	return dates, nil
}

func TestService_Create_SimpleTask(t *testing.T) {
	repo := &mockRepository{}
	gen := &mockGenerator{}
	service := NewService(repo, gen, map[taskdomain.RecurrenceType]int{})

	title := "Test Task"
	description := "Test Description"
	scheduledAt := time.Now().Add(24 * time.Hour)

	input := CreateInput{
		Title:       title,
		Description: description,
		ScheduledAt: &scheduledAt,
	}

	task, err := service.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if task.Title != title {
		t.Errorf("Expected title %s, got %s", title, task.Title)
	}

	if task.Description != description {
		t.Errorf("Expected description %s, got %s", description, task.Description)
	}

	if task.Status != taskdomain.StatusNew {
		t.Errorf("Expected status %s, got %s", taskdomain.StatusNew, task.Status)
	}
}

func TestService_Create_WithRecurrence(t *testing.T) {
	repo := &mockRepository{}
	gen := &mockGenerator{}
	planningCounts := map[taskdomain.RecurrenceType]int{
		taskdomain.TypeDaily: 5,
	}
	service := NewService(repo, gen, planningCounts)

	title := "Recurring Task"
	scheduledAt := time.Now().Add(24 * time.Hour)

	input := CreateInput{
		Title:          title,
		RecurrenceType: taskdomain.TypeDaily,
		Recurrence: &taskdomain.RecurrenceParams{
			Interval: 1,
		},
		ScheduledAt: &scheduledAt,
	}

	task, err := service.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if task.Title != title {
		t.Errorf("Expected title %s, got %s", title, task.Title)
	}

	if len(repo.tasks) != 5 {
		t.Errorf("Expected 5 tasks to be created, got %d", len(repo.tasks))
	}
}

func TestService_Create_InvalidTitle(t *testing.T) {
	repo := &mockRepository{}
	gen := &mockGenerator{}
	service := NewService(repo, gen, map[taskdomain.RecurrenceType]int{})

	input := CreateInput{
		Title: "",
	}

	_, err := service.Create(context.Background(), input)
	if err == nil {
		t.Error("Expected error for empty title")
	}
}

func TestService_Create_PastScheduledDate(t *testing.T) {
	repo := &mockRepository{}
	gen := &mockGenerator{}
	service := NewService(repo, gen, map[taskdomain.RecurrenceType]int{})

	pastDate := time.Now().Add(-24 * time.Hour)
	input := CreateInput{
		Title:       "Test",
		ScheduledAt: &pastDate,
	}

	_, err := service.Create(context.Background(), input)
	if err == nil {
		t.Error("Expected error for past scheduled date")
	}
}

func TestService_GetByID(t *testing.T) {
	repo := &mockRepository{}
	gen := &mockGenerator{}
	service := NewService(repo, gen, map[taskdomain.RecurrenceType]int{})

	input := CreateInput{Title: "Test"}
	created, _ := service.Create(context.Background(), input)

	task, err := service.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if task.ID != created.ID {
		t.Errorf("Expected ID %d, got %d", created.ID, task.ID)
	}
}

func TestService_GetByID_InvalidID(t *testing.T) {
	repo := &mockRepository{}
	gen := &mockGenerator{}
	service := NewService(repo, gen, map[taskdomain.RecurrenceType]int{})

	_, err := service.GetByID(context.Background(), -1)
	if err == nil {
		t.Error("Expected error for invalid ID")
	}
}

func TestService_Update(t *testing.T) {
	repo := &mockRepository{}
	gen := &mockGenerator{}
	service := NewService(repo, gen, map[taskdomain.RecurrenceType]int{})

	input := CreateInput{Title: "Original"}
	created, _ := service.Create(context.Background(), input)

	updateInput := UpdateInput{
		Title:       "Updated",
		Description: "Updated description",
		Status:      taskdomain.StatusInProgress,
	}

	task, err := service.Update(context.Background(), created.ID, updateInput)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if task.Title != "Updated" {
		t.Errorf("Expected title Updated, got %s", task.Title)
	}
}

func TestService_Delete(t *testing.T) {
	repo := &mockRepository{}
	gen := &mockGenerator{}
	service := NewService(repo, gen, map[taskdomain.RecurrenceType]int{})

	input := CreateInput{Title: "Test"}
	created, _ := service.Create(context.Background(), input)

	err := service.Delete(context.Background(), created.ID, taskdomain.DeleteModeSingle, false)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = service.GetByID(context.Background(), created.ID)
	if err == nil {
		t.Error("Expected task to be deleted")
	}
}

func TestService_List(t *testing.T) {
	repo := &mockRepository{}
	gen := &mockGenerator{}
	service := NewService(repo, gen, map[taskdomain.RecurrenceType]int{})

	service.Create(context.Background(), CreateInput{Title: "Task 1"})
	service.Create(context.Background(), CreateInput{Title: "Task 2"})

	tasks, err := service.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(tasks) != 2 {
		t.Errorf("Expected 2 tasks, got %d", len(tasks))
	}
}
