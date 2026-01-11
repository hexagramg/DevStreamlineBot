package consumers

import (
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strings"
	"time"

	botgolang "github.com/mail-ru-im/bot-golang"
	gitlab "gitlab.com/gitlab-org/api/client-go"
	"gorm.io/gorm"

	"devstreamlinebot/models"
	"devstreamlinebot/utils"
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
			c.AssignReviewers()
		}
	}()
}

// getLabelReviewerGroups returns label reviewers grouped by label name.
// Only includes labels that the MR has AND have configured reviewers.
// Filters out users on vacation and the MR author.
func (c *MRReviewerConsumer) getLabelReviewerGroups(mr *models.MergeRequest) map[string][]models.User {
	if len(mr.Labels) == 0 {
		return nil
	}

	labelNames := make([]string, len(mr.Labels))
	for i, label := range mr.Labels {
		labelNames[i] = label.Name
	}

	// Fetch all label reviewers for MR's labels
	var labelReviewers []models.LabelReviewer
	if err := c.db.Where("repository_id = ? AND label_name IN ?", mr.RepositoryID, labelNames).
		Find(&labelReviewers).Error; err != nil {
		log.Printf("failed to fetch label reviewers: %v", err)
		return nil
	}

	if len(labelReviewers) == 0 {
		return nil
	}

	// Collect all user IDs
	userIDSet := make(map[uint]bool)
	for _, lr := range labelReviewers {
		userIDSet[lr.UserID] = true
	}
	userIDs := make([]uint, 0, len(userIDSet))
	for id := range userIDSet {
		userIDs = append(userIDs, id)
	}

	// Fetch users, excluding MR author and users on vacation
	var users []models.User
	if err := c.db.Where("id IN ? AND id <> ? AND on_vacation = ?", userIDs, mr.AuthorID, false).
		Find(&users).Error; err != nil {
		log.Printf("failed to fetch label reviewer users: %v", err)
		return nil
	}

	// Build user map for quick lookup
	userMap := make(map[uint]models.User)
	for _, u := range users {
		userMap[u.ID] = u
	}

	// Group by label name (only include users that passed filters)
	groups := make(map[string][]models.User)
	for _, lr := range labelReviewers {
		if user, ok := userMap[lr.UserID]; ok {
			groups[lr.LabelName] = append(groups[lr.LabelName], user)
		}
	}

	// Remove empty groups
	for label, labelUsers := range groups {
		if len(labelUsers) == 0 {
			delete(groups, label)
		}
	}

	if len(groups) == 0 {
		return nil
	}

	log.Printf("Found %d label groups with reviewers for MR %d", len(groups), mr.ID)
	return groups
}

// getDefaultReviewers returns the default reviewer pool for a repository.
// Filters out users on vacation and the MR author.
func (c *MRReviewerConsumer) getDefaultReviewers(mr *models.MergeRequest) []models.User {
	var prs []models.PossibleReviewer
	if err := c.db.Where("repository_id = ?", mr.RepositoryID).Find(&prs).Error; err != nil {
		log.Printf("failed to fetch possible reviewers: %v", err)
		return nil
	}

	if len(prs) == 0 {
		return nil
	}

	userIDs := make([]uint, len(prs))
	for i, pr := range prs {
		userIDs[i] = pr.UserID
	}

	var users []models.User
	if err := c.db.Where("id IN ? AND id <> ? AND on_vacation = ?", userIDs, mr.AuthorID, false).
		Find(&users).Error; err != nil {
		log.Printf("failed to fetch default reviewer users: %v", err)
		return nil
	}

	return users
}

// selectReviewers implements the reviewer selection algorithm:
// 1. If MR has labels with configured label reviewers:
//   - Pick exactly 1 from each label group (no reuse across groups)
//   - If total < minCount, pick additional from combined remaining pool
//
// 2. If no label reviewers available, pick minCount from default pool
func (c *MRReviewerConsumer) selectReviewers(mr *models.MergeRequest, minCount int) []models.User {
	if minCount <= 0 {
		minCount = 1
	}

	// Try label-based selection first
	labelGroups := c.getLabelReviewerGroups(mr)
	if len(labelGroups) > 0 {
		return c.selectFromLabelGroups(mr, labelGroups, minCount)
	}

	// Fall back to default pool
	defaultReviewers := c.getDefaultReviewers(mr)
	if len(defaultReviewers) == 0 {
		return nil
	}

	return c.pickMultipleReviewers(defaultReviewers, minCount)
}

