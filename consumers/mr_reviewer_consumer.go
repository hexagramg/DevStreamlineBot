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

	"devstreamlinebot/interfaces"
	"devstreamlinebot/models"
	"devstreamlinebot/utils"
)

// MRReviewerConsumer periodically assigns reviewers to new merge requests and notifies chats.
type MRReviewerConsumer struct {
	db        *gorm.DB
	vkBot     interfaces.VKBot
	glClient  *gitlab.Client
	interval  time.Duration
	startTime time.Time
}

// NewMRReviewerConsumer initializes a new MRReviewerConsumer.
// If startTime is nil, defaults to 2 days before now.
func NewMRReviewerConsumer(db *gorm.DB, vkBot *botgolang.Bot, glClient *gitlab.Client, interval time.Duration, startTime *time.Time) *MRReviewerConsumer {
	var bot interfaces.VKBot
	if vkBot != nil {
		bot = &interfaces.RealVKBot{Bot: vkBot}
	}
	st := time.Now().AddDate(0, 0, -2)
	if startTime != nil {
		st = *startTime
	}
	return &MRReviewerConsumer{
		db:        db,
		vkBot:     bot,
		glClient:  glClient,
		interval:  interval,
		startTime: st,
	}
}

// NewMRReviewerConsumerWithBot initializes a consumer with a custom VKBot (for testing).
// If startTime is nil, defaults to 2 days before now.
func NewMRReviewerConsumerWithBot(db *gorm.DB, vkBot interfaces.VKBot, glClient *gitlab.Client, interval time.Duration, startTime *time.Time) *MRReviewerConsumer {
	st := time.Now().AddDate(0, 0, -2)
	if startTime != nil {
		st = *startTime
	}
	return &MRReviewerConsumer{
		db:        db,
		vkBot:     vkBot,
		glClient:  glClient,
		interval:  interval,
		startTime: st,
	}
}

// Start begins the polling loop for assigning reviewers and processing notifications.
func (c *MRReviewerConsumer) StartConsumer() {
	ticker := time.NewTicker(c.interval)
	go func() {
		defer ticker.Stop()
		for range ticker.C {
			c.AssignReviewers()
			c.ProcessStateChangeNotifications()
			c.ProcessReviewerRemovalNotifications()
			c.ProcessFullyApprovedNotifications()
		}
	}()
}

