package polling

import (
	"fmt"
	"log"
	"time"

	"devstreamlinebot/models"

	gitlab "gitlab.com/gitlab-org/api/client-go"
	"gorm.io/gorm"
)

// recordMRAction creates an MRAction entry with duplicate detection.
// It checks if an identical action already exists within a short time window to avoid duplicates.
// commentID links to MRComment for comment-related actions (ActionCommentAdded, ActionCommentResolved).
func recordMRAction(db *gorm.DB, mrID uint, actionType models.MRActionType, actorID *uint, targetUserID *uint, commentID *uint, timestamp time.Time, metadata string) {
	// Check for duplicate within 1 minute window
	var existing models.MRAction
	query := db.Where("merge_request_id = ? AND action_type = ? AND timestamp > ?",
		mrID, actionType, timestamp.Add(-time.Minute))

	if actorID != nil {
		query = query.Where("actor_id = ?", *actorID)
	} else {
		query = query.Where("actor_id IS NULL")
	}

	if targetUserID != nil {
		query = query.Where("target_user_id = ?", *targetUserID)
	} else {
		query = query.Where("target_user_id IS NULL")
	}

	if commentID != nil {
		query = query.Where("comment_id = ?", *commentID)
	} else {
		query = query.Where("comment_id IS NULL")
	}

	if err := query.First(&existing).Error; err == nil {
		// Duplicate found, skip
		return
	}

	action := models.MRAction{
		MergeRequestID: mrID,
		ActionType:     actionType,
		ActorID:        actorID,
		TargetUserID:   targetUserID,
		CommentID:      commentID,
		Timestamp:      timestamp,
		Metadata:       metadata,
	}

	if err := db.Create(&action).Error; err != nil {
		log.Printf("Error recording MR action %s for MR %d: %v", actionType, mrID, err)
	}
}

// detectBlockLabelChanges detects when block labels are added or removed from an MR.
// Records ActionBlockLabelAdded/ActionBlockLabelRemoved actions for SLA tracking.
func detectBlockLabelChanges(db *gorm.DB, mrID uint, repoID uint, oldLabels, newLabels []string) {
	// Get block labels for this repo
	var blockLabels []models.BlockLabel
	db.Where("repository_id = ?", repoID).Find(&blockLabels)
	if len(blockLabels) == 0 {
		return
	}

	blockLabelSet := make(map[string]bool)
	for _, bl := range blockLabels {
		blockLabelSet[bl.LabelName] = true
	}

	oldSet := make(map[string]bool)
	for _, l := range oldLabels {
		oldSet[l] = true
	}
	newSet := make(map[string]bool)
	for _, l := range newLabels {
		newSet[l] = true
	}

	now := time.Now()

	// Check for added block labels
	for label := range newSet {
		if blockLabelSet[label] && !oldSet[label] {
			recordMRAction(db, mrID, models.ActionBlockLabelAdded, nil, nil, nil, now,
				fmt.Sprintf(`{"label":"%s"}`, label))
		}
	}

	// Check for removed block labels
	for label := range oldSet {
		if blockLabelSet[label] && !newSet[label] {
			recordMRAction(db, mrID, models.ActionBlockLabelRemoved, nil, nil, nil, now,
				fmt.Sprintf(`{"label":"%s"}`, label))
		}
	}
}

// detectAndRecordStateChanges compares old and new MR state and records relevant actions.
func detectAndRecordStateChanges(db *gorm.DB, existingMR *models.MergeRequest, newMR *gitlab.BasicMergeRequest, localMRID uint) {
	now := time.Now()

	// Detect draft toggle
	if existingMR != nil && existingMR.Draft != newMR.Draft {
		recordMRAction(db, localMRID, models.ActionDraftToggled, nil, nil, nil, now, fmt.Sprintf(`{"draft":%t}`, newMR.Draft))
	}

	// Detect merge
	if existingMR != nil && existingMR.State != "merged" && newMR.State == "merged" {
		timestamp := now
		if newMR.MergedAt != nil {
			timestamp = *newMR.MergedAt
		}
		recordMRAction(db, localMRID, models.ActionMerged, nil, nil, nil, timestamp, "")
	}

	// Detect close
	if existingMR != nil && existingMR.State != "closed" && newMR.State == "closed" {
		timestamp := now
		if newMR.ClosedAt != nil {
			timestamp = *newMR.ClosedAt
		}
		recordMRAction(db, localMRID, models.ActionClosed, nil, nil, nil, timestamp, "")
	}
}