// selectFromLabelGroups picks reviewers from label groups:
// 1. Pick exactly 1 from each group (with no reuse)
// 2. If total < minCount, pick additional from combined remaining label reviewers + default pool
func (c *MRReviewerConsumer) selectFromLabelGroups(mr *models.MergeRequest, groups map[string][]models.User, minCount int) []models.User {
	// Build workload map for weighted selection
	reviewCounts := c.getRecentReviewCounts(groups)

	// Track selected users to enforce no-reuse rule
	selectedSet := make(map[uint]bool)
	var selected []models.User

	// Get label names and sort for deterministic order
	labelNames := make([]string, 0, len(groups))
	for label := range groups {
		labelNames = append(labelNames, label)
	}
	sort.Strings(labelNames)

	// Phase 1: Pick exactly 1 from each label group
	for _, label := range labelNames {
		pool := groups[label]

		// Filter out already selected users
		available := make([]models.User, 0)
		for _, u := range pool {
			if !selectedSet[u.ID] {
				available = append(available, u)
			}
		}

		if len(available) == 0 {
			log.Printf("No available reviewers for label %s (all already selected)", label)
			continue
		}

		// Pick one using weighted probability
		idx := c.pickReviewerFromPool(available, reviewCounts)
		picked := available[idx]
		selected = append(selected, picked)
		selectedSet[picked.ID] = true
		log.Printf("Picked reviewer %s (ID %d) for label %s", picked.Username, picked.ID, label)
	}

	// Phase 2: If we haven't reached minCount, pick additional from combined remaining pool + default pool
	if len(selected) < minCount {
		// Build combined pool of all remaining label reviewers
		var combinedPool []models.User
		seenInPool := make(map[uint]bool)
		for _, pool := range groups {
			for _, u := range pool {
				if !selectedSet[u.ID] && !seenInPool[u.ID] {
					combinedPool = append(combinedPool, u)
					seenInPool[u.ID] = true
				}
			}
		}

		// Add default reviewers to the combined pool
		defaultReviewers := c.getDefaultReviewers(mr)
		var newUserIDs []uint
		for _, u := range defaultReviewers {
			if !selectedSet[u.ID] && !seenInPool[u.ID] {
				combinedPool = append(combinedPool, u)
				seenInPool[u.ID] = true
				// Track new user IDs that need review counts fetched
				if _, hasCount := reviewCounts[u.ID]; !hasCount {
					newUserIDs = append(newUserIDs, u.ID)
				}
			}
		}

		// Fetch review counts for default reviewers not already in reviewCounts
		if len(newUserIDs) > 0 {
			defaultCounts := c.getReviewCountsForUserIDs(newUserIDs)
			for userID, count := range defaultCounts {
				reviewCounts[userID] = count
			}
		}

		// Pick additional reviewers
		needed := minCount - len(selected)
		additional := c.pickMultipleFromPool(combinedPool, needed, reviewCounts)
		for _, u := range additional {
			selected = append(selected, u)
			selectedSet[u.ID] = true
		}
	}

	if len(selected) == 0 {
		return nil
	}

	log.Printf("Selected %d reviewer(s) from label groups (min was %d)", len(selected), minCount)
	return selected
}

// getRecentReviewCounts returns review counts for users in the given groups.
func (c *MRReviewerConsumer) getRecentReviewCounts(groups map[string][]models.User) map[uint]int {
	// Collect all user IDs
	userIDs := make([]uint, 0)
	userIDSet := make(map[uint]bool)
	for _, pool := range groups {
		for _, u := range pool {
			if !userIDSet[u.ID] {
				userIDs = append(userIDs, u.ID)
				userIDSet[u.ID] = true
			}
		}
	}

	return c.getReviewCountsForUserIDs(userIDs)
}

