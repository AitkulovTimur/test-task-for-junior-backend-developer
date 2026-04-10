package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	taskdomain "example.com/taskservice/internal/domain/task"
	usecase "example.com/taskservice/internal/usecase/task"
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
		RETURNING id, title, description, status, scheduled_at, NULL, is_modified, created_at, updated_at
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
       INSERT INTO recurrence_rules (type, params, scheduled_at, created_at)
       VALUES ($1, $2, $3, $4)
       RETURNING id
    `
	var ruleID int64
	err = tx.QueryRow(ctx, ruleQuery, rule.Type, rule.Params, rule.ScheduledAt, rule.CreatedAt).Scan(&ruleID)
	if err != nil {
		return nil, err
	}

	// 3. Insert series tasks
	// We return the first created task
	var firstCreatedTask *taskdomain.Task

	const taskQuery = `
       INSERT INTO tasks (title, description, status, scheduled_at, parent_rule_id, created_at, updated_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7)
       RETURNING id, title, description, status, scheduled_at, parent_rule_id, is_modified, created_at, updated_at
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
		SELECT id, title, description, status, scheduled_at, parent_rule_id, is_modified, created_at, updated_at
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

func (r *Repository) GetRuleByID(ctx context.Context, id int64) (*taskdomain.RecurrenceRule, error) {
	const query = `
		SELECT id, type, params, scheduled_at, created_at
		FROM recurrence_rules
		WHERE id = $1
	`

	var rule taskdomain.RecurrenceRule
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&rule.ID,
		&rule.Type,
		&rule.Params,
		&rule.ScheduledAt,
		&rule.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, taskdomain.ErrNotFound
		}
		return nil, err
	}

	return &rule, nil
}

func (r *Repository) Update(ctx context.Context, task *taskdomain.Task) (*taskdomain.Task, error) {
	const query = `
		UPDATE tasks
		SET title = $1,
			description = $2,
			status = $3,
			scheduled_at = $4,
			is_modified = true,
			updated_at = $5
		WHERE id = $6
		RETURNING id, title, description, status, scheduled_at, parent_rule_id, is_modified, created_at, updated_at
	`

	row := r.pool.QueryRow(ctx, query, task.Title, task.Description, task.Status, task.ScheduledAt, task.UpdatedAt, task.ID)
	updated, err := scanTask(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, taskdomain.ErrNotFound
		}

		return nil, err
	}

	return updated, nil
}

