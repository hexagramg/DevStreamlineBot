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
	"devstreamlinebot/utils"

	gitlab "gitlab.com/gitlab-org/api/client-go"
	"golang.org/x/time/rate"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type RateLimitedTransport struct {
	limiter     *rate.Limiter
	underlying  http.RoundTripper
	maxWaitTime time.Duration
}

func (t *RateLimitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	if err := t.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	return t.underlying.RoundTrip(req)
}

func main() {
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

	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	db, err := gorm.Open(sqlite.Open(cfg.Database.DSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	if err := db.AutoMigrate(
		&models.Repository{}, &models.User{}, &models.Label{}, &models.Milestone{}, &models.MergeRequest{},
		&models.Chat{}, &models.VKUser{}, &models.VKMessage{}, &models.RepositorySubscription{}, &models.PossibleReviewer{},
		&models.LabelReviewer{}, &models.RepositorySLA{}, &models.Holiday{}, &models.MRAction{}, &models.MRComment{},
		&models.DailyDigestPreference{}, &models.BlockLabel{}, &models.ReleaseManager{}, &models.ReleaseLabel{},
		&models.AutoReleaseBranchConfig{}, &models.ReleaseReadyLabel{}, &models.JiraProjectPrefix{},
		&models.ReleaseSubscription{}, &models.MRNotificationState{},
		&models.FeatureReleaseLabel{}, &models.FeatureReleaseBranch{},
		&models.DeployTrackingRule{}, &models.TrackedDeployJob{},
	); err != nil {
		log.Fatalf("failed to migrate database schemas: %v", err)
	}

	if err := utils.BackfillThreadMetadata(db); err != nil {
		log.Printf("Warning: thread metadata backfill failed: %v", err)
	}

	limiter := rate.NewLimiter(rate.Limit(5), 10)

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
				GitlabID:          p.ID,
				Name:              p.Name,
				Path:              p.Path,
				PathWithNamespace: p.PathWithNamespace,
				Description:       p.Description,
				WebURL:            p.WebURL,
			}
			db.Where(models.Repository{GitlabID: p.ID}).Assign(repo).FirstOrCreate(&models.Repository{})
		}

		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	vkBot, vkEvents := polling.StartVKPolling(db, cfg.VK.BaseURL, cfg.VK.Token)

	polling.StartUserEmailPolling(db, glClient, cfg.Gitlab.PollInterval)

	vkCommandConsumer := consumers.NewVKCommandConsumer(db, vkBot, glClient, vkEvents)
	vkCommandConsumer.StartConsumer()

	var startTime *time.Time
	if cfg.StartTime != "" {
		parsed, err := time.Parse("2006-01-02", cfg.StartTime)
		if err != nil {
			log.Fatalf("invalid start_time format (expected YYYY-MM-DD): %v", err)
		}
		startTime = &parsed
	}
	mrReviewerConsumer := consumers.NewMRReviewerConsumer(db, vkBot, glClient, cfg.Gitlab.PollInterval, startTime)

	reviewDigestConsumer := consumers.NewReviewDigestConsumer(db, vkBot)
	reviewDigestConsumer.StartConsumer()

	personalDigestConsumer := consumers.NewPersonalDigestConsumer(db, vkBot)
	personalDigestConsumer.StartConsumer()

	autoReleaseConsumer := consumers.NewAutoReleaseConsumer(db, glClient, cfg.Jira.BaseURL)

	releaseNotificationConsumer := consumers.NewReleaseNotificationConsumer(db, vkBot)

	deployTrackingConsumer := consumers.NewDeployTrackingConsumer(db, vkBot, glClient)

	go func() {
		ticker := time.NewTicker(cfg.Gitlab.PollInterval)
		defer ticker.Stop()

		for range ticker.C {
			polling.PollRepositories(db, glClient)
			polling.PollMergeRequests(db, glClient)
			mrReviewerConsumer.AssignReviewers()
			mrReviewerConsumer.ProcessStateChangeNotifications()
			mrReviewerConsumer.ProcessReviewerRemovalNotifications()
			mrReviewerConsumer.ProcessFullyApprovedNotifications()
			mrReviewerConsumer.CleanupOldUnnotifiedActions()
			autoReleaseConsumer.ProcessAutoReleaseBranches()
			autoReleaseConsumer.ProcessReleaseMRDescriptions()
			autoReleaseConsumer.ProcessFeatureReleaseMRDescriptions()
			releaseNotificationConsumer.ProcessNewReleaseNotifications()
			releaseNotificationConsumer.ProcessReleaseMRDescriptionChanges()
			releaseNotificationConsumer.ProcessReleaseMergedNotifications()
			deployTrackingConsumer.PollDeployJobs()
		}
	}()

	select {}
}
