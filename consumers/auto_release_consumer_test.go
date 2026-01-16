package consumers

import (
	"errors"
	"strings"
	"testing"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"devstreamlinebot/mocks"
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
	if !strings.Contains(result, "- [!123 Add feature](https://gitlab.com/mr/123) by @alice") {
		t.Errorf("expected result to contain MR entry, got: %s", result)
	}
}

func TestBuildReleaseMRDescription_ExistingSection(t *testing.T) {
	existingDesc := "Some description\n\n---\n## Included MRs\n- [!100 Old MR](https://gitlab.com/mr/100) by @bob"
	mrs := []includedMR{
		{IID: 123, Title: "Add feature", URL: "https://gitlab.com/mr/123", Author: "alice"},
	}

	result := (&AutoReleaseConsumer{}).buildReleaseMRDescription(existingDesc, mrs)

	if !strings.Contains(result, "- [!100 Old MR]") {
		t.Error("expected result to preserve existing MR entry")
	}
	if !strings.Contains(result, "- [!123 Add feature]") {
		t.Error("expected result to contain new MR entry")
	}
}

func TestBuildReleaseMRDescription_DeduplicatesExistingMRs(t *testing.T) {
	existingDesc := "---\n## Included MRs\n- [!123 Old Title](https://gitlab.com/mr/123) by @bob"
	mrs := []includedMR{
		{IID: 123, Title: "Add feature", URL: "https://gitlab.com/mr/123", Author: "alice"},
	}

	result := (&AutoReleaseConsumer{}).buildReleaseMRDescription(existingDesc, mrs)

	// Should not add duplicate !123
	count := strings.Count(result, "[!123")
	if count != 1 {
		t.Errorf("expected exactly 1 occurrence of [!123, got %d", count)
	}
}

// --- ProcessAutoReleaseBranches Tests ---

func TestProcessAutoReleaseBranches_NoConfigs(t *testing.T) {
	db := testutils.SetupTestDB(t)

	mockMRs := &mocks.MockMergeRequestsService{}
	mockBranches := &mocks.MockBranchesService{}

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)
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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)
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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)
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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)
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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)
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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)
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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)
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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)
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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)

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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)
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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)
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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)
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

	if !strings.Contains(updatedDescription, "[!20 Add new feature]") {
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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)
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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)

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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)
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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, &mocks.MockBranchesService{})

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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)

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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, &mocks.MockBranchesService{})

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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)
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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)
	consumer.ProcessReleaseMRDescriptions()

	// Should have called GetMergeRequestCommits twice (pagination)
	if getCommitsCallCount != 2 {
		t.Errorf("expected 2 GetMergeRequestCommits calls (pagination), got %d", getCommitsCallCount)
	}

	// Should have extracted MRs from both pages
	if !strings.Contains(updatedDescription, "[!100") {
		t.Errorf("expected description to contain MR !100 from page 1")
	}
	if !strings.Contains(updatedDescription, "[!101") {
		t.Errorf("expected description to contain MR !101 from page 2")
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

	consumer := NewAutoReleaseConsumerWithServices(db, &mocks.MockMergeRequestsService{}, mockBranches)

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

	consumer := NewAutoReleaseConsumerWithServices(db, &mocks.MockMergeRequestsService{}, mockBranches)

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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)
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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)
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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)
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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)
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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)
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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)
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

	consumer := NewAutoReleaseConsumerWithServices(db, mockMRs, mockBranches)
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
