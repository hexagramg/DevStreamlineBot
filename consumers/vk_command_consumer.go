package consumers

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	botgolang "github.com/mail-ru-im/bot-golang"
	gitlab "gitlab.com/gitlab-org/api/client-go"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"devstreamlinebot/models"
	"devstreamlinebot/polling"
	"devstreamlinebot/utils"
)

const (
	defaultBlockLabelColor        = "#dc143c" // crimson
	defaultReleaseLabelColor      = "#808080" // gray
	defaultReleaseReadyLabelColor = "#FFD700" // gold
)

// VKCommandConsumer processes VK Teams messages and looks for commands.
type VKCommandConsumer struct {
	db       *gorm.DB
	vkBot    *botgolang.Bot
	glClient *gitlab.Client
	msgChan  <-chan polling.VKEvent
}

// NewVKCommandConsumer creates a VK command consumer with existing bot, channel, and GitLab client.
func NewVKCommandConsumer(db *gorm.DB, vkBot *botgolang.Bot, glClient *gitlab.Client, msgChan <-chan polling.VKEvent) *VKCommandConsumer {
	return &VKCommandConsumer{db: db, vkBot: vkBot, glClient: glClient, msgChan: msgChan}
}

// StartConsumer begins processing VK events from the channel.
func (c *VKCommandConsumer) StartConsumer() {
	go func() {
		for ev := range c.msgChan {
			c.processMessage(ev.Msg, ev.From)
		}
	}()
}

// processMessage handles command messages.
func (c *VKCommandConsumer) processMessage(msg *botgolang.Message, from botgolang.Contact) {
	if msg.Text == "" {
		return
	}

	// Check commands (order matters for prefix matching)
	if strings.HasPrefix(msg.Text, "/subscribers") {
		c.handleSubscribersCommand(msg)
	} else if strings.HasPrefix(msg.Text, "/subscribe") {
		c.handleSubscribeCommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/unsubscribe") {
		c.handleUnsubscribeCommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/label_reviewers") {
		c.handleLabelReviewersCommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/reviewers") {
		c.handleReviewersCommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/actions") {
		c.handleActionsCommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/send_digest") {
		c.handleSendDigestCommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/get_mr_info") {
		c.handleGetMRInfoCommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/vacation") {
		c.handleVacationCommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/assign_count") {
		c.handleAssignCountCommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/holidays") {
		c.handleHolidaysCommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/sla") {
		c.handleSLACommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/daily_digest") {
		c.handleDailyDigestCommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/add_block_label") {
		c.handleAddBlockLabelCommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/add_release_ready_label") {
		c.handleAddReleaseReadyLabelCommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/add_release_label") {
		c.handleAddReleaseLabelCommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/ensure_label") {
		c.handleEnsureLabelCommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/release_managers") {
		c.handleReleaseManagersCommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/auto_release_branch") {
		c.handleAutoReleaseBranchCommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/add_jira_prefix") {
		c.handleAddJiraPrefixCommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/release_unsubscribe") {
		c.handleReleaseUnsubscribeCommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/release_subscribe") {
		c.handleReleaseSubscribeCommand(msg, from)
	}
}

// handleSubscribeCommand processes the /subscribe command to link a chat with a repository.
// Format: /subscribe <repo_id> [--force]
// If another chat already owns the repository, --force is required to take over.
// Settings (reviewers, SLA, holidays) are copied from other repositories in the same chat.
func (c *VKCommandConsumer) handleSubscribeCommand(msg *botgolang.Message, from botgolang.Contact) {
	parts := strings.Fields(msg.Text)
	if len(parts) < 2 {
		c.sendReply(msg, "Usage: /subscribe <repository_id> [--force]")
		return
	}

	forceFlag := false
	for _, p := range parts {
		if p == "--force" {
			forceFlag = true
			break
		}
	}

	repoIDStr := strings.TrimSpace(parts[1])
	repoIDStr = strings.TrimSuffix(repoIDStr, ",")

	repoID, err := strconv.Atoi(repoIDStr)
	if err != nil {
		c.sendReply(msg, fmt.Sprintf("Invalid repository ID: %s", repoIDStr))
		return
	}

	var repo models.Repository
	if err := c.db.Where("gitlab_id = ?", repoID).First(&repo).Error; err != nil {
		c.sendReply(msg, fmt.Sprintf("Repository with ID %d not found", repoID))
		return
	}

	chatID := fmt.Sprint(msg.Chat.ID)
	var chat models.Chat
	chatData := models.Chat{
		ChatID: chatID,
		Type:   msg.Chat.Type,
		Title:  msg.Chat.Title,
	}
	if err := c.db.Where(models.Chat{ChatID: chatID}).Assign(chatData).FirstOrCreate(&chat).Error; err != nil {
		log.Printf("failed to get or create chat %s: %v", chatID, err)
		c.sendReply(msg, "Failed to process chat information. Please try again later.")
		return
	}

	userID := fmt.Sprint(from.ID)
	var user models.VKUser
	vkUserData := models.VKUser{
		UserID:    userID,
		FirstName: from.FirstName,
		LastName:  from.LastName,
	}
	if err := c.db.Where(models.VKUser{UserID: userID}).Assign(vkUserData).FirstOrCreate(&user).Error; err != nil {
		log.Printf("failed to get or create VK user %s: %v", userID, err)
		c.sendReply(msg, "Failed to process user information. Please try again later.")
		return
	}

	var existingSub models.RepositorySubscription
	if err := c.db.Where("repository_id = ? AND chat_id = ?", repo.ID, chat.ID).First(&existingSub).Error; err == nil {
		c.sendReply(msg, fmt.Sprintf("This chat is already subscribed to repository: %s", repo.Name))
		return
	}

	var otherSub models.RepositorySubscription
	takenOver := false
	var oldChatTitle string
	if err := c.db.Preload("Chat").Where("repository_id = ? AND chat_id != ?", repo.ID, chat.ID).First(&otherSub).Error; err == nil {
		if !forceFlag {
			c.sendReply(msg, fmt.Sprintf("Repository %s is already subscribed by chat '%s'. Use '/subscribe %d --force' to take over.",
				repo.Name, otherSub.Chat.Title, repoID))
			return
		}
		oldChatTitle = otherSub.Chat.Title
		takenOver = true
		c.db.Delete(&otherSub)

		c.db.Where("repository_id = ?", repo.ID).Delete(&models.PossibleReviewer{})
		c.db.Where("repository_id = ?", repo.ID).Delete(&models.LabelReviewer{})
		c.db.Where("repository_id = ?", repo.ID).Delete(&models.RepositorySLA{})
		c.db.Where("repository_id = ?", repo.ID).Delete(&models.Holiday{})
		c.db.Where("repository_id = ?", repo.ID).Delete(&models.BlockLabel{})
		c.db.Where("repository_id = ?", repo.ID).Delete(&models.AutoReleaseBranchConfig{})
	}

	subscription := models.RepositorySubscription{
		RepositoryID: repo.ID,
		ChatID:       chat.ID,
		VKUserID:     user.ID,
		SubscribedAt: time.Now(),
	}

	if err := c.db.Create(&subscription).Error; err != nil {
		log.Printf("failed to create subscription: %v", err)
		c.sendReply(msg, "Failed to create subscription. Please try again later.")
		return
	}

	settingsCopied := false
	var existingSubs []models.RepositorySubscription
	c.db.Where("chat_id = ? AND repository_id != ?", chat.ID, repo.ID).Find(&existingSubs)

	if len(existingSubs) > 0 {
		sourceRepoID := existingSubs[0].RepositoryID
		settingsCopied = true

		var existingReviewers []models.PossibleReviewer
		c.db.Where("repository_id = ?", sourceRepoID).Find(&existingReviewers)
		for _, r := range existingReviewers {
			c.db.Create(&models.PossibleReviewer{RepositoryID: repo.ID, UserID: r.UserID})
		}

		var existingLabelReviewers []models.LabelReviewer
		c.db.Where("repository_id = ?", sourceRepoID).Find(&existingLabelReviewers)
		for _, lr := range existingLabelReviewers {
			c.db.Create(&models.LabelReviewer{RepositoryID: repo.ID, LabelName: lr.LabelName, UserID: lr.UserID})
		}

		var existingSLA models.RepositorySLA
		if err := c.db.Where("repository_id = ?", sourceRepoID).First(&existingSLA).Error; err == nil {
			c.db.Create(&models.RepositorySLA{
				RepositoryID:   repo.ID,
				ReviewDuration: existingSLA.ReviewDuration,
				FixesDuration:  existingSLA.FixesDuration,
				AssignCount:    existingSLA.AssignCount,
			})
		}

		var existingHolidays []models.Holiday
		c.db.Where("repository_id = ?", sourceRepoID).Find(&existingHolidays)
		for _, h := range existingHolidays {
			c.db.Create(&models.Holiday{RepositoryID: repo.ID, Date: h.Date})
		}

		var existingBlockLabels []models.BlockLabel
		c.db.Where("repository_id = ?", sourceRepoID).Find(&existingBlockLabels)
		for _, bl := range existingBlockLabels {
			c.db.Create(&models.BlockLabel{RepositoryID: repo.ID, LabelName: bl.LabelName})
		}

		var existingAutoReleaseConfig models.AutoReleaseBranchConfig
		if err := c.db.Where("repository_id = ?", sourceRepoID).First(&existingAutoReleaseConfig).Error; err == nil {
			c.db.Create(&models.AutoReleaseBranchConfig{
				RepositoryID:        repo.ID,
				ReleaseBranchPrefix: existingAutoReleaseConfig.ReleaseBranchPrefix,
				DevBranchName:       existingAutoReleaseConfig.DevBranchName,
			})
		}
	}

	var successMsg string
	if takenOver {
		if settingsCopied {
			successMsg = fmt.Sprintf("Repository %s is now subscribed (taken over from '%s'). Settings copied from existing subscriptions.", repo.Name, oldChatTitle)
		} else {
			successMsg = fmt.Sprintf("Repository %s is now subscribed (taken over from '%s'). Configure reviewers with /reviewers.", repo.Name, oldChatTitle)
		}
	} else if settingsCopied {
		successMsg = fmt.Sprintf("Repository %s is now subscribed. Settings copied from existing subscriptions.", repo.Name)
	} else {
		successMsg = fmt.Sprintf("Repository %s is now subscribed. Configure reviewers with /reviewers.", repo.Name)
	}
	c.sendReply(msg, successMsg)
}