func (r *Repository) UpdateSeriesTaskAndCurrent(ctx context.Context, taskID, ruleID int64, input *usecase.UpdateInput) (*taskdomain.Task, error) {
	const query = `
       UPDATE tasks
       SET 
           title = $1,
           description = $2,
           status = CASE WHEN id = $3 THEN $4 ELSE status END,
           is_modified = CASE WHEN id = $3 THEN true ELSE is_modified END,
           updated_at = NOW()
       WHERE id = $3 
          OR (parent_rule_id = $5 AND status = 'new' AND is_modified = false)
       RETURNING id, title, description, status, scheduled_at, parent_rule_id, is_modified, created_at, updated_at
    `

	rows, err := r.pool.Query(ctx, query,
		input.Title,
		input.Description,
		taskID,
		input.Status,
		ruleID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var updatedCurrentTask *taskdomain.Task

	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		// We need to return the task that was edited by the user
		if task.ID == taskID {
			updatedCurrentTask = task
		}
	}

	if updatedCurrentTask == nil {
		return nil, taskdomain.ErrNotFound
	}

	return updatedCurrentTask, nil
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

func (r *Repository) DeleteFutureTasksTx(ctx context.Context, ruleID int64, taskID int64, scheduledAt *time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Delete the current task
	const deleteCurrentQuery = `DELETE FROM tasks WHERE id = $1`
	result, err := tx.Exec(ctx, deleteCurrentQuery, taskID)
	if err != nil {
		return fmt.Errorf("delete current task: %w", err)
	}
	if result.RowsAffected() == 0 {
		return taskdomain.ErrNotFound
	}

	// TODO: add to README: если у нас идет удадение будущих задач, то подразуемевается,
	// что юзер хочет удалить только шаблонные новые задачи, а не те, что он кастомизировал через single Update.
	const deleteFutureQuery = `
		DELETE FROM tasks 
		WHERE parent_rule_id = $1 AND scheduled_at > $2 AND status = 'new' AND is_modified = false
	`
	_, err = tx.Exec(ctx, deleteFutureQuery, ruleID, scheduledAt)
	if err != nil {
		return fmt.Errorf("delete future tasks: %w", err)
	}

	// Check if any tasks remain for this rule
	const checkRemainingQuery = `SELECT COUNT(*) FROM tasks WHERE parent_rule_id = $1`
	var count int
	err = tx.QueryRow(ctx, checkRemainingQuery, ruleID).Scan(&count)
	if err != nil {
		return fmt.Errorf("check remaining tasks: %w", err)
	}

	// If no tasks remain, delete the rule
	if count == 0 {
		const deleteRuleQuery = `DELETE FROM recurrence_rules WHERE id = $1`
		_, err = tx.Exec(ctx, deleteRuleQuery, ruleID)
		if err != nil {
			return fmt.Errorf("delete rule: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func (r *Repository) DeleteEntireSeriesTx(ctx context.Context, ruleID int64, deleteModified bool) error {
	// Log the deleteModified parameter value
	fmt.Printf("DeleteEntireSeriesTx: ruleID=%d, deleteModified=%t\n", ruleID, deleteModified)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Delete tasks based on deleteModified flag
	var deleteTasksQuery string
	if deleteModified {
		// Delete all tasks associated with the rule
		fmt.Printf("DeleteEntireSeriesTx: Deleting ALL tasks for ruleID=%d\n", ruleID)
		deleteTasksQuery = `DELETE FROM tasks WHERE parent_rule_id = $1`
	} else {
		// Delete only unmodified tasks
		fmt.Printf("DeleteEntireSeriesTx: Deleting only UNMODIFIED tasks for ruleID=%d\n", ruleID)
		deleteTasksQuery = `DELETE FROM tasks WHERE parent_rule_id = $1 AND is_modified = false`
	}

	_, err = tx.Exec(ctx, deleteTasksQuery, ruleID)
	if err != nil {
		return fmt.Errorf("delete series tasks: %w", err)
	}

	// Delete the recurrence rule
	const deleteRuleQuery = `DELETE FROM recurrence_rules WHERE id = $1`
	result, err := tx.Exec(ctx, deleteRuleQuery, ruleID)
	if err != nil {
		return fmt.Errorf("delete recurrence rule: %w", err)
	}
	if result.RowsAffected() == 0 {
		return taskdomain.ErrNotFound
	}

	return tx.Commit(ctx)
}

func (r *Repository) GetRulesWithStatsForReplenish(ctx context.Context,
	recurrenceType taskdomain.RecurrenceType, now time.Time,
	targetCount int) ([]taskdomain.ReplenishInfo, error) {
	// Запрос находит правила и сразу вычисляет:
	// 1. Сколько задач в будущем уже есть (future_count)
	// 2. Дату самой последней задачи в серии (last_task_date)
	// 3. Берет Title и Description из последней задачи для клонирования
	const query = `
       SELECT 
          r.id, r.type, r.params, r.scheduled_at,
          COUNT(t.id) FILTER (WHERE t.scheduled_at > $2) as future_count,
          COALESCE(MAX(t.scheduled_at), r.scheduled_at) as last_task_date,
          (SELECT title FROM tasks WHERE parent_rule_id = r.id ORDER BY scheduled_at DESC LIMIT 1) as last_title,
          (SELECT description FROM tasks WHERE parent_rule_id = r.id ORDER BY scheduled_at DESC LIMIT 1) as last_desc
       FROM recurrence_rules r
       LEFT JOIN tasks t ON r.id = t.parent_rule_id
       WHERE r.type = $1
       GROUP BY r.id
       HAVING COUNT(t.id) FILTER (WHERE t.scheduled_at > $2) < $3
    `

	rows, err := r.pool.Query(ctx, query, recurrenceType, now, targetCount)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []taskdomain.ReplenishInfo
	for rows.Next() {
		var info taskdomain.ReplenishInfo
		var lastDate *time.Time
		var title, desc *string

		err := rows.Scan(
			&info.Rule.ID, &info.Rule.Type, &info.Rule.Params, &info.Rule.ScheduledAt,
			&info.FutureCount, &lastDate, &title, &desc,
		)
		if err != nil {
			return nil, err
		}

		if lastDate != nil {
			info.LastTaskDate = *lastDate
		}
		if title != nil {
			info.BaseTitle = *title
		}
		if desc != nil {
			info.BaseDescription = *desc
		}

		results = append(results, info)
	}
	return results, nil
}

func (r *Repository) CreateTasks(ctx context.Context, tasks []taskdomain.Task) error {
	if len(tasks) == 0 {
		return nil
	}

	// Начинаем транзакцию, чтобы вставить всю пачку атомарно
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	const query = `
		INSERT INTO tasks (
			title, description, status, scheduled_at, 
			parent_rule_id, is_modified, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	// Используем подготовленное выражение для всей пачки
	for _, t := range tasks {
		_, err := tx.Exec(ctx, query,
			t.Title,
			t.Description,
			t.Status,
			t.ScheduledAt,
			t.ParentRuleID,
			t.IsModified, // По умолчанию false
			t.CreatedAt,
			t.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("exec insert task: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

func (r *Repository) UpdateRule(ctx context.Context, ruleID int64, ruleType taskdomain.RecurrenceType, params []byte, scheduledAt *time.Time) error {
	const query = `
		UPDATE recurrence_rules 
		SET type = $1, params = $2, scheduled_at = $3
		WHERE id = $4
	`

	result, err := r.pool.Exec(ctx, query, ruleType, params, scheduledAt, ruleID)
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
		SELECT id, title, description, status, scheduled_at, parent_rule_id, is_modified, created_at, updated_at
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

func (r *Repository) RescheduleSeriesTx(
	ctx context.Context,
	ruleID int64,
	currentTaskID int64,
	ruleType taskdomain.RecurrenceType,
	params []byte,
	scheduledAt *time.Time,
	newTasks []taskdomain.Task,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 1. Обновляем правило
	const ruleQuery = `UPDATE recurrence_rules SET type = $1, params = $2, scheduled_at = $3 WHERE id = $4`
	if _, err := tx.Exec(ctx, ruleQuery, ruleType, params, scheduledAt, ruleID); err != nil {
		return fmt.Errorf("update rule: %w", err)
	}

	// 2. Удаляем будущее
	const deleteQuery = `DELETE FROM tasks WHERE parent_rule_id = $1 AND status = 'new' AND is_modified = false AND id != $2`
	if _, err := tx.Exec(ctx, deleteQuery, ruleID, currentTaskID); err != nil {
		return fmt.Errorf("delete future tasks: %w", err)
	}

	// 3. Создаем новые задачи
	const insertQuery = `
		INSERT INTO tasks (title, description, status, scheduled_at, parent_rule_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	for _, task := range newTasks {
		if _, err := tx.Exec(ctx, insertQuery,
			task.Title, task.Description, task.Status,
			task.ScheduledAt, task.ParentRuleID, task.CreatedAt, task.UpdatedAt,
		); err != nil {
			return fmt.Errorf("insert task: %w", err)
		}
	}

	return tx.Commit(ctx)
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
		&task.IsModified,
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
