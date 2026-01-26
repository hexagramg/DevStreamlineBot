package consumers

import (
	"errors"
	"strings"
	"testing"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"devstreamlinebot/mocks"
	"devstreamlinebot/models"
	"devstreamlinebot/testutils"
)

// --- Helper Function Tests ---

func TestHasLabel_Found(t *testing.T) {
	labels := gitlab.Labels{"feature", "release", "bug"}
	if !hasLabel(labels, "release") {
		t.Error("expected hasLabel to return true for existing label")
	}
}

func TestHasLabel_NotFound(t *testing.T) {
	labels := gitlab.Labels{"feature", "bug"}
	if hasLabel(labels, "release") {
		t.Error("expected hasLabel to return false for non-existing label")
	}
}

func TestHasAnyLabel_Found(t *testing.T) {
	labels := gitlab.Labels{"feature", "blocked", "bug"}
	targets := map[string]bool{"blocked": true, "hold": true}
	if !hasAnyLabel(labels, targets) {
		t.Error("expected hasAnyLabel to return true when a label matches")
	}
}

func TestHasAnyLabel_NoneFound(t *testing.T) {
	labels := gitlab.Labels{"feature", "bug"}
	targets := map[string]bool{"blocked": true, "hold": true}
	if hasAnyLabel(labels, targets) {
		t.Error("expected hasAnyLabel to return false when no labels match")
	}
}

func TestBuildReleaseMRDescription_EmptyDescription(t *testing.T) {
	mrs := []includedMR{
		{IID: 123, Title: "Add feature", URL: "https://gitlab.com/mr/123", Author: "alice"},
	}

	result := (&AutoReleaseConsumer{}).buildReleaseMRDescription("", mrs)

	if !strings.Contains(result, "---\n## Included MRs") {
		t.Error("expected result to contain section header")
	}
	if !strings.Contains(result, "- [Add feature](https://gitlab.com/mr/123) by @alice") {
		t.Errorf("expected result to contain MR entry, got: %s", result)
	}
}

func TestBuildReleaseMRDescription_ExistingSection(t *testing.T) {
	existingDesc := "Some description\n\n---\n## Included MRs\n- [Old MR](https://gitlab.com/group/project/-/merge_requests/100) by @bob"
	mrs := []includedMR{
		{IID: 123, Title: "Add feature", URL: "https://gitlab.com/group/project/-/merge_requests/123", Author: "alice"},
	}

	result := (&AutoReleaseConsumer{}).buildReleaseMRDescription(existingDesc, mrs)

	if !strings.Contains(result, "- [Old MR]") {
		t.Error("expected result to preserve existing MR entry")
	}
	if !strings.Contains(result, "- [Add feature]") {
		t.Error("expected result to contain new MR entry")
	}
}

func TestBuildReleaseMRDescription_DeduplicatesExistingMRs(t *testing.T) {
	existingDesc := "---\n## Included MRs\n- [Old Title](https://gitlab.com/group/project/-/merge_requests/123) by @bob"
	mrs := []includedMR{
		{IID: 123, Title: "Add feature", URL: "https://gitlab.com/group/project/-/merge_requests/123", Author: "alice"},
	}

	result := (&AutoReleaseConsumer{}).buildReleaseMRDescription(existingDesc, mrs)

	// Should not add duplicate based on URL
	count := strings.Count(result, "merge_requests/123")
	if count != 1 {
		t.Errorf("expected exactly 1 occurrence of MR URL, got %d", count)
	}
}

func TestStripJiraPrefix_WithColonSeparator(t *testing.T) {
	title := "INTDEV-123: Add new feature"
	result := stripJiraPrefix(title, "INTDEV-123")
	if result != "Add new feature" {
		t.Errorf("expected 'Add new feature', got '%s'", result)
	}
}

func TestStripJiraPrefix_WithSpaceSeparator(t *testing.T) {
	title := "INTDEV-123 Add new feature"
	result := stripJiraPrefix(title, "INTDEV-123")
	if result != "Add new feature" {
		t.Errorf("expected 'Add new feature', got '%s'", result)
	}
}

func TestStripJiraPrefix_NoPrefix(t *testing.T) {
	title := "Add new feature"
	result := stripJiraPrefix(title, "INTDEV-123")
	if result != "Add new feature" {
		t.Errorf("expected 'Add new feature', got '%s'", result)
	}
}

func TestStripJiraPrefix_EmptyJiraTaskID(t *testing.T) {
	title := "INTDEV-123: Add new feature"
	result := stripJiraPrefix(title, "")
	if result != "INTDEV-123: Add new feature" {
		t.Errorf("expected original title, got '%s'", result)
	}
}

func TestStripJiraPrefix_CaseInsensitive(t *testing.T) {
	title := "intdev-123: Add new feature"
	result := stripJiraPrefix(title, "INTDEV-123")
	if result != "Add new feature" {
		t.Errorf("expected 'Add new feature', got '%s'", result)
	}
}

func TestBuildReleaseMRDescription_WithJiraTask(t *testing.T) {
	consumer := &AutoReleaseConsumer{jiraBaseURL: "https://jira.example.com"}
	mrs := []includedMR{
		{
			IID:        123,
			Title:      "INTDEV-42405: Добавленые грейды и текущяа зп сотрудника",
			URL:        "https://gitlab.com/group/project/-/merge_requests/123",
			Author:     "p.kukushkin",
			JiraTaskID: "INTDEV-42405",
		},
	}

	result := consumer.buildReleaseMRDescription("", mrs)

	expected := "- [INTDEV-42405](https://jira.example.com/browse/INTDEV-42405) [Добавленые грейды и текущяа зп сотрудника](https://gitlab.com/group/project/-/merge_requests/123) by @p.kukushkin"
	if !strings.Contains(result, expected) {
		t.Errorf("expected result to contain Jira-formatted entry.\nExpected: %s\nGot: %s", expected, result)
	}
}

func TestBuildReleaseMRDescription_WithoutJiraTask(t *testing.T) {
	consumer := &AutoReleaseConsumer{jiraBaseURL: "https://jira.example.com"}
	mrs := []includedMR{
		{
			IID:        123,
			Title:      "Add new feature",
			URL:        "https://gitlab.com/group/project/-/merge_requests/123",
			Author:     "alice",
			JiraTaskID: "",
		},
	}

	result := consumer.buildReleaseMRDescription("", mrs)

	expected := "- [Add new feature](https://gitlab.com/group/project/-/merge_requests/123) by @alice"
	if !strings.Contains(result, expected) {
		t.Errorf("expected result to contain simple entry.\nExpected: %s\nGot: %s", expected, result)
	}
}

func TestBuildReleaseMRDescription_JiraTaskWithoutBaseURL(t *testing.T) {
	consumer := &AutoReleaseConsumer{jiraBaseURL: ""}
	mrs := []includedMR{
		{
			IID:        123,
			Title:      "INTDEV-42405: Add new feature",
			URL:        "https://gitlab.com/group/project/-/merge_requests/123",
			Author:     "alice",
			JiraTaskID: "INTDEV-42405",
		},
	}

	result := consumer.buildReleaseMRDescription("", mrs)

	// Without jiraBaseURL, should fall back to simple format (with original title)
	expected := "- [INTDEV-42405: Add new feature](https://gitlab.com/group/project/-/merge_requests/123) by @alice"
	if !strings.Contains(result, expected) {
		t.Errorf("expected result to contain simple entry when jiraBaseURL is empty.\nExpected: %s\nGot: %s", expected, result)
	}
}

func TestBuildReleaseMRDescription_DeduplicatesOldFormat(t *testing.T) {
	// Test that we can still deduplicate entries in the old [!IID Title] format
	existingDesc := "---\n## Included MRs\n- [!123 Old Title](https://gitlab.com/group/project/-/merge_requests/123) by @bob"
	mrs := []includedMR{
		{IID: 123, Title: "Add feature", URL: "https://gitlab.com/group/project/-/merge_requests/123", Author: "alice"},
	}

	result := (&AutoReleaseConsumer{}).buildReleaseMRDescription(existingDesc, mrs)

	// Should not add duplicate based on URL (even though old format used [!IID])
	count := strings.Count(result, "merge_requests/123")
	if count != 1 {
		t.Errorf("expected exactly 1 occurrence of MR URL, got %d", count)
	}
}

// --- ProcessAutoReleaseBranches Tests ---

func TestProcessAutoReleaseBranches_NoConfigs(t *testing.T) {
	db := testutils.SetupTestDB(t)

	mockMRs := &mocks.MockMergeRequestsService{}
	mockBranches := &mocks.MockBranchesService{}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")
	consumer.ProcessAutoReleaseBranches()

	if len(mockBranches.GetBranchCalls) != 0 {
		t.Errorf("expected no GetBranch calls, got %d", len(mockBranches.GetBranchCalls))
	}
}

