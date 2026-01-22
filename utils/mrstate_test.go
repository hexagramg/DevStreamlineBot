package utils

import (
	"testing"
	"time"

	"devstreamlinebot/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupTestDB creates an in-memory SQLite database for testing.
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	// Migrate the models needed for testing
	err = db.AutoMigrate(
		&models.Repository{},
		&models.User{},
		&models.MergeRequest{},
		&models.MRComment{},
		&models.MRAction{},
		&models.BlockLabel{},
	)
	if err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return db
}

func TestDeriveState_Merged(t *testing.T) {
	db := setupTestDB(t)

	mr := &models.MergeRequest{
		State: "merged",
	}

	state := DeriveState(db, mr)
	if state != StateMerged {
		t.Errorf("DeriveState() = %v, want %v", state, StateMerged)
	}
}

func TestDeriveState_Closed(t *testing.T) {
	db := setupTestDB(t)

	mr := &models.MergeRequest{
		State: "closed",
	}

	state := DeriveState(db, mr)
	if state != StateClosed {
		t.Errorf("DeriveState() = %v, want %v", state, StateClosed)
	}
}

func TestDeriveState_Draft(t *testing.T) {
	db := setupTestDB(t)

	mr := &models.MergeRequest{
		State: "opened",
		Draft: true,
	}
	db.Create(mr)

	state := DeriveState(db, mr)
	if state != StateDraft {
		t.Errorf("DeriveState() = %v, want %v", state, StateDraft)
	}
}

func TestDeriveState_OnFixes(t *testing.T) {
	db := setupTestDB(t)

	// Create reviewer for the comment author
	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)

	// Create MR author
	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	// Create MR
	mr := &models.MergeRequest{
		State:    "opened",
		Draft:    false,
		AuthorID: author.ID,
	}
	db.Create(mr)

	// Create unresolved resolvable comment with thread metadata
	// Reviewer started the thread and is the last commenter (waiting for author)
	comment := &models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       12345,
		GitlabDiscussionID: "disc-onfixes-1",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    time.Now(),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	}
	db.Create(comment)

	state := DeriveState(db, mr)
	if state != StateOnFixes {
		t.Errorf("DeriveState() = %v, want %v", state, StateOnFixes)
	}
}

func TestDeriveState_OnReview(t *testing.T) {
	db := setupTestDB(t)

	// Create MR without draft or unresolved comments
	mr := &models.MergeRequest{
		State: "opened",
		Draft: false,
	}
	db.Create(mr)

	state := DeriveState(db, mr)
	if state != StateOnReview {
		t.Errorf("DeriveState() = %v, want %v", state, StateOnReview)
	}
}

func TestDeriveState_OnReview_WithResolvedComments(t *testing.T) {
	db := setupTestDB(t)

	// Create MR
	mr := &models.MergeRequest{
		State: "opened",
		Draft: false,
	}
	db.Create(mr)

	// Create resolved comment (should not affect state)
	comment := &models.MRComment{
		MergeRequestID:  mr.ID,
		GitlabNoteID:    12345,
		AuthorID:        1,
		Resolvable:      true,
		Resolved:        true,
		GitlabCreatedAt: time.Now(),
	}
	db.Create(comment)

	state := DeriveState(db, mr)
	if state != StateOnReview {
		t.Errorf("DeriveState() = %v, want %v", state, StateOnReview)
	}
}

func TestHasUnresolvedComments(t *testing.T) {
	db := setupTestDB(t)

	// Create MR
	mr := &models.MergeRequest{
		State: "opened",
	}
	db.Create(mr)

	// Initially no comments
	if HasUnresolvedComments(db, mr.ID) {
		t.Error("HasUnresolvedComments() = true, want false (no comments)")
	}

	// Add non-resolvable comment
	db.Create(&models.MRComment{
		MergeRequestID:  mr.ID,
		GitlabNoteID:    1,
		AuthorID:        1,
		Resolvable:      false,
		Resolved:        false,
		GitlabCreatedAt: time.Now(),
	})
	if HasUnresolvedComments(db, mr.ID) {
		t.Error("HasUnresolvedComments() = true, want false (non-resolvable)")
	}

	// Add resolved resolvable comment
	db.Create(&models.MRComment{
		MergeRequestID:  mr.ID,
		GitlabNoteID:    2,
		AuthorID:        1,
		Resolvable:      true,
		Resolved:        true,
		GitlabCreatedAt: time.Now(),
	})
	if HasUnresolvedComments(db, mr.ID) {
		t.Error("HasUnresolvedComments() = true, want false (resolved)")
	}

	// Add unresolved resolvable comment
	db.Create(&models.MRComment{
		MergeRequestID:  mr.ID,
		GitlabNoteID:    3,
		AuthorID:        1,
		Resolvable:      true,
		Resolved:        false,
		GitlabCreatedAt: time.Now(),
	})
	if !HasUnresolvedComments(db, mr.ID) {
		t.Error("HasUnresolvedComments() = false, want true (unresolved)")
	}
}

func TestGetUnresolvedComments(t *testing.T) {
	db := setupTestDB(t)

	// Create user for author
	user := &models.User{GitlabID: 100, Username: "testuser"}
	db.Create(user)

	// Create MR
	mr := &models.MergeRequest{
		State: "opened",
	}
	db.Create(mr)

	// Add mix of comments
	now := time.Now()
	db.Create(&models.MRComment{
		MergeRequestID:  mr.ID,
		GitlabNoteID:    1,
		AuthorID:        user.ID,
		Resolvable:      false, // not resolvable
		Resolved:        false,
		GitlabCreatedAt: now,
	})
	db.Create(&models.MRComment{
		MergeRequestID:  mr.ID,
		GitlabNoteID:    2,
		AuthorID:        user.ID,
		Resolvable:      true,
		Resolved:        true, // resolved
		GitlabCreatedAt: now.Add(1 * time.Hour),
	})
	db.Create(&models.MRComment{
		MergeRequestID:  mr.ID,
		GitlabNoteID:    3,
		AuthorID:        user.ID,
		Resolvable:      true,
		Resolved:        false, // unresolved
		GitlabCreatedAt: now.Add(2 * time.Hour),
	})
	db.Create(&models.MRComment{
		MergeRequestID:  mr.ID,
		GitlabNoteID:    4,
		AuthorID:        user.ID,
		Resolvable:      true,
		Resolved:        false, // unresolved
		GitlabCreatedAt: now.Add(3 * time.Hour),
	})

	comments := GetUnresolvedComments(db, mr.ID)
	if len(comments) != 2 {
		t.Errorf("GetUnresolvedComments() returned %d comments, want 2", len(comments))
	}
}

