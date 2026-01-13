package consumers

import (
	"strings"
	"testing"
	"time"

	"devstreamlinebot/mocks"
	"devstreamlinebot/models"
	"devstreamlinebot/testutils"
)

// TestPickReviewerFromPool_EmptyPool tests behavior with empty pool.
func TestPickReviewerFromPool_EmptyPool(t *testing.T) {
	db := testutils.SetupTestDB(t)
	consumer := NewMRReviewerConsumer(db, nil, nil, 0)

	idx := consumer.pickReviewerFromPool([]models.User{}, map[uint]int{})

	if idx != 0 {
		t.Errorf("pickReviewerFromPool with empty pool: got %d, want 0", idx)
	}
}

// TestPickReviewerFromPool_SingleUser tests behavior with single user.
func TestPickReviewerFromPool_SingleUser(t *testing.T) {
	db := testutils.SetupTestDB(t)
	userFactory := testutils.NewUserFactory(db)
	consumer := NewMRReviewerConsumer(db, nil, nil, 0)

	user := userFactory.Create()
	users := []models.User{user}

	idx := consumer.pickReviewerFromPool(users, map[uint]int{})

	if idx != 0 {
		t.Errorf("pickReviewerFromPool with single user: got %d, want 0", idx)
	}
}

// TestPickReviewerFromPool_WeightedSelection tests weighted probability distribution.
func TestPickReviewerFromPool_WeightedSelection(t *testing.T) {
	db := testutils.SetupTestDB(t)
	userFactory := testutils.NewUserFactory(db)
	consumer := NewMRReviewerConsumer(db, nil, nil, 0)

	heavyLoadUser := userFactory.Create(testutils.WithUsername("heavy"))
	lightLoadUser := userFactory.Create(testutils.WithUsername("light"))
	users := []models.User{heavyLoadUser, lightLoadUser}
	counts := map[uint]int{
		heavyLoadUser.ID: 100, // Heavy load
		lightLoadUser.ID: 0,   // Light load - weight = 1/(0+1) = 1
	}

	// Run multiple times to verify statistical distribution
	lightCount := 0
	iterations := 1000
	for i := 0; i < iterations; i++ {
		idx := consumer.pickReviewerFromPool(users, counts)
		if idx == 1 { // lightLoadUser
			lightCount++
		}
	}

	// Light user should be selected significantly more often
	// With weight 1 vs 1/101 ≈ 0.0099, light should get ~99% of selections
	// Using 80% as threshold to account for randomness
	minExpected := iterations * 80 / 100
	if lightCount < minExpected {
		t.Errorf("Weighted selection not working: lightLoadUser selected %d/%d times, expected >%d", lightCount, iterations, minExpected)
	}
}

// TestPickReviewerFromPool_UserMissingFromCounts tests default weight for missing users.
func TestPickReviewerFromPool_UserMissingFromCounts(t *testing.T) {
	db := testutils.SetupTestDB(t)
	userFactory := testutils.NewUserFactory(db)
	consumer := NewMRReviewerConsumer(db, nil, nil, 0)

	user1 := userFactory.Create(testutils.WithUsername("user1"))
	user2 := userFactory.Create(testutils.WithUsername("user2"))
	users := []models.User{user1, user2}

	// Empty counts - both should have weight = 1/(0+1) = 1
	counts := map[uint]int{}

	// Run multiple times - both should be selected roughly equally
	user1Count := 0
	iterations := 1000
	for i := 0; i < iterations; i++ {
		idx := consumer.pickReviewerFromPool(users, counts)
		if idx == 0 {
			user1Count++
		}
	}

	// Should be roughly 50/50, allow 30-70% range
	if user1Count < iterations*30/100 || user1Count > iterations*70/100 {
		t.Errorf("Equal weight selection skewed: user1 selected %d/%d times, expected ~50%%", user1Count, iterations)
	}
}

// TestPickMultipleFromPool_PoolSmallerThanCount tests when pool is smaller than requested count.
func TestPickMultipleFromPool_PoolSmallerThanCount(t *testing.T) {
	db := testutils.SetupTestDB(t)
	userFactory := testutils.NewUserFactory(db)
	consumer := NewMRReviewerConsumer(db, nil, nil, 0)

	user1 := userFactory.Create()
	user2 := userFactory.Create()
	users := []models.User{user1, user2}
	counts := map[uint]int{}

	selected := consumer.pickMultipleFromPool(users, 5, counts)

	if len(selected) != 2 {
		t.Errorf("pickMultipleFromPool with pool < count: got %d users, want 2", len(selected))
	}
}