func TestProcessAutoReleaseBranches_NoReleaseLabel(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	testutils.CreateAutoReleaseBranchConfig(db, repo, "release", "develop")
	// No release label created

	mockMRs := &mocks.MockMergeRequestsService{}
	mockBranches := &mocks.MockBranchesService{}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")
	consumer.ProcessAutoReleaseBranches()

	if len(mockBranches.GetBranchCalls) != 0 {
		t.Errorf("expected no GetBranch calls when no release label, got %d", len(mockBranches.GetBranchCalls))
	}
}

func TestProcessAutoReleaseBranches_CreatesNewBranchAndMR(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateAutoReleaseBranchConfig(db, repo, "release", "develop")

	mockBranches := &mocks.MockBranchesService{
		GetBranchFunc: func(pid interface{}, branch string, opts ...gitlab.RequestOptionFunc) (*gitlab.Branch, *gitlab.Response, error) {
			return &gitlab.Branch{
				Commit: &gitlab.Commit{ID: "abc123def456"},
			}, mocks.NewMockResponse(0), nil
		},
		CreateBranchFunc: func(pid interface{}, opt *gitlab.CreateBranchOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.Branch, *gitlab.Response, error) {
			return &gitlab.Branch{Name: *opt.Branch}, mocks.NewMockResponse(0), nil
		},
	}

	mockMRs := &mocks.MockMergeRequestsService{
		ListProjectMergeRequestsFunc: func(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error) {
			return []*gitlab.BasicMergeRequest{}, mocks.NewMockResponse(0), nil
		},
		CreateMergeRequestFunc: func(pid interface{}, opt *gitlab.CreateMergeRequestOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
			return &gitlab.MergeRequest{BasicMergeRequest: gitlab.BasicMergeRequest{IID: 1}}, mocks.NewMockResponse(0), nil
		},
	}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")
	consumer.ProcessAutoReleaseBranches()

	if len(mockBranches.CreateBranchCalls) != 1 {
		t.Fatalf("expected 1 CreateBranch call, got %d", len(mockBranches.CreateBranchCalls))
	}

	branchCall := mockBranches.CreateBranchCalls[0]
	if !strings.HasPrefix(*branchCall.Opt.Branch, "release_") {
		t.Errorf("branch name should start with 'release_', got %s", *branchCall.Opt.Branch)
	}

	if len(mockMRs.CreateMergeRequestCalls) != 1 {
		t.Fatalf("expected 1 CreateMergeRequest call, got %d", len(mockMRs.CreateMergeRequestCalls))
	}

	mrCall := mockMRs.CreateMergeRequestCalls[0]
	if *mrCall.Opt.TargetBranch != "develop" {
		t.Errorf("expected target branch 'develop', got %s", *mrCall.Opt.TargetBranch)
	}
}

func TestProcessAutoReleaseBranches_ExistingReleaseMR(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateAutoReleaseBranchConfig(db, repo, "release", "develop")

	mockBranches := &mocks.MockBranchesService{}

	existingReleaseMR := &gitlab.BasicMergeRequest{
		IID:          10,
		SourceBranch: "release_2024-01-01_abc123",
		TargetBranch: "develop",
		Labels:       gitlab.Labels{"release"},
	}

	listCallCount := 0
	mockMRs := &mocks.MockMergeRequestsService{
		ListProjectMergeRequestsFunc: func(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error) {
			listCallCount++
			if listCallCount == 1 {
				// First call: retargetOrphanedMRs lists all open MRs
				return []*gitlab.BasicMergeRequest{existingReleaseMR}, mocks.NewMockResponse(0), nil
			}
			if listCallCount == 2 {
				// Second call: findOpenReleaseMR
				return []*gitlab.BasicMergeRequest{existingReleaseMR}, mocks.NewMockResponse(0), nil
			}
			// Third call: retargetMRsToReleaseBranch
			return []*gitlab.BasicMergeRequest{}, mocks.NewMockResponse(0), nil
		},
	}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")
	consumer.ProcessAutoReleaseBranches()

	// Should not create a new branch since release MR exists
	if len(mockBranches.CreateBranchCalls) != 0 {
		t.Errorf("expected no CreateBranch calls when release MR exists, got %d", len(mockBranches.CreateBranchCalls))
	}

	if len(mockMRs.CreateMergeRequestCalls) != 0 {
		t.Errorf("expected no CreateMergeRequest calls when release MR exists, got %d", len(mockMRs.CreateMergeRequestCalls))
	}
}

func TestProcessAutoReleaseBranches_BranchNamingFormat(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateAutoReleaseBranchConfig(db, repo, "myprefix", "develop")

	mockBranches := &mocks.MockBranchesService{
		GetBranchFunc: func(pid interface{}, branch string, opts ...gitlab.RequestOptionFunc) (*gitlab.Branch, *gitlab.Response, error) {
			return &gitlab.Branch{
				Commit: &gitlab.Commit{ID: "deadbeef1234567890"},
			}, mocks.NewMockResponse(0), nil
		},
		CreateBranchFunc: func(pid interface{}, opt *gitlab.CreateBranchOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.Branch, *gitlab.Response, error) {
			return &gitlab.Branch{Name: *opt.Branch}, mocks.NewMockResponse(0), nil
		},
	}

	mockMRs := &mocks.MockMergeRequestsService{
		ListProjectMergeRequestsFunc: func(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error) {
			return []*gitlab.BasicMergeRequest{}, mocks.NewMockResponse(0), nil
		},
		CreateMergeRequestFunc: func(pid interface{}, opt *gitlab.CreateMergeRequestOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
			return &gitlab.MergeRequest{BasicMergeRequest: gitlab.BasicMergeRequest{IID: 1}}, mocks.NewMockResponse(0), nil
		},
	}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")
	consumer.ProcessAutoReleaseBranches()

	if len(mockBranches.CreateBranchCalls) != 1 {
		t.Fatalf("expected 1 CreateBranch call, got %d", len(mockBranches.CreateBranchCalls))
	}

	branchName := *mockBranches.CreateBranchCalls[0].Opt.Branch
	today := time.Now().Format("2006-01-02")

	// Should be: myprefix_YYYY-MM-DD_deadbe
	expectedPrefix := "myprefix_" + today + "_deadbe"
	if branchName != expectedPrefix {
		t.Errorf("expected branch name %s, got %s", expectedPrefix, branchName)
	}
}

func TestProcessAutoReleaseBranches_RetargetsMRsToReleaseBranch(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateAutoReleaseBranchConfig(db, repo, "release", "develop")

	existingReleaseMR := &gitlab.BasicMergeRequest{
		IID:          10,
		SourceBranch: "release_2024-01-01_abc123",
		TargetBranch: "develop",
		Labels:       gitlab.Labels{"release"},
	}

	featureMR := &gitlab.BasicMergeRequest{
		IID:          20,
		SourceBranch: "feature-branch",
		TargetBranch: "develop",
		Labels:       gitlab.Labels{"feature"},
	}

	listCallCount := 0
	mockMRs := &mocks.MockMergeRequestsService{
		ListProjectMergeRequestsFunc: func(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error) {
			listCallCount++
			if listCallCount == 1 {
				// First call: retargetOrphanedMRs lists all open MRs
				return []*gitlab.BasicMergeRequest{existingReleaseMR, featureMR}, mocks.NewMockResponse(0), nil
			}
			if listCallCount == 2 {
				// Second call: findOpenReleaseMR
				return []*gitlab.BasicMergeRequest{existingReleaseMR}, mocks.NewMockResponse(0), nil
			}
			// Third call: retargetMRsToReleaseBranch finds MRs targeting dev
			return []*gitlab.BasicMergeRequest{featureMR}, mocks.NewMockResponse(0), nil
		},
		UpdateMergeRequestFunc: func(pid interface{}, mergeRequest int, opt *gitlab.UpdateMergeRequestOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
			return &gitlab.MergeRequest{BasicMergeRequest: gitlab.BasicMergeRequest{IID: mergeRequest}}, mocks.NewMockResponse(0), nil
		},
	}

	mockBranches := &mocks.MockBranchesService{}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")
	consumer.ProcessAutoReleaseBranches()

	if len(mockMRs.UpdateMergeRequestCalls) != 1 {
		t.Fatalf("expected 1 UpdateMergeRequest call for retargeting, got %d", len(mockMRs.UpdateMergeRequestCalls))
	}

	updateCall := mockMRs.UpdateMergeRequestCalls[0]
	if updateCall.MergeRequest != 20 {
		t.Errorf("expected MR !20 to be retargeted, got !%d", updateCall.MergeRequest)
	}
	if *updateCall.Opt.TargetBranch != "release_2024-01-01_abc123" {
		t.Errorf("expected target branch 'release_2024-01-01_abc123', got %s", *updateCall.Opt.TargetBranch)
	}
}