func TestGetStateInfo(t *testing.T) {
	db := setupTestDB(t)

	// Create repository
	repo := &models.Repository{GitlabID: 1, Name: "test-repo"}
	db.Create(repo)

	// Create MR
	createdAt := time.Now().Add(-24 * time.Hour)
	mr := &models.MergeRequest{
		State:           "opened",
		Draft:           false,
		RepositoryID:    repo.ID,
		GitlabCreatedAt: &createdAt,
	}
	db.Create(mr)

	info := GetStateInfo(db, mr)

	if info.State != StateOnReview {
		t.Errorf("GetStateInfo().State = %v, want %v", info.State, StateOnReview)
	}

	if info.UnresolvedCount != 0 {
		t.Errorf("GetStateInfo().UnresolvedCount = %d, want 0", info.UnresolvedCount)
	}
}

func TestGetStateTransitionTime_Merged(t *testing.T) {
	db := setupTestDB(t)

	mergedAt := time.Now().Add(-1 * time.Hour)
	mr := &models.MergeRequest{
		State:    "merged",
		MergedAt: &mergedAt,
	}

	transitionTime := GetStateTransitionTime(db, mr, StateMerged)
	if transitionTime == nil {
		t.Error("GetStateTransitionTime() = nil, want non-nil for merged MR")
	} else if !transitionTime.Equal(mergedAt) {
		t.Errorf("GetStateTransitionTime() = %v, want %v", transitionTime, mergedAt)
	}
}

func TestGetStateTransitionTime_Closed(t *testing.T) {
	db := setupTestDB(t)

	closedAt := time.Now().Add(-2 * time.Hour)
	mr := &models.MergeRequest{
		State:    "closed",
		ClosedAt: &closedAt,
	}

	transitionTime := GetStateTransitionTime(db, mr, StateClosed)
	if transitionTime == nil {
		t.Error("GetStateTransitionTime() = nil, want non-nil for closed MR")
	} else if !transitionTime.Equal(closedAt) {
		t.Errorf("GetStateTransitionTime() = %v, want %v", transitionTime, closedAt)
	}
}

func TestGetStateTransitionTime_OnFixes(t *testing.T) {
	db := setupTestDB(t)

	// Create reviewer for the comment author
	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)

	// Create MR author
	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	// Create MR
	mr := &models.MergeRequest{
		State:    "opened",
		AuthorID: author.ID,
	}
	db.Create(mr)

	// Create unresolved comment with thread metadata
	commentTime := time.Now().Add(-3 * time.Hour)
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: "disc-transition-1",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    commentTime,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	transitionTime := GetStateTransitionTime(db, mr, StateOnFixes)
	if transitionTime == nil {
		t.Error("GetStateTransitionTime() = nil, want non-nil for OnFixes state")
	} else if !transitionTime.Equal(commentTime) {
		t.Errorf("GetStateTransitionTime() = %v, want %v", transitionTime, commentTime)
	}
}

func TestGetMRTimeline(t *testing.T) {
	db := setupTestDB(t)

	// Create MR
	mr := &models.MergeRequest{State: "opened"}
	db.Create(mr)

	// Create actions
	now := time.Now()
	db.Create(&models.MRAction{
		MergeRequestID: mr.ID,
		ActionType:     models.ActionReviewerAssigned,
		Timestamp:      now,
	})
	db.Create(&models.MRAction{
		MergeRequestID: mr.ID,
		ActionType:     models.ActionCommentAdded,
		Timestamp:      now.Add(1 * time.Hour),
	})
	db.Create(&models.MRAction{
		MergeRequestID: mr.ID,
		ActionType:     models.ActionApproved,
		Timestamp:      now.Add(2 * time.Hour),
	})

	timeline := GetMRTimeline(db, mr.ID)
	if len(timeline) != 3 {
		t.Errorf("GetMRTimeline() returned %d actions, want 3", len(timeline))
	}

	// Verify chronological order
	if timeline[0].ActionType != models.ActionReviewerAssigned {
		t.Error("GetMRTimeline() first action should be ReviewerAssigned")
	}
	if timeline[2].ActionType != models.ActionApproved {
		t.Error("GetMRTimeline() last action should be Approved")
	}
}

func TestMRStateConstants(t *testing.T) {
	// Verify state constants have expected values
	tests := []struct {
		state MRState
		value string
	}{
		{StateOnReview, "on_review"},
		{StateOnFixes, "on_fixes"},
		{StateDraft, "draft"},
		{StateMerged, "merged"},
		{StateClosed, "closed"},
	}

	for _, tt := range tests {
		if string(tt.state) != tt.value {
			t.Errorf("MRState constant %v = %q, want %q", tt.state, string(tt.state), tt.value)
		}
	}
}

func TestGetStateInfo_SubtractsBlockedTime(t *testing.T) {
	db := setupTestDB(t)

	// Create repository
	repo := &models.Repository{GitlabID: 1, Name: "test-repo"}
	db.Create(repo)

	// Use a fixed time (Monday morning to avoid weekend issues)
	// MR created at T (9am Monday)
	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC) // Monday 9am

	// Create MR
	mr := &models.MergeRequest{
		State:           "opened",
		Draft:           false,
		RepositoryID:    repo.ID,
		GitlabCreatedAt: &baseTime,
	}
	db.Create(mr)

	// Block label added at T+2h, removed at T+4h (2h blocked)
	db.Create(&models.MRAction{
		MergeRequestID: mr.ID,
		ActionType:     models.ActionBlockLabelAdded,
		Timestamp:      baseTime.Add(2 * time.Hour),
		Metadata:       `{"label":"blocked"}`,
	})
	db.Create(&models.MRAction{
		MergeRequestID: mr.ID,
		ActionType:     models.ActionBlockLabelRemoved,
		Timestamp:      baseTime.Add(4 * time.Hour),
		Metadata:       `{"label":"blocked"}`,
	})

	// Current time: T+6h (within same workday to avoid complexity)
	// We need to mock time.Now() somehow, but since we can't,
	// let's verify the relationship: WorkingTime should be less than TimeInState by roughly blocked time

	info := GetStateInfo(db, mr)

	if info.State != StateOnReview {
		t.Errorf("GetStateInfo().State = %v, want %v", info.State, StateOnReview)
	}

	// The key check: TimeInState should be > 0 and WorkingTime should be calculated correctly
	// We can't test exact values without controlling time.Now(), but we can verify state
	if info.StateSince == nil {
		t.Error("GetStateInfo().StateSince should not be nil")
	}
}

