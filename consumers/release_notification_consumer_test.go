package consumers

import (
	"strings"
	"testing"

	"devstreamlinebot/mocks"
	"devstreamlinebot/models"
	"devstreamlinebot/testutils"
)

// ============================================================================
// extractNewEntries Unit Tests
// ============================================================================

func TestExtractNewEntries_EmptyOldDescription(t *testing.T) {
	oldDesc := ""
	newDesc := "- [Title](https://gitlab.com/g/p/-/merge_requests/1)"

	entries := extractNewEntries(oldDesc, newDesc)

	if len(entries) != 1 {
		t.Fatalf("Expected 1 new entry, got %d", len(entries))
	}
	if entries[0] != "- [Title](https://gitlab.com/g/p/-/merge_requests/1)" {
		t.Errorf("Expected entry to match, got %s", entries[0])
	}
}

func TestExtractNewEntries_EmptyNewDescription(t *testing.T) {
	oldDesc := "- [Title](https://gitlab.com/g/p/-/merge_requests/1)"
	newDesc := ""

	entries := extractNewEntries(oldDesc, newDesc)

	if len(entries) != 0 {
		t.Errorf("Expected 0 entries for empty new description, got %d", len(entries))
	}
}

func TestExtractNewEntries_BothEmpty(t *testing.T) {
	entries := extractNewEntries("", "")

	if len(entries) != 0 {
		t.Errorf("Expected 0 entries for both empty, got %d", len(entries))
	}
}

func TestExtractNewEntries_IdenticalDescriptions(t *testing.T) {
	desc := "- [MR 1](https://gitlab.com/g/p/-/merge_requests/1)\n- [MR 2](https://gitlab.com/g/p/-/merge_requests/2)"

	entries := extractNewEntries(desc, desc)

	if len(entries) != 0 {
		t.Errorf("Expected 0 entries for identical descriptions, got %d", len(entries))
	}
}

func TestExtractNewEntries_NewEntryAdded(t *testing.T) {
	oldDesc := "- [MR 1](https://gitlab.com/g/p/-/merge_requests/1)"
	newDesc := "- [MR 1](https://gitlab.com/g/p/-/merge_requests/1)\n- [MR 2](https://gitlab.com/g/p/-/merge_requests/2)"

	entries := extractNewEntries(oldDesc, newDesc)

	if len(entries) != 1 {
		t.Fatalf("Expected 1 new entry, got %d", len(entries))
	}
	if !strings.Contains(entries[0], "merge_requests/2") {
		t.Errorf("Expected new entry to contain merge_requests/2, got %s", entries[0])
	}
}

func TestExtractNewEntries_MultipleNewEntries(t *testing.T) {
	oldDesc := "- [MR 1](https://gitlab.com/g/p/-/merge_requests/1)"
	newDesc := `- [MR 1](https://gitlab.com/g/p/-/merge_requests/1)
- [MR 2](https://gitlab.com/g/p/-/merge_requests/2)
- [MR 3](https://gitlab.com/g/p/-/merge_requests/3)`

	entries := extractNewEntries(oldDesc, newDesc)

	if len(entries) != 2 {
		t.Fatalf("Expected 2 new entries, got %d", len(entries))
	}
}

func TestExtractNewEntries_DuplicateURL(t *testing.T) {
	oldDesc := "- [MR 1](https://gitlab.com/g/p/-/merge_requests/1)"
	newDesc := "- [Updated MR 1](https://gitlab.com/g/p/-/merge_requests/1)"

	entries := extractNewEntries(oldDesc, newDesc)

	if len(entries) != 0 {
		t.Errorf("Expected 0 entries for same URL, got %d", len(entries))
	}
}

func TestExtractNewEntries_LineWithoutURL(t *testing.T) {
	oldDesc := ""
	newDesc := "- Some text without URL"

	entries := extractNewEntries(oldDesc, newDesc)

	if len(entries) != 0 {
		t.Errorf("Expected 0 entries for line without URL, got %d", len(entries))
	}
}

