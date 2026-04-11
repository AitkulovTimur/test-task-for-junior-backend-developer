package postgres

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	taskdomain "example.com/taskservice/internal/domain/task"
	usecase "example.com/taskservice/internal/usecase/task"
)

// Repository handles database operations for tasks and recurrence rules
type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create inserts a new task into the database and returns the created task
func (r *Repository) Create(ctx context.Context, task *taskdomain.Task) (*taskdomain.Task, error) {
	log.Printf("Repository: creating task '%s'", task.Title)
	const query = `
		INSERT INTO tasks (title, description, status, scheduled_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, title, description, status, scheduled_at, NULL, is_modified, created_at, updated_at
	`

	row := r.pool.QueryRow(ctx, query, task.Title, task.Description, task.Status, task.ScheduledAt, task.CreatedAt, task.UpdatedAt)
	created, err := scanTask(row)
	if err != nil {
		log.Printf("Repository: failed to create task: %v", err)
		return nil, err
	}

	log.Printf("Repository: successfully created task with ID %d", created.ID)
	return created, nil
}

// CreateSeries creates a recurrence rule and associated tasks in a single transaction
// Returns the first created task from the series
func (r *Repository) CreateSeries(
	ctx context.Context,
	rule *taskdomain.RecurrenceRule,
	tasks []taskdomain.Task,
) (*taskdomain.Task, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Insert the recurrence rule
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

	// Insert series tasks and return the first created task
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

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return firstCreatedTask, nil
}

// GetByID retrieves a task by its ID
func (r *Repository) GetByID(ctx context.Context, id int64) (*taskdomain.Task, error) {
	log.Printf("Repository: getting task by ID %d", id)
	const query = `
		SELECT id, title, description, status, scheduled_at, parent_rule_id, is_modified, created_at, updated_at
		FROM tasks
		WHERE id = $1
	`

	row := r.pool.QueryRow(ctx, query, id)
	found, err := scanTask(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Printf("Repository: task with ID %d not found", id)
			return nil, taskdomain.ErrNotFound
		}

		log.Printf("Repository: failed to get task %d: %v", id, err)
		return nil, err
	}

	return found, nil
}

// GetRuleByID retrieves a recurrence rule by its ID
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

// Update modifies an existing task and marks it as modified
func (r *Repository) Update(ctx context.Context, task *taskdomain.Task) (*taskdomain.Task, error) {
	log.Printf("Repository: updating task ID %d", task.ID)
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
			log.Printf("Repository: task with ID %d not found for update", task.ID)
			return nil, taskdomain.ErrNotFound
		}

		log.Printf("Repository: failed to update task %d: %v", task.ID, err)
		return nil, err
	}

	log.Printf("Repository: successfully updated task %d", task.ID)
	return updated, nil
}

// UpdateSeriesTaskAndCurrent updates a specific task and applies title/description changes
// to all future unmodified tasks in the series. Returns the updated current task.
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
		// Return the task that was edited by the user
		if task.ID == taskID {
			updatedCurrentTask = task
		}
	}

	if updatedCurrentTask == nil {
		return nil, taskdomain.ErrNotFound
	}

	return updatedCurrentTask, nil
}

