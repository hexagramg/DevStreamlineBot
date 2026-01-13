package utils

import (
	"devstreamlinebot/models"
	"fmt"
	"sort"
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
	if err := db.Where("user_id LIKE ?", user.Username+"%").First(&vkUser).Error; err == nil {
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

// BuildEnhancedReviewDigest builds a digest with two sections:
// - PENDING REVIEW: MRs awaiting reviewer action (StateOnReview)
// - PENDING FIXES: MRs awaiting author fixes (StateOnFixes, StateDraft)
// Each entry shows time in state and SLA status.
func BuildEnhancedReviewDigest(db *gorm.DB, digestMRs []DigestMR) string {
	if len(digestMRs) == 0 {
		return "No pending reviews found."
	}

	var pendingReview []DigestMR
	var pendingFixes []DigestMR

	for _, dmr := range digestMRs {
		switch dmr.State {
		case StateOnReview:
			pendingReview = append(pendingReview, dmr)
		case StateOnFixes, StateDraft:
			pendingFixes = append(pendingFixes, dmr)
		}
	}

	// Sort by SLA percentage descending (most urgent first)
	sort.Slice(pendingReview, func(i, j int) bool {
		return pendingReview[i].SLAPercentage > pendingReview[j].SLAPercentage
	})
	sort.Slice(pendingFixes, func(i, j int) bool {
		return pendingFixes[i].SLAPercentage > pendingFixes[j].SLAPercentage
	})

	var sb strings.Builder

	// Section 1: Pending Review (for reviewers)
	if len(pendingReview) > 0 {
		sb.WriteString("PENDING REVIEW:\n")
		for _, dmr := range pendingReview {
			writeDigestEntry(db, &sb, &dmr)
		}
	}

	// Section 2: Pending Fixes (for developers)
	if len(pendingFixes) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("PENDING FIXES:\n")
		for _, dmr := range pendingFixes {
			writeDigestEntry(db, &sb, &dmr)
		}
	}

	if sb.Len() == 0 {
		return "No pending reviews found."
	}

	return sb.String()
}

// writeDigestEntry writes a single MR entry to the digest.
func writeDigestEntry(db *gorm.DB, sb *strings.Builder, dmr *DigestMR) {
	mr := &dmr.MR
	authorMention := GetUserMention(db, &mr.Author)

	// Build reviewer mentions
	var reviewerMentions []string
	for _, r := range mr.Reviewers {
		reviewerMentions = append(reviewerMentions, "@["+GetUserMention(db, &r)+"]")
	}
	reviewerStr := strings.Join(reviewerMentions, ", ")
	if reviewerStr == "" {
		reviewerStr = "none"
	}

	sanitizedTitle := SanitizeTitle(mr.Title)

	// Format time in state
	timeStr := FormatDuration(dmr.TimeInState)

	// SLA status indicator (use pre-computed values from DigestMR)
	slaStatus := formatSLAFromDigest(dmr)

	// State indicator for fixes section
	stateIndicator := ""
	if dmr.State == StateDraft {
		stateIndicator = " [DRAFT]"
	}

	sb.WriteString(fmt.Sprintf("- %s%s\n", sanitizedTitle, stateIndicator))
	sb.WriteString(fmt.Sprintf("  %s\n", mr.WebURL))
	sb.WriteString(fmt.Sprintf("  by @[%s] → %s\n", authorMention, reviewerStr))
	sb.WriteString(fmt.Sprintf("  ⏱ %s | SLA: %s\n", timeStr, slaStatus))
}

// formatSLAFromDigest formats SLA status from pre-computed DigestMR fields.
func formatSLAFromDigest(dmr *DigestMR) string {
	if dmr.SLAPercentage == 0 {
		return "N/A"
	}
	if dmr.SLAExceeded {
		return fmt.Sprintf("%.0f%% ❌", dmr.SLAPercentage)
	} else if dmr.SLAPercentage >= 80 {
		return fmt.Sprintf("%.0f%% ⚠️", dmr.SLAPercentage)
	}
	return fmt.Sprintf("%.0f%%", dmr.SLAPercentage)
}
