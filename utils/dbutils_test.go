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
	blockLabels := map[string]struct{}{
		"blocked": {},
		"wip":     {},
	}

	result := isMRBlockedFromCache(labels, blockLabels)

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
	blockLabels := map[string]struct{}{
		"blocked": {},
		"wip":     {},
	}

	result := isMRBlockedFromCache(labels, blockLabels)

	if result {
		t.Error("expected MR to not be blocked, got blocked")
	}
}

func TestIsMRBlockedFromCache_EmptyLabels(t *testing.T) {
	labels := []models.Label{}
	blockLabels := map[string]struct{}{
		"blocked": {},
		"wip":     {},
	}

	result := isMRBlockedFromCache(labels, blockLabels)

	if result {
		t.Error("expected MR with no labels to not be blocked, got blocked")
	}
}

func TestIsMRBlockedFromCache_EmptyBlockList(t *testing.T) {
	labels := []models.Label{
		{Name: "feature"},
		{Name: "blocked"},
	}
	blockLabels := map[string]struct{}{}

	result := isMRBlockedFromCache(labels, blockLabels)

	if result {
		t.Error("expected MR to not be blocked when no block labels configured, got blocked")
	}
}

func TestIsMRBlockedFromCache_NilBlockList(t *testing.T) {
	labels := []models.Label{
		{Name: "feature"},
		{Name: "blocked"},
	}

	result := isMRBlockedFromCache(labels, nil)

	if result {
		t.Error("expected MR to not be blocked when block labels map is nil, got blocked")
	}
}

func TestIsMRBlockedFromCache_MultipleBlockingLabels(t *testing.T) {
	labels := []models.Label{
		{Name: "blocked"},
		{Name: "wip"},
	}
	blockLabels := map[string]struct{}{
		"blocked": {},
		"wip":     {},
	}

	result := isMRBlockedFromCache(labels, blockLabels)

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
	releaseLabels := map[string]struct{}{
		"release":  {},
		"released": {},
	}

	result := hasReleaseLabelFromCache(labels, releaseLabels)

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
	releaseLabels := map[string]struct{}{
		"release":  {},
		"released": {},
	}

	result := hasReleaseLabelFromCache(labels, releaseLabels)

	if result {
		t.Error("expected MR to not have release label, got true")
	}
}

func TestHasReleaseLabelFromCache_EmptyLabels(t *testing.T) {
	labels := []models.Label{}
	releaseLabels := map[string]struct{}{
		"release":  {},
		"released": {},
	}

	result := hasReleaseLabelFromCache(labels, releaseLabels)

	if result {
		t.Error("expected MR with no labels to not have release label, got true")
	}
}

func TestHasReleaseLabelFromCache_EmptyReleaseList(t *testing.T) {
	labels := []models.Label{
		{Name: "feature"},
		{Name: "release"},
	}
	releaseLabels := map[string]struct{}{}

	result := hasReleaseLabelFromCache(labels, releaseLabels)

	if result {
		t.Error("expected MR to not have release label when no release labels configured, got true")
	}
}

func TestHasReleaseLabelFromCache_NilReleaseList(t *testing.T) {
	labels := []models.Label{
		{Name: "feature"},
		{Name: "release"},
	}

	result := hasReleaseLabelFromCache(labels, nil)

	if result {
		t.Error("expected MR to not have release label when release labels map is nil, got true")
	}
}

func TestHasReleaseLabelFromCache_MultipleReleaseLabels(t *testing.T) {
	labels := []models.Label{
		{Name: "release"},
		{Name: "released"},
	}
	releaseLabels := map[string]struct{}{
		"release":  {},
		"released": {},
	}

	result := hasReleaseLabelFromCache(labels, releaseLabels)

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
