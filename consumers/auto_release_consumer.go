package consumers

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go"
	"gorm.io/gorm"

	"devstreamlinebot/interfaces"
	"devstreamlinebot/models"
)

type AutoReleaseConsumer struct {
	db        *gorm.DB
	mrService interfaces.GitLabMergeRequestsService
	brService interfaces.GitLabBranchesService
}

func NewAutoReleaseConsumer(db *gorm.DB, glClient *gitlab.Client) *AutoReleaseConsumer {
	return &AutoReleaseConsumer{
		db:        db,
		mrService: glClient.MergeRequests,
		brService: glClient.Branches,
	}
}

func NewAutoReleaseConsumerWithServices(
	db *gorm.DB,
	mrService interfaces.GitLabMergeRequestsService,
	brService interfaces.GitLabBranchesService,
) *AutoReleaseConsumer {
	return &AutoReleaseConsumer{
		db:        db,
		mrService: mrService,
		brService: brService,
	}
}

// ProcessAutoReleaseBranches handles release branch creation and MR retargeting.
func (c *AutoReleaseConsumer) ProcessAutoReleaseBranches() {
	var configs []models.AutoReleaseBranchConfig
	if err := c.db.Preload("Repository").Find(&configs).Error; err != nil {
		log.Printf("failed to fetch auto-release configs: %v", err)
		return
	}

	for _, config := range configs {
		c.processRepoReleaseBranch(config)
	}
}

func (c *AutoReleaseConsumer) processRepoReleaseBranch(config models.AutoReleaseBranchConfig) {
	repo := config.Repository

	var releaseLabel models.ReleaseLabel
	if err := c.db.Where("repository_id = ?", config.RepositoryID).First(&releaseLabel).Error; err != nil {
		log.Printf("No release label for repo %d, skipping auto-release", config.RepositoryID)
		return
	}

	openReleaseMR := c.findOpenReleaseMR(repo.GitlabID, releaseLabel.LabelName)

	var currentReleaseBranch string

	if openReleaseMR == nil {
		branch, err := c.createReleaseBranch(repo.GitlabID, config.ReleaseBranchPrefix, config.DevBranchName)
		if err != nil {
			log.Printf("Failed to create release branch for repo %d: %v", repo.GitlabID, err)
			return
		}
		currentReleaseBranch = branch

		if err := c.createReleaseMR(repo.GitlabID, branch, config.DevBranchName, releaseLabel.LabelName); err != nil {
			log.Printf("Failed to create release MR for repo %d: %v", repo.GitlabID, err)
			return
		}

		log.Printf("Created release branch %s and MR for repo %s", branch, repo.Name)
	} else {
		currentReleaseBranch = openReleaseMR.SourceBranch
	}

	var blockLabels []models.BlockLabel
	c.db.Where("repository_id = ?", config.RepositoryID).Find(&blockLabels)
	blockLabelNames := make(map[string]bool)
	for _, bl := range blockLabels {
		blockLabelNames[bl.LabelName] = true
	}

	c.retargetMRsToReleaseBranch(
		repo.GitlabID,
		config.DevBranchName,
		currentReleaseBranch,
		releaseLabel.LabelName,
		blockLabelNames,
	)
}

func (c *AutoReleaseConsumer) findOpenReleaseMR(projectID int, releaseLabel string) *gitlab.BasicMergeRequest {
	opts := &gitlab.ListProjectMergeRequestsOptions{
		State:       gitlab.Ptr("opened"),
		Labels:      &gitlab.LabelOptions{releaseLabel},
		ListOptions: gitlab.ListOptions{PerPage: 10, Page: 1},
	}

	mrs, _, err := c.mrService.ListProjectMergeRequests(projectID, opts)
	if err != nil {
		log.Printf("Error checking for release MRs in project %d: %v", projectID, err)
		return nil
	}

	if len(mrs) > 0 {
		return mrs[0]
	}
	return nil
}