// TestPickMultipleFromPool_PoolEqualsCount tests when pool equals requested count.
func TestPickMultipleFromPool_PoolEqualsCount(t *testing.T) {
	db := testutils.SetupTestDB(t)
	userFactory := testutils.NewUserFactory(db)
	consumer := NewMRReviewerConsumer(db, nil, nil, 0)

	user1 := userFactory.Create()
	user2 := userFactory.Create()
	user3 := userFactory.Create()
	users := []models.User{user1, user2, user3}
	counts := map[uint]int{}

	selected := consumer.pickMultipleFromPool(users, 3, counts)

	if len(selected) != 3 {
		t.Errorf("pickMultipleFromPool with pool == count: got %d users, want 3", len(selected))
	}
}

// TestPickMultipleFromPool_PoolLargerThanCount tests when pool is larger than requested count.
func TestPickMultipleFromPool_PoolLargerThanCount(t *testing.T) {
	db := testutils.SetupTestDB(t)
	userFactory := testutils.NewUserFactory(db)
	consumer := NewMRReviewerConsumer(db, nil, nil, 0)

	var users []models.User
	for i := 0; i < 10; i++ {
		users = append(users, userFactory.Create())
	}
	counts := map[uint]int{}

	selected := consumer.pickMultipleFromPool(users, 3, counts)

	if len(selected) != 3 {
		t.Errorf("pickMultipleFromPool with pool > count: got %d users, want 3", len(selected))
	}
}

// TestPickMultipleFromPool_NoDuplicates tests that selected users are unique.
func TestPickMultipleFromPool_NoDuplicates(t *testing.T) {
	db := testutils.SetupTestDB(t)
	userFactory := testutils.NewUserFactory(db)
	consumer := NewMRReviewerConsumer(db, nil, nil, 0)

	var users []models.User
	for i := 0; i < 5; i++ {
		users = append(users, userFactory.Create())
	}
	counts := map[uint]int{}

	// Run multiple times to catch potential duplicates
	for i := 0; i < 100; i++ {
		selected := consumer.pickMultipleFromPool(users, 3, counts)

		seen := make(map[uint]bool)
		for _, u := range selected {
			if seen[u.ID] {
				t.Fatalf("Duplicate user selected: ID %d", u.ID)
			}
			seen[u.ID] = true
		}
	}
}

// TestPickMultipleFromPool_EmptyPool tests behavior with empty pool.
func TestPickMultipleFromPool_EmptyPool(t *testing.T) {
	db := testutils.SetupTestDB(t)
	consumer := NewMRReviewerConsumer(db, nil, nil, 0)

	selected := consumer.pickMultipleFromPool([]models.User{}, 3, map[uint]int{})

	if selected != nil {
		t.Errorf("pickMultipleFromPool with empty pool: got %v, want nil", selected)
	}
}

// TestPickMultipleFromPool_ZeroCount tests behavior with zero count.
func TestPickMultipleFromPool_ZeroCount(t *testing.T) {
	db := testutils.SetupTestDB(t)
	userFactory := testutils.NewUserFactory(db)
	consumer := NewMRReviewerConsumer(db, nil, nil, 0)

	users := []models.User{userFactory.Create(), userFactory.Create()}

	selected := consumer.pickMultipleFromPool(users, 0, map[uint]int{})

	if selected != nil {
		t.Errorf("pickMultipleFromPool with zero count: got %v, want nil", selected)
	}
}

// TestGetLabelReviewerGroups_NoLabels tests when MR has no labels.
func TestGetLabelReviewerGroups_NoLabels(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	consumer := NewMRReviewerConsumer(db, nil, nil, 0)
	groups := consumer.getLabelReviewerGroups(&mr)

	if groups != nil {
		t.Errorf("getLabelReviewerGroups with no labels: got %v, want nil", groups)
	}
}

// TestGetLabelReviewerGroups_LabelsWithoutReviewers tests labels without configured reviewers.
func TestGetLabelReviewerGroups_LabelsWithoutReviewers(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "backend", "frontend"))

	consumer := NewMRReviewerConsumer(db, nil, nil, 0)
	groups := consumer.getLabelReviewerGroups(&mr)

	if groups != nil {
		t.Errorf("getLabelReviewerGroups with labels but no reviewers: got %v, want nil", groups)
	}
}