func (c *VKCommandConsumer) handleUnsubscribeCommand(msg *botgolang.Message, _ botgolang.Contact) {
	parts := strings.Fields(msg.Text)
	if len(parts) < 2 {
		c.sendReply(msg, "Usage: /unsubscribe <repository_id>")
		return
	}

	repoIDStr := strings.TrimSuffix(strings.TrimSpace(parts[1]), ",")
	repoID, err := strconv.Atoi(repoIDStr)
	if err != nil {
		c.sendReply(msg, fmt.Sprintf("Invalid repository ID: %s", repoIDStr))
		return
	}

	var repo models.Repository
	if err := c.db.Where("gitlab_id = ?", repoID).First(&repo).Error; err != nil {
		c.sendReply(msg, fmt.Sprintf("Repository with ID %d not found", repoID))
		return
	}

	chatID := fmt.Sprint(msg.Chat.ID)
	var chat models.Chat
	if err := c.db.Where("chat_id = ?", chatID).First(&chat).Error; err != nil {
		c.sendReply(msg, "Chat not found")
		return
	}

	var sub models.RepositorySubscription
	if err := c.db.Where("repository_id = ? AND chat_id = ?", repo.ID, chat.ID).First(&sub).Error; err != nil {
		c.sendReply(msg, fmt.Sprintf("No subscription found for repository %s", repo.Name))
		return
	}

	if err := c.db.Delete(&sub).Error; err != nil {
		c.sendReply(msg, fmt.Sprintf("Failed to unsubscribe from repository %s", repo.Name))
		return
	}

	c.sendReply(msg, fmt.Sprintf("Unsubscribed from repository %s", repo.Name))
}

func (c *VKCommandConsumer) handleReviewersCommand(msg *botgolang.Message, _ botgolang.Contact) {
	chatID := fmt.Sprint(msg.Chat.ID)
	var chat models.Chat
	if err := c.db.Where("chat_id = ?", chatID).First(&chat).Error; err != nil {
		c.sendReply(msg, "Chat not found in subscriptions")
		return
	}
	var subs []models.RepositorySubscription
	c.db.Preload("Repository").Where("chat_id = ?", chat.ID).Find(&subs)
	if len(subs) == 0 {
		c.sendReply(msg, "No repository subscription found. Use /subscribe first.")
		return
	}
	repoIDs := make([]uint, len(subs))
	repoNames := make([]string, len(subs))
	for i, s := range subs {
		repoIDs[i] = s.Repository.ID
		repoNames[i] = s.Repository.Name
	}

	argStr := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/reviewers"))
	if argStr == "" {
		if err := c.db.Where("repository_id IN ?", repoIDs).Delete(&models.PossibleReviewer{}).Error; err != nil {
			c.sendReply(msg, "Failed to clear reviewers")
			return
		}
		c.sendReply(msg, fmt.Sprintf("Cleared all reviewers for repositories: %s", strings.Join(repoNames, ",")))
		return
	}

	names := strings.Split(argStr, ",")
	var added []string
	var notFoundUsers []string
	for _, name := range names {
		uname := strings.TrimSpace(name)
		if uname == "" {
			continue
		}

		var user models.User
		err := c.db.Where("username = ?", uname).First(&user).Error

		if err != nil {
			if gorm.ErrRecordNotFound == err {
				users, _, glErr := c.glClient.Users.ListUsers(&gitlab.ListUsersOptions{Username: gitlab.Ptr(uname)})
				if glErr != nil || len(users) == 0 {
					log.Printf("User %s not found in GitLab or API error: %v", uname, glErr)
					notFoundUsers = append(notFoundUsers, uname)
					continue
				}
				glUser := users[0]
				userData := models.User{
					GitlabID:  glUser.ID,
					Username:  glUser.Username,
					Name:      glUser.Name,
					State:     glUser.State,
					CreatedAt: glUser.CreatedAt,
					AvatarURL: glUser.AvatarURL,
					WebURL:    glUser.WebURL,
					Email:     glUser.Email,
				}
				if err := c.db.Where(models.User{GitlabID: glUser.ID}).Assign(userData).FirstOrCreate(&user).Error; err != nil {
					log.Printf("Failed to upsert GitLab user %s (ID: %d): %v", uname, glUser.ID, err)
					c.sendReply(msg, fmt.Sprintf("Error processing user: %s. Please try again.", uname))
					return
				}
			} else {
				log.Printf("DB error looking up user %s: %v", uname, err)
				c.sendReply(msg, fmt.Sprintf("Database error while looking up user: %s.", uname))
				return
			}
		}
		for _, rid := range repoIDs {
			pr := models.PossibleReviewer{RepositoryID: rid, UserID: user.ID}
			if err := c.db.FirstOrCreate(&pr, models.PossibleReviewer{RepositoryID: rid, UserID: user.ID}).Error; err != nil {
				log.Printf("Failed to create possible reviewer link for repo %d and user %d: %v", rid, user.ID, err)
			}
		}
		added = append(added, user.Username)
	}

	replyText := fmt.Sprintf("Reviewers for repositories %s updated: %s.", strings.Join(repoNames, ", "), strings.Join(added, ", "))
	if len(notFoundUsers) > 0 {
		replyText += fmt.Sprintf(" Users not found: %s.", strings.Join(notFoundUsers, ", "))
	}
	c.sendReply(msg, replyText)
}

