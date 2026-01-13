package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"devstreamlinebot/config"
	"devstreamlinebot/consumers"
	"devstreamlinebot/models"
	"devstreamlinebot/polling"

	gitlab "gitlab.com/gitlab-org/api/client-go"
	"golang.org/x/time/rate"
	"gorm.io/driver/sqlite" // Import SQLite driver
	"gorm.io/gorm"
)

// RateLimitedTransport implements http.RoundTripper with rate limiting
type RateLimitedTransport struct {
	limiter     *rate.Limiter
	underlying  http.RoundTripper
	maxWaitTime time.Duration
}

// RoundTrip implements the http.RoundTripper interface with rate limiting
func (t *RateLimitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	if err := t.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	return t.underlying.RoundTrip(req)
}

func main() {
	// --- LOG SETUP ---
	logsDir := "logs"
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		log.Fatalf("failed to create logs directory: %v", err)
	}
	logPath := filepath.Join(logsDir, "app.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Fatalf("failed to open log file %s: %v", logPath, err)
	}
	defer f.Close()
	log.SetOutput(f)
	// -----------------

	// Load application configuration
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Initialize database connection
	db, err := gorm.Open(sqlite.Open(cfg.Database.DSN), &gorm.Config{}) // Use SQLite
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	// Auto-migrate database schemas
	if err := db.AutoMigrate(
		&models.Repository{}, &models.User{}, &models.Label{}, &models.Milestone{}, &models.MergeRequest{},
		&models.Chat{}, &models.VKUser{}, &models.VKMessage{}, &models.RepositorySubscription{}, &models.PossibleReviewer{},
		&models.LabelReviewer{}, &models.RepositorySLA{}, &models.Holiday{}, &models.MRAction{}, &models.MRComment{},
		&models.DailyDigestPreference{},
	); err != nil {
		log.Fatalf("failed to migrate database schemas: %v", err)
	}

	// Initialize GitLab client with rate limiting
	// Create a rate limiter that allows 5 requests per second with a burst of 10
	limiter := rate.NewLimiter(rate.Limit(5), 10)

	// Create custom HTTP client with rate limiting
	httpClient := &http.Client{
		Transport: &RateLimitedTransport{
			limiter:     limiter,
			underlying:  http.DefaultTransport,
			maxWaitTime: 30 * time.Second,
		},
	}

	// Initialize the GitLab client with rate limiting
	glClient, err := gitlab.NewClient(cfg.Gitlab.Token,
		gitlab.WithBaseURL(cfg.Gitlab.BaseURL),
		gitlab.WithHTTPClient(httpClient))
	if err != nil {
		log.Fatalf("failed to create GitLab client: %v", err)
	}

	// Initial load of repositories the authenticated user has access to
	opt := &gitlab.ListProjectsOptions{
		Membership: gitlab.Ptr(true),
		ListOptions: gitlab.ListOptions{
			PerPage: 100,
			Page:    1,
		},
	}
	for {
		projects, resp, err := glClient.Projects.ListProjects(opt)
		if err != nil {
			log.Fatalf("failed to list GitLab projects: %v", err)
		}

		for _, p := range projects {
			repo := models.Repository{
				GitlabID:    p.ID,
				Name:        p.Name,
				Description: p.Description,
				WebURL:      p.WebURL,
			}
			db.FirstOrCreate(&repo, models.Repository{GitlabID: p.ID})
		}

		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	// Start VK message polling and get bot instance and events channel
	vkBot, vkEvents := polling.StartVKPolling(db, cfg.VK.BaseURL, cfg.VK.Token)

	// Start GitLab user email polling (fetch missing emails)
	polling.StartUserEmailPolling(db, glClient, cfg.Gitlab.PollInterval)

	// Initialize and start VK command consumer
	vkCommandConsumer := consumers.NewVKCommandConsumer(db, vkBot, glClient, vkEvents)
	vkCommandConsumer.StartConsumer()

	// Initialize MR reviewer assignment consumer
	var startTime *time.Time
	if cfg.StartTime != "" {
		parsed, err := time.Parse("2006-01-02", cfg.StartTime)
		if err != nil {
			log.Fatalf("invalid start_time format (expected YYYY-MM-DD): %v", err)
		}
		startTime = &parsed
	}
	mrReviewerConsumer := consumers.NewMRReviewerConsumer(db, vkBot, glClient, cfg.Gitlab.PollInterval, startTime)

	// Initialize and start review digest consumer
	reviewDigestConsumer := consumers.NewReviewDigestConsumer(db, vkBot)
	reviewDigestConsumer.StartConsumer()

	// Initialize and start personal digest consumer
	personalDigestConsumer := consumers.NewPersonalDigestConsumer(db, vkBot)
	personalDigestConsumer.StartConsumer()

	// Run sequentially:
	// 1. Poll repositories
	// 2. Poll merge requests
	// 3. Assign reviewers
	go func() {
		ticker := time.NewTicker(cfg.Gitlab.PollInterval)
		defer ticker.Stop()

		for range ticker.C {
			polling.PollRepositories(db, glClient)
			polling.PollMergeRequests(db, glClient)
			mrReviewerConsumer.AssignReviewers()
		}
	}()

	// Block forever
	select {}
}
