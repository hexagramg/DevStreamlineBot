package utils

import (
	"testing"

	"devstreamlinebot/models"
	"devstreamlinebot/testutils"
)

func TestIsMRBlockedFromCache_Blocked(t *testing.T) {
	labels := []models.Label{
		{Name: "feature"},
		{Name: "blocked"},
		{Name: "urgent"},
	}
	cache := &MRDataCache{
		BlockLabels: map[uint]map[string]struct{}{
			1: {"blocked": {}, "wip": {}},
		},
	}

	result := cache.IsMRBlockedFromCache(labels, 1)

	if !result {
		t.Error("expected MR to be blocked, got not blocked")
	}
}

func TestIsMRBlockedFromCache_NotBlocked(t *testing.T) {
	labels := []models.Label{
		{Name: "feature"},
		{Name: "ready-for-review"},
		{Name: "urgent"},
	}
	cache := &MRDataCache{
		BlockLabels: map[uint]map[string]struct{}{
			1: {"blocked": {}, "wip": {}},
		},
	}

	result := cache.IsMRBlockedFromCache(labels, 1)

	if result {
		t.Error("expected MR to not be blocked, got blocked")
	}
}

func TestIsMRBlockedFromCache_EmptyLabels(t *testing.T) {
	labels := []models.Label{}
	cache := &MRDataCache{
		BlockLabels: map[uint]map[string]struct{}{
			1: {"blocked": {}, "wip": {}},
		},
	}

	result := cache.IsMRBlockedFromCache(labels, 1)

	if result {
		t.Error("expected MR with no labels to not be blocked, got blocked")
	}
}

func TestIsMRBlockedFromCache_EmptyBlockList(t *testing.T) {
	labels := []models.Label{
		{Name: "feature"},
		{Name: "blocked"},
	}
	cache := &MRDataCache{
		BlockLabels: map[uint]map[string]struct{}{
			1: {},
		},
	}

	result := cache.IsMRBlockedFromCache(labels, 1)

	if result {
		t.Error("expected MR to not be blocked when no block labels configured, got blocked")
	}
}

func TestIsMRBlockedFromCache_NilBlockList(t *testing.T) {
	labels := []models.Label{
		{Name: "feature"},
		{Name: "blocked"},
	}
	cache := &MRDataCache{
		BlockLabels: map[uint]map[string]struct{}{},
	}

	result := cache.IsMRBlockedFromCache(labels, 1)

	if result {
		t.Error("expected MR to not be blocked when block labels map is nil, got blocked")
	}
}

func TestIsMRBlockedFromCache_MultipleBlockingLabels(t *testing.T) {
	labels := []models.Label{
		{Name: "blocked"},
		{Name: "wip"},
	}
	cache := &MRDataCache{
		BlockLabels: map[uint]map[string]struct{}{
			1: {"blocked": {}, "wip": {}},
		},
	}

	result := cache.IsMRBlockedFromCache(labels, 1)

	if !result {
		t.Error("expected MR with multiple blocking labels to be blocked, got not blocked")
	}
}

// --- Release Label Tests ---

func TestHasReleaseLabelFromCache_HasReleaseLabel(t *testing.T) {
	labels := []models.Label{
		{Name: "feature"},
		{Name: "release"},
		{Name: "urgent"},
	}
	cache := &MRDataCache{
		ReleaseLabels: map[uint]map[string]struct{}{
			1: {"release": {}, "released": {}},
		},
	}

	result := cache.HasReleaseLabelFromCache(labels, 1)

	if !result {
		t.Error("expected MR to have release label, got false")
	}
}

func TestHasReleaseLabelFromCache_NoReleaseLabel(t *testing.T) {
	labels := []models.Label{
		{Name: "feature"},
		{Name: "ready-for-review"},
		{Name: "urgent"},
	}
	cache := &MRDataCache{
		ReleaseLabels: map[uint]map[string]struct{}{
			1: {"release": {}, "released": {}},
		},
	}

	result := cache.HasReleaseLabelFromCache(labels, 1)

	if result {
		t.Error("expected MR to not have release label, got true")
	}
}

func TestHasReleaseLabelFromCache_EmptyLabels(t *testing.T) {
	labels := []models.Label{}
	cache := &MRDataCache{
		ReleaseLabels: map[uint]map[string]struct{}{
			1: {"release": {}, "released": {}},
		},
	}

	result := cache.HasReleaseLabelFromCache(labels, 1)

	if result {
		t.Error("expected MR with no labels to not have release label, got true")
	}
}

func TestHasReleaseLabelFromCache_EmptyReleaseList(t *testing.T) {
	labels := []models.Label{
		{Name: "feature"},
		{Name: "release"},
	}
	cache := &MRDataCache{
		ReleaseLabels: map[uint]map[string]struct{}{
			1: {},
		},
	}

	result := cache.HasReleaseLabelFromCache(labels, 1)

	if result {
		t.Error("expected MR to not have release label when no release labels configured, got true")
	}
}

func TestHasReleaseLabelFromCache_NilReleaseList(t *testing.T) {
	labels := []models.Label{
		{Name: "feature"},
		{Name: "release"},
	}
	cache := &MRDataCache{
		ReleaseLabels: map[uint]map[string]struct{}{},
	}

	result := cache.HasReleaseLabelFromCache(labels, 1)

	if result {
		t.Error("expected MR to not have release label when release labels map is nil, got true")
	}
}

func TestHasReleaseLabelFromCache_MultipleReleaseLabels(t *testing.T) {
	labels := []models.Label{
		{Name: "release"},
		{Name: "released"},
	}
	cache := &MRDataCache{
		ReleaseLabels: map[uint]map[string]struct{}{
			1: {"release": {}, "released": {}},
		},
	}

	result := cache.HasReleaseLabelFromCache(labels, 1)

	if !result {
		t.Error("expected MR with multiple release labels to be detected, got false")
	}
}

func TestHasReleaseLabel_MRWithNoLabels(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	// Configure release label for repo
	testutils.CreateReleaseLabel(db, repo, "release")

	result := HasReleaseLabel(db, &mr)

	if result {
		t.Error("expected MR with no labels to return false")
	}
}

func TestHasReleaseLabel_MRWithLabelsNoRelease(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "backend", "feature"))

	// Configure release label for repo
	testutils.CreateReleaseLabel(db, repo, "release")

	result := HasReleaseLabel(db, &mr)

	if result {
		t.Error("expected MR without release label to return false")
	}
}

func TestHasReleaseLabel_MRWithReleaseLabel(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "release"))

	// Configure release label for repo
	testutils.CreateReleaseLabel(db, repo, "release")

	result := HasReleaseLabel(db, &mr)

	if !result {
		t.Error("expected MR with release label to return true")
	}
}

func TestHasReleaseLabel_MRWithMultipleLabelsOneRelease(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "backend", "release", "urgent"))

	// Configure release label for repo
	testutils.CreateReleaseLabel(db, repo, "release")

	result := HasReleaseLabel(db, &mr)

	if !result {
		t.Error("expected MR with release label among multiple to return true")
	}
}

func TestHasReleaseLabel_ReleaseLabelForDifferentRepo(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo1 := repoFactory.Create()
	repo2 := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo1, author, testutils.WithLabels(db, "release"))

	// Configure release label only for repo2, not repo1
	testutils.CreateReleaseLabel(db, repo2, "release")

	result := HasReleaseLabel(db, &mr)

	if result {
		t.Error("expected MR to not match release label from different repo")
	}
}

// --- FindDigestMergeRequestsWithState Release Label Filtering Tests ---

