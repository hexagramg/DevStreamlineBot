package polling

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"devstreamlinebot/models"

	gitlab "gitlab.com/gitlab-org/api/client-go"
	"gorm.io/gorm"
)

// recordMRAction creates an MRAction entry with duplicate detection.
// It checks if an identical action already exists within a short time window to avoid duplicates.
// commentID links to MRComment for comment-related actions (ActionCommentAdded, ActionCommentResolved).
func recordMRAction(db *gorm.DB, mrID uint, actionType models.MRActionType, actorID *uint, targetUserID *uint, commentID *uint, timestamp time.Time, metadata string) {
	err := db.Transaction(func(tx *gorm.DB) error {
		var existing models.MRAction
		query := tx.Where("merge_request_id = ? AND action_type = ? AND timestamp > ?",
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
			return nil
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

		return tx.Create(&action).Error
	})

	if err != nil {
		log.Printf("Error recording MR action %s for MR %d: %v", actionType, mrID, err)
	}
}

func detectBlockLabelChanges(db *gorm.DB, mrID uint, repoID uint, oldLabels, newLabels []string) {
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

	now := time.Now().UTC()

	for label := range newSet {
		if blockLabelSet[label] && !oldSet[label] {
			recordMRAction(db, mrID, models.ActionBlockLabelAdded, nil, nil, nil, now,
				fmt.Sprintf(`{"label":"%s"}`, label))
		}
	}

	for label := range oldSet {
		if blockLabelSet[label] && !newSet[label] {
			recordMRAction(db, mrID, models.ActionBlockLabelRemoved, nil, nil, nil, now,
				fmt.Sprintf(`{"label":"%s"}`, label))
		}
	}
}

func detectReleaseReadyLabelChanges(db *gorm.DB, mrID uint, repoID uint, oldLabels, newLabels []string) {
	var releaseReadyLabels []models.ReleaseReadyLabel
	db.Where("repository_id = ?", repoID).Find(&releaseReadyLabels)
	if len(releaseReadyLabels) == 0 {
		return
	}

	releaseReadyLabelSet := make(map[string]bool)
	for _, rrl := range releaseReadyLabels {
		releaseReadyLabelSet[rrl.LabelName] = true
	}

	oldSet := make(map[string]bool)
	for _, l := range oldLabels {
		oldSet[l] = true
	}
	newSet := make(map[string]bool)
	for _, l := range newLabels {
		newSet[l] = true
	}

	now := time.Now().UTC()

	for label := range newSet {
		if releaseReadyLabelSet[label] && !oldSet[label] {
			recordMRAction(db, mrID, models.ActionReleaseReadyLabelAdded, nil, nil, nil, now,
				fmt.Sprintf(`{"label":"%s"}`, label))
		}
	}
}

func buildJiraPrefixPattern(db *gorm.DB, repoID uint) *regexp.Regexp {
	var prefixes []models.JiraProjectPrefix
	db.Where("repository_id = ?", repoID).Find(&prefixes)
	if len(prefixes) == 0 {
		return nil
	}

	var prefixStrs []string
	for _, p := range prefixes {
		prefixStrs = append(prefixStrs, regexp.QuoteMeta(p.Prefix))
	}
	pattern := fmt.Sprintf(`(?i)(%s)-\d+`, strings.Join(prefixStrs, "|"))
	return regexp.MustCompile(pattern)
}

func extractJiraTaskID(jiraPattern *regexp.Regexp, branch, title string) string {
	if jiraPattern == nil {
		return ""
	}
	if match := jiraPattern.FindString(branch); match != "" {
		return match
	}
	if match := jiraPattern.FindString(title); match != "" {
		return match
	}
	return ""
}

func detectAndRecordStateChanges(db *gorm.DB, existingMR *models.MergeRequest, newMR *gitlab.BasicMergeRequest, localMRID uint) {
	now := time.Now().UTC()

	if existingMR != nil && existingMR.Draft != newMR.Draft {
		recordMRAction(db, localMRID, models.ActionDraftToggled, nil, nil, nil, now, fmt.Sprintf(`{"draft":%t}`, newMR.Draft))
	}

	if existingMR != nil && existingMR.State != "merged" && newMR.State == "merged" {
		timestamp := now
		if newMR.MergedAt != nil {
			timestamp = *newMR.MergedAt
		}
		recordMRAction(db, localMRID, models.ActionMerged, nil, nil, nil, timestamp, "")
	}

	if existingMR != nil && existingMR.State != "closed" && newMR.State == "closed" {
		timestamp := now
		if newMR.ClosedAt != nil {
			timestamp = *newMR.ClosedAt
		}
		recordMRAction(db, localMRID, models.ActionClosed, nil, nil, nil, timestamp, "")
	}
}

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
			var threadStarterID *uint
			var lastNoteID int
			var nonSystemNotes []*gitlab.Note

			for _, note := range discussion.Notes {
				if note.System {
					continue
				}
				nonSystemNotes = append(nonSystemNotes, note)

				if threadStarterID == nil && note.Resolvable {
					var starterUser models.User
					if note.Author.ID != 0 {
						starterData := models.User{
							GitlabID:  note.Author.ID,
							Username:  note.Author.Username,
							Name:      note.Author.Name,
							State:     note.Author.State,
							AvatarURL: note.Author.AvatarURL,
							WebURL:    note.Author.WebURL,
						}
						if err := db.Where(models.User{GitlabID: note.Author.ID}).Assign(starterData).FirstOrCreate(&starterUser).Error; err != nil {
							log.Printf("Error upserting thread starter GitlabID %d: %v", note.Author.ID, err)
						} else {
							threadStarterID = &starterUser.ID
						}
					}
				}
			}

			if len(nonSystemNotes) > 0 {
				lastNoteID = nonSystemNotes[len(nonSystemNotes)-1].ID
			}

			var processedCommentIDs []uint

			for _, note := range nonSystemNotes {
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

				var existingComment models.MRComment
				err := db.Where("gitlab_note_id = ?", note.ID).First(&existingComment).Error

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

				gitlabCreatedAt := time.Now()
			if note.CreatedAt != nil {
				gitlabCreatedAt = *note.CreatedAt
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
					GitlabCreatedAt:    gitlabCreatedAt,
					ThreadStarterID:    threadStarterID,
					IsLastInThread:     note.ID == lastNoteID,
				}
				if note.UpdatedAt != nil {
					comment.GitlabUpdatedAt = *note.UpdatedAt
				}

				if err == gorm.ErrRecordNotFound {
					if err := db.Create(&comment).Error; err != nil {
						log.Printf("Error creating comment for MR %d, note %d: %v", localMRID, note.ID, err)
						continue
					}
					processedCommentIDs = append(processedCommentIDs, comment.ID)
					recordMRAction(db, localMRID, models.ActionCommentAdded, &author.ID, nil, &comment.ID, gitlabCreatedAt, "")
				} else if err == nil {
					wasResolved := existingComment.Resolved
					isResolved := note.Resolved

					comment.ID = existingComment.ID
					if err := db.Model(&existingComment).Select("*").Updates(comment).Error; err != nil {
						log.Printf("Error updating comment for MR %d, note %d: %v", localMRID, note.ID, err)
						continue
					}
					processedCommentIDs = append(processedCommentIDs, comment.ID)

					if note.Resolvable && !wasResolved && isResolved && note.ResolvedAt != nil {
						recordMRAction(db, localMRID, models.ActionCommentResolved, resolvedByID, &author.ID, &comment.ID, *note.ResolvedAt, "")
					}
				}
			}

			if len(processedCommentIDs) > 0 && discussion.ID != "" {
				db.Model(&models.MRComment{}).
					Where("gitlab_discussion_id = ? AND is_last_in_thread = ? AND id NOT IN ?",
						discussion.ID, true, processedCommentIDs).
					Update("is_last_in_thread", false)
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
}

func checkAndRecordFullyApproved(db *gorm.DB, mrID uint, reviewers []models.User, approvers []models.User) {
	if len(reviewers) == 0 {
		return
	}

	approverIDs := make(map[uint]bool)
	for _, a := range approvers {
		approverIDs[a.ID] = true
	}
	for _, r := range reviewers {
		if !approverIDs[r.ID] {
			return // Not fully approved yet
		}
	}

	var existingAction models.MRAction
	err := db.Where("merge_request_id = ? AND action_type = ?", mrID, models.ActionFullyApproved).First(&existingAction).Error
	if err == nil {
		return
	}

	recordMRAction(db, mrID, models.ActionFullyApproved, nil, nil, nil, time.Now().UTC(), "")
	log.Printf("MR %d is now fully approved", mrID)
}

func syncMRApprovals(db *gorm.DB, client *gitlab.Client, projectID int, mrIID int, localMRID uint) []models.User {
	approvals, _, err := client.MergeRequests.GetMergeRequestApprovals(projectID, mrIID)
	if err != nil {
		log.Printf("Failed to fetch MR approvals for project %d MR IID %d: %v", projectID, mrIID, err)
		return nil
	}

	if approvals == nil {
		return nil
	}

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

		if !existingApproverIDs[ap.User.ID] {
			timestamp := time.Now().UTC()
			// Note: GitLab API doesn't provide ApprovedAt in approvals.ApprovedBy
			// Using current time as fallback
			recordMRAction(db, localMRID, models.ActionApproved, &u.ID, nil, nil, timestamp, "")
		}
	}

	for _, existing := range existingApprovers {
		found := false
		for _, ap := range approvals.ApprovedBy {
			if ap.User != nil && ap.User.ID == existing.GitlabID {
				found = true
				break
			}
		}
		if !found {
			recordMRAction(db, localMRID, models.ActionUnapproved, &existing.ID, nil, nil, time.Now().UTC(), "")
		}
	}

	return approverUsers
}

