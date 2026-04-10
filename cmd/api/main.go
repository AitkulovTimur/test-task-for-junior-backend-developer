package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
	infrastructurepostgres "example.com/taskservice/internal/infrastructure/postgres"
	logic "example.com/taskservice/internal/logic/task"
	postgresrepo "example.com/taskservice/internal/repository/postgres"
	transporthttp "example.com/taskservice/internal/transport/http"
	swaggerdocs "example.com/taskservice/internal/transport/http/docs"
	httphandlers "example.com/taskservice/internal/transport/http/handlers"
	"example.com/taskservice/internal/usecase/task"
	"example.com/taskservice/internal/worker"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg := loadConfig()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := infrastructurepostgres.Open(ctx, cfg.DatabaseDSN)
	if err != nil {
		logger.Error("open postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	taskRepo := postgresrepo.New(pool)
	dateGenerator := logic.NewGenerator()
	taskUsecase := task.NewService(taskRepo, dateGenerator, cfg.PlanningCounts)

	// Initialize and start planner
	planner := worker.NewPlanner(taskUsecase, cfg.PlannerInterval)
	go planner.Start(ctx)

	taskHandler := httphandlers.NewTaskHandler(taskUsecase)
	docsHandler := swaggerdocs.NewHandler()
	router := transporthttp.NewRouter(taskHandler, docsHandler)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown http server", "error", err)
		}
	}()

	logger.Info("http server started", "addr", cfg.HTTPAddr)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("listen and serve", "error", err)
		os.Exit(1)
	}
}

type config struct {
	HTTPAddr        string
	DatabaseDSN     string
	PlannerInterval time.Duration
	// Map for dates generation limits
	PlanningCounts map[taskdomain.RecurrenceType]int
}

func loadConfig() config {
	cfg := config{
		HTTPAddr:        envOrDefault("HTTP_ADDR", ":8080"),
		DatabaseDSN:     envOrDefault("DATABASE_DSN", "postgres://postgres:postgres@localhost:5432/taskservice?sslmode=disable"),
		PlannerInterval: getDurationOrDefault("PLANNER_INTERVAL", 1*time.Minute), //For test purposes
		PlanningCounts: map[taskdomain.RecurrenceType]int{
			//TODO добавить в README: число задач для предгенерации. Сделал конфигурируемыми, несмотря на то, что в требованиях не было такого условия
			taskdomain.TypeDaily:   7,
			taskdomain.TypeWeekly:  7,
			taskdomain.TypeMonthly: 3, //quarter of the year
			taskdomain.TypeParity:  7,
			// Specific doesn't require a limit because always fully pregenerated
		},
	}

	if cfg.DatabaseDSN == "" {
		panic(fmt.Errorf("DATABASE_DSN is required"))
	}

	return cfg
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func getDurationOrDefault(key string, fallback time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return fallback
}
