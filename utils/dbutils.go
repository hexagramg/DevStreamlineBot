package utils

import (
	"devstreamlinebot/models"

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