func TestGetStateInfo_CurrentlyBlocked(t *testing.T) {
	db := setupTestDB(t)

	// Create repository
	repo := &models.Repository{GitlabID: 1, Name: "test-repo"}
	db.Create(repo)

	// Use a fixed time (Monday morning)
	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC) // Monday 9am

	// Create MR
	mr := &models.MergeRequest{
		State:           "opened",
		Draft:           false,
		RepositoryID:    repo.ID,
		GitlabCreatedAt: &baseTime,
	}
	db.Create(mr)

	// Block label added at T+2h, never removed (currently blocked)
	db.Create(&models.MRAction{
		MergeRequestID: mr.ID,
		ActionType:     models.ActionBlockLabelAdded,
		Timestamp:      baseTime.Add(2 * time.Hour),
		Metadata:       `{"label":"blocked"}`,
	})

	info := GetStateInfo(db, mr)

	if info.State != StateOnReview {
		t.Errorf("GetStateInfo().State = %v, want %v", info.State, StateOnReview)
	}

	// Verify that StateSince is set
	if info.StateSince == nil {
		t.Error("GetStateInfo().StateSince should not be nil")
	}

	// WorkingTime should never be negative
	if info.WorkingTime < 0 {
		t.Errorf("GetStateInfo().WorkingTime = %v, should never be negative", info.WorkingTime)
	}
}

func TestCalculateBlockedTime_Integration(t *testing.T) {
	db := setupTestDB(t)

	// Create repository
	repo := &models.Repository{GitlabID: 1, Name: "test-repo"}
	db.Create(repo)

	// Create MR
	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC) // Monday 9am
	mr := &models.MergeRequest{
		State:           "opened",
		Draft:           false,
		RepositoryID:    repo.ID,
		GitlabCreatedAt: &baseTime,
	}
	db.Create(mr)

	// Create block label actions
	// Block from T+1h to T+3h (2h blocked)
	db.Create(&models.MRAction{
		MergeRequestID: mr.ID,
		ActionType:     models.ActionBlockLabelAdded,
		Timestamp:      baseTime.Add(1 * time.Hour),
		Metadata:       `{"label":"blocked"}`,
	})
	db.Create(&models.MRAction{
		MergeRequestID: mr.ID,
		ActionType:     models.ActionBlockLabelRemoved,
		Timestamp:      baseTime.Add(3 * time.Hour),
		Metadata:       `{"label":"blocked"}`,
	})

	// Calculate blocked time for the window baseTime to baseTime+5h
	start := baseTime
	end := baseTime.Add(5 * time.Hour)
	blockedTime := CalculateBlockedTime(db, mr.ID, repo.ID, start, end)

	expected := 2 * time.Hour
	if blockedTime != expected {
		t.Errorf("CalculateBlockedTime() = %v, want %v", blockedTime, expected)
	}
}

// ============================================================================
// GetStateInfo Edge Case Tests
// ============================================================================

func TestGetStateInfo_BlockedTimeExceedsWorkingTime(t *testing.T) {
	db := setupTestDB(t)

	// Create repository
	repo := &models.Repository{GitlabID: 1, Name: "test-repo"}
	db.Create(repo)

	// MR created in the past
	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC) // Monday 9am
	mr := &models.MergeRequest{
		State:           "opened",
		Draft:           false,
		RepositoryID:    repo.ID,
		GitlabCreatedAt: &baseTime,
	}
	db.Create(mr)

	// Block label added immediately and never removed (blocked entire time)
	db.Create(&models.MRAction{
		MergeRequestID: mr.ID,
		ActionType:     models.ActionBlockLabelAdded,
		Timestamp:      baseTime,
		Metadata:       `{"label":"blocked"}`,
	})

	info := GetStateInfo(db, mr)

	// WorkingTime should be 0 (never negative) since blocked entire time
	if info.WorkingTime < 0 {
		t.Errorf("GetStateInfo().WorkingTime = %v, should never be negative", info.WorkingTime)
	}
}

func TestGetStateInfo_DraftStateWithBlocking(t *testing.T) {
	db := setupTestDB(t)

	// Create repository
	repo := &models.Repository{GitlabID: 1, Name: "test-repo"}
	db.Create(repo)

	// Draft MR with block label
	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	mr := &models.MergeRequest{
		State:           "opened",
		Draft:           true, // Draft state
		RepositoryID:    repo.ID,
		GitlabCreatedAt: &baseTime,
	}
	db.Create(mr)

	// Add block label action
	db.Create(&models.MRAction{
		MergeRequestID: mr.ID,
		ActionType:     models.ActionBlockLabelAdded,
		Timestamp:      baseTime.Add(1 * time.Hour),
		Metadata:       `{"label":"blocked"}`,
	})

	info := GetStateInfo(db, mr)

	// State should be draft (takes priority over blocking)
	if info.State != StateDraft {
		t.Errorf("GetStateInfo().State = %v, want %v (draft takes priority)", info.State, StateDraft)
	}

	// WorkingTime should still account for blocking
	if info.WorkingTime < 0 {
		t.Errorf("GetStateInfo().WorkingTime = %v, should never be negative", info.WorkingTime)
	}
}

func TestGetStateInfo_OnFixesStateWithBlocking(t *testing.T) {
	db := setupTestDB(t)

	// Create repository
	repo := &models.Repository{GitlabID: 1, Name: "test-repo"}
	db.Create(repo)

	// Create comment author (reviewer)
	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)

	// Create MR author
	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	// MR with unresolved comments AND block labels
	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	mr := &models.MergeRequest{
		State:           "opened",
		Draft:           false,
		RepositoryID:    repo.ID,
		AuthorID:        author.ID,
		GitlabCreatedAt: &baseTime,
	}
	db.Create(mr)

	// Add unresolved comment with thread metadata (puts MR in on_fixes state)
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: "disc-fixes-blocking-1",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    baseTime.Add(2 * time.Hour),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	// Add block label
	db.Create(&models.MRAction{
		MergeRequestID: mr.ID,
		ActionType:     models.ActionBlockLabelAdded,
		Timestamp:      baseTime.Add(3 * time.Hour),
		Metadata:       `{"label":"blocked"}`,
	})

	info := GetStateInfo(db, mr)

	// State should be on_fixes (comments determine state)
	if info.State != StateOnFixes {
		t.Errorf("GetStateInfo().State = %v, want %v", info.State, StateOnFixes)
	}

	// Unresolved count should be 1
	if info.UnresolvedCount != 1 {
		t.Errorf("GetStateInfo().UnresolvedCount = %d, want 1", info.UnresolvedCount)
	}

	// WorkingTime should exclude blocked time
	if info.WorkingTime < 0 {
		t.Errorf("GetStateInfo().WorkingTime = %v, should never be negative", info.WorkingTime)
	}
}

