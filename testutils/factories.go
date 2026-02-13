package testutils

import (
	"fmt"
	"testing"
	"time"

	"devstreamlinebot/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// SetupTestDB creates an in-memory SQLite database for testing with all models migrated.
func SetupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	err = db.AutoMigrate(
		&models.Repository{},
		&models.User{},
		&models.Label{},
		&models.Milestone{},
		&models.MergeRequest{},
		&models.Chat{},
		&models.VKUser{},
		&models.RepositorySubscription{},
		&models.PossibleReviewer{},
		&models.LabelReviewer{},
		&models.RepositorySLA{},
		&models.Holiday{},
		&models.MRAction{},
		&models.MRComment{},
		&models.BlockLabel{},
		&models.ReleaseLabel{},
		&models.ReleaseManager{},
		&models.AutoReleaseBranchConfig{},
		&models.ReleaseReadyLabel{},
		&models.ReleaseSubscription{},
		&models.JiraProjectPrefix{},
		&models.MRNotificationState{},
		&models.FeatureReleaseLabel{},
		&models.FeatureReleaseBranch{},
		&models.DeployTrackingRule{},
		&models.TrackedDeployJob{},
	)
	if err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return db
}

// UserFactory creates test users with auto-incrementing IDs.
type UserFactory struct {
	db      *gorm.DB
	counter int
}

func NewUserFactory(db *gorm.DB) *UserFactory {
	return &UserFactory{db: db}
}

// UserOption is a functional option for creating users.
type UserOption func(*models.User)

func WithOnVacation() UserOption {
	return func(u *models.User) { u.OnVacation = true }
}

func WithUsername(username string) UserOption {
	return func(u *models.User) { u.Username = username }
}

func WithEmail(email string) UserOption {
	return func(u *models.User) { u.Email = email }
}

func WithGitlabID(id int) UserOption {
	return func(u *models.User) { u.GitlabID = id }
}

func (f *UserFactory) Create(opts ...UserOption) models.User {
	f.counter++
	user := models.User{
		GitlabID:   f.counter * 100,
		Username:   fmt.Sprintf("user%d", f.counter),
		Name:       fmt.Sprintf("Test User %d", f.counter),
		State:      "active",
		Email:      fmt.Sprintf("user%d@example.com", f.counter),
		OnVacation: false,
	}
	for _, opt := range opts {
		opt(&user)
	}
	f.db.Create(&user)
	return user
}

// RepositoryFactory creates test repositories.
type RepositoryFactory struct {
	db      *gorm.DB
	counter int
}

func NewRepositoryFactory(db *gorm.DB) *RepositoryFactory {
	return &RepositoryFactory{db: db}
}

// RepoOption is a functional option for creating repositories.
type RepoOption func(*models.Repository)

func WithRepoName(name string) RepoOption {
	return func(r *models.Repository) { r.Name = name }
}

func WithRepoGitlabID(id int) RepoOption {
	return func(r *models.Repository) { r.GitlabID = id }
}

func WithRepoPath(path string) RepoOption {
	return func(r *models.Repository) { r.Path = path }
}

func WithRepoPathWithNamespace(pathWithNamespace string) RepoOption {
	return func(r *models.Repository) { r.PathWithNamespace = pathWithNamespace }
}

func (f *RepositoryFactory) Create(opts ...RepoOption) models.Repository {
	f.counter++
	repo := models.Repository{
		GitlabID:          f.counter * 1000,
		Name:              fmt.Sprintf("repo%d", f.counter),
		Path:              fmt.Sprintf("repo%d", f.counter),
		PathWithNamespace: fmt.Sprintf("group/repo%d", f.counter),
		Description:       fmt.Sprintf("Test Repository %d", f.counter),
		WebURL:            fmt.Sprintf("https://gitlab.example.com/group/repo%d", f.counter),
	}
	for _, opt := range opts {
		opt(&repo)
	}
	f.db.Create(&repo)
	return repo
}

// MergeRequestFactory creates test merge requests.
type MergeRequestFactory struct {
	db      *gorm.DB
	counter int
}

func NewMergeRequestFactory(db *gorm.DB) *MergeRequestFactory {
	return &MergeRequestFactory{db: db}
}

// MROption is a functional option for creating merge requests.
type MROption func(*models.MergeRequest)

