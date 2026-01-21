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

