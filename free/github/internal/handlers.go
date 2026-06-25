package internal

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Server holds all dependencies for the HTTP server.
type Server struct {
	db             *DB
	pool           *pgxpool.Pool
	client         *GitHubClient
	syncService    *SyncService
	webhookHandler *WebhookHandler
	webhookSecret  string
	startTime      time.Time
}

// NewServer creates a new Server instance with all dependencies wired up.
func NewServer(pool *pgxpool.Pool, cfg *Config) *Server {
	db := NewDB(pool, "primary")
	client := NewGitHubClient(cfg.Token)
	syncService := NewSyncService(pool, client, cfg, "primary")
	webhookHandler := NewWebhookHandler(db)

	return &Server{
		db:             db,
		pool:           pool,
		client:         client,
		syncService:    syncService,
		webhookHandler: webhookHandler,
		webhookSecret:  cfg.WebhookSecret,
		startTime:      time.Now(),
	}
}

// Router builds the chi router with all endpoints registered.
// Size-cap exception: single-responsibility HTTP route handler — 63L of request decode + validate + DB op + response encode; splitting adds indirection without cohesion gain.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health / readiness / liveness
	r.Get("/health", s.handleHealth)
	r.Get("/ready", s.handleReady)
	r.Get("/live", s.handleLive)
	r.Get("/status", s.handleStatus)

	// Webhook endpoint
	r.Post("/webhooks/github", s.handleWebhook)

	// Sync endpoint
	r.Post("/sync", s.handleSync)

	// API endpoints
	r.Route("/api", func(r chi.Router) {
		r.Get("/repos", s.handleListRepos)
		r.Get("/repos/{fullName}", s.handleGetRepo)

		r.Get("/issues", s.handleListIssues)
		r.Get("/prs", s.handleListPRs)
		r.Get("/commits", s.handleListCommits)
		r.Get("/releases", s.handleListReleases)
		r.Get("/branches", s.handleListBranches)
		r.Get("/tags", s.handleListTags)
		r.Get("/milestones", s.handleListMilestones)
		r.Get("/labels", s.handleListLabels)

		r.Get("/workflows", s.handleListWorkflows)
		r.Get("/workflow-runs", s.handleListWorkflowRuns)
		r.Get("/workflow-jobs", s.handleListWorkflowJobs)

		r.Get("/check-suites", s.handleListCheckSuites)
		r.Get("/check-runs", s.handleListCheckRuns)

		r.Get("/deployments", s.handleListDeployments)
		r.Get("/teams", s.handleListTeams)
		r.Get("/collaborators", s.handleListCollaborators)

		r.Get("/pr-reviews", s.handleListPRReviews)
		r.Get("/issue-comments", s.handleListIssueComments)
		r.Get("/pr-review-comments", s.handleListPRReviewComments)
		r.Get("/commit-comments", s.handleListCommitComments)

		r.Get("/events", s.handleListEvents)
		r.Get("/stats", s.handleStats)
	})

	return r
}

