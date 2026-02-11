package utils

import (
	"devstreamlinebot/models"

	"gorm.io/gorm"
)

// MRDataCache holds preloaded data for batch MR processing to avoid N+1 queries.
type MRDataCache struct {
	// Label caches - keyed by RepositoryID
	BlockLabels          map[uint]map[string]struct{}
	ReleaseLabels        map[uint]map[string]struct{}
	FeatureReleaseLabels map[uint]map[string]struct{}

	// SLA cache - keyed by RepositoryID
	SLAs map[uint]*models.RepositorySLA

	// Holiday cache - keyed by RepositoryID -> date string -> bool
	Holidays map[uint]map[string]bool

	// MRAction cache - keyed by MergeRequestID
	Actions map[uint][]models.MRAction

	// MRComment cache - keyed by MergeRequestID
	Comments map[uint][]models.MRComment

	// Comment cache by DiscussionID for thread timing
	CommentsByDiscussion map[string][]models.MRComment
}

// LoadMRDataCache batch loads all data needed for MR processing.
// Executes a fixed number of queries regardless of MR count.
func LoadMRDataCache(db *gorm.DB, mrIDs []uint, repoIDs []uint) (*MRDataCache, error) {
	cache := &MRDataCache{
		BlockLabels:          make(map[uint]map[string]struct{}),
		ReleaseLabels:        make(map[uint]map[string]struct{}),
		FeatureReleaseLabels: make(map[uint]map[string]struct{}),
		SLAs:                 make(map[uint]*models.RepositorySLA),
		Holidays:             make(map[uint]map[string]bool),
		Actions:              make(map[uint][]models.MRAction),
		Comments:             make(map[uint][]models.MRComment),
		CommentsByDiscussion: make(map[string][]models.MRComment),
	}

	if len(repoIDs) == 0 && len(mrIDs) == 0 {
		return cache, nil
	}

	// Load block labels for all repos
	if len(repoIDs) > 0 {
		var blockLabels []models.BlockLabel
		if err := db.Where("repository_id IN ?", repoIDs).Find(&blockLabels).Error; err != nil {
			return nil, err
		}
		for _, bl := range blockLabels {
			if cache.BlockLabels[bl.RepositoryID] == nil {
				cache.BlockLabels[bl.RepositoryID] = make(map[string]struct{})
			}
			cache.BlockLabels[bl.RepositoryID][bl.LabelName] = struct{}{}
		}

		// Load release labels for all repos
		var releaseLabels []models.ReleaseLabel
		if err := db.Where("repository_id IN ?", repoIDs).Find(&releaseLabels).Error; err != nil {
			return nil, err
		}
		for _, rl := range releaseLabels {
			if cache.ReleaseLabels[rl.RepositoryID] == nil {
				cache.ReleaseLabels[rl.RepositoryID] = make(map[string]struct{})
			}
			cache.ReleaseLabels[rl.RepositoryID][rl.LabelName] = struct{}{}
		}

		// Load feature release labels for all repos
		var featureReleaseLabels []models.FeatureReleaseLabel
		if err := db.Where("repository_id IN ?", repoIDs).Find(&featureReleaseLabels).Error; err != nil {
			return nil, err
		}
		for _, frl := range featureReleaseLabels {
			if cache.FeatureReleaseLabels[frl.RepositoryID] == nil {
				cache.FeatureReleaseLabels[frl.RepositoryID] = make(map[string]struct{})
			}
			cache.FeatureReleaseLabels[frl.RepositoryID][frl.LabelName] = struct{}{}
		}

		// Load SLAs for all repos
		var slas []models.RepositorySLA
		if err := db.Where("repository_id IN ?", repoIDs).Find(&slas).Error; err != nil {
			return nil, err
		}
		for i := range slas {
			cache.SLAs[slas[i].RepositoryID] = &slas[i]
		}

		// Load holidays for all repos
		var holidays []models.Holiday
		if err := db.Where("repository_id IN ?", repoIDs).Find(&holidays).Error; err != nil {
			return nil, err
		}
		for _, h := range holidays {
			if cache.Holidays[h.RepositoryID] == nil {
				cache.Holidays[h.RepositoryID] = make(map[string]bool)
			}
			cache.Holidays[h.RepositoryID][h.Date.Format("2006-01-02")] = true
		}
	}

	// Load actions for all MRs
	if len(mrIDs) > 0 {
		var actions []models.MRAction
		if err := db.Where("merge_request_id IN ?", mrIDs).
			Order("timestamp ASC").
			Find(&actions).Error; err != nil {
			return nil, err
		}
		for _, a := range actions {
			cache.Actions[a.MergeRequestID] = append(cache.Actions[a.MergeRequestID], a)
		}

		// Load comments for all MRs
		var comments []models.MRComment
		if err := db.Where("merge_request_id IN ?", mrIDs).
			Order("gitlab_created_at ASC").
			Find(&comments).Error; err != nil {
			return nil, err
		}
		for _, c := range comments {
			cache.Comments[c.MergeRequestID] = append(cache.Comments[c.MergeRequestID], c)
			cache.CommentsByDiscussion[c.GitlabDiscussionID] = append(cache.CommentsByDiscussion[c.GitlabDiscussionID], c)
		}
	}

	return cache, nil
}

// GetSLAFromCache returns the SLA for a repository, with default fallback.
func (c *MRDataCache) GetSLAFromCache(repoID uint) *models.RepositorySLA {
	if sla, ok := c.SLAs[repoID]; ok {
		return sla
	}
	return &models.RepositorySLA{
		RepositoryID:   repoID,
		ReviewDuration: DefaultSLADuration,
		FixesDuration:  DefaultSLADuration,
		AssignCount:    1,
	}
}

// HasReleaseLabelFromCache checks if MR has a release or feature release label using cached data.
func (c *MRDataCache) HasReleaseLabelFromCache(labels []models.Label, repoID uint) bool {
	if len(labels) == 0 {
		return false
	}
	releaseLabels := c.ReleaseLabels[repoID]
	featureReleaseLabels := c.FeatureReleaseLabels[repoID]
	if len(releaseLabels) == 0 && len(featureReleaseLabels) == 0 {
		return false
	}
	for _, l := range labels {
		if _, ok := releaseLabels[l.Name]; ok {
			return true
		}
		if _, ok := featureReleaseLabels[l.Name]; ok {
			return true
		}
	}
	return false
}

// IsMRBlockedFromCache checks if MR is blocked using cached data.
func (c *MRDataCache) IsMRBlockedFromCache(labels []models.Label, repoID uint) bool {
	blockLabels := c.BlockLabels[repoID]
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

// collectUniqueIDs extracts unique MR IDs and repo IDs from a slice of MRs.
func CollectUniqueIDs(mrs []models.MergeRequest) (mrIDs []uint, repoIDs []uint) {
	mrIDSet := make(map[uint]struct{})
	repoIDSet := make(map[uint]struct{})

	for _, mr := range mrs {
		mrIDSet[mr.ID] = struct{}{}
		repoIDSet[mr.RepositoryID] = struct{}{}
	}

	mrIDs = make([]uint, 0, len(mrIDSet))
	for id := range mrIDSet {
		mrIDs = append(mrIDs, id)
	}

	repoIDs = make([]uint, 0, len(repoIDSet))
	for id := range repoIDSet {
		repoIDs = append(repoIDs, id)
	}

	return mrIDs, repoIDs
}
