package consumers

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	botgolang "github.com/mail-ru-im/bot-golang"
	"gorm.io/gorm"

	"devstreamlinebot/interfaces"
	"devstreamlinebot/models"
)

type ReleaseNotificationConsumer struct {
	db    *gorm.DB
	vkBot interfaces.VKBot
}

func NewReleaseNotificationConsumer(db *gorm.DB, vkBot *botgolang.Bot) *ReleaseNotificationConsumer {
	return &ReleaseNotificationConsumer{
		db:    db,
		vkBot: &interfaces.RealVKBot{Bot: vkBot},
	}
}

func NewReleaseNotificationConsumerWithBot(db *gorm.DB, vkBot interfaces.VKBot) *ReleaseNotificationConsumer {
	return &ReleaseNotificationConsumer{
		db:    db,
		vkBot: vkBot,
	}
}

// ProcessNewReleaseNotifications sends notifications when a release MR gets the ReleaseReadyLabel.
func (c *ReleaseNotificationConsumer) ProcessNewReleaseNotifications() {
	var actions []models.MRAction
	if err := c.db.
		Where("action_type = ? AND notified = ?", models.ActionReleaseReadyLabelAdded, false).
		Preload("MergeRequest").
		Preload("MergeRequest.Repository").
		Preload("MergeRequest.Labels").
		Find(&actions).Error; err != nil {
		log.Printf("failed to fetch unnotified release ready label actions: %v", err)
		return
	}

	for _, action := range actions {
		c.processNewReleaseAction(action)
	}
}

func (c *ReleaseNotificationConsumer) processNewReleaseAction(action models.MRAction) {
	mr := action.MergeRequest
	repo := mr.Repository

	var releaseLabel models.ReleaseLabel
	if err := c.db.Where("repository_id = ?", mr.RepositoryID).First(&releaseLabel).Error; err != nil {
		c.markActionNotified(action.ID)
		return
	}

	hasReleaseLabel := false
	for _, label := range mr.Labels {
		if label.Name == releaseLabel.LabelName {
			hasReleaseLabel = true
			break
		}
	}

	if !hasReleaseLabel {
		c.markActionNotified(action.ID)
		return
	}

	var subs []models.ReleaseSubscription
	if err := c.db.
		Where("repository_id = ?", mr.RepositoryID).
		Preload("Chat").
		Find(&subs).Error; err != nil {
		log.Printf("failed to fetch release subscriptions for repo %d: %v", mr.RepositoryID, err)
		return
	}

	if len(subs) == 0 {
		c.markActionNotified(action.ID)
		return
	}

	releaseDate := time.Now().Format("02.01.2006")
	description := convertToVKMarkdown(mr.Description)
	message := fmt.Sprintf("Новый релиз %s %s: [Release MR](%s)\n\n%s", repo.Name, releaseDate, mr.WebURL, description)

	for _, sub := range subs {
		msg := c.vkBot.NewMarkdownMessage(sub.Chat.ChatID, message)
		if err := msg.Send(); err != nil {
			log.Printf("failed to send release notification to chat %s: %v", sub.Chat.ChatID, err)
		}
	}

	if err := c.db.Model(&models.MergeRequest{}).
		Where("id = ?", mr.ID).
		Update("last_notified_description", mr.Description).Error; err != nil {
		log.Printf("failed to update last_notified_description for MR %d: %v", mr.ID, err)
	}

	c.markActionNotified(action.ID)
	log.Printf("Sent new release notification for MR %d (%s) to %d chats", mr.ID, repo.Name, len(subs))
}

// ProcessReleaseMRDescriptionChanges sends notifications when new entries are added to release MR descriptions.
func (c *ReleaseNotificationConsumer) ProcessReleaseMRDescriptionChanges() {
	var subs []models.ReleaseSubscription
	if err := c.db.
		Select("DISTINCT repository_id").
		Find(&subs).Error; err != nil {
		log.Printf("failed to fetch release subscriptions: %v", err)
		return
	}

	for _, sub := range subs {
		c.processRepoDescriptionChanges(sub.RepositoryID)
	}
}

