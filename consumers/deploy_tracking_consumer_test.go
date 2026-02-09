package consumers

import (
	"strings"
	"testing"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"devstreamlinebot/mocks"
	"devstreamlinebot/models"
	"devstreamlinebot/testutils"
)

// ============================================================================
// parseJobURL Tests
// ============================================================================

func TestParseJobURL_ValidURL(t *testing.T) {
	path, jobID, err := parseJobURL("https://gitlab.corp.mail.ru/is-team/ansible/projects/joboffer/-/jobs/293024539")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "is-team/ansible/projects/joboffer" {
		t.Errorf("expected path 'is-team/ansible/projects/joboffer', got '%s'", path)
	}
	if jobID != 293024539 {
		t.Errorf("expected jobID 293024539, got %d", jobID)
	}
}

func TestParseJobURL_SimpleProjectPath(t *testing.T) {
	path, jobID, err := parseJobURL("https://gitlab.com/group/project/-/jobs/123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "group/project" {
		t.Errorf("expected path 'group/project', got '%s'", path)
	}
	if jobID != 123 {
		t.Errorf("expected jobID 123, got %d", jobID)
	}
}

func TestParseJobURL_NoJobsPattern(t *testing.T) {
	_, _, err := parseJobURL("https://gitlab.com/group/project/-/pipelines/123")
	if err == nil {
		t.Fatal("expected error for URL without /-/jobs/ pattern")
	}
}

func TestParseJobURL_InvalidJobID(t *testing.T) {
	_, _, err := parseJobURL("https://gitlab.com/group/project/-/jobs/abc")
	if err == nil {
		t.Fatal("expected error for invalid job ID")
	}
}

func TestParseJobURL_TrailingSlash(t *testing.T) {
	path, jobID, err := parseJobURL("https://gitlab.com/group/project/-/jobs/456/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "group/project" {
		t.Errorf("expected path 'group/project', got '%s'", path)
	}
	if jobID != 456 {
		t.Errorf("expected jobID 456, got %d", jobID)
	}
}

// ============================================================================
// PollDeployJobs Tests
// ============================================================================

func TestPollDeployJobs_NoRules(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()
	mockJobs := &mocks.MockJobsService{}

	consumer := NewDeployTrackingConsumerWithDeps(db, mockBot, mockJobs)
	consumer.PollDeployJobs()

	if len(mockJobs.ListProjectJobsCalls) != 0 {
		t.Errorf("expected no API calls, got %d", len(mockJobs.ListProjectJobsCalls))
	}
}

func TestPollDeployJobs_NewRunningJob_NotifiesStart(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create(testutils.WithRepoName("myapp"))
	chat := chatFactory.Create()
	vkUser := vkUserFactory.Create()

	testutils.CreateReleaseSubscription(db, repo, chat, vkUser)
	rule := testutils.CreateDeployTrackingRule(db, "group/ansible", 999, "deploy_prod", repo, chat, vkUser)

	now := time.Now()
	mockJobs := &mocks.MockJobsService{
		ListProjectJobsFunc: func(pid interface{}, opts *gitlab.ListJobsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.Job, *gitlab.Response, error) {
			return []*gitlab.Job{
				{
					ID:        5001,
					Name:      "deploy_prod",
					Status:    "running",
					Ref:       "main",
					WebURL:    "https://gitlab.com/-/jobs/5001",
					CreatedAt: &now,
					StartedAt: &now,
					User:      &gitlab.User{Username: "deployer"},
				},
			}, mocks.NewMockResponse(0), nil
		},
	}

	consumer := NewDeployTrackingConsumerWithDeps(db, mockBot, mockJobs)
	consumer.PollDeployJobs()

	sent := mockBot.GetSentMessages()
	if len(sent) != 1 {
		t.Fatalf("expected 1 message, got %d", len(sent))
	}
	if !strings.Contains(sent[0].Text, "Начат деплой myapp") {
		t.Errorf("expected start notification, got: %s", sent[0].Text)
	}
	if !strings.Contains(sent[0].Text, "deployer") {
		t.Errorf("expected username in notification, got: %s", sent[0].Text)
	}
	if !strings.Contains(sent[0].Text, "main") {
		t.Errorf("expected ref in notification, got: %s", sent[0].Text)
	}

	var tracked models.TrackedDeployJob
	if err := db.Where("gitlab_job_id = ? AND deploy_tracking_rule_id = ?", 5001, rule.ID).First(&tracked).Error; err != nil {
		t.Fatalf("tracked job not created: %v", err)
	}
	if !tracked.NotifiedRunning {
		t.Error("expected NotifiedRunning to be true")
	}
	if tracked.NotifiedFinished {
		t.Error("expected NotifiedFinished to be false")
	}
}

