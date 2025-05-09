package polling

import (
	"log"
	"time"

	"devstreamlinebot/models"

	gitlab "gitlab.com/gitlab-org/api/client-go"
	"gorm.io/gorm"
)

// StartUserEmailPolling periodically finds DB users without email, fetches their email from GitLab, and updates the DB.
func StartUserEmailPolling(db *gorm.DB, client *gitlab.Client, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for range ticker.C {
			var users []models.User
			if err := db.Where("email = ''").Find(&users).Error; err != nil {
				log.Printf("failed to query users without email: %v", err)
				continue
			}
			for _, u := range users {
				glUser, _, err := client.Users.GetUser(u.GitlabID, gitlab.GetUsersOptions{})
				if err != nil {
					log.Printf("failed to fetch user %d from GitLab: %v", u.GitlabID, err)
					continue
				}
				if glUser.PublicEmail != "" && glUser.PublicEmail != u.Email {
					u.Email = glUser.PublicEmail
				}
				if err := db.Model(&u).Updates(models.User{Email: u.Email, Locked: glUser.Locked, EmailFetched: true}).Error; err != nil {
					log.Printf("failed to update email for user %d: %v", u.GitlabID, err)
				}
			}
		}
	}()
}
