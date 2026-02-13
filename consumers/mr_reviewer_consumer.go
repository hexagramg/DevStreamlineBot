package consumers

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	botgolang "github.com/mail-ru-im/bot-golang"
	gitlab "gitlab.com/gitlab-org/api/client-go"
	"gorm.io/gorm"

	"devstreamlinebot/interfaces"
	"devstreamlinebot/models"
	"devstreamlinebot/utils"
)

type threadSafeRand struct {
	mu  sync.Mutex
	rnd *rand.Rand
}

var safeRand = &threadSafeRand{
	rnd: rand.New(rand.NewSource(time.Now().UnixNano())),
}

func (r *threadSafeRand) Intn(n int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rnd.Intn(n)
}

func (r *threadSafeRand) Float64() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rnd.Float64()
}

type MRReviewerConsumer struct {
	db        *gorm.DB
	vkBot     interfaces.VKBot
	glClient  *gitlab.Client
	interval  time.Duration
	startTime time.Time
}

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

func (c *MRReviewerConsumer) getLabelReviewerGroups(mr *models.MergeRequest, excludeUsers []models.User) map[string][]models.User {
	if len(mr.Labels) == 0 {
		return nil
	}

	labelNames := make([]string, len(mr.Labels))
	for i, label := range mr.Labels {
		labelNames[i] = label.Name
	}

	var labelReviewers []models.LabelReviewer
	if err := c.db.Where("repository_id = ? AND label_name IN ?", mr.RepositoryID, labelNames).
		Find(&labelReviewers).Error; err != nil {
		log.Printf("failed to fetch label reviewers: %v", err)
		return nil
	}

	if len(labelReviewers) == 0 {
		return nil
	}

	userIDSet := make(map[uint]bool)
	for _, lr := range labelReviewers {
		userIDSet[lr.UserID] = true
	}
	userIDs := make([]uint, 0, len(userIDSet))
	for id := range userIDSet {
		userIDs = append(userIDs, id)
	}

	excludeIDs := []uint{mr.AuthorID}
	for _, u := range excludeUsers {
		excludeIDs = append(excludeIDs, u.ID)
	}

	var users []models.User
	if err := c.db.Where("id IN ? AND id NOT IN ? AND on_vacation = ?", userIDs, excludeIDs, false).
		Find(&users).Error; err != nil {
		log.Printf("failed to fetch label reviewer users: %v", err)
		return nil
	}

	userMap := make(map[uint]models.User)
	for _, u := range users {
		userMap[u.ID] = u
	}

	groups := make(map[string][]models.User)
	for _, lr := range labelReviewers {
		if user, ok := userMap[lr.UserID]; ok {
			groups[lr.LabelName] = append(groups[lr.LabelName], user)
		}
	}

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
func (c *MRReviewerConsumer) selectReviewers(mr *models.MergeRequest, minCount int, excludeUsers []models.User) []models.User {
	if minCount <= 0 {
		minCount = 1
	}

	labelGroups := c.getLabelReviewerGroups(mr, excludeUsers)
	if len(labelGroups) > 0 {
		return c.selectFromLabelGroups(mr, labelGroups, minCount, excludeUsers)
	}

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
	reviewCounts := c.getRecentReviewCounts(groups)

	selectedSet := make(map[uint]bool)
	var selected []models.User

	labelNames := make([]string, 0, len(groups))
	for label := range groups {
		labelNames = append(labelNames, label)
	}
	sort.Strings(labelNames)

	for _, label := range labelNames {
		pool := groups[label]

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

		idx := c.pickReviewerFromPool(available, reviewCounts)
		picked := available[idx]
		selected = append(selected, picked)
		selectedSet[picked.ID] = true
		log.Printf("Picked reviewer %s (ID %d) for label %s", picked.Username, picked.ID, label)
	}

	if len(selected) < minCount {
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

		defaultReviewers := c.getDefaultReviewers(mr, excludeUsers)
		var newUserIDs []uint
		for _, u := range defaultReviewers {
			if !selectedSet[u.ID] && !seenInPool[u.ID] {
				combinedPool = append(combinedPool, u)
				seenInPool[u.ID] = true
				if _, hasCount := reviewCounts[u.ID]; !hasCount {
					newUserIDs = append(newUserIDs, u.ID)
				}
			}
		}

		if len(newUserIDs) > 0 {
			defaultCounts := c.getReviewCountsForUserIDs(newUserIDs)
			for userID, count := range defaultCounts {
				reviewCounts[userID] = count
			}
		}

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

func (c *MRReviewerConsumer) getRecentReviewCounts(groups map[string][]models.User) map[uint]int {
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

func (c *MRReviewerConsumer) pickMultipleReviewers(users []models.User, count int) []models.User {
	if len(users) == 0 {
		return nil
	}

	if count <= 0 {
		count = 1
	}

	if len(users) <= count {
		return users
	}

	userIDs := make([]uint, len(users))
	for i, user := range users {
		userIDs[i] = user.ID
	}
	reviewCounts := c.getReviewCountsForUserIDs(userIDs)

	return c.pickMultipleFromPool(users, count, reviewCounts)
}

func (c *MRReviewerConsumer) pickReviewerFromPool(users []models.User, reviewCounts map[uint]int) int {
	if len(users) == 0 {
		return 0
	}

	if len(users) == 1 {
		return 0
	}

	weights := make([]float64, len(users))
	totalWeight := 0.0

	for i, user := range users {
		count := reviewCounts[user.ID]
		weight := 1.0 / float64(count+1)
		weights[i] = weight
		totalWeight += weight
	}

	if totalWeight <= 0 {
		return safeRand.Intn(len(users))
	}

	for i := range weights {
		weights[i] /= totalWeight
	}

	r := safeRand.Float64()
	cumulativeWeight := 0.0

	for i, weight := range weights {
		cumulativeWeight += weight
		if r <= cumulativeWeight {
			return i
		}
	}

	return safeRand.Intn(len(users))
}

// formatReviewerMentions uses batch query to avoid N+1 DB queries when looking up VKUsers.
func (c *MRReviewerConsumer) formatReviewerMentions(reviewers []models.User) string {
	var usernamesToLookup []string
	for _, r := range reviewers {
		if r.Email == "" {
			usernamesToLookup = append(usernamesToLookup, r.Username)
		}
	}

	vkUserMap := make(map[string]string)
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

		usernameSet := make(map[string]struct{})
		for _, username := range usernamesToLookup {
			usernameSet[username] = struct{}{}
		}

		for _, vk := range vkUsers {
			username := vk.UserID
			if idx := strings.Index(vk.UserID, "@"); idx > 0 {
				username = vk.UserID[:idx]
			}
			if _, ok := usernameSet[username]; ok {
				vkUserMap[username] = vk.UserID
			}
		}
	}

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

// AssignReviewers finds MRs needing reviewers, assigns them using label-based priority cascade,
// notifies chats, and updates GitLab. Handles backfill when MR has fewer reviewers than AssignCount.
func (c *MRReviewerConsumer) AssignReviewers() {
	var mrs []models.MergeRequest
	if err := c.db.
		Preload("Repository").Preload("Author").Preload("Labels").Preload("Reviewers").
		Where("merge_requests.state = ? AND merge_requests.draft = ? AND merge_requests.merged_at IS NULL", "opened", false).
		Where("EXISTS (SELECT 1 FROM repository_subscriptions WHERE repository_subscriptions.repository_id = merge_requests.repository_id)").
		Where("merge_requests.gitlab_created_at > ?", c.startTime).
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
		if utils.HasReleaseLabel(c.db, &mr) {
			continue
		}
		if utils.IsMRBlocked(c.db, &mr) {
			continue
		}

		minCount := c.getAssignCount(mr.RepositoryID)
		existingReviewers := mr.Reviewers
		needed := minCount - len(existingReviewers)

		if needed <= 0 {
			continue
		}

		isBackfill := len(existingReviewers) > 0
		log.Printf("MR %d needs %d more reviewer(s) (has %d, min %d)", mr.ID, needed, len(existingReviewers), minCount)

		newReviewers := c.selectReviewers(&mr, needed, existingReviewers)
		if len(newReviewers) == 0 {
			log.Printf("no available reviewers for repository %d (MR %d)", mr.RepositoryID, mr.ID)
			continue
		}

		allReviewers := append(existingReviewers, newReviewers...)
		reviewerIDs := make([]int, len(allReviewers))
		for i, r := range allReviewers {
			reviewerIDs[i] = r.GitlabID
		}

		if _, _, err := c.glClient.MergeRequests.UpdateMergeRequest(
			mr.Repository.GitlabID, mr.IID,
			&gitlab.UpdateMergeRequestOptions{ReviewerIDs: &reviewerIDs},
		); err != nil {
			log.Printf("failed to assign reviewers in GitLab: %v", err)
			continue
		}

		var latestNotif models.MRNotificationState
		if err := c.db.Where("merge_request_id = ?", mr.ID).
			Order("created_at desc").First(&latestNotif).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			c.db.Create(&models.MRNotificationState{
				MergeRequestID: mr.ID,
				NotifiedState:  "on_review",
			})
		}

		var subs []models.RepositorySubscription
		if err := c.db.Preload("Chat").Where("repository_id = ?", mr.RepositoryID).Find(&subs).Error; err != nil {
			log.Printf("failed to fetch subscriptions: %v", err)
			continue
		}

		authorMention := mr.Author.Email
		if authorMention == "" {
			var vkUser models.VKUser
			if err := c.db.Where("user_id LIKE ?", mr.Author.Username+"%").First(&vkUser).Error; err == nil {
				authorMention = vkUser.UserID
			} else {
				authorMention = mr.Author.Username
			}
		}

		newReviewerMentions := c.formatReviewerMentions(newReviewers)

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

		if err := c.db.Model(&mr).Association("Reviewers").Append(newReviewers); err != nil {
			log.Printf("failed to mark MR reviewers: %v", err)
		}

		for _, reviewer := range newReviewers {
			c.notifyUserDM(reviewer.Email, fmt.Sprintf(
				"🔍 New MR for review [%s]:\n%s\n%s",
				mr.Repository.Name,
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

func (c *MRReviewerConsumer) notifyUserDM(userEmail, text string) {
	if userEmail == "" {
		return
	}
	msg := c.vkBot.NewTextMessage(userEmail, text)
	if err := msg.Send(); err != nil {
		log.Printf("DM to %s failed (user may not have messaged bot): %v", userEmail, err)
	}
}

func (c *MRReviewerConsumer) ProcessReviewerRemovalNotifications() {
	var actions []models.MRAction
	err := c.db.
		Preload("MergeRequest").
		Preload("MergeRequest.Repository").
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
			"You were removed from review [%s]:\n%s\n%s",
			action.MergeRequest.Repository.Name,
			action.MergeRequest.Title,
			action.MergeRequest.WebURL,
		))
		c.markActionNotified(action.ID)
	}
}

func (c *MRReviewerConsumer) ProcessFullyApprovedNotifications() {
	var actions []models.MRAction
	err := c.db.
		Preload("MergeRequest").
		Preload("MergeRequest.Repository").
		Preload("MergeRequest.Author").
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

		if mr.Author.Email != "" {
			c.notifyUserDM(mr.Author.Email, fmt.Sprintf(
				"Your MR is fully approved [%s]:\n%s\n%s",
				mr.Repository.Name,
				mr.Title,
				mr.WebURL,
			))
		}

		var releaseManagers []models.ReleaseManager
		if err := c.db.Preload("User").Where("repository_id = ?", mr.RepositoryID).Find(&releaseManagers).Error; err != nil {
			log.Printf("failed to fetch release managers for repo %d: %v", mr.RepositoryID, err)
			c.markActionNotified(action.ID)
			continue
		}

		for _, rm := range releaseManagers {
			if rm.User.Email == "" {
				continue
			}
			c.notifyUserDM(rm.User.Email, fmt.Sprintf(
				"MR ready for release [%s]:\n%s\n%s",
				mr.Repository.Name,
				mr.Title,
				mr.WebURL,
			))
		}

		c.markActionNotified(action.ID)
	}
}

func (c *MRReviewerConsumer) ProcessStateChangeNotifications() {
	recentCutoff := time.Now().UTC().Add(-30 * time.Minute)

	var actions []models.MRAction
	err := c.db.
		Preload("MergeRequest").
		Preload("MergeRequest.Author").
		Preload("MergeRequest.Reviewers").
		Preload("MergeRequest.Approvers").
		Preload("MergeRequest.Repository").
		Where("notified = ? AND action_type IN ? AND timestamp > ?", false, []models.MRActionType{
			models.ActionCommentAdded,
			models.ActionCommentResolved,
		}, recentCutoff).
		Order("timestamp ASC").
		Limit(100).
		Find(&actions).Error
	if err != nil {
		log.Printf("failed to fetch unnotified actions: %v", err)
		return
	}

	mrActions := make(map[uint][]models.MRAction)
	for _, action := range actions {
		mrActions[action.MergeRequestID] = append(mrActions[action.MergeRequestID], action)
	}

	for _, actionList := range mrActions {
		mr := actionList[0].MergeRequest

		if mr.State != "opened" {
			for _, action := range actionList {
				c.markActionNotified(action.ID)
			}
			continue
		}

		currentState := string(utils.GetStateInfo(c.db, &mr).State)

		var latestNotif models.MRNotificationState
		notifErr := c.db.Where("merge_request_id = ?", mr.ID).
			Order("created_at desc").First(&latestNotif).Error
		if notifErr != nil && !errors.Is(notifErr, gorm.ErrRecordNotFound) {
			log.Printf("failed to get notification state for MR %d: %v", mr.ID, notifErr)
			for _, action := range actionList {
				c.markActionNotified(action.ID)
			}
			continue
		}

		if currentState != latestNotif.NotifiedState {
			switch utils.MRState(currentState) {
			case utils.StateOnFixes:
				c.notifyUserDM(mr.Author.Email, fmt.Sprintf(
					"🔧 Your MR needs fixes [%s]:\n%s\n%s\nReviewer left comments",
					mr.Repository.Name,
					mr.Title,
					mr.WebURL,
				))

			case utils.StateOnReview:
				if latestNotif.NotifiedState == string(utils.StateOnFixes) {
					approverIDs := make(map[uint]bool)
					for _, approver := range mr.Approvers {
						approverIDs[approver.ID] = true
					}
					for _, reviewer := range mr.Reviewers {
						if approverIDs[reviewer.ID] {
							continue
						}
						c.notifyUserDM(reviewer.Email, fmt.Sprintf(
							"MR ready for re-review [%s]:\n%s\n%s",
							mr.Repository.Name,
							mr.Title,
							mr.WebURL,
						))
					}
				}
			}

			c.db.Create(&models.MRNotificationState{
				MergeRequestID:      mr.ID,
				NotifiedState:       currentState,
				NotifiedDescription: latestNotif.NotifiedDescription,
			})
		}

		for _, action := range actionList {
			c.markActionNotified(action.ID)
		}
	}
}

func (c *MRReviewerConsumer) markActionNotified(actionID uint) {
	if err := c.db.Model(&models.MRAction{}).Where("id = ?", actionID).Update("notified", true).Error; err != nil {
		log.Printf("failed to mark action %d as notified: %v", actionID, err)
	}
}

// CleanupOldUnnotifiedActions marks old unprocessed actions as notified to prevent
// them from being processed and potentially causing spam notifications.
func (c *MRReviewerConsumer) CleanupOldUnnotifiedActions() {
	oldCutoff := time.Now().UTC().Add(-1 * time.Hour)
	result := c.db.Model(&models.MRAction{}).
		Where("notified = ? AND timestamp < ?", false, oldCutoff).
		Update("notified", true)
	if result.Error != nil {
		log.Printf("failed to cleanup old unnotified actions: %v", result.Error)
	} else if result.RowsAffected > 0 {
		log.Printf("cleaned up %d old unnotified actions", result.RowsAffected)
	}
}
