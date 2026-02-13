package consumers

import (
	"errors"
	"fmt"
	"html"
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

	// Check if MR has a release label or feature release label
	releaseLabelNames := make(map[string]bool)

	var releaseLabels []models.ReleaseLabel
	c.db.Where("repository_id = ?", mr.RepositoryID).Find(&releaseLabels)
	for _, rl := range releaseLabels {
		releaseLabelNames[rl.LabelName] = true
	}

	var featureReleaseLabels []models.FeatureReleaseLabel
	c.db.Where("repository_id = ?", mr.RepositoryID).Find(&featureReleaseLabels)
	for _, frl := range featureReleaseLabels {
		releaseLabelNames[frl.LabelName] = true
	}

	hasReleaseLabel := false
	for _, label := range mr.Labels {
		if releaseLabelNames[label.Name] {
			hasReleaseLabel = true
			break
		}
	}

	if !hasReleaseLabel {
		log.Printf("release notification skipped for MR %d: no release label found among %d labels", mr.ID, len(mr.Labels))
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
		log.Printf("release notification skipped for MR %d: no release subscriptions for repo %d", mr.ID, mr.RepositoryID)
		c.markActionNotified(action.ID)
		return
	}

	releaseDate := time.Now().Format("02.01.2006")
	description := convertToVKHTML(mr.Description)
	message := fmt.Sprintf("Новый релиз %s %s (%s): <a href=\"%s\">Release MR</a>\n\n%s",
		html.EscapeString(repo.Name), releaseDate, html.EscapeString(mr.Title), mr.WebURL, description)

	for _, sub := range subs {
		if err := c.sendHTMLWithFallback(sub.Chat.ChatID, message); err != nil {
			log.Printf("failed to send release notification to chat %s: %v", sub.Chat.ChatID, err)
		}
	}

	c.db.Create(&models.MRNotificationState{
		MergeRequestID:      mr.ID,
		NotifiedDescription: mr.Description,
	})

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

	var releaseReadyLabel models.ReleaseReadyLabel
	if err := c.db.Where("repository_id = ?", repoID).First(&releaseReadyLabel).Error; err != nil {
		return
	}

	// Collect all release label names (both regular and feature release)
	allReleaseLabelNames := make(map[string]bool)

	var releaseLabels []models.ReleaseLabel
	c.db.Where("repository_id = ?", repoID).Find(&releaseLabels)
	for _, rl := range releaseLabels {
		allReleaseLabelNames[rl.LabelName] = true
	}

	var featureReleaseLabels []models.FeatureReleaseLabel
	c.db.Where("repository_id = ?", repoID).Find(&featureReleaseLabels)
	for _, frl := range featureReleaseLabels {
		allReleaseLabelNames[frl.LabelName] = true
	}

	if len(allReleaseLabelNames) == 0 {
		return
	}

	labelNamesList := make([]string, 0, len(allReleaseLabelNames))
	for name := range allReleaseLabelNames {
		labelNamesList = append(labelNamesList, name)
	}

	// Find all open MRs with any release/feature release label
	var releaseMRs []models.MergeRequest
	if err := c.db.
		Joins("JOIN merge_request_labels ON merge_request_labels.merge_request_id = merge_requests.id").
		Joins("JOIN labels ON labels.id = merge_request_labels.label_id").
		Where("merge_requests.repository_id = ? AND merge_requests.state = ? AND labels.name IN ?",
			repoID, "opened", labelNamesList).
		Preload("Labels").
		Find(&releaseMRs).Error; err != nil {
		return
	}

	for _, releaseMR := range releaseMRs {
		// Verify MR has both a release/feature release label AND the ready label
		hasAnyReleaseLabel := false
		hasReadyLabel := false
		for _, label := range releaseMR.Labels {
			if allReleaseLabelNames[label.Name] {
				hasAnyReleaseLabel = true
			}
			if label.Name == releaseReadyLabel.LabelName {
				hasReadyLabel = true
			}
		}

		if !hasAnyReleaseLabel || !hasReadyLabel {
			continue
		}

		c.notifyDescriptionChanges(releaseMR, repo, repoID)
	}
}

