package polling

import (
	"log"
	"time"

	"devstreamlinebot/models"

	gitlab "gitlab.com/gitlab-org/api/client-go"
	"gorm.io/gorm"
)

func StartUserEmailPolling(db *gorm.DB, client *gitlab.Client, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()

		lastAPICall := time.Now().Add(-10 * time.Second)

		for range ticker.C {
			var users []models.User
			if err := db.Where("email = '' AND email_fetched = false").Find(&users).Error; err != nil {
				log.Printf("failed to query users without email: %v", err)
				continue
			}
			for _, u := range users {
				elapsed := time.Since(lastAPICall)
				if elapsed < 10*time.Second {
					waitTime := 10*time.Second - elapsed
					time.Sleep(waitTime)
				}

				glUser, _, err := client.Users.GetUser(u.GitlabID, gitlab.GetUsersOptions{})
				lastAPICall = time.Now()

				if err != nil {
					log.Printf("failed to fetch user %d from GitLab: %v", u.GitlabID, err)
					continue
				}
				if glUser.PublicEmail != "" && glUser.PublicEmail != u.Email {
					u.Email = glUser.PublicEmail
				}
				now := time.Now()
				if err := db.Model(&u).Updates(map[string]interface{}{"email": u.Email, "locked": glUser.Locked, "email_fetched": true, "updated_at": now}).Error; err != nil {
					log.Printf("failed to update email for user %d: %v", u.GitlabID, err)
				}
			}

			var mappings []struct {
				UserID   uint
				NewEmail string
			}
			if err := db.Table("users u").Select("u.id as user_id, vu.user_id as new_email").
				Joins("JOIN vk_users vu on vu.user_id LIKE (u.username || '@%')").
				Where("u.email = '' AND u.email_fetched = true AND u.username <> ''").
				Order("u.id, vu.id desc").
				Scan(&mappings).Error; err != nil {
				log.Printf("failed to batch join query VK users: %v", err)
				continue
			}
			newMap := make(map[uint]string, len(mappings))
			for _, m := range mappings {
				if _, exists := newMap[m.UserID]; !exists {
					newMap[m.UserID] = m.NewEmail
				}
			}
			for userID, email := range newMap {
				now := time.Now()
				if err := db.Model(&models.User{}).Where("id = ?", userID).
					Updates(map[string]interface{}{"email": email, "updated_at": &now}).Error; err != nil {
					log.Printf("failed to update email for user ID %d: %v", userID, err)
				} else {
					log.Printf("updated user ID %d email to %s", userID, email)
				}
			}

			oneDayAgo := time.Now().Add(-24 * time.Hour)
			result := db.Model(&models.User{}).
				Where("email = '' AND email_fetched = true AND username NOT LIKE '%--%' AND updated_at < ?", oneDayAgo).
				Updates(map[string]interface{}{"email_fetched": false})

			if result.Error != nil {
				log.Printf("failed to reset email_fetched flag: %v", result.Error)
			} else if result.RowsAffected > 0 {
				log.Printf("reset email_fetched flag for %d users that were updated more than a day ago and have no email", result.RowsAffected)
			}
		}
	}()
}