func TestFindDigestMergeRequestsWithState_IncludesNormalMR(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer)

	results, err := FindDigestMergeRequestsWithState(db, []uint{repo.ID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 MR in results, got %d", len(results))
	}
}

func TestFindDigestMergeRequestsWithState_ExcludesReleaseLabeledMR(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	// Create MR with release label
	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "release"))
	testutils.AssignReviewers(db, &mr, reviewer)

	// Configure release label
	testutils.CreateReleaseLabel(db, repo, "release")

	results, err := FindDigestMergeRequestsWithState(db, []uint{repo.ID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 MRs in results (release-labeled excluded), got %d", len(results))
	}
}

func TestFindDigestMergeRequestsWithState_MixedMRs(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	// Normal MR
	mr1 := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr1, reviewer)

	// Release-labeled MR
	mr2 := mrFactory.Create(repo, author, testutils.WithLabels(db, "release"))
	testutils.AssignReviewers(db, &mr2, reviewer)

	// Configure release label
	testutils.CreateReleaseLabel(db, repo, "release")

	results, err := FindDigestMergeRequestsWithState(db, []uint{repo.ID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 MR in results (only normal MR), got %d", len(results))
	}
	if len(results) == 1 && results[0].MR.ID != mr1.ID {
		t.Errorf("expected normal MR in results, got different MR")
	}
}

// --- FindUserActionMRs Release Label Filtering Tests ---

func TestFindUserActionMRs_ExcludesReleaseLabeledReviewerMR(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	// MR where reviewer is assigned, but has release label
	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "release"))
	testutils.AssignReviewers(db, &mr, reviewer)

	// Configure release label
	testutils.CreateReleaseLabel(db, repo, "release")

	reviewMRs, _, _, err := FindUserActionMRs(db, reviewer.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reviewMRs) != 0 {
		t.Errorf("expected 0 review MRs (release-labeled excluded), got %d", len(reviewMRs))
	}
}

func TestFindUserActionMRs_ExcludesReleaseLabeledAuthorMR(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	// MR authored by user with release label and unresolved comment (on_fixes state)
	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "release"))
	testutils.AssignReviewers(db, &mr, reviewer)
	testutils.CreateMRComment(db, mr, reviewer, 1,
		testutils.WithResolvable(),
		testutils.WithThreadStarter(&reviewer),
		testutils.WithIsLastInThread())

	// Configure release label
	testutils.CreateReleaseLabel(db, repo, "release")

	_, fixesMRs, _, err := FindUserActionMRs(db, author.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fixesMRs) != 0 {
		t.Errorf("expected 0 fixes MRs (release-labeled excluded), got %d", len(fixesMRs))
	}
}

func TestFindUserActionMRs_IncludesNormalMRs(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	// Normal MR (no release label)
	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer)

	reviewMRs, _, _, err := FindUserActionMRs(db, reviewer.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reviewMRs) != 1 {
		t.Errorf("expected 1 review MR, got %d", len(reviewMRs))
	}
}

// --- FindReleaseManagerActionMRs Release Label Filtering Tests ---

func TestFindReleaseManagerActionMRs_ExcludesReleaseLabeledMR(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()
	releaseManager := userFactory.Create()

	// Fully approved MR with release label
	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "release"))
	testutils.AssignReviewers(db, &mr, reviewer)
	testutils.AssignApprovers(db, &mr, reviewer)

	// Configure release manager and release label
	testutils.CreateReleaseManager(db, repo, releaseManager)
	testutils.CreateReleaseLabel(db, repo, "release")

	results, err := FindReleaseManagerActionMRs(db, releaseManager.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 MRs (release-labeled excluded), got %d", len(results))
	}
}

func TestFindReleaseManagerActionMRs_IncludesNormalApprovedMR(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()
	releaseManager := userFactory.Create()

	// Fully approved MR without release label
	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer)
	testutils.AssignApprovers(db, &mr, reviewer)

	// Configure release manager
	testutils.CreateReleaseManager(db, repo, releaseManager)

	results, err := FindReleaseManagerActionMRs(db, releaseManager.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 MR, got %d", len(results))
	}
}

// --- Per-User Action Tracking Tests ---

// TestFindUserActionMRs_ReviewerWithUnresolvedCommentsExcluded tests the critical bug scenario:
// Multi-comment thread where reviewer follows up. In GitLab, only thread starter has Resolvable=true.
// OLD query (resolvable=true AND is_last_in_thread=true): 0 matches → reviewer WOULD need action (BUG!)
// NEW query (EXISTS subquery): finds starter with resolvable=true → reviewer excluded (CORRECT!)
func TestFindUserActionMRs_ReviewerWithUnresolvedCommentsExcluded(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	// Create MR and assign reviewer
	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer)

	discussionID := "disc-multi-unresolved"

	// Reviewer starts a thread (only thread starter has Resolvable=true)
	testutils.CreateMRComment(db, mr, reviewer, 1,
		testutils.WithResolvable(),
		testutils.WithDiscussionID(discussionID),
		testutils.WithThreadStarter(&reviewer))
	// Note: No WithIsLastInThread() - defaults to false (not last since there's a reply)

	// Reviewer adds follow-up reply (reply has Resolvable=false in GitLab)
	testutils.CreateMRComment(db, mr, reviewer, 2,
		testutils.WithDiscussionID(discussionID), // Same thread!
		testutils.WithThreadStarter(&reviewer),
		testutils.WithIsLastInThread())

	// Reviewer should NOT appear in reviewMRs because they have unresolved thread awaiting author
	reviewMRs, _, _, err := FindUserActionMRs(db, reviewer.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reviewMRs) != 0 {
		t.Errorf("expected 0 review MRs (multi-comment thread awaiting author), got %d", len(reviewMRs))
	}
}

func TestFindUserActionMRs_ReviewerAfterThreadResolved(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	// Create MR and assign reviewer
	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer)

	// Reviewer creates a resolvable comment that is now resolved (author fixed it)
	testutils.CreateMRComment(db, mr, reviewer, 1, testutils.WithResolvable(), testutils.WithResolved(&author))

	// Reviewer should appear in reviewMRs because their thread was resolved
	reviewMRs, _, _, err := FindUserActionMRs(db, reviewer.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reviewMRs) != 1 {
		t.Errorf("expected 1 review MR (thread resolved, reviewer needs to re-review), got %d", len(reviewMRs))
	}
}

func TestFindUserActionMRs_ReviewerNoCommentsNeedsAction(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	// Create MR and assign reviewer
	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer)

	// No comments by reviewer - they need to review
	reviewMRs, _, _, err := FindUserActionMRs(db, reviewer.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reviewMRs) != 1 {
		t.Errorf("expected 1 review MR (no comments, needs to review), got %d", len(reviewMRs))
	}
}

func TestFindUserActionMRs_ReviewerWithOnlyNonResolvableComments(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	// Create MR and assign reviewer
	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer)

	// Reviewer creates a non-resolvable comment (doesn't affect action tracking)
	testutils.CreateMRComment(db, mr, reviewer, 1)

	// Reviewer should still appear in reviewMRs because non-resolvable comments don't count
	reviewMRs, _, _, err := FindUserActionMRs(db, reviewer.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reviewMRs) != 1 {
		t.Errorf("expected 1 review MR (non-resolvable comments don't block), got %d", len(reviewMRs))
	}
}