func WithDraft() MROption {
	return func(mr *models.MergeRequest) { mr.Draft = true }
}

func WithMRState(state string) MROption {
	return func(mr *models.MergeRequest) { mr.State = state }
}

func WithTitle(title string) MROption {
	return func(mr *models.MergeRequest) { mr.Title = title }
}

func WithLabels(db *gorm.DB, labelNames ...string) MROption {
	return func(mr *models.MergeRequest) {
		var labels []models.Label
		for _, name := range labelNames {
			var label models.Label
			db.Where(models.Label{Name: name}).FirstOrCreate(&label, models.Label{Name: name})
			labels = append(labels, label)
		}
		mr.Labels = labels
	}
}

func WithMRGitlabID(id int) MROption {
	return func(mr *models.MergeRequest) { mr.GitlabID = id }
}

func WithCreatedAt(t time.Time) MROption {
	return func(mr *models.MergeRequest) { mr.GitlabCreatedAt = &t }
}

func (f *MergeRequestFactory) Create(repo models.Repository, author models.User, opts ...MROption) models.MergeRequest {
	f.counter++
	now := time.Now()
	mr := models.MergeRequest{
		GitlabID:        f.counter * 10000,
		IID:             f.counter,
		Title:           fmt.Sprintf("Test MR %d", f.counter),
		Description:     fmt.Sprintf("Description for MR %d", f.counter),
		State:           "opened",
		Draft:           false,
		SourceBranch:    fmt.Sprintf("feature/test-%d", f.counter),
		TargetBranch:    "main",
		RepositoryID:    repo.ID,
		AuthorID:        author.ID,
		GitlabCreatedAt: &now,
		WebURL:          fmt.Sprintf("https://gitlab.example.com/group/repo/-/merge_requests/%d", f.counter),
	}
	for _, opt := range opts {
		opt(&mr)
	}
	f.db.Create(&mr)
	return mr
}

// ChatFactory creates test chats.
type ChatFactory struct {
	db      *gorm.DB
	counter int
}

func NewChatFactory(db *gorm.DB) *ChatFactory {
	return &ChatFactory{db: db}
}

func (f *ChatFactory) Create() models.Chat {
	f.counter++
	chat := models.Chat{
		ChatID: fmt.Sprintf("chat%d@example.com", f.counter),
		Type:   "group",
		Title:  fmt.Sprintf("Test Chat %d", f.counter),
	}
	f.db.Create(&chat)
	return chat
}

// VKUserFactory creates test VK users.
type VKUserFactory struct {
	db      *gorm.DB
	counter int
}

func NewVKUserFactory(db *gorm.DB) *VKUserFactory {
	return &VKUserFactory{db: db}
}

func (f *VKUserFactory) Create() models.VKUser {
	f.counter++
	vkUser := models.VKUser{
		UserID:    fmt.Sprintf("vkuser%d@example.com", f.counter),
		FirstName: fmt.Sprintf("VKUser%d", f.counter),
		LastName:  "Test",
	}
	f.db.Create(&vkUser)
	return vkUser
}

func CreateLabelReviewer(db *gorm.DB, repo models.Repository, labelName string, user models.User) models.LabelReviewer {
	lr := models.LabelReviewer{
		RepositoryID: repo.ID,
		LabelName:    labelName,
		UserID:       user.ID,
	}
	db.Create(&lr)
	return lr
}

func CreatePossibleReviewer(db *gorm.DB, repo models.Repository, user models.User) models.PossibleReviewer {
	pr := models.PossibleReviewer{
		RepositoryID: repo.ID,
		UserID:       user.ID,
	}
	db.Create(&pr)
	return pr
}

func CreateRepositorySLA(db *gorm.DB, repo models.Repository, assignCount int) models.RepositorySLA {
	sla := models.RepositorySLA{
		RepositoryID:   repo.ID,
		ReviewDuration: models.Duration(48 * time.Hour),
		FixesDuration:  models.Duration(48 * time.Hour),
		AssignCount:    assignCount,
	}
	db.Create(&sla)
	return sla
}

func CreateSubscription(db *gorm.DB, repo models.Repository, chat models.Chat, vkUser models.VKUser) models.RepositorySubscription {
	sub := models.RepositorySubscription{
		RepositoryID: repo.ID,
		ChatID:       chat.ID,
		VKUserID:     vkUser.ID,
		SubscribedAt: time.Now(),
	}
	db.Create(&sub)
	return sub
}

