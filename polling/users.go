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

		// Track the last API call time
		lastAPICall := time.Now().Add(-10 * time.Second) // Initialize to allow immediate first call

		for range ticker.C {
			// First process users with email_fetched = false
			var users []models.User
			if err := db.Where("email = '' AND email_fetched = false").Find(&users).Error; err != nil {
				log.Printf("failed to query users without email: %v", err)
				continue
			}
			for _, u := range users {
				// Throttle API calls to 1 every 10 seconds
				elapsed := time.Since(lastAPICall)
				if elapsed < 10*time.Second {
					// Wait for the remaining time to complete 10 seconds
					waitTime := 10*time.Second - elapsed
					time.Sleep(waitTime)
				}

				glUser, _, err := client.Users.GetUser(u.GitlabID, gitlab.GetUsersOptions{})
				// Update last API call timestamp after each call
				lastAPICall = time.Now()

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

			// Now process users with email_fetched = true but still have empty emails
			// Try to match them with VK users - exclude users with empty usernames directly in the query
			var emptyEmailUsers []models.User
			if err := db.Where("email = '' AND email_fetched = true AND username <> ''").Find(&emptyEmailUsers).Error; err != nil {
				log.Printf("failed to query users with empty emails (email_fetched = true): %v", err)
				continue
			}

			for _, user := range emptyEmailUsers {
				// Only find VK users where UserID follows the pattern "username@..." ordered by ID in descending order
				var vkUsers []models.VKUser
				if err := db.Where("user_id LIKE ?", user.Username+"@%").Order("id DESC").Find(&vkUsers).Error; err != nil {
					log.Printf("failed to query VK users for GitLab user %s: %v", user.Username, err)
					continue
				}

				// If we find matching VK users
				if len(vkUsers) > 0 {
					// Use the first match (they all match the username@ pattern and now the first one has the highest ID)
					vkUser := vkUsers[0]

					// Update GitLab user with the VK user ID as email
					if err := db.Model(&user).Update("email", vkUser.UserID).Error; err != nil {
						log.Printf("failed to update email for user %s with VK user ID: %v", user.Username, err)
					} else {
						log.Printf("updated user %s email with VK user ID %s", user.Username, vkUser.UserID)
					}
				}
			}
		}
	}()
}
