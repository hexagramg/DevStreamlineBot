package models

import (
	"database/sql/driver"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Duration is a custom type for storing time.Duration in database as int64 nanoseconds.
// Implements database/sql Scanner and driver.Valuer interfaces for GORM compatibility.
type Duration time.Duration

func (d Duration) Value() (driver.Value, error) {
	return int64(d), nil
}

func (d *Duration) Scan(value interface{}) error {
	if value == nil {
		*d = 0
		return nil
	}
	switch v := value.(type) {
	case int64:
		*d = Duration(v)
	default:
		return fmt.Errorf("cannot scan %T into Duration", value)
	}
	return nil
}

func (d Duration) ToDuration() time.Duration {
	return time.Duration(d)
}

type Repository struct {
	gorm.Model
	GitlabID      int `gorm:"uniqueIndex;not null"`
	Name          string
	Description   string
	WebURL        string
	MergeRequests []MergeRequest
	Subscriptions []RepositorySubscription
}

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
	OnVacation   bool   `gorm:"default:false"` // User is on vacation and should not be assigned as reviewer

	UpdatedAt *time.Time

	AuthoredMergeRequests []MergeRequest `gorm:"foreignKey:AuthorID"`
	AssignedMergeRequests []MergeRequest `gorm:"foreignKey:AssigneeID"`
	ReviewedMergeRequests []MergeRequest `gorm:"many2many:merge_request_reviewers"`
}

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

type IssueReferences struct {
	Short    string `json:"short"`
	Relative string `json:"relative"`
	Full     string `json:"full"`
}

type TimeStats struct {
	HumanTimeEstimate   string `json:"human_time_estimate"`
	HumanTotalTimeSpent string `json:"human_total_time_spent"`
	TimeEstimate        int    `json:"time_estimate"`
	TotalTimeSpent      int    `json:"total_time_spent"`
}

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

type MergeRequest struct {
	gorm.Model

	GitlabID int `json:"id" gorm:"not null;uniqueIndex:idx_mr_gitlab_id"`       // global MR ID, should be unique
	IID      int `json:"iid" gorm:"not null;uniqueIndex:idx_mr_repository_iid"` // project-scoped MR IID, unique within a repository

	SourceBranch string
	TargetBranch string

	Title                    string
	Description              string
	State                    string
	WebURL                   string
	Upvotes                  int
	Downvotes                int
	DiscussionLocked         bool
	ShouldRemoveSourceBranch bool
	ForceRemoveSourceBranch  bool

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

	GitlabCreatedAt *time.Time
	GitlabUpdatedAt *time.Time
	MergedAt        *time.Time
	MergeAfter      *time.Time
	PreparedAt      *time.Time
	ClosedAt        *time.Time

	// Last local sync time
	LastUpdate *time.Time `gorm:"index"`

	// Last state for which DM notification was sent (for state change notifications)
	LastNotifiedState string `gorm:"type:varchar(20)"`

	Labels     []Label         `gorm:"many2many:merge_request_labels"`
	References IssueReferences `gorm:"embedded;embeddedPrefix:references_"`
	TimeStats  TimeStats       `gorm:"embedded;embeddedPrefix:time_stats_"`

	MilestoneID *uint
	Milestone   *Milestone `gorm:"constraint:OnDelete:SET NULL;"`

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

	RepositoryID uint `gorm:"uniqueIndex:idx_mr_repository_iid"` // Part of composite unique key with IID
	Repository   Repository
}

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

type PossibleReviewer struct {
	gorm.Model
	RepositoryID uint       `gorm:"not null"`
	Repository   Repository `gorm:"constraint:OnDelete:CASCADE;"`
	UserID       uint       `gorm:"not null"`
	User         User       `gorm:"constraint:OnDelete:CASCADE;"`
}

// ReleaseManager links a GitLab repository with a user who manages releases.
// Release managers are notified when MRs are fully approved and ready for release.
type ReleaseManager struct {
	gorm.Model
	RepositoryID uint       `gorm:"not null;uniqueIndex:idx_release_manager_unique,priority:1"`
	Repository   Repository `gorm:"constraint:OnDelete:CASCADE;"`
	UserID       uint       `gorm:"not null;uniqueIndex:idx_release_manager_unique,priority:2"`
	User         User       `gorm:"constraint:OnDelete:CASCADE;"`
}

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

// LabelReviewer links a label name to a user for label-based reviewer assignment.
// When an MR has matching labels, these reviewers take priority over the default pool.
type LabelReviewer struct {
	gorm.Model
	RepositoryID uint       `gorm:"not null;uniqueIndex:idx_label_reviewer_unique,priority:1"`
	Repository   Repository `gorm:"constraint:OnDelete:CASCADE;"`
	LabelName    string     `gorm:"not null;uniqueIndex:idx_label_reviewer_unique,priority:2"`
	UserID       uint       `gorm:"not null;uniqueIndex:idx_label_reviewer_unique,priority:3"`
	User         User       `gorm:"constraint:OnDelete:CASCADE;"`
}

// RepositorySLA stores SLA settings per repository.
type RepositorySLA struct {
	gorm.Model
	RepositoryID   uint       `gorm:"uniqueIndex;not null"`
	Repository     Repository `gorm:"constraint:OnDelete:CASCADE;"`
	ReviewDuration Duration   `gorm:"not null;default:172800000000000"` // SLA duration for review phase (default 48h)
	FixesDuration  Duration   `gorm:"not null;default:172800000000000"` // SLA duration for fixes phase (default 48h)
	AssignCount    int        `gorm:"not null;default:1"`               // Number of reviewers to assign
}