func TestExtractNewEntries_LineWithoutDashPrefix(t *testing.T) {
	oldDesc := ""
	newDesc := "[MR 1](https://gitlab.com/g/p/-/merge_requests/1)"

	entries := extractNewEntries(oldDesc, newDesc)

	if len(entries) != 0 {
		t.Errorf("Expected 0 entries for line without dash prefix, got %d", len(entries))
	}
}

func TestExtractNewEntries_NonGitLabURL(t *testing.T) {
	oldDesc := ""
	newDesc := "- [Link](https://example.com/something)"

	entries := extractNewEntries(oldDesc, newDesc)

	if len(entries) != 0 {
		t.Errorf("Expected 0 entries for non-GitLab URL, got %d", len(entries))
	}
}

func TestExtractNewEntries_ComplexMarkdown(t *testing.T) {
	oldDesc := ""
	newDesc := "- [**Bold** MR](https://gitlab.com/g/p/-/merge_requests/1) - extra text [other](https://gitlab.com/g/p/-/merge_requests/2)"

	entries := extractNewEntries(oldDesc, newDesc)

	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry (first URL match), got %d", len(entries))
	}
}

// ============================================================================
// ProcessNewReleaseNotifications Tests
// ============================================================================

func TestProcessNewReleaseNotifications_NoUnnotifiedActions(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	consumer := NewReleaseNotificationConsumerWithBot(db, mockBot)
	consumer.ProcessNewReleaseNotifications()

	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 0 {
		t.Errorf("Expected no messages, got %d", len(sentMessages))
	}
}

func TestProcessNewReleaseNotifications_ActionWithoutReleaseLabel(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	testutils.CreateReleaseSubscription(db, repo, chatFactory.Create(), vkUserFactory.Create())

	action := testutils.CreateMRAction(db, mr, models.ActionReleaseReadyLabelAdded)

	consumer := NewReleaseNotificationConsumerWithBot(db, mockBot)
	consumer.ProcessNewReleaseNotifications()

	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 0 {
		t.Errorf("Expected no messages (no release label), got %d", len(sentMessages))
	}

	var updatedAction models.MRAction
	db.First(&updatedAction, action.ID)
	if !updatedAction.Notified {
		t.Error("Action should be marked as notified")
	}
}

func TestProcessNewReleaseNotifications_MRLacksReleaseLabel(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "release-ready"))

	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateReleaseSubscription(db, repo, chatFactory.Create(), vkUserFactory.Create())

	action := testutils.CreateMRAction(db, mr, models.ActionReleaseReadyLabelAdded)

	consumer := NewReleaseNotificationConsumerWithBot(db, mockBot)
	consumer.ProcessNewReleaseNotifications()

	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 0 {
		t.Errorf("Expected no messages (MR lacks release label), got %d", len(sentMessages))
	}

	var updatedAction models.MRAction
	db.First(&updatedAction, action.ID)
	if !updatedAction.Notified {
		t.Error("Action should be marked as notified")
	}
}

func TestProcessNewReleaseNotifications_NoSubscriptions(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "release"))

	testutils.CreateReleaseLabel(db, repo, "release")

	action := testutils.CreateMRAction(db, mr, models.ActionReleaseReadyLabelAdded)

	consumer := NewReleaseNotificationConsumerWithBot(db, mockBot)
	consumer.ProcessNewReleaseNotifications()

	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 0 {
		t.Errorf("Expected no messages (no subscriptions), got %d", len(sentMessages))
	}

	var updatedAction models.MRAction
	db.First(&updatedAction, action.ID)
	if !updatedAction.Notified {
		t.Error("Action should be marked as notified")
	}
}

