package task

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
)

// CompareScheduledAtOnlyTime compares only the time component (hour, minute, second) of two time pointers
// Returns true if time components are different, false if they are the same
func IsScheduledTimeDifferent(oldTime, newTime *time.Time, taskId int64) (bool, error) {
	if oldTime == nil || newTime == nil {
		return false, fmt.Errorf("scheduledAt is null in DB or in input task with id: %d", taskId)
	}

	oldHour, oldMin, _ := oldTime.Clock()
	newHour, newMin, _ := newTime.Clock()

	return oldHour != newHour || oldMin != newMin, nil
}

// RecurrenceChanged compares recurrence rule from database with incoming parameters
// Returns true if recurrence parameters changed, false otherwise
func RecurrenceChanged(
	ctx context.Context,
	repo Repository,
	currentTask *taskdomain.Task,
	input *UpdateInput,
) (bool, error) {

	// TODO: добавить в README: при серийном обновление разрешено с фронта присылать ScheduledAt отличную только по времени. День менять нельзя в серийном обновлении
	// + предполагаю, что ScheduledAt обязателен
	scheduledTimeChanged, err := IsScheduledTimeDifferent(currentTask.ScheduledAt, input.ScheduledAt, currentTask.ID)
	if scheduledTimeChanged {
		return true, err
	}

	if input.RecurrenceType == "" && input.Recurrence == nil {
		return false, fmt.Errorf("%w: recurrence is always required in update task", ErrInvalidInput)
	}

	currentRule, err := repo.GetRuleByID(ctx, *currentTask.ParentRuleID)
	if err != nil {
		return false, err
	}

	if currentRule.Type != input.RecurrenceType {
		return true, nil
	}

	var currentParams taskdomain.RecurrenceParams
	if err := json.Unmarshal(currentRule.Params, &currentParams); err != nil {
		return false, fmt.Errorf("failed to unmarshal current rule params: %w", err)
	}

	return !RecurrenceParamsEqual(&currentParams, input.Recurrence), nil
}

// RecurrenceParamsEqual compares two RecurrenceParams structs
// Returns true if they are equal, false otherwise
func RecurrenceParamsEqual(a, b *taskdomain.RecurrenceParams) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	if a.Interval != b.Interval {
		return false
	}

	if a.MonthDay != b.MonthDay {
		return false
	}

	if (a.IsEven == nil) != (b.IsEven == nil) {
		return false
	}
	if a.IsEven != nil && b.IsEven != nil && *a.IsEven != *b.IsEven {
		return false
	}

	slices.Sort(a.WeekDays)
	slices.Sort(b.WeekDays)
	if !slices.Equal(a.WeekDays, b.WeekDays) {
		return false
	}

	if len(a.SpecificDates) != len(b.SpecificDates) {
		return false
	}
	for i, date := range a.SpecificDates {
		if !date.Equal(b.SpecificDates[i]) {
			return false
		}
	}

	return true
}