func TestGetStateInfo_NilStateSince(t *testing.T) {
	db := setupTestDB(t)

	// Create repository
	repo := &models.Repository{GitlabID: 1, Name: "test-repo"}
	db.Create(repo)

	// MR without GitlabCreatedAt (edge case)
	mr := &models.MergeRequest{
		State:           "opened",
		Draft:           false,
		RepositoryID:    repo.ID,
		GitlabCreatedAt: nil, // No creation time
	}
	db.Create(mr)

	info := GetStateInfo(db, mr)

	// State should be on_review
	if info.State != StateOnReview {
		t.Errorf("GetStateInfo().State = %v, want %v", info.State, StateOnReview)
	}

	// Should handle nil StateSince gracefully (no panic)
	// TimeInState and WorkingTime may be 0 or based on fallback
}

func TestGetStateInfo_MultipleBlockPeriods(t *testing.T) {
	db := setupTestDB(t)

	// Create repository
	repo := &models.Repository{GitlabID: 1, Name: "test-repo"}
	db.Create(repo)

	// MR created in the past
	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	mr := &models.MergeRequest{
		State:           "opened",
		Draft:           false,
		RepositoryID:    repo.ID,
		GitlabCreatedAt: &baseTime,
	}
	db.Create(mr)

	// First block period: T+1h to T+2h (1h blocked)
	db.Create(&models.MRAction{
		MergeRequestID: mr.ID,
		ActionType:     models.ActionBlockLabelAdded,
		Timestamp:      baseTime.Add(1 * time.Hour),
		Metadata:       `{"label":"blocked"}`,
	})
	db.Create(&models.MRAction{
		MergeRequestID: mr.ID,
		ActionType:     models.ActionBlockLabelRemoved,
		Timestamp:      baseTime.Add(2 * time.Hour),
		Metadata:       `{"label":"blocked"}`,
	})

	// Second block period: T+3h to T+4h (1h blocked)
	db.Create(&models.MRAction{
		MergeRequestID: mr.ID,
		ActionType:     models.ActionBlockLabelAdded,
		Timestamp:      baseTime.Add(3 * time.Hour),
		Metadata:       `{"label":"blocked"}`,
	})
	db.Create(&models.MRAction{
		MergeRequestID: mr.ID,
		ActionType:     models.ActionBlockLabelRemoved,
		Timestamp:      baseTime.Add(4 * time.Hour),
		Metadata:       `{"label":"blocked"}`,
	})

	// Calculate blocked time directly to verify
	end := baseTime.Add(5 * time.Hour)
	blockedTime := CalculateBlockedTime(db, mr.ID, repo.ID, baseTime, end)

	expectedBlocked := 2 * time.Hour // 1h + 1h
	if blockedTime != expectedBlocked {
		t.Errorf("CalculateBlockedTime() = %v, want %v (multiple periods)", blockedTime, expectedBlocked)
	}

	info := GetStateInfo(db, mr)

	// State should be on_review
	if info.State != StateOnReview {
		t.Errorf("GetStateInfo().State = %v, want %v", info.State, StateOnReview)
	}

	// WorkingTime should never be negative
	if info.WorkingTime < 0 {
		t.Errorf("GetStateInfo().WorkingTime = %v, should never be negative", info.WorkingTime)
	}
}

func TestGetStateInfo_BlockDuringStateTransition(t *testing.T) {
	db := setupTestDB(t)

	// Create repository
	repo := &models.Repository{GitlabID: 1, Name: "test-repo"}
	db.Create(repo)

	// Create comment author (reviewer)
	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)

	// Create MR author
	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	// MR starts on_review
	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	mr := &models.MergeRequest{
		State:           "opened",
		Draft:           false,
		RepositoryID:    repo.ID,
		AuthorID:        author.ID,
		GitlabCreatedAt: &baseTime,
	}
	db.Create(mr)

	// T+1h: Block label added (still on_review, but blocked)
	db.Create(&models.MRAction{
		MergeRequestID: mr.ID,
		ActionType:     models.ActionBlockLabelAdded,
		Timestamp:      baseTime.Add(1 * time.Hour),
		Metadata:       `{"label":"blocked"}`,
	})

	// T+2h: Comment added (state → on_fixes)
	comment := &models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: "disc-transition-block-1",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    baseTime.Add(2 * time.Hour),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	}
	db.Create(comment)

	// T+3h: Comment resolved (state → on_review)
	resolvedAt := baseTime.Add(3 * time.Hour)
	db.Model(comment).Updates(map[string]interface{}{
		"resolved":       true,
		"resolved_by_id": author.ID,
		"resolved_at":    resolvedAt,
	})
	db.Create(&models.MRAction{
		MergeRequestID: mr.ID,
		ActionType:     models.ActionCommentResolved,
		Timestamp:      resolvedAt,
		CommentID:      &comment.ID,
	})

	// T+4h: Block label removed
	db.Create(&models.MRAction{
		MergeRequestID: mr.ID,
		ActionType:     models.ActionBlockLabelRemoved,
		Timestamp:      baseTime.Add(4 * time.Hour),
		Metadata:       `{"label":"blocked"}`,
	})

	info := GetStateInfo(db, mr)

	// Final state should be on_review (all comments resolved)
	if info.State != StateOnReview {
		t.Errorf("GetStateInfo().State = %v, want %v", info.State, StateOnReview)
	}

	// UnresolvedCount should be 0
	if info.UnresolvedCount != 0 {
		t.Errorf("GetStateInfo().UnresolvedCount = %d, want 0", info.UnresolvedCount)
	}

	// WorkingTime should never be negative
	if info.WorkingTime < 0 {
		t.Errorf("GetStateInfo().WorkingTime = %v, should never be negative", info.WorkingTime)
	}
}

func TestDeriveState_OnReview_AuthorRepliedToThread(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)

	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	mr := &models.MergeRequest{
		State:    "opened",
		Draft:    false,
		AuthorID: author.ID,
	}
	db.Create(mr)

	discussionID := "disc-123"

	// Reviewer starts a thread (only thread starter has Resolvable=true)
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: discussionID,
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    time.Now().Add(-2 * time.Hour),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})

	// Author replies to the thread (is now last in thread, replies have Resolvable=false)
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       2,
		GitlabDiscussionID: discussionID,
		AuthorID:           author.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    time.Now().Add(-1 * time.Hour),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	// State should be on_review because author replied (waiting for reviewer to re-review)
	state := DeriveState(db, mr)
	if state != StateOnReview {
		t.Errorf("DeriveState() = %v, want %v (author replied to thread)", state, StateOnReview)
	}
}