// getReviewCountsForUserIDs fetches recent review counts for specific user IDs.
func (c *MRReviewerConsumer) getReviewCountsForUserIDs(userIDs []uint) map[uint]int {
	reviewCounts := make(map[uint]int)

	if len(userIDs) == 0 {
		return reviewCounts
	}

	twoWeeksAgo := time.Now().AddDate(0, 0, -14)

	var counts []struct {
		UserID uint
		Count  int
	}

	if err := c.db.Table("merge_request_reviewers").
		Joins("JOIN merge_requests ON merge_requests.id = merge_request_reviewers.merge_request_id").
		Where("merge_requests.gitlab_created_at > ?", twoWeeksAgo).
		Where("merge_request_reviewers.user_id IN ?", userIDs).
		Select("merge_request_reviewers.user_id, COUNT(*) as count").
		Group("merge_request_reviewers.user_id").
		Find(&counts).Error; err != nil {
		log.Printf("failed to fetch recent reviewer counts: %v", err)
	} else {
		for _, rc := range counts {
			reviewCounts[rc.UserID] = rc.Count
		}
	}

	return reviewCounts
}

// pickMultipleFromPool picks multiple reviewers from a pool using weighted probability.
func (c *MRReviewerConsumer) pickMultipleFromPool(users []models.User, count int, reviewCounts map[uint]int) []models.User {
	if len(users) == 0 || count <= 0 {
		return nil
	}

	if len(users) <= count {
		return users
	}

	selected := make([]models.User, 0, count)
	remaining := make([]models.User, len(users))
	copy(remaining, users)

	for i := 0; i < count && len(remaining) > 0; i++ {
		idx := c.pickReviewerFromPool(remaining, reviewCounts)
		selected = append(selected, remaining[idx])
		remaining = append(remaining[:idx], remaining[idx+1:]...)
	}

	return selected
}

// getAssignCount returns the number of reviewers to assign for a repository.
// Defaults to 1 if no SLA is configured.
func (c *MRReviewerConsumer) getAssignCount(repoID uint) int {
	sla, err := utils.GetRepositorySLA(c.db, repoID)
	if err != nil {
		log.Printf("failed to get repository SLA: %v", err)
		return 1
	}
	if sla.AssignCount <= 0 {
		return 1
	}
	return sla.AssignCount
}

// pickMultipleReviewers selects N reviewers using weighted probability based on workload.
// Returns up to `count` reviewers (may be fewer if not enough candidates).
func (c *MRReviewerConsumer) pickMultipleReviewers(users []models.User, count int) []models.User {
	if len(users) == 0 {
		return nil
	}

	if count <= 0 {
		count = 1
	}

	// If we have fewer users than requested, return all
	if len(users) <= count {
		return users
	}

	// Fetch review counts for weighted selection
	userIDs := make([]uint, len(users))
	for i, user := range users {
		userIDs[i] = user.ID
	}
	reviewCounts := c.getReviewCountsForUserIDs(userIDs)

	return c.pickMultipleFromPool(users, count, reviewCounts)
}

// pickReviewerFromPool selects a single reviewer from a pool using weighted probability.
func (c *MRReviewerConsumer) pickReviewerFromPool(users []models.User, reviewCounts map[uint]int) int {
	if len(users) == 0 {
		return 0
	}

	if len(users) == 1 {
		return 0
	}

	// Calculate weights based on historical review counts
	weights := make([]float64, len(users))
	totalWeight := 0.0

	for i, user := range users {
		count := reviewCounts[user.ID]
		weight := 1.0 / float64(count+1)
		weights[i] = weight
		totalWeight += weight
	}

	if totalWeight <= 0 {
		return rand.Intn(len(users))
	}

	// Normalize weights
	for i := range weights {
		weights[i] /= totalWeight
	}

	// Select based on weighted probability
	r := rand.Float64()
	cumulativeWeight := 0.0

	for i, weight := range weights {
		cumulativeWeight += weight
		if r <= cumulativeWeight {
			return i
		}
	}

	return rand.Intn(len(users))
}