func TestFindUserActionMRs_MultipleReviewersIndependent(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer1 := userFactory.Create()
	reviewer2 := userFactory.Create()

	// Create MR and assign both reviewers
	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer1, reviewer2)

	// Reviewer1 creates an unresolved comment
	testutils.CreateMRComment(db, mr, reviewer1, 1,
		testutils.WithResolvable(),
		testutils.WithThreadStarter(&reviewer1),
		testutils.WithIsLastInThread())

	// Reviewer1 should NOT see the MR (has unresolved comment)
	reviewMRs1, _, _, err := FindUserActionMRs(db, reviewer1.ID)
	if err != nil {
		t.Fatalf("unexpected error for reviewer1: %v", err)
	}
	if len(reviewMRs1) != 0 {
		t.Errorf("expected 0 review MRs for reviewer1 (has unresolved comment), got %d", len(reviewMRs1))
	}

	// Reviewer2 should still see the MR (no comments by them)
	reviewMRs2, _, _, err := FindUserActionMRs(db, reviewer2.ID)
	if err != nil {
		t.Fatalf("unexpected error for reviewer2: %v", err)
	}
	if len(reviewMRs2) != 1 {
		t.Errorf("expected 1 review MR for reviewer2 (no comments by them), got %d", len(reviewMRs2))
	}
}

func TestFindUserActionMRs_ReviewerSeesOnFixesMR(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer1 := userFactory.Create()
	reviewer2 := userFactory.Create()

	// Create MR and assign both reviewers
	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer1, reviewer2)

	// Reviewer1 creates an unresolved comment (MR is now in on_fixes state globally)
	testutils.CreateMRComment(db, mr, reviewer1, 1,
		testutils.WithResolvable(),
		testutils.WithThreadStarter(&reviewer1),
		testutils.WithIsLastInThread())

	// Reviewer2 should still see the MR even though global state is on_fixes
	// because they personally have no unresolved comments
	reviewMRs2, _, _, err := FindUserActionMRs(db, reviewer2.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reviewMRs2) != 1 {
		t.Errorf("expected 1 review MR for reviewer2 (global on_fixes but no personal comments), got %d", len(reviewMRs2))
	}
}

func TestFindUserActionMRs_AuthorSeesFixesMR(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	// Create MR with reviewer and unresolved comment
	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer)
	testutils.CreateMRComment(db, mr, reviewer, 1,
		testutils.WithResolvable(),
		testutils.WithThreadStarter(&reviewer),
		testutils.WithIsLastInThread())

	// Author should see MR in fixes
	_, fixesMRs, _, err := FindUserActionMRs(db, author.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fixesMRs) != 1 {
		t.Errorf("expected 1 fixes MR for author, got %d", len(fixesMRs))
	}
}

func TestFindUserActionMRs_ReviewerReReviewCycle(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer)

	// Step 1: New assignment - reviewer needs action
	reviewMRs, _, _, _ := FindUserActionMRs(db, reviewer.ID)
	if len(reviewMRs) != 1 {
		t.Errorf("Step 1: expected 1 review MR (new assignment), got %d", len(reviewMRs))
	}

	// Step 2: Reviewer creates unresolved comment - no longer needs action
	comment := testutils.CreateMRComment(db, mr, reviewer, 1,
		testutils.WithResolvable(),
		testutils.WithThreadStarter(&reviewer),
		testutils.WithIsLastInThread())
	reviewMRs, _, _, _ = FindUserActionMRs(db, reviewer.ID)
	if len(reviewMRs) != 0 {
		t.Errorf("Step 2: expected 0 review MRs (waiting for author), got %d", len(reviewMRs))
	}

	// Step 3: Author resolves comment - reviewer needs action again
	db.Model(&comment).Updates(map[string]interface{}{
		"resolved":       true,
		"resolved_by_id": author.ID,
	})
	reviewMRs, _, _, _ = FindUserActionMRs(db, reviewer.ID)
	if len(reviewMRs) != 1 {
		t.Errorf("Step 3: expected 1 review MR (thread resolved, needs re-review), got %d", len(reviewMRs))
	}

	// Step 4: Reviewer creates another unresolved comment - no longer needs action
	testutils.CreateMRComment(db, mr, reviewer, 2,
		testutils.WithResolvable(),
		testutils.WithThreadStarter(&reviewer),
		testutils.WithIsLastInThread())
	reviewMRs, _, _, _ = FindUserActionMRs(db, reviewer.ID)
	if len(reviewMRs) != 0 {
		t.Errorf("Step 4: expected 0 review MRs (new thread, waiting for author), got %d", len(reviewMRs))
	}
}

// --- Global Digest State Tests ---

func TestFindDigestMergeRequestsWithState_UsesGlobalState(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer1 := userFactory.Create()
	reviewer2 := userFactory.Create()

	// Create MR and assign both reviewers
	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer1, reviewer2)

	// Reviewer1 creates an unresolved comment (global state = on_fixes)
	testutils.CreateMRComment(db, mr, reviewer1, 1,
		testutils.WithResolvable(),
		testutils.WithThreadStarter(&reviewer1),
		testutils.WithIsLastInThread())

	// Global digest should show MR as on_fixes regardless of which reviewer's perspective
	results, err := FindDigestMergeRequestsWithState(db, []uint{repo.ID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 MR in digest, got %d", len(results))
	}

	// The global state should be on_fixes (not per-user)
	if results[0].State != StateOnFixes {
		t.Errorf("expected global state on_fixes, got %s", results[0].State)
	}
}

// --- GetActiveReviewers Tests ---

