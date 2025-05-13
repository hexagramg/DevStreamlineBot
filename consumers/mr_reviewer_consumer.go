package consumers

import (
	"fmt"
	"log"
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
	startTime time.Time
}

// NewMRReviewerConsumer initializes a new MRReviewerConsumer.
func NewMRReviewerConsumer(db *gorm.DB, vkBot *botgolang.Bot, glClient *gitlab.Client, interval time.Duration) *MRReviewerConsumer {
	return &MRReviewerConsumer{
		db:        db,
		vkBot:     vkBot,
		glClient:  glClient,
		interval:  interval,
		startTime: time.Now().AddDate(0, 0, -2),
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
		log.Print("No merge requests need reviewers.")
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
			log.Printf("no possible reviewers for repository %d", mr.RepositoryID)
			continue
		}
		ids := make([]uint, len(prs))
		for i, pr := range prs {
			ids[i] = pr.UserID
		}
		var users []models.User
		if err := c.db.Where("id IN ? AND id <> ?", ids, mr.AuthorID).Find(&users).Error; err != nil || len(users) == 0 {
			log.Printf("failed to fetch users for repository %d: %v", mr.RepositoryID, err)
			continue
		}
		// select reviewer using uniform distribution with balancing
		if len(users) == 0 {
			log.Printf("no possible reviewers for repository %d", mr.RepositoryID)
			continue
		}
		idx := c.pickReviewer(users)
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

// pickReviewer selects a reviewer using a uniform distribution with workload balancing
// taking into account reviewer assignments from the past two weeks
func (c *MRReviewerConsumer) pickReviewer(users []models.User) int {
	if len(users) == 0 {
		return 0
	}

	if len(users) == 1 {
		return 0
	}

	twoWeeksAgo := time.Now().AddDate(0, 0, -14)
	recentReviewCounts := make(map[uint]int)

	userIDs := make([]uint, len(users))
	for i, user := range users {
		userIDs[i] = user.ID
	}

	// Query recent reviewer assignments from the database using the join table
	var reviewerCounts []struct {
		UserID uint
		Count  int
	}

	dbQueryFailed := false
	if err := c.db.Table("merge_request_reviewers").
		Joins("JOIN merge_requests ON merge_requests.id = merge_request_reviewers.merge_request_id").
		Where("merge_requests.gitlab_created_at > ?", twoWeeksAgo).
		Where("merge_request_reviewers.user_id IN ?", userIDs).
		Select("merge_request_reviewers.user_id, COUNT(*) as count").
		Group("merge_request_reviewers.user_id").
		Find(&reviewerCounts).Error; err != nil {
		log.Printf("failed to fetch recent reviewer counts: %v", err)
		dbQueryFailed = true
	} else {
		for _, count := range reviewerCounts {
			recentReviewCounts[count.UserID] = count.Count
		}
	}

	// If database query failed or returned no results, use uniform distribution
	if dbQueryFailed || len(reviewerCounts) == 0 {
		return rand.Intn(len(users))
	}

	// Calculate weights based on historical review counts
	weights := make([]float64, len(users))
	totalWeight := 0.0

	for i, user := range users {
		count := recentReviewCounts[user.ID]
		weight := 1.0 / float64(count+1)
		weights[i] = weight
		totalWeight += weight
	}

	if totalWeight <= 0 {
		// Fallback to uniform distribution if weights calculation failed
		return rand.Intn(len(users))
	}

	// Normalize weights to form a probability distribution
	for i := range weights {
		weights[i] /= totalWeight
	}

	// Select reviewer based on weighted probability
	r := rand.Float64()
	cumulativeWeight := 0.0

	for i, weight := range weights {
		cumulativeWeight += weight
		if r <= cumulativeWeight {
			return i
		}
	}
	log.Printf("Failed to select reviewer based on weights, falling back to random selection")
	// Fallback to random selection if all else fails
	return rand.Intn(len(users))
}
