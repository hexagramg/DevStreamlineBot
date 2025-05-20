package polling

import (
	"log"

	"devstreamlinebot/models"

	gitlab "gitlab.com/gitlab-org/api/client-go"
	"gorm.io/gorm"
)

// PollRepositories loops to sync GitLab projects into the database.
func PollRepositories(db *gorm.DB, client *gitlab.Client) {
	projects, _, err := client.Projects.ListProjects(&gitlab.ListProjectsOptions{
		Membership:  gitlab.Ptr(true),
		ListOptions: gitlab.ListOptions{PerPage: 100, Page: 1},
	})
	if err != nil {
		log.Printf("error updating repositories: %v", err)
		return
	}
	for _, p := range projects {
		repoData := models.Repository{
			GitlabID:    p.ID,
			Name:        p.Name,
			Description: p.Description,
			WebURL:      p.WebURL,
		}
		// Upsert the repository:
		// Find by GitlabID. If exists, update with new data from repoData.
		// If not exists, create with data from repoData.
		// The result (including database ID) will be stored back in a new repo variable if needed,
		// but here we just care about the upsert itself.
		var repo models.Repository
		if err := db.Where(models.Repository{GitlabID: repoData.GitlabID}).Assign(repoData).FirstOrCreate(&repo).Error; err != nil {
			log.Printf("Error upserting repository GitlabID %d: %v", repoData.GitlabID, err)
			// Continue to the next repository if this one fails
			continue
		}
	}
}
