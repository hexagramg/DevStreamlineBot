package interfaces

import (
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// GitLabMergeRequestsService abstracts GitLab MR operations for testing.
type GitLabMergeRequestsService interface {
	UpdateMergeRequest(pid interface{}, mergeRequest int, opt *gitlab.UpdateMergeRequestOptions, options ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error)
	GetMergeRequestApprovals(pid interface{}, mergeRequest int, options ...gitlab.RequestOptionFunc) (*gitlab.MergeRequestApprovals, *gitlab.Response, error)
	ListProjectMergeRequests(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error)
	GetMergeRequest(pid interface{}, mergeRequest int, opt *gitlab.GetMergeRequestsOptions, options ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error)
	CreateMergeRequest(pid interface{}, opt *gitlab.CreateMergeRequestOptions, options ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error)
	GetMergeRequestCommits(pid interface{}, mergeRequest int, opt *gitlab.GetMergeRequestCommitsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.Commit, *gitlab.Response, error)
}

// GitLabDiscussionsService abstracts GitLab discussion operations for testing.
type GitLabDiscussionsService interface {
	ListMergeRequestDiscussions(pid interface{}, mergeRequest int, opt *gitlab.ListMergeRequestDiscussionsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.Discussion, *gitlab.Response, error)
}

// GitLabUsersService abstracts GitLab user operations for testing.
type GitLabUsersService interface {
	ListUsers(opt *gitlab.ListUsersOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.User, *gitlab.Response, error)
	GetUser(user int, opt gitlab.GetUsersOptions, options ...gitlab.RequestOptionFunc) (*gitlab.User, *gitlab.Response, error)
}

// GitLabLabelsService abstracts GitLab label operations for testing.
type GitLabLabelsService interface {
	ListLabels(pid interface{}, opt *gitlab.ListLabelsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.Label, *gitlab.Response, error)
	CreateLabel(pid interface{}, opt *gitlab.CreateLabelOptions, options ...gitlab.RequestOptionFunc) (*gitlab.Label, *gitlab.Response, error)
}

// GitLabBranchesService abstracts GitLab branch operations for testing.
type GitLabBranchesService interface {
	GetBranch(pid interface{}, branch string, options ...gitlab.RequestOptionFunc) (*gitlab.Branch, *gitlab.Response, error)
	CreateBranch(pid interface{}, opt *gitlab.CreateBranchOptions, options ...gitlab.RequestOptionFunc) (*gitlab.Branch, *gitlab.Response, error)
}