// TestGetLabelReviewerGroups_ExcludesAuthor tests that MR author is excluded.
func TestGetLabelReviewerGroups_ExcludesAuthor(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create(testutils.WithUsername("author"))
	otherReviewer := userFactory.Create(testutils.WithUsername("other"))

	// Author is also a label reviewer
	testutils.CreateLabelReviewer(db, repo, "backend", author)
	testutils.CreateLabelReviewer(db, repo, "backend", otherReviewer)

	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "backend"))

	consumer := NewMRReviewerConsumer(db, nil, nil, 0)
	groups := consumer.getLabelReviewerGroups(&mr)

	if len(groups["backend"]) != 1 {
		t.Fatalf("Expected 1 reviewer in group (author excluded), got %d", len(groups["backend"]))
	}
	if groups["backend"][0].ID != otherReviewer.ID {
		t.Error("Author should be excluded from label reviewer groups")
	}
}

// TestGetLabelReviewerGroups_ExcludesVacationUsers tests that vacation users are excluded.
func TestGetLabelReviewerGroups_ExcludesVacationUsers(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create(testutils.WithUsername("author"))
	vacationUser := userFactory.Create(testutils.WithUsername("vacation"), testutils.WithOnVacation())
	activeUser := userFactory.Create(testutils.WithUsername("active"))

	testutils.CreateLabelReviewer(db, repo, "frontend", vacationUser)
	testutils.CreateLabelReviewer(db, repo, "frontend", activeUser)

	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "frontend"))

	consumer := NewMRReviewerConsumer(db, nil, nil, 0)
	groups := consumer.getLabelReviewerGroups(&mr)

	if len(groups["frontend"]) != 1 {
		t.Fatalf("Expected 1 reviewer (vacation user excluded), got %d", len(groups["frontend"]))
	}
	if groups["frontend"][0].ID != activeUser.ID {
		t.Error("Vacation user should be excluded from label reviewer groups")
	}
}

// TestGetLabelReviewerGroups_MultipleLabels tests grouping by multiple labels.
func TestGetLabelReviewerGroups_MultipleLabels(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create(testutils.WithUsername("author"))
	backendReviewer := userFactory.Create(testutils.WithUsername("backend-dev"))
	frontendReviewer := userFactory.Create(testutils.WithUsername("frontend-dev"))

	testutils.CreateLabelReviewer(db, repo, "backend", backendReviewer)
	testutils.CreateLabelReviewer(db, repo, "frontend", frontendReviewer)

	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "backend", "frontend"))

	consumer := NewMRReviewerConsumer(db, nil, nil, 0)
	groups := consumer.getLabelReviewerGroups(&mr)

	if len(groups) != 2 {
		t.Fatalf("Expected 2 label groups, got %d", len(groups))
	}
	if len(groups["backend"]) != 1 || groups["backend"][0].ID != backendReviewer.ID {
		t.Error("backend group should have backendReviewer")
	}
	if len(groups["frontend"]) != 1 || groups["frontend"][0].ID != frontendReviewer.ID {
		t.Error("frontend group should have frontendReviewer")
	}
}

// TestGetDefaultReviewers_EmptyPool tests when no default reviewers configured.
func TestGetDefaultReviewers_EmptyPool(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	consumer := NewMRReviewerConsumer(db, nil, nil, 0)
	reviewers := consumer.getDefaultReviewers(&mr)

	if len(reviewers) != 0 {
		t.Errorf("getDefaultReviewers with empty pool: got %d, want 0", len(reviewers))
	}
}

// TestGetDefaultReviewers_ExcludesAuthor tests that MR author is excluded.
func TestGetDefaultReviewers_ExcludesAuthor(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	other := userFactory.Create()

	testutils.CreatePossibleReviewer(db, repo, author)
	testutils.CreatePossibleReviewer(db, repo, other)

	mr := mrFactory.Create(repo, author)

	consumer := NewMRReviewerConsumer(db, nil, nil, 0)
	reviewers := consumer.getDefaultReviewers(&mr)

	if len(reviewers) != 1 {
		t.Fatalf("Expected 1 reviewer (author excluded), got %d", len(reviewers))
	}
	if reviewers[0].ID != other.ID {
		t.Error("Author should be excluded from default reviewers")
	}
}

