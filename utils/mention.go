package utils

import (
	"devstreamlinebot/models"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
)

func GetUserMention(db *gorm.DB, user *models.User) string {
	if user == nil {
		return ""
	}
	if user.Email != "" {
		return user.Email
	}
	var vkUser models.VKUser
	if err := db.Where("user_id LIKE ?", user.Username+"%").First(&vkUser).Error; err == nil {
		return vkUser.UserID
	}
	return user.Username
}

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

	usernameSet := make(map[string]struct{})
	for _, username := range usernamesToLookup {
		usernameSet[username] = struct{}{}
	}

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

func SanitizeTitle(title string) string {
	title = strings.ReplaceAll(title, "\n", " ")
	title = strings.ReplaceAll(title, "\r", " ")

	return strings.Join(strings.Fields(title), " ")
}

func BuildReviewDigest(db *gorm.DB, mrs []models.MergeRequest) string {
	if len(mrs) == 0 {
		return "No pending reviews found."
	}

	mrIDs := make([]uint, len(mrs))
	for i, mr := range mrs {
		mrIDs[i] = mr.ID
	}

	activeReviewersMap, err := GetActiveReviewers(db, mrIDs)
	if err != nil {
		activeReviewersMap = make(map[uint][]models.User)
	}

	var allUsers []models.User
	for _, mr := range mrs {
		allUsers = append(allUsers, mr.Author)
		allUsers = append(allUsers, activeReviewersMap[mr.ID]...)
	}
	mentionMap := BatchGetUserMentions(db, allUsers)

	var sb strings.Builder
	sb.WriteString("REVIEW DIGEST:")
	for _, mr := range mrs {
		authorMention := mentionMap[mr.Author.ID]

		activeReviewers := activeReviewersMap[mr.ID]
		reviewerMentions := make([]string, 0, len(activeReviewers))
		for _, r := range activeReviewers {
			reviewerMentions = append(reviewerMentions, "@["+mentionMap[r.ID]+"]")
		}
		reviewerStr := strings.Join(reviewerMentions, ", ")
		if reviewerStr == "" {
			reviewerStr = "none"
		}

		sanitizedTitle := SanitizeTitle(mr.Title)
		repoName := mr.Repository.Name
		sb.WriteString(
			fmt.Sprintf("\n- [%s] %s\n  %s\n  author: @[%s] reviewer: %s\n\n", repoName, sanitizedTitle, mr.WebURL, authorMention, reviewerStr),
		)
	}
	return sb.String()
}

func BuildEnhancedReviewDigest(db *gorm.DB, digestMRs []DigestMR) string {
	if len(digestMRs) == 0 {
		return "No pending reviews found."
	}

	mrIDs := make([]uint, len(digestMRs))
	for i, dmr := range digestMRs {
		mrIDs[i] = dmr.MR.ID
	}

	activeReviewersMap, err := GetActiveReviewers(db, mrIDs)
	if err != nil {
		activeReviewersMap = make(map[uint][]models.User)
	}

	var allUsers []models.User
	for _, dmr := range digestMRs {
		allUsers = append(allUsers, dmr.MR.Author)
		allUsers = append(allUsers, activeReviewersMap[dmr.MR.ID]...)
	}
	mentionMap := BatchGetUserMentions(db, allUsers)

	var pendingReview []DigestMR
	var pendingFixes []DigestMR
	var blocked []DigestMR

	for _, dmr := range digestMRs {
		if dmr.Blocked {
			blocked = append(blocked, dmr)
			continue
		}
		switch dmr.State {
		case StateOnReview:
			pendingReview = append(pendingReview, dmr)
		case StateOnFixes, StateDraft:
			pendingFixes = append(pendingFixes, dmr)
		}
	}

	sort.Slice(pendingReview, func(i, j int) bool {
		return pendingReview[i].SLAPercentage > pendingReview[j].SLAPercentage
	})
	sort.Slice(pendingFixes, func(i, j int) bool {
		return pendingFixes[i].SLAPercentage > pendingFixes[j].SLAPercentage
	})
	sort.Slice(blocked, func(i, j int) bool {
		return blocked[i].SLAPercentage > blocked[j].SLAPercentage
	})

	var sb strings.Builder

	if len(pendingReview) > 0 {
		sb.WriteString("PENDING REVIEW:\n")
		for _, dmr := range pendingReview {
			writeDigestEntry(&sb, &dmr, mentionMap, activeReviewersMap[dmr.MR.ID])
		}
	}

	if len(pendingFixes) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("PENDING FIXES:\n")
		for _, dmr := range pendingFixes {
			writeDigestEntry(&sb, &dmr, mentionMap, activeReviewersMap[dmr.MR.ID])
		}
	}

	if len(blocked) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("BLOCKED:\n")
		for _, dmr := range blocked {
			writeDigestEntry(&sb, &dmr, mentionMap, activeReviewersMap[dmr.MR.ID])
		}
	}

	if sb.Len() == 0 {
		return "No pending reviews found."
	}

	return sb.String()
}

