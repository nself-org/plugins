package main

import (
    "context"
    "fmt"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
    "github.com/nself-org/nself-workflows/internal"
)

func main() {
    cfg := internal.LoadConfig()

    pool, err := internal.NewDB(context.Background(), cfg.DatabaseURL)
    if err != nil {
        log.Fatalf("db connect: %v", err)
    }
    defer pool.Close()

    h := internal.NewHandlers(pool)

    r := chi.NewRouter()
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)
    r.Use(middleware.Timeout(60 * time.Second))

    r.Get("/health", h.Health)
    r.Get("/ready", h.Ready)

    // Workflows
    r.Route("/api/v1/workflows", func(r chi.Router) {
        r.Get("/", h.ListWorkflows)
        r.Post("/", h.CreateWorkflow)
        r.Get("/{id}", h.GetWorkflow)
        r.Put("/{id}", h.UpdateWorkflow)
        r.Delete("/{id}", h.DeleteWorkflow)
        r.Post("/{id}/publish", h.PublishWorkflow)
        r.Post("/{id}/unpublish", h.UnpublishWorkflow)
        r.Post("/{id}/duplicate", h.DuplicateWorkflow)
        r.Post("/{id}/execute", h.ExecuteWorkflow)
        r.Post("/{id}/test", h.TestWorkflow)
        r.Get("/{id}/executions", h.ListExecutions)
    })

    // Executions
    r.Route("/api/v1/executions", func(r chi.Router) {
        r.Get("/", h.ListAllExecutions)
        r.Get("/{id}", h.GetExecution)
        r.Post("/{id}/cancel", h.CancelExecution)
        r.Get("/{id}/steps", h.ListExecutionSteps)
    })

    // Triggers
    r.Route("/api/v1/triggers", func(r chi.Router) {
        r.Get("/", h.ListTriggers)
        r.Post("/", h.CreateTrigger)
        r.Get("/{id}", h.GetTrigger)
        r.Put("/{id}", h.UpdateTrigger)
        r.Delete("/{id}", h.DeleteTrigger)
    })

    // Templates
    r.Route("/api/v1/templates", func(r chi.Router) {
        r.Get("/", h.ListTemplates)
        r.Post("/", h.CreateTemplate)
        r.Get("/{id}", h.GetTemplate)
        r.Put("/{id}", h.UpdateTemplate)
        r.Delete("/{id}", h.DeleteTemplate)
    })

    // Variables
    r.Route("/api/v1/variables", func(r chi.Router) {
        r.Get("/", h.ListVariables)
        r.Post("/", h.CreateVariable)
        r.Get("/{id}", h.GetVariable)
        r.Put("/{id}", h.UpdateVariable)
        r.Delete("/{id}", h.DeleteVariable)
    })

    // Webhooks
    r.Post("/webhooks/{token}", h.HandleWebhook)

    srv := &http.Server{
        Addr:         fmt.Sprintf(":%s", cfg.Port),
        Handler:      r,
        ReadTimeout:  30 * time.Second,
        WriteTimeout: 30 * time.Second,
        IdleTimeout:  120 * time.Second,
    }

    go func() {
        log.Printf("workflows plugin listening on :%s", cfg.Port)
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("listen: %v", err)
        }
    }()

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    if err := srv.Shutdown(ctx); err != nil {
        log.Fatalf("shutdown: %v", err)
    }
}
