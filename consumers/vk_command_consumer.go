package consumers

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	botgolang "github.com/mail-ru-im/bot-golang"
	gitlab "gitlab.com/gitlab-org/api/client-go"
	"gorm.io/gorm"

	"devstreamlinebot/models"
	"devstreamlinebot/polling"
	"devstreamlinebot/utils"
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

	// Check commands
	if strings.HasPrefix(msg.Text, "/subscribe") {
		c.handleSubscribeCommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/unsubscribe") {
		c.handleUnsubscribeCommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/reviewers") {
		c.handleReviewersCommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/reviews") {
		c.handleReviewsCommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/send_digest") {
		c.handleSendDigestCommand(msg, from)
	} else if strings.HasPrefix(msg.Text, "/get_mr_info") {
		c.handleGetMRInfoCommand(msg, from)
	}
}

// handleSubscribeCommand processes the /subscribe command to link a chat with a repository.
// Format: /subscribe 1234 where 1234 is the GitLab repository ID
func (c *VKCommandConsumer) handleSubscribeCommand(msg *botgolang.Message, from botgolang.Contact) {
	parts := strings.Fields(msg.Text)
	if len(parts) < 2 {
		c.sendReply(msg, "Usage: /subscribe <repository_id>")
		return
	}

	// Parse repository ID
	repoIDStr := strings.TrimSpace(parts[1])
	repoIDStr = strings.TrimSuffix(repoIDStr, ",") // Remove trailing comma if present

	repoID, err := strconv.Atoi(repoIDStr)
	if err != nil {
		c.sendReply(msg, fmt.Sprintf("Invalid repository ID: %s", repoIDStr))
		return
	}

	// Find repository
	var repo models.Repository
	if err := c.db.Where("gitlab_id = ?", repoID).First(&repo).Error; err != nil {
		c.sendReply(msg, fmt.Sprintf("Repository with ID %d not found", repoID))
		return
	}

	// Get or create chat
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

	// Get or create user
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

	// Check if subscription already exists
	var existingSub models.RepositorySubscription
	if err := c.db.Where("repository_id = ? AND chat_id = ?", repo.ID, chat.ID).First(&existingSub).Error; err == nil {
		c.sendReply(msg, fmt.Sprintf("This chat is already subscribed to repository: %s", repo.Name))
		return
	}

	// Create subscription
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

	// Reply with success message
	c.sendReply(msg, fmt.Sprintf("Repository %s is now subscribed", repo.Name))
}

// handleUnsubscribeCommand processes the /unsubscribe command to remove a subscription.
// Format: /unsubscribe 1234 where 1234 is the GitLab repository ID
func (c *VKCommandConsumer) handleUnsubscribeCommand(msg *botgolang.Message, _ botgolang.Contact) {
	parts := strings.Fields(msg.Text)
	if len(parts) < 2 {
		c.sendReply(msg, "Usage: /unsubscribe <repository_id>")
		return
	}

	// Parse repository ID
	repoIDStr := strings.TrimSuffix(strings.TrimSpace(parts[1]), ",")
	repoID, err := strconv.Atoi(repoIDStr)
	if err != nil {
		c.sendReply(msg, fmt.Sprintf("Invalid repository ID: %s", repoIDStr))
		return
	}

	// Find repository
	var repo models.Repository
	if err := c.db.Where("gitlab_id = ?", repoID).First(&repo).Error; err != nil {
		c.sendReply(msg, fmt.Sprintf("Repository with ID %d not found", repoID))
		return
	}

	// Find chat
	chatID := fmt.Sprint(msg.Chat.ID)
	var chat models.Chat
	if err := c.db.Where("chat_id = ?", chatID).First(&chat).Error; err != nil {
		c.sendReply(msg, "Chat not found")
		return
	}

	// Find subscription
	var sub models.RepositorySubscription
	if err := c.db.Where("repository_id = ? AND chat_id = ?", repo.ID, chat.ID).First(&sub).Error; err != nil {
		c.sendReply(msg, fmt.Sprintf("No subscription found for repository %s", repo.Name))
		return
	}

	// Delete subscription
	if err := c.db.Delete(&sub).Error; err != nil {
		c.sendReply(msg, fmt.Sprintf("Failed to unsubscribe from repository %s", repo.Name))
		return
	}

	// Reply with success message
	c.sendReply(msg, fmt.Sprintf("Unsubscribed from repository %s", repo.Name))
}