func TestGetActiveReviewers_NoApprovals(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer1 := userFactory.Create()
	reviewer2 := userFactory.Create()

	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer1, reviewer2)

	result, err := GetActiveReviewers(db, []uint{mr.ID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	activeReviewers := result[mr.ID]
	if len(activeReviewers) != 2 {
		t.Errorf("expected 2 active reviewers, got %d", len(activeReviewers))
	}
}

func TestGetActiveReviewers_SomeApproved(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer1 := userFactory.Create()
	reviewer2 := userFactory.Create()

	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer1, reviewer2)
	testutils.AssignApprovers(db, &mr, reviewer1)

	result, err := GetActiveReviewers(db, []uint{mr.ID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	activeReviewers := result[mr.ID]
	if len(activeReviewers) != 1 {
		t.Errorf("expected 1 active reviewer (reviewer2), got %d", len(activeReviewers))
	}
	if len(activeReviewers) == 1 && activeReviewers[0].ID != reviewer2.ID {
		t.Errorf("expected reviewer2 to be active, got user ID %d", activeReviewers[0].ID)
	}
}

// TestGetActiveReviewers_WithUnresolvedComments tests the critical bug scenario:
// Multi-comment thread where reviewer follows up. In GitLab, only thread starter has Resolvable=true.
// OLD query (resolvable=true AND is_last_in_thread=true): 0 matches → reviewer WOULD be active (BUG!)
// NEW query (EXISTS subquery): finds starter with resolvable=true → reviewer excluded (CORRECT!)
func TestGetActiveReviewers_WithUnresolvedComments(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer1 := userFactory.Create()
	reviewer2 := userFactory.Create()

	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer1, reviewer2)

	discussionID := "disc-active-reviewers"

	// Reviewer1 starts a thread (only thread starter has Resolvable=true)
	testutils.CreateMRComment(db, mr, reviewer1, 1,
		testutils.WithResolvable(),
		testutils.WithDiscussionID(discussionID),
		testutils.WithThreadStarter(&reviewer1))
	// Note: No WithIsLastInThread() - defaults to false (not last since there's a reply)

	// Reviewer1 adds follow-up reply (reply has Resolvable=false in GitLab)
	testutils.CreateMRComment(db, mr, reviewer1, 2,
		testutils.WithDiscussionID(discussionID), // Same thread!
		testutils.WithThreadStarter(&reviewer1),
		testutils.WithIsLastInThread())

	result, err := GetActiveReviewers(db, []uint{mr.ID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	activeReviewers := result[mr.ID]
	if len(activeReviewers) != 1 {
		t.Errorf("expected 1 active reviewer (reviewer2), got %d", len(activeReviewers))
	}
	if len(activeReviewers) == 1 && activeReviewers[0].ID != reviewer2.ID {
		t.Errorf("expected reviewer2 to be active, got user ID %d", activeReviewers[0].ID)
	}
}

func TestGetActiveReviewers_MixedConditions(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer1 := userFactory.Create() // Will approve
	reviewer2 := userFactory.Create() // Will have unresolved comment
	reviewer3 := userFactory.Create() // Will be active

	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer1, reviewer2, reviewer3)
	testutils.AssignApprovers(db, &mr, reviewer1)
	testutils.CreateMRComment(db, mr, reviewer2, 1,
		testutils.WithResolvable(),
		testutils.WithThreadStarter(&reviewer2),
		testutils.WithIsLastInThread())

	result, err := GetActiveReviewers(db, []uint{mr.ID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	activeReviewers := result[mr.ID]
	if len(activeReviewers) != 1 {
		t.Errorf("expected 1 active reviewer (reviewer3), got %d", len(activeReviewers))
	}
	if len(activeReviewers) == 1 && activeReviewers[0].ID != reviewer3.ID {
		t.Errorf("expected reviewer3 to be active, got user ID %d", activeReviewers[0].ID)
	}
}

func TestGetActiveReviewers_AllInactive(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer1 := userFactory.Create()
	reviewer2 := userFactory.Create()

	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer1, reviewer2)
	testutils.AssignApprovers(db, &mr, reviewer1, reviewer2)

	result, err := GetActiveReviewers(db, []uint{mr.ID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	activeReviewers := result[mr.ID]
	if len(activeReviewers) != 0 {
		t.Errorf("expected 0 active reviewers (all approved), got %d", len(activeReviewers))
	}
}

func TestGetActiveReviewers_EmptyInput(t *testing.T) {
	db := testutils.SetupTestDB(t)

	result, err := GetActiveReviewers(db, []uint{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected empty map for empty input, got %d entries", len(result))
	}
}

func TestGetActiveReviewers_MultipleMRs(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer1 := userFactory.Create()
	reviewer2 := userFactory.Create()

	mr1 := mrFactory.Create(repo, author)
	mr2 := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr1, reviewer1, reviewer2)
	testutils.AssignReviewers(db, &mr2, reviewer1, reviewer2)

	// MR1: reviewer1 approved
	testutils.AssignApprovers(db, &mr1, reviewer1)
	// MR2: reviewer2 has unresolved comment
	testutils.CreateMRComment(db, mr2, reviewer2, 1,
		testutils.WithResolvable(),
		testutils.WithThreadStarter(&reviewer2),
		testutils.WithIsLastInThread())

	result, err := GetActiveReviewers(db, []uint{mr1.ID, mr2.ID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mr1Active := result[mr1.ID]
	if len(mr1Active) != 1 {
		t.Errorf("MR1: expected 1 active reviewer, got %d", len(mr1Active))
	}
	if len(mr1Active) == 1 && mr1Active[0].ID != reviewer2.ID {
		t.Errorf("MR1: expected reviewer2 active, got user ID %d", mr1Active[0].ID)
	}

	mr2Active := result[mr2.ID]
	if len(mr2Active) != 1 {
		t.Errorf("MR2: expected 1 active reviewer, got %d", len(mr2Active))
	}
	if len(mr2Active) == 1 && mr2Active[0].ID != reviewer1.ID {
		t.Errorf("MR2: expected reviewer1 active, got user ID %d", mr2Active[0].ID)
	}
}

func TestGetActiveReviewers_ResolvedCommentDoesNotExclude(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer)

	// Reviewer has a resolved comment - should still be active
	testutils.CreateMRComment(db, mr, reviewer, 1, testutils.WithResolvable(), testutils.WithResolved(&author))

	result, err := GetActiveReviewers(db, []uint{mr.ID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	activeReviewers := result[mr.ID]
	if len(activeReviewers) != 1 {
		t.Errorf("expected 1 active reviewer (resolved comment doesn't exclude), got %d", len(activeReviewers))
	}
}

func TestGetActiveReviewers_NonResolvableCommentDoesNotExclude(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer)

	// Reviewer has non-resolvable comment - should still be active
	testutils.CreateMRComment(db, mr, reviewer, 1)

	result, err := GetActiveReviewers(db, []uint{mr.ID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	activeReviewers := result[mr.ID]
	if len(activeReviewers) != 1 {
		t.Errorf("expected 1 active reviewer (non-resolvable comment doesn't exclude), got %d", len(activeReviewers))
	}
}

func TestFindUserActionMRs_ReviewerNeedsActionAfterAuthorReply(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer)

	discussionID := "disc-test-1"

	// Reviewer starts a thread (only thread starter has Resolvable=true)
	testutils.CreateMRComment(db, mr, reviewer, 1,
		testutils.WithResolvable(),
		testutils.WithDiscussionID(discussionID),
		testutils.WithThreadStarter(&reviewer),
		testutils.WithNotLastInThread()) // Not last since author will reply

	// Author replies (becomes last in thread, replies have Resolvable=false)
	testutils.CreateMRComment(db, mr, author, 2,
		testutils.WithDiscussionID(discussionID),
		testutils.WithThreadStarter(&reviewer),
		testutils.WithIsLastInThread())

	// Reviewer should now need action (author replied to their thread)
	reviewMRs, _, _, err := FindUserActionMRs(db, reviewer.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reviewMRs) != 1 {
		t.Errorf("expected 1 review MR (author replied, reviewer needs to re-review), got %d", len(reviewMRs))
	}
}

func TestGetActiveReviewers_AfterAuthorReply(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer)

	discussionID := "disc-test-2"

	// Reviewer starts a thread (only thread starter has Resolvable=true)
	testutils.CreateMRComment(db, mr, reviewer, 1,
		testutils.WithResolvable(),
		testutils.WithDiscussionID(discussionID),
		testutils.WithThreadStarter(&reviewer),
		testutils.WithNotLastInThread()) // Not last since author will reply

	// Author replies (becomes last in thread, replies have Resolvable=false)
	testutils.CreateMRComment(db, mr, author, 2,
		testutils.WithDiscussionID(discussionID),
		testutils.WithThreadStarter(&reviewer),
		testutils.WithIsLastInThread())

	// Reviewer should be active (author replied, waiting for reviewer to re-review)
	result, err := GetActiveReviewers(db, []uint{mr.ID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	activeReviewers := result[mr.ID]
	if len(activeReviewers) != 1 {
		t.Errorf("expected 1 active reviewer (author replied), got %d", len(activeReviewers))
	}
	if len(activeReviewers) == 1 && activeReviewers[0].ID != reviewer.ID {
		t.Errorf("expected reviewer to be active after author reply, got user ID %d", activeReviewers[0].ID)
	}
}

// ============================================================================
// Cache Tests - Verify cache-aware functions produce correct results
// ============================================================================

func TestLoadMRDataCache_LoadsAllData(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "feature", "blocked"))
	testutils.AssignReviewers(db, &mr, reviewer)

	// Create block and release labels
	testutils.CreateBlockLabel(db, repo, "blocked")
	testutils.CreateReleaseLabel(db, repo, "release")

	// Create SLA
	testutils.CreateRepositorySLA(db, repo, 1)

	// Create a comment
	testutils.CreateMRComment(db, mr, reviewer, 1,
		testutils.WithResolvable(),
		testutils.WithThreadStarter(&reviewer),
		testutils.WithIsLastInThread())

	// Load cache
	cache, err := LoadMRDataCache(db, []uint{mr.ID}, []uint{repo.ID})
	if err != nil {
		t.Fatalf("failed to load cache: %v", err)
	}

	// Verify block labels loaded
	if _, ok := cache.BlockLabels[repo.ID]["blocked"]; !ok {
		t.Error("expected block label 'blocked' to be in cache")
	}

	// Verify release labels loaded
	if _, ok := cache.ReleaseLabels[repo.ID]["release"]; !ok {
		t.Error("expected release label 'release' to be in cache")
	}

	// Verify SLA loaded
	if cache.SLAs[repo.ID] == nil {
		t.Error("expected SLA to be in cache")
	}

	// Verify comments loaded
	if len(cache.Comments[mr.ID]) != 1 {
		t.Errorf("expected 1 comment in cache, got %d", len(cache.Comments[mr.ID]))
	}
}

func TestCacheStateMatchesDBState_OnReview(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer)

	// Get state from DB
	dbState := DeriveState(db, &mr)

	// Get state from cache
	cache, _ := LoadMRDataCache(db, []uint{mr.ID}, []uint{repo.ID})
	cacheState := DeriveStateFromCache(&mr, cache)

	if dbState != cacheState {
		t.Errorf("state mismatch: DB=%s, Cache=%s", dbState, cacheState)
	}
	if cacheState != StateOnReview {
		t.Errorf("expected on_review state, got %s", cacheState)
	}
}

func TestCacheStateMatchesDBState_OnFixes(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer)

	// Add unresolved comment to put MR in on_fixes state
	testutils.CreateMRComment(db, mr, reviewer, 1,
		testutils.WithResolvable(),
		testutils.WithThreadStarter(&reviewer),
		testutils.WithIsLastInThread())

	// Get state from DB
	dbState := DeriveState(db, &mr)

	// Get state from cache
	cache, _ := LoadMRDataCache(db, []uint{mr.ID}, []uint{repo.ID})
	cacheState := DeriveStateFromCache(&mr, cache)

	if dbState != cacheState {
		t.Errorf("state mismatch: DB=%s, Cache=%s", dbState, cacheState)
	}
	if cacheState != StateOnFixes {
		t.Errorf("expected on_fixes state, got %s", cacheState)
	}
}

func TestCacheStateMatchesDBState_Draft(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	mr := mrFactory.Create(repo, author, testutils.WithDraft())
	testutils.AssignReviewers(db, &mr, reviewer)

	// Get state from DB
	dbState := DeriveState(db, &mr)

	// Get state from cache
	cache, _ := LoadMRDataCache(db, []uint{mr.ID}, []uint{repo.ID})
	cacheState := DeriveStateFromCache(&mr, cache)

	if dbState != cacheState {
		t.Errorf("state mismatch: DB=%s, Cache=%s", dbState, cacheState)
	}
	if cacheState != StateDraft {
		t.Errorf("expected draft state, got %s", cacheState)
	}
}

func TestCacheStateInfoMatchesDBStateInfo(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer)

	// Add unresolved comment
	testutils.CreateMRComment(db, mr, reviewer, 1,
		testutils.WithResolvable(),
		testutils.WithThreadStarter(&reviewer),
		testutils.WithIsLastInThread())

	// Get state info from DB
	dbStateInfo := GetStateInfo(db, &mr)

	// Get state info from cache
	cache, _ := LoadMRDataCache(db, []uint{mr.ID}, []uint{repo.ID})
	cacheStateInfo := GetStateInfoFromCache(&mr, cache)

	if dbStateInfo.State != cacheStateInfo.State {
		t.Errorf("state mismatch: DB=%s, Cache=%s", dbStateInfo.State, cacheStateInfo.State)
	}

	if dbStateInfo.UnresolvedCount != cacheStateInfo.UnresolvedCount {
		t.Errorf("unresolved count mismatch: DB=%d, Cache=%d", dbStateInfo.UnresolvedCount, cacheStateInfo.UnresolvedCount)
	}
}

func TestCacheHasThreadsAwaitingAuthorMatchesDB(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer)

	// Add unresolved comment from reviewer
	testutils.CreateMRComment(db, mr, reviewer, 1,
		testutils.WithResolvable(),
		testutils.WithThreadStarter(&reviewer),
		testutils.WithIsLastInThread())

	// Check DB version
	dbResult := HasThreadsAwaitingAuthor(db, mr.ID, author.ID)

	// Check cache version
	cache, _ := LoadMRDataCache(db, []uint{mr.ID}, []uint{repo.ID})
	cacheResult := HasThreadsAwaitingAuthorFromCache(mr.ID, author.ID, cache)

	if dbResult != cacheResult {
		t.Errorf("HasThreadsAwaitingAuthor mismatch: DB=%v, Cache=%v", dbResult, cacheResult)
	}
	if !cacheResult {
		t.Error("expected threads awaiting author")
	}
}

func TestCacheHasThreadsAwaitingAuthor_AfterAuthorReply(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer)

	discussionID := "disc-cache-test"

	// Reviewer starts thread
	testutils.CreateMRComment(db, mr, reviewer, 1,
		testutils.WithResolvable(),
		testutils.WithDiscussionID(discussionID),
		testutils.WithThreadStarter(&reviewer),
		testutils.WithNotLastInThread())

	// Author replies (becomes last in thread)
	testutils.CreateMRComment(db, mr, author, 2,
		testutils.WithDiscussionID(discussionID),
		testutils.WithThreadStarter(&reviewer),
		testutils.WithIsLastInThread())

	// Check DB version - should be false since author replied
	dbResult := HasThreadsAwaitingAuthor(db, mr.ID, author.ID)

	// Check cache version
	cache, _ := LoadMRDataCache(db, []uint{mr.ID}, []uint{repo.ID})
	cacheResult := HasThreadsAwaitingAuthorFromCache(mr.ID, author.ID, cache)

	if dbResult != cacheResult {
		t.Errorf("HasThreadsAwaitingAuthor mismatch: DB=%v, Cache=%v", dbResult, cacheResult)
	}
	if cacheResult {
		t.Error("expected no threads awaiting author after author reply")
	}
}

func TestFindUserActionMRs_CacheProducesCorrectResults(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer1 := userFactory.Create()
	reviewer2 := userFactory.Create()

	// MR1: reviewer1 needs action (no comments)
	mr1 := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr1, reviewer1)

	// MR2: reviewer1 waiting for author (has unresolved comment)
	mr2 := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr2, reviewer1)
	testutils.CreateMRComment(db, mr2, reviewer1, 1,
		testutils.WithResolvable(),
		testutils.WithThreadStarter(&reviewer1),
		testutils.WithIsLastInThread())

	// MR3: reviewer2 needs action
	mr3 := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr3, reviewer2)

	// Test reviewer1: should see MR1 (needs review), not MR2 (waiting for author)
	reviewMRs1, _, _, err := FindUserActionMRs(db, reviewer1.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reviewMRs1) != 1 {
		t.Errorf("reviewer1: expected 1 review MR, got %d", len(reviewMRs1))
	}
	if len(reviewMRs1) == 1 && reviewMRs1[0].MR.ID != mr1.ID {
		t.Errorf("reviewer1: expected MR1, got MR ID %d", reviewMRs1[0].MR.ID)
	}

	// Test reviewer2: should see MR3
	reviewMRs2, _, _, err := FindUserActionMRs(db, reviewer2.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reviewMRs2) != 1 {
		t.Errorf("reviewer2: expected 1 review MR, got %d", len(reviewMRs2))
	}
	if len(reviewMRs2) == 1 && reviewMRs2[0].MR.ID != mr3.ID {
		t.Errorf("reviewer2: expected MR3, got MR ID %d", reviewMRs2[0].MR.ID)
	}

	// Test author: should see MR2 in fixes (unresolved comment from reviewer)
	_, fixesMRs, _, err := FindUserActionMRs(db, author.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fixesMRs) != 1 {
		t.Errorf("author: expected 1 fixes MR, got %d", len(fixesMRs))
	}
	if len(fixesMRs) == 1 && fixesMRs[0].MR.ID != mr2.ID {
		t.Errorf("author: expected MR2 in fixes, got MR ID %d", fixesMRs[0].MR.ID)
	}
}