func (c *VKCommandConsumer) handleReleaseManagersCommand(msg *botgolang.Message, _ botgolang.Contact) {
	chatID := fmt.Sprint(msg.Chat.ID)
	var chat models.Chat
	if err := c.db.Where("chat_id = ?", chatID).First(&chat).Error; err != nil {
		c.sendReply(msg, "Chat not found in subscriptions")
		return
	}
	var subs []models.RepositorySubscription
	c.db.Preload("Repository").Where("chat_id = ?", chat.ID).Find(&subs)
	if len(subs) == 0 {
		c.sendReply(msg, "No repository subscription found. Use /subscribe first.")
		return
	}
	repoIDs := make([]uint, len(subs))
	repoNames := make([]string, len(subs))
	for i, s := range subs {
		repoIDs[i] = s.Repository.ID
		repoNames[i] = s.Repository.Name
	}

	argStr := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/release_managers"))
	if argStr == "" {
		var managers []models.ReleaseManager
		c.db.Preload("User").Where("repository_id IN ?", repoIDs).Find(&managers)
		if len(managers) == 0 {
			c.sendReply(msg, "No release managers configured. Use /release_managers user1,user2,... to set.")
			return
		}
		usernames := make(map[string]bool)
		for _, m := range managers {
			usernames[m.User.Username] = true
		}
		var names []string
		for u := range usernames {
			names = append(names, u)
		}
		c.sendReply(msg, fmt.Sprintf("Current release managers: %s", strings.Join(names, ", ")))
		return
	}

	if err := c.db.Where("repository_id IN ?", repoIDs).Delete(&models.ReleaseManager{}).Error; err != nil {
		c.sendReply(msg, "Failed to clear existing release managers")
		return
	}

	names := strings.Split(argStr, ",")
	var added []string
	var notFoundUsers []string
	for _, name := range names {
		uname := strings.TrimSpace(name)
		if uname == "" {
			continue
		}

		var user models.User
		err := c.db.Where("username = ?", uname).First(&user).Error

		if err != nil {
			if gorm.ErrRecordNotFound == err {
				users, _, glErr := c.glClient.Users.ListUsers(&gitlab.ListUsersOptions{Username: gitlab.Ptr(uname)})
				if glErr != nil || len(users) == 0 {
					log.Printf("User %s not found in GitLab or API error: %v", uname, glErr)
					notFoundUsers = append(notFoundUsers, uname)
					continue
				}
				glUser := users[0]
				userData := models.User{
					GitlabID:  glUser.ID,
					Username:  glUser.Username,
					Name:      glUser.Name,
					State:     glUser.State,
					CreatedAt: glUser.CreatedAt,
					AvatarURL: glUser.AvatarURL,
					WebURL:    glUser.WebURL,
					Email:     glUser.Email,
				}
				if err := c.db.Where(models.User{GitlabID: glUser.ID}).Assign(userData).FirstOrCreate(&user).Error; err != nil {
					log.Printf("Failed to upsert GitLab user %s (ID: %d): %v", uname, glUser.ID, err)
					c.sendReply(msg, fmt.Sprintf("Error processing user: %s. Please try again.", uname))
					return
				}
			} else {
				log.Printf("DB error looking up user %s: %v", uname, err)
				c.sendReply(msg, fmt.Sprintf("Database error while looking up user: %s.", uname))
				return
			}
		}
		for _, rid := range repoIDs {
			rm := models.ReleaseManager{RepositoryID: rid, UserID: user.ID}
			if err := c.db.FirstOrCreate(&rm, models.ReleaseManager{RepositoryID: rid, UserID: user.ID}).Error; err != nil {
				log.Printf("Failed to create release manager link for repo %d and user %d: %v", rid, user.ID, err)
			}
		}
		added = append(added, user.Username)
	}

	replyText := fmt.Sprintf("Release managers for repositories %s updated: %s.", strings.Join(repoNames, ", "), strings.Join(added, ", "))
	if len(notFoundUsers) > 0 {
		replyText += fmt.Sprintf(" Users not found: %s.", strings.Join(notFoundUsers, ", "))
	}
	c.sendReply(msg, replyText)
}

func (c *VKCommandConsumer) handleActionsCommand(msg *botgolang.Message, from botgolang.Contact) {
	parts := strings.Fields(msg.Text)
	var username string
	if len(parts) < 2 {
		vkID := fmt.Sprint(from.ID)
		var vkUser models.VKUser
		if err := c.db.Where("user_id = ?", vkID).First(&vkUser).Error; err != nil {
			c.sendReply(msg, "Cannot determine your account. Please specify a GitLab username: /actions <username>")
			return
		}
		var user models.User
		if err := c.db.Where("email = ?", vkUser.UserID).First(&user).Error; err != nil {
			c.sendReply(msg, "No linked GitLab user found for your VK account. Please specify a username: /actions <username>")
			return
		}
		username = user.Username
	} else {
		username = strings.TrimSpace(parts[1])
	}

	var user models.User
	if err := c.db.Where("username = ?", username).First(&user).Error; err != nil {
		c.sendReply(msg, fmt.Sprintf("User %s not found", username))
		return
	}

	reviewMRs, fixesMRs, authorOnReviewMRs, err := utils.FindUserActionMRs(c.db, user.ID)
	if err != nil {
		log.Printf("failed to fetch actions for user %s: %v", username, err)
		c.sendReply(msg, "Failed to fetch actions. Please try again later.")
		return
	}

	releaseMRs, err := utils.FindReleaseManagerActionMRs(c.db, user.ID)
	if err != nil {
		log.Printf("failed to fetch release manager MRs for user %s: %v", username, err)
	}

	text := utils.BuildUserActionsDigest(c.db, reviewMRs, fixesMRs, authorOnReviewMRs, releaseMRs, username)
	replyMsg := c.vkBot.NewTextMessage(fmt.Sprint(msg.Chat.ID), text)
	if err := replyMsg.Send(); err != nil {
		log.Printf("failed to send actions digest: %v", err)
	}
}

func (c *VKCommandConsumer) handleSendDigestCommand(msg *botgolang.Message, _ botgolang.Contact) {
	chatID := fmt.Sprint(msg.Chat.ID)

	var chat models.Chat
	if err := c.db.Where("chat_id = ?", chatID).First(&chat).Error; err != nil {
		c.sendReply(msg, "Chat not found in database")
		return
	}

	var subs []models.RepositorySubscription
	if err := c.db.
		Preload("Repository").
		Where("chat_id = ?", chat.ID).
		Find(&subs).Error; err != nil {
		c.sendReply(msg, "Failed to fetch subscriptions. Please try again later.")
		return
	}

	var repoIDs []uint
	for _, s := range subs {
		repoIDs = append(repoIDs, s.RepositoryID)
	}

	if len(repoIDs) == 0 {
		c.sendReply(msg, "No repository subscriptions found for this chat")
		return
	}

	mrs, err := utils.FindDigestMergeRequests(c.db, repoIDs)
	if err != nil {
		c.sendReply(msg, "Failed to fetch merge requests. Please try again later.")
		return
	}

	if len(mrs) == 0 {
		c.sendReply(msg, "No pending reviews found for subscribed repositories")
		return
	}

	text := utils.BuildReviewDigest(c.db, mrs)
	c.sendReply(msg, text)
}

func (c *VKCommandConsumer) handleGetMRInfoCommand(msg *botgolang.Message, _ botgolang.Contact) {
	parts := strings.Fields(msg.Text)
	if len(parts) < 2 {
		c.sendReply(msg, "Usage: /get_mr_info <project_path!iid> (e.g., intdev/jobofferapp!2103)")
		return
	}
	ref := strings.TrimSpace(parts[1])
	bangIdx := strings.LastIndex(ref, "!")
	if bangIdx == -1 || bangIdx == 0 || bangIdx == len(ref)-1 {
		c.sendReply(msg, "Invalid reference format. Use <project_path!iid> (e.g., intdev/jobofferapp!2103)")
		return
	}
	projectPath := ref[:bangIdx]
	mrIID := ref[bangIdx+1:]

	var repo models.Repository
	if err := c.db.Where("web_url LIKE ?", "%"+projectPath+"%").First(&repo).Error; err != nil {
		c.sendReply(msg, "Repository not found for this reference.")
		return
	}

	var mr models.MergeRequest
	if err := c.db.Where("repository_id = ? AND iid = ?", repo.ID, mrIID).
		Preload("Author").
		Preload("Reviewers").
		Preload("Approvers").
		First(&mr).Error; err != nil {
		c.sendReply(msg, "Merge request not found in local database.")
		return
	}

	reviewerNames := make([]string, 0, len(mr.Reviewers))
	for _, u := range mr.Reviewers {
		reviewerNames = append(reviewerNames, "@"+u.Username)
	}
	approverNames := make([]string, 0, len(mr.Approvers))
	for _, u := range mr.Approvers {
		approverNames = append(approverNames, "@"+u.Username)
	}

	var subs []models.RepositorySubscription
	if err := c.db.Where("repository_id = ?", repo.ID).Preload("Chat").Find(&subs).Error; err != nil {
		subs = nil
	}
	chatTitles := make([]string, 0, len(subs))
	for _, s := range subs {
		if s.Chat.Title != "" {
			chatTitles = append(chatTitles, s.Chat.Title)
		}
	}

	createdAt := ""
	if mr.GitlabCreatedAt != nil {
		createdAt = mr.GitlabCreatedAt.Format("2006-01-02 15:04:05")
	}

	info := fmt.Sprintf(
		"MR #%d: %s\nState: %s\nAuthor: @%s\nCreated: %s\nURL: %s\nReviewers: %s\nApprovers: %s\nActive subscriptions: %s",
		mr.IID,
		mr.Title,
		mr.State,
		mr.Author.Username,
		createdAt,
		mr.WebURL,
		strings.Join(reviewerNames, ", "),
		strings.Join(approverNames, ", "),
		strings.Join(chatTitles, ", "))
	c.sendReply(msg, info)
}

