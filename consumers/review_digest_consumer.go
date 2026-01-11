package consumers

import (
	"log"
	"time"

	botgolang "github.com/mail-ru-im/bot-golang"
	"gorm.io/gorm"

	"devstreamlinebot/models"
	"devstreamlinebot/utils"
)

// ReviewDigestConsumer sends a daily summary of open merge requests awaiting review approvals.
// It runs at 10:00 on every weekday (Monday to Friday).
type ReviewDigestConsumer struct {
	db    *gorm.DB
	vkBot *botgolang.Bot
}

// NewReviewDigestConsumer initializes a ReviewDigestConsumer.
func NewReviewDigestConsumer(db *gorm.DB, vkBot *botgolang.Bot) *ReviewDigestConsumer {
	return &ReviewDigestConsumer{db: db, vkBot: vkBot}
}

// StartConsumer schedules the daily digest at 10:00 on weekdays.
func (c *ReviewDigestConsumer) StartConsumer() {
	go func() {
		for {
			now := time.Now()
			// calculate next 10:00
			loc := now.Location()
			next := time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, loc)
			if now.After(next) || next.Weekday() == time.Saturday || next.Weekday() == time.Sunday {
				// advance to next weekday
				days := 1
				for {
					candidate := next.AddDate(0, 0, days)
					if wd := candidate.Weekday(); wd != time.Saturday && wd != time.Sunday {
						next = candidate
						break
					}
					days++
				}
			}
			sleepDur := next.Sub(now)
			time.Sleep(sleepDur)

			// send digest
			c.sendDigest()
		}
	}()
}

// sendDigest fetches chats with subscriptions and sends the merge request review digest.
func (c *ReviewDigestConsumer) sendDigest() {
	// find all chats that have at least one subscription
	var chats []models.Chat
	if err := c.db.
		Model(&models.Chat{}).
		Joins("JOIN repository_subscriptions ON repository_subscriptions.chat_id = chats.id").
		Group("chats.id").
		Find(&chats).Error; err != nil {
		log.Printf("failed to fetch subscribed chats: %v", err)
		return
	}

	for _, chat := range chats {
		// fetch repositories subscribed by this chat
		var subs []models.RepositorySubscription
		if err := c.db.
			Preload("Repository").
			Where("chat_id = ?", chat.ID).
			Find(&subs).Error; err != nil {
			log.Printf("failed to fetch subscriptions for chat %s: %v", chat.ChatID, err)
			continue
		}
		var repoIDs []uint
		for _, s := range subs {
			repoIDs = append(repoIDs, s.RepositoryID)
		}
		if len(repoIDs) == 0 {
			continue
		}

		// find open MRs with state information for enhanced digest
		digestMRs, err := utils.FindDigestMergeRequestsWithState(c.db, repoIDs)
		if err != nil {
			log.Printf("failed to fetch pending MRs for chat %s: %v", chat.ChatID, err)
			continue
		}
		if len(digestMRs) == 0 {
			continue // nothing to report
		}

		// build enhanced message with PENDING REVIEW and PENDING FIXES sections
		text := utils.BuildEnhancedReviewDigest(c.db, digestMRs)
		msg := c.vkBot.NewTextMessage(chat.ChatID, text)
		if err := msg.Send(); err != nil {
			log.Printf("failed to send review digest to chat %s: %v", chat.ChatID, err)
		}
	}
}