func TestFindUserActionMRs_CacheBlockedAndReleaseLabels(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	// MR1: normal, should be included
	mr1 := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr1, reviewer)

	// MR2: has release label, should be excluded
	mr2 := mrFactory.Create(repo, author, testutils.WithLabels(db, "release"))
	testutils.AssignReviewers(db, &mr2, reviewer)

	// MR3: has block label, should be included but marked as blocked
	mr3 := mrFactory.Create(repo, author, testutils.WithLabels(db, "blocked"))
	testutils.AssignReviewers(db, &mr3, reviewer)

	// Configure labels
	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateBlockLabel(db, repo, "blocked")

	reviewMRs, _, _, err := FindUserActionMRs(db, reviewer.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 2 MRs (mr1 and mr3), excluding mr2 with release label
	if len(reviewMRs) != 2 {
		t.Errorf("expected 2 review MRs (excluding release-labeled), got %d", len(reviewMRs))
	}

	// Check that mr3 is marked as blocked
	var foundBlocked bool
	for _, dmr := range reviewMRs {
		if dmr.MR.ID == mr3.ID {
			if !dmr.Blocked {
				t.Error("expected MR3 to be marked as blocked")
			}
			foundBlocked = true
		}
	}
	if !foundBlocked {
		t.Error("MR3 should be in results")
	}
}