func (c *AutoReleaseConsumer) createReleaseBranch(projectID int, prefix, devBranch string) (string, error) {
	branch, _, err := c.brService.GetBranch(projectID, devBranch)
	if err != nil {
		return "", fmt.Errorf("failed to get dev branch %s: %w", devBranch, err)
	}

	sha := branch.Commit.ID
	shortSHA := sha
	if len(sha) > 6 {
		shortSHA = sha[:6]
	}

	branchName := fmt.Sprintf("%s_%s_%s",
		prefix,
		time.Now().Format("2006-01-02"),
		shortSHA,
	)

	_, _, err = c.brService.CreateBranch(projectID, &gitlab.CreateBranchOptions{
		Branch: gitlab.Ptr(branchName),
		Ref:    gitlab.Ptr(devBranch),
	})
	if err != nil {
		return "", fmt.Errorf("failed to create branch %s: %w", branchName, err)
	}

	return branchName, nil
}

func (c *AutoReleaseConsumer) createReleaseMR(projectID int, sourceBranch, targetBranch, releaseLabel string) error {
	title := fmt.Sprintf("Release %s", time.Now().Format("2006-01-02"))

	_, _, err := c.mrService.CreateMergeRequest(projectID, &gitlab.CreateMergeRequestOptions{
		SourceBranch: gitlab.Ptr(sourceBranch),
		TargetBranch: gitlab.Ptr(targetBranch),
		Title:        gitlab.Ptr(title),
		Labels:       &gitlab.LabelOptions{releaseLabel},
	})
	if err != nil {
		return fmt.Errorf("failed to create release MR: %w", err)
	}

	return nil
}