func TestPollDeployJobs_RunningJobCompletesSuccessfully(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create(testutils.WithRepoName("myapp"))
	chat := chatFactory.Create()
	vkUser := vkUserFactory.Create()

	testutils.CreateReleaseSubscription(db, repo, chat, vkUser)
	rule := testutils.CreateDeployTrackingRule(db, "group/ansible", 999, "deploy_prod", repo, chat, vkUser)

	// Pre-create a tracked job that was already notified as running
	tracked := testutils.CreateTrackedDeployJob(db, rule, 5001, "running")
	db.Model(&tracked).Update("notified_running", true)

	now := time.Now()
	mockJobs := &mocks.MockJobsService{
		ListProjectJobsFunc: func(pid interface{}, opts *gitlab.ListJobsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.Job, *gitlab.Response, error) {
			return []*gitlab.Job{
				{
					ID:         5001,
					Name:       "deploy_prod",
					Status:     "success",
					Ref:        "main",
					WebURL:     "https://gitlab.com/-/jobs/5001",
					CreatedAt:  &now,
					FinishedAt: &now,
					User:       &gitlab.User{Username: "deployer"},
				},
			}, mocks.NewMockResponse(0), nil
		},
	}

	consumer := NewDeployTrackingConsumerWithDeps(db, mockBot, mockJobs)
	consumer.PollDeployJobs()

	sent := mockBot.GetSentMessages()
	if len(sent) != 1 {
		t.Fatalf("expected 1 message, got %d", len(sent))
	}
	if !strings.Contains(sent[0].Text, "завершён успешно") {
		t.Errorf("expected success notification, got: %s", sent[0].Text)
	}

	var updated models.TrackedDeployJob
	db.First(&updated, tracked.ID)
	if !updated.NotifiedFinished {
		t.Error("expected NotifiedFinished to be true")
	}
}

func TestPollDeployJobs_FailedJob(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create(testutils.WithRepoName("myapp"))
	chat := chatFactory.Create()
	vkUser := vkUserFactory.Create()

	testutils.CreateReleaseSubscription(db, repo, chat, vkUser)
	rule := testutils.CreateDeployTrackingRule(db, "group/ansible", 999, "deploy_prod", repo, chat, vkUser)

	tracked := testutils.CreateTrackedDeployJob(db, rule, 5002, "running")
	db.Model(&tracked).Update("notified_running", true)

	now := time.Now()
	mockJobs := &mocks.MockJobsService{
		ListProjectJobsFunc: func(pid interface{}, opts *gitlab.ListJobsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.Job, *gitlab.Response, error) {
			return []*gitlab.Job{
				{
					ID:         5002,
					Name:       "deploy_prod",
					Status:     "failed",
					Ref:        "main",
					WebURL:     "https://gitlab.com/-/jobs/5002",
					CreatedAt:  &now,
					FinishedAt: &now,
					User:       &gitlab.User{Username: "deployer"},
				},
			}, mocks.NewMockResponse(0), nil
		},
	}

	consumer := NewDeployTrackingConsumerWithDeps(db, mockBot, mockJobs)
	consumer.PollDeployJobs()

	sent := mockBot.GetSentMessages()
	if len(sent) != 1 {
		t.Fatalf("expected 1 message, got %d", len(sent))
	}
	if !strings.Contains(sent[0].Text, "провален") {
		t.Errorf("expected failed notification, got: %s", sent[0].Text)
	}
}

func TestPollDeployJobs_CanceledJob(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create(testutils.WithRepoName("myapp"))
	chat := chatFactory.Create()
	vkUser := vkUserFactory.Create()

	testutils.CreateReleaseSubscription(db, repo, chat, vkUser)
	rule := testutils.CreateDeployTrackingRule(db, "group/ansible", 999, "deploy_prod", repo, chat, vkUser)

	tracked := testutils.CreateTrackedDeployJob(db, rule, 5003, "running")
	db.Model(&tracked).Update("notified_running", true)

	now := time.Now()
	mockJobs := &mocks.MockJobsService{
		ListProjectJobsFunc: func(pid interface{}, opts *gitlab.ListJobsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.Job, *gitlab.Response, error) {
			return []*gitlab.Job{
				{
					ID:         5003,
					Name:       "deploy_prod",
					Status:     "canceled",
					Ref:        "main",
					WebURL:     "https://gitlab.com/-/jobs/5003",
					CreatedAt:  &now,
					FinishedAt: &now,
					User:       &gitlab.User{Username: "deployer"},
				},
			}, mocks.NewMockResponse(0), nil
		},
	}

	consumer := NewDeployTrackingConsumerWithDeps(db, mockBot, mockJobs)
	consumer.PollDeployJobs()

	sent := mockBot.GetSentMessages()
	if len(sent) != 1 {
		t.Fatalf("expected 1 message, got %d", len(sent))
	}
	if !strings.Contains(sent[0].Text, "отменён") {
		t.Errorf("expected canceled notification, got: %s", sent[0].Text)
	}
}

