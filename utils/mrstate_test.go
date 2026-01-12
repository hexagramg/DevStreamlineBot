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

	// Create MR
	mr := &models.MergeRequest{
		State: "opened",
		Draft: false,
	}
	db.Create(mr)

	// Create unresolved resolvable comment
	comment := &models.MRComment{
		MergeRequestID:  mr.ID,
		GitlabNoteID:    12345,
		AuthorID:        1,
		Resolvable:      true,
		Resolved:        false,
		GitlabCreatedAt: time.Now(),
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

	// Create MR
	mr := &models.MergeRequest{
		State: "opened",
	}
	db.Create(mr)

	// Create unresolved comment
	commentTime := time.Now().Add(-3 * time.Hour)
	db.Create(&models.MRComment{
		MergeRequestID:  mr.ID,
		GitlabNoteID:    1,
		AuthorID:        1,
		Resolvable:      true,
		Resolved:        false,
		GitlabCreatedAt: commentTime,
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