func TestProcessNewReleaseNotifications_HappyPath(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create(testutils.WithRepoName("MyProject"))
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "release"))
	db.Model(&mr).Update("description", "## Included MRs\n- [MR](https://gitlab.com/.../merge_requests/1)")

	testutils.CreateReleaseLabel(db, repo, "release")

	chat := chatFactory.Create()
	vkUser := vkUserFactory.Create()
	testutils.CreateReleaseSubscription(db, repo, chat, vkUser)

	action := testutils.CreateMRAction(db, mr, models.ActionReleaseReadyLabelAdded)

	consumer := NewReleaseNotificationConsumerWithBot(db, mockBot)
	consumer.ProcessNewReleaseNotifications()

	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(sentMessages))
	}

	if !strings.Contains(sentMessages[0].Text, "Новый релиз") {
		t.Errorf("Expected message to contain 'Новый релиз'")
	}
	if !strings.Contains(sentMessages[0].Text, "MyProject") {
		t.Errorf("Expected message to contain repo name 'MyProject'")
	}

	var updatedAction models.MRAction
	db.First(&updatedAction, action.ID)
	if !updatedAction.Notified {
		t.Error("Action should be marked as notified")
	}

	var updatedState models.MRNotificationState
	db.Where("merge_request_id = ?", mr.ID).Order("created_at desc").First(&updatedState)
	if updatedState.NotifiedDescription == "" {
		t.Error("NotifiedDescription should be updated in notification state")
	}
}

func TestProcessNewReleaseNotifications_MultipleSubscriptions(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "release"))

	testutils.CreateReleaseLabel(db, repo, "release")

	testutils.CreateReleaseSubscription(db, repo, chatFactory.Create(), vkUserFactory.Create())
	testutils.CreateReleaseSubscription(db, repo, chatFactory.Create(), vkUserFactory.Create())
	testutils.CreateReleaseSubscription(db, repo, chatFactory.Create(), vkUserFactory.Create())

	testutils.CreateMRAction(db, mr, models.ActionReleaseReadyLabelAdded)

	consumer := NewReleaseNotificationConsumerWithBot(db, mockBot)
	consumer.ProcessNewReleaseNotifications()

	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 3 {
		t.Errorf("Expected 3 messages (one per subscription), got %d", len(sentMessages))
	}
}

func TestProcessNewReleaseNotifications_MultipleActions(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()

	mr1 := mrFactory.Create(repo, author, testutils.WithLabels(db, "release"))
	mr2 := mrFactory.Create(repo, author, testutils.WithLabels(db, "release"))

	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateReleaseSubscription(db, repo, chatFactory.Create(), vkUserFactory.Create())

	testutils.CreateMRAction(db, mr1, models.ActionReleaseReadyLabelAdded)
	testutils.CreateMRAction(db, mr2, models.ActionReleaseReadyLabelAdded)

	consumer := NewReleaseNotificationConsumerWithBot(db, mockBot)
	consumer.ProcessNewReleaseNotifications()

	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 2 {
		t.Errorf("Expected 2 messages (one per MR), got %d", len(sentMessages))
	}
}

func TestProcessNewReleaseNotifications_MessageFormat(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create(testutils.WithRepoName("TestRepo"))
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author, testutils.WithTitle("Fixes for redesign"), testutils.WithLabels(db, "release"))
	db.Model(&mr).Update("description", "Release description content")

	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateReleaseSubscription(db, repo, chatFactory.Create(), vkUserFactory.Create())

	testutils.CreateMRAction(db, mr, models.ActionReleaseReadyLabelAdded)

	consumer := NewReleaseNotificationConsumerWithBot(db, mockBot)
	consumer.ProcessNewReleaseNotifications()

	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(sentMessages))
	}

	msg := sentMessages[0].Text
	if !strings.Contains(msg, "Новый релиз") {
		t.Error("Message should contain 'Новый релиз'")
	}
	if !strings.Contains(msg, "TestRepo") {
		t.Error("Message should contain repo name")
	}
	if !strings.Contains(msg, "Fixes for redesign") {
		t.Error("Message should contain MR title")
	}
	if !strings.Contains(msg, "Release description content") {
		t.Error("Message should contain MR description")
	}
}

func TestProcessNewReleaseNotifications_AlreadyNotifiedAction(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "release"))

	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateReleaseSubscription(db, repo, chatFactory.Create(), vkUserFactory.Create())

	action := testutils.CreateMRAction(db, mr, models.ActionReleaseReadyLabelAdded)
	db.Model(&action).Update("notified", true)

	consumer := NewReleaseNotificationConsumerWithBot(db, mockBot)
	consumer.ProcessNewReleaseNotifications()

	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 0 {
		t.Errorf("Expected no messages (already notified), got %d", len(sentMessages))
	}
}

// ============================================================================
// ProcessReleaseMRDescriptionChanges Tests
// ============================================================================

