package polling

import (
	"testing"
	"time"

	"devstreamlinebot/models"
	"devstreamlinebot/testutils"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// TestRecordMRAction_NewAction tests that a new action is created.
func TestRecordMRAction_NewAction(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	actorID := author.ID
	now := time.Now()

	recordMRAction(db, mr.ID, models.ActionApproved, &actorID, nil, nil, now, "")

	var actions []models.MRAction
	db.Where("merge_request_id = ?", mr.ID).Find(&actions)

	if len(actions) != 1 {
		t.Fatalf("Expected 1 action, got %d", len(actions))
	}
	if actions[0].ActionType != models.ActionApproved {
		t.Errorf("Expected action type %s, got %s", models.ActionApproved, actions[0].ActionType)
	}
	if *actions[0].ActorID != actorID {
		t.Errorf("Expected actor ID %d, got %d", actorID, *actions[0].ActorID)
	}
}

// TestRecordMRAction_DuplicateWithin1Min tests that duplicates within 1 minute are skipped.
func TestRecordMRAction_DuplicateWithin1Min(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	actorID := author.ID
	now := time.Now()

	// Record first action
	recordMRAction(db, mr.ID, models.ActionApproved, &actorID, nil, nil, now, "")

	// Try to record same action 30 seconds later
	recordMRAction(db, mr.ID, models.ActionApproved, &actorID, nil, nil, now.Add(30*time.Second), "")

	var actions []models.MRAction
	db.Where("merge_request_id = ?", mr.ID).Find(&actions)

	if len(actions) != 1 {
		t.Errorf("Expected 1 action (duplicate skipped), got %d", len(actions))
	}
}

// TestRecordMRAction_DifferentActor tests that different actors create separate actions.
func TestRecordMRAction_DifferentActor(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	actor1 := userFactory.Create()
	actor2 := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	now := time.Now()

	// Record actions from different actors
	recordMRAction(db, mr.ID, models.ActionApproved, &actor1.ID, nil, nil, now, "")
	recordMRAction(db, mr.ID, models.ActionApproved, &actor2.ID, nil, nil, now, "")

	var actions []models.MRAction
	db.Where("merge_request_id = ?", mr.ID).Find(&actions)

	if len(actions) != 2 {
		t.Errorf("Expected 2 actions (different actors), got %d", len(actions))
	}
}

// TestRecordMRAction_DifferentType tests that different action types are not duplicates.
func TestRecordMRAction_DifferentType(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	actorID := author.ID
	now := time.Now()

	// Record different action types
	recordMRAction(db, mr.ID, models.ActionApproved, &actorID, nil, nil, now, "")
	recordMRAction(db, mr.ID, models.ActionCommentAdded, &actorID, nil, nil, now, "")

	var actions []models.MRAction
	db.Where("merge_request_id = ?", mr.ID).Find(&actions)

	if len(actions) != 2 {
		t.Errorf("Expected 2 actions (different types), got %d", len(actions))
	}
}

// TestRecordMRAction_After1Min tests that actions after 1 minute are not considered duplicates.
func TestRecordMRAction_After1Min(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	actorID := author.ID
	now := time.Now()

	// Record first action
	recordMRAction(db, mr.ID, models.ActionApproved, &actorID, nil, nil, now, "")

	// Record same action 2 minutes later (outside dedup window)
	recordMRAction(db, mr.ID, models.ActionApproved, &actorID, nil, nil, now.Add(2*time.Minute), "")

	var actions []models.MRAction
	db.Where("merge_request_id = ?", mr.ID).Find(&actions)

	if len(actions) != 2 {
		t.Errorf("Expected 2 actions (outside dedup window), got %d", len(actions))
	}
}

// TestRecordMRAction_NullableFields tests handling of nullable fields.
func TestRecordMRAction_NullableFields(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	now := time.Now()

	// Record action with all nullable fields as nil
	recordMRAction(db, mr.ID, models.ActionMerged, nil, nil, nil, now, "")

	var actions []models.MRAction
	db.Where("merge_request_id = ?", mr.ID).Find(&actions)

	if len(actions) != 1 {
		t.Fatalf("Expected 1 action, got %d", len(actions))
	}
	if actions[0].ActorID != nil {
		t.Error("Expected ActorID to be nil")
	}
	if actions[0].TargetUserID != nil {
		t.Error("Expected TargetUserID to be nil")
	}
	if actions[0].CommentID != nil {
		t.Error("Expected CommentID to be nil")
	}
}

// TestRecordMRAction_WithMetadata tests metadata is stored correctly.
func TestRecordMRAction_WithMetadata(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	now := time.Now()
	metadata := `{"draft":true}`

	recordMRAction(db, mr.ID, models.ActionDraftToggled, nil, nil, nil, now, metadata)

	var actions []models.MRAction
	db.Where("merge_request_id = ?", mr.ID).Find(&actions)

	if len(actions) != 1 {
		t.Fatalf("Expected 1 action, got %d", len(actions))
	}
	if actions[0].Metadata != metadata {
		t.Errorf("Expected metadata %q, got %q", metadata, actions[0].Metadata)
	}
}

// TestDetectAndRecordStateChanges_DraftToggleTrue tests draft toggle detection (to true).
func TestDetectAndRecordStateChanges_DraftToggleTrue(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	existingMR := mrFactory.Create(repo, author)
	existingMR.Draft = false

	newMR := &gitlab.BasicMergeRequest{
		ID:    existingMR.GitlabID,
		IID:   existingMR.IID,
		Draft: true,
		State: "opened",
	}

	detectAndRecordStateChanges(db, &existingMR, newMR, existingMR.ID)

	var actions []models.MRAction
	db.Where("merge_request_id = ? AND action_type = ?", existingMR.ID, models.ActionDraftToggled).Find(&actions)

	if len(actions) != 1 {
		t.Fatalf("Expected 1 draft toggle action, got %d", len(actions))
	}
	if actions[0].Metadata != `{"draft":true}` {
		t.Errorf("Expected metadata {\"draft\":true}, got %s", actions[0].Metadata)
	}
}

// TestDetectAndRecordStateChanges_DraftToggleFalse tests draft toggle detection (to false).
func TestDetectAndRecordStateChanges_DraftToggleFalse(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	existingMR := mrFactory.Create(repo, author, testutils.WithDraft())

	newMR := &gitlab.BasicMergeRequest{
		ID:    existingMR.GitlabID,
		IID:   existingMR.IID,
		Draft: false,
		State: "opened",
	}

	detectAndRecordStateChanges(db, &existingMR, newMR, existingMR.ID)

	var actions []models.MRAction
	db.Where("merge_request_id = ? AND action_type = ?", existingMR.ID, models.ActionDraftToggled).Find(&actions)

	if len(actions) != 1 {
		t.Fatalf("Expected 1 draft toggle action, got %d", len(actions))
	}
	if actions[0].Metadata != `{"draft":false}` {
		t.Errorf("Expected metadata {\"draft\":false}, got %s", actions[0].Metadata)
	}
}

// TestDetectAndRecordStateChanges_Merged tests merge detection.
func TestDetectAndRecordStateChanges_Merged(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	existingMR := mrFactory.Create(repo, author)

	mergedAt := time.Now()
	newMR := &gitlab.BasicMergeRequest{
		ID:       existingMR.GitlabID,
		IID:      existingMR.IID,
		Draft:    false,
		State:    "merged",
		MergedAt: &mergedAt,
	}

	detectAndRecordStateChanges(db, &existingMR, newMR, existingMR.ID)

	var actions []models.MRAction
	db.Where("merge_request_id = ? AND action_type = ?", existingMR.ID, models.ActionMerged).Find(&actions)

	if len(actions) != 1 {
		t.Fatalf("Expected 1 merged action, got %d", len(actions))
	}
}

// TestDetectAndRecordStateChanges_Closed tests close detection.
func TestDetectAndRecordStateChanges_Closed(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	existingMR := mrFactory.Create(repo, author)

	closedAt := time.Now()
	newMR := &gitlab.BasicMergeRequest{
		ID:       existingMR.GitlabID,
		IID:      existingMR.IID,
		Draft:    false,
		State:    "closed",
		ClosedAt: &closedAt,
	}

	detectAndRecordStateChanges(db, &existingMR, newMR, existingMR.ID)

	var actions []models.MRAction
	db.Where("merge_request_id = ? AND action_type = ?", existingMR.ID, models.ActionClosed).Find(&actions)

	if len(actions) != 1 {
		t.Fatalf("Expected 1 closed action, got %d", len(actions))
	}
}

// TestDetectAndRecordStateChanges_NoChange tests that no action is recorded when nothing changed.
func TestDetectAndRecordStateChanges_NoChange(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	existingMR := mrFactory.Create(repo, author)

	newMR := &gitlab.BasicMergeRequest{
		ID:    existingMR.GitlabID,
		IID:   existingMR.IID,
		Draft: existingMR.Draft,
		State: existingMR.State,
	}

	detectAndRecordStateChanges(db, &existingMR, newMR, existingMR.ID)

	var actions []models.MRAction
	db.Where("merge_request_id = ?", existingMR.ID).Find(&actions)

	if len(actions) != 0 {
		t.Errorf("Expected 0 actions (no change), got %d", len(actions))
	}
}

// TestDetectAndRecordStateChanges_ExistingMRNil tests behavior when existingMR is nil.
func TestDetectAndRecordStateChanges_ExistingMRNil(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	newMR := &gitlab.BasicMergeRequest{
		ID:    mr.GitlabID,
		IID:   mr.IID,
		Draft: true,
		State: "opened",
	}

	// Should not panic and should not record any actions
	detectAndRecordStateChanges(db, nil, newMR, mr.ID)

	var actions []models.MRAction
	db.Where("merge_request_id = ?", mr.ID).Find(&actions)

	if len(actions) != 0 {
		t.Errorf("Expected 0 actions (nil existingMR), got %d", len(actions))
	}
}

// TestDetectAndRecordStateChanges_AlreadyMerged tests that no duplicate merge action is recorded.
func TestDetectAndRecordStateChanges_AlreadyMerged(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	existingMR := mrFactory.Create(repo, author, testutils.WithMRState("merged"))

	newMR := &gitlab.BasicMergeRequest{
		ID:    existingMR.GitlabID,
		IID:   existingMR.IID,
		Draft: false,
		State: "merged",
	}

	detectAndRecordStateChanges(db, &existingMR, newMR, existingMR.ID)

	var actions []models.MRAction
	db.Where("merge_request_id = ? AND action_type = ?", existingMR.ID, models.ActionMerged).Find(&actions)

	if len(actions) != 0 {
		t.Errorf("Expected 0 merge actions (already merged), got %d", len(actions))
	}
}

// TestRecordMRAction_WithCommentID tests that comment ID is stored correctly.
func TestRecordMRAction_WithCommentID(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)
	comment := testutils.CreateMRComment(db, mr, author, 12345)

	now := time.Now()
	recordMRAction(db, mr.ID, models.ActionCommentAdded, &author.ID, nil, &comment.ID, now, "")

	var actions []models.MRAction
	db.Where("merge_request_id = ?", mr.ID).Find(&actions)

	if len(actions) != 1 {
		t.Fatalf("Expected 1 action, got %d", len(actions))
	}
	if actions[0].CommentID == nil || *actions[0].CommentID != comment.ID {
		t.Errorf("Expected comment ID %d, got %v", comment.ID, actions[0].CommentID)
	}
}

// TestRecordMRAction_DifferentCommentIDs tests that different comment IDs are not duplicates.
func TestRecordMRAction_DifferentCommentIDs(t *testing.T) {
	db := testutils.SetupTestDB(t)
	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)
	comment1 := testutils.CreateMRComment(db, mr, author, 111)
	comment2 := testutils.CreateMRComment(db, mr, author, 222)

	now := time.Now()
	recordMRAction(db, mr.ID, models.ActionCommentAdded, &author.ID, nil, &comment1.ID, now, "")
	recordMRAction(db, mr.ID, models.ActionCommentAdded, &author.ID, nil, &comment2.ID, now, "")

	var actions []models.MRAction
	db.Where("merge_request_id = ?", mr.ID).Find(&actions)

	if len(actions) != 2 {
		t.Errorf("Expected 2 actions (different comment IDs), got %d", len(actions))
	}
}