func syncGitLabMRToDB(db *gorm.DB, client *gitlab.Client, mr *gitlab.BasicMergeRequest, localRepositoryID uint, gitlabProjectID int, jiraPattern *regexp.Regexp) (uint, error) {
	var mrModelID uint
	var reviewersToAssociate []models.User
	now := time.Now().UTC()

	err := db.Transaction(func(tx *gorm.DB) error {
		var author models.User
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
		if err := tx.Where(models.User{GitlabID: mr.Author.ID}).Assign(authorData).FirstOrCreate(&author).Error; err != nil {
			log.Printf("Error upserting author GitlabID %d for MR %d: %v", mr.Author.ID, mr.ID, err)
			return fmt.Errorf("upserting author GitlabID %d: %w", mr.Author.ID, err)
		}

		var assignee models.User
		var assigneeID uint
		if mr.Assignee != nil {
			assigneeData := models.User{
				GitlabID:  mr.Assignee.ID,
				Username:  mr.Assignee.Username,
				Name:      mr.Assignee.Name,
				State:     mr.Assignee.State,
				CreatedAt: mr.Assignee.CreatedAt,
				AvatarURL: mr.Assignee.AvatarURL,
				WebURL:    mr.Assignee.WebURL,
			}
			if err := tx.Where(models.User{GitlabID: mr.Assignee.ID}).Assign(assigneeData).FirstOrCreate(&assignee).Error; err != nil {
				log.Printf("Error upserting assignee GitlabID %d for MR %d: %v", mr.Assignee.ID, mr.ID, err)
				return fmt.Errorf("upserting assignee GitlabID %d: %w", mr.Assignee.ID, err)
			}
			assigneeID = assignee.ID
		}

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
			JiraTaskID:                  extractJiraTaskID(jiraPattern, mr.SourceBranch, mr.Title),
		}

		if mr.References != nil {
			mrModel.References = models.IssueReferences{
				Short:    mr.References.Short,
				Relative: mr.References.Relative,
				Full:     mr.References.Full,
			}
		}

		if mr.TimeStats != nil {
			mrModel.TimeStats = models.TimeStats{
				HumanTimeEstimate:   mr.TimeStats.HumanTimeEstimate,
				HumanTotalTimeSpent: mr.TimeStats.HumanTotalTimeSpent,
				TimeEstimate:        mr.TimeStats.TimeEstimate,
				TotalTimeSpent:      mr.TimeStats.TotalTimeSpent,
			}
		}

		mrModel.LastUpdate = &now

		var existingMR models.MergeRequest
		var isNewMR bool
		err := tx.Where(models.MergeRequest{GitlabID: mrModel.GitlabID}).First(&existingMR).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			log.Printf("Error checking for existing merge request GitlabID %d: %v", mrModel.GitlabID, err)
			return fmt.Errorf("checking for existing merge request GitlabID %d: %w", mrModel.GitlabID, err)
		}
		if err == gorm.ErrRecordNotFound {
			isNewMR = true
			if err := tx.Create(&mrModel).Error; err != nil {
				log.Printf("Error creating merge request GitlabID %d: %v", mrModel.GitlabID, err)
				return fmt.Errorf("creating merge request GitlabID %d: %w", mrModel.GitlabID, err)
			}
		} else {
			detectAndRecordStateChanges(tx, &existingMR, mr, existingMR.ID)

			mrModel.ID = existingMR.ID
			if err := tx.Model(&existingMR).Select("*").Updates(mrModel).Error; err != nil {
				log.Printf("Error updating merge request GitlabID %d: %v", mrModel.GitlabID, err)
				return fmt.Errorf("updating merge request GitlabID %d: %w", mrModel.GitlabID, err)
			}
			mrModel.ID = existingMR.ID
		}

		var existingReviewerIDs map[int]bool
		if !isNewMR {
			var existingReviewers []models.User
			tx.Model(&existingMR).Association("Reviewers").Find(&existingReviewers)
			existingReviewerIDs = make(map[int]bool)
			for _, r := range existingReviewers {
				existingReviewerIDs[r.GitlabID] = true
			}
		}

		if !isNewMR {
			var existingLabels []models.Label
			tx.Model(&existingMR).Association("Labels").Find(&existingLabels)
			var oldLabelNames []string
			for _, l := range existingLabels {
				oldLabelNames = append(oldLabelNames, l.Name)
			}
			detectBlockLabelChanges(tx, existingMR.ID, localRepositoryID, oldLabelNames, mr.Labels)
			detectReleaseReadyLabelChanges(tx, existingMR.ID, localRepositoryID, oldLabelNames, mr.Labels)
		}

		var labelsToAssociate []models.Label
		for _, name := range mr.Labels {
			var lbl models.Label
			labelData := models.Label{Name: name}
			if err := tx.Where(models.Label{Name: name}).Assign(labelData).FirstOrCreate(&lbl).Error; err != nil {
				log.Printf("Error upserting label %s for MR GitlabID %d: %v", name, mr.ID, err)
				return fmt.Errorf("upserting label %s for MR GitlabID %d: %w", name, mr.ID, err)
			}
			labelsToAssociate = append(labelsToAssociate, lbl)
		}
		if err := tx.Model(&mrModel).Association("Labels").Replace(labelsToAssociate); err != nil {
			log.Printf("Error replacing labels for MR GitlabID %d: %v", mrModel.GitlabID, err)
			return fmt.Errorf("replacing labels for MR GitlabID %d: %w", mrModel.GitlabID, err)
		}

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
			if err := tx.Where(models.User{GitlabID: rv.ID}).Assign(reviewerData).FirstOrCreate(&u).Error; err != nil {
				log.Printf("Error upserting reviewer GitlabID %d for MR GitlabID %d: %v", rv.ID, mr.ID, err)
				return fmt.Errorf("upserting reviewer GitlabID %d for MR GitlabID %d: %w", rv.ID, mr.ID, err)
			}
			reviewersToAssociate = append(reviewersToAssociate, u)

			if existingReviewerIDs != nil && !existingReviewerIDs[rv.ID] {
				recordMRAction(tx, mrModel.ID, models.ActionReviewerAssigned, nil, &u.ID, nil, now, "")
			}
		}

		if !isNewMR {
			var existingReviewers []models.User
			tx.Model(&existingMR).Association("Reviewers").Find(&existingReviewers)

			newReviewerIDs := make(map[int]struct{})
			for _, rv := range mr.Reviewers {
				newReviewerIDs[rv.ID] = struct{}{}
			}

			for _, existing := range existingReviewers {
				if _, found := newReviewerIDs[existing.GitlabID]; !found {
					recordMRAction(tx, mrModel.ID, models.ActionReviewerRemoved, nil, &existing.ID, nil, now, "")
				}
			}
		}

		if err := tx.Model(&mrModel).Association("Reviewers").Replace(reviewersToAssociate); err != nil {
			log.Printf("Error replacing reviewers for MR GitlabID %d: %v", mrModel.GitlabID, err)
			return fmt.Errorf("replacing reviewers for MR GitlabID %d: %w", mrModel.GitlabID, err)
		}

		mrModelID = mrModel.ID
		return nil
	})

	if err != nil {
		return 0, err
	}

	if mr.State == "opened" || mr.State == "locked" {
		approverUsers := syncMRApprovals(db, client, gitlabProjectID, mr.IID, mrModelID)
		if approverUsers != nil {
			if err := db.Model(&models.MergeRequest{Model: gorm.Model{ID: mrModelID}}).Association("Approvers").Replace(approverUsers); err != nil {
				log.Printf("Error replacing approvers for MR GitlabID %d: %v", mr.ID, err)
				return mrModelID, fmt.Errorf("replacing approvers for MR GitlabID %d: %w", mr.ID, err)
			}
		}

		checkAndRecordFullyApproved(db, mrModelID, reviewersToAssociate, approverUsers)

		syncMRDiscussions(db, client, gitlabProjectID, mr.IID, mrModelID)
	} else {
		if err := db.Model(&models.MergeRequest{Model: gorm.Model{ID: mrModelID}}).Association("Approvers").Clear(); err != nil {
			log.Printf("Error clearing approvers for non-opened MR GitlabID %d: %v", mr.ID, err)
			return mrModelID, fmt.Errorf("clearing approvers for MR GitlabID %d: %w", mr.ID, err)
		}
	}
	return mrModelID, nil
}