func TestProcessAutoReleaseBranches_SkipsBlockedMRs(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateBlockLabel(db, repo, "blocked")
	testutils.CreateAutoReleaseBranchConfig(db, repo, "release", "develop")

	existingReleaseMR := &gitlab.BasicMergeRequest{
		IID:          10,
		SourceBranch: "release_2024-01-01_abc123",
		Labels:       gitlab.Labels{"release"},
	}

	blockedMR := &gitlab.BasicMergeRequest{
		IID:          20,
		SourceBranch: "feature-branch",
		Labels:       gitlab.Labels{"blocked"},
	}

	listCallCount := 0
	mockMRs := &mocks.MockMergeRequestsService{
		ListProjectMergeRequestsFunc: func(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error) {
			listCallCount++
			if listCallCount == 1 {
				return []*gitlab.BasicMergeRequest{existingReleaseMR}, mocks.NewMockResponse(0), nil
			}
			return []*gitlab.BasicMergeRequest{blockedMR}, mocks.NewMockResponse(0), nil
		},
		UpdateMergeRequestFunc: func(pid interface{}, mergeRequest int, opt *gitlab.UpdateMergeRequestOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
			return &gitlab.MergeRequest{BasicMergeRequest: gitlab.BasicMergeRequest{IID: mergeRequest}}, mocks.NewMockResponse(0), nil
		},
	}

	mockBranches := &mocks.MockBranchesService{}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")
	consumer.ProcessAutoReleaseBranches()

	// Should not retarget the blocked MR
	if len(mockMRs.UpdateMergeRequestCalls) != 0 {
		t.Errorf("expected no UpdateMergeRequest calls for blocked MR, got %d", len(mockMRs.UpdateMergeRequestCalls))
	}
}

func TestProcessAutoReleaseBranches_SkipsReleaseMR(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateAutoReleaseBranchConfig(db, repo, "release", "develop")

	existingReleaseMR := &gitlab.BasicMergeRequest{
		IID:          10,
		SourceBranch: "release_2024-01-01_abc123",
		Labels:       gitlab.Labels{"release"},
	}

	listCallCount := 0
	mockMRs := &mocks.MockMergeRequestsService{
		ListProjectMergeRequestsFunc: func(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error) {
			listCallCount++
			if listCallCount == 1 {
				return []*gitlab.BasicMergeRequest{existingReleaseMR}, mocks.NewMockResponse(0), nil
			}
			// Return the release MR itself as targeting dev (shouldn't happen in reality but tests the skip logic)
			return []*gitlab.BasicMergeRequest{existingReleaseMR}, mocks.NewMockResponse(0), nil
		},
		UpdateMergeRequestFunc: func(pid interface{}, mergeRequest int, opt *gitlab.UpdateMergeRequestOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
			return &gitlab.MergeRequest{BasicMergeRequest: gitlab.BasicMergeRequest{IID: mergeRequest}}, mocks.NewMockResponse(0), nil
		},
	}

	mockBranches := &mocks.MockBranchesService{}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")
	consumer.ProcessAutoReleaseBranches()

	// Should not retarget the release MR itself
	if len(mockMRs.UpdateMergeRequestCalls) != 0 {
		t.Errorf("expected no UpdateMergeRequest calls for release MR, got %d", len(mockMRs.UpdateMergeRequestCalls))
	}
}

func TestProcessAutoReleaseBranches_CreateBranchError(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateAutoReleaseBranchConfig(db, repo, "release", "develop")

	mockBranches := &mocks.MockBranchesService{
		GetBranchFunc: func(pid interface{}, branch string, opts ...gitlab.RequestOptionFunc) (*gitlab.Branch, *gitlab.Response, error) {
			return &gitlab.Branch{
				Commit: &gitlab.Commit{ID: "abc123"},
			}, mocks.NewMockResponse(0), nil
		},
		CreateBranchFunc: func(pid interface{}, opt *gitlab.CreateBranchOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.Branch, *gitlab.Response, error) {
			return nil, mocks.NewMockResponse(0), errors.New("branch creation failed")
		},
	}

	mockMRs := &mocks.MockMergeRequestsService{
		ListProjectMergeRequestsFunc: func(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error) {
			return []*gitlab.BasicMergeRequest{}, mocks.NewMockResponse(0), nil
		},
	}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")

	// Should not panic
	consumer.ProcessAutoReleaseBranches()

	// Should not try to create MR after branch creation failed
	if len(mockMRs.CreateMergeRequestCalls) != 0 {
		t.Errorf("expected no CreateMergeRequest calls after branch error, got %d", len(mockMRs.CreateMergeRequestCalls))
	}
}

// --- ProcessReleaseMRDescriptions Tests ---

func TestProcessReleaseMRDescriptions_NoConfigs(t *testing.T) {
	db := testutils.SetupTestDB(t)

	mockMRs := &mocks.MockMergeRequestsService{}
	mockBranches := &mocks.MockBranchesService{}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")
	consumer.ProcessReleaseMRDescriptions()

	if len(mockMRs.ListProjectMergeRequestsCalls) != 0 {
		t.Errorf("expected no API calls, got %d", len(mockMRs.ListProjectMergeRequestsCalls))
	}
}

func TestProcessReleaseMRDescriptions_NoOpenReleaseMR(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateAutoReleaseBranchConfig(db, repo, "release", "develop")

	mockMRs := &mocks.MockMergeRequestsService{
		ListProjectMergeRequestsFunc: func(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error) {
			return []*gitlab.BasicMergeRequest{}, mocks.NewMockResponse(0), nil
		},
	}

	mockBranches := &mocks.MockBranchesService{}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")
	consumer.ProcessReleaseMRDescriptions()

	// Should not try to get commits if no release MR
	if len(mockMRs.GetMergeRequestCommitsCalls) != 0 {
		t.Errorf("expected no GetMergeRequestCommits calls, got %d", len(mockMRs.GetMergeRequestCommitsCalls))
	}
}

func TestProcessReleaseMRDescriptions_AddsMRsToDescription(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateAutoReleaseBranchConfig(db, repo, "release", "develop")

	releaseMR := &gitlab.BasicMergeRequest{
		IID:          10,
		SourceBranch: "release_2024-01-01_abc123",
		Labels:       gitlab.Labels{"release"},
	}

	mergeCommit := &gitlab.Commit{
		ID:      "commit123",
		Message: "Merge branch 'feature' into 'release_2024-01-01_abc123'\n\nSee merge request mygroup/myproject!20",
	}

	includedMR := &gitlab.MergeRequest{
		BasicMergeRequest: gitlab.BasicMergeRequest{
			IID:    20,
			Title:  "Add new feature",
			WebURL: "https://gitlab.com/mygroup/myproject/-/merge_requests/20",
			Author: &gitlab.BasicUser{Username: "alice"},
		},
	}

	fullReleaseMR := &gitlab.MergeRequest{
		BasicMergeRequest: gitlab.BasicMergeRequest{
			IID:         10,
			Description: "Initial release description",
		},
	}

	var updatedDescription string

	mockMRs := &mocks.MockMergeRequestsService{
		ListProjectMergeRequestsFunc: func(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error) {
			return []*gitlab.BasicMergeRequest{releaseMR}, mocks.NewMockResponse(0), nil
		},
		GetMergeRequestCommitsFunc: func(pid interface{}, mergeRequest int, opt *gitlab.GetMergeRequestCommitsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.Commit, *gitlab.Response, error) {
			return []*gitlab.Commit{mergeCommit}, mocks.NewMockResponse(0), nil
		},
		GetMergeRequestFunc: func(pid interface{}, mergeRequest int, opt *gitlab.GetMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
			if mergeRequest == 10 {
				return fullReleaseMR, mocks.NewMockResponse(0), nil
			}
			return includedMR, mocks.NewMockResponse(0), nil
		},
		UpdateMergeRequestFunc: func(pid interface{}, mergeRequest int, opt *gitlab.UpdateMergeRequestOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
			if opt.Description != nil {
				updatedDescription = *opt.Description
			}
			return &gitlab.MergeRequest{BasicMergeRequest: gitlab.BasicMergeRequest{IID: mergeRequest}}, mocks.NewMockResponse(0), nil
		},
	}

	mockBranches := &mocks.MockBranchesService{}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")
	consumer.ProcessReleaseMRDescriptions()

	if len(mockMRs.UpdateMergeRequestCalls) != 1 {
		t.Fatalf("expected 1 UpdateMergeRequest call, got %d", len(mockMRs.UpdateMergeRequestCalls))
	}

	if !strings.Contains(updatedDescription, "Initial release description") {
		t.Error("expected updated description to preserve original content")
	}

	if !strings.Contains(updatedDescription, "## Included MRs") {
		t.Error("expected updated description to contain section header")
	}

	if !strings.Contains(updatedDescription, "[Add new feature]") {
		t.Errorf("expected updated description to contain MR link, got: %s", updatedDescription)
	}

	if !strings.Contains(updatedDescription, "@alice") {
		t.Errorf("expected updated description to contain author, got: %s", updatedDescription)
	}
}