// getLabelReviewerGroups returns label reviewers grouped by label name.
// Only includes labels that the MR has AND have configured reviewers.
// Filters out users on vacation, the MR author, and any users in excludeUsers.
func (c *MRReviewerConsumer) getLabelReviewerGroups(mr *models.MergeRequest, excludeUsers []models.User) map[string][]models.User {
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

	// Build list of user IDs to exclude (author + excludeUsers)
	excludeIDs := []uint{mr.AuthorID}
	for _, u := range excludeUsers {
		excludeIDs = append(excludeIDs, u.ID)
	}

	// Fetch users, excluding MR author, users on vacation, and excludeUsers
	var users []models.User
	if err := c.db.Where("id IN ? AND id NOT IN ? AND on_vacation = ?", userIDs, excludeIDs, false).
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
// Filters out users on vacation, the MR author, and any users in excludeUsers.
func (c *MRReviewerConsumer) getDefaultReviewers(mr *models.MergeRequest, excludeUsers []models.User) []models.User {
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

	// Build list of user IDs to exclude (author + excludeUsers)
	excludeIDs := []uint{mr.AuthorID}
	for _, u := range excludeUsers {
		excludeIDs = append(excludeIDs, u.ID)
	}

	var users []models.User
	if err := c.db.Where("id IN ? AND id NOT IN ? AND on_vacation = ?", userIDs, excludeIDs, false).
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
//
// excludeUsers are filtered out from all pools (used when adding reviewers to MRs that already have some).
func (c *MRReviewerConsumer) selectReviewers(mr *models.MergeRequest, minCount int, excludeUsers []models.User) []models.User {
	if minCount <= 0 {
		minCount = 1
	}

	// Try label-based selection first
	labelGroups := c.getLabelReviewerGroups(mr, excludeUsers)
	if len(labelGroups) > 0 {
		return c.selectFromLabelGroups(mr, labelGroups, minCount, excludeUsers)
	}

	// Fall back to default pool
	defaultReviewers := c.getDefaultReviewers(mr, excludeUsers)
	if len(defaultReviewers) == 0 {
		return nil
	}

	return c.pickMultipleReviewers(defaultReviewers, minCount)
}

// selectFromLabelGroups picks reviewers from label groups:
// 1. Pick exactly 1 from each group (with no reuse)
// 2. If total < minCount, pick additional from combined remaining label reviewers + default pool
func (c *MRReviewerConsumer) selectFromLabelGroups(mr *models.MergeRequest, groups map[string][]models.User, minCount int, excludeUsers []models.User) []models.User {
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
		defaultReviewers := c.getDefaultReviewers(mr, excludeUsers)
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
// Uses batch query to avoid N+1 DB queries when looking up VKUsers.
func (c *MRReviewerConsumer) formatReviewerMentions(reviewers []models.User) string {
	// Collect usernames needing VK lookup (those without email)
	var usernamesToLookup []string
	for _, r := range reviewers {
		if r.Email == "" {
			usernamesToLookup = append(usernamesToLookup, r.Username)
		}
	}

	// Batch fetch VKUsers with OR conditions
	vkUserMap := make(map[string]string) // username -> UserID
	if len(usernamesToLookup) > 0 {
		var vkUsers []models.VKUser
		query := c.db
		for i, username := range usernamesToLookup {
			if i == 0 {
				query = query.Where("user_id LIKE ?", username+"%")
			} else {
				query = query.Or("user_id LIKE ?", username+"%")
			}
		}
		query.Find(&vkUsers)

		// Build set of usernames for O(1) lookup
		usernameSet := make(map[string]struct{})
		for _, username := range usernamesToLookup {
			usernameSet[username] = struct{}{}
		}

		// Map VKUsers by extracting username prefix
		for _, vk := range vkUsers {
			// VK UserID format is "username@domain" - extract username part
			username := vk.UserID
			if idx := strings.Index(vk.UserID, "@"); idx > 0 {
				username = vk.UserID[:idx]
			}
			if _, ok := usernameSet[username]; ok {
				vkUserMap[username] = vk.UserID
			}
		}
	}

	// Build mentions using the pre-fetched map
	mentions := make([]string, len(reviewers))
	for i, reviewer := range reviewers {
		mention := reviewer.Email
		if mention == "" {
			if vkID, ok := vkUserMap[reviewer.Username]; ok {
				mention = vkID
			} else {
				mention = reviewer.Username
			}
		}
		mentions[i] = "@[" + mention + "]"
	}
	return strings.Join(mentions, ", ")
}

// AssignReviewers finds MRs with insufficient reviewers, picks reviewers, notifies chats, and updates MR with reviewers.
// Supports multiple reviewers based on RepositorySLA.AssignCount setting.
// Uses label-based priority cascade: label reviewers first, then default pool.
// Filters out users on vacation.
// Also handles backfill: if an MR already has some reviewers but fewer than AssignCount, adds more.
func (c *MRReviewerConsumer) AssignReviewers() {
	var mrs []models.MergeRequest
	// Load open, not draft, not merged MRs for repositories with subscriptions.
	// Preload existing Reviewers to check if we need to add more.
	if err := c.db.
		Preload("Repository").Preload("Author").Preload("Labels").Preload("Reviewers").
		Where("merge_requests.state = ? AND merge_requests.draft = ? AND merge_requests.merged_at IS NULL", "opened", false).
		Where("EXISTS (SELECT 1 FROM repository_subscriptions WHERE repository_subscriptions.repository_id = merge_requests.repository_id)").
		Where("merge_requests.gitlab_created_at > ?", c.startTime). // Only process MRs created after consumer start
		Find(&mrs).Error; err != nil {
		log.Printf("failed to fetch merge requests: %v", err)
		return
	}

	if len(mrs) == 0 {
		log.Print("No open merge requests found.")
		return
	}

	processedCount := 0
	for _, mr := range mrs {
		// Skip MRs with release labels (completely ignored)
		if utils.HasReleaseLabel(c.db, &mr) {
			continue
		}

		// Get minimum number of reviewers to assign from SLA settings
		minCount := c.getAssignCount(mr.RepositoryID)
		existingReviewers := mr.Reviewers
		needed := minCount - len(existingReviewers)

		// Skip if we already have enough reviewers
		if needed <= 0 {
			continue
		}

		isBackfill := len(existingReviewers) > 0
		log.Printf("MR %d needs %d more reviewer(s) (has %d, min %d)", mr.ID, needed, len(existingReviewers), minCount)

		// Select additional reviewers, excluding existing ones from pools
		newReviewers := c.selectReviewers(&mr, needed, existingReviewers)
		if len(newReviewers) == 0 {
			log.Printf("no available reviewers for repository %d (MR %d)", mr.RepositoryID, mr.ID)
			continue
		}

		// Build GitLab reviewer IDs (all reviewers: existing + new)
		allReviewers := append(existingReviewers, newReviewers...)
		reviewerIDs := make([]int, len(allReviewers))
		for i, r := range allReviewers {
			reviewerIDs[i] = r.GitlabID
		}

		// Update reviewers in GitLab
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

		// Prepare reviewer mentions (only new reviewers for notification)
		newReviewerMentions := c.formatReviewerMentions(newReviewers)

		// Send notifications with different message format for backfill
		reviewerLabel := "reviewer"
		if len(newReviewers) > 1 {
			reviewerLabel = "reviewers"
		}
		for _, sub := range subs {
			var text string
			if isBackfill {
				text = fmt.Sprintf(
					"%s\n%s\nAdditional %s: %s",
					mr.Title,
					mr.WebURL,
					reviewerLabel,
					newReviewerMentions,
				)
			} else {
				text = fmt.Sprintf(
					"%s\n%s\nby @[%s] %s: %s",
					mr.Title,
					mr.WebURL,
					authorMention,
					reviewerLabel,
					newReviewerMentions,
				)
			}
			msg := c.vkBot.NewTextMessage(sub.Chat.ChatID, text)
			if err := msg.Send(); err != nil {
				log.Printf("failed to send review assignment: %v", err)
			}
		}

		// Mark MR as having reviewers in local DB (only add new ones)
		if err := c.db.Model(&mr).Association("Reviewers").Append(newReviewers); err != nil {
			log.Printf("failed to mark MR reviewers: %v", err)
		}

		// Send DM to each newly assigned reviewer
		for _, reviewer := range newReviewers {
			c.notifyUserDM(reviewer.Email, fmt.Sprintf(
				"🔍 New MR for review:\n%s\n%s",
				mr.Title,
				mr.WebURL,
			))
		}

		log.Printf("Assigned %d new reviewer(s) to MR %d (total: %d): %v", len(newReviewers), mr.ID, len(allReviewers), reviewerIDs)
		processedCount++
	}

	if processedCount == 0 {
		log.Print("No merge requests need additional reviewers.")
	}
}

// notifyUserDM attempts to send a direct message to a user.
// Logs a warning if the message fails (e.g., user hasn't messaged the bot before).
func (c *MRReviewerConsumer) notifyUserDM(userEmail, text string) {
	if userEmail == "" {
		return
	}
	msg := c.vkBot.NewTextMessage(userEmail, text)
	if err := msg.Send(); err != nil {
		log.Printf("DM to %s failed (user may not have messaged bot): %v", userEmail, err)
	}
}

// ProcessReviewerRemovalNotifications sends DMs to reviewers who were removed from MRs.
// Called periodically from the main consumer loop.
func (c *MRReviewerConsumer) ProcessReviewerRemovalNotifications() {
	var actions []models.MRAction
	err := c.db.
		Preload("MergeRequest").
		Preload("TargetUser").
		Where("notified = ? AND action_type = ?", false, models.ActionReviewerRemoved).
		Order("timestamp ASC").
		Limit(100).
		Find(&actions).Error
	if err != nil {
		log.Printf("failed to fetch unnotified reviewer removal actions: %v", err)
		return
	}

	for _, action := range actions {
		if action.TargetUser == nil || action.TargetUser.Email == "" {
			c.markActionNotified(action.ID)
			continue
		}

		c.notifyUserDM(action.TargetUser.Email, fmt.Sprintf(
			"You were removed from review:\n%s\n%s",
			action.MergeRequest.Title,
			action.MergeRequest.WebURL,
		))
		c.markActionNotified(action.ID)
	}
}

// ProcessFullyApprovedNotifications sends DMs to release managers when MRs are fully approved.
// Called periodically from the main consumer loop.
func (c *MRReviewerConsumer) ProcessFullyApprovedNotifications() {
	var actions []models.MRAction
	err := c.db.
		Preload("MergeRequest").
		Preload("MergeRequest.Repository").
		Where("notified = ? AND action_type = ?", false, models.ActionFullyApproved).
		Order("timestamp ASC").
		Limit(100).
		Find(&actions).Error
	if err != nil {
		log.Printf("failed to fetch unnotified fully approved actions: %v", err)
		return
	}

	for _, action := range actions {
		mr := action.MergeRequest

		// Find release managers for this repository
		var releaseManagers []models.ReleaseManager
		if err := c.db.Preload("User").Where("repository_id = ?", mr.RepositoryID).Find(&releaseManagers).Error; err != nil {
			log.Printf("failed to fetch release managers for repo %d: %v", mr.RepositoryID, err)
			c.markActionNotified(action.ID)
			continue
		}

		// Send DM to each release manager
		for _, rm := range releaseManagers {
			if rm.User.Email == "" {
				continue
			}
			c.notifyUserDM(rm.User.Email, fmt.Sprintf(
				"✅ MR ready for release:\n%s\n%s\nAll reviewers approved",
				mr.Title,
				mr.WebURL,
			))
		}

		c.markActionNotified(action.ID)
	}
}

// ProcessStateChangeNotifications checks for state changes and sends DMs only on actual transitions.
// Called periodically from the main consumer loop.
func (c *MRReviewerConsumer) ProcessStateChangeNotifications() {
	// Find unnotified comment actions that may indicate state changes
	var actions []models.MRAction
	err := c.db.
		Preload("MergeRequest").
		Preload("MergeRequest.Author").
		Preload("MergeRequest.Reviewers").
		Where("notified = ? AND action_type IN ?", false, []models.MRActionType{
			models.ActionCommentAdded,
			models.ActionCommentResolved,
		}).
		Order("timestamp ASC").
		Limit(100).
		Find(&actions).Error
	if err != nil {
		log.Printf("failed to fetch unnotified actions: %v", err)
		return
	}

	// Group actions by MR ID
	mrActions := make(map[uint][]models.MRAction)
	for _, action := range actions {
		mrActions[action.MergeRequestID] = append(mrActions[action.MergeRequestID], action)
	}

	// Process each MR's actions
	for _, actionList := range mrActions {
		mr := actionList[0].MergeRequest

		// Skip if MR is not in an active state
		if mr.State != "opened" {
			for _, action := range actionList {
				c.markActionNotified(action.ID)
			}
			continue
		}

		// Get current derived state
		currentState := string(utils.GetStateInfo(c.db, &mr).State)

		// Only notify if state actually changed from last notified state
		if currentState != mr.LastNotifiedState {
			switch utils.MRState(currentState) {
			case utils.StateOnFixes:
				// Notify author that MR needs fixes
				c.notifyUserDM(mr.Author.Email, fmt.Sprintf(
					"🔧 Your MR needs fixes:\n%s\n%s\nReviewer left comments",
					mr.Title,
					mr.WebURL,
				))

			case utils.StateOnReview:
				// Only notify reviewers if transitioning FROM on_fixes (not initial assignment)
				if mr.LastNotifiedState == string(utils.StateOnFixes) {
					for _, reviewer := range mr.Reviewers {
						c.notifyUserDM(reviewer.Email, fmt.Sprintf(
							"✅ MR ready for re-review:\n%s\n%s\nAuthor addressed comments",
							mr.Title,
							mr.WebURL,
						))
					}
				}
			}

			// Update last notified state
			if err := c.db.Model(&mr).Update("last_notified_state", currentState).Error; err != nil {
				log.Printf("failed to update last_notified_state for MR %d: %v", mr.ID, err)
			}
		}

		// Mark all actions for this MR as notified
		for _, action := range actionList {
			c.markActionNotified(action.ID)
		}
	}
}

// markActionNotified marks an action as having been processed for notifications.
func (c *MRReviewerConsumer) markActionNotified(actionID uint) {
	if err := c.db.Model(&models.MRAction{}).Where("id = ?", actionID).Update("notified", true).Error; err != nil {
		log.Printf("failed to mark action %d as notified: %v", actionID, err)
	}
}