// Holiday stores holiday dates per repository for SLA calculation.
type Holiday struct {
	gorm.Model
	RepositoryID uint       `gorm:"not null;uniqueIndex:idx_holiday_unique,priority:1"`
	Repository   Repository `gorm:"constraint:OnDelete:CASCADE;"`
	Date         time.Time  `gorm:"type:date;not null;uniqueIndex:idx_holiday_unique,priority:2"`
}

// BlockLabel stores labels that pause SLA tracking for merge requests.
// When an MR has a block label, its SLA timer is frozen.
type BlockLabel struct {
	gorm.Model
	RepositoryID uint       `gorm:"not null;uniqueIndex:idx_block_label_unique,priority:1"`
	Repository   Repository `gorm:"constraint:OnDelete:CASCADE;"`
	LabelName    string     `gorm:"not null;uniqueIndex:idx_block_label_unique,priority:2"`
}

// ReleaseLabel stores labels that mark MRs to be completely ignored.
// MRs with release labels are excluded from reviewer assignment and digests.
type ReleaseLabel struct {
	gorm.Model
	RepositoryID uint       `gorm:"not null;uniqueIndex:idx_release_label_unique,priority:1"`
	Repository   Repository `gorm:"constraint:OnDelete:CASCADE;"`
	LabelName    string     `gorm:"not null;uniqueIndex:idx_release_label_unique,priority:2"`
}

// AutoReleaseBranchConfig stores auto-release branch settings per repository.
// Only works when a ReleaseLabel is also configured for the repository.
type AutoReleaseBranchConfig struct {
	gorm.Model
	RepositoryID        uint       `gorm:"uniqueIndex;not null"`
	Repository          Repository `gorm:"constraint:OnDelete:CASCADE;"`
	ReleaseBranchPrefix string     `gorm:"not null"`
	DevBranchName       string     `gorm:"not null"`
}

type MRActionType string

const (
	ActionReviewerAssigned  MRActionType = "reviewer_assigned"
	ActionReviewerRemoved   MRActionType = "reviewer_removed"
	ActionCommentAdded      MRActionType = "comment_added"
	ActionCommentResolved   MRActionType = "comment_resolved"
	ActionApproved          MRActionType = "approved"
	ActionUnapproved        MRActionType = "unapproved"
	ActionDraftToggled      MRActionType = "draft_toggled"
	ActionMerged            MRActionType = "merged"
	ActionClosed            MRActionType = "closed"
	ActionBlockLabelAdded   MRActionType = "block_label_added"
	ActionBlockLabelRemoved MRActionType = "block_label_removed"
	ActionFullyApproved     MRActionType = "fully_approved" // All reviewers have approved
)

// MRAction records timestamped actions for MR timeline tracking.
// Used to calculate review time and fix time periods per reviewer and per MR.
type MRAction struct {
	gorm.Model
	MergeRequestID uint         `gorm:"not null;index"`
	MergeRequest   MergeRequest `gorm:"constraint:OnDelete:CASCADE;"`
	ActionType     MRActionType `gorm:"type:varchar(50);not null;index"`
	ActorID        *uint        `gorm:"index"` // User who performed the action (nullable for system actions)
	Actor          *User        `gorm:"constraint:OnDelete:SET NULL;"`
	TargetUserID   *uint        `gorm:"index"` // For reviewer-specific actions (e.g., which reviewer was assigned)
	TargetUser     *User        `gorm:"constraint:OnDelete:SET NULL;"`
	CommentID      *uint        `gorm:"index"` // Reference to MRComment for comment-related actions
	Comment        *MRComment   `gorm:"constraint:OnDelete:SET NULL;"`
	Timestamp      time.Time    `gorm:"not null;index"`
	Metadata       string       `gorm:"type:text"` // JSON for additional context (e.g., draft state)
	Notified       bool         `gorm:"default:false;index"` // Whether DM notification was sent for this action
}

type MRComment struct {
	gorm.Model
	MergeRequestID     uint         `gorm:"not null;index"`
	MergeRequest       MergeRequest `gorm:"constraint:OnDelete:CASCADE;"`
	GitlabNoteID       int          `gorm:"not null;uniqueIndex"` // GitLab note ID
	GitlabDiscussionID string       `gorm:"index"`                // GitLab discussion ID
	AuthorID           uint         `gorm:"not null"`
	Author             User         `gorm:"constraint:OnDelete:CASCADE;"`
	Body               string       `gorm:"type:text"`
	Resolvable         bool         `gorm:"default:false"`
	Resolved           bool         `gorm:"default:false"`
	ResolvedByID       *uint
	ResolvedBy         *User      `gorm:"constraint:OnDelete:SET NULL;"`
	ResolvedAt         *time.Time // From GitLab API resolved_at field
	GitlabCreatedAt    time.Time  `gorm:"not null"`
	GitlabUpdatedAt    time.Time
}

// DailyDigestPreference stores user preferences for personal daily digest notifications.
type DailyDigestPreference struct {
	gorm.Model
	VKUserID       uint   `gorm:"uniqueIndex;not null"` // Foreign key to VKUser
	VKUser         VKUser `gorm:"constraint:OnDelete:CASCADE;"`
	DMChatID       string `gorm:"not null"`            // Chat ID for sending DM
	Enabled        bool   `gorm:"default:false"`       // Whether digest is enabled
	TimezoneOffset int    `gorm:"default:3"`           // Hours from UTC (default +3)
	LastSentAt     *time.Time                          // Track last send to avoid duplicates
}
