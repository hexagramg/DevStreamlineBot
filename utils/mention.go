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

// SanitizeTitle removes newlines and other problematic characters from a title
func SanitizeTitle(title string) string {
	// Replace newlines with spaces
	title = strings.ReplaceAll(title, "\n", " ")
	title = strings.ReplaceAll(title, "\r", " ")

	// Remove any consecutive spaces
	for strings.Contains(title, "  ") {
		title = strings.ReplaceAll(title, "  ", " ")
	}

	return strings.TrimSpace(title)
}

// BuildReviewDigest builds a digest message for a slice of merge requests.
func BuildReviewDigest(db *gorm.DB, mrs []models.MergeRequest) string {
	if len(mrs) == 0 {
		return "No pending reviews found."
	}
	var sb strings.Builder
	sb.WriteString("REVIEW DIGEST:")
	for _, mr := range mrs {
		authorMention := GetUserMention(db, &mr.Author)
		reviewerMention := ""
		if len(mr.Reviewers) > 0 {
			reviewerMention = GetUserMention(db, &mr.Reviewers[0])
		}
		sanitizedTitle := SanitizeTitle(mr.Title)
		sb.WriteString(
			fmt.Sprintf("\n- %s\n  %s\n  author: @[%s] reviewer: @[%s]\n", sanitizedTitle, mr.WebURL, authorMention, reviewerMention),
		)
	}
	return sb.String()
}