// TestGetDefaultReviewers_ExcludesVacationUsers tests that vacation users are excluded.
func TestGetDefaultReviewers_ExcludesVacationUsers(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	vacationUser := userFactory.Create(testutils.WithOnVacation())
	activeUser := userFactory.Create()

	testutils.CreatePossibleReviewer(db, repo, vacationUser)
	testutils.CreatePossibleReviewer(db, repo, activeUser)

	mr := mrFactory.Create(repo, author)

	consumer := NewMRReviewerConsumer(db, nil, nil, 0)
	reviewers := consumer.getDefaultReviewers(&mr)

	if len(reviewers) != 1 {
		t.Fatalf("Expected 1 reviewer (vacation user excluded), got %d", len(reviewers))
	}
	if reviewers[0].ID != activeUser.ID {
		t.Error("Vacation user should be excluded from default reviewers")
	}
}

// TestGetReviewCountsForUserIDs_EmptyIDs tests with empty user IDs.
func TestGetReviewCountsForUserIDs_EmptyIDs(t *testing.T) {
	db := testutils.SetupTestDB(t)
	consumer := NewMRReviewerConsumer(db, nil, nil, 0)

	counts := consumer.getReviewCountsForUserIDs([]uint{})

	if len(counts) != 0 {
		t.Errorf("getReviewCountsForUserIDs with empty IDs: got %v, want empty map", counts)
	}
}

// TestGetReviewCountsForUserIDs_UserWithReviews tests counting recent reviews.
func TestGetReviewCountsForUserIDs_UserWithReviews(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	// Create 3 MRs with the reviewer assigned (within last 14 days)
	recentTime := time.Now().Add(-24 * time.Hour)
	for i := 0; i < 3; i++ {
		mr := mrFactory.Create(repo, author, testutils.WithCreatedAt(recentTime))
		testutils.AssignReviewers(db, &mr, reviewer)
	}

	consumer := NewMRReviewerConsumer(db, nil, nil, 0)
	counts := consumer.getReviewCountsForUserIDs([]uint{reviewer.ID})

	if counts[reviewer.ID] != 3 {
		t.Errorf("Expected review count 3, got %d", counts[reviewer.ID])
	}
}

// TestGetReviewCountsForUserIDs_OldReviewsNotCounted tests that old reviews aren't counted.
func TestGetReviewCountsForUserIDs_OldReviewsNotCounted(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	// Create MR older than 14 days
	oldTime := time.Now().Add(-15 * 24 * time.Hour)
	mr := mrFactory.Create(repo, author, testutils.WithCreatedAt(oldTime))
	testutils.AssignReviewers(db, &mr, reviewer)

	consumer := NewMRReviewerConsumer(db, nil, nil, 0)
	counts := consumer.getReviewCountsForUserIDs([]uint{reviewer.ID})

	if counts[reviewer.ID] != 0 {
		t.Errorf("Expected review count 0 (old reviews), got %d", counts[reviewer.ID])
	}
}

// TestSelectReviewers_UsesLabelReviewers tests that label reviewers are used when available.
func TestSelectReviewers_UsesLabelReviewers(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	labelReviewer := userFactory.Create(testutils.WithUsername("label-reviewer"))
	defaultReviewer := userFactory.Create(testutils.WithUsername("default-reviewer"))

	testutils.CreateLabelReviewer(db, repo, "backend", labelReviewer)
	testutils.CreatePossibleReviewer(db, repo, defaultReviewer)

	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "backend"))

	consumer := NewMRReviewerConsumer(db, nil, nil, 0)
	selected := consumer.selectReviewers(&mr, 1)

	if len(selected) != 1 {
		t.Fatalf("Expected 1 reviewer, got %d", len(selected))
	}
	if selected[0].ID != labelReviewer.ID {
		t.Errorf("Expected label reviewer, got %s", selected[0].Username)
	}
}

// TestSelectReviewers_FallsBackToDefaultPool tests fallback when no label reviewers.
func TestSelectReviewers_FallsBackToDefaultPool(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	defaultReviewer := userFactory.Create(testutils.WithUsername("default-reviewer"))

	testutils.CreatePossibleReviewer(db, repo, defaultReviewer)

	mr := mrFactory.Create(repo, author)

	consumer := NewMRReviewerConsumer(db, nil, nil, 0)
	selected := consumer.selectReviewers(&mr, 1)

	if len(selected) != 1 {
		t.Fatalf("Expected 1 reviewer, got %d", len(selected))
	}
	if selected[0].ID != defaultReviewer.ID {
		t.Errorf("Expected default reviewer, got %s", selected[0].Username)
	}
}