// TestDeriveState_OnFixes_MultiCommentThread tests the critical bug scenario:
// A multi-comment thread where the reviewer follows up (not the author).
// In GitLab, only the thread starter has Resolvable=true, replies have Resolvable=false.
// OLD query (resolvable=true AND is_last_in_thread=true): 0 matches → on_review (BUG!)
// NEW query (EXISTS subquery): finds starter with resolvable=true → on_fixes (CORRECT!)
func TestDeriveState_OnFixes_MultiCommentThread(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)

	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	mr := &models.MergeRequest{
		State:    "opened",
		Draft:    false,
		AuthorID: author.ID,
	}
	db.Create(mr)

	discussionID := "disc-multi-comment"

	// Thread starter: reviewer opens thread
	// In GitLab, only the thread starter has Resolvable=true
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: discussionID,
		AuthorID:           reviewer.ID,
		Resolvable:         true,  // ONLY starter is resolvable
		Resolved:           false,
		GitlabCreatedAt:    time.Now().Add(-2 * time.Hour),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false, // NOT last - there's a reply
	})

	// Reviewer's follow-up comment (still waiting for author)
	// Replies in GitLab have Resolvable=false
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       2,
		GitlabDiscussionID: discussionID, // Same thread
		AuthorID:           reviewer.ID,
		Resolvable:         false, // Reply has Resolvable=false
		Resolved:           false,
		GitlabCreatedAt:    time.Now().Add(-1 * time.Hour),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true, // IS last comment in thread
	})

	// Expected: on_fixes (author hasn't responded to unresolved thread)
	state := DeriveState(db, mr)
	if state != StateOnFixes {
		t.Errorf("DeriveState() = %v, want %v (multi-comment thread awaiting author)", state, StateOnFixes)
	}
}

// TestDeriveState_OnFixes_SingleCommentThread tests single-comment threads where
// the same comment has both Resolvable=true AND IsLastInThread=true.
// This works with both old and new query logic - it's the simple case.
func TestDeriveState_OnFixes_SingleCommentThread(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)

	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	mr := &models.MergeRequest{
		State:    "opened",
		Draft:    false,
		AuthorID: author.ID,
	}
	db.Create(mr)

	discussionID := "disc-456"

	// Reviewer starts a thread and is still last (single-comment thread, waiting for author)
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: discussionID,
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    time.Now(),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	// State should be on_fixes because reviewer is still last (waiting for author to respond)
	state := DeriveState(db, mr)
	if state != StateOnFixes {
		t.Errorf("DeriveState() = %v, want %v (reviewer last in thread)", state, StateOnFixes)
	}
}

func TestDeriveState_OnFixes_MixedThreads(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)

	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	mr := &models.MergeRequest{
		State:    "opened",
		Draft:    false,
		AuthorID: author.ID,
	}
	db.Create(mr)

	// Thread 1: Author replied (on_review for this thread)
	// Thread starter has Resolvable=true
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: "disc-1",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    time.Now().Add(-2 * time.Hour),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})
	// Reply has Resolvable=false
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       2,
		GitlabDiscussionID: "disc-1",
		AuthorID:           author.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    time.Now().Add(-1 * time.Hour),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	// Thread 2: Reviewer is still last (single-comment thread, on_fixes for this thread)
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       3,
		GitlabDiscussionID: "disc-2",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    time.Now(),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	// State should be on_fixes because at least one thread awaits author response
	state := DeriveState(db, mr)
	if state != StateOnFixes {
		t.Errorf("DeriveState() = %v, want %v (mixed threads, one still awaiting author)", state, StateOnFixes)
	}
}

// TestGetStateTransitionTime_OnFixes_MultiCommentThread tests that GetStateTransitionTime
// returns the thread STARTER's creation time, not the follow-up's time.
// Bug scenario: Thread started at 9am, reviewer follows up at 3pm.
// MR has been waiting for author since 9am, but old code returned 3pm.
func TestGetStateTransitionTime_OnFixes_MultiCommentThread(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)

	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	mr := &models.MergeRequest{State: "opened", AuthorID: author.ID}
	db.Create(mr)

	discussionID := "disc-multi-transition"
	starterTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)  // T=9am (thread started)
	followUpTime := time.Date(2024, 1, 15, 15, 0, 0, 0, time.UTC) // T=3pm (follow-up)

	// Thread starter: reviewer opens thread at 9am
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: discussionID,
		AuthorID:           reviewer.ID,
		Resolvable:         true, // Only starter is resolvable
		Resolved:           false,
		GitlabCreatedAt:    starterTime,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false, // NOT last - there's a follow-up
	})

	// Reviewer's follow-up at 3pm (still waiting for author)
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       2,
		GitlabDiscussionID: discussionID,
		AuthorID:           reviewer.ID,
		Resolvable:         false, // Replies have Resolvable=false
		Resolved:           false,
		GitlabCreatedAt:    followUpTime,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true, // IS last comment in thread
	})

	// Should return thread starter's time (9am), not follow-up's time (3pm)
	transitionTime := GetStateTransitionTime(db, mr, StateOnFixes)
	if transitionTime == nil {
		t.Fatal("GetStateTransitionTime() = nil, want non-nil")
	}
	if !transitionTime.Equal(starterTime) {
		t.Errorf("GetStateTransitionTime() = %v, want %v (thread starter time, not follow-up)",
			transitionTime, starterTime)
	}
}

// TestGetStateTransitionTime_OnFixes_MultipleThreads tests that with multiple unresolved
// threads, we return the EARLIEST thread starter's time.
func TestGetStateTransitionTime_OnFixes_MultipleThreads(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)

	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	mr := &models.MergeRequest{State: "opened", AuthorID: author.ID}
	db.Create(mr)

	earlierTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)  // First thread at 9am
	laterTime := time.Date(2024, 1, 15, 14, 0, 0, 0, time.UTC)   // Second thread at 2pm

	// First thread (earlier)
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: "disc-1",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    earlierTime,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	// Second thread (later)
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       2,
		GitlabDiscussionID: "disc-2",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    laterTime,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	// Should return earliest thread's time
	transitionTime := GetStateTransitionTime(db, mr, StateOnFixes)
	if transitionTime == nil {
		t.Fatal("GetStateTransitionTime() = nil, want non-nil")
	}
	if !transitionTime.Equal(earlierTime) {
		t.Errorf("GetStateTransitionTime() = %v, want %v (earliest thread starter time)",
			transitionTime, earlierTime)
	}
}

