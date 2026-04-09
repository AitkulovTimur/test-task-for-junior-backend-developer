package task

import (
	"testing"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
	ports "example.com/taskservice/internal/usecase/task"
)

func TestDateGenerator_GenerateDates_NoRecurrence(t *testing.T) {
	gen := NewGenerator()

	start := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	input := &ports.CreateInput{
		Recurrence:     nil,
		RecurrenceType: "",
	}

	dates, err := gen.GenerateDates(start, input, 5)
	if err != nil {
		t.Fatalf("GenerateDates failed: %v", err)
	}

	if len(dates) != 1 {
		t.Errorf("Expected 1 date, got %d", len(dates))
	}

	if !dates[0].Equal(start) {
		t.Errorf("Expected start date %v, got %v", start, dates[0])
	}
}

func TestDateGenerator_GenerateDates_Daily(t *testing.T) {
	gen := NewGenerator()

	start := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	input := &ports.CreateInput{
		Recurrence: &taskdomain.RecurrenceParams{
			Interval: 2,
		},
		RecurrenceType: taskdomain.TypeDaily,
	}

	dates, err := gen.GenerateDates(start, input, 3)
	if err != nil {
		t.Fatalf("GenerateDates failed: %v", err)
	}

	if len(dates) != 3 {
		t.Errorf("Expected 3 dates, got %d", len(dates))
	}

	expected := []time.Time{
		time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 3, 10, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 5, 10, 0, 0, 0, time.UTC),
	}

	for i, date := range dates {
		if !date.Equal(expected[i]) {
			t.Errorf("Date %d: expected %v, got %v", i, expected[i], date)
		}
	}
}

func TestDateGenerator_GenerateDates_Daily_InvalidInterval(t *testing.T) {
	gen := NewGenerator()

	start := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	input := &ports.CreateInput{
		Recurrence: &taskdomain.RecurrenceParams{
			Interval: 0,
		},
		RecurrenceType: taskdomain.TypeDaily,
	}

	_, err := gen.GenerateDates(start, input, 3)
	if err == nil {
		t.Error("Expected error for invalid interval")
	}
}

func TestDateGenerator_GenerateDates_Weekly(t *testing.T) {
	gen := NewGenerator()

	start := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC) // Monday
	input := &ports.CreateInput{
		Recurrence: &taskdomain.RecurrenceParams{
			WeekDays: []int{1, 3, 5}, // Mon, Wed, Fri
		},
		RecurrenceType: taskdomain.TypeWeekly,
	}

	dates, err := gen.GenerateDates(start, input, 3)
	if err != nil {
		t.Fatalf("GenerateDates failed: %v", err)
	}

	if len(dates) != 3 {
		t.Errorf("Expected 3 dates, got %d", len(dates))
	}

	expected := []time.Time{
		time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC), // Monday
		time.Date(2024, 1, 3, 10, 0, 0, 0, time.UTC), // Wednesday
		time.Date(2024, 1, 5, 10, 0, 0, 0, time.UTC), // Friday
	}

	for i, date := range dates {
		if !date.Equal(expected[i]) {
			t.Errorf("Date %d: expected %v, got %v", i, expected[i], date)
		}
	}
}

func TestDateGenerator_GenerateDates_Weekly_NoWeekDays(t *testing.T) {
	gen := NewGenerator()

	start := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	input := &ports.CreateInput{
		Recurrence:     &taskdomain.RecurrenceParams{},
		RecurrenceType: taskdomain.TypeWeekly,
	}

	_, err := gen.GenerateDates(start, input, 3)
	if err == nil {
		t.Error("Expected error for missing week days")
	}
}

func TestDateGenerator_GenerateDates_Monthly(t *testing.T) {
	gen := NewGenerator()

	start := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	input := &ports.CreateInput{
		Recurrence: &taskdomain.RecurrenceParams{
			MonthDay: 15,
		},
		RecurrenceType: taskdomain.TypeMonthly,
	}

	dates, err := gen.GenerateDates(start, input, 3)
	if err != nil {
		t.Fatalf("GenerateDates failed: %v", err)
	}

	if len(dates) != 3 {
		t.Errorf("Expected 3 dates, got %d", len(dates))
	}

	expected := []time.Time{
		time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		time.Date(2024, 2, 15, 10, 0, 0, 0, time.UTC),
		time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC),
	}

	for i, date := range dates {
		if !date.Equal(expected[i]) {
			t.Errorf("Date %d: expected %v, got %v", i, expected[i], date)
		}
	}
}