// TestSelectFromLabelGroups_OneLabel tests picking from single label group.
func TestSelectFromLabelGroups_OneLabel(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	reviewer := userFactory.Create()

	testutils.CreateLabelReviewer(db, repo, "backend", reviewer)

	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "backend"))

	consumer := NewMRReviewerConsumer(db, nil, nil, 0)
	groups := consumer.getLabelReviewerGroups(&mr)
	selected := consumer.selectFromLabelGroups(&mr, groups, 1)

	if len(selected) != 1 {
		t.Fatalf("Expected 1 reviewer, got %d", len(selected))
	}
	if selected[0].ID != reviewer.ID {
		t.Error("Expected the label reviewer to be selected")
	}
}

// TestSelectFromLabelGroups_TwoLabels tests picking from two label groups.
func TestSelectFromLabelGroups_TwoLabels(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	backendReviewer := userFactory.Create(testutils.WithUsername("backend"))
	frontendReviewer := userFactory.Create(testutils.WithUsername("frontend"))

	testutils.CreateLabelReviewer(db, repo, "backend", backendReviewer)
	testutils.CreateLabelReviewer(db, repo, "frontend", frontendReviewer)

	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "backend", "frontend"))

	consumer := NewMRReviewerConsumer(db, nil, nil, 0)
	groups := consumer.getLabelReviewerGroups(&mr)
	selected := consumer.selectFromLabelGroups(&mr, groups, 2)

	if len(selected) != 2 {
		t.Fatalf("Expected 2 reviewers, got %d", len(selected))
	}

	// Verify both reviewers are in the selection
	selectedIDs := make(map[uint]bool)
	for _, s := range selected {
		selectedIDs[s.ID] = true
	}
	if !selectedIDs[backendReviewer.ID] || !selectedIDs[frontendReviewer.ID] {
		t.Error("Expected one reviewer from each label group")
	}
}

// TestSelectFromLabelGroups_NoReuseAcrossLabels tests that same user isn't picked twice.
func TestSelectFromLabelGroups_NoReuseAcrossLabels(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	sharedReviewer := userFactory.Create(testutils.WithUsername("shared"))
	backendOnly := userFactory.Create(testutils.WithUsername("backend-only"))

	// sharedReviewer is in both groups
	testutils.CreateLabelReviewer(db, repo, "backend", sharedReviewer)
	testutils.CreateLabelReviewer(db, repo, "backend", backendOnly)
	testutils.CreateLabelReviewer(db, repo, "frontend", sharedReviewer)

	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "backend", "frontend"))

	consumer := NewMRReviewerConsumer(db, nil, nil, 0)

	// Run multiple times to ensure no duplicates
	for i := 0; i < 50; i++ {
		groups := consumer.getLabelReviewerGroups(&mr)
		selected := consumer.selectFromLabelGroups(&mr, groups, 2)

		// Check for duplicates
		seen := make(map[uint]bool)
		for _, s := range selected {
			if seen[s.ID] {
				t.Fatalf("Duplicate user selected: %s (ID %d)", s.Username, s.ID)
			}
			seen[s.ID] = true
		}
	}
}

// TestSelectFromLabelGroups_FillsFromDefaultPool tests filling from default pool when < minCount.
func TestSelectFromLabelGroups_FillsFromDefaultPool(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	labelReviewer := userFactory.Create(testutils.WithUsername("label"))
	defaultReviewer1 := userFactory.Create(testutils.WithUsername("default1"))
	defaultReviewer2 := userFactory.Create(testutils.WithUsername("default2"))

	testutils.CreateLabelReviewer(db, repo, "backend", labelReviewer)
	testutils.CreatePossibleReviewer(db, repo, defaultReviewer1)
	testutils.CreatePossibleReviewer(db, repo, defaultReviewer2)

	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "backend"))

	consumer := NewMRReviewerConsumer(db, nil, nil, 0)
	groups := consumer.getLabelReviewerGroups(&mr)
	selected := consumer.selectFromLabelGroups(&mr, groups, 3)

	if len(selected) != 3 {
		t.Fatalf("Expected 3 reviewers, got %d", len(selected))
	}

	// Verify label reviewer is in selection
	hasLabelReviewer := false
	for _, s := range selected {
		if s.ID == labelReviewer.ID {
			hasLabelReviewer = true
			break
		}
	}
	if !hasLabelReviewer {
		t.Error("Expected label reviewer to be in selection")
	}
}

// TestGetAssignCount_ReturnsConfiguredValue tests returning SLA assign count.
func TestGetAssignCount_ReturnsConfiguredValue(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create()
	testutils.CreateRepositorySLA(db, repo, 3)

	consumer := NewMRReviewerConsumer(db, nil, nil, 0)
	count := consumer.getAssignCount(repo.ID)

	if count != 3 {
		t.Errorf("Expected assign count 3, got %d", count)
	}
}

