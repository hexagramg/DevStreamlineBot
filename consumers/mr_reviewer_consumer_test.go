package consumers

import (
	"testing"
	"time"

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
