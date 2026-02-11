package utils

import (
	"devstreamlinebot/models"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// FindRepositoryByIdentifier resolves a repository by GitLab numeric ID,
// path_with_namespace (e.g., "intdev/jobofferapp"), path slug (e.g., "jobofferapp"),
// or name (fallback for backward compatibility).
func FindRepositoryByIdentifier(db *gorm.DB, identifier string) (models.Repository, error) {
	var repo models.Repository

	if gitlabID, err := strconv.Atoi(identifier); err == nil {
		if err := db.Where("gitlab_id = ?", gitlabID).First(&repo).Error; err == nil {
			return repo, nil
		}
	}

	if strings.Contains(identifier, "/") {
		if err := db.Where("path_with_namespace = ?", identifier).First(&repo).Error; err == nil {
			return repo, nil
		}
		return repo, fmt.Errorf("repository not found: %s", identifier)
	}

	var repos []models.Repository
	if err := db.Where("path = ?", identifier).Find(&repos).Error; err == nil && len(repos) == 1 {
		return repos[0], nil
	} else if len(repos) > 1 {
		paths := make([]string, len(repos))
		for i, r := range repos {
			paths[i] = r.PathWithNamespace
		}
		return repo, fmt.Errorf("multiple repositories match '%s', specify full path: %s",
			identifier, strings.Join(paths, ", "))
	}

	if err := db.Where("name = ?", identifier).First(&repo).Error; err == nil {
		return repo, nil
	}

	return repo, fmt.Errorf("repository not found: %s", identifier)
}

func FindDigestMergeRequests(db *gorm.DB, repoIDs []uint) ([]models.MergeRequest, error) {
	var mrs []models.MergeRequest
	err := db.
		Preload("Author").
		Preload("Reviewers").
		Preload("Repository").
		Where("merge_requests.state = ? AND merge_requests.merged_at IS NULL", "opened").
		Where("EXISTS (SELECT 1 FROM merge_request_reviewers mrr WHERE mrr.merge_request_id = merge_requests.id)").
		Where("NOT EXISTS (SELECT 1 FROM merge_request_approvers mra WHERE mra.merge_request_id = merge_requests.id)").
		Where("repository_id IN ?", repoIDs).
		Find(&mrs).Error
	return mrs, err
}

// DigestMR contains MR with derived state information for enhanced digest.
type DigestMR struct {
	MR            models.MergeRequest
	State         MRState
	StateSince    *time.Time
	TimeInState   time.Duration // Working time only
	SLAExceeded   bool
	SLAPercentage float64
	Blocked       bool // Whether MR currently has a block label
}

func IsMRFullyApproved(mr *models.MergeRequest) bool {
	if len(mr.Reviewers) == 0 {
		return false
	}
	approverIDs := make(map[uint]bool)
	for _, a := range mr.Approvers {
		approverIDs[a.ID] = true
	}
	for _, r := range mr.Reviewers {
		if !approverIDs[r.ID] {
			return false
		}
	}
	return true
}

func IsMRBlocked(db *gorm.DB, mr *models.MergeRequest) bool {
	if len(mr.Labels) == 0 {
		return false
	}

	labelNames := make([]string, len(mr.Labels))
	for i, l := range mr.Labels {
		labelNames[i] = l.Name
	}

	var count int64
	db.Model(&models.BlockLabel{}).
		Where("repository_id = ? AND label_name IN ?", mr.RepositoryID, labelNames).
		Count(&count)

	return count > 0
}

func HasReleaseLabel(db *gorm.DB, mr *models.MergeRequest) bool {
	if len(mr.Labels) == 0 {
		return false
	}

	labelNames := make([]string, len(mr.Labels))
	for i, l := range mr.Labels {
		labelNames[i] = l.Name
	}

	var count int64
	db.Model(&models.ReleaseLabel{}).
		Where("repository_id = ? AND label_name IN ?", mr.RepositoryID, labelNames).
		Count(&count)

	if count > 0 {
		return true
	}

	db.Model(&models.FeatureReleaseLabel{}).
		Where("repository_id = ? AND label_name IN ?", mr.RepositoryID, labelNames).
		Count(&count)

	return count > 0
}

func FindDigestMergeRequestsWithState(db *gorm.DB, repoIDs []uint) ([]DigestMR, error) {
	var mrs []models.MergeRequest
	err := db.
		Preload("Author").
		Preload("Reviewers").
		Preload("Repository").
		Preload("Labels").
		Where("merge_requests.state = ? AND merge_requests.merged_at IS NULL", "opened").
		Where("EXISTS (SELECT 1 FROM merge_request_reviewers mrr WHERE mrr.merge_request_id = merge_requests.id)").
		Where("repository_id IN ?", repoIDs).
		Find(&mrs).Error
	if err != nil {
		return nil, err
	}

	if len(mrs) == 0 {
		return nil, nil
	}

	// Load cache for all MRs
	mrIDs, collectedRepoIDs := CollectUniqueIDs(mrs)
	cache, err := LoadMRDataCache(db, mrIDs, collectedRepoIDs)
	if err != nil {
		return nil, err
	}

	var digestMRs []DigestMR
	for _, mr := range mrs {
		if cache.HasReleaseLabelFromCache(mr.Labels, mr.RepositoryID) {
			continue
		}

		stateInfo := GetStateInfoFromCache(&mr, cache)

		blocked := cache.IsMRBlockedFromCache(mr.Labels, mr.RepositoryID)

		sla := cache.GetSLAFromCache(mr.RepositoryID)

		var threshold time.Duration
		if stateInfo.State == StateOnReview {
			threshold = sla.ReviewDuration.ToDuration()
		} else if stateInfo.State == StateOnFixes || stateInfo.State == StateDraft {
			threshold = sla.FixesDuration.ToDuration()
		}

		exceeded, percentage := CheckSLAStatus(stateInfo.WorkingTime, threshold)

		digestMRs = append(digestMRs, DigestMR{
			MR:            mr,
			State:         stateInfo.State,
			StateSince:    stateInfo.StateSince,
			TimeInState:   stateInfo.WorkingTime,
			SLAExceeded:   exceeded,
			SLAPercentage: percentage,
			Blocked:       blocked,
		})
	}

	return digestMRs, nil
}


// FindUserActionMRs returns MRs requiring action from a specific user.
// Returns three slices:
// - reviewMRs: MRs where user is reviewer and needs to take action (no pending threads awaiting author)
// - fixesMRs: MRs where user is author and state is on_fixes or draft
// - authorOnReviewMRs: MRs where user is author and state is on_review (waiting for reviewers)
//
// Reviewer needs action when ALL their threads are "handled" - meaning they have NO
// unresolved threads where they commented last (waiting for author to respond).
//
// SLA times are calculated per-user using GetUserStateTransitionTime, not global MR state.
func FindUserActionMRs(db *gorm.DB, userID uint) (reviewMRs []DigestMR, fixesMRs []DigestMR, authorOnReviewMRs []DigestMR, err error) {
	var reviewerMRs []models.MergeRequest
	err = db.
		Preload("Author").
		Preload("Reviewers").
		Preload("Repository").
		Preload("Labels").
		Where("merge_requests.state = ? AND merge_requests.merged_at IS NULL", "opened").
		Where("EXISTS (SELECT 1 FROM merge_request_reviewers mrr WHERE mrr.merge_request_id = merge_requests.id AND mrr.user_id = ?)", userID).
		Where("NOT EXISTS (SELECT 1 FROM merge_request_approvers mra WHERE mra.merge_request_id = merge_requests.id AND mra.user_id = ?)", userID).
		Where(`NOT EXISTS (
			SELECT 1 FROM mr_comments mc
			WHERE mc.merge_request_id = merge_requests.id
			  AND mc.is_last_in_thread = ?
			  AND mc.author_id != merge_requests.author_id
			  AND EXISTS (SELECT 1 FROM mr_comments starter WHERE starter.gitlab_discussion_id = mc.gitlab_discussion_id AND starter.resolvable = ? AND starter.resolved = ?)
			  AND EXISTS (
				SELECT 1 FROM mr_comments rc
				WHERE rc.gitlab_discussion_id = mc.gitlab_discussion_id
				  AND rc.author_id = ?
				  AND rc.gitlab_created_at > COALESCE(
					(SELECT MAX(ac.gitlab_created_at) FROM mr_comments ac
					 WHERE ac.gitlab_discussion_id = mc.gitlab_discussion_id
					   AND ac.author_id = merge_requests.author_id),
					'0001-01-01')
			  )
		)`, true, true, false, userID).
		Find(&reviewerMRs).Error
	if err != nil {
		return nil, nil, nil, err
	}

	var authorMRs []models.MergeRequest
	err = db.
		Preload("Author").
		Preload("Reviewers").
		Preload("Repository").
		Preload("Labels").
		Where("merge_requests.state = ? AND merge_requests.merged_at IS NULL", "opened").
		Where("author_id = ?", userID).
		Where("EXISTS (SELECT 1 FROM merge_request_reviewers mrr WHERE mrr.merge_request_id = merge_requests.id)").
		Find(&authorMRs).Error
	if err != nil {
		return nil, nil, nil, err
	}

	// Collect all MRs and load cache once
	allMRs := append([]models.MergeRequest{}, reviewerMRs...)
	allMRs = append(allMRs, authorMRs...)

	if len(allMRs) == 0 {
		return nil, nil, nil, nil
	}

	mrIDs, repoIDs := CollectUniqueIDs(allMRs)
	cache, err := LoadMRDataCache(db, mrIDs, repoIDs)
	if err != nil {
		return nil, nil, nil, err
	}

	for _, mr := range reviewerMRs {
		if cache.HasReleaseLabelFromCache(mr.Labels, mr.RepositoryID) {
			continue
		}

		stateInfo := GetStateInfoFromCache(&mr, cache)
		userStateSince := GetUserStateTransitionTimeFromCache(&mr, userID, cache)
		workingTime := calculateUserWorkingTimeFromCache(&mr, userStateSince, cache)

		blocked := cache.IsMRBlockedFromCache(mr.Labels, mr.RepositoryID)
		sla := cache.GetSLAFromCache(mr.RepositoryID)
		threshold := sla.ReviewDuration.ToDuration()
		exceeded, percentage := CheckSLAStatus(workingTime, threshold)

		reviewMRs = append(reviewMRs, DigestMR{
			MR:            mr,
			State:         stateInfo.State,
			StateSince:    userStateSince,
			TimeInState:   workingTime,
			SLAExceeded:   exceeded,
			SLAPercentage: percentage,
			Blocked:       blocked,
		})
	}

	for _, mr := range authorMRs {
		if cache.HasReleaseLabelFromCache(mr.Labels, mr.RepositoryID) {
			continue
		}

		stateInfo := GetStateInfoFromCache(&mr, cache)
		userStateSince := GetUserStateTransitionTimeFromCache(&mr, userID, cache)
		workingTime := calculateUserWorkingTimeFromCache(&mr, userStateSince, cache)

		blocked := cache.IsMRBlockedFromCache(mr.Labels, mr.RepositoryID)
		sla := cache.GetSLAFromCache(mr.RepositoryID)

		if stateInfo.State == StateOnFixes || stateInfo.State == StateDraft {
			threshold := sla.FixesDuration.ToDuration()
			exceeded, percentage := CheckSLAStatus(workingTime, threshold)

			fixesMRs = append(fixesMRs, DigestMR{
				MR:            mr,
				State:         stateInfo.State,
				StateSince:    userStateSince,
				TimeInState:   workingTime,
				SLAExceeded:   exceeded,
				SLAPercentage: percentage,
				Blocked:       blocked,
			})
		} else if stateInfo.State == StateOnReview {
			threshold := sla.ReviewDuration.ToDuration()
			exceeded, percentage := CheckSLAStatus(workingTime, threshold)

			authorOnReviewMRs = append(authorOnReviewMRs, DigestMR{
				MR:            mr,
				State:         stateInfo.State,
				StateSince:    userStateSince,
				TimeInState:   workingTime,
				SLAExceeded:   exceeded,
				SLAPercentage: percentage,
				Blocked:       blocked,
			})
		}
	}

	return reviewMRs, fixesMRs, authorOnReviewMRs, nil
}

// calculateUserWorkingTime calculates working time from stateSince to now,
// excluding weekends, holidays, and blocked time.
func calculateUserWorkingTime(db *gorm.DB, mr *models.MergeRequest, stateSince *time.Time) time.Duration {
	if stateSince == nil {
		return 0
	}

	now := time.Now()
	workingTime := CalculateWorkingTime(db, mr.RepositoryID, *stateSince, now)

	blockedTime := CalculateBlockedTime(db, mr.ID, mr.RepositoryID, *stateSince, now)
	workingTime -= blockedTime
	if workingTime < 0 {
		workingTime = 0
	}

	return workingTime
}

// calculateUserWorkingTimeFromCache calculates working time using cached data.
func calculateUserWorkingTimeFromCache(mr *models.MergeRequest, stateSince *time.Time, cache *MRDataCache) time.Duration {
	if stateSince == nil {
		return 0
	}

	now := time.Now()
	workingTime := CalculateWorkingTimeFromCache(mr.RepositoryID, *stateSince, now, cache.Holidays[mr.RepositoryID])

	blockedTime := CalculateBlockedTimeFromCache(mr.ID, mr.RepositoryID, *stateSince, now, cache.Actions[mr.ID], cache.Holidays[mr.RepositoryID])
	workingTime -= blockedTime
	if workingTime < 0 {
		workingTime = 0
	}

	return workingTime
}

// GetActiveReviewers returns active reviewers for each MR.
// A reviewer is active if they: are assigned, haven't approved, and have no unresolved threads
// where they participated after the MR author's last reply (waiting for author to respond).
func GetActiveReviewers(db *gorm.DB, mrIDs []uint) (map[uint][]models.User, error) {
	result := make(map[uint][]models.User)
	if len(mrIDs) == 0 {
		return result, nil
	}

	type ReviewerRow struct {
		MergeRequestID uint `gorm:"column:merge_request_id"`
		UserID         uint `gorm:"column:user_id"`
	}

	var reviewerRows []ReviewerRow
	if err := db.Table("merge_request_reviewers").
		Where("merge_request_id IN ?", mrIDs).
		Scan(&reviewerRows).Error; err != nil {
		return nil, err
	}

	var approverRows []ReviewerRow
	if err := db.Table("merge_request_approvers").
		Where("merge_request_id IN ?", mrIDs).
		Scan(&approverRows).Error; err != nil {
		return nil, err
	}

	approverSet := make(map[uint]map[uint]bool)
	for _, row := range approverRows {
		if approverSet[row.MergeRequestID] == nil {
			approverSet[row.MergeRequestID] = make(map[uint]bool)
		}
		approverSet[row.MergeRequestID][row.UserID] = true
	}

	type WaitingReviewerRow struct {
		MergeRequestID    uint `gorm:"column:merge_request_id"`
		WaitingReviewerID uint `gorm:"column:waiting_reviewer_id"`
	}
	var waitingRows []WaitingReviewerRow
	if err := db.Raw(`
		SELECT DISTINCT rc.merge_request_id, rc.author_id as waiting_reviewer_id
		FROM mr_comments rc
		JOIN merge_requests mr ON mr.id = rc.merge_request_id
		WHERE rc.merge_request_id IN ?
		  AND rc.author_id != mr.author_id
		  AND EXISTS (
			SELECT 1 FROM mr_comments starter
			WHERE starter.gitlab_discussion_id = rc.gitlab_discussion_id
			  AND starter.resolvable = ? AND starter.resolved = ?
		  )
		  AND EXISTS (
			SELECT 1 FROM mr_comments last_c
			WHERE last_c.gitlab_discussion_id = rc.gitlab_discussion_id
			  AND last_c.is_last_in_thread = ?
			  AND last_c.author_id != mr.author_id
		  )
		  AND rc.gitlab_created_at > COALESCE(
			(SELECT MAX(ac.gitlab_created_at) FROM mr_comments ac
			 WHERE ac.gitlab_discussion_id = rc.gitlab_discussion_id
			   AND ac.author_id = mr.author_id),
			'0001-01-01'
		  )
	`, mrIDs, true, false, true).Scan(&waitingRows).Error; err != nil {
		return nil, err
	}

	hasThreadAwaitingAuthor := make(map[uint]map[uint]bool)
	for _, row := range waitingRows {
		if hasThreadAwaitingAuthor[row.MergeRequestID] == nil {
			hasThreadAwaitingAuthor[row.MergeRequestID] = make(map[uint]bool)
		}
		hasThreadAwaitingAuthor[row.MergeRequestID][row.WaitingReviewerID] = true
	}

	activeReviewerIDs := make(map[uint][]uint)
	allUserIDs := make(map[uint]bool)
	for _, row := range reviewerRows {
		if approverSet[row.MergeRequestID][row.UserID] {
			continue
		}
		if hasThreadAwaitingAuthor[row.MergeRequestID][row.UserID] {
			continue
		}
		activeReviewerIDs[row.MergeRequestID] = append(activeReviewerIDs[row.MergeRequestID], row.UserID)
		allUserIDs[row.UserID] = true
	}

	if len(allUserIDs) == 0 {
		return result, nil
	}

	userIDList := make([]uint, 0, len(allUserIDs))
	for id := range allUserIDs {
		userIDList = append(userIDList, id)
	}

	var users []models.User
	if err := db.Where("id IN ?", userIDList).Find(&users).Error; err != nil {
		return nil, err
	}

	userMap := make(map[uint]models.User)
	for _, u := range users {
		userMap[u.ID] = u
	}

	for mrID, reviewerIDs := range activeReviewerIDs {
		for _, reviewerID := range reviewerIDs {
			if user, ok := userMap[reviewerID]; ok {
				result[mrID] = append(result[mrID], user)
			}
		}
	}

	return result, nil
}

// FindReleaseManagerActionMRs returns MRs that are fully approved and ready for release
// for repositories where the user is a release manager.
func FindReleaseManagerActionMRs(db *gorm.DB, userID uint) ([]DigestMR, error) {
	var releaseManagerLinks []models.ReleaseManager
	if err := db.Where("user_id = ?", userID).Find(&releaseManagerLinks).Error; err != nil {
		return nil, err
	}

	if len(releaseManagerLinks) == 0 {
		return nil, nil
	}

	repoIDs := make([]uint, len(releaseManagerLinks))
	for i, rm := range releaseManagerLinks {
		repoIDs[i] = rm.RepositoryID
	}

	var mrs []models.MergeRequest
	err := db.
		Preload("Author").
		Preload("Reviewers").
		Preload("Approvers").
		Preload("Repository").
		Preload("Labels").
		Where("merge_requests.state = ? AND merge_requests.merged_at IS NULL AND merge_requests.draft = ?", "opened", false).
		Where("EXISTS (SELECT 1 FROM merge_request_reviewers mrr WHERE mrr.merge_request_id = merge_requests.id)").
		Where("repository_id IN ?", repoIDs).
		Find(&mrs).Error
	if err != nil {
		return nil, err
	}

	if len(mrs) == 0 {
		return nil, nil
	}

	// Load cache for all MRs
	mrIDs, collectedRepoIDs := CollectUniqueIDs(mrs)
	cache, err := LoadMRDataCache(db, mrIDs, collectedRepoIDs)
	if err != nil {
		return nil, err
	}

	var releaseMRs []DigestMR
	for _, mr := range mrs {
		if cache.HasReleaseLabelFromCache(mr.Labels, mr.RepositoryID) {
			continue
		}

		if !IsMRFullyApproved(&mr) {
			continue
		}

		stateInfo := GetStateInfoFromCache(&mr, cache)
		blocked := cache.IsMRBlockedFromCache(mr.Labels, mr.RepositoryID)

		releaseMRs = append(releaseMRs, DigestMR{
			MR:          mr,
			State:       stateInfo.State,
			StateSince:  stateInfo.StateSince,
			TimeInState: stateInfo.WorkingTime,
			Blocked:     blocked,
		})
	}

	return releaseMRs, nil
}