// TestGetAssignCount_DefaultsToOne tests default value when no SLA configured.
func TestGetAssignCount_DefaultsToOne(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)

	repo := repoFactory.Create()

	consumer := NewMRReviewerConsumer(db, nil, nil, 0)
	count := consumer.getAssignCount(repo.ID)

	if count != 1 {
		t.Errorf("Expected default assign count 1, got %d", count)
	}
}

// ============================================================================
// State Change Notification Tests
// ============================================================================

// TestProcessStateChangeNotifications_NoActions verifies no messages sent when no unnotified actions exist.
func TestProcessStateChangeNotifications_NoActions(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	consumer := NewMRReviewerConsumerWithBot(db, mockBot, nil, 0)
	consumer.ProcessStateChangeNotifications()

	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 0 {
		t.Errorf("Expected no messages, got %d", len(sentMessages))
	}
}

// TestProcessStateChangeNotifications_ClosedMR verifies actions on closed MRs are marked notified without sending messages.
func TestProcessStateChangeNotifications_ClosedMR(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()
	userFactory := testutils.NewUserFactory(db)
	repoFactory := testutils.NewRepositoryFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create(testutils.WithEmail("author@example.com"))
	reviewer := userFactory.Create(testutils.WithEmail("reviewer@example.com"))

	mr := mrFactory.Create(repo, author, testutils.WithMRState("closed"))
	testutils.AssignReviewers(db, &mr, reviewer)

	// Create unnotified action
	action := testutils.CreateMRAction(db, mr, models.ActionCommentAdded, testutils.WithActor(reviewer))

	consumer := NewMRReviewerConsumerWithBot(db, mockBot, nil, 0)
	consumer.ProcessStateChangeNotifications()

	// Verify no messages sent
	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 0 {
		t.Errorf("Expected no messages for closed MR, got %d", len(sentMessages))
	}

	// Verify action marked as notified
	var updatedAction models.MRAction
	db.First(&updatedAction, action.ID)
	if !updatedAction.Notified {
		t.Error("Action should be marked as notified")
	}
}

// TestStateChange_OnReviewToOnFixes verifies author is notified when MR transitions to on_fixes.
func TestStateChange_OnReviewToOnFixes(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()
	userFactory := testutils.NewUserFactory(db)
	repoFactory := testutils.NewRepositoryFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create(testutils.WithEmail("author@example.com"))
	reviewer := userFactory.Create(testutils.WithEmail("reviewer@example.com"))

	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer)

	// Create unresolved resolvable comment (triggers on_fixes state)
	comment := testutils.CreateMRComment(db, mr, reviewer, 123, testutils.WithResolvable())

	// Create unnotified action for the comment
	testutils.CreateMRAction(db, mr, models.ActionCommentAdded,
		testutils.WithActor(reviewer),
		testutils.WithCommentID(comment.ID),
	)

	consumer := NewMRReviewerConsumerWithBot(db, mockBot, nil, 0)
	consumer.ProcessStateChangeNotifications()

	// Verify author received notification
	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(sentMessages))
	}
	if sentMessages[0].ChatID != "author@example.com" {
		t.Errorf("Expected message to author@example.com, got %s", sentMessages[0].ChatID)
	}
	if !strings.Contains(sentMessages[0].Text, "🔧") {
		t.Error("Expected fixes notification with 🔧 emoji")
	}

	// Verify LastNotifiedState updated
	var updatedMR models.MergeRequest
	db.First(&updatedMR, mr.ID)
	if updatedMR.LastNotifiedState != "on_fixes" {
		t.Errorf("Expected LastNotifiedState='on_fixes', got '%s'", updatedMR.LastNotifiedState)
	}
}

