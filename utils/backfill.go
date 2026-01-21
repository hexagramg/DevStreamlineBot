package utils

import (
	"log"

	"devstreamlinebot/models"

	"gorm.io/gorm"
)

// BackfillThreadMetadata populates ThreadStarterID and IsLastInThread fields
// for existing MRComment records. This is a one-time migration that should run
// on startup if needed.
func BackfillThreadMetadata(db *gorm.DB) error {
	var count int64
	db.Model(&models.MRComment{}).
		Where("thread_starter_id IS NULL AND gitlab_discussion_id IS NOT NULL AND gitlab_discussion_id != ''").
		Count(&count)

	if count == 0 {
		log.Println("Thread metadata backfill: No comments need backfilling")
		return nil
	}

	log.Printf("Thread metadata backfill: Processing %d comments", count)

	type DiscussionRow struct {
		GitlabDiscussionID string `gorm:"column:gitlab_discussion_id"`
	}
	var discussions []DiscussionRow
	if err := db.Table("mr_comments").
		Select("DISTINCT gitlab_discussion_id").
		Where("gitlab_discussion_id IS NOT NULL AND gitlab_discussion_id != ''").
		Scan(&discussions).Error; err != nil {
		return err
	}

	for _, disc := range discussions {
		if err := backfillDiscussion(db, disc.GitlabDiscussionID); err != nil {
			log.Printf("Thread metadata backfill: Error processing discussion %s: %v", disc.GitlabDiscussionID, err)
		}
	}

	log.Println("Thread metadata backfill: Complete")
	return nil
}

func backfillDiscussion(db *gorm.DB, discussionID string) error {
	var comments []models.MRComment
	if err := db.Where("gitlab_discussion_id = ?", discussionID).
		Order("gitlab_created_at ASC").
		Find(&comments).Error; err != nil {
		return err
	}

	if len(comments) == 0 {
		return nil
	}

	var threadStarterID *uint
	for _, c := range comments {
		if c.Resolvable {
			threadStarterID = &c.AuthorID
			break
		}
	}

	lastCommentID := comments[len(comments)-1].ID

	for _, c := range comments {
		updates := map[string]interface{}{
			"thread_starter_id": threadStarterID,
			"is_last_in_thread": c.ID == lastCommentID,
		}
		if err := db.Model(&models.MRComment{}).Where("id = ?", c.ID).Updates(updates).Error; err != nil {
			return err
		}
	}

	return nil
}