func TestProcessReleaseMRDescriptionChanges_NoSubscriptions(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	consumer := NewReleaseNotificationConsumerWithBot(db, mockBot)
	consumer.ProcessReleaseMRDescriptionChanges()

	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 0 {
		t.Errorf("Expected no messages, got %d", len(sentMessages))
	}
}

func TestProcessReleaseMRDescriptionChanges_MissingReleaseLabel(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mrFactory.Create(repo, author, testutils.WithMRState("opened"))

	testutils.CreateReleaseReadyLabel(db, repo, "release-ready")
	testutils.CreateReleaseSubscription(db, repo, chatFactory.Create(), vkUserFactory.Create())

	consumer := NewReleaseNotificationConsumerWithBot(db, mockBot)
	consumer.ProcessReleaseMRDescriptionChanges()

	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 0 {
		t.Errorf("Expected no messages (missing release label), got %d", len(sentMessages))
	}
}

func TestProcessReleaseMRDescriptionChanges_MissingReleaseReadyLabel(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mrFactory.Create(repo, author, testutils.WithMRState("opened"))

	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateReleaseSubscription(db, repo, chatFactory.Create(), vkUserFactory.Create())

	consumer := NewReleaseNotificationConsumerWithBot(db, mockBot)
	consumer.ProcessReleaseMRDescriptionChanges()

	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 0 {
		t.Errorf("Expected no messages (missing release-ready label), got %d", len(sentMessages))
	}
}

func TestProcessReleaseMRDescriptionChanges_NoOpenMR(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mrFactory.Create(repo, author, testutils.WithMRState("merged"), testutils.WithLabels(db, "release", "release-ready"))

	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateReleaseReadyLabel(db, repo, "release-ready")
	testutils.CreateReleaseSubscription(db, repo, chatFactory.Create(), vkUserFactory.Create())

	consumer := NewReleaseNotificationConsumerWithBot(db, mockBot)
	consumer.ProcessReleaseMRDescriptionChanges()

	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 0 {
		t.Errorf("Expected no messages (no open MR), got %d", len(sentMessages))
	}
}

func TestProcessReleaseMRDescriptionChanges_MRMissingLabels(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mrFactory.Create(repo, author, testutils.WithMRState("opened"), testutils.WithLabels(db, "feature"))

	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateReleaseReadyLabel(db, repo, "release-ready")
	testutils.CreateReleaseSubscription(db, repo, chatFactory.Create(), vkUserFactory.Create())

	consumer := NewReleaseNotificationConsumerWithBot(db, mockBot)
	consumer.ProcessReleaseMRDescriptionChanges()

	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 0 {
		t.Errorf("Expected no messages (MR missing labels), got %d", len(sentMessages))
	}
}

func TestProcessReleaseMRDescriptionChanges_EmptyLastNotified(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author, testutils.WithMRState("opened"), testutils.WithLabels(db, "release", "release-ready"))
	db.Model(&mr).Update("description", "- [MR](https://gitlab.com/g/p/-/merge_requests/1)")

	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateReleaseReadyLabel(db, repo, "release-ready")
	testutils.CreateReleaseSubscription(db, repo, chatFactory.Create(), vkUserFactory.Create())

	consumer := NewReleaseNotificationConsumerWithBot(db, mockBot)
	consumer.ProcessReleaseMRDescriptionChanges()

	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 0 {
		t.Errorf("Expected no messages (empty last notified - first notification handled elsewhere), got %d", len(sentMessages))
	}
}

func TestProcessReleaseMRDescriptionChanges_NoDescriptionChange(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author, testutils.WithMRState("opened"), testutils.WithLabels(db, "release", "release-ready"))

	desc := "- [MR](https://gitlab.com/g/p/-/merge_requests/1)"
	db.Model(&mr).Update("description", desc)
	testutils.CreateNotificationState(db, mr, "", desc)

	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateReleaseReadyLabel(db, repo, "release-ready")
	testutils.CreateReleaseSubscription(db, repo, chatFactory.Create(), vkUserFactory.Create())

	consumer := NewReleaseNotificationConsumerWithBot(db, mockBot)
	consumer.ProcessReleaseMRDescriptionChanges()

	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 0 {
		t.Errorf("Expected no messages (description unchanged), got %d", len(sentMessages))
	}
}

