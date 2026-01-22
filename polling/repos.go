package polling

import (
	"log"

	"devstreamlinebot/models"

	gitlab "gitlab.com/gitlab-org/api/client-go"
	"gorm.io/gorm"
)

func PollRepositories(db *gorm.DB, client *gitlab.Client) {
	opts := &gitlab.ListProjectsOptions{
		Membership:  gitlab.Ptr(true),
		ListOptions: gitlab.ListOptions{PerPage: 100, Page: 1},
	}

	for {
		projects, resp, err := client.Projects.ListProjects(opts)
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
			var repo models.Repository
			if err := db.Where(models.Repository{GitlabID: repoData.GitlabID}).Assign(repoData).FirstOrCreate(&repo).Error; err != nil {
				log.Printf("Error upserting repository GitlabID %d: %v", repoData.GitlabID, err)
				continue
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
}