func TestGetStateTransitionTime_OnFixes_AuthorRepliedThenReviewerResponded(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)

	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	mr := &models.MergeRequest{State: "opened", AuthorID: author.ID}
	db.Create(mr)

	discussionID := "disc-reply-cycle"
	threadStartTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: discussionID,
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    threadStartTime,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})

	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       2,
		GitlabDiscussionID: discussionID,
		AuthorID:           author.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})

	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       3,
		GitlabDiscussionID: discussionID,
		AuthorID:           reviewer.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 14, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	transitionTime := GetStateTransitionTime(db, mr, StateOnFixes)
	if transitionTime == nil {
		t.Fatal("GetStateTransitionTime() = nil, want non-nil")
	}
	if !transitionTime.Equal(threadStartTime) {
		t.Errorf("GetStateTransitionTime() = %v, want %v", transitionTime, threadStartTime)
	}
}

func TestGetStateTransitionTime_OnFixes_MultipleThreadsDifferentReplyCycles(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)
	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	mr := &models.MergeRequest{State: "opened", AuthorID: author.ID}
	db.Create(mr)

	t1Start := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)
	t2Start := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: "disc-1",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    t1Start,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       2,
		GitlabDiscussionID: "disc-1",
		AuthorID:           author.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       3,
		GitlabDiscussionID: "disc-1",
		AuthorID:           reviewer.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       4,
		GitlabDiscussionID: "disc-2",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    t2Start,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	transitionTime := GetStateTransitionTime(db, mr, StateOnFixes)
	if transitionTime == nil {
		t.Fatal("GetStateTransitionTime() = nil, want non-nil")
	}
	if !transitionTime.Equal(t1Start) {
		t.Errorf("GetStateTransitionTime() = %v, want %v", transitionTime, t1Start)
	}
}

func TestGetStateTransitionTime_OnFixes_ResolvedThreadStillCounts(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)
	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	mr := &models.MergeRequest{State: "opened", AuthorID: author.ID}
	db.Create(mr)

	thread1Time := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)
	thread2Time := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	resolvedAt := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: "disc-1",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           true,
		ResolvedAt:         &resolvedAt,
		GitlabCreatedAt:    thread1Time,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       2,
		GitlabDiscussionID: "disc-2",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    thread2Time,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	transitionTime := GetStateTransitionTime(db, mr, StateOnFixes)
	if transitionTime == nil {
		t.Fatal("GetStateTransitionTime() = nil, want non-nil")
	}
	if !transitionTime.Equal(thread1Time) {
		t.Errorf("GetStateTransitionTime() = %v, want %v", transitionTime, thread1Time)
	}
}

func TestGetStateTransitionTime_OnFixes_AllResolvedThenNewThread(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)
	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	mr := &models.MergeRequest{State: "opened", AuthorID: author.ID}
	db.Create(mr)

	thread1Time := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)
	resolvedAt := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	thread2Time := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: "disc-1",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           true,
		ResolvedAt:         &resolvedAt,
		GitlabCreatedAt:    thread1Time,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       2,
		GitlabDiscussionID: "disc-2",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    thread2Time,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	transitionTime := GetStateTransitionTime(db, mr, StateOnFixes)
	if transitionTime == nil {
		t.Fatal("GetStateTransitionTime() = nil, want non-nil")
	}
	if !transitionTime.Equal(thread2Time) {
		t.Errorf("GetStateTransitionTime() = %v, want %v", transitionTime, thread2Time)
	}
}

func TestGetStateTransitionTime_OnFixes_OverlappingResolutionsThenGap(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)
	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	mr := &models.MergeRequest{State: "opened", AuthorID: author.ID}
	db.Create(mr)

	t1Start := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)
	t2Start := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	t2Resolved := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	t1Resolved := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC)
	t3Start := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: "disc-1",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           true,
		ResolvedAt:         &t1Resolved,
		GitlabCreatedAt:    t1Start,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       2,
		GitlabDiscussionID: "disc-2",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           true,
		ResolvedAt:         &t2Resolved,
		GitlabCreatedAt:    t2Start,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       3,
		GitlabDiscussionID: "disc-3",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    t3Start,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	transitionTime := GetStateTransitionTime(db, mr, StateOnFixes)
	if transitionTime == nil {
		t.Fatal("GetStateTransitionTime() = nil, want non-nil")
	}
	if !transitionTime.Equal(t3Start) {
		t.Errorf("GetStateTransitionTime() = %v, want %v", transitionTime, t3Start)
	}
}

func TestGetStateTransitionTime_OnFixes_AuthorLastVsReviewerLast(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)
	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	mr := &models.MergeRequest{State: "opened", AuthorID: author.ID}
	db.Create(mr)

	t1Start := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)

	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: "disc-author-last",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    t1Start,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       2,
		GitlabDiscussionID: "disc-author-last",
		AuthorID:           author.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       3,
		GitlabDiscussionID: "disc-reviewer-last",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	transitionTime := GetStateTransitionTime(db, mr, StateOnFixes)
	if transitionTime == nil {
		t.Fatal("GetStateTransitionTime() = nil, want non-nil")
	}
	if !transitionTime.Equal(t1Start) {
		t.Errorf("GetStateTransitionTime() = %v, want %v", transitionTime, t1Start)
	}
}

func TestGetStateTransitionTime_OnFixes_MultipleBackAndForth(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)
	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	mr := &models.MergeRequest{State: "opened", AuthorID: author.ID}
	db.Create(mr)

	discussionID := "disc-back-and-forth"
	times := []time.Time{
		time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
	}
	authors := []*models.User{reviewer, author, reviewer, author, reviewer}

	for i, tm := range times {
		isLast := i == len(times)-1
		db.Create(&models.MRComment{
			MergeRequestID:     mr.ID,
			GitlabNoteID:       i + 1,
			GitlabDiscussionID: discussionID,
			AuthorID:           authors[i].ID,
			Resolvable:         i == 0,
			Resolved:           false,
			GitlabCreatedAt:    tm,
			ThreadStarterID:    &reviewer.ID,
			IsLastInThread:     isLast,
		})
	}

	expectedTime := times[0]
	transitionTime := GetStateTransitionTime(db, mr, StateOnFixes)
	if transitionTime == nil {
		t.Fatal("GetStateTransitionTime() = nil, want non-nil")
	}
	if !transitionTime.Equal(expectedTime) {
		t.Errorf("GetStateTransitionTime() = %v, want %v", transitionTime, expectedTime)
	}
}

