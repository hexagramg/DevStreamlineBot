package utils

import (
	"devstreamlinebot/models"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// GetUserMention returns the best mention string for a user: email, VK user ID, or username.
func GetUserMention(db *gorm.DB, user *models.User) string {
	if user == nil {
		return ""
	}
	if user.Email != "" {
		return user.Email
	}
	// Try to find VKUser by UserID LIKE username
	var vkUser models.VKUser
	if err := db.Where("user_id LIKE ?", user.Username+"% ").First(&vkUser).Error; err == nil {
		return vkUser.UserID
	}
	return user.Username
}

// BuildReviewDigest builds a digest message for a slice of merge requests.
func BuildReviewDigest(db *gorm.DB, mrs []models.MergeRequest) string {
	if len(mrs) == 0 {
		return "No pending reviews found."
	}
	var sb strings.Builder
	sb.WriteString("REVIEW DIGEST:\n")
	for _, mr := range mrs {
		authorMention := GetUserMention(db, &mr.Author)
		reviewerMention := ""
		if len(mr.Reviewers) > 0 {
			reviewerMention = GetUserMention(db, &mr.Reviewers[0])
		}
		sb.WriteString(
			fmt.Sprintf("- %s\n  %s\n  author: @[%s] reviewer: @[%s]\n", mr.Title, mr.WebURL, authorMention, reviewerMention),
		)
	}
	return sb.String()
}
