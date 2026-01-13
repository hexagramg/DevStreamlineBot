package consumers

import (
	"log"
	"time"

	botgolang "github.com/mail-ru-im/bot-golang"
	"gorm.io/gorm"

	"devstreamlinebot/models"
	"devstreamlinebot/utils"
)

// PersonalDigestConsumer sends personalized daily digests to users who have opted in.
// It runs at 10:00 in each user's configured timezone.
type PersonalDigestConsumer struct {
	db    *gorm.DB
	vkBot *botgolang.Bot
}

// NewPersonalDigestConsumer creates a new PersonalDigestConsumer.
func NewPersonalDigestConsumer(db *gorm.DB, vkBot *botgolang.Bot) *PersonalDigestConsumer {
	return &PersonalDigestConsumer{db: db, vkBot: vkBot}
}

// StartConsumer starts the scheduler that checks every minute for digests to send.
func (c *PersonalDigestConsumer) StartConsumer() {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			c.checkAndSendDigests()
		}
	}()
}

// checkAndSendDigests checks all enabled preferences and sends digests where appropriate.
func (c *PersonalDigestConsumer) checkAndSendDigests() {
	var prefs []models.DailyDigestPreference
	if err := c.db.Where("enabled = ?", true).Preload("VKUser").Find(&prefs).Error; err != nil {
		log.Printf("failed to fetch daily digest preferences: %v", err)
		return
	}

	for _, pref := range prefs {
		c.processPreference(pref)
	}
}

// processPreference checks if a digest should be sent for a specific user preference.
func (c *PersonalDigestConsumer) processPreference(pref models.DailyDigestPreference) {
	// Calculate current time in user's timezone
	userTime := time.Now().UTC().Add(time.Duration(pref.TimezoneOffset) * time.Hour)

	// Check if it's 10:00 in user's timezone
	if userTime.Hour() != 10 || userTime.Minute() != 0 {
		return
	}

	// Check if already sent today (in user's timezone)
	userToday := time.Date(userTime.Year(), userTime.Month(), userTime.Day(), 0, 0, 0, 0, time.UTC)
	if pref.LastSentAt != nil {
		lastSentUserTime := pref.LastSentAt.UTC().Add(time.Duration(pref.TimezoneOffset) * time.Hour)
		lastSentDay := time.Date(lastSentUserTime.Year(), lastSentUserTime.Month(), lastSentUserTime.Day(), 0, 0, 0, 0, time.UTC)
		if !lastSentDay.Before(userToday) {
			return // Already sent today
		}
	}

	// Check if today is weekend
	if userTime.Weekday() == time.Saturday || userTime.Weekday() == time.Sunday {
		return
	}

	// Find the GitLab user linked to this VK user
	var gitlabUser models.User
	if err := c.db.Where("email = ?", pref.VKUser.UserID).First(&gitlabUser).Error; err != nil {
		log.Printf("failed to find GitLab user for VK user %s: %v", pref.VKUser.UserID, err)
		return
	}

	// Fetch user's pending actions
	reviewMRs, fixesMRs, err := utils.FindUserActionMRs(c.db, gitlabUser.ID)
	if err != nil {
		log.Printf("failed to fetch actions for user %s: %v", gitlabUser.Username, err)
		return
	}

	// Check if today is a holiday in all repos with pending actions
	if c.isHolidayForAllRepos(reviewMRs, fixesMRs, userTime) {
		return
	}

	// Build and send digest
	text := utils.BuildUserActionsDigest(c.db, reviewMRs, fixesMRs, gitlabUser.Username)
	msg := c.vkBot.NewTextMessage(pref.DMChatID, text)
	if err := msg.Send(); err != nil {
		log.Printf("failed to send personal digest to %s: %v", pref.VKUser.UserID, err)
		return
	}

	// Update LastSentAt
	now := time.Now()
	pref.LastSentAt = &now
	if err := c.db.Save(&pref).Error; err != nil {
		log.Printf("failed to update LastSentAt for preference %d: %v", pref.ID, err)
	}
}

// isHolidayForAllRepos checks if today is a holiday in all repositories where user has pending actions.
// Returns true if there are actions and ALL repos have today as a holiday.
// Returns false if no actions or at least one repo doesn't have today as holiday.
func (c *PersonalDigestConsumer) isHolidayForAllRepos(reviewMRs, fixesMRs []utils.DigestMR, userTime time.Time) bool {
	// Collect unique repository IDs
	repoIDs := make(map[uint]bool)
	for _, dmr := range reviewMRs {
		repoIDs[dmr.MR.RepositoryID] = true
	}
	for _, dmr := range fixesMRs {
		repoIDs[dmr.MR.RepositoryID] = true
	}

	if len(repoIDs) == 0 {
		return false // No pending actions, not a holiday concern
	}

	// Check holidays for each repo
	todayStr := userTime.Format("2006-01-02")
	for repoID := range repoIDs {
		var count int64
		c.db.Model(&models.Holiday{}).
			Where("repository_id = ? AND DATE(date) = ?", repoID, todayStr).
			Count(&count)
		if count == 0 {
			return false // At least one repo doesn't have today as holiday
		}
	}

	return true // All repos have today as holiday
}
