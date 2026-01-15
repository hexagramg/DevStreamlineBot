package consumers

import (
	"log"
	"time"

	botgolang "github.com/mail-ru-im/bot-golang"
	"gorm.io/gorm"

	"devstreamlinebot/models"
	"devstreamlinebot/utils"
)

type PersonalDigestConsumer struct {
	db    *gorm.DB
	vkBot *botgolang.Bot
}

func NewPersonalDigestConsumer(db *gorm.DB, vkBot *botgolang.Bot) *PersonalDigestConsumer {
	return &PersonalDigestConsumer{db: db, vkBot: vkBot}
}

func (c *PersonalDigestConsumer) StartConsumer() {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			c.checkAndSendDigests()
		}
	}()
}

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

func (c *PersonalDigestConsumer) processPreference(pref models.DailyDigestPreference) {
	userTime := time.Now().UTC().Add(time.Duration(pref.TimezoneOffset) * time.Hour)

	if userTime.Hour() != 10 || userTime.Minute() != 0 {
		return
	}

	userToday := time.Date(userTime.Year(), userTime.Month(), userTime.Day(), 0, 0, 0, 0, time.UTC)
	if pref.LastSentAt != nil {
		lastSentUserTime := pref.LastSentAt.UTC().Add(time.Duration(pref.TimezoneOffset) * time.Hour)
		lastSentDay := time.Date(lastSentUserTime.Year(), lastSentUserTime.Month(), lastSentUserTime.Day(), 0, 0, 0, 0, time.UTC)
		if !lastSentDay.Before(userToday) {
		}
	}

	if userTime.Weekday() == time.Saturday || userTime.Weekday() == time.Sunday {
		return
	}

	var gitlabUser models.User
	if err := c.db.Where("email = ?", pref.VKUser.UserID).First(&gitlabUser).Error; err != nil {
		log.Printf("failed to find GitLab user for VK user %s: %v", pref.VKUser.UserID, err)
		return
	}

	reviewMRs, fixesMRs, err := utils.FindUserActionMRs(c.db, gitlabUser.ID)
	if err != nil {
		log.Printf("failed to fetch actions for user %s: %v", gitlabUser.Username, err)
		return
	}

	releaseMRs, err := utils.FindReleaseManagerActionMRs(c.db, gitlabUser.ID)
	if err != nil {
		log.Printf("failed to fetch release manager MRs for user %s: %v", gitlabUser.Username, err)
	}

	if c.isHolidayForAllRepos(reviewMRs, fixesMRs, userTime) {
		return
	}

	text := utils.BuildUserActionsDigest(c.db, reviewMRs, fixesMRs, releaseMRs, gitlabUser.Username)
	msg := c.vkBot.NewTextMessage(pref.DMChatID, text)
	if err := msg.Send(); err != nil {
		log.Printf("failed to send personal digest to %s: %v", pref.VKUser.UserID, err)
		return
	}

	now := time.Now()
	pref.LastSentAt = &now
	if err := c.db.Save(&pref).Error; err != nil {
		log.Printf("failed to update LastSentAt for preference %d: %v", pref.ID, err)
	}
}

func (c *PersonalDigestConsumer) isHolidayForAllRepos(reviewMRs, fixesMRs []utils.DigestMR, userTime time.Time) bool {
	repoIDs := make(map[uint]bool)
	for _, dmr := range reviewMRs {
		repoIDs[dmr.MR.RepositoryID] = true
	}
	for _, dmr := range fixesMRs {
		repoIDs[dmr.MR.RepositoryID] = true
	}

	if len(repoIDs) == 0 {
	}

	repoIDSlice := make([]uint, 0, len(repoIDs))
	for id := range repoIDs {
		repoIDSlice = append(repoIDSlice, id)
	}

	todayStr := userTime.Format("2006-01-02")
	var holidayRepoCount int64
	c.db.Model(&models.Holiday{}).
		Where("repository_id IN ? AND DATE(date) = ?", repoIDSlice, todayStr).
		Distinct("repository_id").
		Count(&holidayRepoCount)

	return holidayRepoCount == int64(len(repoIDs))
}
