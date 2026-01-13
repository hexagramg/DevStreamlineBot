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

// NewUserFactory creates a new UserFactory.
func NewUserFactory(db *gorm.DB) *UserFactory {
	return &UserFactory{db: db}
}

// UserOption is a functional option for creating users.
type UserOption func(*models.User)

// WithOnVacation sets the user as on vacation.
func WithOnVacation() UserOption {
	return func(u *models.User) { u.OnVacation = true }
}

// WithUsername sets a specific username.
func WithUsername(username string) UserOption {
	return func(u *models.User) { u.Username = username }
}

// WithEmail sets a specific email.
func WithEmail(email string) UserOption {
	return func(u *models.User) { u.Email = email }
}

// WithGitlabID sets a specific GitLab ID.
func WithGitlabID(id int) UserOption {
	return func(u *models.User) { u.GitlabID = id }
}

// Create creates a new user with the given options.
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

// NewRepositoryFactory creates a new RepositoryFactory.
func NewRepositoryFactory(db *gorm.DB) *RepositoryFactory {
	return &RepositoryFactory{db: db}
}

// RepoOption is a functional option for creating repositories.
type RepoOption func(*models.Repository)

// WithRepoName sets a specific repository name.
func WithRepoName(name string) RepoOption {
	return func(r *models.Repository) { r.Name = name }
}

// WithRepoGitlabID sets a specific GitLab ID.
func WithRepoGitlabID(id int) RepoOption {
	return func(r *models.Repository) { r.GitlabID = id }
}

// Create creates a new repository with the given options.
func (f *RepositoryFactory) Create(opts ...RepoOption) models.Repository {
	f.counter++
	repo := models.Repository{
		GitlabID:    f.counter * 1000,
		Name:        fmt.Sprintf("repo%d", f.counter),
		Description: fmt.Sprintf("Test Repository %d", f.counter),
		WebURL:      fmt.Sprintf("https://gitlab.example.com/group/repo%d", f.counter),
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

// NewMergeRequestFactory creates a new MergeRequestFactory.
func NewMergeRequestFactory(db *gorm.DB) *MergeRequestFactory {
	return &MergeRequestFactory{db: db}
}

// MROption is a functional option for creating merge requests.
type MROption func(*models.MergeRequest)

// WithDraft sets the MR as draft.
func WithDraft() MROption {
	return func(mr *models.MergeRequest) { mr.Draft = true }
}

// WithMRState sets the MR state.
func WithMRState(state string) MROption {
	return func(mr *models.MergeRequest) { mr.State = state }
}

// WithTitle sets the MR title.
func WithTitle(title string) MROption {
	return func(mr *models.MergeRequest) { mr.Title = title }
}

// WithLabels adds labels to the MR.
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

// WithMRGitlabID sets a specific GitLab ID.
func WithMRGitlabID(id int) MROption {
	return func(mr *models.MergeRequest) { mr.GitlabID = id }
}

// WithCreatedAt sets the GitlabCreatedAt timestamp.
func WithCreatedAt(t time.Time) MROption {
	return func(mr *models.MergeRequest) { mr.GitlabCreatedAt = &t }
}

// Create creates a new merge request with the given options.
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

// NewChatFactory creates a new ChatFactory.
func NewChatFactory(db *gorm.DB) *ChatFactory {
	return &ChatFactory{db: db}
}

// Create creates a new chat.
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

// NewVKUserFactory creates a new VKUserFactory.
func NewVKUserFactory(db *gorm.DB) *VKUserFactory {
	return &VKUserFactory{db: db}
}

// Create creates a new VK user.
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

// CreateLabelReviewer creates a label reviewer association.
func CreateLabelReviewer(db *gorm.DB, repo models.Repository, labelName string, user models.User) models.LabelReviewer {
	lr := models.LabelReviewer{
		RepositoryID: repo.ID,
		LabelName:    labelName,
		UserID:       user.ID,
	}
	db.Create(&lr)
	return lr
}

// CreatePossibleReviewer creates a default reviewer association.
func CreatePossibleReviewer(db *gorm.DB, repo models.Repository, user models.User) models.PossibleReviewer {
	pr := models.PossibleReviewer{
		RepositoryID: repo.ID,
		UserID:       user.ID,
	}
	db.Create(&pr)
	return pr
}

// CreateRepositorySLA creates SLA settings for a repository.
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

// CreateSubscription creates a repository subscription for a chat.
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

// WithActor sets the actor for the action.
func WithActor(user models.User) ActionOption {
	return func(a *models.MRAction) { a.ActorID = &user.ID }
}

// WithTargetUser sets the target user for the action.
func WithTargetUser(user models.User) ActionOption {
	return func(a *models.MRAction) { a.TargetUserID = &user.ID }
}

// WithTimestamp sets the timestamp for the action.
func WithTimestamp(t time.Time) ActionOption {
	return func(a *models.MRAction) { a.Timestamp = t }
}

// WithMetadata sets metadata for the action.
func WithMetadata(metadata string) ActionOption {
	return func(a *models.MRAction) { a.Metadata = metadata }
}

// WithCommentID sets the comment ID for the action.
func WithCommentID(commentID uint) ActionOption {
	return func(a *models.MRAction) { a.CommentID = &commentID }
}

// CreateMRAction creates an MR action.
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

// WithResolvable sets the comment as resolvable.
func WithResolvable() CommentOption {
	return func(c *models.MRComment) { c.Resolvable = true }
}

// WithResolved sets the comment as resolved.
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

// CreateMRComment creates an MR comment.
func CreateMRComment(db *gorm.DB, mr models.MergeRequest, author models.User, noteID int, opts ...CommentOption) models.MRComment {
	comment := models.MRComment{
		MergeRequestID:  mr.ID,
		GitlabNoteID:    noteID,
		AuthorID:        author.ID,
		Body:            fmt.Sprintf("Comment %d", noteID),
		Resolvable:      false,
		Resolved:        false,
		GitlabCreatedAt: time.Now(),
		GitlabUpdatedAt: time.Now(),
	}
	for _, opt := range opts {
		opt(&comment)
	}
	db.Create(&comment)
	return comment
}

// AssignReviewers assigns reviewers to an MR.
func AssignReviewers(db *gorm.DB, mr *models.MergeRequest, reviewers ...models.User) {
	db.Model(mr).Association("Reviewers").Append(reviewers)
}

// CreateHoliday creates a holiday for a repository.
func CreateHoliday(db *gorm.DB, repo models.Repository, date time.Time) models.Holiday {
	holiday := models.Holiday{
		RepositoryID: repo.ID,
		Date:         date,
	}
	db.Create(&holiday)
	return holiday
}