// ActionOption is a functional option for creating MR actions.
type ActionOption func(*models.MRAction)

func WithActor(user models.User) ActionOption {
	return func(a *models.MRAction) { a.ActorID = &user.ID }
}

func WithTargetUser(user models.User) ActionOption {
	return func(a *models.MRAction) { a.TargetUserID = &user.ID }
}

func WithTimestamp(t time.Time) ActionOption {
	return func(a *models.MRAction) { a.Timestamp = t }
}

func WithMetadata(metadata string) ActionOption {
	return func(a *models.MRAction) { a.Metadata = metadata }
}

func WithCommentID(commentID uint) ActionOption {
	return func(a *models.MRAction) { a.CommentID = &commentID }
}

func CreateMRAction(db *gorm.DB, mr models.MergeRequest, actionType models.MRActionType, opts ...ActionOption) models.MRAction {
	action := models.MRAction{
		MergeRequestID: mr.ID,
		ActionType:     actionType,
		Timestamp:      time.Now(),
	}
	for _, opt := range opts {
		opt(&action)
	}
	db.Create(&action)
	return action
}

// CommentOption is a functional option for creating MR comments.
type CommentOption func(*models.MRComment)

func WithResolvable() CommentOption {
	return func(c *models.MRComment) {
		c.Resolvable = true
		// In GitLab, resolvable comments are always thread starters.
		// For single-comment threads, they're also the last in thread.
		// Tests can override IsLastInThread=false for multi-comment threads.
		c.ThreadStarterID = &c.AuthorID
		c.IsLastInThread = true
	}
}

func WithResolved(resolvedBy *models.User) CommentOption {
	return func(c *models.MRComment) {
		c.Resolved = true
		if resolvedBy != nil {
			c.ResolvedByID = &resolvedBy.ID
			now := time.Now()
			c.ResolvedAt = &now
		}
	}
}

func WithThreadStarter(user *models.User) CommentOption {
	return func(c *models.MRComment) {
		if user != nil {
			c.ThreadStarterID = &user.ID
		}
	}
}

func WithIsLastInThread() CommentOption {
	return func(c *models.MRComment) { c.IsLastInThread = true }
}

func WithNotLastInThread() CommentOption {
	return func(c *models.MRComment) { c.IsLastInThread = false }
}

func WithDiscussionID(id string) CommentOption {
	return func(c *models.MRComment) { c.GitlabDiscussionID = id }
}

func WithCommentCreatedAt(t time.Time) CommentOption {
	return func(c *models.MRComment) {
		c.GitlabCreatedAt = t
		c.GitlabUpdatedAt = t
	}
}

func CreateMRComment(db *gorm.DB, mr models.MergeRequest, author models.User, noteID int, opts ...CommentOption) models.MRComment {
	comment := models.MRComment{
		MergeRequestID:     mr.ID,
		GitlabNoteID:       noteID,
		GitlabDiscussionID: fmt.Sprintf("disc-mr%d-note%d", mr.ID, noteID),
		AuthorID:           author.ID,
		Body:               fmt.Sprintf("Comment %d", noteID),
		Resolvable:         false,
		Resolved:           false,
		GitlabCreatedAt:    time.Now(),
		GitlabUpdatedAt:    time.Now(),
	}
	for _, opt := range opts {
		opt(&comment)
	}
	db.Create(&comment)
	return comment
}

func AssignReviewers(db *gorm.DB, mr *models.MergeRequest, reviewers ...models.User) {
	db.Model(mr).Association("Reviewers").Append(reviewers)
}

func CreateHoliday(db *gorm.DB, repo models.Repository, date time.Time) models.Holiday {
	holiday := models.Holiday{
		RepositoryID: repo.ID,
		Date:         date,
	}
	db.Create(&holiday)
	return holiday
}

func CreateBlockLabel(db *gorm.DB, repo models.Repository, labelName string) models.BlockLabel {
	bl := models.BlockLabel{
		RepositoryID: repo.ID,
		LabelName:    labelName,
	}
	db.Create(&bl)
	return bl
}

