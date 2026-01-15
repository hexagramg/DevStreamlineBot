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

// BatchGetUserMentions returns mention strings for multiple users with a single DB query.
// Returns a map from user ID to mention string.
func BatchGetUserMentions(db *gorm.DB, users []models.User) map[uint]string {
	result := make(map[uint]string)
	if len(users) == 0 {
		return result
	}

	var usernamesToLookup []string
	usernameToUserID := make(map[string]uint)

	for _, u := range users {
		if u.Email != "" {
			result[u.ID] = u.Email
		} else {
			usernamesToLookup = append(usernamesToLookup, u.Username)
			usernameToUserID[u.Username] = u.ID
		}
	}

	if len(usernamesToLookup) == 0 {
		return result
	}

	// Batch query VK users
	var vkUsers []models.VKUser
	query := db
	for i, username := range usernamesToLookup {
		if i == 0 {
			query = query.Where("user_id LIKE ?", username+"%")
		} else {
			query = query.Or("user_id LIKE ?", username+"%")
		}
	}
	query.Find(&vkUsers)

	// Build set of usernames for O(1) lookup
	usernameSet := make(map[string]struct{})
	for _, username := range usernamesToLookup {
		usernameSet[username] = struct{}{}
	}

	// Map VK users by extracting username prefix
	vkMap := make(map[string]string)
	for _, vk := range vkUsers {
		username := vk.UserID
		if idx := strings.Index(vk.UserID, "@"); idx > 0 {
			username = vk.UserID[:idx]
		}
		if _, ok := usernameSet[username]; ok {
			vkMap[username] = vk.UserID
		}
	}

	// Fill results for users without email
	for _, u := range users {
		if u.Email == "" {
			if vkID, ok := vkMap[u.Username]; ok {
				result[u.ID] = vkID
			} else {
				result[u.ID] = u.Username
			}
		}
	}

	return result
}

// SanitizeTitle removes newlines and other problematic characters from a title
func SanitizeTitle(title string) string {
	// Replace newlines with spaces
	title = strings.ReplaceAll(title, "\n", " ")
	title = strings.ReplaceAll(title, "\r", " ")

	// Remove consecutive spaces using O(n) approach
	return strings.Join(strings.Fields(title), " ")
}

// BuildReviewDigest builds a digest message for a slice of merge requests.
func BuildReviewDigest(db *gorm.DB, mrs []models.MergeRequest) string {
	if len(mrs) == 0 {
		return "No pending reviews found."
	}

	// Collect all users for batch mention lookup
	var allUsers []models.User
	for _, mr := range mrs {
		allUsers = append(allUsers, mr.Author)
		if len(mr.Reviewers) > 0 {
			allUsers = append(allUsers, mr.Reviewers[0])
		}
	}
	mentionMap := BatchGetUserMentions(db, allUsers)

	var sb strings.Builder
	sb.WriteString("REVIEW DIGEST:")
	for _, mr := range mrs {
		authorMention := mentionMap[mr.Author.ID]
		reviewerMention := ""
		if len(mr.Reviewers) > 0 {
			reviewerMention = mentionMap[mr.Reviewers[0].ID]
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

	// Collect all users for batch mention lookup
	var allUsers []models.User
	for _, dmr := range digestMRs {
		allUsers = append(allUsers, dmr.MR.Author)
		allUsers = append(allUsers, dmr.MR.Reviewers...)
	}
	mentionMap := BatchGetUserMentions(db, allUsers)

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
			writeDigestEntry(&sb, &dmr, mentionMap)
		}
	}

	// Section 2: Pending Fixes (for developers)
	if len(pendingFixes) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("PENDING FIXES:\n")
		for _, dmr := range pendingFixes {
			writeDigestEntry(&sb, &dmr, mentionMap)
		}
	}

	if sb.Len() == 0 {
		return "No pending reviews found."
	}

	return sb.String()
}