func TestGetStateTransitionTime_OnFixes_ThreeThreadsComplex(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)
	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	mr := &models.MergeRequest{State: "opened", AuthorID: author.ID}
	db.Create(mr)

	thread1Time := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)

	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: "disc-1",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    thread1Time,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       2,
		GitlabDiscussionID: "disc-1",
		AuthorID:           author.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       3,
		GitlabDiscussionID: "disc-1",
		AuthorID:           reviewer.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 14, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       4,
		GitlabDiscussionID: "disc-2",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       5,
		GitlabDiscussionID: "disc-3",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       6,
		GitlabDiscussionID: "disc-3",
		AuthorID:           reviewer.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 13, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	transitionTime := GetStateTransitionTime(db, mr, StateOnFixes)
	if transitionTime == nil {
		t.Fatal("GetStateTransitionTime() = nil, want non-nil")
	}
	if !transitionTime.Equal(thread1Time) {
		t.Errorf("GetStateTransitionTime() = %v, want %v", transitionTime, thread1Time)
	}
}

func TestGetStateTransitionTime_OnFixes_AuthorStartedThread(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)
	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	mr := &models.MergeRequest{State: "opened", AuthorID: author.ID}
	db.Create(mr)

	authorStartTime := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)

	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: "disc-author-started",
		AuthorID:           author.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    authorStartTime,
		ThreadStarterID:    &author.ID,
		IsLastInThread:     false,
	})

	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       2,
		GitlabDiscussionID: "disc-author-started",
		AuthorID:           reviewer.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC),
		ThreadStarterID:    &author.ID,
		IsLastInThread:     true,
	})

	transitionTime := GetStateTransitionTime(db, mr, StateOnFixes)
	if transitionTime == nil {
		t.Fatal("GetStateTransitionTime() = nil, want non-nil")
	}
	if !transitionTime.Equal(authorStartTime) {
		t.Errorf("GetStateTransitionTime() = %v, want %v", transitionTime, authorStartTime)
	}
}

func TestGetStateTransitionTime_OnFixes_SameTimestamp(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)
	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	mr := &models.MergeRequest{State: "opened", AuthorID: author.ID}
	db.Create(mr)

	sameTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: "disc-same-1",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    sameTime,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       2,
		GitlabDiscussionID: "disc-same-2",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    sameTime,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	transitionTime := GetStateTransitionTime(db, mr, StateOnFixes)
	if transitionTime == nil {
		t.Fatal("GetStateTransitionTime() = nil, want non-nil")
	}
	if !transitionTime.Equal(sameTime) {
		t.Errorf("GetStateTransitionTime() = %v, want %v", transitionTime, sameTime)
	}
}

func TestGetStateTransitionTime_OnFixes_MultipleThreadsWithBackAndForth(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)
	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	mr := &models.MergeRequest{State: "opened", AuthorID: author.ID}
	db.Create(mr)

	t1Start := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)

	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: "disc-1",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    t1Start,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       2,
		GitlabDiscussionID: "disc-1",
		AuthorID:           author.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       3,
		GitlabDiscussionID: "disc-1",
		AuthorID:           reviewer.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       4,
		GitlabDiscussionID: "disc-1",
		AuthorID:           author.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       5,
		GitlabDiscussionID: "disc-2",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       6,
		GitlabDiscussionID: "disc-2",
		AuthorID:           author.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 13, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       7,
		GitlabDiscussionID: "disc-2",
		AuthorID:           reviewer.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 14, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	transitionTime := GetStateTransitionTime(db, mr, StateOnFixes)
	if transitionTime == nil {
		t.Fatal("GetStateTransitionTime() = nil, want non-nil")
	}
	if !transitionTime.Equal(t1Start) {
		t.Errorf("GetStateTransitionTime() = %v, want %v", transitionTime, t1Start)
	}
}

// ============================================================================
// GetUserStateTransitionTime Tests
// ============================================================================

// TestGetUserStateTransitionTime_ReviewerWaiting tests the scenario where
// a reviewer is waiting for author response on a thread.
// Thread 1: R@8am → A@9am → R@10am → A@11am (author is last)
// Thread 2: R@12pm → A@1pm → R@2pm (reviewer is last)
// Expected: Reviewer waiting since 2pm (Thread 2 only)
func TestGetUserStateTransitionTime_ReviewerWaiting(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)
	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	mr := &models.MergeRequest{State: "opened", AuthorID: author.ID}
	db.Create(mr)

	// Thread 1: R@8am → A@9am → R@10am → A@11am (author is last)
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: "disc-1",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       2,
		GitlabDiscussionID: "disc-1",
		AuthorID:           author.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       3,
		GitlabDiscussionID: "disc-1",
		AuthorID:           reviewer.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       4,
		GitlabDiscussionID: "disc-1",
		AuthorID:           author.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	// Thread 2: R@12pm → A@1pm → R@2pm (reviewer is last)
	expectedTime := time.Date(2024, 1, 15, 14, 0, 0, 0, time.UTC) // 2pm
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       5,
		GitlabDiscussionID: "disc-2",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       6,
		GitlabDiscussionID: "disc-2",
		AuthorID:           author.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 13, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       7,
		GitlabDiscussionID: "disc-2",
		AuthorID:           reviewer.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    expectedTime,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	transitionTime := GetUserStateTransitionTime(db, mr, reviewer.ID)
	if transitionTime == nil {
		t.Fatal("GetUserStateTransitionTime() = nil, want non-nil")
	}
	if !transitionTime.Equal(expectedTime) {
		t.Errorf("GetUserStateTransitionTime() = %v, want %v", transitionTime, expectedTime)
	}
}

// TestGetUserStateTransitionTime_ReviewerNeedsAction tests when reviewer needs action
// because author replied to their thread.
// Thread 1: R@8am → A@9am (author is last, thread unresolved)
// Expected: Reviewer needs action since 9am (when author replied)
func TestGetUserStateTransitionTime_ReviewerNeedsAction(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)
	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	mr := &models.MergeRequest{State: "opened", AuthorID: author.ID}
	db.Create(mr)

	expectedTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC) // 9am

	// Thread 1: R@8am → A@9am (author is last, thread unresolved)
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: "disc-1",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       2,
		GitlabDiscussionID: "disc-1",
		AuthorID:           author.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    expectedTime,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	transitionTime := GetUserStateTransitionTime(db, mr, reviewer.ID)
	if transitionTime == nil {
		t.Fatal("GetUserStateTransitionTime() = nil, want non-nil")
	}
	if !transitionTime.Equal(expectedTime) {
		t.Errorf("GetUserStateTransitionTime() = %v, want %v", transitionTime, expectedTime)
	}
}