func CreateBlockLabelAction(db *gorm.DB, mr models.MergeRequest, actionType models.MRActionType, label string, timestamp time.Time) models.MRAction {
	action := models.MRAction{
		MergeRequestID: mr.ID,
		ActionType:     actionType,
		Timestamp:      timestamp,
		Metadata:       fmt.Sprintf(`{"label":"%s"}`, label),
	}
	db.Create(&action)
	return action
}

func CreateReleaseLabel(db *gorm.DB, repo models.Repository, labelName string) models.ReleaseLabel {
	rl := models.ReleaseLabel{
		RepositoryID: repo.ID,
		LabelName:    labelName,
	}
	db.Create(&rl)
	return rl
}

func CreateFeatureReleaseLabel(db *gorm.DB, repo models.Repository, labelName string) models.FeatureReleaseLabel {
	frl := models.FeatureReleaseLabel{
		RepositoryID: repo.ID,
		LabelName:    labelName,
	}
	db.Create(&frl)
	return frl
}

func CreateReleaseManager(db *gorm.DB, repo models.Repository, user models.User) models.ReleaseManager {
	rm := models.ReleaseManager{
		RepositoryID: repo.ID,
		UserID:       user.ID,
	}
	db.Create(&rm)
	return rm
}

func AssignApprovers(db *gorm.DB, mr *models.MergeRequest, approvers ...models.User) {
	db.Model(mr).Association("Approvers").Append(approvers)
}

func CreateAutoReleaseBranchConfig(db *gorm.DB, repo models.Repository, prefix, devBranch string) models.AutoReleaseBranchConfig {
	config := models.AutoReleaseBranchConfig{
		RepositoryID:        repo.ID,
		ReleaseBranchPrefix: prefix,
		DevBranchName:       devBranch,
	}
	db.Create(&config)
	return config
}

func CreateReleaseReadyLabel(db *gorm.DB, repo models.Repository, labelName string) models.ReleaseReadyLabel {
	rrl := models.ReleaseReadyLabel{
		RepositoryID: repo.ID,
		LabelName:    labelName,
	}
	db.Create(&rrl)
	return rrl
}

func CreateReleaseSubscription(db *gorm.DB, repo models.Repository, chat models.Chat, vkUser models.VKUser) models.ReleaseSubscription {
	sub := models.ReleaseSubscription{
		RepositoryID: repo.ID,
		ChatID:       chat.ID,
		VKUserID:     vkUser.ID,
		SubscribedAt: time.Now(),
	}
	db.Create(&sub)
	return sub
}

func CreateJiraProjectPrefix(db *gorm.DB, repo models.Repository, prefix string) models.JiraProjectPrefix {
	jp := models.JiraProjectPrefix{
		RepositoryID: repo.ID,
		Prefix:       prefix,
	}
	db.Create(&jp)
	return jp
}

func CreateNotificationState(db *gorm.DB, mr models.MergeRequest, notifiedState string, notifiedDescription string) models.MRNotificationState {
	ns := models.MRNotificationState{
		MergeRequestID:  mr.ID,
		NotifiedState:       notifiedState,
		NotifiedDescription: notifiedDescription,
	}
	db.Create(&ns)
	return ns
}

func CreateDeployTrackingRule(db *gorm.DB, deployProjectPath string, deployProjectID int, jobName string, targetRepo models.Repository, chat models.Chat, vkUser models.VKUser) models.DeployTrackingRule {
	rule := models.DeployTrackingRule{
		DeployProjectPath:  deployProjectPath,
		DeployProjectID:    deployProjectID,
		JobName:            jobName,
		TargetRepositoryID: targetRepo.ID,
		ChatID:             chat.ID,
		CreatedByID:        vkUser.ID,
	}
	db.Create(&rule)
	return rule
}

func CreateTrackedDeployJob(db *gorm.DB, rule models.DeployTrackingRule, gitlabJobID int, status string) models.TrackedDeployJob {
	tracked := models.TrackedDeployJob{
		DeployTrackingRuleID: rule.ID,
		GitlabJobID:         gitlabJobID,
		Status:              status,
		Ref:                 "main",
		TriggeredBy:         "testuser",
		WebURL:              fmt.Sprintf("https://gitlab.example.com/-/jobs/%d", gitlabJobID),
	}
	db.Create(&tracked)
	return tracked
}