func TestPollDeployJobs_JobFirstSeenAsTerminal_OnlyFinishNotification(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create(testutils.WithRepoName("myapp"))
	chat := chatFactory.Create()
	vkUser := vkUserFactory.Create()

	testutils.CreateReleaseSubscription(db, repo, chat, vkUser)
	testutils.CreateDeployTrackingRule(db, "group/ansible", 999, "deploy_prod", repo, chat, vkUser)

	now := time.Now()
	mockJobs := &mocks.MockJobsService{
		ListProjectJobsFunc: func(pid interface{}, opts *gitlab.ListJobsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.Job, *gitlab.Response, error) {
			return []*gitlab.Job{
				{
					ID:         5004,
					Name:       "deploy_prod",
					Status:     "success",
					Ref:        "main",
					WebURL:     "https://gitlab.com/-/jobs/5004",
					CreatedAt:  &now,
					FinishedAt: &now,
					User:       &gitlab.User{Username: "deployer"},
				},
			}, mocks.NewMockResponse(0), nil
		},
	}

	consumer := NewDeployTrackingConsumerWithDeps(db, mockBot, mockJobs)
	consumer.PollDeployJobs()

	sent := mockBot.GetSentMessages()
	if len(sent) != 1 {
		t.Fatalf("expected 1 message (only finish), got %d", len(sent))
	}
	if !strings.Contains(sent[0].Text, "завершён успешно") {
		t.Errorf("expected success notification, got: %s", sent[0].Text)
	}
}

func TestPollDeployJobs_DuplicatePoll_NoDoubleNotification(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create(testutils.WithRepoName("myapp"))
	chat := chatFactory.Create()
	vkUser := vkUserFactory.Create()

	testutils.CreateReleaseSubscription(db, repo, chat, vkUser)
	rule := testutils.CreateDeployTrackingRule(db, "group/ansible", 999, "deploy_prod", repo, chat, vkUser)

	// Pre-create fully notified job
	tracked := testutils.CreateTrackedDeployJob(db, rule, 5005, "success")
	db.Model(&tracked).Updates(map[string]interface{}{"notified_running": true, "notified_finished": true})

	now := time.Now()
	mockJobs := &mocks.MockJobsService{
		ListProjectJobsFunc: func(pid interface{}, opts *gitlab.ListJobsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.Job, *gitlab.Response, error) {
			return []*gitlab.Job{
				{
					ID:         5005,
					Name:       "deploy_prod",
					Status:     "success",
					Ref:        "main",
					WebURL:     "https://gitlab.com/-/jobs/5005",
					CreatedAt:  &now,
					FinishedAt: &now,
					User:       &gitlab.User{Username: "deployer"},
				},
			}, mocks.NewMockResponse(0), nil
		},
	}

	consumer := NewDeployTrackingConsumerWithDeps(db, mockBot, mockJobs)
	consumer.PollDeployJobs()

	sent := mockBot.GetSentMessages()
	if len(sent) != 0 {
		t.Errorf("expected no messages for already notified job, got %d", len(sent))
	}
}

func TestPollDeployJobs_NonMatchingJobName_Ignored(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create()
	chat := chatFactory.Create()
	vkUser := vkUserFactory.Create()

	testutils.CreateReleaseSubscription(db, repo, chat, vkUser)
	testutils.CreateDeployTrackingRule(db, "group/ansible", 999, "deploy_prod", repo, chat, vkUser)

	now := time.Now()
	mockJobs := &mocks.MockJobsService{
		ListProjectJobsFunc: func(pid interface{}, opts *gitlab.ListJobsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.Job, *gitlab.Response, error) {
			return []*gitlab.Job{
				{
					ID:        5006,
					Name:      "build",
					Status:    "running",
					Ref:       "main",
					WebURL:    "https://gitlab.com/-/jobs/5006",
					CreatedAt: &now,
					User:      &gitlab.User{Username: "builder"},
				},
			}, mocks.NewMockResponse(0), nil
		},
	}

	consumer := NewDeployTrackingConsumerWithDeps(db, mockBot, mockJobs)
	consumer.PollDeployJobs()

	sent := mockBot.GetSentMessages()
	if len(sent) != 0 {
		t.Errorf("expected no messages for non-matching job, got %d", len(sent))
	}
}