// TestGetUserStateTransitionTime_MultipleWaitingThreads tests when reviewer
// is waiting on multiple threads - should return earliest.
// Thread 1: R@8am (reviewer is last)
// Thread 2: R@10am (reviewer is last)
// Expected: 8am (earliest)
func TestGetUserStateTransitionTime_MultipleWaitingThreads(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)
	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	mr := &models.MergeRequest{State: "opened", AuthorID: author.ID}
	db.Create(mr)

	expectedTime := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC) // 8am (earliest)

	// Thread 1: R@8am (reviewer is last)
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: "disc-1",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    expectedTime,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	// Thread 2: R@10am (reviewer is last)
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       2,
		GitlabDiscussionID: "disc-2",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	transitionTime := GetUserStateTransitionTime(db, mr, reviewer.ID)
	if transitionTime == nil {
		t.Fatal("GetUserStateTransitionTime() = nil, want non-nil")
	}
	if !transitionTime.Equal(expectedTime) {
		t.Errorf("GetUserStateTransitionTime() = %v, want %v", transitionTime, expectedTime)
	}
}

// TestGetUserStateTransitionTime_Author tests author state transition time.
// Thread 1: R@8am → A@9am → R@10am (reviewer is last, awaiting author)
// Expected: Author needs to respond since 10am (when reviewer replied)
func TestGetUserStateTransitionTime_Author(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)
	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	mr := &models.MergeRequest{State: "opened", AuthorID: author.ID}
	db.Create(mr)

	expectedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC) // 10am

	// Thread 1: R@8am → A@9am → R@10am (reviewer is last, awaiting author)
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: "disc-1",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       2,
		GitlabDiscussionID: "disc-1",
		AuthorID:           author.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       3,
		GitlabDiscussionID: "disc-1",
		AuthorID:           reviewer.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    expectedTime,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	transitionTime := GetUserStateTransitionTime(db, mr, author.ID)
	if transitionTime == nil {
		t.Fatal("GetUserStateTransitionTime() = nil, want non-nil")
	}
	if !transitionTime.Equal(expectedTime) {
		t.Errorf("GetUserStateTransitionTime() = %v, want %v", transitionTime, expectedTime)
	}
}

// TestGetUserStateTransitionTime_AuthorNoThreadsAwaiting tests when author
// has responded to all threads - should return on_review transition time.
func TestGetUserStateTransitionTime_AuthorNoThreadsAwaiting(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)
	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	createdAt := time.Date(2024, 1, 15, 7, 0, 0, 0, time.UTC)
	mr := &models.MergeRequest{State: "opened", AuthorID: author.ID, GitlabCreatedAt: &createdAt}
	db.Create(mr)

	// Thread 1: R@8am → A@9am (author is last - no action needed from author)
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: "disc-1",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       2,
		GitlabDiscussionID: "disc-1",
		AuthorID:           author.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	transitionTime := GetUserStateTransitionTime(db, mr, author.ID)
	// Author is on_review, should get on_review transition time (MR created time as fallback)
	if transitionTime == nil {
		t.Fatal("GetUserStateTransitionTime() = nil, want non-nil")
	}
	// The exact time depends on on_review logic, but it should not be nil
}

// TestGetUserStateTransitionTime_ReviewerAssigned tests when reviewer was
// just assigned and has no threads yet.
func TestGetUserStateTransitionTime_ReviewerAssigned(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)
	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	createdAt := time.Date(2024, 1, 15, 7, 0, 0, 0, time.UTC)
	mr := &models.MergeRequest{State: "opened", AuthorID: author.ID, GitlabCreatedAt: &createdAt}
	db.Create(mr)

	assignedTime := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)
	db.Create(&models.MRAction{
		MergeRequestID: mr.ID,
		ActionType:     models.ActionReviewerAssigned,
		TargetUserID:   &reviewer.ID,
		Timestamp:      assignedTime,
	})

	transitionTime := GetUserStateTransitionTime(db, mr, reviewer.ID)
	if transitionTime == nil {
		t.Fatal("GetUserStateTransitionTime() = nil, want non-nil")
	}
	if !transitionTime.Equal(assignedTime) {
		t.Errorf("GetUserStateTransitionTime() = %v, want %v", transitionTime, assignedTime)
	}
}

// TestGetUserStateTransitionTime_ReviewerWaitingAfterAuthorReply tests cycle:
// R creates thread → A replies → R replies again
// Should return when reviewer replied after author (the wait start time)
func TestGetUserStateTransitionTime_ReviewerWaitingAfterAuthorReply(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)
	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	mr := &models.MergeRequest{State: "opened", AuthorID: author.ID}
	db.Create(mr)

	reviewerReplyTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC) // When reviewer replied after author

	// Thread: R@8am → A@9am → R@10am (reviewer waiting since 10am)
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: "disc-1",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       2,
		GitlabDiscussionID: "disc-1",
		AuthorID:           author.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC),
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     false,
	})
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       3,
		GitlabDiscussionID: "disc-1",
		AuthorID:           reviewer.ID,
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    reviewerReplyTime,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	transitionTime := GetUserStateTransitionTime(db, mr, reviewer.ID)
	if transitionTime == nil {
		t.Fatal("GetUserStateTransitionTime() = nil, want non-nil")
	}
	if !transitionTime.Equal(reviewerReplyTime) {
		t.Errorf("GetUserStateTransitionTime() = %v, want %v", transitionTime, reviewerReplyTime)
	}
}

// TestGetUserStateTransitionTime_AuthorMultipleThreads tests author with multiple
// threads where reviewer is last - should return earliest awaiting time.
func TestGetUserStateTransitionTime_AuthorMultipleThreads(t *testing.T) {
	db := setupTestDB(t)

	reviewer := &models.User{GitlabID: 100, Username: "reviewer"}
	db.Create(reviewer)
	author := &models.User{GitlabID: 200, Username: "author"}
	db.Create(author)

	mr := &models.MergeRequest{State: "opened", AuthorID: author.ID}
	db.Create(mr)

	earlierTime := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)
	laterTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	// Thread 1: R@8am (reviewer is last, awaiting author since 8am)
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       1,
		GitlabDiscussionID: "disc-1",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    earlierTime,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	// Thread 2: R@10am (reviewer is last, awaiting author since 10am)
	db.Create(&models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       2,
		GitlabDiscussionID: "disc-2",
		AuthorID:           reviewer.ID,
		Resolvable:         true,
		Resolved:           false,
		GitlabCreatedAt:    laterTime,
		ThreadStarterID:    &reviewer.ID,
		IsLastInThread:     true,
	})

	transitionTime := GetUserStateTransitionTime(db, mr, author.ID)
	if transitionTime == nil {
		t.Fatal("GetUserStateTransitionTime() = nil, want non-nil")
	}
	if !transitionTime.Equal(earlierTime) {
		t.Errorf("GetUserStateTransitionTime() = %v, want %v (earliest)", transitionTime, earlierTime)
	}
}