func (c *VKCommandConsumer) handleVacationCommand(msg *botgolang.Message, _ botgolang.Contact) {
	parts := strings.Fields(msg.Text)
	if len(parts) < 2 {
		c.sendReply(msg, "Usage: /vacation <username>")
		return
	}

	username := strings.TrimSpace(parts[1])

	var user models.User
	if err := c.db.Where("username = ?", username).First(&user).Error; err != nil {
		c.sendReply(msg, fmt.Sprintf("User %s not found", username))
		return
	}

	user.OnVacation = !user.OnVacation
	if err := c.db.Save(&user).Error; err != nil {
		log.Printf("failed to update vacation status for user %s: %v", username, err)
		c.sendReply(msg, "Failed to update vacation status")
		return
	}

	status := "off vacation"
	if user.OnVacation {
		status = "on vacation"
	}
	c.sendReply(msg, fmt.Sprintf("User %s is now %s", username, status))
}

func (c *VKCommandConsumer) handleAssignCountCommand(msg *botgolang.Message, _ botgolang.Contact) {
	parts := strings.Fields(msg.Text)
	if len(parts) < 2 {
		c.sendReply(msg, "Usage: /assign_count <N>")
		return
	}

	count, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || count < 1 {
		c.sendReply(msg, "Invalid count. Must be a positive integer.")
		return
	}

	chatID := fmt.Sprint(msg.Chat.ID)
	var chat models.Chat
	if err := c.db.Where("chat_id = ?", chatID).First(&chat).Error; err != nil {
		c.sendReply(msg, "Chat not found")
		return
	}

	var subs []models.RepositorySubscription
	c.db.Preload("Repository").Where("chat_id = ?", chat.ID).Find(&subs)
	if len(subs) == 0 {
		c.sendReply(msg, "No repository subscription found. Use /subscribe first.")
		return
	}

	repoNames := make([]string, 0, len(subs))
	for _, sub := range subs {
		var sla models.RepositorySLA
		if err := c.db.Where(models.RepositorySLA{RepositoryID: sub.RepositoryID}).
			Assign(models.RepositorySLA{AssignCount: count}).
			FirstOrCreate(&sla).Error; err != nil {
			log.Printf("failed to set assign count for repo %d: %v", sub.RepositoryID, err)
		}
		repoNames = append(repoNames, sub.Repository.Name)
	}

	c.sendReply(msg, fmt.Sprintf("Assign count set to %d for: %s", count, strings.Join(repoNames, ", ")))
}

func (c *VKCommandConsumer) handleHolidaysCommand(msg *botgolang.Message, _ botgolang.Contact) {
	chatID := fmt.Sprint(msg.Chat.ID)
	var chat models.Chat
	if err := c.db.Where("chat_id = ?", chatID).First(&chat).Error; err != nil {
		c.sendReply(msg, "Chat not found")
		return
	}

	var subs []models.RepositorySubscription
	c.db.Preload("Repository").Where("chat_id = ?", chat.ID).Find(&subs)
	if len(subs) == 0 {
		c.sendReply(msg, "No repository subscription found. Use /subscribe first.")
		return
	}

	repoIDs := make([]uint, len(subs))
	for i, s := range subs {
		repoIDs[i] = s.RepositoryID
	}

	argStr := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/holidays"))

	if argStr == "" {
		var holidays []models.Holiday
		c.db.Where("repository_id IN ?", repoIDs).Order("date").Find(&holidays)

		if len(holidays) == 0 {
			c.sendReply(msg, "No holidays configured.")
			return
		}

		var dates []string
		seen := make(map[string]bool)
		for _, h := range holidays {
			dateStr := h.Date.Format("02.01.2006")
			if !seen[dateStr] {
				dates = append(dates, dateStr)
				seen[dateStr] = true
			}
		}
		c.sendReply(msg, "Holidays: "+strings.Join(dates, ", "))
		return
	}

	if strings.HasPrefix(argStr, "remove ") {
		dateStrs := strings.Fields(strings.TrimPrefix(argStr, "remove "))
		var removed []string
		var failed []string

		for _, dateStr := range dateStrs {
			date, err := time.Parse("02.01.2006", dateStr)
			if err != nil {
				failed = append(failed, dateStr)
				continue
			}

			result := c.db.Where("repository_id IN ? AND date = ?", repoIDs, date).Delete(&models.Holiday{})
			if result.RowsAffected > 0 {
				removed = append(removed, dateStr)
			} else {
				failed = append(failed, dateStr+" (not found)")
			}
		}

		reply := ""
		if len(removed) > 0 {
			reply = fmt.Sprintf("Removed holidays: %s", strings.Join(removed, ", "))
		}
		if len(failed) > 0 {
			if reply != "" {
				reply += "\n"
			}
			reply += fmt.Sprintf("Failed: %s", strings.Join(failed, ", "))
		}
		c.sendReply(msg, reply)
		return
	}

	dateStrs := strings.Fields(argStr)
	var added []string
	var failed []string
	var holidays []models.Holiday

	for _, dateStr := range dateStrs {
		date, err := time.Parse("02.01.2006", dateStr)
		if err != nil {
			failed = append(failed, dateStr)
			continue
		}

		for _, repoID := range repoIDs {
			holidays = append(holidays, models.Holiday{RepositoryID: repoID, Date: date})
		}
		added = append(added, dateStr)
	}

	if len(holidays) > 0 {
		c.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&holidays)
	}

	reply := ""
	if len(added) > 0 {
		reply = fmt.Sprintf("Added holidays: %s", strings.Join(added, ", "))
	}
	if len(failed) > 0 {
		if reply != "" {
			reply += "\n"
		}
		reply += fmt.Sprintf("Failed to parse: %s (use DD.MM.YYYY)", strings.Join(failed, ", "))
	}
	c.sendReply(msg, reply)
}

func (c *VKCommandConsumer) handleSLACommand(msg *botgolang.Message, _ botgolang.Contact) {
	chatID := fmt.Sprint(msg.Chat.ID)
	var chat models.Chat
	if err := c.db.Where("chat_id = ?", chatID).First(&chat).Error; err != nil {
		c.sendReply(msg, "Chat not found")
		return
	}

	var subs []models.RepositorySubscription
	c.db.Preload("Repository").Where("chat_id = ?", chat.ID).Find(&subs)
	if len(subs) == 0 {
		c.sendReply(msg, "No repository subscription found. Use /subscribe first.")
		return
	}

	parts := strings.Fields(msg.Text)

	if len(parts) < 2 {
		var lines []string
		for _, sub := range subs {
			var sla models.RepositorySLA
			if err := c.db.Where("repository_id = ?", sub.RepositoryID).First(&sla).Error; err != nil {
				lines = append(lines, fmt.Sprintf("%s: not configured", sub.Repository.Name))
			} else {
				lines = append(lines, fmt.Sprintf("%s: review=%s, fixes=%s, assign_count=%d",
					sub.Repository.Name,
					formatSLADuration(sla.ReviewDuration.ToDuration()),
					formatSLADuration(sla.FixesDuration.ToDuration()),
					sla.AssignCount))
			}
		}
		c.sendReply(msg, "SLA Settings:\n"+strings.Join(lines, "\n"))
		return
	}

	if len(parts) < 3 {
		c.sendReply(msg, "Usage: /sla review <duration> or /sla fixes <duration>\nDuration format: 1h, 2d, 1w")
		return
	}

	slaType := strings.ToLower(parts[1])
	if slaType != "review" && slaType != "fixes" {
		c.sendReply(msg, "SLA type must be 'review' or 'fixes'")
		return
	}

	duration, err := utils.ParseDuration(parts[2])
	if err != nil {
		c.sendReply(msg, fmt.Sprintf("Invalid duration: %s. Use format like 1h, 2d, 1w", parts[2]))
		return
	}

	var repoNames []string
	for _, sub := range subs {
		var sla models.RepositorySLA
		if err := c.db.Where(models.RepositorySLA{RepositoryID: sub.RepositoryID}).FirstOrCreate(&sla).Error; err != nil {
			log.Printf("failed to get/create SLA for repo %d: %v", sub.RepositoryID, err)
			continue
		}

		if slaType == "review" {
			sla.ReviewDuration = models.Duration(duration)
		} else {
			sla.FixesDuration = models.Duration(duration)
		}
		if err := c.db.Save(&sla).Error; err != nil {
			log.Printf("failed to save SLA for repo %d: %v", sub.RepositoryID, err)
		}

		var repo models.Repository
		c.db.First(&repo, sub.RepositoryID)
		repoNames = append(repoNames, repo.Name)
	}

	c.sendReply(msg, fmt.Sprintf("SLA %s set to %s for: %s", slaType, parts[2], strings.Join(repoNames, ", ")))
}

