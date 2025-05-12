package models

import (
	"time"

	"gorm.io/gorm"
)

// Repository represents a GitLab project tracked in the system.
type Repository struct {
	gorm.Model
	GitlabID      int `gorm:"uniqueIndex;not null"`
	Name          string
	Description   string
	WebURL        string
	MergeRequests []MergeRequest
	Subscriptions []RepositorySubscription
}

// User represents a GitLab user stored in the database.
type User struct {
	gorm.Model

	GitlabID     int `gorm:"uniqueIndex;not null"`
	Username     string
	Name         string
	State        string
	Locked       bool
	CreatedAt    *time.Time
	AvatarURL    string
	WebURL       string
	Email        string `gorm:"index"`
	EmailFetched bool   `gorm:"default:false"`

	UpdatedAt *time.Time

	AuthoredMergeRequests []MergeRequest `gorm:"foreignKey:AuthorID"`
	AssignedMergeRequests []MergeRequest `gorm:"foreignKey:AssigneeID"`
	ReviewedMergeRequests []MergeRequest `gorm:"many2many:merge_request_reviewers"`
}

// Label represents a GitLab label.
type Label struct {
	gorm.Model
	GitlabID               int    `json:"id"`
	Name                   string `gorm:"uniqueIndex:idx_label_name;not null"`
	Color                  string
	TextColor              string
	Description            string
	OpenIssuesCount        int
	ClosedIssuesCount      int
	OpenMergeRequestsCount int
	Subscribed             bool
	Priority               int
	IsProjectLabel         bool
	MergeRequests          []MergeRequest `gorm:"many2many:merge_request_labels"`
}

// IssueReferences maps MR reference paths.
type IssueReferences struct {
	Short    string `json:"short"`
	Relative string `json:"relative"`
	Full     string `json:"full"`
}

// TimeStats holds MR time estimates and spent data.
type TimeStats struct {
	HumanTimeEstimate   string `json:"human_time_estimate"`
	HumanTotalTimeSpent string `json:"human_total_time_spent"`
	TimeEstimate        int    `json:"time_estimate"`
	TotalTimeSpent      int    `json:"total_time_spent"`
}

// Milestone represents a GitLab milestone.
type Milestone struct {
	gorm.Model
	GitlabID      int `json:"id" gorm:"not null;index"`
	IID           int `json:"iid"`
	GroupID       int `json:"group_id"`
	ProjectID     int `json:"project_id"`
	Title         string
	Description   string
	StartDate     *time.Time `json:"start_date"`
	DueDate       *time.Time `json:"due_date"`
	State         string
	WebURL        string     `json:"web_url"`
	UpdatedAt     *time.Time `json:"updated_at"`
	CreatedAt     *time.Time `json:"created_at"`
	Expired       *bool      `json:"expired"`
	MergeRequests []MergeRequest
}

// MergeRequest represents a GitLab merge request in our DB.
type MergeRequest struct {
	gorm.Model

	// GitLab identifiers
	GitlabID int `json:"id" gorm:"not null;index:idx_mr_project_iid,unique"`  // global MR ID
	IID      int `json:"iid" gorm:"not null;index:idx_mr_project_iid,unique"` // project-scoped MR IID

	// Branch info
	SourceBranch string
	TargetBranch string

	// Metadata
	Title                    string
	Description              string
	State                    string
	WebURL                   string
	Upvotes                  int
	Downvotes                int
	DiscussionLocked         bool
	ShouldRemoveSourceBranch bool
	ForceRemoveSourceBranch  bool

	// Additional GitLab fields
	Imported                    bool
	ImportedFrom                string
	SourceProjectID             int
	TargetProjectID             int
	Draft                       bool
	MergeWhenPipelineSucceeds   bool
	DetailedMergeStatus         string
	SHA                         string
	MergeCommitSHA              string
	SquashCommitSHA             string
	Squash                      bool
	SquashOnMerge               bool
	UserNotesCount              int
	HasConflicts                bool
	BlockingDiscussionsResolved bool

	// Timestamps from GitLab
	GitlabCreatedAt *time.Time
	GitlabUpdatedAt *time.Time
	MergedAt        *time.Time
	MergeAfter      *time.Time
	PreparedAt      *time.Time
	ClosedAt        *time.Time

	// Last local sync time
	LastUpdate *time.Time `gorm:"index"`

	// Nested data normalized
	Labels     []Label         `gorm:"many2many:merge_request_labels"`
	References IssueReferences `gorm:"embedded;embeddedPrefix:references_"`
	TimeStats  TimeStats       `gorm:"embedded;embeddedPrefix:time_stats_"`

	// Milestone relation
	MilestoneID *uint
	Milestone   *Milestone `gorm:"constraint:OnDelete:SET NULL;"`

	// Associations
	AuthorID    uint
	Author      User
	AssigneeID  uint
	Assignee    User
	MergeUserID uint
	MergeUser   User
	ClosedByID  uint
	ClosedBy    User
	Reviewers   []User `gorm:"many2many:merge_request_reviewers"`
	Approvers   []User `gorm:"many2many:merge_request_approvers"`

	RepositoryID uint
	Repository   Repository
}

// Chat represents a VK Teams chat.
type Chat struct {
	gorm.Model
	ChatID         string `gorm:"uniqueIndex;not null"` // ID of the chat
	Type           string // type: private, group, channel
	FirstName      string
	LastName       string
	Nick           string
	About          string // user about or group/channel description
	Rules          string // rules of the group/channel
	Title          string // title of the chat
	IsBot          bool   // is this chat a bot?
	Public         bool   // is chat public?
	JoinModeration bool   // chat has join moderation?
	InviteLink     string // invite link for chat
}

// VKUser represents a VK Teams user.
type VKUser struct {
	gorm.Model
	UserID    string `gorm:"uniqueIndex;not null;index"` // unique user identifier (email)
	FirstName string
	LastName  string
	Nick      string
	About     string // user about
}

// RepositorySubscription links a VK Teams chat to a GitLab repository for notifications.
type RepositorySubscription struct {
	gorm.Model
	RepositoryID uint `gorm:"not null"`
	Repository   Repository
	ChatID       uint `gorm:"not null"`
	Chat         Chat
	VKUserID     uint `gorm:"not null"` // User who created the subscription
	VKUser       VKUser
	SubscribedAt time.Time `gorm:"not null"`
}

// PossibleReviewer links a GitLab repository with a GitLab user for potential reviewing.
type PossibleReviewer struct {
	gorm.Model
	RepositoryID uint       `gorm:"not null"`
	Repository   Repository `gorm:"constraint:OnDelete:CASCADE;"`
	UserID       uint       `gorm:"not null"`
	User         User       `gorm:"constraint:OnDelete:CASCADE;"`
}

// VKMessage represents a message received by the bot.
type VKMessage struct {
	gorm.Model
	MessageID     string `gorm:"uniqueIndex;not null"` // unique message identifier
	ChatID        uint   `gorm:"not null"`             // reference to Chat
	Chat          Chat
	UserID        uint `gorm:"not null"` // reference to VKUser
	User          VKUser
	ContentType   int // message content type
	Text          string
	FileID        string    // file identifier
	ReplyMsgID    string    // replied message ID
	ForwardMsgID  string    // forwarded message ID
	ForwardChatID string    // forwarded chat ID
	RequestID     string    // request ID from VK Teams
	ParseMode     string    // parse mode: HTML, MarkdownV2
	Deeplink      string    // deeplink for content type Deeplink
	Timestamp     time.Time // message timestamp
}