// formatReviewerMentions formats reviewer mentions for notification message.
func (c *MRReviewerConsumer) formatReviewerMentions(reviewers []models.User) string {
	mentions := make([]string, len(reviewers))
	for i, reviewer := range reviewers {
		mention := reviewer.Email
		if mention == "" {
			var vkUser models.VKUser
			if err := c.db.Where("user_id LIKE ?", reviewer.Username+"%").First(&vkUser).Error; err == nil {
				mention = vkUser.UserID
			} else {
				mention = reviewer.Username
			}
		}
		mentions[i] = "@[" + mention + "]"
	}
	return strings.Join(mentions, ", ")
}

// AssignReviewers finds unreviewed MRs, picks reviewers, notifies chats, and marks MR with reviewers.
// Supports multiple reviewers based on RepositorySLA.AssignCount setting.
// Uses label-based priority cascade: label reviewers first, then default pool.
// Filters out users on vacation.
func (c *MRReviewerConsumer) AssignReviewers() {
	var mrs []models.MergeRequest
	// Load open, not draft, not merged, without existing reviewers or approvers,
	// and for repositories that have subscriptions.
	// Also preload Labels for label-based reviewer selection.
	if err := c.db.
		Preload("Repository").Preload("Author").Preload("Labels").
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
		log.Print("No merge requests need reviewers.")
		return
	}

	for _, mr := range mrs {
		// Get minimum number of reviewers to assign from SLA settings
		minCount := c.getAssignCount(mr.RepositoryID)

		// Select reviewers using new algorithm:
		// - If MR has labels with label reviewers: pick 1 from each label, then fill to min
		// - Otherwise: pick min from default pool
		reviewers := c.selectReviewers(&mr, minCount)
		if len(reviewers) == 0 {
			log.Printf("no available reviewers for repository %d (MR %d)", mr.RepositoryID, mr.ID)
			continue
		}

		// Build GitLab reviewer IDs
		reviewerIDs := make([]int, len(reviewers))
		for i, r := range reviewers {
			reviewerIDs[i] = r.GitlabID
		}

		// Assign reviewers in GitLab
		if _, _, err := c.glClient.MergeRequests.UpdateMergeRequest(
			mr.Repository.GitlabID, mr.IID,
			&gitlab.UpdateMergeRequestOptions{ReviewerIDs: &reviewerIDs},
		); err != nil {
			log.Printf("failed to assign reviewers in GitLab: %v", err)
			continue
		}

		// Notify all subscribed chats
		var subs []models.RepositorySubscription
		if err := c.db.Preload("Chat").Where("repository_id = ?", mr.RepositoryID).Find(&subs).Error; err != nil {
			log.Printf("failed to fetch subscriptions: %v", err)
			continue
		}

		// Prepare author mention
		authorMention := mr.Author.Email
		if authorMention == "" {
			var vkUser models.VKUser
			if err := c.db.Where("user_id LIKE ?", mr.Author.Username+"%").First(&vkUser).Error; err == nil {
				authorMention = vkUser.UserID
			} else {
				authorMention = mr.Author.Username
			}
		}

		// Prepare reviewer mentions
		reviewerMentions := c.formatReviewerMentions(reviewers)

		// Send notifications
		reviewerLabel := "reviewer"
		if len(reviewers) > 1 {
			reviewerLabel = "reviewers"
		}
		for _, sub := range subs {
			text := fmt.Sprintf(
				"%s\n%s\nby @[%s] %s: %s",
				mr.Title,
				mr.WebURL,
				authorMention,
				reviewerLabel,
				reviewerMentions,
			)
			msg := c.vkBot.NewTextMessage(sub.Chat.ChatID, text)
			if err := msg.Send(); err != nil {
				log.Printf("failed to send review assignment: %v", err)
			}
		}

		// Mark MR as having reviewers in local DB
		if err := c.db.Model(&mr).Association("Reviewers").Append(reviewers); err != nil {
			log.Printf("failed to mark MR reviewers: %v", err)
		}

		log.Printf("Assigned %d reviewer(s) to MR %d: %v", len(reviewers), mr.ID, reviewerIDs)
	}
}