func TestProcessReleaseMRDescriptions_NoMergeCommits(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateAutoReleaseBranchConfig(db, repo, "release", "develop")

	releaseMR := &gitlab.BasicMergeRequest{
		IID:          10,
		SourceBranch: "release_2024-01-01_abc123",
		Labels:       gitlab.Labels{"release"},
	}

	regularCommit := &gitlab.Commit{
		ID:      "commit123",
		Message: "Fix bug in feature",
	}

	fullReleaseMR := &gitlab.MergeRequest{
		BasicMergeRequest: gitlab.BasicMergeRequest{
			IID:         10,
			Description: "Release description",
		},
	}

	mockMRs := &mocks.MockMergeRequestsService{
		ListProjectMergeRequestsFunc: func(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error) {
			return []*gitlab.BasicMergeRequest{releaseMR}, mocks.NewMockResponse(0), nil
		},
		GetMergeRequestCommitsFunc: func(pid interface{}, mergeRequest int, opt *gitlab.GetMergeRequestCommitsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.Commit, *gitlab.Response, error) {
			return []*gitlab.Commit{regularCommit}, mocks.NewMockResponse(0), nil
		},
		GetMergeRequestFunc: func(pid interface{}, mergeRequest int, opt *gitlab.GetMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
			return fullReleaseMR, mocks.NewMockResponse(0), nil
		},
	}

	mockBranches := &mocks.MockBranchesService{}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")
	consumer.ProcessReleaseMRDescriptions()

	// Should not update description when no merge commits found
	if len(mockMRs.UpdateMergeRequestCalls) != 0 {
		t.Errorf("expected no UpdateMergeRequest calls when no merge commits, got %d", len(mockMRs.UpdateMergeRequestCalls))
	}
}

// --- Missing Tests from Plan ---

func TestProcessAutoReleaseBranches_CreateMRError(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateAutoReleaseBranchConfig(db, repo, "release", "develop")

	mockBranches := &mocks.MockBranchesService{
		GetBranchFunc: func(pid interface{}, branch string, opts ...gitlab.RequestOptionFunc) (*gitlab.Branch, *gitlab.Response, error) {
			return &gitlab.Branch{
				Commit: &gitlab.Commit{ID: "abc123def456"},
			}, mocks.NewMockResponse(0), nil
		},
		CreateBranchFunc: func(pid interface{}, opt *gitlab.CreateBranchOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.Branch, *gitlab.Response, error) {
			return &gitlab.Branch{Name: *opt.Branch}, mocks.NewMockResponse(0), nil
		},
	}

	mockMRs := &mocks.MockMergeRequestsService{
		ListProjectMergeRequestsFunc: func(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error) {
			return []*gitlab.BasicMergeRequest{}, mocks.NewMockResponse(0), nil
		},
		CreateMergeRequestFunc: func(pid interface{}, opt *gitlab.CreateMergeRequestOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
			return nil, mocks.NewMockResponse(0), errors.New("MR creation failed")
		},
	}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")

	// Should not panic
	consumer.ProcessAutoReleaseBranches()

	// Branch should be created
	if len(mockBranches.CreateBranchCalls) != 1 {
		t.Errorf("expected 1 CreateBranch call, got %d", len(mockBranches.CreateBranchCalls))
	}

	// MR creation should be attempted
	if len(mockMRs.CreateMergeRequestCalls) != 1 {
		t.Errorf("expected 1 CreateMergeRequest call, got %d", len(mockMRs.CreateMergeRequestCalls))
	}

	// Should not try to retarget MRs after MR creation failed
	if len(mockMRs.UpdateMergeRequestCalls) != 0 {
		t.Errorf("expected no UpdateMergeRequest calls after MR creation error, got %d", len(mockMRs.UpdateMergeRequestCalls))
	}
}

func TestProcessReleaseMRDescriptions_NoCommits(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateAutoReleaseBranchConfig(db, repo, "release", "develop")

	releaseMR := &gitlab.BasicMergeRequest{
		IID:          10,
		SourceBranch: "release_2024-01-01_abc123",
		Labels:       gitlab.Labels{"release"},
	}

	mockMRs := &mocks.MockMergeRequestsService{
		ListProjectMergeRequestsFunc: func(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error) {
			return []*gitlab.BasicMergeRequest{releaseMR}, mocks.NewMockResponse(0), nil
		},
		GetMergeRequestCommitsFunc: func(pid interface{}, mergeRequest int, opt *gitlab.GetMergeRequestCommitsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.Commit, *gitlab.Response, error) {
			return []*gitlab.Commit{}, mocks.NewMockResponse(0), nil
		},
	}

	mockBranches := &mocks.MockBranchesService{}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")
	consumer.ProcessReleaseMRDescriptions()

	// Should not try to get full MR details or update when no commits
	if len(mockMRs.GetMergeRequestCalls) != 0 {
		t.Errorf("expected no GetMergeRequest calls when no commits, got %d", len(mockMRs.GetMergeRequestCalls))
	}
	if len(mockMRs.UpdateMergeRequestCalls) != 0 {
		t.Errorf("expected no UpdateMergeRequest calls when no commits, got %d", len(mockMRs.UpdateMergeRequestCalls))
	}
}

func TestExtractIncludedMRs_ParsesMergeCommits(t *testing.T) {
	db := testutils.SetupTestDB(t)

	commits := []*gitlab.Commit{
		{ID: "c1", Message: "Merge branch 'feature' into 'develop'\n\nSee merge request group/project!123"},
		{ID: "c2", Message: "Merge branch 'bugfix' into 'develop'\n\nSee merge request ns/group/project!456"},
		{ID: "c3", Message: "Normal commit without MR reference"},
		{ID: "c4", Message: "Merge branch 'dup' into 'develop'\n\nSee merge request group/project!123"}, // duplicate
	}

	mrDetails := map[int]*gitlab.MergeRequest{
		123: {BasicMergeRequest: gitlab.BasicMergeRequest{IID: 123, Title: "Feature", WebURL: "https://gitlab.com/mr/123", Author: &gitlab.BasicUser{Username: "alice"}}},
		456: {BasicMergeRequest: gitlab.BasicMergeRequest{IID: 456, Title: "Bugfix", WebURL: "https://gitlab.com/mr/456", Author: &gitlab.BasicUser{Username: "bob"}}},
	}

	mockMRs := &mocks.MockMergeRequestsService{
		GetMergeRequestFunc: func(pid interface{}, mergeRequest int, opt *gitlab.GetMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
			if mr, ok := mrDetails[mergeRequest]; ok {
				return mr, mocks.NewMockResponse(0), nil
			}
			return nil, mocks.NewMockResponse404(), errors.New("not found")
		},
	}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, &mocks.MockBranchesService{}, "")

	result := consumer.extractIncludedMRs(commits, 123)

	// Should extract 2 unique MRs (123 and 456), not 3 (duplicate 123 ignored)
	if len(result) != 2 {
		t.Fatalf("expected 2 unique MRs, got %d", len(result))
	}

	// Verify first MR
	if result[0].IID != 123 {
		t.Errorf("expected first MR IID 123, got %d", result[0].IID)
	}
	if result[0].Author != "alice" {
		t.Errorf("expected first MR author 'alice', got %s", result[0].Author)
	}

	// Verify second MR
	if result[1].IID != 456 {
		t.Errorf("expected second MR IID 456, got %d", result[1].IID)
	}
}

