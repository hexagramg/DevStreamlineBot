package consumers

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"time"

	botgolang "github.com/mail-ru-im/bot-golang"
	gitlab "gitlab.com/gitlab-org/api/client-go"
	"gorm.io/gorm"

	"devstreamlinebot/models"
)

// MRReviewerConsumer periodically assigns reviewers to new merge requests and notifies chats.
type MRReviewerConsumer struct {
	db        *gorm.DB
	vkBot     *botgolang.Bot
	glClient  *gitlab.Client
	interval  time.Duration
	startTime time.Time // New field to track when consumer started
}

// NewMRReviewerConsumer initializes a new MRReviewerConsumer.
func NewMRReviewerConsumer(db *gorm.DB, vkBot *botgolang.Bot, glClient *gitlab.Client, interval time.Duration) *MRReviewerConsumer {
	return &MRReviewerConsumer{
		db:        db,
		vkBot:     vkBot,
		glClient:  glClient,
		interval:  interval,
		startTime: time.Now().AddDate(0, 0, -1), // Initialize with current time minus one day
	}
}

// Start begins the polling loop for assigning reviewers.
func (c *MRReviewerConsumer) StartConsumer() {
	ticker := time.NewTicker(c.interval)
	go func() {
		defer ticker.Stop()
		for range ticker.C {
			c.assignReviewers()
		}
	}()
}

// assignReviewers finds unreviewed MRs, picks a reviewer, notifies chats, and marks MR with reviewer.
func (c *MRReviewerConsumer) assignReviewers() {
	var mrs []models.MergeRequest
	// load open, not draft, not merged, without existing reviewers or approvers,
	// and for repositories that have subscriptions
	if err := c.db.
		Preload("Repository").Preload("Author").
		Where("merge_requests.state = ? AND merge_requests.draft = ? AND merge_requests.merged_at IS NULL", "opened", false).
		Where("NOT EXISTS (SELECT 1 FROM merge_request_reviewers WHERE merge_request_reviewers.merge_request_id = merge_requests.id)").
		Where("NOT EXISTS (SELECT 1 FROM merge_request_approvers WHERE merge_request_approvers.merge_request_id = merge_requests.id)").
		Where("EXISTS (SELECT 1 FROM repository_subscriptions WHERE repository_subscriptions.repository_id = merge_requests.repository_id)").
		Where("merge_requests.gitlab_created_at > ?", c.startTime). // Only process MRs created after consumer start
		Find(&mrs).Error; err != nil {
		log.Printf("failed to fetch merge requests: %v", err)
		return
	}

	if len(mrs) == 0 {
		// No MRs need reviewers at this moment.
		return
	}

	for _, mr := range mrs {
		// fetch possible reviewers and batch-load users to avoid N+1 queries
		var prs []models.PossibleReviewer
		if err := c.db.Where("repository_id = ?", mr.RepositoryID).Find(&prs).Error; err != nil {
			log.Printf("failed to fetch possible reviewers: %v", err)
			continue
		}
		if len(prs) == 0 {
			continue
		}
		ids := make([]uint, len(prs))
		for i, pr := range prs {
			ids[i] = pr.UserID
		}
		var users []models.User
		if err := c.db.Where("id IN ? AND id <> ?", ids, mr.AuthorID).Find(&users).Error; err != nil || len(users) == 0 {
			continue
		}
		// select reviewer by normal distribution
		idx := pickReviewer(len(users))
		reviewer := users[idx]

		// assign reviewer in GitLab
		if _, _, err := c.glClient.MergeRequests.UpdateMergeRequest(
			int(mr.Repository.GitlabID), mr.IID,
			&gitlab.UpdateMergeRequestOptions{ReviewerIDs: &[]int{int(reviewer.GitlabID)}},
		); err != nil {
			log.Printf("failed to assign reviewer in GitLab: %v", err)
			continue
		}

		// notify all subscribed chats
		var subs []models.RepositorySubscription
		if err := c.db.Preload("Chat").Where("repository_id = ?", mr.RepositoryID).Find(&subs).Error; err != nil {
			log.Printf("failed to fetch subscriptions: %v", err)
			continue
		}
		for _, sub := range subs {
			chatID := sub.Chat.ChatID

			// Prepare author mention
			authorMention := mr.Author.Email
			if authorMention == "" {
				// Look for VKUser with matching username prefix
				var vkUser models.VKUser
				if err := c.db.Where("user_id LIKE ?", mr.Author.Username+"%").First(&vkUser).Error; err == nil {
					authorMention = vkUser.UserID
				} else {
					authorMention = mr.Author.Username // Fallback to username without mention
				}
			}

			// Prepare reviewer mention
			reviewerMention := reviewer.Email
			if reviewerMention == "" {
				// Look for VKUser with matching username prefix
				var vkUser models.VKUser
				if err := c.db.Where("user_id LIKE ?", reviewer.Username+"%").First(&vkUser).Error; err == nil {
					reviewerMention = vkUser.UserID
				} else {
					reviewerMention = reviewer.Username // Fallback to username without mention
				}
			}

			text := fmt.Sprintf(
				"%s\n%s\n by @[%s] reviewer: @[%s]",
				mr.Title,
				mr.WebURL,
				authorMention,
				reviewerMention,
			)
			msg := c.vkBot.NewTextMessage(chatID, text)
			if err := msg.Send(); err != nil {
				log.Printf("failed to send review assignment: %v", err)
			}
		}
		// mark MR as having a reviewer to avoid repeated notifications
		if err := c.db.Model(&mr).Association("Reviewers").Append(&reviewer); err != nil {
			log.Printf("failed to mark MR reviewer: %v", err)
		}
	}
}

// pickReviewer returns an index from [0,n) using a normal distribution centered at mid.
func pickReviewer(n int) int {
	mean := float64(n-1) / 2.0
	sigma := float64(n) / 4.0
	var idx int
	// clamp generated index to valid range
	for {
		v := rand.NormFloat64()*sigma + mean
		idx = int(math.Round(v))
		if idx >= 0 && idx < n {
			break
		}
	}
	return idx
}