func (c *VKCommandConsumer) handleLabelReviewersCommand(msg *botgolang.Message, _ botgolang.Contact) {
	chatID := fmt.Sprint(msg.Chat.ID)
	var chat models.Chat
	if err := c.db.Where("chat_id = ?", chatID).First(&chat).Error; err != nil {
		c.sendReply(msg, "Chat not found")
		return
	}

	var subs []models.RepositorySubscription
	c.db.Preload("Repository").Where("chat_id = ?", chat.ID).Find(&subs)
	if len(subs) == 0 {
		c.sendReply(msg, "No repository subscription found. Use /subscribe first.")
		return
	}

	repoIDs := make([]uint, len(subs))
	for i, s := range subs {
		repoIDs[i] = s.RepositoryID
	}

	argStr := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/label_reviewers"))

	if argStr == "" {
		var labelReviewers []models.LabelReviewer
		c.db.Where("repository_id IN ?", repoIDs).Preload("User").Find(&labelReviewers)

		if len(labelReviewers) == 0 {
			c.sendReply(msg, "No label reviewers configured.")
			return
		}

		labelMap := make(map[string][]string)
		for _, lr := range labelReviewers {
			labelMap[lr.LabelName] = append(labelMap[lr.LabelName], lr.User.Username)
		}

		var lines []string
		for label, users := range labelMap {
			lines = append(lines, fmt.Sprintf("%s: %s", label, strings.Join(users, ", ")))
		}
		c.sendReply(msg, "Label reviewers:\n"+strings.Join(lines, "\n"))
		return
	}

	parts := strings.SplitN(argStr, " ", 2)
	labelName := strings.TrimSpace(parts[0])

	if len(parts) == 1 {
		c.db.Where("repository_id IN ? AND label_name = ?", repoIDs, labelName).Delete(&models.LabelReviewer{})
		c.sendReply(msg, fmt.Sprintf("Cleared reviewers for label '%s'", labelName))
		return
	}

	usernames := strings.Split(parts[1], ",")
	var added []string
	var notFound []string

	for _, uname := range usernames {
		uname = strings.TrimSpace(uname)
		if uname == "" {
			continue
		}

		var user models.User
		if err := c.db.Where("username = ?", uname).First(&user).Error; err != nil {
			users, _, glErr := c.glClient.Users.ListUsers(&gitlab.ListUsersOptions{Username: gitlab.Ptr(uname)})
			if glErr != nil || len(users) == 0 {
				notFound = append(notFound, uname)
				continue
			}
			userData := models.User{
				GitlabID:  users[0].ID,
				Username:  users[0].Username,
				Name:      users[0].Name,
				State:     users[0].State,
				AvatarURL: users[0].AvatarURL,
				WebURL:    users[0].WebURL,
				Email:     users[0].Email,
			}
			c.db.Where(models.User{GitlabID: users[0].ID}).Assign(userData).FirstOrCreate(&user)
		}

		for _, repoID := range repoIDs {
			lr := models.LabelReviewer{RepositoryID: repoID, LabelName: labelName, UserID: user.ID}
			c.db.FirstOrCreate(&lr, models.LabelReviewer{RepositoryID: repoID, LabelName: labelName, UserID: user.ID})
		}
		added = append(added, uname)
	}

	reply := fmt.Sprintf("Label '%s' reviewers set: %s", labelName, strings.Join(added, ", "))
	if len(notFound) > 0 {
		reply += fmt.Sprintf(". Not found: %s", strings.Join(notFound, ", "))
	}
	c.sendReply(msg, reply)
}

func (c *VKCommandConsumer) handleDailyDigestCommand(msg *botgolang.Message, from botgolang.Contact) {
	if msg.Chat.Type != "private" {
		c.sendReply(msg, "The /daily_digest command must be used in a private chat with the bot.")
		return
	}

	userID := fmt.Sprint(from.ID)
	var vkUser models.VKUser
	vkUserData := models.VKUser{
		UserID:    userID,
		FirstName: from.FirstName,
		LastName:  from.LastName,
	}
	if err := c.db.Where(models.VKUser{UserID: userID}).Assign(vkUserData).FirstOrCreate(&vkUser).Error; err != nil {
		log.Printf("failed to get or create VK user %s: %v", userID, err)
		c.sendReply(msg, "Failed to process user information. Please try again later.")
		return
	}

	argStr := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/daily_digest"))
	chatID := fmt.Sprint(msg.Chat.ID)

	var pref models.DailyDigestPreference
	isNew := c.db.Where("vk_user_id = ?", vkUser.ID).First(&pref).Error != nil

	if isNew {
		pref = models.DailyDigestPreference{
			VKUserID:       vkUser.ID,
			DMChatID:       chatID,
			Enabled:        false,
			TimezoneOffset: 3,
		}
	}

	pref.DMChatID = chatID

	if argStr == "" {
		pref.Enabled = !pref.Enabled
	} else if argStr == "off" {
		pref.Enabled = false
	} else {
		offset, err := parseTimezoneOffset(argStr)
		if err != nil {
			c.sendReply(msg, "Invalid timezone format. Use +N or -N (e.g., +3, -5).")
			return
		}
		pref.TimezoneOffset = offset
		pref.Enabled = true
	}

	if err := c.db.Save(&pref).Error; err != nil {
		log.Printf("failed to save daily digest preference for user %s: %v", userID, err)
		c.sendReply(msg, "Failed to save preferences. Please try again later.")
		return
	}

	status := "disabled"
	if pref.Enabled {
		offsetStr := fmt.Sprintf("+%d", pref.TimezoneOffset)
		if pref.TimezoneOffset < 0 {
			offsetStr = fmt.Sprintf("%d", pref.TimezoneOffset)
		}
		status = fmt.Sprintf("enabled at 10:00 UTC%s", offsetStr)
	}
	c.sendReply(msg, fmt.Sprintf("Daily digest is now %s.", status))
}

func (c *VKCommandConsumer) handleSubscribersCommand(msg *botgolang.Message) {
	var prefs []models.DailyDigestPreference
	c.db.Preload("VKUser").Where("enabled = ?", true).Find(&prefs)

	if len(prefs) == 0 {
		c.sendReply(msg, "No users subscribed to daily digests.")
		return
	}

	var lines []string
	for _, pref := range prefs {
		tzStr := formatTimezone(pref.TimezoneOffset)
		lines = append(lines, fmt.Sprintf("%s (%s)", pref.VKUser.Nick, tzStr))
	}
	c.sendReply(msg, "Daily digest subscribers:\n"+strings.Join(lines, "\n"))
}

func formatTimezone(offset int) string {
	if offset >= 0 {
		return fmt.Sprintf("UTC+%d", offset)
	}
	return fmt.Sprintf("UTC%d", offset)
}

func parseTimezoneOffset(s string) (int, error) {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return 0, fmt.Errorf("invalid offset")
	}

	sign := 1
	numStr := s
	if s[0] == '+' {
		numStr = s[1:]
	} else if s[0] == '-' {
		sign = -1
		numStr = s[1:]
	}

	offset, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, err
	}

	if offset < 0 || offset > 14 {
		return 0, fmt.Errorf("offset out of range")
	}

	return sign * offset, nil
}

func formatSLADuration(d time.Duration) string {
	if d == 0 {
		return "not set"
	}
	return utils.FormatDuration(d)
}