func TestProcessAutoReleaseBranches_GetBranchError(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateAutoReleaseBranchConfig(db, repo, "release", "develop")

	mockBranches := &mocks.MockBranchesService{
		GetBranchFunc: func(pid interface{}, branch string, opts ...gitlab.RequestOptionFunc) (*gitlab.Branch, *gitlab.Response, error) {
			return nil, mocks.NewMockResponse404(), errors.New("branch not found")
		},
	}

	mockMRs := &mocks.MockMergeRequestsService{
		ListProjectMergeRequestsFunc: func(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error) {
			return []*gitlab.BasicMergeRequest{}, mocks.NewMockResponse(0), nil
		},
	}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")

	// Should not panic
	consumer.ProcessAutoReleaseBranches()

	// Should try to get dev branch
	if len(mockBranches.GetBranchCalls) != 1 {
		t.Errorf("expected 1 GetBranch call, got %d", len(mockBranches.GetBranchCalls))
	}

	// Should not try to create branch or MR after GetBranch error
	if len(mockBranches.CreateBranchCalls) != 0 {
		t.Errorf("expected no CreateBranch calls after GetBranch error, got %d", len(mockBranches.CreateBranchCalls))
	}
	if len(mockMRs.CreateMergeRequestCalls) != 0 {
		t.Errorf("expected no CreateMergeRequest calls after GetBranch error, got %d", len(mockMRs.CreateMergeRequestCalls))
	}
}

func TestExtractIncludedMRs_MRWithoutAuthor(t *testing.T) {
	db := testutils.SetupTestDB(t)

	commits := []*gitlab.Commit{
		{ID: "c1", Message: "Merge branch 'feature' into 'develop'\n\nSee merge request group/project!123"},
	}

	mockMRs := &mocks.MockMergeRequestsService{
		GetMergeRequestFunc: func(pid interface{}, mergeRequest int, opt *gitlab.GetMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
			return &gitlab.MergeRequest{
				BasicMergeRequest: gitlab.BasicMergeRequest{
					IID:    123,
					Title:  "Feature without author",
					WebURL: "https://gitlab.com/mr/123",
					Author: nil, // Author is nil
				},
			}, mocks.NewMockResponse(0), nil
		},
	}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, &mocks.MockBranchesService{}, "")

	result := consumer.extractIncludedMRs(commits, 123)

	if len(result) != 1 {
		t.Fatalf("expected 1 MR, got %d", len(result))
	}

	// Author should be empty string when nil
	if result[0].Author != "" {
		t.Errorf("expected empty author for nil Author, got %s", result[0].Author)
	}
}

func TestProcessAutoReleaseBranches_Pagination(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateAutoReleaseBranchConfig(db, repo, "release", "develop")

	existingReleaseMR := &gitlab.BasicMergeRequest{
		IID:          10,
		SourceBranch: "release_2024-01-01_abc123",
		TargetBranch: "develop",
		Labels:       gitlab.Labels{"release"},
	}

	featureMR1 := &gitlab.BasicMergeRequest{IID: 20, SourceBranch: "feature1", TargetBranch: "develop", Labels: gitlab.Labels{}}
	featureMR2 := &gitlab.BasicMergeRequest{IID: 21, SourceBranch: "feature2", TargetBranch: "develop", Labels: gitlab.Labels{}}

	listCallCount := 0
	mockMRs := &mocks.MockMergeRequestsService{
		ListProjectMergeRequestsFunc: func(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error) {
			listCallCount++
			if listCallCount == 1 {
				// First call: retargetOrphanedMRs lists all open MRs
				return []*gitlab.BasicMergeRequest{existingReleaseMR, featureMR1, featureMR2}, mocks.NewMockResponse(0), nil
			}
			if listCallCount == 2 {
				// Second call: findOpenReleaseMR
				return []*gitlab.BasicMergeRequest{existingReleaseMR}, mocks.NewMockResponse(0), nil
			}
			if listCallCount == 3 {
				// Third call: first page of MRs to retarget (retargetMRsToReleaseBranch)
				return []*gitlab.BasicMergeRequest{featureMR1}, mocks.NewMockResponse(2), nil // NextPage=2
			}
			if listCallCount == 4 {
				// Fourth call: second page of MRs to retarget
				return []*gitlab.BasicMergeRequest{featureMR2}, mocks.NewMockResponse(0), nil
			}
			return []*gitlab.BasicMergeRequest{}, mocks.NewMockResponse(0), nil
		},
		UpdateMergeRequestFunc: func(pid interface{}, mergeRequest int, opt *gitlab.UpdateMergeRequestOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
			return &gitlab.MergeRequest{BasicMergeRequest: gitlab.BasicMergeRequest{IID: mergeRequest}}, mocks.NewMockResponse(0), nil
		},
	}

	mockBranches := &mocks.MockBranchesService{}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")
	consumer.ProcessAutoReleaseBranches()

	// Should retarget both MRs from both pages
	if len(mockMRs.UpdateMergeRequestCalls) != 2 {
		t.Errorf("expected 2 UpdateMergeRequest calls (pagination), got %d", len(mockMRs.UpdateMergeRequestCalls))
	}

	// Verify both MRs were retargeted
	retargetedIIDs := make(map[int]bool)
	for _, call := range mockMRs.UpdateMergeRequestCalls {
		retargetedIIDs[call.MergeRequest] = true
	}
	if !retargetedIIDs[20] || !retargetedIIDs[21] {
		t.Errorf("expected MRs 20 and 21 to be retargeted, got %v", retargetedIIDs)
	}
}

func TestProcessReleaseMRDescriptions_CommitsPagination(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateAutoReleaseBranchConfig(db, repo, "release", "develop")

	releaseMR := &gitlab.BasicMergeRequest{
		IID:          10,
		SourceBranch: "release_2024-01-01_abc123",
		Labels:       gitlab.Labels{"release"},
	}

	commit1 := &gitlab.Commit{ID: "c1", Message: "Merge branch 'feature1'\n\nSee merge request group/project!100"}
	commit2 := &gitlab.Commit{ID: "c2", Message: "Merge branch 'feature2'\n\nSee merge request group/project!101"}

	getCommitsCallCount := 0

	var updatedDescription string

	mockMRs := &mocks.MockMergeRequestsService{
		ListProjectMergeRequestsFunc: func(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error) {
			return []*gitlab.BasicMergeRequest{releaseMR}, mocks.NewMockResponse(0), nil
		},
		GetMergeRequestCommitsFunc: func(pid interface{}, mergeRequest int, opt *gitlab.GetMergeRequestCommitsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.Commit, *gitlab.Response, error) {
			getCommitsCallCount++
			if getCommitsCallCount == 1 {
				return []*gitlab.Commit{commit1}, mocks.NewMockResponse(2), nil // NextPage=2
			}
			return []*gitlab.Commit{commit2}, mocks.NewMockResponse(0), nil
		},
		GetMergeRequestFunc: func(pid interface{}, mergeRequest int, opt *gitlab.GetMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
			if mergeRequest == 10 {
				return &gitlab.MergeRequest{BasicMergeRequest: gitlab.BasicMergeRequest{IID: 10, Description: ""}}, mocks.NewMockResponse(0), nil
			}
			return &gitlab.MergeRequest{
				BasicMergeRequest: gitlab.BasicMergeRequest{
					IID:    mergeRequest,
					Title:  "MR Title",
					WebURL: "https://gitlab.com/mr/" + string(rune(mergeRequest)),
					Author: &gitlab.BasicUser{Username: "dev"},
				},
			}, mocks.NewMockResponse(0), nil
		},
		UpdateMergeRequestFunc: func(pid interface{}, mergeRequest int, opt *gitlab.UpdateMergeRequestOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
			if opt.Description != nil {
				updatedDescription = *opt.Description
			}
			return &gitlab.MergeRequest{BasicMergeRequest: gitlab.BasicMergeRequest{IID: mergeRequest}}, mocks.NewMockResponse(0), nil
		},
	}

	mockBranches := &mocks.MockBranchesService{}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")
	consumer.ProcessReleaseMRDescriptions()

	// Should have called GetMergeRequestCommits twice (pagination)
	if getCommitsCallCount != 2 {
		t.Errorf("expected 2 GetMergeRequestCommits calls (pagination), got %d", getCommitsCallCount)
	}

	// Should have extracted MRs from both pages
	if !strings.Contains(updatedDescription, "[MR Title]") {
		t.Errorf("expected description to contain MR from page 1")
	}
	// Both MRs have the same title in this test, just check both entries exist
	count := strings.Count(updatedDescription, "[MR Title]")
	if count != 2 {
		t.Errorf("expected description to contain 2 MR entries, got %d", count)
	}
}

// --- Orphaned MR Retargeting Tests ---

func TestBranchExists_BranchFound(t *testing.T) {
	db := testutils.SetupTestDB(t)

	mockBranches := &mocks.MockBranchesService{
		GetBranchFunc: func(pid interface{}, branch string, opts ...gitlab.RequestOptionFunc) (*gitlab.Branch, *gitlab.Response, error) {
			return &gitlab.Branch{Name: branch}, mocks.NewMockResponse(0), nil
		},
	}

	consumer := NewAutoReleaseConsumerWithServices(db, &mocks.MockMergeRequestsService{}, mockBranches, "")

	if !consumer.branchExists(123, "develop") {
		t.Error("expected branchExists to return true for existing branch")
	}
}