func (c *ReleaseNotificationConsumer) notifyDescriptionChanges(releaseMR models.MergeRequest, repo models.Repository, repoID uint) {
	var latest models.MRNotificationState
	err := c.db.Where("merge_request_id = ?", releaseMR.ID).
		Order("created_at desc").First(&latest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("seeding notification state for MR %d (first encounter)", releaseMR.ID)
		c.db.Create(&models.MRNotificationState{
			MergeRequestID:      releaseMR.ID,
			NotifiedDescription: releaseMR.Description,
		})
		return
	}
	if err != nil {
		log.Printf("failed to get notification state for MR %d: %v", releaseMR.ID, err)
		return
	}

	if releaseMR.Description == latest.NotifiedDescription {
		return
	}

	newEntries := extractNewEntries(latest.NotifiedDescription, releaseMR.Description)
	if len(newEntries) == 0 {
		c.db.Create(&models.MRNotificationState{
			MergeRequestID:      releaseMR.ID,
			NotifiedState:       latest.NotifiedState,
			NotifiedDescription: releaseMR.Description,
		})
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

	header := "Добавлена задача в релиз"
	if len(newEntries) > 1 {
		header = "Добавлены задачи в релиз"
	}
	message := fmt.Sprintf("%s %s (%s)\n%s", header, html.EscapeString(repo.Name), html.EscapeString(releaseMR.Title), convertToVKHTML(strings.Join(newEntries, "\n")))

	for _, sub := range subs {
		if err := c.sendHTMLWithFallback(sub.Chat.ChatID, message); err != nil {
			log.Printf("failed to send release update notification to chat %s: %v", sub.Chat.ChatID, err)
		}
	}

	c.db.Create(&models.MRNotificationState{
		MergeRequestID:      releaseMR.ID,
		NotifiedState:       latest.NotifiedState,
		NotifiedDescription: releaseMR.Description,
	})

	log.Printf("Sent release update notification for MR %d (%s) with %d new entries", releaseMR.ID, repo.Name, len(newEntries))
}

var linkRegex = regexp.MustCompile(`\[([^\]]*)\]\(([^)]*)\)`)

var linkPlaceholderRegex = regexp.MustCompile("\x00LINK:(.*?)\x00TEXT:(.*?)\x00END")

func convertToVKHTML(text string) string {
	lines := strings.Split(text, "\n")
	var result []string
	inList := false

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

		// Replace markdown links with placeholders before HTML escaping
		line = linkRegex.ReplaceAllStringFunc(line, func(match string) string {
			parts := linkRegex.FindStringSubmatch(match)
			if len(parts) == 3 {
				return "\x00LINK:" + parts[2] + "\x00TEXT:" + parts[1] + "\x00END"
			}
			return match
		})

		// Escape HTML in non-link text
		line = html.EscapeString(line)

		// Restore links as HTML <a> tags
		line = linkPlaceholderRegex.ReplaceAllString(line, `<a href="$1">$2</a>`)

		// Strip @ from mentions
		line = strings.ReplaceAll(line, "@", "")

		// Convert markdown list items to HTML list items
		isList := strings.HasPrefix(strings.TrimSpace(line), "- ")
		if isList {
			if !inList {
				result = append(result, "<ul>")
				inList = true
			}
			content := strings.TrimPrefix(strings.TrimSpace(line), "- ")
			result = append(result, "<li>"+content+"</li>")
		} else {
			if inList {
				result = append(result, "</ul>")
				inList = false
			}
			result = append(result, line)
		}
	}

	if inList {
		result = append(result, "</ul>")
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

func (c *ReleaseNotificationConsumer) sendHTMLWithFallback(chatID, text string) error {
	msg := c.vkBot.NewHTMLMessage(chatID, text)
	if err := msg.Send(); err != nil {
		if strings.Contains(err.Error(), "Format error") {
			log.Printf("HTML format rejected for chat %s, retrying as plain text", chatID)
			return c.vkBot.NewTextMessage(chatID, text).Send()
		}
		return err
	}
	return nil
}

// ProcessReleaseMergedNotifications sends notifications when a release MR is merged.
func (c *ReleaseNotificationConsumer) ProcessReleaseMergedNotifications() {
	var actions []models.MRAction
	if err := c.db.
		Where("action_type = ? AND notified = ?", models.ActionMerged, false).
		Preload("MergeRequest").
		Preload("MergeRequest.Repository").
		Preload("MergeRequest.Labels").
		Find(&actions).Error; err != nil {
		log.Printf("failed to fetch unnotified merged actions: %v", err)
		return
	}

	for _, action := range actions {
		c.processReleaseMergedAction(action)
	}
}

func (c *ReleaseNotificationConsumer) processReleaseMergedAction(action models.MRAction) {
	mr := action.MergeRequest
	repo := mr.Repository

	releaseLabelNames := make(map[string]bool)

	var releaseLabels []models.ReleaseLabel
	c.db.Where("repository_id = ?", mr.RepositoryID).Find(&releaseLabels)
	for _, rl := range releaseLabels {
		releaseLabelNames[rl.LabelName] = true
	}

	var featureReleaseLabels []models.FeatureReleaseLabel
	c.db.Where("repository_id = ?", mr.RepositoryID).Find(&featureReleaseLabels)
	for _, frl := range featureReleaseLabels {
		releaseLabelNames[frl.LabelName] = true
	}

	hasReleaseLabel := false
	for _, label := range mr.Labels {
		if releaseLabelNames[label.Name] {
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

	message := fmt.Sprintf("Релиз ушел на золото %s: <a href=\"%s\">%s</a>",
		html.EscapeString(mr.Title), mr.WebURL, html.EscapeString(mr.Title))

	for _, sub := range subs {
		if err := c.sendHTMLWithFallback(sub.Chat.ChatID, message); err != nil {
			log.Printf("failed to send release merged notification to chat %s: %v", sub.Chat.ChatID, err)
		}
	}

	c.markActionNotified(action.ID)
	log.Printf("Sent release merged notification for MR %d (%s) to %d chats", mr.ID, repo.Name, len(subs))
}

func (c *ReleaseNotificationConsumer) markActionNotified(actionID uint) {
	if err := c.db.Model(&models.MRAction{}).
		Where("id = ?", actionID).
		Update("notified", true).Error; err != nil {
		log.Printf("failed to mark action %d as notified: %v", actionID, err)
	}
}