func TestProcessReleaseMRDescriptionChanges_ChangeButNoNewEntries(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author, testutils.WithMRState("opened"), testutils.WithLabels(db, "release", "release-ready"))

	oldDesc := "- [MR](https://gitlab.com/g/p/-/merge_requests/1)"
	newDesc := "- [Updated MR](https://gitlab.com/g/p/-/merge_requests/1)"
	db.Model(&mr).Update("description", newDesc)
	testutils.CreateNotificationState(db, mr, "", oldDesc)

	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateReleaseReadyLabel(db, repo, "release-ready")
	testutils.CreateReleaseSubscription(db, repo, chatFactory.Create(), vkUserFactory.Create())

	consumer := NewReleaseNotificationConsumerWithBot(db, mockBot)
	consumer.ProcessReleaseMRDescriptionChanges()

	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 0 {
		t.Errorf("Expected no messages (no new entries), got %d", len(sentMessages))
	}

	var updatedState models.MRNotificationState
	db.Where("merge_request_id = ?", mr.ID).Order("created_at desc").First(&updatedState)
	if updatedState.NotifiedDescription != newDesc {
		t.Error("NotifiedDescription should be updated even without new entries")
	}
}

func TestProcessReleaseMRDescriptionChanges_HappyPath(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create(testutils.WithRepoName("TestProject"))
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author, testutils.WithMRState("opened"), testutils.WithLabels(db, "release", "release-ready"))

	oldDesc := "- [MR 1](https://gitlab.com/g/p/-/merge_requests/1)"
	newDesc := "- [MR 1](https://gitlab.com/g/p/-/merge_requests/1)\n- [MR 2](https://gitlab.com/g/p/-/merge_requests/2)"
	db.Model(&mr).Update("description", newDesc)
	testutils.CreateNotificationState(db, mr, "", oldDesc)

	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateReleaseReadyLabel(db, repo, "release-ready")
	testutils.CreateReleaseSubscription(db, repo, chatFactory.Create(), vkUserFactory.Create())

	consumer := NewReleaseNotificationConsumerWithBot(db, mockBot)
	consumer.ProcessReleaseMRDescriptionChanges()

	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(sentMessages))
	}

	if !strings.Contains(sentMessages[0].Text, "Добавлена задача в релиз") {
		t.Error("Message should contain 'Добавлена задача в релиз'")
	}
	if !strings.Contains(sentMessages[0].Text, "TestProject") {
		t.Error("Message should contain repo name")
	}
	if !strings.Contains(sentMessages[0].Text, "merge_requests/2") {
		t.Error("Message should contain the new entry")
	}

	var updatedState models.MRNotificationState
	db.Where("merge_request_id = ?", mr.ID).Order("created_at desc").First(&updatedState)
	if updatedState.NotifiedDescription != newDesc {
		t.Error("NotifiedDescription should be updated")
	}
}

func TestProcessReleaseMRDescriptionChanges_MessageFormat(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create(testutils.WithRepoName("FormatTestRepo"))
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author, testutils.WithTitle("Feature Release Test"), testutils.WithMRState("opened"), testutils.WithLabels(db, "release", "release-ready"))

	newDesc := "- [Feature A](https://gitlab.com/g/p/-/merge_requests/100)"
	db.Model(&mr).Update("description", newDesc)
	testutils.CreateNotificationState(db, mr, "", "some old content")

	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateReleaseReadyLabel(db, repo, "release-ready")
	testutils.CreateReleaseSubscription(db, repo, chatFactory.Create(), vkUserFactory.Create())

	consumer := NewReleaseNotificationConsumerWithBot(db, mockBot)
	consumer.ProcessReleaseMRDescriptionChanges()

	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(sentMessages))
	}

	msg := sentMessages[0].Text
	if !strings.Contains(msg, "Добавлена задача в релиз") {
		t.Error("Message should contain 'Добавлена задача в релиз'")
	}
	if !strings.Contains(msg, "FormatTestRepo") {
		t.Error("Message should contain repo name")
	}
	if !strings.Contains(msg, "Feature Release Test") {
		t.Error("Message should contain MR title")
	}
	if !strings.Contains(msg, "Feature A") {
		t.Error("Message should contain the new entry text")
	}
}

