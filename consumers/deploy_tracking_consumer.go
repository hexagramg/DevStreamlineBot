package consumers

import (
	"errors"
	"fmt"
	"log"

	botgolang "github.com/mail-ru-im/bot-golang"
	gitlab "gitlab.com/gitlab-org/api/client-go"
	"gorm.io/gorm"

	"devstreamlinebot/interfaces"
	"devstreamlinebot/models"
)

// DeployTrackingConsumer polls GitLab for deploy jobs matching tracking rules
// and sends notifications to ReleaseSubscription chats.
type DeployTrackingConsumer struct {
	db         *gorm.DB
	vkBot      interfaces.VKBot
	jobService interfaces.GitLabJobsService
}

func NewDeployTrackingConsumer(db *gorm.DB, vkBot *botgolang.Bot, glClient *gitlab.Client) *DeployTrackingConsumer {
	return &DeployTrackingConsumer{
		db:         db,
		vkBot:      &interfaces.RealVKBot{Bot: vkBot},
		jobService: glClient.Jobs,
	}
}

func NewDeployTrackingConsumerWithDeps(db *gorm.DB, vkBot interfaces.VKBot, jobService interfaces.GitLabJobsService) *DeployTrackingConsumer {
	return &DeployTrackingConsumer{
		db:         db,
		vkBot:      vkBot,
		jobService: jobService,
	}
}

// PollDeployJobs checks for new/changed deploy jobs and sends notifications.
func (c *DeployTrackingConsumer) PollDeployJobs() {
	var rules []models.DeployTrackingRule
	if err := c.db.Preload("TargetRepository").Find(&rules).Error; err != nil {
		log.Printf("failed to fetch deploy tracking rules: %v", err)
		return
	}
	if len(rules) == 0 {
		return
	}

	// Group rules by deploy project to minimize API calls
	rulesByProject := make(map[int][]models.DeployTrackingRule)
	for _, rule := range rules {
		rulesByProject[rule.DeployProjectID] = append(rulesByProject[rule.DeployProjectID], rule)
	}

	for deployProjectID, projectRules := range rulesByProject {
		c.pollProjectJobs(deployProjectID, projectRules)
	}
}

func (c *DeployTrackingConsumer) pollProjectJobs(deployProjectID int, rules []models.DeployTrackingRule) {
	// Build set of job names we care about
	jobNames := make(map[string]bool)
	for _, rule := range rules {
		jobNames[rule.JobName] = true
	}

	scopes := []gitlab.BuildStateValue{gitlab.Running, gitlab.Success, gitlab.Failed, gitlab.Canceled}
	opts := &gitlab.ListJobsOptions{
		Scope:       &scopes,
		ListOptions: gitlab.ListOptions{PerPage: 50, Page: 1},
	}

	jobs, _, err := c.jobService.ListProjectJobs(deployProjectID, opts)
	if err != nil {
		log.Printf("failed to list jobs for deploy project %d: %v", deployProjectID, err)
		return
	}

	for _, job := range jobs {
		if !jobNames[job.Name] {
			continue
		}
		c.processJob(job, rules)
	}
}

func (c *DeployTrackingConsumer) processJob(job *gitlab.Job, rules []models.DeployTrackingRule) {
	for _, rule := range rules {
		if rule.JobName != job.Name {
			continue
		}

		// Skip jobs created before the tracking rule
		if job.CreatedAt != nil && job.CreatedAt.Before(rule.CreatedAt) {
			continue
		}

		var tracked models.TrackedDeployJob
		err := c.db.Where("gitlab_job_id = ? AND deploy_tracking_rule_id = ?", job.ID, rule.ID).First(&tracked).Error

		if errors.Is(err, gorm.ErrRecordNotFound) {
			triggeredBy := ""
			if job.User != nil {
				triggeredBy = job.User.Username
			}
			tracked = models.TrackedDeployJob{
				DeployTrackingRuleID: rule.ID,
				GitlabJobID:         job.ID,
				Status:              job.Status,
				Ref:                 job.Ref,
				TriggeredBy:         triggeredBy,
				WebURL:              job.WebURL,
				StartedAt:           job.StartedAt,
				FinishedAt:          job.FinishedAt,
			}
			if err := c.db.Create(&tracked).Error; err != nil {
				log.Printf("failed to create tracked deploy job %d: %v", job.ID, err)
				continue
			}
		} else if err != nil {
			log.Printf("failed to query tracked deploy job %d: %v", job.ID, err)
			continue
		} else if tracked.Status != job.Status {
			tracked.Status = job.Status
			tracked.FinishedAt = job.FinishedAt
			if err := c.db.Save(&tracked).Error; err != nil {
				log.Printf("failed to update tracked deploy job %d: %v", job.ID, err)
				continue
			}
		}

		c.notifyIfNeeded(&tracked, rule)
	}
}

func (c *DeployTrackingConsumer) notifyIfNeeded(tracked *models.TrackedDeployJob, rule models.DeployTrackingRule) {
	repoName := rule.TargetRepository.Name

	if tracked.Status == "running" && !tracked.NotifiedRunning {
		message := fmt.Sprintf("Начат деплой %s\nВетка: %s\nЗапустил: %s\n%s",
			repoName, tracked.Ref, tracked.TriggeredBy, tracked.WebURL)
		c.sendToReleaseSubscribers(rule.TargetRepositoryID, message)
		c.db.Model(tracked).Update("notified_running", true)
	}

	isTerminal := tracked.Status == "success" || tracked.Status == "failed" || tracked.Status == "canceled"
	if isTerminal && !tracked.NotifiedFinished {
		var message string
		switch tracked.Status {
		case "success":
			message = fmt.Sprintf("Деплой %s завершён успешно\nВетка: %s\nЗапустил: %s\n%s",
				repoName, tracked.Ref, tracked.TriggeredBy, tracked.WebURL)
		case "failed":
			message = fmt.Sprintf("Деплой %s провален\nВетка: %s\nЗапустил: %s\n%s",
				repoName, tracked.Ref, tracked.TriggeredBy, tracked.WebURL)
		case "canceled":
			message = fmt.Sprintf("Деплой %s отменён\nВетка: %s\nЗапустил: %s\n%s",
				repoName, tracked.Ref, tracked.TriggeredBy, tracked.WebURL)
		}
		c.sendToReleaseSubscribers(rule.TargetRepositoryID, message)
		c.db.Model(tracked).Update("notified_finished", true)
	}
}

func (c *DeployTrackingConsumer) sendToReleaseSubscribers(targetRepoID uint, message string) {
	var subs []models.ReleaseSubscription
	if err := c.db.Where("repository_id = ?", targetRepoID).Preload("Chat").Find(&subs).Error; err != nil {
		log.Printf("failed to fetch release subscriptions for deploy notification: %v", err)
		return
	}
	for _, sub := range subs {
		msg := c.vkBot.NewTextMessage(sub.Chat.ChatID, message)
		if err := msg.Send(); err != nil {
			log.Printf("failed to send deploy notification to chat %s: %v", sub.Chat.ChatID, err)
		}
	}
}