func TestBranchExists_BranchNotFound(t *testing.T) {
	db := testutils.SetupTestDB(t)

	mockBranches := &mocks.MockBranchesService{
		GetBranchFunc: func(pid interface{}, branch string, opts ...gitlab.RequestOptionFunc) (*gitlab.Branch, *gitlab.Response, error) {
			return nil, mocks.NewMockResponse404(), errors.New("branch not found")
		},
	}

	consumer := NewAutoReleaseConsumerWithServices(db, &mocks.MockMergeRequestsService{}, mockBranches, "")

	if consumer.branchExists(123, "deleted-branch") {
		t.Error("expected branchExists to return false for non-existing branch")
	}
}

func TestRetargetOrphanedMRs_RetargetsMRsWithDeletedTargetBranch(t *testing.T) {
	db := testutils.SetupTestDB(t)

	orphanedMR := &gitlab.BasicMergeRequest{
		IID:          20,
		SourceBranch: "feature-branch",
		TargetBranch: "release_2024-01-01_deleted",
		Labels:       gitlab.Labels{"feature"},
	}

	mockMRs := &mocks.MockMergeRequestsService{
		ListProjectMergeRequestsFunc: func(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error) {
			return []*gitlab.BasicMergeRequest{orphanedMR}, mocks.NewMockResponse(0), nil
		},
		UpdateMergeRequestFunc: func(pid interface{}, mergeRequest int, opt *gitlab.UpdateMergeRequestOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
			return &gitlab.MergeRequest{BasicMergeRequest: gitlab.BasicMergeRequest{IID: mergeRequest}}, mocks.NewMockResponse(0), nil
		},
	}

	mockBranches := &mocks.MockBranchesService{
		GetBranchFunc: func(pid interface{}, branch string, opts ...gitlab.RequestOptionFunc) (*gitlab.Branch, *gitlab.Response, error) {
			if branch == "release_2024-01-01_deleted" {
				return nil, mocks.NewMockResponse404(), errors.New("branch not found")
			}
			return &gitlab.Branch{Name: branch}, mocks.NewMockResponse(0), nil
		},
	}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")
	consumer.retargetOrphanedMRs(123, "develop", "release")

	if len(mockMRs.UpdateMergeRequestCalls) != 1 {
		t.Fatalf("expected 1 UpdateMergeRequest call, got %d", len(mockMRs.UpdateMergeRequestCalls))
	}

	updateCall := mockMRs.UpdateMergeRequestCalls[0]
	if updateCall.MergeRequest != 20 {
		t.Errorf("expected MR !20 to be retargeted, got !%d", updateCall.MergeRequest)
	}
	if *updateCall.Opt.TargetBranch != "develop" {
		t.Errorf("expected target branch 'develop', got %s", *updateCall.Opt.TargetBranch)
	}
}

func TestRetargetOrphanedMRs_SkipsMRsAlreadyTargetingDev(t *testing.T) {
	db := testutils.SetupTestDB(t)

	mrTargetingDev := &gitlab.BasicMergeRequest{
		IID:          20,
		SourceBranch: "feature-branch",
		TargetBranch: "develop",
		Labels:       gitlab.Labels{"feature"},
	}

	mockMRs := &mocks.MockMergeRequestsService{
		ListProjectMergeRequestsFunc: func(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error) {
			return []*gitlab.BasicMergeRequest{mrTargetingDev}, mocks.NewMockResponse(0), nil
		},
		UpdateMergeRequestFunc: func(pid interface{}, mergeRequest int, opt *gitlab.UpdateMergeRequestOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
			return &gitlab.MergeRequest{BasicMergeRequest: gitlab.BasicMergeRequest{IID: mergeRequest}}, mocks.NewMockResponse(0), nil
		},
	}

	mockBranches := &mocks.MockBranchesService{}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")
	consumer.retargetOrphanedMRs(123, "develop", "release")

	// Should not retarget MRs already targeting dev
	if len(mockMRs.UpdateMergeRequestCalls) != 0 {
		t.Errorf("expected no UpdateMergeRequest calls for MR targeting dev, got %d", len(mockMRs.UpdateMergeRequestCalls))
	}

	// Should not check branch existence for dev branch
	if len(mockBranches.GetBranchCalls) != 0 {
		t.Errorf("expected no GetBranch calls for MR targeting dev, got %d", len(mockBranches.GetBranchCalls))
	}
}

func TestRetargetOrphanedMRs_SkipsMRsWithReleaseLabel(t *testing.T) {
	db := testutils.SetupTestDB(t)

	releaseMR := &gitlab.BasicMergeRequest{
		IID:          10,
		SourceBranch: "release_2024-01-01_abc123",
		TargetBranch: "main",
		Labels:       gitlab.Labels{"release"},
	}

	mockMRs := &mocks.MockMergeRequestsService{
		ListProjectMergeRequestsFunc: func(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error) {
			return []*gitlab.BasicMergeRequest{releaseMR}, mocks.NewMockResponse(0), nil
		},
		UpdateMergeRequestFunc: func(pid interface{}, mergeRequest int, opt *gitlab.UpdateMergeRequestOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
			return &gitlab.MergeRequest{BasicMergeRequest: gitlab.BasicMergeRequest{IID: mergeRequest}}, mocks.NewMockResponse(0), nil
		},
	}

	mockBranches := &mocks.MockBranchesService{}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")
	consumer.retargetOrphanedMRs(123, "develop", "release")

	// Should not retarget release MRs
	if len(mockMRs.UpdateMergeRequestCalls) != 0 {
		t.Errorf("expected no UpdateMergeRequest calls for release MR, got %d", len(mockMRs.UpdateMergeRequestCalls))
	}
}

func TestRetargetOrphanedMRs_IncludesBlockedMRs(t *testing.T) {
	db := testutils.SetupTestDB(t)

	blockedMR := &gitlab.BasicMergeRequest{
		IID:          20,
		SourceBranch: "feature-branch",
		TargetBranch: "release_2024-01-01_deleted",
		Labels:       gitlab.Labels{"blocked"},
	}

	mockMRs := &mocks.MockMergeRequestsService{
		ListProjectMergeRequestsFunc: func(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error) {
			return []*gitlab.BasicMergeRequest{blockedMR}, mocks.NewMockResponse(0), nil
		},
		UpdateMergeRequestFunc: func(pid interface{}, mergeRequest int, opt *gitlab.UpdateMergeRequestOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
			return &gitlab.MergeRequest{BasicMergeRequest: gitlab.BasicMergeRequest{IID: mergeRequest}}, mocks.NewMockResponse(0), nil
		},
	}

	mockBranches := &mocks.MockBranchesService{
		GetBranchFunc: func(pid interface{}, branch string, opts ...gitlab.RequestOptionFunc) (*gitlab.Branch, *gitlab.Response, error) {
			return nil, mocks.NewMockResponse404(), errors.New("branch not found")
		},
	}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")
	consumer.retargetOrphanedMRs(123, "develop", "release")

	// Should retarget blocked MRs (unlike normal retargeting which skips them)
	if len(mockMRs.UpdateMergeRequestCalls) != 1 {
		t.Fatalf("expected 1 UpdateMergeRequest call for blocked MR, got %d", len(mockMRs.UpdateMergeRequestCalls))
	}

	if mockMRs.UpdateMergeRequestCalls[0].MergeRequest != 20 {
		t.Errorf("expected blocked MR !20 to be retargeted")
	}
}

func TestRetargetOrphanedMRs_CachesBranchExistenceChecks(t *testing.T) {
	db := testutils.SetupTestDB(t)

	// Two MRs targeting the same deleted branch
	mr1 := &gitlab.BasicMergeRequest{
		IID:          20,
		SourceBranch: "feature1",
		TargetBranch: "release_2024-01-01_deleted",
		Labels:       gitlab.Labels{},
	}
	mr2 := &gitlab.BasicMergeRequest{
		IID:          21,
		SourceBranch: "feature2",
		TargetBranch: "release_2024-01-01_deleted",
		Labels:       gitlab.Labels{},
	}

	mockMRs := &mocks.MockMergeRequestsService{
		ListProjectMergeRequestsFunc: func(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error) {
			return []*gitlab.BasicMergeRequest{mr1, mr2}, mocks.NewMockResponse(0), nil
		},
		UpdateMergeRequestFunc: func(pid interface{}, mergeRequest int, opt *gitlab.UpdateMergeRequestOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
			return &gitlab.MergeRequest{BasicMergeRequest: gitlab.BasicMergeRequest{IID: mergeRequest}}, mocks.NewMockResponse(0), nil
		},
	}

	mockBranches := &mocks.MockBranchesService{
		GetBranchFunc: func(pid interface{}, branch string, opts ...gitlab.RequestOptionFunc) (*gitlab.Branch, *gitlab.Response, error) {
			return nil, mocks.NewMockResponse404(), errors.New("branch not found")
		},
	}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")
	consumer.retargetOrphanedMRs(123, "develop", "release")

	// Should only check branch existence once (cached)
	if len(mockBranches.GetBranchCalls) != 1 {
		t.Errorf("expected 1 GetBranch call (cached), got %d", len(mockBranches.GetBranchCalls))
	}

	// Should still retarget both MRs
	if len(mockMRs.UpdateMergeRequestCalls) != 2 {
		t.Errorf("expected 2 UpdateMergeRequest calls, got %d", len(mockMRs.UpdateMergeRequestCalls))
	}
}