func TestProcessReleaseMRDescriptionChanges_MultipleRepos(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo1 := repoFactory.Create(testutils.WithRepoName("Repo1"))
	repo2 := repoFactory.Create(testutils.WithRepoName("Repo2"))
	author := userFactory.Create()

	mr1 := mrFactory.Create(repo1, author, testutils.WithMRState("opened"), testutils.WithLabels(db, "release", "release-ready"))
	mr2 := mrFactory.Create(repo2, author, testutils.WithMRState("opened"), testutils.WithLabels(db, "release", "release-ready"))

	testutils.CreateReleaseLabel(db, repo1, "release")
	testutils.CreateReleaseReadyLabel(db, repo1, "release-ready")
	testutils.CreateReleaseLabel(db, repo2, "release")
	testutils.CreateReleaseReadyLabel(db, repo2, "release-ready")

	testutils.CreateReleaseSubscription(db, repo1, chatFactory.Create(), vkUserFactory.Create())
	testutils.CreateReleaseSubscription(db, repo2, chatFactory.Create(), vkUserFactory.Create())

	db.Model(&mr1).Update("description", "- [MR](https://gitlab.com/g/p/-/merge_requests/1)")
	testutils.CreateNotificationState(db, mr1, "", "old")
	db.Model(&mr2).Update("description", "- [MR](https://gitlab.com/g/p/-/merge_requests/2)")
	testutils.CreateNotificationState(db, mr2, "", "old")

	consumer := NewReleaseNotificationConsumerWithBot(db, mockBot)
	consumer.ProcessReleaseMRDescriptionChanges()

	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 2 {
		t.Errorf("Expected 2 messages (one per repo), got %d", len(sentMessages))
	}
}

func TestProcessReleaseMRDescriptionChanges_DuplicateRepoSubscriptions(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)
	chatFactory := testutils.NewChatFactory(db)
	vkUserFactory := testutils.NewVKUserFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author, testutils.WithMRState("opened"), testutils.WithLabels(db, "release", "release-ready"))

	testutils.CreateReleaseLabel(db, repo, "release")
	testutils.CreateReleaseReadyLabel(db, repo, "release-ready")

	chat1 := chatFactory.Create()
	chat2 := chatFactory.Create()
	testutils.CreateReleaseSubscription(db, repo, chat1, vkUserFactory.Create())
	testutils.CreateReleaseSubscription(db, repo, chat2, vkUserFactory.Create())

	db.Model(&mr).Update("description", "- [MR](https://gitlab.com/g/p/-/merge_requests/1)")
	testutils.CreateNotificationState(db, mr, "", "old")

	consumer := NewReleaseNotificationConsumerWithBot(db, mockBot)
	consumer.ProcessReleaseMRDescriptionChanges()

	sentMessages := mockBot.GetSentMessages()
	if len(sentMessages) != 2 {
		t.Errorf("Expected 2 messages (one per chat), got %d", len(sentMessages))
	}

	chatIDs := make(map[string]bool)
	for _, msg := range sentMessages {
		chatIDs[msg.ChatID] = true
	}
	if !chatIDs[chat1.ChatID] {
		t.Error("Expected message to chat1")
	}
	if !chatIDs[chat2.ChatID] {
		t.Error("Expected message to chat2")
	}
}

// ============================================================================
// markActionNotified Tests
// ============================================================================

func TestMarkActionNotified_Success(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	action := testutils.CreateMRAction(db, mr, models.ActionReleaseReadyLabelAdded)

	consumer := NewReleaseNotificationConsumerWithBot(db, mockBot)
	consumer.markActionNotified(action.ID)

	var updatedAction models.MRAction
	db.First(&updatedAction, action.ID)
	if !updatedAction.Notified {
		t.Error("Action should be marked as notified")
	}
}