func TestPollDeployJobs_JobCreatedBeforeRule_Ignored(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create()
	chat := chatFactory.Create()
	vkUser := vkUserFactory.Create()

	testutils.CreateReleaseSubscription(db, repo, chat, vkUser)
	testutils.CreateDeployTrackingRule(db, "group/ansible", 999, "deploy_prod", repo, chat, vkUser)

	// Job created an hour before the rule
	oldTime := time.Now().Add(-1 * time.Hour)
	mockJobs := &mocks.MockJobsService{
		ListProjectJobsFunc: func(pid interface{}, opts *gitlab.ListJobsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.Job, *gitlab.Response, error) {
			return []*gitlab.Job{
				{
					ID:        5007,
					Name:      "deploy_prod",
					Status:    "running",
					Ref:       "main",
					WebURL:    "https://gitlab.com/-/jobs/5007",
					CreatedAt: &oldTime,
					User:      &gitlab.User{Username: "deployer"},
				},
			}, mocks.NewMockResponse(0), nil
		},
	}

	consumer := NewDeployTrackingConsumerWithDeps(db, mockBot, mockJobs)
	consumer.PollDeployJobs()

	sent := mockBot.GetSentMessages()
	if len(sent) != 0 {
		t.Errorf("expected no messages for job created before rule, got %d", len(sent))
	}
}

func TestPollDeployJobs_MultipleRulesSameProject_SingleAPICall(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo1 := repoFactory.Create(testutils.WithRepoName("app1"))
	repo2 := repoFactory.Create(testutils.WithRepoName("app2"))
	chat := chatFactory.Create()
	vkUser := vkUserFactory.Create()

	testutils.CreateReleaseSubscription(db, repo1, chat, vkUser)
	testutils.CreateReleaseSubscription(db, repo2, chat, vkUser)
	testutils.CreateDeployTrackingRule(db, "group/ansible", 999, "deploy_app1", repo1, chat, vkUser)
	testutils.CreateDeployTrackingRule(db, "group/ansible", 999, "deploy_app2", repo2, chat, vkUser)

	now := time.Now()
	mockJobs := &mocks.MockJobsService{
		ListProjectJobsFunc: func(pid interface{}, opts *gitlab.ListJobsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.Job, *gitlab.Response, error) {
			return []*gitlab.Job{
				{
					ID:        6001,
					Name:      "deploy_app1",
					Status:    "running",
					Ref:       "main",
					WebURL:    "https://gitlab.com/-/jobs/6001",
					CreatedAt: &now,
					User:      &gitlab.User{Username: "deployer"},
				},
				{
					ID:        6002,
					Name:      "deploy_app2",
					Status:    "success",
					Ref:       "main",
					WebURL:    "https://gitlab.com/-/jobs/6002",
					CreatedAt: &now,
					User:      &gitlab.User{Username: "deployer"},
				},
			}, mocks.NewMockResponse(0), nil
		},
	}

	consumer := NewDeployTrackingConsumerWithDeps(db, mockBot, mockJobs)
	consumer.PollDeployJobs()

	// Should only make 1 API call (grouped by project)
	if len(mockJobs.ListProjectJobsCalls) != 1 {
		t.Errorf("expected 1 API call (grouped), got %d", len(mockJobs.ListProjectJobsCalls))
	}

	sent := mockBot.GetSentMessages()
	if len(sent) != 2 {
		t.Fatalf("expected 2 messages (one per rule), got %d", len(sent))
	}
}

func TestPollDeployJobs_NoReleaseSubscription_NoMessages(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create(testutils.WithRepoName("myapp"))
	chat := chatFactory.Create()
	vkUser := vkUserFactory.Create()

	// No release subscription created
	testutils.CreateDeployTrackingRule(db, "group/ansible", 999, "deploy_prod", repo, chat, vkUser)

	now := time.Now()
	mockJobs := &mocks.MockJobsService{
		ListProjectJobsFunc: func(pid interface{}, opts *gitlab.ListJobsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.Job, *gitlab.Response, error) {
			return []*gitlab.Job{
				{
					ID:        5010,
					Name:      "deploy_prod",
					Status:    "running",
					Ref:       "main",
					WebURL:    "https://gitlab.com/-/jobs/5010",
					CreatedAt: &now,
					User:      &gitlab.User{Username: "deployer"},
				},
			}, mocks.NewMockResponse(0), nil
		},
	}

	consumer := NewDeployTrackingConsumerWithDeps(db, mockBot, mockJobs)
	consumer.PollDeployJobs()

	// Job should be tracked but no messages sent (no subscribers)
	sent := mockBot.GetSentMessages()
	if len(sent) != 0 {
		t.Errorf("expected no messages without release subscription, got %d", len(sent))
	}

	var count int64
	db.Model(&models.TrackedDeployJob{}).Count(&count)
	if count != 1 {
		t.Errorf("expected tracked job to be created, got count %d", count)
	}
}