func writeDigestEntry(sb *strings.Builder, dmr *DigestMR, mentionMap map[uint]string, activeReviewers []models.User) {
	mr := &dmr.MR
	authorMention := mentionMap[mr.Author.ID]

	reviewerMentions := make([]string, 0, len(activeReviewers))
	for _, r := range activeReviewers {
		reviewerMentions = append(reviewerMentions, "@["+mentionMap[r.ID]+"]")
	}
	reviewerStr := strings.Join(reviewerMentions, ", ")
	if reviewerStr == "" {
		reviewerStr = "none"
	}

	sanitizedTitle := SanitizeTitle(mr.Title)

	timeStr := FormatDuration(dmr.TimeInState)

	slaStatus := formatSLAFromDigest(dmr)

	stateIndicator := ""
	if dmr.State == StateDraft {
		stateIndicator = " [DRAFT]"
	}

	repoName := mr.Repository.Name
	sb.WriteString(fmt.Sprintf("- [%s] %s%s\n", repoName, sanitizedTitle, stateIndicator))
	sb.WriteString(fmt.Sprintf("  %s\n", mr.WebURL))
	sb.WriteString(fmt.Sprintf("  by @[%s] → %s\n", authorMention, reviewerStr))
	sb.WriteString(fmt.Sprintf("  ⏱ %s | SLA: %s\n\n", timeStr, slaStatus))
}

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

	if dmr.Blocked {
		result += " ⏸"
	}
	return result
}

func BuildUserActionsDigest(db *gorm.DB, reviewMRs, fixesMRs, authorOnReviewMRs, releaseMRs []DigestMR, username string) string {
	if len(reviewMRs) == 0 && len(fixesMRs) == 0 && len(authorOnReviewMRs) == 0 && len(releaseMRs) == 0 {
		return fmt.Sprintf("No pending actions for %s.", username)
	}

	var allMRIDs []uint
	for _, dmr := range reviewMRs {
		allMRIDs = append(allMRIDs, dmr.MR.ID)
	}
	for _, dmr := range fixesMRs {
		allMRIDs = append(allMRIDs, dmr.MR.ID)
	}
	for _, dmr := range authorOnReviewMRs {
		allMRIDs = append(allMRIDs, dmr.MR.ID)
	}
	for _, dmr := range releaseMRs {
		allMRIDs = append(allMRIDs, dmr.MR.ID)
	}

	activeReviewersMap, err := GetActiveReviewers(db, allMRIDs)
	if err != nil {
		activeReviewersMap = make(map[uint][]models.User)
	}

	var allUsers []models.User
	for _, dmr := range reviewMRs {
		allUsers = append(allUsers, dmr.MR.Author)
		allUsers = append(allUsers, activeReviewersMap[dmr.MR.ID]...)
	}
	for _, dmr := range fixesMRs {
		allUsers = append(allUsers, dmr.MR.Author)
		allUsers = append(allUsers, activeReviewersMap[dmr.MR.ID]...)
	}
	for _, dmr := range authorOnReviewMRs {
		allUsers = append(allUsers, dmr.MR.Author)
		allUsers = append(allUsers, activeReviewersMap[dmr.MR.ID]...)
	}
	for _, dmr := range releaseMRs {
		allUsers = append(allUsers, dmr.MR.Author)
		allUsers = append(allUsers, activeReviewersMap[dmr.MR.ID]...)
	}
	mentionMap := BatchGetUserMentions(db, allUsers)

	sort.Slice(reviewMRs, func(i, j int) bool {
		return reviewMRs[i].SLAPercentage > reviewMRs[j].SLAPercentage
	})
	sort.Slice(fixesMRs, func(i, j int) bool {
		return fixesMRs[i].SLAPercentage > fixesMRs[j].SLAPercentage
	})
	sort.Slice(authorOnReviewMRs, func(i, j int) bool {
		return authorOnReviewMRs[i].SLAPercentage > authorOnReviewMRs[j].SLAPercentage
	})

	var activeReviewMRs []DigestMR
	var blockedReviewMRs []DigestMR
	for _, dmr := range reviewMRs {
		if dmr.Blocked {
			blockedReviewMRs = append(blockedReviewMRs, dmr)
		} else {
			activeReviewMRs = append(activeReviewMRs, dmr)
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("ACTIONS FOR %s:\n", username))

	if len(activeReviewMRs) > 0 {
		sb.WriteString("\nPENDING REVIEW:\n")
		for _, dmr := range activeReviewMRs {
			writeDigestEntry(&sb, &dmr, mentionMap, activeReviewersMap[dmr.MR.ID])
		}
	}

	if len(fixesMRs) > 0 {
		sb.WriteString("\nPENDING FIXES:\n")
		for _, dmr := range fixesMRs {
			writeDigestEntry(&sb, &dmr, mentionMap, activeReviewersMap[dmr.MR.ID])
		}
	}

	if len(authorOnReviewMRs) > 0 {
		sb.WriteString("\nMY MRS IN REVIEW:\n")
		for _, dmr := range authorOnReviewMRs {
			writeDigestEntry(&sb, &dmr, mentionMap, activeReviewersMap[dmr.MR.ID])
		}
	}

	if len(releaseMRs) > 0 {
		sb.WriteString("\nREADY FOR RELEASE:\n")
		repoMRs := make(map[string][]DigestMR)
		for _, dmr := range releaseMRs {
			repoName := dmr.MR.Repository.Name
			repoMRs[repoName] = append(repoMRs[repoName], dmr)
		}
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

	if len(blockedReviewMRs) > 0 {
		sb.WriteString("\nBLOCKED:\n")
		for _, dmr := range blockedReviewMRs {
			writeDigestEntry(&sb, &dmr, mentionMap, activeReviewersMap[dmr.MR.ID])
		}
	}

	return sb.String()
}

func writeReleaseEntry(sb *strings.Builder, dmr *DigestMR, mentionMap map[uint]string) {
	mr := &dmr.MR
	authorMention := mentionMap[mr.Author.ID]

	sb.WriteString(fmt.Sprintf("- %s\n", SanitizeTitle(mr.Title)))
	sb.WriteString(fmt.Sprintf("  %s\n", mr.WebURL))
	sb.WriteString(fmt.Sprintf("  by @[%s]\n\n", authorMention))
}
