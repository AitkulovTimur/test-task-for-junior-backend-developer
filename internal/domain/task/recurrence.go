package task

import (
	"fmt"
	"time"
)

type RecurrenceType string

const (
	TypeDaily  RecurrenceType = "daily"
	TypeWeekly RecurrenceType = "weekly"
	//TODO добавить в README: добавил от себя, так как не сложно в реализации, но заказчик бы добавил,
	//так как у медперсонала есть некоторые процедуры по дням неделям
	TypeMonthly  RecurrenceType = "monthly"
	TypeParity   RecurrenceType = "parity"
	TypeSpecific RecurrenceType = "specific"
)

func (t RecurrenceType) Valid() bool {
	switch t {
	case TypeDaily, TypeWeekly, TypeMonthly, TypeParity, TypeSpecific:
		return true
	}
	return false
}

type RecurrenceRule struct {
	ID          int64          `json:"id"`
	Type        RecurrenceType `json:"type"`
	Params      []byte         `json:"params"`
	ScheduledAt *time.Time     `json:"scheduled_at,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// TODO: добавить в README: Может быть заполнено только 1 поле. Иначе упадет валидация (оптимизация для свагера займет время, решил оставить так, для экономии времени)
type RecurrenceParams struct {
	// daily: каждые N дней (1 - каждый день, 2 - через день)
	Interval      int         `json:"interval,omitempty"`
	MonthDay      int         `json:"month_day,omitempty"`
	IsEven        *bool       `json:"is_even,omitempty"`
	SpecificDates []time.Time `json:"specific_dates,omitempty"`
	// Дни недели для типа "weekly" (например, [1, 3, 5] для Пн, Ср, Пт)
	WeekDays []int `json:"week_days,omitempty"`
}

// ReplenishInfo — model of Recurrence for generator
type ReplenishInfo struct {
	Rule            RecurrenceRule
	FutureCount     int
	LastTaskDate    time.Time
	BaseTitle       string
	BaseDescription string
}

func (p *RecurrenceParams) ValidateFieldsFilling(t RecurrenceType) error {
	if !t.Valid() {
		return fmt.Errorf("invalid recurrence type: %s", t)
	}

	switch t {
	case TypeDaily:
		if p.Interval <= 0 {
			return fmt.Errorf("daily recurrence requires interval > 0")
		}

		if p.MonthDay != 0 || p.IsEven != nil || len(p.SpecificDates) > 0 || len(p.WeekDays) > 0 {
			return fmt.Errorf("daily recurrence must only have interval field")
		}

	case TypeWeekly:
		if len(p.WeekDays) == 0 {
			return fmt.Errorf("weekly recurrence requires week_days (1-7)")
		}

		for _, day := range p.WeekDays {
			if day < 1 || day > 7 {
				return fmt.Errorf("weekly recurrence week_days must be between 1 and 7")
			}
		}
		if p.Interval != 0 || p.MonthDay != 0 || p.IsEven != nil || len(p.SpecificDates) > 0 {
			return fmt.Errorf("weekly recurrence must only have week_days field")
		}

	case TypeMonthly:
		if p.MonthDay < 1 || p.MonthDay > 31 {
			return fmt.Errorf("monthly recurrence requires month_day between 1 and 31")
		}
		if p.Interval != 0 || p.IsEven != nil || len(p.SpecificDates) > 0 || len(p.WeekDays) > 0 {
			return fmt.Errorf("monthly recurrence must only have month_day field")
		}

	case TypeParity:
		if p.IsEven == nil {
			return fmt.Errorf("parity recurrence requires is_even field")
		}
		if p.Interval != 0 || p.MonthDay != 0 || len(p.SpecificDates) > 0 || len(p.WeekDays) > 0 {
			return fmt.Errorf("parity recurrence must only have is_even field")
		}

	case TypeSpecific:
		if len(p.SpecificDates) == 0 {
			return fmt.Errorf("specific recurrence requires specific_dates")
		}

		now := time.Now()
		y, m, d := now.Date()
		today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())

		for _, date := range p.SpecificDates {
			if date.IsZero() {
				return fmt.Errorf("specific date cannot be empty/zero")
			}

			dy, dm, dd := date.Date()
			dateOnly := time.Date(dy, dm, dd, 0, 0, 0, 0, date.Location())

			if dateOnly.Before(today) {
				return fmt.Errorf(
					"specific date %s cannot be in the past",
					date.Format("2006-01-02"),
				)
			}
		}

		if p.Interval != 0 || p.MonthDay != 0 || p.IsEven != nil || len(p.WeekDays) > 0 {
			return fmt.Errorf("specific recurrence must only have specific_dates field")
		}

	default:
		return fmt.Errorf("unknown recurrence type: %s", t)
	}

	return nil
}