func TestRetargetOrphanedMRs_DoesNotRetargetWhenBranchExists(t *testing.T) {
	db := testutils.SetupTestDB(t)

	mrTargetingExistingBranch := &gitlab.BasicMergeRequest{
		IID:          20,
		SourceBranch: "feature-branch",
		TargetBranch: "some-existing-branch",
		Labels:       gitlab.Labels{},
	}

	mockMRs := &mocks.MockMergeRequestsService{
		ListProjectMergeRequestsFunc: func(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error) {
			return []*gitlab.BasicMergeRequest{mrTargetingExistingBranch}, mocks.NewMockResponse(0), nil
		},
		UpdateMergeRequestFunc: func(pid interface{}, mergeRequest int, opt *gitlab.UpdateMergeRequestOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
			return &gitlab.MergeRequest{BasicMergeRequest: gitlab.BasicMergeRequest{IID: mergeRequest}}, mocks.NewMockResponse(0), nil
		},
	}

	mockBranches := &mocks.MockBranchesService{
		GetBranchFunc: func(pid interface{}, branch string, opts ...gitlab.RequestOptionFunc) (*gitlab.Branch, *gitlab.Response, error) {
			return &gitlab.Branch{Name: branch}, mocks.NewMockResponse(0), nil
		},
	}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")
	consumer.retargetOrphanedMRs(123, "develop", "release")

	// Should not retarget MRs targeting existing branches
	if len(mockMRs.UpdateMergeRequestCalls) != 0 {
		t.Errorf("expected no UpdateMergeRequest calls when branch exists, got %d", len(mockMRs.UpdateMergeRequestCalls))
	}
}

func TestProcessAutoReleaseBranches_CallsRetargetOrphanedMRsFirst(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateAutoReleaseBranchConfig(db, repo, "release", "develop")

	existingReleaseMR := &gitlab.BasicMergeRequest{
		IID:          10,
		SourceBranch: "release_2024-01-01_abc123",
		Labels:       gitlab.Labels{"release"},
	}

	orphanedMR := &gitlab.BasicMergeRequest{
		IID:          20,
		SourceBranch: "feature-branch",
		TargetBranch: "release_2024-01-01_deleted",
		Labels:       gitlab.Labels{},
	}

	listCallCount := 0
	mockMRs := &mocks.MockMergeRequestsService{
		ListProjectMergeRequestsFunc: func(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error) {
			listCallCount++
			if listCallCount == 1 {
				// First call: retargetOrphanedMRs lists all open MRs
				return []*gitlab.BasicMergeRequest{orphanedMR, existingReleaseMR}, mocks.NewMockResponse(0), nil
			}
			if listCallCount == 2 {
				// Second call: findOpenReleaseMR
				return []*gitlab.BasicMergeRequest{existingReleaseMR}, mocks.NewMockResponse(0), nil
			}
			// Third call: retargetMRsToReleaseBranch
			return []*gitlab.BasicMergeRequest{}, mocks.NewMockResponse(0), nil
		},
		UpdateMergeRequestFunc: func(pid interface{}, mergeRequest int, opt *gitlab.UpdateMergeRequestOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
			return &gitlab.MergeRequest{BasicMergeRequest: gitlab.BasicMergeRequest{IID: mergeRequest}}, mocks.NewMockResponse(0), nil
		},
	}

	mockBranches := &mocks.MockBranchesService{
		GetBranchFunc: func(pid interface{}, branch string, opts ...gitlab.RequestOptionFunc) (*gitlab.Branch, *gitlab.Response, error) {
			if branch == "release_2024-01-01_deleted" {
				return nil, mocks.NewMockResponse404(), errors.New("branch not found")
			}
			return &gitlab.Branch{Name: branch}, mocks.NewMockResponse(0), nil
		},
	}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches, "")
	consumer.ProcessAutoReleaseBranches()

	// Should retarget the orphaned MR
	if len(mockMRs.UpdateMergeRequestCalls) != 1 {
		t.Fatalf("expected 1 UpdateMergeRequest call, got %d", len(mockMRs.UpdateMergeRequestCalls))
	}

	updateCall := mockMRs.UpdateMergeRequestCalls[0]
	if updateCall.MergeRequest != 20 {
		t.Errorf("expected orphaned MR !20 to be retargeted, got !%d", updateCall.MergeRequest)
	}
	if *updateCall.Opt.TargetBranch != "develop" {
		t.Errorf("expected target branch 'develop', got %s", *updateCall.Opt.TargetBranch)
	}
}

// --- buildJiraPrefixPatternForProject Tests ---

func TestBuildJiraPrefixPattern_NoRepository(t *testing.T) {
	db := testutils.SetupTestDB(t)

	consumer := NewAutoReleaseConsumerWithServices(db, &mocks.MockMergeRequestsService{}, &mocks.MockBranchesService{}, "")

	// Non-existent GitLab project ID
	result := consumer.buildJiraPrefixPatternForProject(99999)

	if result != nil {
		t.Errorf("expected nil pattern when repository not found, got %v", result)
	}
}

func TestBuildJiraPrefixPattern_NoPrefixesConfigured(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	// No Jira prefixes created

	consumer := NewAutoReleaseConsumerWithServices(db, &mocks.MockMergeRequestsService{}, &mocks.MockBranchesService{}, "")

	result := consumer.buildJiraPrefixPatternForProject(repo.GitlabID)

	if result != nil {
		t.Errorf("expected nil pattern when no prefixes configured, got %v", result)
	}
}

func TestBuildJiraPrefixPattern_SinglePrefix(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	testutils.CreateJiraProjectPrefix(db, repo, "INTDEV")

	consumer := NewAutoReleaseConsumerWithServices(db, &mocks.MockMergeRequestsService{}, &mocks.MockBranchesService{}, "")

	result := consumer.buildJiraPrefixPatternForProject(repo.GitlabID)

	if result == nil {
		t.Fatal("expected non-nil pattern for single prefix")
	}

	// Pattern should match INTDEV-39577
	if !result.MatchString("INTDEV-39577") {
		t.Error("pattern should match INTDEV-39577")
	}

	// Should not match invalid format
	if result.MatchString("INTDEV39577") {
		t.Error("pattern should not match INTDEV39577 (missing hyphen)")
	}

	if result.MatchString("OTHER-123") {
		t.Error("pattern should not match OTHER-123 (wrong prefix)")
	}
}

func TestBuildJiraPrefixPattern_MultiplePrefixes(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	testutils.CreateJiraProjectPrefix(db, repo, "INTDEV")
	testutils.CreateJiraProjectPrefix(db, repo, "PROJ")
	testutils.CreateJiraProjectPrefix(db, repo, "TASK")

	consumer := NewAutoReleaseConsumerWithServices(db, &mocks.MockMergeRequestsService{}, &mocks.MockBranchesService{}, "")

	result := consumer.buildJiraPrefixPatternForProject(repo.GitlabID)

	if result == nil {
		t.Fatal("expected non-nil pattern for multiple prefixes")
	}

	// Should match any of the configured prefixes
	if !result.MatchString("INTDEV-123") {
		t.Error("pattern should match INTDEV-123")
	}

	if !result.MatchString("PROJ-456") {
		t.Error("pattern should match PROJ-456")
	}

	if !result.MatchString("TASK-789") {
		t.Error("pattern should match TASK-789")
	}

	// Should not match unconfigured prefix
	if result.MatchString("OTHER-123") {
		t.Error("pattern should not match OTHER-123 (unconfigured prefix)")
	}
}

