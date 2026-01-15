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

// IsMRFullyApproved returns true if all assigned reviewers have approved the MR.
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

// HasReleaseLabel checks if an MR has any release labels configured for its repository.
// MRs with release labels should be completely ignored (no reviewer assignment, not in digests).
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

	if len(mrs) == 0 {
		return nil, nil
	}

	// Batch fetch SLAs for all unique repos
	slaMap := make(map[uint]*models.RepositorySLA)
	var slas []models.RepositorySLA
	db.Where("repository_id IN ?", repoIDs).Find(&slas)
	for i := range slas {
		slaMap[slas[i].RepositoryID] = &slas[i]
	}

	// Batch fetch block labels for all repos
	var blockLabels []models.BlockLabel
	db.Where("repository_id IN ?", repoIDs).Find(&blockLabels)
	blockLabelMap := make(map[uint]map[string]struct{})
	for _, bl := range blockLabels {
		if blockLabelMap[bl.RepositoryID] == nil {
			blockLabelMap[bl.RepositoryID] = make(map[string]struct{})
		}
		blockLabelMap[bl.RepositoryID][bl.LabelName] = struct{}{}
	}

	// Batch fetch release labels for all repos
	var releaseLabels []models.ReleaseLabel
	db.Where("repository_id IN ?", repoIDs).Find(&releaseLabels)
	releaseLabelMap := make(map[uint]map[string]struct{})
	for _, rl := range releaseLabels {
		if releaseLabelMap[rl.RepositoryID] == nil {
			releaseLabelMap[rl.RepositoryID] = make(map[string]struct{})
		}
		releaseLabelMap[rl.RepositoryID][rl.LabelName] = struct{}{}
	}

	var digestMRs []DigestMR
	for _, mr := range mrs {
		// Skip MRs with release labels (completely ignored)
		if hasReleaseLabelFromCache(mr.Labels, releaseLabelMap[mr.RepositoryID]) {
			continue
		}

		stateInfo := GetStateInfo(db, &mr)

		// Check if MR is blocked using pre-fetched block labels
		blocked := isMRBlockedFromCache(mr.Labels, blockLabelMap[mr.RepositoryID])

		// Get SLA from pre-fetched map, or use defaults
		sla := slaMap[mr.RepositoryID]
		if sla == nil {
			sla = &models.RepositorySLA{
				RepositoryID:   mr.RepositoryID,
				ReviewDuration: DefaultSLADuration,
				FixesDuration:  DefaultSLADuration,
				AssignCount:    1,
			}
		}

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

// isMRBlockedFromCache checks if MR has block labels using pre-fetched cache.
func isMRBlockedFromCache(labels []models.Label, blockLabels map[string]struct{}) bool {
	if len(labels) == 0 || len(blockLabels) == 0 {
		return false
	}
	for _, l := range labels {
		if _, ok := blockLabels[l.Name]; ok {
			return true
		}
	}
	return false
}

// hasReleaseLabelFromCache checks if MR has release labels using pre-fetched cache.
func hasReleaseLabelFromCache(labels []models.Label, releaseLabels map[string]struct{}) bool {
	if len(labels) == 0 || len(releaseLabels) == 0 {
		return false
	}
	for _, l := range labels {
		if _, ok := releaseLabels[l.Name]; ok {
			return true
		}
	}
	return false
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
		// Skip MRs with release labels (completely ignored)
		if HasReleaseLabel(db, &mr) {
			continue
		}

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
		// Skip MRs with release labels (completely ignored)
		if HasReleaseLabel(db, &mr) {
			continue
		}

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

// FindReleaseManagerActionMRs returns MRs that are fully approved and ready for release
// for repositories where the user is a release manager.
func FindReleaseManagerActionMRs(db *gorm.DB, userID uint) ([]DigestMR, error) {
	// Find repositories where user is a release manager
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

	// Find open MRs with reviewers in those repos
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

	var releaseMRs []DigestMR
	for _, mr := range mrs {
		// Skip MRs with release labels (completely ignored)
		if HasReleaseLabel(db, &mr) {
			continue
		}

		// Only include fully approved MRs
		if !IsMRFullyApproved(&mr) {
			continue
		}

		stateInfo := GetStateInfo(db, &mr)
		blocked := IsMRBlocked(db, &mr)

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