// handleReviewersCommand processes the /reviewers command to set or clear possible reviewers.
// Format: /reviewers                -> clear all possible reviewers for the repo
//
//	/reviewers user1,user2,... -> set possible reviewers by GitLab username
func (c *VKCommandConsumer) handleReviewersCommand(msg *botgolang.Message, _ botgolang.Contact) {
	// Determine current repository by chat subscription
	chatID := fmt.Sprint(msg.Chat.ID)
	var chat models.Chat
	if err := c.db.Where("chat_id = ?", chatID).First(&chat).Error; err != nil {
		c.sendReply(msg, "Chat not found in subscriptions")
		return
	}
	var subs []models.RepositorySubscription
	c.db.Where("chat_id = ?", chat.ID).Find(&subs)
	if len(subs) == 0 {
		c.sendReply(msg, "No repository subscription found. Use /subscribe first.")
		return
	}
	// Gather all repository IDs and names for subscriptions
	var repoIDs []uint
	var repoNames []string
	for _, s := range subs {
		// preload Repository
		var r models.Repository
		c.db.First(&r, s.RepositoryID)
		repoIDs = append(repoIDs, r.ID)
		repoNames = append(repoNames, r.Name)
	}

	// Parse command args
	argStr := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/reviewers"))
	if argStr == "" {
		// Clear all possible reviewers for all subscribed repositories
		if err := c.db.Where("repository_id IN ?", repoIDs).Delete(&models.PossibleReviewer{}).Error; err != nil {
			c.sendReply(msg, "Failed to clear reviewers")
			return
		}
		c.sendReply(msg, fmt.Sprintf("Cleared all reviewers for repositories: %s", strings.Join(repoNames, ",")))
		return
	}

	// Set reviewers list for each repository
	names := strings.Split(argStr, ",")
	var added []string
	var notFoundUsers []string
	for _, name := range names {
		uname := strings.TrimSpace(name)
		if uname == "" {
			continue
		}

		var user models.User
		// Try to find user in DB first
		err := c.db.Where("username = ?", uname).First(&user).Error

		if err != nil { // Not found in DB or other error
			if gorm.ErrRecordNotFound == err { // Specifically not found, try fetching from GitLab
				users, _, glErr := c.glClient.Users.ListUsers(&gitlab.ListUsersOptions{Username: gitlab.Ptr(uname)})
				if glErr != nil || len(users) == 0 {
					log.Printf("User %s not found in GitLab or API error: %v", uname, glErr)
					notFoundUsers = append(notFoundUsers, uname)
					continue // Skip this user
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
				// Upsert GitLab user
				if err := c.db.Where(models.User{GitlabID: glUser.ID}).Assign(userData).FirstOrCreate(&user).Error; err != nil {
					log.Printf("Failed to upsert GitLab user %s (ID: %d): %v", uname, glUser.ID, err)
					c.sendReply(msg, fmt.Sprintf("Error processing user: %s. Please try again.", uname))
					return // Abort on DB error during critical user upsert
				}
			} else { // Some other DB error
				log.Printf("DB error looking up user %s: %v", uname, err)
				c.sendReply(msg, fmt.Sprintf("Database error while looking up user: %s.", uname))
				return // Abort on other DB errors
			}
		}
		// Link as possible reviewer for all repos
		for _, rid := range repoIDs {
			pr := models.PossibleReviewer{RepositoryID: rid, UserID: user.ID}
			// Using FirstOrCreate for the join table is fine, ensure no duplicates.
			if err := c.db.FirstOrCreate(&pr, models.PossibleReviewer{RepositoryID: rid, UserID: user.ID}).Error; err != nil {
				log.Printf("Failed to create possible reviewer link for repo %d and user %d: %v", rid, user.ID, err)
				// Potentially notify about this specific failure but continue with others
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

// handleReviewsCommand processes the /reviews command to list merge requests where a user is a reviewer.
func (c *VKCommandConsumer) handleReviewsCommand(msg *botgolang.Message, from botgolang.Contact) {
	parts := strings.Fields(msg.Text)
	var username string
	if len(parts) < 2 {
		// No arg: resolve GitLab user from VK caller link
		vkID := fmt.Sprint(from.ID)
		var vkUser models.VKUser
		if err := c.db.Where("user_id = ?", vkID).First(&vkUser).Error; err != nil {
			c.sendReply(msg, "Cannot determine your account. Please specify a GitLab username: /reviews <username>")
			return
		}
		// Find GitLab user by email matching VKUser.UserID
		var user models.User
		if err := c.db.Where("email = ?", vkUser.UserID).First(&user).Error; err != nil {
			c.sendReply(msg, "No linked GitLab user found for your VK account. Please specify a username: /reviews <username>")
			return
		}
		username = user.Username
	} else {
		username = strings.TrimSpace(parts[1])
	}

	// Find GitLab user by username
	var user models.User
	if err := c.db.Where("username = ?", username).First(&user).Error; err != nil {
		c.sendReply(msg, fmt.Sprintf("User %s not found", username))
		return
	}

	// Find open merge requests where this user is a reviewer and has not approved
	var mrs []models.MergeRequest
	if err := c.db.
		Preload("Author").
		Where("merge_requests.state = ? AND merge_requests.merged_at IS NULL", "opened").
		Where("EXISTS (SELECT 1 FROM merge_request_reviewers mrr WHERE mrr.merge_request_id = merge_requests.id AND mrr.user_id = ?)", user.ID).
		Where("NOT EXISTS (SELECT 1 FROM merge_request_approvers mra WHERE mra.merge_request_id = merge_requests.id AND mra.user_id = ?)", user.ID).
		Find(&mrs).Error; err != nil {
		log.Printf("failed to fetch merge requests for reviewer %s: %v", username, err)
		c.sendReply(msg, "Failed to fetch reviews. Please try again later.")
		return
	}
	if len(mrs) == 0 {
		c.sendReply(msg, fmt.Sprintf("No pending reviews for user %s", username))
		return
	}

	// Build digest message
	text := fmt.Sprintf("REVIEWS FOR %s:\n", username)
	for _, mr := range mrs {
		text += fmt.Sprintf("- %s\n  %s\n  author: @[%s]\n", mr.Title, mr.WebURL, mr.Author.Username)
	}

	// Send reply
	replyMsg := c.vkBot.NewTextMessage(fmt.Sprint(msg.Chat.ID), text)
	if err := replyMsg.Send(); err != nil {
		log.Printf("failed to send reviews digest: %v", err)
	}
}

// handleSendDigestCommand sends an immediate review digest for the current chat.
func (c *VKCommandConsumer) handleSendDigestCommand(msg *botgolang.Message, _ botgolang.Contact) {
	chatID := fmt.Sprint(msg.Chat.ID)

	// Find chat in database
	var chat models.Chat
	if err := c.db.Where("chat_id = ?", chatID).First(&chat).Error; err != nil {
		c.sendReply(msg, "Chat not found in database")
		return
	}

	// Fetch repositories subscribed by this chat
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

	// Find open MRs with reviewers but no approvers in these repos
	mrs, err := utils.FindDigestMergeRequests(c.db, repoIDs)
	if err != nil {
		c.sendReply(msg, "Failed to fetch merge requests. Please try again later.")
		return
	}

	if len(mrs) == 0 {
		c.sendReply(msg, "No pending reviews found for subscribed repositories")
		return
	}

	// Build digest message
	text := utils.BuildReviewDigest(c.db, mrs)
	// Send digest
	c.sendReply(msg, text)
}

// handleGetMRInfoCommand processes the /get_mr_info command to fetch local MR info by reference (e.g., intdev/jobofferapp!2103).
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
	// Find repository by WebURL containing projectPath
	var repo models.Repository
	if err := c.db.Where("web_url LIKE ?", "%"+projectPath+"%").First(&repo).Error; err != nil {
		c.sendReply(msg, "Repository not found for this reference.")
		return
	}
	// Find MR by repo and IID, preload Author, Reviewers, Approvers
	var mr models.MergeRequest
	if err := c.db.Where("repository_id = ? AND iid = ?", repo.ID, mrIID).
		Preload("Author").
		Preload("Reviewers").
		Preload("Approvers").
		First(&mr).Error; err != nil {
		c.sendReply(msg, "Merge request not found in local database.")
		return
	}
	// Get reviewers and approvers usernames
	reviewerNames := make([]string, 0, len(mr.Reviewers))
	for _, u := range mr.Reviewers {
		reviewerNames = append(reviewerNames, "@"+u.Username)
	}
	approverNames := make([]string, 0, len(mr.Approvers))
	for _, u := range mr.Approvers {
		approverNames = append(approverNames, "@"+u.Username)
	}
	// Get active subscriptions (chat titles)
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
	// Format gitlab_created_at
	createdAt := ""
	if mr.GitlabCreatedAt != nil {
		createdAt = mr.GitlabCreatedAt.Format("2006-01-02 15:04:05")
	}
	// Build info message
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

// sendReply sends a reply message to the given message.
func (c *VKCommandConsumer) sendReply(msg *botgolang.Message, text string) {
	replyMsg := c.vkBot.NewTextMessage(fmt.Sprint(msg.Chat.ID), text)
	err := replyMsg.Send()
	if err != nil {
		log.Printf("failed to send reply message: %v", err)
	}
}