func (c *ReleaseNotificationConsumer) processRepoDescriptionChanges(repoID uint) {
	var repo models.Repository
	if err := c.db.First(&repo, repoID).Error; err != nil {
		return
	}

	var releaseLabel models.ReleaseLabel
	if err := c.db.Where("repository_id = ?", repoID).First(&releaseLabel).Error; err != nil {
		return
	}

	var releaseReadyLabel models.ReleaseReadyLabel
	if err := c.db.Where("repository_id = ?", repoID).First(&releaseReadyLabel).Error; err != nil {
		return
	}

	var releaseMR models.MergeRequest
	if err := c.db.
		Joins("JOIN merge_request_labels ON merge_request_labels.merge_request_id = merge_requests.id").
		Joins("JOIN labels ON labels.id = merge_request_labels.label_id").
		Where("merge_requests.repository_id = ? AND merge_requests.state = ? AND labels.name = ?",
			repoID, "opened", releaseLabel.LabelName).
		Preload("Labels").
		First(&releaseMR).Error; err != nil {
		return
	}

	hasReleaseLabel := false
	hasReleaseReadyLabel := false
	for _, label := range releaseMR.Labels {
		if label.Name == releaseLabel.LabelName {
			hasReleaseLabel = true
		}
		if label.Name == releaseReadyLabel.LabelName {
			hasReleaseReadyLabel = true
		}
	}

	if !hasReleaseLabel || !hasReleaseReadyLabel {
		return
	}

	if releaseMR.LastNotifiedDescription == "" {
		c.db.Model(&models.MergeRequest{}).
			Where("id = ?", releaseMR.ID).
			Update("last_notified_description", releaseMR.Description)
		return
	}

	if releaseMR.Description == releaseMR.LastNotifiedDescription {
		return
	}

	newEntries := extractNewEntries(releaseMR.LastNotifiedDescription, releaseMR.Description)
	if len(newEntries) == 0 {
		if err := c.db.Model(&models.MergeRequest{}).
			Where("id = ?", releaseMR.ID).
			Update("last_notified_description", releaseMR.Description).Error; err != nil {
			log.Printf("failed to update last_notified_description for MR %d: %v", releaseMR.ID, err)
		}
		return
	}

	var subs []models.ReleaseSubscription
	if err := c.db.
		Where("repository_id = ?", repoID).
		Preload("Chat").
		Find(&subs).Error; err != nil {
		log.Printf("failed to fetch release subscriptions for repo %d: %v", repoID, err)
		return
	}

	if len(subs) == 0 {
		return
	}

	message := fmt.Sprintf("Добавлена задача в релиз %s\n%s", repo.Name, strings.Join(newEntries, "\n"))

	for _, sub := range subs {
		msg := c.vkBot.NewMarkdownMessage(sub.Chat.ChatID, message)
		if err := msg.Send(); err != nil {
			log.Printf("failed to send release update notification to chat %s: %v", sub.Chat.ChatID, err)
		}
	}

	if err := c.db.Model(&models.MergeRequest{}).
		Where("id = ?", releaseMR.ID).
		Update("last_notified_description", releaseMR.Description).Error; err != nil {
		log.Printf("failed to update last_notified_description for MR %d: %v", releaseMR.ID, err)
	}

	log.Printf("Sent release update notification for MR %d (%s) with %d new entries", releaseMR.ID, repo.Name, len(newEntries))
}

func convertToVKMarkdown(text string) string {
	lines := strings.Split(text, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "---" || trimmed == "***" || trimmed == "___" {
			continue
		}

		if strings.HasPrefix(trimmed, "# ") {
			line = strings.TrimPrefix(trimmed, "# ")
		} else if strings.HasPrefix(trimmed, "## ") {
			line = strings.TrimPrefix(trimmed, "## ")
		} else if strings.HasPrefix(trimmed, "### ") {
			line = strings.TrimPrefix(trimmed, "### ")
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

func extractNewEntries(oldDesc, newDesc string) []string {
	urlRegex := regexp.MustCompile(`\]\((https?://[^\)]+/merge_requests/\d+)\)`)

	oldMatches := urlRegex.FindAllStringSubmatch(oldDesc, -1)
	oldURLs := make(map[string]bool)
	for _, match := range oldMatches {
		if len(match) >= 2 {
			oldURLs[match[1]] = true
		}
	}

	lineRegex := regexp.MustCompile(`(?m)^- .+$`)
	lines := lineRegex.FindAllString(newDesc, -1)

	var newEntries []string
	for _, line := range lines {
		urlMatches := urlRegex.FindAllStringSubmatch(line, -1)
		for _, match := range urlMatches {
			if len(match) >= 2 && !oldURLs[match[1]] {
				newEntries = append(newEntries, line)
				break
			}
		}
	}

	return newEntries
}

func (c *ReleaseNotificationConsumer) markActionNotified(actionID uint) {
	if err := c.db.Model(&models.MRAction{}).
		Where("id = ?", actionID).
		Update("notified", true).Error; err != nil {
		log.Printf("failed to mark action %d as notified: %v", actionID, err)
	}
}
