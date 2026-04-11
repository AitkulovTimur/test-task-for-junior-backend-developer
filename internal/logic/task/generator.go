package task

import (
	"fmt"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
	ports "example.com/taskservice/internal/usecase/task"
)

// dateGenerator implements generating task dates
// based on different recurrence patterns (daily, weekly, monthly, parity, specific)
type dateGenerator struct{}

func NewGenerator() ports.Generator {
	return &dateGenerator{}
}

// GenerateDates generates a slice of dates based on the recurrence rules
// startFrom: the initial date to start generating from
// input: contains recurrence parameters and type
// count: number of dates to generate
func (g *dateGenerator) GenerateDates(
	startFrom time.Time,
	input *ports.CreateInput,
	count int,
) ([]time.Time, error) {

	// If no recurrence is specified, return only the start date
	if input.Recurrence == nil {
		return []time.Time{startFrom}, nil
	}

	switch input.RecurrenceType {
	case taskdomain.TypeDaily:
		return g.generateDaily(startFrom, input.Recurrence, count)

	case taskdomain.TypeWeekly:
		return g.generateWeekly(startFrom, input.Recurrence, count)

	case taskdomain.TypeMonthly:
		return g.generateMonthly(startFrom, input.Recurrence, count)

	case taskdomain.TypeParity:
		return g.generateParity(startFrom, input.Recurrence, count)

	case taskdomain.TypeSpecific:
		return g.generateSpecific(startFrom, input.Recurrence)

	default:
		return nil, fmt.Errorf("unsupported recurrence type: %s", input.RecurrenceType)
	}
}

// generateDaily creates dates for daily recurring tasks
// Interval specifies how many days to skip between each occurrence
func (g *dateGenerator) generateDaily(
	start time.Time,
	params *taskdomain.RecurrenceParams,
	count int,
) ([]time.Time, error) {

	// Validate that interval is positive (can't skip negative days)
	if params.Interval <= 0 {
		return nil, fmt.Errorf("interval must be positive for daily tasks")
	}

	dates := make([]time.Time, 0, count)
	current := start

	for len(dates) < count {
		dates = append(dates, current)
		current = current.AddDate(0, 0, params.Interval)
	}

	return dates, nil
}

// generateWeekly creates dates for weekly recurring tasks on specific weekdays.
// WeekDays defines the days of the week when the task should occur.
//
// Weekday mapping follows Go's time.Weekday convention:
//
//	0 = Sunday
//	1 = Monday
//	...
//	6 = Saturday
//
// Values outside the 0–6 range will be normalized using modulo 7.
func (g *dateGenerator) generateWeekly(
	start time.Time,
	params *taskdomain.RecurrenceParams,
	count int,
) ([]time.Time, error) {

	// Validate that at least one weekday is specified
	if len(params.WeekDays) == 0 {
		return nil, fmt.Errorf("weekly tasks require week_days")
	}

	const daysInWeek = 7

	// Convert weekday numbers to a map for quick lookup
	days := make(map[time.Weekday]bool)
	for _, d := range params.WeekDays {
		days[time.Weekday(d%daysInWeek)] = true
	}

	dates := make([]time.Time, 0, count)
	current := start

	for len(dates) < count {
		if days[current.Weekday()] {
			dates = append(dates, current)
		}
		current = current.AddDate(0, 0, 1)
	}

	return dates, nil
}

// generateMonthly creates dates for monthly recurring tasks on a specific day of the month
// MonthDay specifies which day of the month (1-31) the task should occur
func (g *dateGenerator) generateMonthly(
	start time.Time,
	params *taskdomain.RecurrenceParams,
	count int,
) ([]time.Time, error) {

	// Validate that month day is within valid range
	if params.MonthDay < 1 || params.MonthDay > 31 {
		return nil, fmt.Errorf("invalid month_day: %d", params.MonthDay)
	}

	dates := make([]time.Time, 0, count)
	current := start

	y, m, _ := current.Date()

	// If current day already passed target day for this month, move to next month
	if current.Day() > params.MonthDay {
		m++
	}

	for len(dates) < count {
		t := time.Date(
			y, m, params.MonthDay,
			current.Hour(),
			current.Minute(),
			current.Second(),
			0,
			current.Location(),
		)

		// Handle month overflow (e.g. Feb 31 becomes Mar 3)
		// If the generated date is in a different month, it means overflow occurred
		if t.Month() != m {
			// Use the last day of the target month instead
			// day 0 in time.Date means "last day of previous month"
			t = time.Date(
				y, m+1, 0,
				current.Hour(),
				current.Minute(),
				current.Second(),
				0,
				current.Location(),
			)
		}

		if !t.Before(start) {
			dates = append(dates, t)
		}

		m++
	}

	return dates, nil
}

// generateParity creates dates based on day parity (even or odd days of month)
// IsEven specifies whether to generate dates on even (true) or odd (false) days
func (g *dateGenerator) generateParity(
	start time.Time,
	params *taskdomain.RecurrenceParams,
	count int,
) ([]time.Time, error) {

	// Validate that parity parameter is specified
	if params.IsEven == nil {
		return nil, fmt.Errorf("is_even is required for parity tasks")
	}

	dates := make([]time.Time, 0, count)
	current := start

	for len(dates) < count {
		isEven := current.Day()%2 == 0
		if isEven == *params.IsEven {
			dates = append(dates, current)
		}
		current = current.AddDate(0, 0, 1)
	}

	return dates, nil
}

// generateSpecific returns pre-defined specific dates for the task
// SpecificDates contains the exact dates when the task should occur
// count parameter is ignored as we use all provided dates
func (g *dateGenerator) generateSpecific(
	start time.Time,
	params *taskdomain.RecurrenceParams,
) ([]time.Time, error) {

	var dates []time.Time

	// Filter dates to include only those that are on or after the start date
	for _, d := range params.SpecificDates {
		if !d.Before(start) {
			dates = append(dates, d)
		}
	}

	return dates, nil
}