func (c *VKCommandConsumer) handleAddBlockLabelCommand(msg *botgolang.Message, _ botgolang.Contact) {
	chatID := fmt.Sprint(msg.Chat.ID)
	var chat models.Chat
	if err := c.db.Where("chat_id = ?", chatID).First(&chat).Error; err != nil {
		c.sendReply(msg, "Chat not found")
		return
	}

	var subs []models.RepositorySubscription
	c.db.Where("chat_id = ?", chat.ID).Preload("Repository").Find(&subs)
	if len(subs) == 0 {
		c.sendReply(msg, "No repository subscription found. Use /subscribe first.")
		return
	}

	argStr := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/add_block_label"))
	if argStr == "" {
		c.sendReply(msg, "Usage: /add_block_label <label1> [#color1], <label2> [#color2], ...\nDefault color: #dc143c (crimson)")
		return
	}

	labelSpecs := parseLabelSpecs(argStr)
	if len(labelSpecs) == 0 {
		c.sendReply(msg, "Usage: /add_block_label <label1> [#color1], <label2> [#color2], ...\nDefault color: #dc143c (crimson)")
		return
	}

	var successRepos []string

	for _, sub := range subs {
		repo := sub.Repository

		for _, spec := range labelSpecs {
			labels, _, err := c.glClient.Labels.ListLabels(repo.GitlabID, &gitlab.ListLabelsOptions{
				Search: gitlab.Ptr(spec.name),
			})

			labelExists := false
			if err == nil {
				for _, l := range labels {
					if l.Name == spec.name {
						labelExists = true
						break
					}
				}
			}

			if !labelExists {
				_, _, err := c.glClient.Labels.CreateLabel(repo.GitlabID, &gitlab.CreateLabelOptions{
					Name:  gitlab.Ptr(spec.name),
					Color: gitlab.Ptr(spec.color),
				})
				if err != nil {
					log.Printf("failed to create label %s in repo %d: %v", spec.name, repo.GitlabID, err)
					c.sendReply(msg, fmt.Sprintf("Failed to create label '%s' in repo %s: %v", spec.name, repo.Name, err))
					return
				}
			}

			blockLabel := models.BlockLabel{
				RepositoryID: repo.ID,
				LabelName:    spec.name,
			}
			if err := c.db.FirstOrCreate(&blockLabel, models.BlockLabel{
				RepositoryID: repo.ID,
				LabelName:    spec.name,
			}).Error; err != nil {
				log.Printf("failed to save block label %s for repo %d: %v", spec.name, repo.ID, err)
				c.sendReply(msg, fmt.Sprintf("Failed to save block label '%s' for repo %s: %v", spec.name, repo.Name, err))
				return
			}
		}

		successRepos = append(successRepos, repo.Name)
	}

	labelNames := make([]string, len(labelSpecs))
	for i, spec := range labelSpecs {
		labelNames[i] = spec.name
	}
	c.sendReply(msg, fmt.Sprintf("Block label(s) '%s' added for: %s", strings.Join(labelNames, ", "), strings.Join(successRepos, ", ")))
}

func (c *VKCommandConsumer) handleAddReleaseLabelCommand(msg *botgolang.Message, _ botgolang.Contact) {
	chatID := fmt.Sprint(msg.Chat.ID)
	var chat models.Chat
	if err := c.db.Where("chat_id = ?", chatID).First(&chat).Error; err != nil {
		c.sendReply(msg, "Chat not found")
		return
	}

	var subs []models.RepositorySubscription
	c.db.Where("chat_id = ?", chat.ID).Preload("Repository").Find(&subs)
	if len(subs) == 0 {
		c.sendReply(msg, "No repository subscription found. Use /subscribe first.")
		return
	}

	argStr := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/add_release_label"))
	if argStr == "" {
		c.sendReply(msg, "Usage: /add_release_label <label_name> [#hexcolor]\nDefault color: #808080 (gray)")
		return
	}

	parts := strings.Fields(argStr)
	labelName := parts[0]
	color := defaultReleaseLabelColor

	if len(parts) >= 2 {
		lastPart := parts[len(parts)-1]
		if strings.HasPrefix(lastPart, "#") && isValidHexColor(lastPart) {
			color = lastPart
			if len(parts) > 2 {
				labelName = strings.Join(parts[:len(parts)-1], " ")
			}
		} else {
			labelName = argStr
		}
	}

	var successRepos []string
	var failedRepos []string

	for _, sub := range subs {
		repo := sub.Repository

		labels, _, err := c.glClient.Labels.ListLabels(repo.GitlabID, &gitlab.ListLabelsOptions{
			Search: gitlab.Ptr(labelName),
		})

		labelExists := false
		if err == nil {
			for _, l := range labels {
				if l.Name == labelName {
					labelExists = true
					break
				}
			}
		}

		if !labelExists {
			_, _, err := c.glClient.Labels.CreateLabel(repo.GitlabID, &gitlab.CreateLabelOptions{
				Name:  gitlab.Ptr(labelName),
				Color: gitlab.Ptr(color),
			})
			if err != nil {
				log.Printf("failed to create label %s in repo %d: %v", labelName, repo.GitlabID, err)
				failedRepos = append(failedRepos, repo.Name)
				continue
			}
		}

		releaseLabel := models.ReleaseLabel{
			RepositoryID: repo.ID,
			LabelName:    labelName,
		}
		if err := c.db.FirstOrCreate(&releaseLabel, models.ReleaseLabel{
			RepositoryID: repo.ID,
			LabelName:    labelName,
		}).Error; err != nil {
			log.Printf("failed to save release label %s for repo %d: %v", labelName, repo.ID, err)
			failedRepos = append(failedRepos, repo.Name)
			continue
		}

		successRepos = append(successRepos, repo.Name)
	}

	var reply string
	if len(successRepos) > 0 {
		reply = fmt.Sprintf("Release label '%s' added for: %s", labelName, strings.Join(successRepos, ", "))
	}
	if len(failedRepos) > 0 {
		if reply != "" {
			reply += "\n"
		}
		reply += fmt.Sprintf("Failed for: %s", strings.Join(failedRepos, ", "))
	}
	if reply == "" {
		reply = "No repositories were updated."
	}
	c.sendReply(msg, reply)
}

func (c *VKCommandConsumer) handleAddReleaseReadyLabelCommand(msg *botgolang.Message, _ botgolang.Contact) {
	chatID := fmt.Sprint(msg.Chat.ID)
	var chat models.Chat
	if err := c.db.Where("chat_id = ?", chatID).First(&chat).Error; err != nil {
		c.sendReply(msg, "Chat not found")
		return
	}

	var subs []models.RepositorySubscription
	c.db.Where("chat_id = ?", chat.ID).Preload("Repository").Find(&subs)
	if len(subs) == 0 {
		c.sendReply(msg, "No repository subscription found. Use /subscribe first.")
		return
	}

	argStr := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/add_release_ready_label"))
	if argStr == "" {
		c.sendReply(msg, "Usage: /add_release_ready_label <label_name> [#hexcolor]\nDefault color: #FFD700 (gold)")
		return
	}

	parts := strings.Fields(argStr)
	labelName := parts[0]
	color := defaultReleaseReadyLabelColor

	if len(parts) >= 2 {
		lastPart := parts[len(parts)-1]
		if strings.HasPrefix(lastPart, "#") && isValidHexColor(lastPart) {
			color = lastPart
			if len(parts) > 2 {
				labelName = strings.Join(parts[:len(parts)-1], " ")
			}
		} else {
			labelName = argStr
		}
	}

	var successRepos []string
	var failedRepos []string

	for _, sub := range subs {
		repo := sub.Repository

		labels, _, err := c.glClient.Labels.ListLabels(repo.GitlabID, &gitlab.ListLabelsOptions{
			Search: gitlab.Ptr(labelName),
		})

		labelExists := false
		if err == nil {
			for _, l := range labels {
				if l.Name == labelName {
					labelExists = true
					break
				}
			}
		}

		if !labelExists {
			_, _, err := c.glClient.Labels.CreateLabel(repo.GitlabID, &gitlab.CreateLabelOptions{
				Name:  gitlab.Ptr(labelName),
				Color: gitlab.Ptr(color),
			})
			if err != nil {
				log.Printf("failed to create label %s in repo %d: %v", labelName, repo.GitlabID, err)
				failedRepos = append(failedRepos, repo.Name)
				continue
			}
		}

		releaseReadyLabel := models.ReleaseReadyLabel{
			RepositoryID: repo.ID,
			LabelName:    labelName,
		}
		if err := c.db.FirstOrCreate(&releaseReadyLabel, models.ReleaseReadyLabel{
			RepositoryID: repo.ID,
			LabelName:    labelName,
		}).Error; err != nil {
			log.Printf("failed to save release ready label %s for repo %d: %v", labelName, repo.ID, err)
			failedRepos = append(failedRepos, repo.Name)
			continue
		}

		successRepos = append(successRepos, repo.Name)
	}

	var reply string
	if len(successRepos) > 0 {
		reply = fmt.Sprintf("Release ready label '%s' added for: %s", labelName, strings.Join(successRepos, ", "))
	}
	if len(failedRepos) > 0 {
		if reply != "" {
			reply += "\n"
		}
		reply += fmt.Sprintf("Failed for: %s", strings.Join(failedRepos, ", "))
	}
	if reply == "" {
		reply = "No repositories were updated."
	}
	c.sendReply(msg, reply)
}