func TestBuildJiraPrefixPattern_CaseInsensitive(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	testutils.CreateJiraProjectPrefix(db, repo, "INTDEV")

	consumer := NewAutoReleaseConsumerWithServices(db, &mocks.MockMergeRequestsService{}, &mocks.MockBranchesService{}, "")

	result := consumer.buildJiraPrefixPatternForProject(repo.GitlabID)

	if result == nil {
		t.Fatal("expected non-nil pattern")
	}

	// Prefix configured as INTDEV should match lowercase
	if !result.MatchString("intdev-123") {
		t.Error("pattern should match lowercase: intdev-123")
	}

	// Should match mixed case
	if !result.MatchString("Intdev-456") {
		t.Error("pattern should match mixed case: Intdev-456")
	}

	if !result.MatchString("IntDev-789") {
		t.Error("pattern should match mixed case: IntDev-789")
	}

	// Should still match uppercase
	if !result.MatchString("INTDEV-000") {
		t.Error("pattern should match uppercase: INTDEV-000")
	}
}

func TestBuildJiraPrefixPattern_CaseInsensitive_LowercaseConfig(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	// Configure with lowercase prefix
	testutils.CreateJiraProjectPrefix(db, repo, "intdev")

	consumer := NewAutoReleaseConsumerWithServices(db, &mocks.MockMergeRequestsService{}, &mocks.MockBranchesService{}, "")

	result := consumer.buildJiraPrefixPatternForProject(repo.GitlabID)

	if result == nil {
		t.Fatal("expected non-nil pattern")
	}

	// Lowercase config should match uppercase input
	if !result.MatchString("INTDEV-789") {
		t.Error("lowercase config 'intdev' should match uppercase: INTDEV-789")
	}

	// Should also match lowercase
	if !result.MatchString("intdev-123") {
		t.Error("lowercase config should match lowercase: intdev-123")
	}
}

func TestBuildJiraPrefixPattern_ExtractsFromTitle(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	testutils.CreateJiraProjectPrefix(db, repo, "INTDEV")

	consumer := NewAutoReleaseConsumerWithServices(db, &mocks.MockMergeRequestsService{}, &mocks.MockBranchesService{}, "")

	result := consumer.buildJiraPrefixPatternForProject(repo.GitlabID)

	if result == nil {
		t.Fatal("expected non-nil pattern")
	}

	// Test extraction from real MR title format
	title := "INTDEV-39577 offer: ограничение на сохранение офферов которые содержат невалидные позиции"
	match := result.FindString(title)

	if match == "" {
		t.Error("pattern should find match in title")
	}

	// Should extract the full Jira ID with case-insensitive matching
	upperMatch := strings.ToUpper(match)
	if upperMatch != "INTDEV-39577" {
		t.Errorf("expected to extract INTDEV-39577, got %s", match)
	}
}

func TestBuildJiraPrefixPattern_ExtractsFromTitleWithColon(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	testutils.CreateJiraProjectPrefix(db, repo, "INTDEV")

	consumer := NewAutoReleaseConsumerWithServices(db, &mocks.MockMergeRequestsService{}, &mocks.MockBranchesService{}, "")

	result := consumer.buildJiraPrefixPatternForProject(repo.GitlabID)

	if result == nil {
		t.Fatal("expected non-nil pattern")
	}

	// Test extraction from title with colon separator
	title := "INTDEV-42405: Добавленые грейды и текущяа зп сотрудника"
	match := result.FindString(title)

	upperMatch := strings.ToUpper(match)
	if upperMatch != "INTDEV-42405" {
		t.Errorf("expected to extract INTDEV-42405, got %s", match)
	}
}

func TestBuildJiraPrefixPattern_SpecialCharactersInPrefix(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	// Prefix with characters that need escaping in regex
	testutils.CreateJiraProjectPrefix(db, repo, "INT.DEV")

	consumer := NewAutoReleaseConsumerWithServices(db, &mocks.MockMergeRequestsService{}, &mocks.MockBranchesService{}, "")

	result := consumer.buildJiraPrefixPatternForProject(repo.GitlabID)

	if result == nil {
		t.Fatal("expected non-nil pattern")
	}

	// Should match literal INT.DEV, not INT + any char + DEV
	if !result.MatchString("INT.DEV-123") {
		t.Error("pattern should match INT.DEV-123")
	}

	// Should NOT match INTXDEV (where . matches any char)
	if result.MatchString("INTXDEV-123") {
		t.Error("pattern should NOT match INTXDEV-123 (dot should be escaped)")
	}
}

// --- Integration Test for extractIncludedMRs with Jira extraction ---

func TestExtractIncludedMRs_ExtractsJiraFromTitleWhenDBEmpty(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	author := userFactory.Create(testutils.WithUsername("alice"))
	testutils.CreateJiraProjectPrefix(db, repo, "INTDEV")

	// Create local MR with empty JiraTaskID
	localMR := mrFactory.Create(repo, author,
		testutils.WithMRGitlabID(50000),
		testutils.WithTitle("INTDEV-39577 offer: ограничение на сохранение офферов"),
	)

	// Verify initial state - JiraTaskID should be empty
	var initialMR models.MergeRequest
	db.First(&initialMR, localMR.ID)
	if initialMR.JiraTaskID != "" {
		t.Fatalf("expected empty JiraTaskID initially, got %s", initialMR.JiraTaskID)
	}

	commits := []*gitlab.Commit{
		{ID: "c1", Message: "Merge branch 'feature' into 'develop'\n\nSee merge request group/project!1"},
	}

	mockMRs := &mocks.MockMergeRequestsService{
		GetMergeRequestFunc: func(pid interface{}, mergeRequest int, opt *gitlab.GetMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
			return &gitlab.MergeRequest{
				BasicMergeRequest: gitlab.BasicMergeRequest{
					ID:     50000, // GitlabID matches local MR
					IID:    1,
					Title:  "INTDEV-39577 offer: ограничение на сохранение офферов",
					WebURL: "https://gitlab.com/mr/1",
					Author: &gitlab.BasicUser{Username: "alice"},
				},
			}, mocks.NewMockResponse(0), nil
		},
	}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, &mocks.MockBranchesService{}, "https://jira.example.com")

	result := consumer.extractIncludedMRs(commits, repo.GitlabID)

	if len(result) != 1 {
		t.Fatalf("expected 1 MR, got %d", len(result))
	}

	// Check that JiraTaskID was extracted and returned
	if result[0].JiraTaskID != "INTDEV-39577" {
		t.Errorf("expected JiraTaskID 'INTDEV-39577', got '%s'", result[0].JiraTaskID)
	}

	// Check that JiraTaskID was persisted to DB
	var updatedMR models.MergeRequest
	db.First(&updatedMR, localMR.ID)
	if updatedMR.JiraTaskID != "INTDEV-39577" {
		t.Errorf("expected DB JiraTaskID 'INTDEV-39577', got '%s'", updatedMR.JiraTaskID)
	}
}

func TestExtractIncludedMRs_DoesNotOverwriteExistingJiraTaskID(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create(testutils.WithRepoGitlabID(123))
	author := userFactory.Create(testutils.WithUsername("alice"))
	testutils.CreateJiraProjectPrefix(db, repo, "INTDEV")

	// Create local MR with existing JiraTaskID
	localMR := mrFactory.Create(repo, author,
		testutils.WithMRGitlabID(50000),
		testutils.WithTitle("INTDEV-99999 some other task"),
	)
	db.Model(&localMR).Update("jira_task_id", "INTDEV-11111")

	commits := []*gitlab.Commit{
		{ID: "c1", Message: "Merge branch 'feature' into 'develop'\n\nSee merge request group/project!1"},
	}

	mockMRs := &mocks.MockMergeRequestsService{
		GetMergeRequestFunc: func(pid interface{}, mergeRequest int, opt *gitlab.GetMergeRequestsOptions, opts ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
			return &gitlab.MergeRequest{
				BasicMergeRequest: gitlab.BasicMergeRequest{
					ID:     50000,
					IID:    1,
					Title:  "INTDEV-99999 some other task",
					WebURL: "https://gitlab.com/mr/1",
					Author: &gitlab.BasicUser{Username: "alice"},
				},
			}, mocks.NewMockResponse(0), nil
		},
	}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, &mocks.MockBranchesService{}, "https://jira.example.com")

	result := consumer.extractIncludedMRs(commits, repo.GitlabID)

	if len(result) != 1 {
		t.Fatalf("expected 1 MR, got %d", len(result))
	}

	// Should use existing JiraTaskID from DB, not extract from title
	if result[0].JiraTaskID != "INTDEV-11111" {
		t.Errorf("expected JiraTaskID 'INTDEV-11111' (from DB), got '%s'", result[0].JiraTaskID)
	}

	// DB should still have original value
	var updatedMR models.MergeRequest
	db.First(&updatedMR, localMR.ID)
	if updatedMR.JiraTaskID != "INTDEV-11111" {
		t.Errorf("expected DB JiraTaskID 'INTDEV-11111' (unchanged), got '%s'", updatedMR.JiraTaskID)
	}
}
