package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	taskdomain "example.com/taskservice/internal/domain/task"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, task *taskdomain.Task) (*taskdomain.Task, error) {
	const query = `
		INSERT INTO tasks (title, description, status, scheduled_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, title, description, status, scheduled_at, NULL, created_at, updated_at
	`

	row := r.pool.QueryRow(ctx, query, task.Title, task.Description, task.Status, task.ScheduledAt, task.CreatedAt, task.UpdatedAt)
	created, err := scanTask(row)
	if err != nil {
		return nil, err
	}

	return created, nil
}

func (r *Repository) CreateSeries(
	ctx context.Context,
	rule *taskdomain.RecurrenceRule,
	tasks []taskdomain.Task,
) (*taskdomain.Task, error) {
	// 1. Start tx
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	// Rollback if something goes wrong (after Commit, Rollback will do nothing)
	defer tx.Rollback(ctx)

	// 2. Insert the rule
	const ruleQuery = `
       INSERT INTO recurrence_rules (type, params, created_at)
       VALUES ($1, $2, $3)
       RETURNING id
    `
	var ruleID int64
	err = tx.QueryRow(ctx, ruleQuery, rule.Type, rule.Params, rule.CreatedAt).Scan(&ruleID)
	if err != nil {
		return nil, err
	}

	// 3. Insert series tasks
	// We return the first created task
	var firstCreatedTask *taskdomain.Task

	const taskQuery = `
       INSERT INTO tasks (title, description, status, scheduled_at, parent_rule_id, created_at, updated_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7)
       RETURNING id, title, description, status, scheduled_at, parent_rule_id, created_at, updated_at
    `

	for i, t := range tasks {
		row := tx.QueryRow(ctx, taskQuery,
			t.Title,
			t.Description,
			t.Status,
			t.ScheduledAt,
			ruleID,
			t.CreatedAt,
			t.UpdatedAt,
		)

		created, err := scanTask(row)
		if err != nil {
			return nil, err
		}

		if i == 0 {
			firstCreatedTask = created
		}
	}

	// 4. Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return firstCreatedTask, nil
}

func (r *Repository) GetByID(ctx context.Context, id int64) (*taskdomain.Task, error) {
	const query = `
		SELECT id, title, description, status, scheduled_at, parent_rule_id, created_at, updated_at
		FROM tasks
		WHERE id = $1
	`

	row := r.pool.QueryRow(ctx, query, id)
	found, err := scanTask(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, taskdomain.ErrNotFound
		}

		return nil, err
	}

	return found, nil
}

func (r *Repository) Update(ctx context.Context, task *taskdomain.Task) (*taskdomain.Task, error) {
	const query = `
		UPDATE tasks
		SET title = $1,
			description = $2,
			status = $3,
			updated_at = $4
		WHERE id = $5
		RETURNING id, title, description, status, NULL, NULL, created_at, updated_at
	`

	row := r.pool.QueryRow(ctx, query, task.Title, task.Description, task.Status, task.UpdatedAt, task.ID)
	updated, err := scanTask(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, taskdomain.ErrNotFound
		}

		return nil, err
	}

	return updated, nil
}

func (r *Repository) Delete(ctx context.Context, id int64) error {
	const query = `DELETE FROM tasks WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return taskdomain.ErrNotFound
	}

	return nil
}

func (r *Repository) List(ctx context.Context) ([]taskdomain.Task, error) {
	const query = `
		SELECT id, title, description, status, scheduled_at, parent_rule_id, created_at, updated_at
		FROM tasks
		ORDER BY id DESC
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := make([]taskdomain.Task, 0)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}

		tasks = append(tasks, *task)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tasks, nil
}

type taskScanner interface {
	Scan(dest ...any) error
}

func scanTask(scanner taskScanner) (*taskdomain.Task, error) {
	var (
		task        taskdomain.Task
		status      string
		scheduledAt *time.Time
		ruleID      *int64
	)

	if err := scanner.Scan(
		&task.ID,
		&task.Title,
		&task.Description,
		&status,
		&scheduledAt,
		&ruleID,
		&task.CreatedAt,
		&task.UpdatedAt,
	); err != nil {
		return nil, err
	}

	task.Status = taskdomain.Status(status)
	task.ScheduledAt = scheduledAt
	task.ParentRuleID = ruleID

	return &task, nil
}
