package utils

import (
	"sort"
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
// Note: on_fixes only applies when there are unresolved threads where the last
// comment is NOT from the MR author (i.e., author hasn't responded yet).
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

	if HasThreadsAwaitingAuthor(db, mr.ID, mr.AuthorID) {
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
		var threads []struct {
			GitlabCreatedAt time.Time
			Resolved        bool
			ResolvedAt      *time.Time
		}
		db.Model(&models.MRComment{}).
			Select("gitlab_created_at, resolved, resolved_at").
			Where(`merge_request_id = ? AND resolvable = ?`, mr.ID, true).
			Find(&threads)

		if len(threads) == 0 {
			return nil
		}

		type event struct {
			time    time.Time
			isStart bool
		}
		var events []event
		for _, t := range threads {
			events = append(events, event{t.GitlabCreatedAt, true})
			if t.Resolved && t.ResolvedAt != nil {
				events = append(events, event{*t.ResolvedAt, false})
			}
		}

		sort.Slice(events, func(i, j int) bool {
			return events[i].time.Before(events[j].time)
		})

		activeCount := 0
		var periodStart *time.Time
		for _, e := range events {
			if e.isStart {
				if activeCount == 0 {
					t := e.time
					periodStart = &t
				}
				activeCount++
			} else {
				activeCount--
				if activeCount == 0 {
					periodStart = nil
				}
			}
		}

		return periodStart

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

// HasThreadsAwaitingAuthor returns true if MR has any unresolved threads
// where the last comment is NOT from the MR author.
// A thread where the author commented last is considered "handled" (awaiting reviewer action).
func HasThreadsAwaitingAuthor(db *gorm.DB, mrID uint, mrAuthorID uint) bool {
	var count int64
	db.Model(&models.MRComment{}).
		Where(`merge_request_id = ? AND is_last_in_thread = ? AND thread_starter_id IS NOT NULL AND author_id != ?
			AND EXISTS (SELECT 1 FROM mr_comments starter WHERE starter.gitlab_discussion_id = mr_comments.gitlab_discussion_id AND starter.resolvable = ? AND starter.resolved = ?)`,
			mrID, true, mrAuthorID, true, false).
		Count(&count)
	return count > 0
}

// CountThreadsAwaitingAuthor returns count of unresolved threads where the last
// comment is NOT from the MR author.
func CountThreadsAwaitingAuthor(db *gorm.DB, mrID uint, mrAuthorID uint) int64 {
	var count int64
	db.Model(&models.MRComment{}).
		Where(`merge_request_id = ? AND is_last_in_thread = ? AND thread_starter_id IS NOT NULL AND author_id != ?
			AND EXISTS (SELECT 1 FROM mr_comments starter WHERE starter.gitlab_discussion_id = mr_comments.gitlab_discussion_id AND starter.resolvable = ? AND starter.resolved = ?)`,
			mrID, true, mrAuthorID, true, false).
		Count(&count)
	return count
}

func GetUnresolvedComments(db *gorm.DB, mrID uint) []models.MRComment {
	var comments []models.MRComment

	db.Where("merge_request_id = ? AND resolvable = ? AND resolved = ?", mrID, true, false).
		Preload("Author").
		Order("gitlab_created_at ASC").
		Find(&comments)

	return comments
}

// GetUserStateTransitionTime calculates when a specific user entered their current action state.
// For authors: when they entered "needs to fix" state (unresolved threads awaiting response)
// For reviewers: when they entered "waiting" or "needs action" state
func GetUserStateTransitionTime(db *gorm.DB, mr *models.MergeRequest, userID uint) *time.Time {
	if mr.AuthorID == userID {
		return getAuthorStateTransitionTime(db, mr)
	}
	return getReviewerStateTransitionTime(db, mr, userID)
}

// getAuthorStateTransitionTime returns when author entered current state.
// If author has unresolved threads awaiting their response → on_fixes → earliest awaiting thread
// Otherwise → on_review → use existing logic
func getAuthorStateTransitionTime(db *gorm.DB, mr *models.MergeRequest) *time.Time {
	if HasThreadsAwaitingAuthor(db, mr.ID, mr.AuthorID) {
		return getAuthorOnFixesTime(db, mr)
	}
	return GetStateTransitionTime(db, mr, StateOnReview)
}

// getAuthorOnFixesTime finds when author entered on_fixes state.
// Returns the earliest time when an unresolved thread started awaiting author response.
func getAuthorOnFixesTime(db *gorm.DB, mr *models.MergeRequest) *time.Time {
	var awaitingThreads []struct {
		DiscussionID string
	}
	db.Model(&models.MRComment{}).
		Select("DISTINCT gitlab_discussion_id as discussion_id").
		Where(`merge_request_id = ? AND is_last_in_thread = ? AND author_id != ?
			AND EXISTS (SELECT 1 FROM mr_comments starter
				WHERE starter.gitlab_discussion_id = mr_comments.gitlab_discussion_id
				AND starter.resolvable = ? AND starter.resolved = ?)`,
			mr.ID, true, mr.AuthorID, true, false).
		Find(&awaitingThreads)

	if len(awaitingThreads) == 0 {
		return nil
	}

	var earliestTime *time.Time
	for _, thread := range awaitingThreads {
		waitStart := getThreadAwaitingAuthorTime(db, thread.DiscussionID, mr.AuthorID)
		if waitStart != nil && (earliestTime == nil || waitStart.Before(*earliestTime)) {
			earliestTime = waitStart
		}
	}
	return earliestTime
}

// getThreadAwaitingAuthorTime returns when author started being awaited in this thread.
// This is when reviewer commented after author's last reply (or thread creation if no author reply).
func getThreadAwaitingAuthorTime(db *gorm.DB, discussionID string, mrAuthorID uint) *time.Time {
	var comments []models.MRComment
	db.Where("gitlab_discussion_id = ?", discussionID).
		Order("gitlab_created_at ASC").
		Find(&comments)

	if len(comments) == 0 {
		return nil
	}

	var lastAuthorTime *time.Time
	for i := len(comments) - 1; i >= 0; i-- {
		if comments[i].AuthorID == mrAuthorID {
			lastAuthorTime = &comments[i].GitlabCreatedAt
			break
		}
	}

	if lastAuthorTime == nil {
		for _, c := range comments {
			if c.AuthorID != mrAuthorID {
				t := c.GitlabCreatedAt
				return &t
			}
		}
		return nil
	}

	for _, c := range comments {
		if c.AuthorID != mrAuthorID && c.GitlabCreatedAt.After(*lastAuthorTime) {
			t := c.GitlabCreatedAt
			return &t
		}
	}

	return nil
}

// getReviewerStateTransitionTime returns when reviewer entered their current state.
// If reviewer has unresolved threads where they're last → waiting for author → earliest wait start
// Otherwise → needs action → when they entered that state
func getReviewerStateTransitionTime(db *gorm.DB, mr *models.MergeRequest, reviewerID uint) *time.Time {
	var awaitingThreads []struct {
		DiscussionID string
	}
	db.Model(&models.MRComment{}).
		Select("DISTINCT gitlab_discussion_id as discussion_id").
		Where(`merge_request_id = ? AND is_last_in_thread = ? AND author_id = ?
			AND EXISTS (SELECT 1 FROM mr_comments starter
				WHERE starter.gitlab_discussion_id = mr_comments.gitlab_discussion_id
				AND starter.resolvable = ? AND starter.resolved = ?)`,
			mr.ID, true, reviewerID, true, false).
		Find(&awaitingThreads)

	if len(awaitingThreads) == 0 {
		return getReviewerNeedsActionTime(db, mr, reviewerID)
	}

	var earliestWaitStart *time.Time
	for _, thread := range awaitingThreads {
		waitStart := getThreadWaitStartForReviewer(db, thread.DiscussionID, mr.AuthorID)
		if waitStart != nil && (earliestWaitStart == nil || waitStart.Before(*earliestWaitStart)) {
			earliestWaitStart = waitStart
		}
	}
	return earliestWaitStart
}

// getThreadWaitStartForReviewer returns when reviewer started waiting for author in this thread.
// This is when reviewer commented after author's last reply (or thread creation if no author reply).
func getThreadWaitStartForReviewer(db *gorm.DB, discussionID string, mrAuthorID uint) *time.Time {
	var comments []models.MRComment
	db.Where("gitlab_discussion_id = ?", discussionID).
		Order("gitlab_created_at ASC").
		Find(&comments)

	if len(comments) == 0 {
		return nil
	}

	var lastAuthorTime *time.Time
	for i := len(comments) - 1; i >= 0; i-- {
		if comments[i].AuthorID == mrAuthorID {
			lastAuthorTime = &comments[i].GitlabCreatedAt
			break
		}
	}

	if lastAuthorTime == nil {
		t := comments[0].GitlabCreatedAt
		return &t
	}

	for _, c := range comments {
		if c.AuthorID != mrAuthorID && c.GitlabCreatedAt.After(*lastAuthorTime) {
			t := c.GitlabCreatedAt
			return &t
		}
	}

	return nil
}

// getReviewerNeedsActionTime returns when reviewer entered "needs action" state.
// This happens when:
// 1. Author replied to all threads where reviewer was waiting
// 2. Threads got resolved
// 3. Reviewer was assigned (if no waiting history)
func getReviewerNeedsActionTime(db *gorm.DB, mr *models.MergeRequest, reviewerID uint) *time.Time {
	var candidates []time.Time

	var lastAuthorReply models.MRComment
	err := db.Where(`merge_request_id = ? AND author_id = ?
		AND EXISTS (SELECT 1 FROM mr_comments starter
			WHERE starter.gitlab_discussion_id = mr_comments.gitlab_discussion_id
			AND starter.thread_starter_id = ?)`,
		mr.ID, mr.AuthorID, reviewerID).
		Order("gitlab_created_at DESC").
		First(&lastAuthorReply).Error
	if err == nil {
		candidates = append(candidates, lastAuthorReply.GitlabCreatedAt)
	}

	var lastResolved models.MRAction
	err = db.Where(`merge_request_id = ? AND action_type = ?
		AND comment_id IN (SELECT id FROM mr_comments WHERE thread_starter_id = ?)`,
		mr.ID, models.ActionCommentResolved, reviewerID).
		Order("timestamp DESC").
		First(&lastResolved).Error
	if err == nil {
		candidates = append(candidates, lastResolved.Timestamp)
	}

	var reviewerAssigned models.MRAction
	err = db.Where("merge_request_id = ? AND action_type = ? AND target_user_id = ?",
		mr.ID, models.ActionReviewerAssigned, reviewerID).
		Order("timestamp DESC").
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