func TestDateGenerator_GenerateDates_Monthly_InvalidDay(t *testing.T) {
	gen := NewGenerator()

	start := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	input := &ports.CreateInput{
		Recurrence: &taskdomain.RecurrenceParams{
			MonthDay: 32,
		},
		RecurrenceType: taskdomain.TypeMonthly,
	}

	_, err := gen.GenerateDates(start, input, 3)
	if err == nil {
		t.Error("Expected error for invalid month day")
	}
}

func TestDateGenerator_GenerateDates_Parity(t *testing.T) {
	gen := NewGenerator()

	start := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC) // Monday (odd)
	isEven := false
	input := &ports.CreateInput{
		Recurrence: &taskdomain.RecurrenceParams{
			IsEven: &isEven,
		},
		RecurrenceType: taskdomain.TypeParity,
	}

	dates, err := gen.GenerateDates(start, input, 3)
	if err != nil {
		t.Fatalf("GenerateDates failed: %v", err)
	}

	if len(dates) != 3 {
		t.Errorf("Expected 3 dates, got %d", len(dates))
	}

	for i, date := range dates {
		if date.Day()%2 == 0 {
			t.Errorf("Date %d: expected odd day, got even day %d", i, date.Day())
		}
	}
}

func TestDateGenerator_GenerateDates_Parity_NoIsEven(t *testing.T) {
	gen := NewGenerator()

	start := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	input := &ports.CreateInput{
		Recurrence:     &taskdomain.RecurrenceParams{},
		RecurrenceType: taskdomain.TypeParity,
	}

	_, err := gen.GenerateDates(start, input, 3)
	if err == nil {
		t.Error("Expected error for missing is_even field")
	}
}

func TestDateGenerator_GenerateDates_Specific(t *testing.T) {
	gen := NewGenerator()

	start := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	specificDates := []time.Time{
		time.Date(2024, 1, 5, 10, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 10, 10, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
	}
	input := &ports.CreateInput{
		Recurrence: &taskdomain.RecurrenceParams{
			SpecificDates: specificDates,
		},
		RecurrenceType: taskdomain.TypeSpecific,
	}

	dates, err := gen.GenerateDates(start, input, 10)
	if err != nil {
		t.Fatalf("GenerateDates failed: %v", err)
	}

	if len(dates) != 3 {
		t.Errorf("Expected 3 dates, got %d", len(dates))
	}

	for i, date := range dates {
		if !date.Equal(specificDates[i]) {
			t.Errorf("Date %d: expected %v, got %v", i, specificDates[i], date)
		}
	}
}

func TestDateGenerator_GenerateDates_Specific_PastDates(t *testing.T) {
	gen := NewGenerator()

	start := time.Date(2024, 1, 10, 10, 0, 0, 0, time.UTC)
	specificDates := []time.Time{
		time.Date(2024, 1, 5, 10, 0, 0, 0, time.UTC),  // Past
		time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC), // Future
		time.Date(2024, 1, 20, 10, 0, 0, 0, time.UTC), // Future
	}
	input := &ports.CreateInput{
		Recurrence: &taskdomain.RecurrenceParams{
			SpecificDates: specificDates,
		},
		RecurrenceType: taskdomain.TypeSpecific,
	}

	dates, err := gen.GenerateDates(start, input, 10)
	if err != nil {
		t.Fatalf("GenerateDates failed: %v", err)
	}

	if len(dates) != 2 {
		t.Errorf("Expected 2 dates (past filtered out), got %d", len(dates))
	}

	expected := []time.Time{
		time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 20, 10, 0, 0, 0, time.UTC),
	}

	for i, date := range dates {
		if !date.Equal(expected[i]) {
			t.Errorf("Date %d: expected %v, got %v", i, expected[i], date)
		}
	}
}

func TestDateGenerator_GenerateDates_UnsupportedType(t *testing.T) {
	gen := NewGenerator()

	start := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	input := &ports.CreateInput{
		Recurrence:     &taskdomain.RecurrenceParams{},
		RecurrenceType: "unsupported",
	}

	_, err := gen.GenerateDates(start, input, 3)
	if err == nil {
		t.Error("Expected error for unsupported recurrence type")
	}
}