func (c *VKCommandConsumer) handleEnsureLabelCommand(msg *botgolang.Message, _ botgolang.Contact) {
	chatID := fmt.Sprint(msg.Chat.ID)
	var chat models.Chat
	if err := c.db.Where("chat_id = ?", chatID).First(&chat).Error; err != nil {
		c.sendReply(msg, "Chat not found")
		return
	}

	var subs []models.RepositorySubscription
	c.db.Where("chat_id = ?", chat.ID).Preload("Repository").Find(&subs)
	if len(subs) == 0 {
		c.sendReply(msg, "No repository subscription found. Use /subscribe first.")
		return
	}

	argStr := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/ensure_label"))
	if argStr == "" {
		c.sendReply(msg, "Usage: /ensure_label <label_name> <#hexcolor>")
		return
	}

	parts := strings.Fields(argStr)
	if len(parts) < 2 {
		c.sendReply(msg, "Usage: /ensure_label <label_name> <#hexcolor>")
		return
	}

	color := parts[len(parts)-1]
	if !strings.HasPrefix(color, "#") || !isValidHexColor(color) {
		c.sendReply(msg, "Invalid hex color. Use format: #RRGGBB or #RGB")
		return
	}

	labelName := strings.Join(parts[:len(parts)-1], " ")

	var createdRepos []string
	var existsRepos []string
	var failedRepos []string

	for _, sub := range subs {
		repo := sub.Repository

		labels, _, err := c.glClient.Labels.ListLabels(repo.GitlabID, &gitlab.ListLabelsOptions{
			Search: gitlab.Ptr(labelName),
		})

		labelExists := false
		if err == nil {
			for _, l := range labels {
				if l.Name == labelName {
					labelExists = true
					break
				}
			}
		}

		if labelExists {
			existsRepos = append(existsRepos, repo.Name)
			continue
		}

		_, _, err = c.glClient.Labels.CreateLabel(repo.GitlabID, &gitlab.CreateLabelOptions{
			Name:  gitlab.Ptr(labelName),
			Color: gitlab.Ptr(color),
		})
		if err != nil {
			log.Printf("failed to create label %s in repo %d: %v", labelName, repo.GitlabID, err)
			failedRepos = append(failedRepos, repo.Name)
			continue
		}

		createdRepos = append(createdRepos, repo.Name)
	}

	var parts2 []string
	if len(createdRepos) > 0 {
		parts2 = append(parts2, fmt.Sprintf("Created: %s", strings.Join(createdRepos, ", ")))
	}
	if len(existsRepos) > 0 {
		parts2 = append(parts2, fmt.Sprintf("Already exists: %s", strings.Join(existsRepos, ", ")))
	}
	if len(failedRepos) > 0 {
		parts2 = append(parts2, fmt.Sprintf("Failed: %s", strings.Join(failedRepos, ", ")))
	}

	if len(parts2) == 0 {
		c.sendReply(msg, "No repositories were processed.")
		return
	}

	reply := fmt.Sprintf("Label '%s' (%s):\n%s", labelName, color, strings.Join(parts2, "\n"))
	c.sendReply(msg, reply)
}

type labelSpec struct {
	name  string
	color string
}

func parseLabelSpecs(argStr string) []labelSpec {
	var specs []labelSpec
	argStr = strings.TrimSpace(argStr)
	if argStr == "" {
		return specs
	}

	entries := splitRespectingQuotes(argStr, ',')

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		name, color := parseLabelEntry(entry)
		if name != "" {
			specs = append(specs, labelSpec{name: name, color: color})
		}
	}
	return specs
}

