package utils

import (
	"time"

	"devstreamlinebot/models"

	"gorm.io/gorm"
)

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
	if mr.State == "merged" {
		return StateMerged
	}
	if mr.State == "closed" {
		return StateClosed
	}

	if mr.Draft {
		return StateDraft
	}

	if HasUnresolvedComments(db, mr.ID) {
		return StateOnFixes
	}

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

func GetStateInfo(db *gorm.DB, mr *models.MergeRequest) StateInfo {
	state := DeriveState(db, mr)

	info := StateInfo{
		State: state,
	}

	info.StateSince = GetStateTransitionTime(db, mr, state)

	if info.StateSince != nil {
		now := time.Now()
		info.TimeInState = now.Sub(*info.StateSince)

		info.WorkingTime = CalculateWorkingTime(db, mr.RepositoryID, *info.StateSince, now)

		blockedTime := CalculateBlockedTime(db, mr.ID, mr.RepositoryID, *info.StateSince, now)
		info.WorkingTime -= blockedTime
		if info.WorkingTime < 0 {
			info.WorkingTime = 0
		}
	}

	db.Model(&models.MRComment{}).
		Where("merge_request_id = ? AND resolvable = ? AND resolved = ?", mr.ID, true, false).
		Count(&info.UnresolvedCount)

	return info
}

// CalculateBlockedTime calculates total working time an MR was blocked by block labels
// within the given time window. Uses MRAction records for retrospective calculation.
// Handles overlapping block labels (multiple block labels = still just blocked once).
func CalculateBlockedTime(db *gorm.DB, mrID uint, repoID uint, start, end time.Time) time.Duration {
	var actions []models.MRAction
	db.Where("merge_request_id = ? AND action_type IN ?", mrID,
		[]models.MRActionType{models.ActionBlockLabelAdded, models.ActionBlockLabelRemoved}).
		Order("timestamp ASC").
		Find(&actions)

	if len(actions) == 0 {
		return 0
	}

	activeCount := 0
	var blockStart *time.Time
	var totalBlocked time.Duration

	for _, action := range actions {
		if !action.Timestamp.Before(start) {
			break
		}
		if action.ActionType == models.ActionBlockLabelAdded {
			activeCount++
		} else {
			activeCount--
		}
	}

	if activeCount > 0 {
		blockStart = &start
	}

	for _, action := range actions {
		ts := action.Timestamp
		if ts.Before(start) {
			continue
		}
		if ts.After(end) {
			break
		}

		if action.ActionType == models.ActionBlockLabelAdded {
			if activeCount == 0 {
				blockStart = &ts
			}
			activeCount++
		} else {
			activeCount--
			if activeCount == 0 && blockStart != nil {
				totalBlocked += CalculateWorkingTime(db, repoID, *blockStart, ts)
				blockStart = nil
			}
		}
	}

	if activeCount > 0 && blockStart != nil {
		totalBlocked += CalculateWorkingTime(db, repoID, *blockStart, end)
	}

	return totalBlocked
}

func GetStateTransitionTime(db *gorm.DB, mr *models.MergeRequest, state MRState) *time.Time {
	switch state {
	case StateMerged:
		return mr.MergedAt

	case StateClosed:
		return mr.ClosedAt

	case StateDraft:
		var action models.MRAction
		err := db.Where("merge_request_id = ? AND action_type = ?", mr.ID, models.ActionDraftToggled).
			Order("timestamp DESC").
			First(&action).Error
		if err == nil && action.Metadata == `{"draft":true}` {
			return &action.Timestamp
		}
		return mr.GitlabCreatedAt

	case StateOnFixes:
		var firstUnresolvedComment models.MRComment
		err := db.Where("merge_request_id = ? AND resolvable = ? AND resolved = ?", mr.ID, true, false).
			Order("gitlab_created_at ASC").
			First(&firstUnresolvedComment).Error
		if err == nil {
			return &firstUnresolvedComment.GitlabCreatedAt
		}
		return nil

	case StateOnReview:

		var candidates []time.Time

		var lastResolved models.MRAction
		err := db.Where("merge_request_id = ? AND action_type = ?", mr.ID, models.ActionCommentResolved).
			Order("timestamp DESC").
			First(&lastResolved).Error
		if err == nil {
			candidates = append(candidates, lastResolved.Timestamp)
		}

		var draftToggle models.MRAction
		err = db.Where("merge_request_id = ? AND action_type = ? AND metadata = ?",
			mr.ID, models.ActionDraftToggled, `{"draft":false}`).
			Order("timestamp DESC").
			First(&draftToggle).Error
		if err == nil {
			candidates = append(candidates, draftToggle.Timestamp)
		}

		var reviewerAssigned models.MRAction
		err = db.Where("merge_request_id = ? AND action_type = ?", mr.ID, models.ActionReviewerAssigned).
			Order("timestamp ASC").
			First(&reviewerAssigned).Error
		if err == nil {
			candidates = append(candidates, reviewerAssigned.Timestamp)
		}

		if len(candidates) > 0 {
			latest := candidates[0]
			for _, t := range candidates[1:] {
				if t.After(latest) {
					latest = t
				}
			}
			return &latest
		}

		return mr.GitlabCreatedAt
	}

	return nil
}

func GetReviewerTimeline(db *gorm.DB, mrID uint, reviewerID uint) []models.MRAction {
	var actions []models.MRAction

	db.Where("merge_request_id = ? AND (actor_id = ? OR target_user_id = ?)", mrID, reviewerID, reviewerID).
		Order("timestamp ASC").
		Find(&actions)

	return actions
}

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

func HasUnresolvedComments(db *gorm.DB, mrID uint) bool {
	var count int64
	db.Model(&models.MRComment{}).
		Where("merge_request_id = ? AND resolvable = ? AND resolved = ?", mrID, true, false).
		Count(&count)
	return count > 0
}

func GetUnresolvedComments(db *gorm.DB, mrID uint) []models.MRComment {
	var comments []models.MRComment

	db.Where("merge_request_id = ? AND resolvable = ? AND resolved = ?", mrID, true, false).
		Preload("Author").
		Order("gitlab_created_at ASC").
		Find(&comments)

	return comments
}