// writeDigestEntry writes a single MR entry to the digest.
func writeDigestEntry(sb *strings.Builder, dmr *DigestMR, mentionMap map[uint]string) {
	mr := &dmr.MR
	authorMention := mentionMap[mr.Author.ID]

	// Build reviewer mentions
	reviewerMentions := make([]string, 0, len(mr.Reviewers))
	for _, r := range mr.Reviewers {
		reviewerMentions = append(reviewerMentions, "@["+mentionMap[r.ID]+"]")
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
// Appends ⏸ icon if MR is currently blocked.
func formatSLAFromDigest(dmr *DigestMR) string {
	var result string
	if dmr.SLAPercentage == 0 {
		result = "N/A"
	} else if dmr.SLAExceeded {
		result = fmt.Sprintf("%.0f%% ❌", dmr.SLAPercentage)
	} else if dmr.SLAPercentage >= 80 {
		result = fmt.Sprintf("%.0f%% ⚠️", dmr.SLAPercentage)
	} else {
		result = fmt.Sprintf("%.0f%%", dmr.SLAPercentage)
	}

	// Append pause icon if currently blocked
	if dmr.Blocked {
		result += " ⏸"
	}
	return result
}

// BuildUserActionsDigest builds a digest of actions required from a specific user.
// Shows sections:
// - PENDING REVIEW: MRs where user needs to review
// - PENDING FIXES: MRs where user (as author) needs to address comments
// - READY FOR RELEASE: MRs where user is release manager and all reviewers approved (grouped by repo)
func BuildUserActionsDigest(db *gorm.DB, reviewMRs, fixesMRs, releaseMRs []DigestMR, username string) string {
	if len(reviewMRs) == 0 && len(fixesMRs) == 0 && len(releaseMRs) == 0 {
		return fmt.Sprintf("No pending actions for %s.", username)
	}

	// Collect all users for batch mention lookup
	var allUsers []models.User
	for _, dmr := range reviewMRs {
		allUsers = append(allUsers, dmr.MR.Author)
		allUsers = append(allUsers, dmr.MR.Reviewers...)
	}
	for _, dmr := range fixesMRs {
		allUsers = append(allUsers, dmr.MR.Author)
		allUsers = append(allUsers, dmr.MR.Reviewers...)
	}
	for _, dmr := range releaseMRs {
		allUsers = append(allUsers, dmr.MR.Author)
		allUsers = append(allUsers, dmr.MR.Reviewers...)
	}
	mentionMap := BatchGetUserMentions(db, allUsers)

	// Sort by SLA percentage descending (most urgent first)
	sort.Slice(reviewMRs, func(i, j int) bool {
		return reviewMRs[i].SLAPercentage > reviewMRs[j].SLAPercentage
	})
	sort.Slice(fixesMRs, func(i, j int) bool {
		return fixesMRs[i].SLAPercentage > fixesMRs[j].SLAPercentage
	})

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("ACTIONS FOR %s:\n", username))

	// Section 1: Pending Review (user needs to review these)
	if len(reviewMRs) > 0 {
		sb.WriteString("\nPENDING REVIEW:\n")
		for _, dmr := range reviewMRs {
			writeDigestEntry(&sb, &dmr, mentionMap)
		}
	}

	// Section 2: Pending Fixes (user needs to fix these as author)
	if len(fixesMRs) > 0 {
		sb.WriteString("\nPENDING FIXES:\n")
		for _, dmr := range fixesMRs {
			writeDigestEntry(&sb, &dmr, mentionMap)
		}
	}

	// Section 3: Ready for Release (user is release manager, grouped by repo)
	if len(releaseMRs) > 0 {
		sb.WriteString("\nREADY FOR RELEASE:\n")
		// Group by repository
		repoMRs := make(map[string][]DigestMR)
		for _, dmr := range releaseMRs {
			repoName := dmr.MR.Repository.Name
			repoMRs[repoName] = append(repoMRs[repoName], dmr)
		}
		// Sort repo names for consistent output
		var repoNames []string
		for name := range repoMRs {
			repoNames = append(repoNames, name)
		}
		sort.Strings(repoNames)
		for _, repoName := range repoNames {
			sb.WriteString(fmt.Sprintf("\n%s:\n", repoName))
			for _, dmr := range repoMRs[repoName] {
				writeReleaseEntry(&sb, &dmr, mentionMap)
			}
		}
	}

	return sb.String()
}

// writeReleaseEntry writes a single release MR entry (simplified format).
func writeReleaseEntry(sb *strings.Builder, dmr *DigestMR, mentionMap map[uint]string) {
	mr := &dmr.MR
	authorMention := mentionMap[mr.Author.ID]

	sb.WriteString(fmt.Sprintf("- %s\n", SanitizeTitle(mr.Title)))
	sb.WriteString(fmt.Sprintf("  %s\n", mr.WebURL))
	sb.WriteString(fmt.Sprintf("  by @[%s]\n", authorMention))
}
