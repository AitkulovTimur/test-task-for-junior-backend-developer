package task

import (
	"fmt"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
	ports "example.com/taskservice/internal/usecase/task"
)

type dateGenerator struct{}

func NewGenerator() ports.Generator {
	return &dateGenerator{}
}

func (g *dateGenerator) GenerateDates(
	startFrom time.Time,
	input *ports.CreateInput,
	count int,
) ([]time.Time, error) {

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

func (g *dateGenerator) generateDaily(
	start time.Time,
	params *taskdomain.RecurrenceParams,
	count int,
) ([]time.Time, error) {

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

func (g *dateGenerator) generateWeekly(
	start time.Time,
	params *taskdomain.RecurrenceParams,
	count int,
) ([]time.Time, error) {

	if len(params.WeekDays) == 0 {
		return nil, fmt.Errorf("weekly tasks require week_days")
	}

	const daysInWeek = 7

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

func (g *dateGenerator) generateMonthly(
	start time.Time,
	params *taskdomain.RecurrenceParams,
	count int,
) ([]time.Time, error) {

	if params.MonthDay < 1 || params.MonthDay > 31 {
		return nil, fmt.Errorf("invalid month_day: %d", params.MonthDay)
	}

	dates := make([]time.Time, 0, count)
	current := start

	y, m, _ := current.Date()

	// If current day already passed target day — move to next month
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

		// Handle overflow (e.g. Feb 31 → Mar 3)
		if t.Month() != m {
			// Move to next month, day 0 → last day of target month
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

func (g *dateGenerator) generateParity(
	start time.Time,
	params *taskdomain.RecurrenceParams,
	count int,
) ([]time.Time, error) {

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

func (g *dateGenerator) generateSpecific(
	start time.Time,
	params *taskdomain.RecurrenceParams,
) ([]time.Time, error) {

	var dates []time.Time

	for _, d := range params.SpecificDates {
		if !d.Before(start) {
			dates = append(dates, d)
		}
	}

	return dates, nil
}
