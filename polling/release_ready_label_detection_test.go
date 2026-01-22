package polling

import (
	"testing"

	"devstreamlinebot/models"
	"devstreamlinebot/testutils"
)

func TestDetectReleaseReadyLabelChanges_NoLabelsConfigured(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	oldLabels := []string{}
	newLabels := []string{"release-ready", "feature"}

	detectReleaseReadyLabelChanges(db, mr.ID, repo.ID, oldLabels, newLabels)

	var actions []models.MRAction
	db.Where("merge_request_id = ?", mr.ID).Find(&actions)

	if len(actions) != 0 {
		t.Errorf("Expected 0 actions, got %d (no release-ready labels configured)", len(actions))
	}
}

func TestDetectReleaseReadyLabelChanges_LabelAdded(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	testutils.CreateReleaseReadyLabel(db, repo, "release-ready")

	oldLabels := []string{"feature"}
	newLabels := []string{"feature", "release-ready"}

	detectReleaseReadyLabelChanges(db, mr.ID, repo.ID, oldLabels, newLabels)

	var actions []models.MRAction
	db.Where("merge_request_id = ? AND action_type = ?", mr.ID, models.ActionReleaseReadyLabelAdded).Find(&actions)

	if len(actions) != 1 {
		t.Errorf("Expected 1 ActionReleaseReadyLabelAdded action, got %d", len(actions))
	}

	if len(actions) > 0 && actions[0].Metadata != `{"label":"release-ready"}` {
		t.Errorf("Expected metadata to be {\"label\":\"release-ready\"}, got %s", actions[0].Metadata)
	}
}

func TestDetectReleaseReadyLabelChanges_NonRelevantLabel(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	testutils.CreateReleaseReadyLabel(db, repo, "release-ready")

	oldLabels := []string{"feature"}
	newLabels := []string{"feature", "urgent"}

	detectReleaseReadyLabelChanges(db, mr.ID, repo.ID, oldLabels, newLabels)

	var actions []models.MRAction
	db.Where("merge_request_id = ?", mr.ID).Find(&actions)

	if len(actions) != 0 {
		t.Errorf("Expected 0 actions (non-release-ready label changes), got %d", len(actions))
	}
}

func TestDetectReleaseReadyLabelChanges_LabelAlreadyPresent(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	testutils.CreateReleaseReadyLabel(db, repo, "release-ready")

	oldLabels := []string{"feature", "release-ready"}
	newLabels := []string{"feature", "release-ready", "urgent"}

	detectReleaseReadyLabelChanges(db, mr.ID, repo.ID, oldLabels, newLabels)

	var actions []models.MRAction
	db.Where("merge_request_id = ? AND action_type = ?", mr.ID, models.ActionReleaseReadyLabelAdded).Find(&actions)

	if len(actions) != 0 {
		t.Errorf("Expected 0 actions (label already present), got %d", len(actions))
	}
}

func TestDetectReleaseReadyLabelChanges_MultipleLabelChanges(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	testutils.CreateReleaseReadyLabel(db, repo, "release-ready")
	testutils.CreateReleaseReadyLabel(db, repo, "prod-ready")

	oldLabels := []string{"feature"}
	newLabels := []string{"feature", "release-ready", "prod-ready"}

	detectReleaseReadyLabelChanges(db, mr.ID, repo.ID, oldLabels, newLabels)

	var actions []models.MRAction
	db.Where("merge_request_id = ? AND action_type = ?", mr.ID, models.ActionReleaseReadyLabelAdded).Find(&actions)

	if len(actions) < 1 {
		t.Errorf("Expected at least 1 ActionReleaseReadyLabelAdded action, got %d", len(actions))
	}
}

func TestDetectReleaseReadyLabelChanges_LabelRemoved(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	testutils.CreateReleaseReadyLabel(db, repo, "release-ready")

	oldLabels := []string{"feature", "release-ready"}
	newLabels := []string{"feature"}

	detectReleaseReadyLabelChanges(db, mr.ID, repo.ID, oldLabels, newLabels)

	var actions []models.MRAction
	db.Where("merge_request_id = ?", mr.ID).Find(&actions)

	if len(actions) != 0 {
		t.Errorf("Expected 0 actions (removal not tracked), got %d", len(actions))
	}
}