func splitRespectingQuotes(s string, delim rune) []string {
	var result []string
	var current strings.Builder
	inQuotes := false

	for _, r := range s {
		if r == '"' {
			inQuotes = !inQuotes
			current.WriteRune(r)
		} else if r == delim && !inQuotes {
			result = append(result, current.String())
			current.Reset()
		} else {
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		result = append(result, current.String())
	}
	return result
}

func parseLabelEntry(entry string) (name, color string) {
	color = defaultBlockLabelColor
	entry = strings.TrimSpace(entry)

	if strings.HasPrefix(entry, "\"") {
		endQuote := strings.Index(entry[1:], "\"")
		if endQuote == -1 {
			name = strings.Trim(entry, "\"")
			return
		}
		name = entry[1 : endQuote+1]
		rest := strings.TrimSpace(entry[endQuote+2:])
		if strings.HasPrefix(rest, "#") && isValidHexColor(rest) {
			color = rest
		}
	} else {
		parts := strings.Fields(entry)
		if len(parts) == 0 {
			return "", ""
		}
		name = parts[0]
		if len(parts) >= 2 && strings.HasPrefix(parts[1], "#") && isValidHexColor(parts[1]) {
			color = parts[1]
		}
	}
	return
}

func isValidHexColor(s string) bool {
	if !strings.HasPrefix(s, "#") {
		return false
	}
	hex := s[1:]
	if len(hex) != 3 && len(hex) != 6 {
		return false
	}
	for _, c := range hex {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func (c *VKCommandConsumer) handleAutoReleaseBranchCommand(msg *botgolang.Message, _ botgolang.Contact) {
	chatID := fmt.Sprint(msg.Chat.ID)
	var chat models.Chat
	if err := c.db.Where("chat_id = ?", chatID).First(&chat).Error; err != nil {
		c.sendReply(msg, "Chat not found")
		return
	}

	var subs []models.RepositorySubscription
	c.db.Preload("Repository").Where("chat_id = ?", chat.ID).Find(&subs)
	if len(subs) == 0 {
		c.sendReply(msg, "No repository subscription found. Use /subscribe first.")
		return
	}

	repoIDs := make([]uint, len(subs))
	for i, s := range subs {
		repoIDs[i] = s.RepositoryID
	}

	argStr := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/auto_release_branch"))

	// No arguments: clear config
	if argStr == "" {
		if err := c.db.Where("repository_id IN ?", repoIDs).Delete(&models.AutoReleaseBranchConfig{}).Error; err != nil {
			c.sendReply(msg, "Failed to clear auto-release branch settings")
			return
		}
		c.sendReply(msg, "Auto-release branch settings cleared for subscribed repositories")
		return
	}

	// Parse: <prefix> : <dev-branch>
	parts := strings.SplitN(argStr, ":", 2)
	if len(parts) != 2 {
		c.sendReply(msg, "Usage: /auto_release_branch <release-branch-prefix> : <main-dev-branch>\nExample: /auto_release_branch release : develop\nCall without arguments to clear settings.")
		return
	}

	prefix := strings.TrimSpace(parts[0])
	devBranch := strings.TrimSpace(parts[1])

	if prefix == "" || devBranch == "" {
		c.sendReply(msg, "Both prefix and dev branch must be specified.\nUsage: /auto_release_branch <release-branch-prefix> : <main-dev-branch>")
		return
	}

	// Check that repositories have release labels configured
	var releaseLabels []models.ReleaseLabel
	c.db.Where("repository_id IN ?", repoIDs).Find(&releaseLabels)
	if len(releaseLabels) == 0 {
		c.sendReply(msg, "Auto-release requires a release label. Use /add_release_label first.")
		return
	}

	reposWithReleaseLabel := make(map[uint]bool)
	for _, rl := range releaseLabels {
		reposWithReleaseLabel[rl.RepositoryID] = true
	}

	var configuredRepos []string
	var skippedRepos []string

	for _, sub := range subs {
		if !reposWithReleaseLabel[sub.RepositoryID] {
			skippedRepos = append(skippedRepos, sub.Repository.Name+" (no release label)")
			continue
		}

		config := models.AutoReleaseBranchConfig{
			RepositoryID:        sub.RepositoryID,
			ReleaseBranchPrefix: prefix,
			DevBranchName:       devBranch,
		}

		if err := c.db.Where(models.AutoReleaseBranchConfig{RepositoryID: sub.RepositoryID}).
			Assign(config).
			FirstOrCreate(&config).Error; err != nil {
			log.Printf("failed to save auto-release config for repo %d: %v", sub.RepositoryID, err)
			skippedRepos = append(skippedRepos, sub.Repository.Name+" (error)")
			continue
		}

		configuredRepos = append(configuredRepos, sub.Repository.Name)
	}

	var reply string
	if len(configuredRepos) > 0 {
		reply = fmt.Sprintf("Auto-release configured (prefix: '%s', dev: '%s') for: %s",
			prefix, devBranch, strings.Join(configuredRepos, ", "))
	}
	if len(skippedRepos) > 0 {
		if reply != "" {
			reply += "\n"
		}
		reply += fmt.Sprintf("Skipped: %s", strings.Join(skippedRepos, ", "))
	}
	if reply == "" {
		reply = "No repositories were configured."
	}
	c.sendReply(msg, reply)
}

func (c *VKCommandConsumer) handleAddJiraPrefixCommand(msg *botgolang.Message, _ botgolang.Contact) {
	chatID := fmt.Sprint(msg.Chat.ID)
	var chat models.Chat
	if err := c.db.Where("chat_id = ?", chatID).First(&chat).Error; err != nil {
		c.sendReply(msg, "Chat not found")
		return
	}

	var subs []models.RepositorySubscription
	c.db.Where("chat_id = ?", chat.ID).Preload("Repository").Find(&subs)
	if len(subs) == 0 {
		c.sendReply(msg, "No repository subscription found. Use /subscribe first.")
		return
	}

	argStr := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/add_jira_prefix"))
	if argStr == "" {
		c.sendReply(msg, "Usage: /add_jira_prefix <PREFIX> (e.g., /add_jira_prefix INTDEV)")
		return
	}

	prefix := strings.ToUpper(strings.TrimSpace(argStr))

	matched, _ := regexp.MatchString(`^[A-Z]+$`, prefix)
	if !matched {
		c.sendReply(msg, "Invalid prefix format. Must be uppercase letters only (e.g., INTDEV)")
		return
	}

	var successRepos []string
	var failedRepos []string

	for _, sub := range subs {
		jiraPrefix := models.JiraProjectPrefix{
			RepositoryID: sub.RepositoryID,
			Prefix:       prefix,
		}
		if err := c.db.FirstOrCreate(&jiraPrefix, models.JiraProjectPrefix{
			RepositoryID: sub.RepositoryID,
			Prefix:       prefix,
		}).Error; err != nil {
			log.Printf("failed to save jira prefix %s for repo %d: %v", prefix, sub.RepositoryID, err)
			failedRepos = append(failedRepos, sub.Repository.Name)
			continue
		}
		successRepos = append(successRepos, sub.Repository.Name)
	}

	var reply string
	if len(successRepos) > 0 {
		reply = fmt.Sprintf("Jira prefix '%s' added for: %s", prefix, strings.Join(successRepos, ", "))
	}
	if len(failedRepos) > 0 {
		if reply != "" {
			reply += "\n"
		}
		reply += fmt.Sprintf("Failed for: %s", strings.Join(failedRepos, ", "))
	}
	if reply == "" {
		reply = "No repositories were updated."
	}
	c.sendReply(msg, reply)
}

func (c *VKCommandConsumer) handleReleaseSubscribeCommand(msg *botgolang.Message, from botgolang.Contact) {
	parts := strings.Fields(msg.Text)
	if len(parts) < 2 {
		c.sendReply(msg, "Usage: /release_subscribe <repository_id>")
		return
	}

	repoIDStr := strings.TrimSpace(parts[1])
	repoID, err := strconv.Atoi(repoIDStr)
	if err != nil {
		c.sendReply(msg, fmt.Sprintf("Invalid repository ID: %s", repoIDStr))
		return
	}

	var repo models.Repository
	if err := c.db.Where("gitlab_id = ?", repoID).First(&repo).Error; err != nil {
		c.sendReply(msg, fmt.Sprintf("Repository with ID %d not found", repoID))
		return
	}

	var autoReleaseConfig models.AutoReleaseBranchConfig
	if err := c.db.Where("repository_id = ?", repo.ID).First(&autoReleaseConfig).Error; err != nil {
		c.sendReply(msg, "Auto-release not configured. Use /auto_release_branch first.")
		return
	}

	var releaseReadyLabel models.ReleaseReadyLabel
	if err := c.db.Where("repository_id = ?", repo.ID).First(&releaseReadyLabel).Error; err != nil {
		c.sendReply(msg, "Release ready label not configured. Use /add_release_ready_label first.")
		return
	}

	chatID := fmt.Sprint(msg.Chat.ID)
	userID := fmt.Sprint(from.ID)
	var alreadySubscribed bool

	err = c.db.Transaction(func(tx *gorm.DB) error {
		var chat models.Chat
		chatData := models.Chat{
			ChatID: chatID,
			Type:   msg.Chat.Type,
			Title:  msg.Chat.Title,
		}
		if err := tx.Where(models.Chat{ChatID: chatID}).Assign(chatData).FirstOrCreate(&chat).Error; err != nil {
			return fmt.Errorf("failed to process chat: %w", err)
		}

		var user models.VKUser
		vkUserData := models.VKUser{
			UserID:    userID,
			FirstName: from.FirstName,
			LastName:  from.LastName,
		}
		if err := tx.Where(models.VKUser{UserID: userID}).Assign(vkUserData).FirstOrCreate(&user).Error; err != nil {
			return fmt.Errorf("failed to process user: %w", err)
		}

		var subscription models.ReleaseSubscription
		result := tx.Where(models.ReleaseSubscription{
			RepositoryID: repo.ID,
			ChatID:       chat.ID,
		}).Attrs(models.ReleaseSubscription{
			VKUserID:     user.ID,
			SubscribedAt: time.Now(),
		}).FirstOrCreate(&subscription)

		if result.Error != nil {
			return fmt.Errorf("failed to create subscription: %w", result.Error)
		}

		if result.RowsAffected == 0 {
			alreadySubscribed = true
		}

		return nil
	})

	if err != nil {
		log.Printf("release subscription transaction failed: %v", err)
		c.sendReply(msg, "Failed to create subscription. Please try again later.")
		return
	}

	if alreadySubscribed {
		c.sendReply(msg, fmt.Sprintf("This chat is already subscribed to release notifications for: %s", repo.Name))
		return
	}

	c.sendReply(msg, fmt.Sprintf("Subscribed to release notifications for: %s", repo.Name))
}

func (c *VKCommandConsumer) handleReleaseUnsubscribeCommand(msg *botgolang.Message, _ botgolang.Contact) {
	parts := strings.Fields(msg.Text)
	if len(parts) < 2 {
		c.sendReply(msg, "Usage: /release_unsubscribe <repository_id>")
		return
	}

	repoIDStr := strings.TrimSpace(parts[1])
	repoID, err := strconv.Atoi(repoIDStr)
	if err != nil {
		c.sendReply(msg, fmt.Sprintf("Invalid repository ID: %s", repoIDStr))
		return
	}

	var repo models.Repository
	if err := c.db.Where("gitlab_id = ?", repoID).First(&repo).Error; err != nil {
		c.sendReply(msg, fmt.Sprintf("Repository with ID %d not found", repoID))
		return
	}

	chatID := fmt.Sprint(msg.Chat.ID)
	var chat models.Chat
	if err := c.db.Where("chat_id = ?", chatID).First(&chat).Error; err != nil {
		c.sendReply(msg, "Chat not found")
		return
	}

	var sub models.ReleaseSubscription
	if err := c.db.Where("repository_id = ? AND chat_id = ?", repo.ID, chat.ID).First(&sub).Error; err != nil {
		c.sendReply(msg, fmt.Sprintf("No release subscription found for repository %s", repo.Name))
		return
	}

	if err := c.db.Delete(&sub).Error; err != nil {
		c.sendReply(msg, fmt.Sprintf("Failed to unsubscribe from release notifications for repository %s", repo.Name))
		return
	}

	c.sendReply(msg, fmt.Sprintf("Unsubscribed from release notifications for: %s", repo.Name))
}

func (c *VKCommandConsumer) sendReply(msg *botgolang.Message, text string) {
	replyMsg := c.vkBot.NewTextMessage(fmt.Sprint(msg.Chat.ID), text)
	err := replyMsg.Send()
	if err != nil {
		log.Printf("failed to send reply message: %v", err)
	}
}