// TestStateChange_OnFixesToOnReview verifies reviewers are notified when MR transitions back to on_review.
func TestStateChange_OnFixesToOnReview(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()
	userFactory := testutils.NewUserFactory(db)
	repoFactory := testutils.NewRepositoryFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create(testutils.WithEmail("author@example.com"))
	reviewer := userFactory.Create(testutils.WithEmail("reviewer@example.com"))

	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer)

	// Set LastNotifiedState to on_fixes (simulating previous notification)
	db.Model(&mr).Update("last_notified_state", "on_fixes")

	// Create RESOLVED comment (MR is now on_review)
	comment := testutils.CreateMRComment(db, mr, reviewer, 123,
		testutils.WithResolvable(),
		testutils.WithResolved(&author),
	)

	// Create unnotified action for comment resolution
	testutils.CreateMRAction(db, mr, models.ActionCommentResolved,
		testutils.WithActor(author),
		testutils.WithCommentID(comment.ID),
	)

	consumer := NewMRReviewerConsumerWithBot(db, mockBot, nil, 0)
	consumer.ProcessStateChangeNotifications()

	// Verify reviewer received notification
	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(sentMessages))
	}
	if sentMessages[0].ChatID != "reviewer@example.com" {
		t.Errorf("Expected message to reviewer@example.com, got %s", sentMessages[0].ChatID)
	}
	if !strings.Contains(sentMessages[0].Text, "✅") {
		t.Error("Expected re-review notification with ✅ emoji")
	}

	// Verify LastNotifiedState updated
	var updatedMR models.MergeRequest
	db.First(&updatedMR, mr.ID)
	if updatedMR.LastNotifiedState != "on_review" {
		t.Errorf("Expected LastNotifiedState='on_review', got '%s'", updatedMR.LastNotifiedState)
	}
}

// TestNoNotification_AlreadyOnFixes verifies no duplicate notification when already in on_fixes state.
func TestNoNotification_AlreadyOnFixes(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()
	userFactory := testutils.NewUserFactory(db)
	repoFactory := testutils.NewRepositoryFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create(testutils.WithEmail("author@example.com"))
	reviewer := userFactory.Create(testutils.WithEmail("reviewer@example.com"))

	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer)

	// Set LastNotifiedState to on_fixes (already notified)
	db.Model(&mr).Update("last_notified_state", "on_fixes")

	// Create unresolved comment (still on_fixes)
	comment := testutils.CreateMRComment(db, mr, reviewer, 123, testutils.WithResolvable())

	// Create unnotified action
	action := testutils.CreateMRAction(db, mr, models.ActionCommentAdded,
		testutils.WithActor(reviewer),
		testutils.WithCommentID(comment.ID),
	)

	consumer := NewMRReviewerConsumerWithBot(db, mockBot, nil, 0)
	consumer.ProcessStateChangeNotifications()

	// Verify NO messages sent (already in on_fixes, no state change)
	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 0 {
		t.Errorf("Expected no messages (no state change), got %d", len(sentMessages))
	}

	// Verify action still marked as notified
	var updatedAction models.MRAction
	db.First(&updatedAction, action.ID)
	if !updatedAction.Notified {
		t.Error("Action should be marked as notified even without message")
	}
}

// TestNoNotification_AlreadyOnReview verifies no duplicate notification when already in on_review state.
func TestNoNotification_AlreadyOnReview(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()
	userFactory := testutils.NewUserFactory(db)
	repoFactory := testutils.NewRepositoryFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create(testutils.WithEmail("author@example.com"))
	reviewer := userFactory.Create(testutils.WithEmail("reviewer@example.com"))

	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer)

	// Set LastNotifiedState to on_review (already notified)
	db.Model(&mr).Update("last_notified_state", "on_review")

	// No unresolved comments, so MR is on_review
	// Create a resolved comment action
	comment := testutils.CreateMRComment(db, mr, reviewer, 123,
		testutils.WithResolvable(),
		testutils.WithResolved(&author),
	)

	action := testutils.CreateMRAction(db, mr, models.ActionCommentResolved,
		testutils.WithActor(author),
		testutils.WithCommentID(comment.ID),
	)

	consumer := NewMRReviewerConsumerWithBot(db, mockBot, nil, 0)
	consumer.ProcessStateChangeNotifications()

	// Verify NO messages sent (already in on_review, no state change)
	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 0 {
		t.Errorf("Expected no messages (no state change), got %d", len(sentMessages))
	}

	// Verify action marked as notified
	var updatedAction models.MRAction
	db.First(&updatedAction, action.ID)
	if !updatedAction.Notified {
		t.Error("Action should be marked as notified")
	}
}

