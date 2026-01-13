package utils

import (
	"devstreamlinebot/models"
	"time"

	"gorm.io/gorm"
)

// FindDigestMergeRequests returns open MRs with reviewers but no approvers for the given repositories.
func FindDigestMergeRequests(db *gorm.DB, repoIDs []uint) ([]models.MergeRequest, error) {
	var mrs []models.MergeRequest
	err := db.
		Preload("Author").
		Preload("Reviewers").
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

// IsMRBlocked checks if an MR has any block labels configured for its repository.
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

// FindDigestMergeRequestsWithState returns open MRs with state information for enhanced digest.
// Includes MRs that are:
// - On review (has reviewers, no unresolved comments, not draft)
// - On fixes (has unresolved comments)
// - Draft (marked as draft/WIP)
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

	var digestMRs []DigestMR
	for _, mr := range mrs {
		stateInfo := GetStateInfo(db, &mr)

		// Check if MR is currently blocked
		blocked := IsMRBlocked(db, &mr)

		// Get SLA thresholds
		sla, _ := GetRepositorySLA(db, mr.RepositoryID)
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
// Returns two slices:
// - reviewMRs: MRs where user is reviewer and state is on_review
// - fixesMRs: MRs where user is author and state is on_fixes or draft
func FindUserActionMRs(db *gorm.DB, userID uint) (reviewMRs []DigestMR, fixesMRs []DigestMR, err error) {
	// Find MRs where user is a reviewer (not yet approved)
	var reviewerMRs []models.MergeRequest
	err = db.
		Preload("Author").
		Preload("Reviewers").
		Preload("Repository").
		Preload("Labels").
		Where("merge_requests.state = ? AND merge_requests.merged_at IS NULL", "opened").
		Where("EXISTS (SELECT 1 FROM merge_request_reviewers mrr WHERE mrr.merge_request_id = merge_requests.id AND mrr.user_id = ?)", userID).
		Where("NOT EXISTS (SELECT 1 FROM merge_request_approvers mra WHERE mra.merge_request_id = merge_requests.id AND mra.user_id = ?)", userID).
		Find(&reviewerMRs).Error
	if err != nil {
		return nil, nil, err
	}

	// Find MRs where user is author
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
		return nil, nil, err
	}

	// Process reviewer MRs - only include those on_review state
	for _, mr := range reviewerMRs {
		stateInfo := GetStateInfo(db, &mr)
		if stateInfo.State != StateOnReview {
			continue
		}

		blocked := IsMRBlocked(db, &mr)
		sla, _ := GetRepositorySLA(db, mr.RepositoryID)
		threshold := sla.ReviewDuration.ToDuration()
		exceeded, percentage := CheckSLAStatus(stateInfo.WorkingTime, threshold)

		reviewMRs = append(reviewMRs, DigestMR{
			MR:            mr,
			State:         stateInfo.State,
			StateSince:    stateInfo.StateSince,
			TimeInState:   stateInfo.WorkingTime,
			SLAExceeded:   exceeded,
			SLAPercentage: percentage,
			Blocked:       blocked,
		})
	}

	// Process author MRs - only include those on_fixes or draft state
	for _, mr := range authorMRs {
		stateInfo := GetStateInfo(db, &mr)
		if stateInfo.State != StateOnFixes && stateInfo.State != StateDraft {
			continue
		}

		blocked := IsMRBlocked(db, &mr)
		sla, _ := GetRepositorySLA(db, mr.RepositoryID)
		threshold := sla.FixesDuration.ToDuration()
		exceeded, percentage := CheckSLAStatus(stateInfo.WorkingTime, threshold)

		fixesMRs = append(fixesMRs, DigestMR{
			MR:            mr,
			State:         stateInfo.State,
			StateSince:    stateInfo.StateSince,
			TimeInState:   stateInfo.WorkingTime,
			SLAExceeded:   exceeded,
			SLAPercentage: percentage,
			Blocked:       blocked,
		})
	}

	return reviewMRs, fixesMRs, nil
}
