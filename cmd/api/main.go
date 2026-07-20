package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/artemis/artemis-api/internal/config"
	"github.com/artemis/artemis-api/internal/handler"
	"github.com/artemis/artemis-api/internal/middleware"
	"github.com/artemis/artemis-api/internal/repository"
	"github.com/artemis/artemis-api/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg := config.Load()

	// Database
	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		slog.Error("failed to ping database", "error", err)
		os.Exit(1)
	}
	slog.Info("connected to database")

	// Repos + services
	jobRepo := repository.NewJobRepo(pool)
	companyRepo := repository.NewCompanyRepo(pool)
	userRepo := repository.NewUserRepo(pool)
	scraper := service.NewScraperClient(cfg.ScraperAPIURL)
	slog.Info("scraper API configured", "url", cfg.ScraperAPIURL)

	storage, err := service.NewStorageClient(
		cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket,
	)
	if err != nil {
		slog.Warn("MinIO storage not available", "error", err)
	}

	h := handler.New(jobRepo, companyRepo, userRepo, scraper, storage, cfg.JWTSecret)

	// Router
	r := chi.NewRouter()
	r.Use(middleware.CORSWithOrigins(cfg.AllowedOrigins))
	r.Use(middleware.Logger)

	r.Get("/healthz", h.Healthz)
	r.Get("/readyz", h.Healthz)

	r.Route("/api/v1", func(r chi.Router) {
		// Public: auth
		r.Post("/auth/register", h.Register)
		r.Post("/auth/login", h.Login)
		r.Post("/auth/forgot-password", h.ForgotPassword)
		r.Post("/auth/verify-security", h.VerifySecurity)
		r.Post("/auth/reset-password", h.ResetPassword)

		// Public: limited job search for homepage (returns previews only)
		r.Get("/public/jobs", h.SearchJobsPublic)

		// Protected: API key (admin scrape ops)
		r.Group(func(r chi.Router) {
			r.Use(middleware.APIKeyAuth(cfg.APIKey))
			r.Use(middleware.RateLimit(30))
			r.Post("/companies/{slug}/request", h.RequestCompany)
		})

		// Protected: JWT (all data endpoints)
		r.Group(func(r chi.Router) {
			r.Use(middleware.JWTAuth(cfg.JWTSecret))
			r.Get("/auth/me", h.Me)
			r.Post("/auth/onboarding", h.CompleteOnboarding)
			r.Post("/auth/security-question", h.SetSecurityQuestion)

			r.Get("/jobs", h.ListJobs)
			r.Get("/jobs/search", h.SearchJobs)
			r.Get("/jobs/{id}", h.GetJob)

			r.Get("/companies", h.ListCompanies)
			r.Get("/companies/{slug}", h.GetCompany)
			r.Get("/companies/{slug}/jobs", h.GetCompanyJobs)

			r.Get("/stats", h.GetStats)

			r.Get("/users/{id}", h.GetUser)
			r.Put("/users/{id}", h.UpdateUser)
			r.Put("/users/{id}/preferences", h.UpdatePreferences)
			r.Post("/users/{id}/resume", h.UploadResume)
			r.Get("/users/{id}/resume", h.GetResume)
			r.Get("/users/{id}/resume/progress", h.GetParseProgress)
			r.Get("/users/{id}/recommendations", h.GetRecommendations)

			// Saved jobs
			r.Get("/saved", h.ListSavedJobs)
			r.Get("/saved/ids", h.ListSavedJobIDs)
			r.Post("/saved/{jobId}", h.SaveJob)
			r.Delete("/saved/{jobId}", h.UnsaveJob)

			r.Post("/hunt", h.HuntCompany)
		})
	})

	// Server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("starting server", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-done
	slog.Info("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}
