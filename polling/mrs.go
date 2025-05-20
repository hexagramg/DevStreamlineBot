package polling

import (
	"fmt"
	"log"
	"time"

	"devstreamlinebot/models"

	gitlab "gitlab.com/gitlab-org/api/client-go"
	"gorm.io/gorm"
)

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
	err := db.Where(models.MergeRequest{GitlabID: mrModel.GitlabID}).First(&existingMR).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		log.Printf("Error checking for existing merge request GitlabID %d: %v", mrModel.GitlabID, err)
		return 0, fmt.Errorf("checking for existing merge request GitlabID %d: %w", mrModel.GitlabID, err)
	}
	if err == gorm.ErrRecordNotFound {
		// Not found, create new
		if err := db.Create(&mrModel).Error; err != nil {
			log.Printf("Error creating merge request GitlabID %d: %v", mrModel.GitlabID, err)
			return 0, fmt.Errorf("creating merge request GitlabID %d: %w", mrModel.GitlabID, err)
		}
	} else {
		// Exists, update all fields
		mrModel.ID = existingMR.ID // ensure correct primary key
		if err := db.Model(&existingMR).Select("*").Updates(mrModel).Error; err != nil {
			log.Printf("Error updating merge request GitlabID %d: %v", mrModel.GitlabID, err)
			return 0, fmt.Errorf("updating merge request GitlabID %d: %w", mrModel.GitlabID, err)
		}
		// Keep mrModel.ID in sync for associations
		mrModel.ID = existingMR.ID
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
	}
	if err := db.Model(&mrModel).Association("Reviewers").Replace(reviewersToAssociate); err != nil {
		log.Printf("Error replacing reviewers for MR GitlabID %d: %v", mrModel.GitlabID, err)
		return 0, fmt.Errorf("replacing reviewers for MR GitlabID %d: %w", mrModel.GitlabID, err)
	}

	// Sync approvers
	// Fetch approvals only if MR state suggests it's active (e.g., "opened", "locked")
	if mr.State == "opened" || mr.State == "locked" {
		approvals, _, err := client.MergeRequests.GetMergeRequestApprovals(gitlabProjectID, mr.IID)
		if err != nil {
			log.Printf("Failed to fetch MR approvals for project %d MR IID %d: %v", gitlabProjectID, mr.IID, err)
		} else if approvals != nil {
			var approverUsersToAssociate []models.User
			for _, ap := range approvals.ApprovedBy { // ap.User is *gitlab.BasicUser
				if ap.User == nil {
					continue
				}
				var u models.User
				approverData := models.User{ // BasicUser does not have CreatedAt
					GitlabID:  ap.User.ID,
					Username:  ap.User.Username,
					Name:      ap.User.Name,
					State:     ap.User.State,
					AvatarURL: ap.User.AvatarURL,
					WebURL:    ap.User.WebURL,
				}
				if err := db.Where(models.User{GitlabID: ap.User.ID}).Assign(approverData).FirstOrCreate(&u).Error; err != nil {
					log.Printf("Error upserting approver GitlabID %d for MR GitlabID %d: %v", ap.User.ID, mr.ID, err)
					continue // Skip this approver
				}
				approverUsersToAssociate = append(approverUsersToAssociate, u)
			}
			if err := db.Model(&mrModel).Association("Approvers").Replace(approverUsersToAssociate); err != nil {
				log.Printf("Error replacing approvers for MR GitlabID %d: %v", mrModel.GitlabID, err)
				return 0, fmt.Errorf("replacing approvers for MR GitlabID %d: %w", mrModel.GitlabID, err)
			}
		}
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