func TestMarkActionNotified_NonExistentID(t *testing.T) {
	db := testutils.SetupTestDB(t)
	mockBot := mocks.NewMockVKBot()

	consumer := NewReleaseNotificationConsumerWithBot(db, mockBot)

	// This should not panic - just log an error
	consumer.markActionNotified(99999)
}

func TestConvertToVKHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "plain text passthrough",
			input:    "Hello world",
			expected: "Hello world",
		},
		{
			name:     "horizontal rules stripped",
			input:    "before\n---\nafter\n***\nend\n___\ndone",
			expected: "before\nafter\nend\ndone",
		},
		{
			name:     "headers stripped",
			input:    "# Header 1\n## Header 2\n### Header 3",
			expected: "Header 1\nHeader 2\nHeader 3",
		},
		{
			name:     "markdown link to html link",
			input:    "[Click here](https://example.com)",
			expected: `<a href="https://example.com">Click here</a>`,
		},
		{
			name:     "multiple links on one line",
			input:    "[INTDEV-123](https://jira.example.com/INTDEV-123) [Fix bug](https://gitlab.example.com/mr/1)",
			expected: `<a href="https://jira.example.com/INTDEV-123">INTDEV-123</a> <a href="https://gitlab.example.com/mr/1">Fix bug</a>`,
		},
		{
			name:     "html entities escaped in text",
			input:    "x < y && z > w",
			expected: "x &lt; y &amp;&amp; z &gt; w",
		},
		{
			name:     "html entities escaped inside links too",
			input:    "[A & B](https://example.com?a=1&b=2)",
			expected: `<a href="https://example.com?a=1&amp;b=2">A &amp; B</a>`,
		},
		{
			name:     "at mentions stripped",
			input:    "by @victor.morozov",
			expected: "by victor.morozov",
		},
		{
			name:     "parentheses in link text preserved",
			input:    "[users: поправить(ограничить) права](https://example.com)",
			expected: `<a href="https://example.com">users: поправить(ограничить) права</a>`,
		},
		{
			name:     "list items converted to ul li",
			input:    "- first item\n- second item\n- third item",
			expected: "<ul>\n<li>first item</li>\n<li>second item</li>\n<li>third item</li>\n</ul>",
		},
		{
			name:     "list with non-list text before and after",
			input:    "Header\n- item one\n- item two\nFooter",
			expected: "Header\n<ul>\n<li>item one</li>\n<li>item two</li>\n</ul>\nFooter",
		},
		{
			name: "real world release description",
			input: "---\n## Included MRs\n" +
				"- [INTDEV-42701](https://jira.vk.team/browse/INTDEV-42701) [Фикс блокировки поля сап грейдов](https://gitlab.corp.mail.ru/intdev/jobofferapp/-/merge_requests/2907) by @victor.morozov\n" +
				"- [TBD: Фиксы редизайна](https://gitlab.corp.mail.ru/intdev/jobofferapp/-/merge_requests/3031) by @p.gusev\n" +
				"- [INTDEV-41931](https://jira.vk.team/browse/INTDEV-41931) [users: поправить(ограничить) права рекрутеров](https://gitlab.corp.mail.ru/intdev/jobofferapp/-/merge_requests/2751) by @f.gugnin",
			expected: "Included MRs\n<ul>\n" +
				`<li><a href="https://jira.vk.team/browse/INTDEV-42701">INTDEV-42701</a> <a href="https://gitlab.corp.mail.ru/intdev/jobofferapp/-/merge_requests/2907">Фикс блокировки поля сап грейдов</a> by victor.morozov</li>` + "\n" +
				`<li><a href="https://gitlab.corp.mail.ru/intdev/jobofferapp/-/merge_requests/3031">TBD: Фиксы редизайна</a> by p.gusev</li>` + "\n" +
				`<li><a href="https://jira.vk.team/browse/INTDEV-41931">INTDEV-41931</a> <a href="https://gitlab.corp.mail.ru/intdev/jobofferapp/-/merge_requests/2751">users: поправить(ограничить) права рекрутеров</a> by f.gugnin</li>` + "\n</ul>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertToVKHTML(tt.input)
			if got != tt.expected {
				t.Errorf("convertToVKHTML() =\n%s\nwant:\n%s", got, tt.expected)
			}
		})
	}
}