func TestFindUserActionMRs_CacheWithMultipleMRs(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	// Create 10 MRs to verify cache handles batch correctly
	for i := 0; i < 10; i++ {
		mr := mrFactory.Create(repo, author)
		testutils.AssignReviewers(db, &mr, reviewer)
	}

	reviewMRs, _, _, err := FindUserActionMRs(db, reviewer.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reviewMRs) != 10 {
		t.Errorf("expected 10 review MRs, got %d", len(reviewMRs))
	}

	// Verify all have state on_review
	for _, dmr := range reviewMRs {
		if dmr.State != StateOnReview {
			t.Errorf("expected all MRs to be on_review, got %s for MR %d", dmr.State, dmr.MR.ID)
		}
	}
}

func TestCollectUniqueIDs(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo1 := repoFactory.Create()
	repo2 := repoFactory.Create()
	author := userFactory.Create()

	mr1 := mrFactory.Create(repo1, author)
	mr2 := mrFactory.Create(repo1, author)
	mr3 := mrFactory.Create(repo2, author)

	mrs := []models.MergeRequest{mr1, mr2, mr3, mr1} // mr1 is duplicate

	mrIDs, repoIDs := CollectUniqueIDs(mrs)

	if len(mrIDs) != 3 {
		t.Errorf("expected 3 unique MR IDs, got %d", len(mrIDs))
	}

	if len(repoIDs) != 2 {
		t.Errorf("expected 2 unique repo IDs, got %d", len(repoIDs))
	}
}

