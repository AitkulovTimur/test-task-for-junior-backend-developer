package worker

import (
	"context"
	"log"
	"time"

	taskusecase "example.com/taskservice/internal/usecase/task"
)

type Planner struct {
	taskService taskusecase.Usecase
	interval    time.Duration
}

func NewPlanner(taskService taskusecase.Usecase, interval time.Duration) *Planner {
	return &Planner{
		taskService: taskService,
		interval:    interval,
	}
}

func (p *Planner) Start(ctx context.Context) {
	log.Printf("Planner started with interval: %v", p.interval)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	log.Printf("Planner: running initial task replenishment")
	p.runReplenish(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Planner: stopping due to context cancellation")
			return
		case <-ticker.C:
			log.Printf("Planner: running scheduled task replenishment")
			p.runReplenish(ctx)
		}
	}
}

func (p *Planner) runReplenish(ctx context.Context) {
	start := time.Now()
	log.Printf("Planner: starting task replenishment process at %s", start.Format("2006-01-02 15:04:05"))

	if err := p.taskService.ReplenishTasks(ctx); err != nil {
		log.Printf("Planner: ERROR during task replenishment: %v", err)
		return
	}

	duration := time.Since(start)
	log.Printf("Planner: successfully completed task replenishment in %v", duration)
}