// syncMRDiscussions fetches and syncs GitLab discussions/comments for an MR.
func syncMRDiscussions(db *gorm.DB, client *gitlab.Client, projectID int, mrIID int, localMRID uint) {
	opts := &gitlab.ListMergeRequestDiscussionsOptions{
		PerPage: 100,
		Page:    1,
	}

	for {
		discussions, resp, err := client.Discussions.ListMergeRequestDiscussions(projectID, mrIID, opts)
		if err != nil {
			log.Printf("Error fetching discussions for project %d MR IID %d: %v", projectID, mrIID, err)
			return
		}

		for _, discussion := range discussions {
			for _, note := range discussion.Notes {
				// Skip system notes (e.g., "mentioned in commit", "changed the description")
				if note.System {
					continue
				}

				// Upsert the comment author
				var author models.User
				if note.Author.ID != 0 {
					authorData := models.User{
						GitlabID:  note.Author.ID,
						Username:  note.Author.Username,
						Name:      note.Author.Name,
						State:     note.Author.State,
						AvatarURL: note.Author.AvatarURL,
						WebURL:    note.Author.WebURL,
					}
					if err := db.Where(models.User{GitlabID: note.Author.ID}).Assign(authorData).FirstOrCreate(&author).Error; err != nil {
						log.Printf("Error upserting comment author GitlabID %d: %v", note.Author.ID, err)
						continue
					}
				}

				// Check if comment already exists
				var existingComment models.MRComment
				err := db.Where("gitlab_note_id = ?", note.ID).First(&existingComment).Error

				// Prepare resolved by user if applicable
				var resolvedByID *uint
				if note.ResolvedBy.ID != 0 {
					var resolvedByUser models.User
					resolvedByData := models.User{
						GitlabID:  note.ResolvedBy.ID,
						Username:  note.ResolvedBy.Username,
						Name:      note.ResolvedBy.Name,
						AvatarURL: note.ResolvedBy.AvatarURL,
						WebURL:    note.ResolvedBy.WebURL,
					}
					if err := db.Where(models.User{GitlabID: note.ResolvedBy.ID}).Assign(resolvedByData).FirstOrCreate(&resolvedByUser).Error; err != nil {
						log.Printf("Error upserting resolved_by user GitlabID %d: %v", note.ResolvedBy.ID, err)
					} else {
						resolvedByID = &resolvedByUser.ID
					}
				}

				comment := models.MRComment{
					MergeRequestID:     localMRID,
					GitlabNoteID:       note.ID,
					GitlabDiscussionID: discussion.ID,
					AuthorID:           author.ID,
					Body:               note.Body,
					Resolvable:         note.Resolvable,
					Resolved:           note.Resolved,
					ResolvedByID:       resolvedByID,
					ResolvedAt:         note.ResolvedAt,
					GitlabCreatedAt:    *note.CreatedAt,
				}
				if note.UpdatedAt != nil {
					comment.GitlabUpdatedAt = *note.UpdatedAt
				}

				if err == gorm.ErrRecordNotFound {
					// New comment
					if err := db.Create(&comment).Error; err != nil {
						log.Printf("Error creating comment for MR %d, note %d: %v", localMRID, note.ID, err)
						continue
					}
					// Record action for new comment with link to the comment
					recordMRAction(db, localMRID, models.ActionCommentAdded, &author.ID, nil, &comment.ID, *note.CreatedAt, "")
				} else if err == nil {
					// Existing comment - check for resolved state change (only for resolvable notes)
					wasResolved := existingComment.Resolved
					isResolved := note.Resolved

					// Update the comment
					comment.ID = existingComment.ID
					if err := db.Model(&existingComment).Updates(comment).Error; err != nil {
						log.Printf("Error updating comment for MR %d, note %d: %v", localMRID, note.ID, err)
						continue
					}

					// Record action if resolved state changed (only for resolvable notes)
					if note.Resolvable && !wasResolved && isResolved && note.ResolvedAt != nil {
						recordMRAction(db, localMRID, models.ActionCommentResolved, resolvedByID, &author.ID, &comment.ID, *note.ResolvedAt, "")
					}
				}
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
}

// syncMRApprovals syncs approvals and records approval actions.
func syncMRApprovals(db *gorm.DB, client *gitlab.Client, projectID int, mrIID int, localMRID uint) []models.User {
	approvals, _, err := client.MergeRequests.GetMergeRequestApprovals(projectID, mrIID)
	if err != nil {
		log.Printf("Failed to fetch MR approvals for project %d MR IID %d: %v", projectID, mrIID, err)
		return nil
	}

	if approvals == nil {
		return nil
	}

	// Get existing approvers from DB to detect new approvals
	var existingApprovers []models.User
	db.Model(&models.MergeRequest{Model: gorm.Model{ID: localMRID}}).Association("Approvers").Find(&existingApprovers)
	existingApproverIDs := make(map[int]bool)
	for _, u := range existingApprovers {
		existingApproverIDs[u.GitlabID] = true
	}

	var approverUsers []models.User
	for _, ap := range approvals.ApprovedBy {
		if ap.User == nil {
			continue
		}

		var u models.User
		approverData := models.User{
			GitlabID:  ap.User.ID,
			Username:  ap.User.Username,
			Name:      ap.User.Name,
			State:     ap.User.State,
			AvatarURL: ap.User.AvatarURL,
			WebURL:    ap.User.WebURL,
		}
		if err := db.Where(models.User{GitlabID: ap.User.ID}).Assign(approverData).FirstOrCreate(&u).Error; err != nil {
			log.Printf("Error upserting approver GitlabID %d: %v", ap.User.ID, err)
			continue
		}
		approverUsers = append(approverUsers, u)

		// Record approval action if this is a new approver
		if !existingApproverIDs[ap.User.ID] {
			timestamp := time.Now()
			// Note: GitLab API doesn't provide ApprovedAt in approvals.ApprovedBy
			// Using current time as fallback
			recordMRAction(db, localMRID, models.ActionApproved, &u.ID, nil, nil, timestamp, "")
		}
	}

	// Detect unapprovals (approvers that were removed)
	for _, existing := range existingApprovers {
		found := false
		for _, ap := range approvals.ApprovedBy {
			if ap.User != nil && ap.User.ID == existing.GitlabID {
				found = true
				break
			}
		}
		if !found {
			recordMRAction(db, localMRID, models.ActionUnapproved, &existing.ID, nil, nil, time.Now(), "")
		}
	}

	return approverUsers
}

// syncGitLabMRToDB processes a single merge request from GitLab and upserts it into the database.
// It handles the MR itself, its author, assignee, labels, reviewers, and approvers.
// Assumes models.User.CreatedAt is *time.Time
func syncGitLabMRToDB(db *gorm.DB, client *gitlab.Client, mr *gitlab.BasicMergeRequest, localRepositoryID uint, gitlabProjectID int) (uint, error) {
	// Upsert author
	var author models.User
	// mr.Author is *gitlab.User which has CreatedAt *time.Time and Locked bool
	authorData := models.User{
		GitlabID:  mr.Author.ID,
		Username:  mr.Author.Username,
		Name:      mr.Author.Name,
		State:     mr.Author.State,
		Locked:    mr.Author.Locked,
		CreatedAt: mr.Author.CreatedAt,
		AvatarURL: mr.Author.AvatarURL,
		WebURL:    mr.Author.WebURL,
	}
	if err := db.Where(models.User{GitlabID: mr.Author.ID}).Assign(authorData).FirstOrCreate(&author).Error; err != nil {
		log.Printf("Error upserting author GitlabID %d for MR %d: %v", mr.Author.ID, mr.ID, err)
		return 0, fmt.Errorf("upserting author GitlabID %d: %w", mr.Author.ID, err)
	}

	// Upsert assignee
	var assignee models.User
	var assigneeID uint
	if mr.Assignee != nil {
		// mr.Assignee is *gitlab.User
		assigneeData := models.User{
			GitlabID:  mr.Assignee.ID,
			Username:  mr.Assignee.Username,
			Name:      mr.Assignee.Name,
			State:     mr.Assignee.State,
			Locked:    mr.Assignee.Locked, // Assuming models.User.Locked is bool
			CreatedAt: mr.Assignee.CreatedAt,
			AvatarURL: mr.Assignee.AvatarURL,
			WebURL:    mr.Assignee.WebURL,
		}
		if err := db.Where(models.User{GitlabID: mr.Assignee.ID}).Assign(assigneeData).FirstOrCreate(&assignee).Error; err != nil {
			log.Printf("Error upserting assignee GitlabID %d for MR %d: %v", mr.Assignee.ID, mr.ID, err)
			return 0, fmt.Errorf("upserting assignee GitlabID %d: %w", mr.Assignee.ID, err)
		}
		assigneeID = assignee.ID
	}

	// Build MR model
	mrModel := models.MergeRequest{
		GitlabID:                    mr.ID,
		IID:                         mr.IID,
		Title:                       mr.Title,
		Description:                 mr.Description,
		State:                       mr.State,
		SourceBranch:                mr.SourceBranch,
		TargetBranch:                mr.TargetBranch,
		WebURL:                      mr.WebURL,
		Upvotes:                     mr.Upvotes,
		Downvotes:                   mr.Downvotes,
		DiscussionLocked:            mr.DiscussionLocked,
		ShouldRemoveSourceBranch:    mr.ShouldRemoveSourceBranch,
		ForceRemoveSourceBranch:     mr.ForceRemoveSourceBranch,
		DetailedMergeStatus:         mr.DetailedMergeStatus,
		Draft:                       mr.Draft,
		AuthorID:                    author.ID,
		AssigneeID:                  assigneeID,
		RepositoryID:                localRepositoryID,
		HasConflicts:                mr.HasConflicts,
		BlockingDiscussionsResolved: mr.BlockingDiscussionsResolved,
		GitlabCreatedAt:             mr.CreatedAt,
		GitlabUpdatedAt:             mr.UpdatedAt,
		MergedAt:                    mr.MergedAt,
		MergeAfter:                  mr.MergeAfter,
		PreparedAt:                  mr.PreparedAt,
		ClosedAt:                    mr.ClosedAt,
		SourceProjectID:             mr.SourceProjectID,
		TargetProjectID:             mr.TargetProjectID,
		MergeWhenPipelineSucceeds:   mr.MergeWhenPipelineSucceeds,
		SHA:                         mr.SHA,
		MergeCommitSHA:              mr.MergeCommitSHA,
		SquashCommitSHA:             mr.SquashCommitSHA,
		Squash:                      mr.Squash,
		SquashOnMerge:               mr.SquashOnMerge,
		UserNotesCount:              mr.UserNotesCount,
	}

	// Add References if available
	if mr.References != nil {
		mrModel.References = models.IssueReferences{
			Short:    mr.References.Short,
			Relative: mr.References.Relative,
			Full:     mr.References.Full,
		}
	}

	// Add TimeStats if available
	if mr.TimeStats != nil {
		mrModel.TimeStats = models.TimeStats{
			HumanTimeEstimate:   mr.TimeStats.HumanTimeEstimate,
			HumanTotalTimeSpent: mr.TimeStats.HumanTotalTimeSpent,
			TimeEstimate:        mr.TimeStats.TimeEstimate,
			TotalTimeSpent:      mr.TimeStats.TotalTimeSpent,
		}
	}

	now := time.Now()
	mrModel.LastUpdate = &now

	// Upsert the merge request: update if exists, create if not
	var existingMR models.MergeRequest
	var isNewMR bool
	err := db.Where(models.MergeRequest{GitlabID: mrModel.GitlabID}).First(&existingMR).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		log.Printf("Error checking for existing merge request GitlabID %d: %v", mrModel.GitlabID, err)
		return 0, fmt.Errorf("checking for existing merge request GitlabID %d: %w", mrModel.GitlabID, err)
	}
	if err == gorm.ErrRecordNotFound {
		// Not found, create new
		isNewMR = true
		if err := db.Create(&mrModel).Error; err != nil {
			log.Printf("Error creating merge request GitlabID %d: %v", mrModel.GitlabID, err)
			return 0, fmt.Errorf("creating merge request GitlabID %d: %w", mrModel.GitlabID, err)
		}
	} else {
		// Detect and record state changes before updating
		detectAndRecordStateChanges(db, &existingMR, mr, existingMR.ID)

		// Exists, update all fields
		mrModel.ID = existingMR.ID // ensure correct primary key
		if err := db.Model(&existingMR).Select("*").Updates(mrModel).Error; err != nil {
			log.Printf("Error updating merge request GitlabID %d: %v", mrModel.GitlabID, err)
			return 0, fmt.Errorf("updating merge request GitlabID %d: %w", mrModel.GitlabID, err)
		}
		// Keep mrModel.ID in sync for associations
		mrModel.ID = existingMR.ID
	}

	// For tracking reviewer changes
	var existingReviewerIDs map[int]bool
	if !isNewMR {
		var existingReviewers []models.User
		db.Model(&existingMR).Association("Reviewers").Find(&existingReviewers)
		existingReviewerIDs = make(map[int]bool)
		for _, r := range existingReviewers {
			existingReviewerIDs[r.GitlabID] = true
		}
	}

	// Detect block label changes before syncing labels
	if !isNewMR {
		var existingLabels []models.Label
		db.Model(&existingMR).Association("Labels").Find(&existingLabels)
		var oldLabelNames []string
		for _, l := range existingLabels {
			oldLabelNames = append(oldLabelNames, l.Name)
		}
		detectBlockLabelChanges(db, existingMR.ID, localRepositoryID, oldLabelNames, mr.Labels)
	}

	// Sync labels
	var labelsToAssociate []models.Label
	for _, name := range mr.Labels {
		var lbl models.Label
		labelData := models.Label{Name: name}
		if err := db.Where(models.Label{Name: name}).Assign(labelData).FirstOrCreate(&lbl).Error; err != nil {
			log.Printf("Error upserting label %s for MR GitlabID %d: %v", name, mr.ID, err)
			return 0, fmt.Errorf("upserting label %s for MR GitlabID %d: %w", name, mr.ID, err)
		}
		labelsToAssociate = append(labelsToAssociate, lbl)
	}
	if err := db.Model(&mrModel).Association("Labels").Replace(labelsToAssociate); err != nil {
		log.Printf("Error replacing labels for MR GitlabID %d: %v", mrModel.GitlabID, err)
		return 0, fmt.Errorf("replacing labels for MR GitlabID %d: %w", mrModel.GitlabID, err)
	}

	// Sync reviewers (mr.Reviewers are []*gitlab.BasicUser)
	var reviewersToAssociate []models.User
	for _, rv := range mr.Reviewers {
		var u models.User
		reviewerData := models.User{
			GitlabID:  rv.ID,
			Username:  rv.Username,
			Name:      rv.Name,
			State:     rv.State,
			AvatarURL: rv.AvatarURL,
			WebURL:    rv.WebURL,
		}
		if err := db.Where(models.User{GitlabID: rv.ID}).Assign(reviewerData).FirstOrCreate(&u).Error; err != nil {
			log.Printf("Error upserting reviewer GitlabID %d for MR GitlabID %d: %v", rv.ID, mr.ID, err)
			return 0, fmt.Errorf("upserting reviewer GitlabID %d for MR GitlabID %d: %w", rv.ID, mr.ID, err)
		}
		reviewersToAssociate = append(reviewersToAssociate, u)

		// Record ActionReviewerAssigned for new reviewers
		if existingReviewerIDs != nil && !existingReviewerIDs[rv.ID] {
			recordMRAction(db, mrModel.ID, models.ActionReviewerAssigned, nil, &u.ID, nil, now, "")
		}
	}
	if err := db.Model(&mrModel).Association("Reviewers").Replace(reviewersToAssociate); err != nil {
		log.Printf("Error replacing reviewers for MR GitlabID %d: %v", mrModel.GitlabID, err)
		return 0, fmt.Errorf("replacing reviewers for MR GitlabID %d: %w", mrModel.GitlabID, err)
	}

	// Sync approvers and discussions for active MRs
	if mr.State == "opened" || mr.State == "locked" {
		// Sync approvals with action tracking
		approverUsers := syncMRApprovals(db, client, gitlabProjectID, mr.IID, mrModel.ID)
		if approverUsers != nil {
			if err := db.Model(&mrModel).Association("Approvers").Replace(approverUsers); err != nil {
				log.Printf("Error replacing approvers for MR GitlabID %d: %v", mrModel.GitlabID, err)
				return 0, fmt.Errorf("replacing approvers for MR GitlabID %d: %w", mrModel.GitlabID, err)
			}
		}

		// Sync discussions/comments
		syncMRDiscussions(db, client, gitlabProjectID, mr.IID, mrModel.ID)
	} else {
		// If MR is not 'opened' or 'locked' (e.g., 'merged', 'closed'), clear existing approvers in DB.
		if err := db.Model(&mrModel).Association("Approvers").Clear(); err != nil {
			log.Printf("Error clearing approvers for non-opened MR GitlabID %d: %v", mrModel.GitlabID, err)
			return 0, fmt.Errorf("clearing approvers for MR GitlabID %d: %w", mrModel.GitlabID, err)
		}
	}
	return mrModel.ID, nil
}

// PollMergeRequests repositoriy polling logic.
func PollMergeRequests(db *gorm.DB, client *gitlab.Client) {
	var repos []models.Repository
	if err := db.Where("EXISTS (SELECT 1 FROM repository_subscriptions WHERE repository_subscriptions.repository_id = repositories.id)").
		Find(&repos).Error; err != nil {
		log.Printf("failed to fetch repositories with subscriptions: %v", err)
		return
	}
	for _, repo := range repos {
		log.Printf("Polling merge requests for repository: %s (GitLab ID: %d)", repo.Name, repo.GitlabID)
		allCurrentlyOpenGitlabMRs := []*gitlab.BasicMergeRequest{}
		opts := &gitlab.ListProjectMergeRequestsOptions{
			State:       gitlab.Ptr("opened"),
			ListOptions: gitlab.ListOptions{PerPage: 100, Page: 1},
		}

		// 1. Fetch all currently open MRs from GitLab with pagination
		for {
			mrsPage, resp, err := client.MergeRequests.ListProjectMergeRequests(repo.GitlabID, opts)
			if err != nil {
				log.Printf("Error listing merge requests for project %d page %d: %v", repo.GitlabID, opts.Page, err)
				break // Break from pagination loop for this repo on error
			}

			// Add merge requests directly to our collection without fetching full details
			allCurrentlyOpenGitlabMRs = append(allCurrentlyOpenGitlabMRs, mrsPage...)

			if resp.NextPage == 0 {
				break
			}
			opts.Page = resp.NextPage
		}
		log.Printf("Fetched %d open merge requests from GitLab for repository %s", len(allCurrentlyOpenGitlabMRs), repo.Name)

		// Track database IDs of processed MRs
		processedMRIDs := make([]uint, 0, len(allCurrentlyOpenGitlabMRs))

		// 2. Process/Upsert these fetched open MRs
		for _, gitlabMR := range allCurrentlyOpenGitlabMRs {
			mrID, err := syncGitLabMRToDB(db, client, gitlabMR, repo.ID, repo.GitlabID)
			if err != nil {
				log.Printf("Failed to sync open MR from GitLab API (ProjectID: %d, MR IID: %d, MR ID: %d): %v", repo.GitlabID, gitlabMR.IID, gitlabMR.ID, err)
			} else {
				processedMRIDs = append(processedMRIDs, mrID)
			}
		}

		// 3. Find and sync stale MRs (present in DB as 'opened' but not in the processed list)
		var dbOpenMRs []models.MergeRequest
		query := db.Where("repository_id = ? AND state = ?", repo.ID, "opened")

		// Exclude already processed MRs if we have any
		if len(processedMRIDs) > 0 {
			query = query.Where("id NOT IN ?", processedMRIDs)
		}

		if err := query.Find(&dbOpenMRs).Error; err != nil {
			log.Printf("Error fetching 'opened' MRs from DB for repo %d: %v", repo.ID, err)
			continue // Skip to next repository
		}

		for _, dbMR := range dbOpenMRs {
			// This MR is 'opened' in DB but not in GitLab's 'opened' list. Re-check its status.
			log.Printf("Re-syncing potentially stale MR: RepoGitlabID %d, MR IID %d (DB ID %d, MR GitlabID %d)", repo.GitlabID, dbMR.IID, dbMR.ID, dbMR.GitlabID)
			fullMRDetails, resp, err := client.MergeRequests.GetMergeRequest(repo.GitlabID, dbMR.IID, nil)
			if err != nil {
				if resp != nil && resp.StatusCode == 404 {
					log.Printf("Stale MR not found on GitLab (404): RepoGitlabID %d, MR IID %d. Marking as 'closed'.", repo.GitlabID, dbMR.IID)
					updateData := map[string]interface{}{"state": "closed", "last_update": time.Now()}
					if err := db.Model(&models.MergeRequest{}).Where("id = ?", dbMR.ID).Updates(updateData).Error; err != nil {
						log.Printf("Error updating stale MR (ID %d, GitlabID %d) to closed: %v", dbMR.ID, dbMR.GitlabID, err)
					}
				} else {
					log.Printf("Error fetching details for stale MR: RepoGitlabID %d, MR IID %d: %v", repo.GitlabID, dbMR.IID, err)
				}
				continue // Next stale MR
			}

			// Convert fullMRDetails to *gitlab.BasicMergeRequest
			basicMR := &gitlab.BasicMergeRequest{
				ID:                          fullMRDetails.ID,
				IID:                         fullMRDetails.IID,
				Title:                       fullMRDetails.Title,
				Description:                 fullMRDetails.Description,
				State:                       fullMRDetails.State,
				SourceBranch:                fullMRDetails.SourceBranch,
				TargetBranch:                fullMRDetails.TargetBranch,
				WebURL:                      fullMRDetails.WebURL,
				Upvotes:                     fullMRDetails.Upvotes,
				Downvotes:                   fullMRDetails.Downvotes,
				DiscussionLocked:            fullMRDetails.DiscussionLocked,
				ShouldRemoveSourceBranch:    fullMRDetails.ShouldRemoveSourceBranch,
				ForceRemoveSourceBranch:     fullMRDetails.ForceRemoveSourceBranch,
				Author:                      fullMRDetails.Author,
				Assignee:                    fullMRDetails.Assignee,
				Labels:                      fullMRDetails.Labels,
				Reviewers:                   fullMRDetails.Reviewers,
				HasConflicts:                fullMRDetails.HasConflicts,
				BlockingDiscussionsResolved: fullMRDetails.BlockingDiscussionsResolved,
				DetailedMergeStatus:         fullMRDetails.DetailedMergeStatus,
				Draft:                       fullMRDetails.Draft,
				References:                  fullMRDetails.References,
				TimeStats:                   fullMRDetails.TimeStats,
				CreatedAt:                   fullMRDetails.CreatedAt,
				UpdatedAt:                   fullMRDetails.UpdatedAt,
				MergedAt:                    fullMRDetails.MergedAt,
				MergeAfter:                  fullMRDetails.MergeAfter,
				PreparedAt:                  fullMRDetails.PreparedAt,
				ClosedAt:                    fullMRDetails.ClosedAt,
				SourceProjectID:             fullMRDetails.SourceProjectID,
				TargetProjectID:             fullMRDetails.TargetProjectID,
				MergeWhenPipelineSucceeds:   fullMRDetails.MergeWhenPipelineSucceeds,
				SHA:                         fullMRDetails.SHA,
				MergeCommitSHA:              fullMRDetails.MergeCommitSHA,
				SquashCommitSHA:             fullMRDetails.SquashCommitSHA,
				Squash:                      fullMRDetails.Squash,
				SquashOnMerge:               fullMRDetails.SquashOnMerge,
				UserNotesCount:              fullMRDetails.UserNotesCount,
			}

			// Fetched details, now sync them
			_, err = syncGitLabMRToDB(db, client, basicMR, repo.ID, repo.GitlabID)
			if err != nil {
				log.Printf("Failed to re-sync stale MR (ProjectID: %d, MR IID: %d, MR ID: %d): %v", repo.GitlabID, fullMRDetails.IID, fullMRDetails.ID, err)
			} else {
				log.Printf("Successfully re-synced stale MR: RepoGitlabID %d, MR IID %d. New state: %s", repo.GitlabID, fullMRDetails.IID, fullMRDetails.State)
			}
		}
		log.Printf("Finished polling merge requests for repository: %s", repo.Name)
	}
}