// TestMultipleReviewers_AllNotified verifies all reviewers receive notification on re-review.
func TestMultipleReviewers_AllNotified(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()
	userFactory := testutils.NewUserFactory(db)
	repoFactory := testutils.NewRepositoryFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create(testutils.WithEmail("author@example.com"))
	reviewer1 := userFactory.Create(testutils.WithEmail("reviewer1@example.com"))
	reviewer2 := userFactory.Create(testutils.WithEmail("reviewer2@example.com"))
	reviewer3 := userFactory.Create(testutils.WithEmail("reviewer3@example.com"))

	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer1, reviewer2, reviewer3)

	// Set LastNotifiedState to on_fixes
	db.Model(&mr).Update("last_notified_state", "on_fixes")

	// Create resolved comment (MR is now on_review)
	comment := testutils.CreateMRComment(db, mr, reviewer1, 123,
		testutils.WithResolvable(),
		testutils.WithResolved(&author),
	)

	testutils.CreateMRAction(db, mr, models.ActionCommentResolved,
		testutils.WithActor(author),
		testutils.WithCommentID(comment.ID),
	)

	consumer := NewMRReviewerConsumerWithBot(db, mockBot, nil, 0)
	consumer.ProcessStateChangeNotifications()

	// Verify all 3 reviewers received notifications
	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 3 {
		t.Fatalf("Expected 3 messages (one per reviewer), got %d", len(sentMessages))
	}

	// Verify each reviewer got a message
	emails := make(map[string]bool)
	for _, msg := range sentMessages {
		emails[msg.ChatID] = true
	}
	for _, email := range []string{"reviewer1@example.com", "reviewer2@example.com", "reviewer3@example.com"} {
		if !emails[email] {
			t.Errorf("Expected message to %s", email)
		}
	}
}

// TestBatchProcessing_MultipleMRs verifies multiple MRs are processed correctly in a single call.
func TestBatchProcessing_MultipleMRs(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()
	userFactory := testutils.NewUserFactory(db)
	repoFactory := testutils.NewRepositoryFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author1 := userFactory.Create(testutils.WithEmail("author1@example.com"))
	author2 := userFactory.Create(testutils.WithEmail("author2@example.com"))
	reviewer := userFactory.Create(testutils.WithEmail("reviewer@example.com"))

	// MR1: on_review -> on_fixes (notify author1)
	mr1 := mrFactory.Create(repo, author1)
	testutils.AssignReviewers(db, &mr1, reviewer)
	comment1 := testutils.CreateMRComment(db, mr1, reviewer, 101, testutils.WithResolvable())
	testutils.CreateMRAction(db, mr1, models.ActionCommentAdded,
		testutils.WithActor(reviewer),
		testutils.WithCommentID(comment1.ID),
	)

	// MR2: on_review -> on_fixes (notify author2)
	mr2 := mrFactory.Create(repo, author2)
	testutils.AssignReviewers(db, &mr2, reviewer)
	comment2 := testutils.CreateMRComment(db, mr2, reviewer, 102, testutils.WithResolvable())
	testutils.CreateMRAction(db, mr2, models.ActionCommentAdded,
		testutils.WithActor(reviewer),
		testutils.WithCommentID(comment2.ID),
	)

	consumer := NewMRReviewerConsumerWithBot(db, mockBot, nil, 0)
	consumer.ProcessStateChangeNotifications()

	// Verify both authors received notifications
	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 2 {
		t.Fatalf("Expected 2 messages (one per MR author), got %d", len(sentMessages))
	}

	emails := make(map[string]bool)
	for _, msg := range sentMessages {
		emails[msg.ChatID] = true
	}
	if !emails["author1@example.com"] {
		t.Error("Expected message to author1@example.com")
	}
	if !emails["author2@example.com"] {
		t.Error("Expected message to author2@example.com")
	}
}

// TestOnReviewFromInitial_NoNotification verifies no notification when MR is on_review from initial state.
func TestOnReviewFromInitial_NoNotification(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()
	userFactory := testutils.NewUserFactory(db)
	repoFactory := testutils.NewRepositoryFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create(testutils.WithEmail("author@example.com"))
	reviewer := userFactory.Create(testutils.WithEmail("reviewer@example.com"))

	mr := mrFactory.Create(repo, author)
	testutils.AssignReviewers(db, &mr, reviewer)
	// Note: LastNotifiedState is empty (initial state)

	// No comments, MR is on_review
	// Create a comment resolved action (shouldn't trigger re-review notification from initial state)
	comment := testutils.CreateMRComment(db, mr, reviewer, 123,
		testutils.WithResolvable(),
		testutils.WithResolved(&author),
	)

	testutils.CreateMRAction(db, mr, models.ActionCommentResolved,
		testutils.WithActor(author),
		testutils.WithCommentID(comment.ID),
	)

	consumer := NewMRReviewerConsumerWithBot(db, mockBot, nil, 0)
	consumer.ProcessStateChangeNotifications()

	// Verify NO notification (on_review but not from on_fixes)
	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 0 {
		t.Errorf("Expected no messages (on_review from initial, not from on_fixes), got %d", len(sentMessages))
	}
}