// Delete removes a task by its ID
func (r *Repository) Delete(ctx context.Context, id int64) error {
	log.Printf("Repository: deleting task ID %d", id)
	const query = `DELETE FROM tasks WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		log.Printf("Repository: failed to delete task %d: %v", id, err)
		return err
	}

	if result.RowsAffected() == 0 {
		log.Printf("Repository: task with ID %d not found for deletion", id)
		return taskdomain.ErrNotFound
	}

	log.Printf("Repository: successfully deleted task %d", id)
	return nil
}

// DeleteFutureTasksTx deletes the current task and all future unmodified tasks in a series
// Also deletes the recurrence rule if no tasks remain
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
	var remainingTasksCount int
	err = tx.QueryRow(ctx, checkRemainingQuery, ruleID).Scan(&remainingTasksCount)
	if err != nil {
		return fmt.Errorf("check remaining tasks: %w", err)
	}

	// If no tasks remain, delete the rule
	if remainingTasksCount == 0 {
		const deleteRuleQuery = `DELETE FROM recurrence_rules WHERE id = $1`
		_, err = tx.Exec(ctx, deleteRuleQuery, ruleID)
		if err != nil {
			return fmt.Errorf("delete rule: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// DeleteEntireSeriesTx deletes an entire series of tasks and the recurrence rule
// deleteModified determines whether to delete all tasks (true) or only unmodified ones (false)
func (r *Repository) DeleteEntireSeriesTx(ctx context.Context, ruleID int64, deleteModified bool) error {
	// Log the deleteModified parameter value for debugging
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

// GetRulesWithStatsForReplenish finds recurrence rules that need task replenishment
// Returns rules with statistics about future tasks and last task details
func (r *Repository) GetRulesWithStatsForReplenish(ctx context.Context,
	recurrenceType taskdomain.RecurrenceType, now time.Time,
	targetCount int) ([]taskdomain.ReplenishInfo, error) {
	// Query finds rules and calculates:
	// 1. How many future tasks already exist (future_count)
	// 2. Date of the last task in the series (last_task_date)
	// 3. Title and Description from the last task for cloning
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

	var replenishInfos []taskdomain.ReplenishInfo
	for rows.Next() {
		var replenishInfo taskdomain.ReplenishInfo
		var lastDate *time.Time
		var title, desc *string

		err := rows.Scan(
			&replenishInfo.Rule.ID, &replenishInfo.Rule.Type, &replenishInfo.Rule.Params, &replenishInfo.Rule.ScheduledAt,
			&replenishInfo.FutureCount, &lastDate, &title, &desc,
		)
		if err != nil {
			return nil, err
		}

		if lastDate != nil {
			replenishInfo.LastTaskDate = *lastDate
		}
		if title != nil {
			replenishInfo.BaseTitle = *title
		}
		if desc != nil {
			replenishInfo.BaseDescription = *desc
		}

		replenishInfos = append(replenishInfos, replenishInfo)
	}
	return replenishInfos, nil
}

// CreateTasks inserts multiple tasks in a single transaction
func (r *Repository) CreateTasks(ctx context.Context, tasks []taskdomain.Task) error {
	if len(tasks) == 0 {
		return nil
	}

	// Start transaction to insert all tasks atomically
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

	// Use prepared statement for the entire batch
	for _, t := range tasks {
		_, err := tx.Exec(ctx, query,
			t.Title,
			t.Description,
			t.Status,
			t.ScheduledAt,
			t.ParentRuleID,
			t.IsModified, // Default false
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

// UpdateRule modifies an existing recurrence rule
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

// List retrieves all tasks ordered by ID in descending order
func (r *Repository) List(ctx context.Context) ([]taskdomain.Task, error) {
	log.Printf("Repository: listing all tasks")
	const query = `
		SELECT id, title, description, status, scheduled_at, parent_rule_id, is_modified, created_at, updated_at
		FROM tasks
		ORDER BY id DESC
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		log.Printf("Repository: failed to list tasks: %v", err)
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
		log.Printf("Repository: error iterating task rows: %v", err)
		return nil, err
	}

	log.Printf("Repository: retrieved %d tasks", len(tasks))
	return tasks, nil
}

// RescheduleSeriesTx updates a recurrence rule and reschedules future tasks
// Updates the rule, deletes future unmodified tasks, and creates new tasks
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

	// Update the recurrence rule
	const ruleQuery = `UPDATE recurrence_rules SET type = $1, params = $2, scheduled_at = $3 WHERE id = $4`
	if _, err := tx.Exec(ctx, ruleQuery, ruleType, params, scheduledAt, ruleID); err != nil {
		return fmt.Errorf("update rule: %w", err)
	}

	// Delete future unmodified tasks (except current task)
	const deleteQuery = `DELETE FROM tasks WHERE parent_rule_id = $1 AND status = 'new' AND is_modified = false AND id != $2`
	if _, err := tx.Exec(ctx, deleteQuery, ruleID, currentTaskID); err != nil {
		return fmt.Errorf("delete future tasks: %w", err)
	}

	// Create new tasks
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

// taskScanner interface defines the Scan method for database row scanning
type taskScanner interface {
	Scan(dest ...any) error
}

// scanTask scans database row data into a Task struct
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