func (c *AutoReleaseConsumer) retargetMRsToReleaseBranch(
	projectID int,
	devBranch string,
	releaseBranch string,
	releaseLabel string,
	blockLabelNames map[string]bool,
) {
	opts := &gitlab.ListProjectMergeRequestsOptions{
		State:        gitlab.Ptr("opened"),
		TargetBranch: gitlab.Ptr(devBranch),
		ListOptions:  gitlab.ListOptions{PerPage: 100, Page: 1},
	}

	for {
		mrs, resp, err := c.mrService.ListProjectMergeRequests(projectID, opts)
		if err != nil {
			log.Printf("Error listing MRs for retargeting in project %d: %v", projectID, err)
			return
		}

		for _, mr := range mrs {
			if hasLabel(mr.Labels, releaseLabel) {
				continue
			}

			if hasAnyLabel(mr.Labels, blockLabelNames) {
				log.Printf("Skipping MR !%d (has block label)", mr.IID)
				continue
			}

			_, _, err := c.mrService.UpdateMergeRequest(projectID, mr.IID,
				&gitlab.UpdateMergeRequestOptions{
					TargetBranch: gitlab.Ptr(releaseBranch),
				})
			if err != nil {
				log.Printf("Failed to retarget MR !%d to %s: %v", mr.IID, releaseBranch, err)
				continue
			}
			log.Printf("Retargeted MR !%d to release branch %s", mr.IID, releaseBranch)
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
}

// ProcessReleaseMRDescriptions handles updating release MR descriptions with included MRs.
func (c *AutoReleaseConsumer) ProcessReleaseMRDescriptions() {
	var configs []models.AutoReleaseBranchConfig
	if err := c.db.Preload("Repository").Find(&configs).Error; err != nil {
		log.Printf("failed to fetch auto-release configs: %v", err)
		return
	}

	for _, config := range configs {
		c.updateReleaseMRDescription(config)
	}
}

func (c *AutoReleaseConsumer) updateReleaseMRDescription(config models.AutoReleaseBranchConfig) {
	repo := config.Repository

	var releaseLabel models.ReleaseLabel
	if err := c.db.Where("repository_id = ?", config.RepositoryID).First(&releaseLabel).Error; err != nil {
		return
	}

	releaseMR := c.findOpenReleaseMR(repo.GitlabID, releaseLabel.LabelName)
	if releaseMR == nil {
		return
	}

	commits, err := c.getMergeRequestCommits(repo.GitlabID, releaseMR.IID)
	if err != nil {
		log.Printf("Failed to get commits for release MR !%d: %v", releaseMR.IID, err)
		return
	}

	includedMRs := c.extractIncludedMRs(commits, repo.GitlabID)
	if len(includedMRs) == 0 {
		return
	}

	fullMR, _, err := c.mrService.GetMergeRequest(repo.GitlabID, releaseMR.IID, nil)
	if err != nil {
		log.Printf("Failed to get full MR details for !%d: %v", releaseMR.IID, err)
		return
	}

	newDescription := c.buildReleaseMRDescription(fullMR.Description, includedMRs)
	if newDescription == fullMR.Description {
		return
	}

	_, _, err = c.mrService.UpdateMergeRequest(repo.GitlabID, releaseMR.IID,
		&gitlab.UpdateMergeRequestOptions{
			Description: gitlab.Ptr(newDescription),
		})
	if err != nil {
		log.Printf("Failed to update release MR !%d description: %v", releaseMR.IID, err)
		return
	}

	log.Printf("Updated release MR !%d description with %d included MRs", releaseMR.IID, len(includedMRs))
}

type includedMR struct {
	IID    int
	Title  string
	URL    string
	Author string
}

func (c *AutoReleaseConsumer) getMergeRequestCommits(projectID, mrIID int) ([]*gitlab.Commit, error) {
	var allCommits []*gitlab.Commit
	opts := &gitlab.GetMergeRequestCommitsOptions{
		PerPage: 100,
		Page:    1,
	}

	for {
		commits, resp, err := c.mrService.GetMergeRequestCommits(projectID, mrIID, opts)
		if err != nil {
			return nil, err
		}
		allCommits = append(allCommits, commits...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allCommits, nil
}

func (c *AutoReleaseConsumer) extractIncludedMRs(commits []*gitlab.Commit, projectID int) []includedMR {
	mrRefRegex := regexp.MustCompile(`See merge request [^\s!]+!(\d+)`)

	var included []includedMR
	seenIIDs := make(map[int]bool)

	for _, commit := range commits {
		matches := mrRefRegex.FindStringSubmatch(commit.Message)
		if len(matches) < 2 {
			continue
		}

		iid := 0
		fmt.Sscanf(matches[1], "%d", &iid)
		if iid == 0 || seenIIDs[iid] {
			continue
		}
		seenIIDs[iid] = true

		mr, _, err := c.mrService.GetMergeRequest(projectID, iid, nil)
		if err != nil {
			log.Printf("Failed to fetch MR !%d details: %v", iid, err)
			continue
		}

		authorUsername := ""
		if mr.Author != nil {
			authorUsername = mr.Author.Username
		}

		included = append(included, includedMR{
			IID:    iid,
			Title:  mr.Title,
			URL:    mr.WebURL,
			Author: authorUsername,
		})
	}

	return included
}

func (c *AutoReleaseConsumer) buildReleaseMRDescription(currentDesc string, includedMRs []includedMR) string {
	const sectionHeader = "---\n## Included MRs"

	existingIIDs := make(map[int]bool)
	if idx := strings.Index(currentDesc, sectionHeader); idx != -1 {
		existingSection := currentDesc[idx:]
		iidRegex := regexp.MustCompile(`\[!(\d+)`)
		matches := iidRegex.FindAllStringSubmatch(existingSection, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				iid := 0
				fmt.Sscanf(match[1], "%d", &iid)
				if iid > 0 {
					existingIIDs[iid] = true
				}
			}
		}
	}

	var newEntries []string
	for _, mr := range includedMRs {
		if existingIIDs[mr.IID] {
			continue
		}
		entry := fmt.Sprintf("- [!%d %s](%s) by @%s", mr.IID, mr.Title, mr.URL, mr.Author)
		newEntries = append(newEntries, entry)
	}

	if len(newEntries) == 0 {
		return currentDesc
	}

	if strings.Contains(currentDesc, sectionHeader) {
		return currentDesc + "\n" + strings.Join(newEntries, "\n")
	}

	newSection := sectionHeader + "\n" + strings.Join(newEntries, "\n")
	if currentDesc != "" {
		return currentDesc + "\n\n" + newSection
	}
	return newSection
}

func hasLabel(labels gitlab.Labels, target string) bool {
	for _, l := range labels {
		if l == target {
			return true
		}
	}
	return false
}

func hasAnyLabel(labels gitlab.Labels, targets map[string]bool) bool {
	for _, l := range labels {
		if targets[l] {
			return true
		}
	}
	return false
}