// TestFindUserActionMRs_FullEndToEnd is a comprehensive end-to-end test that verifies
// FindUserActionMRs correctly handles all scenarios with the cache infrastructure.
func TestFindUserActionMRs_FullEndToEnd(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	// Create repositories
	repo1 := repoFactory.Create(testutils.WithRepoName("repo1"))
	repo2 := repoFactory.Create(testutils.WithRepoName("repo2"))

	// Create users
	alice := userFactory.Create(testutils.WithUsername("alice"))   // Will be author of some MRs
	bob := userFactory.Create(testutils.WithUsername("bob"))       // Reviewer
	charlie := userFactory.Create(testutils.WithUsername("charlie")) // Reviewer
	dave := userFactory.Create(testutils.WithUsername("dave"))     // Author of other MRs

	// Configure labels
	testutils.CreateBlockLabel(db, repo1, "blocked")
	testutils.CreateBlockLabel(db, repo1, "wip")
	testutils.CreateReleaseLabel(db, repo1, "release")
	testutils.CreateReleaseLabel(db, repo2, "release-mr")

	// Configure SLAs
	testutils.CreateRepositorySLA(db, repo1, 2)
	testutils.CreateRepositorySLA(db, repo2, 1)

	// ============================================================================
	// Create MRs with various scenarios
	// ============================================================================

	// MR1: Alice's MR, Bob assigned, no comments -> Bob needs action, Alice on_review
	mr1 := mrFactory.Create(repo1, alice, testutils.WithTitle("MR1: Fresh MR"))
	testutils.AssignReviewers(db, &mr1, bob)

	// MR2: Alice's MR, Bob assigned, Bob has unresolved comment -> Bob waiting, Alice on_fixes
	mr2 := mrFactory.Create(repo1, alice, testutils.WithTitle("MR2: Awaiting fixes"))
	testutils.AssignReviewers(db, &mr2, bob)
	testutils.CreateMRComment(db, mr2, bob, 201,
		testutils.WithResolvable(),
		testutils.WithDiscussionID("disc-mr2-1"),
		testutils.WithThreadStarter(&bob),
		testutils.WithIsLastInThread())

	// MR3: Alice's MR, Bob assigned, Bob commented, Alice replied -> Bob needs action
	mr3 := mrFactory.Create(repo1, alice, testutils.WithTitle("MR3: Author replied"))
	testutils.AssignReviewers(db, &mr3, bob)
	testutils.CreateMRComment(db, mr3, bob, 301,
		testutils.WithResolvable(),
		testutils.WithDiscussionID("disc-mr3-1"),
		testutils.WithThreadStarter(&bob),
		testutils.WithNotLastInThread())
	testutils.CreateMRComment(db, mr3, alice, 302,
		testutils.WithDiscussionID("disc-mr3-1"),
		testutils.WithThreadStarter(&bob),
		testutils.WithIsLastInThread())

	// MR4: Alice's MR, Bob assigned, comment resolved -> Bob needs action (re-review)
	mr4 := mrFactory.Create(repo1, alice, testutils.WithTitle("MR4: Thread resolved"))
	testutils.AssignReviewers(db, &mr4, bob)
	testutils.CreateMRComment(db, mr4, bob, 401,
		testutils.WithResolvable(),
		testutils.WithDiscussionID("disc-mr4-1"),
		testutils.WithThreadStarter(&bob),
		testutils.WithIsLastInThread(),
		testutils.WithResolved(&alice))

	// MR5: Alice's MR, Bob AND Charlie assigned, Bob has comment -> Bob waiting, Charlie needs action
	mr5 := mrFactory.Create(repo1, alice, testutils.WithTitle("MR5: Multiple reviewers"))
	testutils.AssignReviewers(db, &mr5, bob, charlie)
	testutils.CreateMRComment(db, mr5, bob, 501,
		testutils.WithResolvable(),
		testutils.WithDiscussionID("disc-mr5-1"),
		testutils.WithThreadStarter(&bob),
		testutils.WithIsLastInThread())

	// MR6: Alice's MR, draft -> Alice on_fixes (draft counts as fixes)
	mr6 := mrFactory.Create(repo1, alice, testutils.WithTitle("MR6: Draft"), testutils.WithDraft())
	testutils.AssignReviewers(db, &mr6, bob)

	// MR7: Alice's MR with release label -> should be EXCLUDED from all results
	mr7 := mrFactory.Create(repo1, alice,
		testutils.WithTitle("MR7: Release MR"),
		testutils.WithLabels(db, "release"))
	testutils.AssignReviewers(db, &mr7, bob)

	// MR8: Alice's MR with block label -> included but marked as blocked
	mr8 := mrFactory.Create(repo1, alice,
		testutils.WithTitle("MR8: Blocked MR"),
		testutils.WithLabels(db, "blocked"))
	testutils.AssignReviewers(db, &mr8, bob)

	// MR9: Dave's MR in repo2, Charlie assigned -> Charlie needs action
	mr9 := mrFactory.Create(repo2, dave, testutils.WithTitle("MR9: Different repo"))
	testutils.AssignReviewers(db, &mr9, charlie)

	// MR10: Dave's MR with multi-comment thread where Bob follows up
	// Thread: Bob starts -> Bob follows up (NOT awaiting author since author hasn't replied)
	mr10 := mrFactory.Create(repo1, dave, testutils.WithTitle("MR10: Multi-comment thread"))
	testutils.AssignReviewers(db, &mr10, bob)
	testutils.CreateMRComment(db, mr10, bob, 1001,
		testutils.WithResolvable(),
		testutils.WithDiscussionID("disc-mr10-1"),
		testutils.WithThreadStarter(&bob),
		testutils.WithNotLastInThread())
	testutils.CreateMRComment(db, mr10, bob, 1002,
		testutils.WithDiscussionID("disc-mr10-1"),
		testutils.WithThreadStarter(&bob),
		testutils.WithIsLastInThread())

	// MR11: Alice's MR, Bob approved -> Bob should NOT appear (already approved)
	mr11 := mrFactory.Create(repo1, alice, testutils.WithTitle("MR11: Already approved"))
	testutils.AssignReviewers(db, &mr11, bob)
	testutils.AssignApprovers(db, &mr11, bob)

	// MR12: Merged MR -> should NOT appear anywhere
	mr12 := mrFactory.Create(repo1, alice, testutils.WithTitle("MR12: Merged"), testutils.WithMRState("merged"))
	testutils.AssignReviewers(db, &mr12, bob)

	// MR13: Closed MR -> should NOT appear anywhere
	mr13 := mrFactory.Create(repo1, alice, testutils.WithTitle("MR13: Closed"), testutils.WithMRState("closed"))
	testutils.AssignReviewers(db, &mr13, bob)

	// ============================================================================
	// Test Bob's view
	// ============================================================================
	t.Run("Bob's actions", func(t *testing.T) {
		reviewMRs, fixesMRs, authorOnReviewMRs, err := FindUserActionMRs(db, bob.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Bob should see in reviewMRs:
		// - MR1 (fresh, no comments)
		// - MR3 (alice replied)
		// - MR4 (thread resolved)
		// - MR6 (draft - reviewer still assigned, will review when undrafted)
		// - MR8 (blocked but still needs review)
		// Bob should NOT see:
		// - MR2 (waiting for alice to fix)
		// - MR5 (waiting for alice to fix)
		// - MR7 (release label)
		// - MR10 (waiting for dave to fix)
		// - MR11 (already approved)
		// - MR12 (merged)
		// - MR13 (closed)

		reviewMRIDs := make(map[uint]DigestMR)
		for _, dmr := range reviewMRs {
			reviewMRIDs[dmr.MR.ID] = dmr
		}

		expectedReviewMRs := []uint{mr1.ID, mr3.ID, mr4.ID, mr6.ID, mr8.ID}
		if len(reviewMRs) != len(expectedReviewMRs) {
			t.Errorf("Bob reviewMRs: expected %d MRs, got %d", len(expectedReviewMRs), len(reviewMRs))
			for _, dmr := range reviewMRs {
				t.Logf("  Got: MR %d (%s)", dmr.MR.ID, dmr.MR.Title)
			}
		}

		for _, expectedID := range expectedReviewMRs {
			if _, ok := reviewMRIDs[expectedID]; !ok {
				t.Errorf("Bob reviewMRs: expected MR %d to be present", expectedID)
			}
		}

		// Verify MR8 is marked as blocked
		if dmr, ok := reviewMRIDs[mr8.ID]; ok {
			if !dmr.Blocked {
				t.Error("Bob reviewMRs: MR8 should be marked as blocked")
			}
		}

		// Verify states
		if dmr, ok := reviewMRIDs[mr1.ID]; ok {
			if dmr.State != StateOnReview {
				t.Errorf("MR1 state: expected on_review, got %s", dmr.State)
			}
		}

		// Verify MR6 is seen as draft state
		if dmr, ok := reviewMRIDs[mr6.ID]; ok {
			if dmr.State != StateDraft {
				t.Errorf("MR6 state: expected draft, got %s", dmr.State)
			}
		}

		// Bob is not an author, so fixesMRs and authorOnReviewMRs should be empty
		if len(fixesMRs) != 0 {
			t.Errorf("Bob fixesMRs: expected 0, got %d", len(fixesMRs))
		}
		if len(authorOnReviewMRs) != 0 {
			t.Errorf("Bob authorOnReviewMRs: expected 0, got %d", len(authorOnReviewMRs))
		}
	})

	// ============================================================================
	// Test Alice's view (as author)
	// ============================================================================
	t.Run("Alice's actions", func(t *testing.T) {
		reviewMRs, fixesMRs, authorOnReviewMRs, err := FindUserActionMRs(db, alice.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Alice should see in fixesMRs (on_fixes or draft):
		// - MR2 (has unresolved comment from Bob)
		// - MR5 (has unresolved comment from Bob)
		// - MR6 (draft)
		// - MR8 (fresh MR with block label, but on_review not on_fixes - wait, it has no comments)
		// Actually MR8 has no unresolved comments so it's on_review

		// Alice should see in authorOnReviewMRs:
		// - MR1 (on_review, waiting for Bob)
		// - MR3 (on_review after she replied)
		// - MR4 (on_review after resolved)
		// - MR8 (on_review, blocked)
		// - MR11 (on_review, Bob approved)

		// Alice should NOT see:
		// - MR7 (release label)
		// - MR12 (merged)
		// - MR13 (closed)

		fixesMRIDs := make(map[uint]bool)
		for _, dmr := range fixesMRs {
			fixesMRIDs[dmr.MR.ID] = true
		}

		authorOnReviewMRIDs := make(map[uint]bool)
		for _, dmr := range authorOnReviewMRs {
			authorOnReviewMRIDs[dmr.MR.ID] = true
		}

		// Expected fixes: MR2, MR5, MR6
		expectedFixesMRs := []uint{mr2.ID, mr5.ID, mr6.ID}
		if len(fixesMRs) != len(expectedFixesMRs) {
			t.Errorf("Alice fixesMRs: expected %d MRs, got %d", len(expectedFixesMRs), len(fixesMRs))
			for _, dmr := range fixesMRs {
				t.Logf("  Got: MR %d (%s) state=%s", dmr.MR.ID, dmr.MR.Title, dmr.State)
			}
		}

		for _, expectedID := range expectedFixesMRs {
			if !fixesMRIDs[expectedID] {
				t.Errorf("Alice fixesMRs: expected MR %d to be present", expectedID)
			}
		}

		// Expected on_review: MR1, MR3, MR4, MR8, MR11
		expectedOnReviewMRs := []uint{mr1.ID, mr3.ID, mr4.ID, mr8.ID, mr11.ID}
		if len(authorOnReviewMRs) != len(expectedOnReviewMRs) {
			t.Errorf("Alice authorOnReviewMRs: expected %d MRs, got %d", len(expectedOnReviewMRs), len(authorOnReviewMRs))
			for _, dmr := range authorOnReviewMRs {
				t.Logf("  Got: MR %d (%s) state=%s", dmr.MR.ID, dmr.MR.Title, dmr.State)
			}
		}

		for _, expectedID := range expectedOnReviewMRs {
			if !authorOnReviewMRIDs[expectedID] {
				t.Errorf("Alice authorOnReviewMRs: expected MR %d to be present", expectedID)
			}
		}

		// Alice is not a reviewer, so reviewMRs should be empty
		if len(reviewMRs) != 0 {
			t.Errorf("Alice reviewMRs: expected 0, got %d", len(reviewMRs))
		}
	})

	// ============================================================================
	// Test Charlie's view
	// ============================================================================
	t.Run("Charlie's actions", func(t *testing.T) {
		reviewMRs, fixesMRs, authorOnReviewMRs, err := FindUserActionMRs(db, charlie.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Charlie should see in reviewMRs:
		// - MR5 (assigned, no comments by Charlie, even though Bob has comment)
		// - MR9 (fresh MR in repo2)

		reviewMRIDs := make(map[uint]bool)
		for _, dmr := range reviewMRs {
			reviewMRIDs[dmr.MR.ID] = true
		}

		expectedReviewMRs := []uint{mr5.ID, mr9.ID}
		if len(reviewMRs) != len(expectedReviewMRs) {
			t.Errorf("Charlie reviewMRs: expected %d MRs, got %d", len(expectedReviewMRs), len(reviewMRs))
			for _, dmr := range reviewMRs {
				t.Logf("  Got: MR %d (%s)", dmr.MR.ID, dmr.MR.Title)
			}
		}

		for _, expectedID := range expectedReviewMRs {
			if !reviewMRIDs[expectedID] {
				t.Errorf("Charlie reviewMRs: expected MR %d to be present", expectedID)
			}
		}

		// Charlie is not an author
		if len(fixesMRs) != 0 {
			t.Errorf("Charlie fixesMRs: expected 0, got %d", len(fixesMRs))
		}
		if len(authorOnReviewMRs) != 0 {
			t.Errorf("Charlie authorOnReviewMRs: expected 0, got %d", len(authorOnReviewMRs))
		}
	})

	// ============================================================================
	// Test Dave's view (as author)
	// ============================================================================
	t.Run("Dave's actions", func(t *testing.T) {
		reviewMRs, fixesMRs, authorOnReviewMRs, err := FindUserActionMRs(db, dave.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Dave should see in fixesMRs:
		// - MR10 (has unresolved thread from Bob awaiting Dave)

		// Dave should see in authorOnReviewMRs:
		// - MR9 (on_review, no comments)

		fixesMRIDs := make(map[uint]bool)
		for _, dmr := range fixesMRs {
			fixesMRIDs[dmr.MR.ID] = true
		}

		authorOnReviewMRIDs := make(map[uint]bool)
		for _, dmr := range authorOnReviewMRs {
			authorOnReviewMRIDs[dmr.MR.ID] = true
		}

		if len(fixesMRs) != 1 || !fixesMRIDs[mr10.ID] {
			t.Errorf("Dave fixesMRs: expected MR10, got %d MRs", len(fixesMRs))
			for _, dmr := range fixesMRs {
				t.Logf("  Got: MR %d (%s)", dmr.MR.ID, dmr.MR.Title)
			}
		}

		if len(authorOnReviewMRs) != 1 || !authorOnReviewMRIDs[mr9.ID] {
			t.Errorf("Dave authorOnReviewMRs: expected MR9, got %d MRs", len(authorOnReviewMRs))
			for _, dmr := range authorOnReviewMRs {
				t.Logf("  Got: MR %d (%s)", dmr.MR.ID, dmr.MR.Title)
			}
		}

		// Dave is not a reviewer
		if len(reviewMRs) != 0 {
			t.Errorf("Dave reviewMRs: expected 0, got %d", len(reviewMRs))
		}
	})

	// ============================================================================
	// Verify cache is being used correctly by checking state consistency
	// ============================================================================
	t.Run("Cache state consistency", func(t *testing.T) {
		// Load all MRs and verify cache produces same results as DB queries
		allMRs := []models.MergeRequest{mr1, mr2, mr3, mr4, mr5, mr6, mr8, mr9, mr10}

		mrIDs, repoIDs := CollectUniqueIDs(allMRs)
		cache, err := LoadMRDataCache(db, mrIDs, repoIDs)
		if err != nil {
			t.Fatalf("failed to load cache: %v", err)
		}

		for _, mr := range allMRs {
			// Compare DB state with cache state
			dbState := DeriveState(db, &mr)
			cacheState := DeriveStateFromCache(&mr, cache)

			if dbState != cacheState {
				t.Errorf("MR %d (%s): state mismatch DB=%s, Cache=%s",
					mr.ID, mr.Title, dbState, cacheState)
			}

			// Compare HasThreadsAwaitingAuthor
			dbAwaiting := HasThreadsAwaitingAuthor(db, mr.ID, mr.AuthorID)
			cacheAwaiting := HasThreadsAwaitingAuthorFromCache(mr.ID, mr.AuthorID, cache)

			if dbAwaiting != cacheAwaiting {
				t.Errorf("MR %d (%s): HasThreadsAwaitingAuthor mismatch DB=%v, Cache=%v",
					mr.ID, mr.Title, dbAwaiting, cacheAwaiting)
			}
		}
	})

	// ============================================================================
	// Verify release labels are correctly excluded
	// ============================================================================
	t.Run("Release label exclusion", func(t *testing.T) {
		// MR7 has release label and should not appear anywhere
		reviewMRs, fixesMRs, authorOnReviewMRs, _ := FindUserActionMRs(db, bob.ID)

		for _, dmr := range reviewMRs {
			if dmr.MR.ID == mr7.ID {
				t.Error("MR7 (release label) should not appear in Bob's reviewMRs")
			}
		}

		reviewMRs, fixesMRs, authorOnReviewMRs, _ = FindUserActionMRs(db, alice.ID)

		for _, dmr := range fixesMRs {
			if dmr.MR.ID == mr7.ID {
				t.Error("MR7 (release label) should not appear in Alice's fixesMRs")
			}
		}
		for _, dmr := range authorOnReviewMRs {
			if dmr.MR.ID == mr7.ID {
				t.Error("MR7 (release label) should not appear in Alice's authorOnReviewMRs")
			}
		}
	})

	// ============================================================================
	// Verify merged/closed MRs are excluded
	// ============================================================================
	t.Run("Merged/closed exclusion", func(t *testing.T) {
		reviewMRs, _, _, _ := FindUserActionMRs(db, bob.ID)

		for _, dmr := range reviewMRs {
			if dmr.MR.ID == mr12.ID {
				t.Error("MR12 (merged) should not appear in results")
			}
			if dmr.MR.ID == mr13.ID {
				t.Error("MR13 (closed) should not appear in results")
			}
		}
	})
}
