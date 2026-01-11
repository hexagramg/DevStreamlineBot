package utils

import (
	"time"

	"devstreamlinebot/models"

	"gorm.io/gorm"
)

// MRState represents the derived state of a merge request.
type MRState string

const (
	StateOnReview MRState = "on_review" // Has reviewers, no unresolved resolvable comments, not draft
	StateOnFixes  MRState = "on_fixes"  // Has unresolved resolvable comments (awaiting author fixes)
	StateDraft    MRState = "draft"     // MR is marked as draft/WIP
	StateMerged   MRState = "merged"    // MR has been merged
	StateClosed   MRState = "closed"    // MR has been closed without merging
)

// DeriveState determines the current state of a merge request based on DB data.
// Priority order: merged > closed > draft > on_fixes > on_review
func DeriveState(db *gorm.DB, mr *models.MergeRequest) MRState {
	// Check terminal states first
	if mr.State == "merged" {
		return StateMerged
	}
	if mr.State == "closed" {
		return StateClosed
	}

	// Check if draft
	if mr.Draft {
		return StateDraft
	}

	// Check for unresolved resolvable comments
	if HasUnresolvedComments(db, mr.ID) {
		return StateOnFixes
	}

	// Default: on review (has reviewers or awaiting assignment)
	return StateOnReview
}

// StateInfo contains derived state information for an MR.
type StateInfo struct {
	State           MRState
	StateSince      *time.Time    // When MR entered current state
	TimeInState     time.Duration // Total time in current state
	WorkingTime     time.Duration // Working time only (excludes weekends/holidays)
	UnresolvedCount int64         // Number of unresolved resolvable comments
}

// GetStateInfo returns comprehensive state information for an MR.
func GetStateInfo(db *gorm.DB, mr *models.MergeRequest) StateInfo {
	state := DeriveState(db, mr)

	info := StateInfo{
		State: state,
	}

	// Get when MR entered current state
	info.StateSince = GetStateTransitionTime(db, mr, state)

	// Calculate time in state
	if info.StateSince != nil {
		info.TimeInState = time.Since(*info.StateSince)

		// Calculate working time (excludes weekends/holidays)
		info.WorkingTime = CalculateWorkingTime(db, mr.RepositoryID, *info.StateSince, time.Now())
	}

	// Count unresolved comments
	db.Model(&models.MRComment{}).
		Where("merge_request_id = ? AND resolvable = ? AND resolved = ?", mr.ID, true, false).
		Count(&info.UnresolvedCount)

	return info
}

// GetStateTransitionTime returns when the MR entered its current state.
// Uses MRAction records and MR timestamps to determine transition time.
func GetStateTransitionTime(db *gorm.DB, mr *models.MergeRequest, state MRState) *time.Time {
	switch state {
	case StateMerged:
		return mr.MergedAt

	case StateClosed:
		return mr.ClosedAt

	case StateDraft:
		// Find the most recent draft toggle action where draft became true
		var action models.MRAction
		err := db.Where("merge_request_id = ? AND action_type = ?", mr.ID, models.ActionDraftToggled).
			Order("timestamp DESC").
			First(&action).Error
		if err == nil && action.Metadata == `{"draft":true}` {
			return &action.Timestamp
		}
		// Fallback to MR creation if it was created as draft
		return mr.GitlabCreatedAt

	case StateOnFixes:
		// Find when the first unresolved comment was added
		// This is when the MR transitioned to "on_fixes"
		var firstUnresolvedComment models.MRComment
		err := db.Where("merge_request_id = ? AND resolvable = ? AND resolved = ?", mr.ID, true, false).
			Order("gitlab_created_at ASC").
			First(&firstUnresolvedComment).Error
		if err == nil {
			return &firstUnresolvedComment.GitlabCreatedAt
		}
		return nil

	case StateOnReview:
		// Find the most recent event that put MR into review state:
		// 1. Last comment resolved (if any were resolved)
		// 2. Draft was unmarked
		// 3. Reviewer was assigned
		// 4. MR was created (if none of the above)

		var candidates []time.Time

		// Check last resolved comment
		var lastResolved models.MRAction
		err := db.Where("merge_request_id = ? AND action_type = ?", mr.ID, models.ActionCommentResolved).
			Order("timestamp DESC").
			First(&lastResolved).Error
		if err == nil {
			candidates = append(candidates, lastResolved.Timestamp)
		}

		// Check draft unmarked
		var draftToggle models.MRAction
		err = db.Where("merge_request_id = ? AND action_type = ? AND metadata = ?",
			mr.ID, models.ActionDraftToggled, `{"draft":false}`).
			Order("timestamp DESC").
			First(&draftToggle).Error
		if err == nil {
			candidates = append(candidates, draftToggle.Timestamp)
		}

		// Check first reviewer assigned
		var reviewerAssigned models.MRAction
		err = db.Where("merge_request_id = ? AND action_type = ?", mr.ID, models.ActionReviewerAssigned).
			Order("timestamp ASC").
			First(&reviewerAssigned).Error
		if err == nil {
			candidates = append(candidates, reviewerAssigned.Timestamp)
		}

		// Find the most recent candidate
		if len(candidates) > 0 {
			latest := candidates[0]
			for _, t := range candidates[1:] {
				if t.After(latest) {
					latest = t
				}
			}
			return &latest
		}

		// Fallback to MR creation
		return mr.GitlabCreatedAt
	}

	return nil
}

// GetReviewerTimeline returns all actions related to a specific reviewer for an MR.
func GetReviewerTimeline(db *gorm.DB, mrID uint, reviewerID uint) []models.MRAction {
	var actions []models.MRAction

	db.Where("merge_request_id = ? AND (actor_id = ? OR target_user_id = ?)", mrID, reviewerID, reviewerID).
		Order("timestamp ASC").
		Find(&actions)

	return actions
}

// GetMRTimeline returns all actions for an MR in chronological order.
func GetMRTimeline(db *gorm.DB, mrID uint) []models.MRAction {
	var actions []models.MRAction

	db.Where("merge_request_id = ?", mrID).
		Preload("Actor").
		Preload("TargetUser").
		Preload("Comment").
		Order("timestamp ASC").
		Find(&actions)

	return actions
}

// HasUnresolvedComments returns true if the MR has any unresolved resolvable comments.
func HasUnresolvedComments(db *gorm.DB, mrID uint) bool {
	var count int64
	db.Model(&models.MRComment{}).
		Where("merge_request_id = ? AND resolvable = ? AND resolved = ?", mrID, true, false).
		Count(&count)
	return count > 0
}

// GetUnresolvedComments returns all unresolved resolvable comments for an MR.
func GetUnresolvedComments(db *gorm.DB, mrID uint) []models.MRComment {
	var comments []models.MRComment

	db.Where("merge_request_id = ? AND resolvable = ? AND resolved = ?", mrID, true, false).
		Preload("Author").
		Order("gitlab_created_at ASC").
		Find(&comments)

	return comments
}