func PollMergeRequests(db *gorm.DB, client *gitlab.Client) {
	var repos []models.Repository
	if err := db.Where("EXISTS (SELECT 1 FROM repository_subscriptions WHERE repository_subscriptions.repository_id = repositories.id)").
		Find(&repos).Error; err != nil {
		log.Printf("failed to fetch repositories with subscriptions: %v", err)
		return
	}
	for _, repo := range repos {
		log.Printf("Polling merge requests for repository: %s (GitLab ID: %d)", repo.Name, repo.GitlabID)
		jiraPattern := buildJiraPrefixPattern(db, repo.ID)
		allCurrentlyOpenGitlabMRs := []*gitlab.BasicMergeRequest{}
		opts := &gitlab.ListProjectMergeRequestsOptions{
			State:       gitlab.Ptr("opened"),
			ListOptions: gitlab.ListOptions{PerPage: 100, Page: 1},
		}

		for {
			mrsPage, resp, err := client.MergeRequests.ListProjectMergeRequests(repo.GitlabID, opts)
			if err != nil {
				log.Printf("Error listing merge requests for project %d page %d: %v", repo.GitlabID, opts.Page, err)
				break // Break from pagination loop for this repo on error
			}

			allCurrentlyOpenGitlabMRs = append(allCurrentlyOpenGitlabMRs, mrsPage...)

			if resp.NextPage == 0 {
				break
			}
			opts.Page = resp.NextPage
		}
		log.Printf("Fetched %d open merge requests from GitLab for repository %s", len(allCurrentlyOpenGitlabMRs), repo.Name)

		processedMRIDs := make([]uint, 0, len(allCurrentlyOpenGitlabMRs))

		for _, gitlabMR := range allCurrentlyOpenGitlabMRs {
			mrID, err := syncGitLabMRToDB(db, client, gitlabMR, repo.ID, repo.GitlabID, jiraPattern)
			if err != nil {
				log.Printf("Failed to sync open MR from GitLab API (ProjectID: %d, MR IID: %d, MR ID: %d): %v", repo.GitlabID, gitlabMR.IID, gitlabMR.ID, err)
			} else {
				processedMRIDs = append(processedMRIDs, mrID)
			}
		}

		// 3. Find and sync stale MRs (present in DB as 'opened' but not in the processed list)
		var dbOpenMRs []models.MergeRequest
		query := db.Where("repository_id = ? AND state = ?", repo.ID, "opened")

		if len(processedMRIDs) > 0 {
			query = query.Where("id NOT IN ?", processedMRIDs)
		}

		if err := query.Find(&dbOpenMRs).Error; err != nil {
			log.Printf("Error fetching 'opened' MRs from DB for repo %d: %v", repo.ID, err)
			continue // Skip to next repository
		}

		for _, dbMR := range dbOpenMRs {
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

			_, err = syncGitLabMRToDB(db, client, basicMR, repo.ID, repo.GitlabID, jiraPattern)
			if err != nil {
				log.Printf("Failed to re-sync stale MR (ProjectID: %d, MR IID: %d, MR ID: %d): %v", repo.GitlabID, fullMRDetails.IID, fullMRDetails.ID, err)
			} else {
				log.Printf("Successfully re-synced stale MR: RepoGitlabID %d, MR IID %d. New state: %s", repo.GitlabID, fullMRDetails.IID, fullMRDetails.State)
			}
		}
		log.Printf("Finished polling merge requests for repository: %s", repo.Name)
	}
}
