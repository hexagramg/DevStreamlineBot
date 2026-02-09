package mocks

import (
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// MockMergeRequestsService is a mock implementation of GitLabMergeRequestsService.
type MockMergeRequestsService struct {
	UpdateMergeRequestFunc         func(pid interface{}, mergeRequest int, opt *gitlab.UpdateMergeRequestOptions, options ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error)
	GetMergeRequestApprovalsFunc   func(pid interface{}, mergeRequest int, options ...gitlab.RequestOptionFunc) (*gitlab.MergeRequestApprovals, *gitlab.Response, error)
	ListProjectMergeRequestsFunc   func(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error)
	GetMergeRequestFunc            func(pid interface{}, mergeRequest int, opt *gitlab.GetMergeRequestsOptions, options ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error)
	CreateMergeRequestFunc         func(pid interface{}, opt *gitlab.CreateMergeRequestOptions, options ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error)
	GetMergeRequestCommitsFunc     func(pid interface{}, mergeRequest int, opt *gitlab.GetMergeRequestCommitsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.Commit, *gitlab.Response, error)

	// Call tracking
	UpdateMergeRequestCalls       []UpdateMergeRequestCall
	GetMergeRequestApprovalsCalls []GetMergeRequestApprovalsCall
	ListProjectMergeRequestsCalls []ListProjectMergeRequestsCall
	GetMergeRequestCalls          []GetMergeRequestCall
	CreateMergeRequestCalls       []CreateMergeRequestCall
	GetMergeRequestCommitsCalls   []GetMergeRequestCommitsCall
}

// UpdateMergeRequestCall tracks a call to UpdateMergeRequest.
type UpdateMergeRequestCall struct {
	PID          interface{}
	MergeRequest int
	Opt          *gitlab.UpdateMergeRequestOptions
}

// GetMergeRequestApprovalsCall tracks a call to GetMergeRequestApprovals.
type GetMergeRequestApprovalsCall struct {
	PID          interface{}
	MergeRequest int
}

// ListProjectMergeRequestsCall tracks a call to ListProjectMergeRequests.
type ListProjectMergeRequestsCall struct {
	PID interface{}
	Opt *gitlab.ListProjectMergeRequestsOptions
}

// GetMergeRequestCall tracks a call to GetMergeRequest.
type GetMergeRequestCall struct {
	PID          interface{}
	MergeRequest int
	Opt          *gitlab.GetMergeRequestsOptions
}

// CreateMergeRequestCall tracks a call to CreateMergeRequest.
type CreateMergeRequestCall struct {
	PID interface{}
	Opt *gitlab.CreateMergeRequestOptions
}

// GetMergeRequestCommitsCall tracks a call to GetMergeRequestCommits.
type GetMergeRequestCommitsCall struct {
	PID          interface{}
	MergeRequest int
	Opt          *gitlab.GetMergeRequestCommitsOptions
}

// UpdateMergeRequest implements the interface method.
func (m *MockMergeRequestsService) UpdateMergeRequest(pid interface{}, mergeRequest int, opt *gitlab.UpdateMergeRequestOptions, options ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
	m.UpdateMergeRequestCalls = append(m.UpdateMergeRequestCalls, UpdateMergeRequestCall{
		PID:          pid,
		MergeRequest: mergeRequest,
		Opt:          opt,
	})
	if m.UpdateMergeRequestFunc != nil {
		return m.UpdateMergeRequestFunc(pid, mergeRequest, opt, options...)
	}
	return nil, NewMockResponse(0), nil
}

// GetMergeRequestApprovals implements the interface method.
func (m *MockMergeRequestsService) GetMergeRequestApprovals(pid interface{}, mergeRequest int, options ...gitlab.RequestOptionFunc) (*gitlab.MergeRequestApprovals, *gitlab.Response, error) {
	m.GetMergeRequestApprovalsCalls = append(m.GetMergeRequestApprovalsCalls, GetMergeRequestApprovalsCall{
		PID:          pid,
		MergeRequest: mergeRequest,
	})
	if m.GetMergeRequestApprovalsFunc != nil {
		return m.GetMergeRequestApprovalsFunc(pid, mergeRequest, options...)
	}
	return &gitlab.MergeRequestApprovals{}, NewMockResponse(0), nil
}

// ListProjectMergeRequests implements the interface method.
func (m *MockMergeRequestsService) ListProjectMergeRequests(pid interface{}, opt *gitlab.ListProjectMergeRequestsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.BasicMergeRequest, *gitlab.Response, error) {
	m.ListProjectMergeRequestsCalls = append(m.ListProjectMergeRequestsCalls, ListProjectMergeRequestsCall{
		PID: pid,
		Opt: opt,
	})
	if m.ListProjectMergeRequestsFunc != nil {
		return m.ListProjectMergeRequestsFunc(pid, opt, options...)
	}
	return nil, NewMockResponse(0), nil
}

// GetMergeRequest implements the interface method.
func (m *MockMergeRequestsService) GetMergeRequest(pid interface{}, mergeRequest int, opt *gitlab.GetMergeRequestsOptions, options ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
	m.GetMergeRequestCalls = append(m.GetMergeRequestCalls, GetMergeRequestCall{
		PID:          pid,
		MergeRequest: mergeRequest,
		Opt:          opt,
	})
	if m.GetMergeRequestFunc != nil {
		return m.GetMergeRequestFunc(pid, mergeRequest, opt, options...)
	}
	return nil, NewMockResponse(0), nil
}

// CreateMergeRequest implements the interface method.
func (m *MockMergeRequestsService) CreateMergeRequest(pid interface{}, opt *gitlab.CreateMergeRequestOptions, options ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
	m.CreateMergeRequestCalls = append(m.CreateMergeRequestCalls, CreateMergeRequestCall{
		PID: pid,
		Opt: opt,
	})
	if m.CreateMergeRequestFunc != nil {
		return m.CreateMergeRequestFunc(pid, opt, options...)
	}
	return nil, NewMockResponse(0), nil
}

// GetMergeRequestCommits implements the interface method.
func (m *MockMergeRequestsService) GetMergeRequestCommits(pid interface{}, mergeRequest int, opt *gitlab.GetMergeRequestCommitsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.Commit, *gitlab.Response, error) {
	m.GetMergeRequestCommitsCalls = append(m.GetMergeRequestCommitsCalls, GetMergeRequestCommitsCall{
		PID:          pid,
		MergeRequest: mergeRequest,
		Opt:          opt,
	})
	if m.GetMergeRequestCommitsFunc != nil {
		return m.GetMergeRequestCommitsFunc(pid, mergeRequest, opt, options...)
	}
	return nil, NewMockResponse(0), nil
}

// MockDiscussionsService is a mock implementation of GitLabDiscussionsService.
type MockDiscussionsService struct {
	ListMergeRequestDiscussionsFunc func(pid interface{}, mergeRequest int, opt *gitlab.ListMergeRequestDiscussionsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.Discussion, *gitlab.Response, error)

	// Call tracking
	ListMergeRequestDiscussionsCalls []ListMergeRequestDiscussionsCall
}

// ListMergeRequestDiscussionsCall tracks a call to ListMergeRequestDiscussions.
type ListMergeRequestDiscussionsCall struct {
	PID          interface{}
	MergeRequest int
	Opt          *gitlab.ListMergeRequestDiscussionsOptions
}

// ListMergeRequestDiscussions implements the interface method.
func (m *MockDiscussionsService) ListMergeRequestDiscussions(pid interface{}, mergeRequest int, opt *gitlab.ListMergeRequestDiscussionsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.Discussion, *gitlab.Response, error) {
	m.ListMergeRequestDiscussionsCalls = append(m.ListMergeRequestDiscussionsCalls, ListMergeRequestDiscussionsCall{
		PID:          pid,
		MergeRequest: mergeRequest,
		Opt:          opt,
	})
	if m.ListMergeRequestDiscussionsFunc != nil {
		return m.ListMergeRequestDiscussionsFunc(pid, mergeRequest, opt, options...)
	}
	return nil, NewMockResponse(0), nil
}

// MockUsersService is a mock implementation of GitLabUsersService.
type MockUsersService struct {
	ListUsersFunc func(opt *gitlab.ListUsersOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.User, *gitlab.Response, error)
	GetUserFunc   func(user int, opt gitlab.GetUsersOptions, options ...gitlab.RequestOptionFunc) (*gitlab.User, *gitlab.Response, error)
}

// ListUsers implements the interface method.
func (m *MockUsersService) ListUsers(opt *gitlab.ListUsersOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.User, *gitlab.Response, error) {
	if m.ListUsersFunc != nil {
		return m.ListUsersFunc(opt, options...)
	}
	return nil, NewMockResponse(0), nil
}

// GetUser implements the interface method.
func (m *MockUsersService) GetUser(user int, opt gitlab.GetUsersOptions, options ...gitlab.RequestOptionFunc) (*gitlab.User, *gitlab.Response, error) {
	if m.GetUserFunc != nil {
		return m.GetUserFunc(user, opt, options...)
	}
	return nil, NewMockResponse(0), nil
}

// MockLabelsService is a mock implementation of GitLabLabelsService.
type MockLabelsService struct {
	ListLabelsFunc   func(pid interface{}, opt *gitlab.ListLabelsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.Label, *gitlab.Response, error)
	CreateLabelFunc  func(pid interface{}, opt *gitlab.CreateLabelOptions, options ...gitlab.RequestOptionFunc) (*gitlab.Label, *gitlab.Response, error)

	// Call tracking
	ListLabelsCalls   []ListLabelsCall
	CreateLabelCalls  []CreateLabelCall
}

// ListLabelsCall tracks a call to ListLabels.
type ListLabelsCall struct {
	PID interface{}
	Opt *gitlab.ListLabelsOptions
}

// CreateLabelCall tracks a call to CreateLabel.
type CreateLabelCall struct {
	PID interface{}
	Opt *gitlab.CreateLabelOptions
}

// ListLabels implements the interface method.
func (m *MockLabelsService) ListLabels(pid interface{}, opt *gitlab.ListLabelsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.Label, *gitlab.Response, error) {
	m.ListLabelsCalls = append(m.ListLabelsCalls, ListLabelsCall{
		PID: pid,
		Opt: opt,
	})
	if m.ListLabelsFunc != nil {
		return m.ListLabelsFunc(pid, opt, options...)
	}
	return nil, NewMockResponse(0), nil
}

// CreateLabel implements the interface method.
func (m *MockLabelsService) CreateLabel(pid interface{}, opt *gitlab.CreateLabelOptions, options ...gitlab.RequestOptionFunc) (*gitlab.Label, *gitlab.Response, error) {
	m.CreateLabelCalls = append(m.CreateLabelCalls, CreateLabelCall{
		PID: pid,
		Opt: opt,
	})
	if m.CreateLabelFunc != nil {
		return m.CreateLabelFunc(pid, opt, options...)
	}
	return &gitlab.Label{}, NewMockResponse(0), nil
}

// NewMockResponse creates a mock GitLab API response with the specified next page.
// Set nextPage to 0 to indicate no more pages.
func NewMockResponse(nextPage int) *gitlab.Response {
	return &gitlab.Response{
		Response: &http.Response{
			StatusCode: 200,
		},
		NextPage: nextPage,
	}
}

// NewMockResponse404 creates a mock GitLab API 404 response.
func NewMockResponse404() *gitlab.Response {
	return &gitlab.Response{
		Response: &http.Response{
			StatusCode: 404,
		},
		NextPage: 0,
	}
}

// MockBranchesService is a mock implementation of GitLabBranchesService.
type MockBranchesService struct {
	GetBranchFunc    func(pid interface{}, branch string, options ...gitlab.RequestOptionFunc) (*gitlab.Branch, *gitlab.Response, error)
	CreateBranchFunc func(pid interface{}, opt *gitlab.CreateBranchOptions, options ...gitlab.RequestOptionFunc) (*gitlab.Branch, *gitlab.Response, error)

	// Call tracking
	GetBranchCalls    []GetBranchCall
	CreateBranchCalls []CreateBranchCall
}

// GetBranchCall tracks a call to GetBranch.
type GetBranchCall struct {
	PID    interface{}
	Branch string
}

// CreateBranchCall tracks a call to CreateBranch.
type CreateBranchCall struct {
	PID interface{}
	Opt *gitlab.CreateBranchOptions
}

// GetBranch implements the interface method.
func (m *MockBranchesService) GetBranch(pid interface{}, branch string, options ...gitlab.RequestOptionFunc) (*gitlab.Branch, *gitlab.Response, error) {
	m.GetBranchCalls = append(m.GetBranchCalls, GetBranchCall{
		PID:    pid,
		Branch: branch,
	})
	if m.GetBranchFunc != nil {
		return m.GetBranchFunc(pid, branch, options...)
	}
	return nil, NewMockResponse(0), nil
}

// CreateBranch implements the interface method.
func (m *MockBranchesService) CreateBranch(pid interface{}, opt *gitlab.CreateBranchOptions, options ...gitlab.RequestOptionFunc) (*gitlab.Branch, *gitlab.Response, error) {
	m.CreateBranchCalls = append(m.CreateBranchCalls, CreateBranchCall{
		PID: pid,
		Opt: opt,
	})
	if m.CreateBranchFunc != nil {
		return m.CreateBranchFunc(pid, opt, options...)
	}
	return nil, NewMockResponse(0), nil
}

// MockJobsService is a mock implementation of GitLabJobsService.
type MockJobsService struct {
	GetJobFunc          func(pid interface{}, jobID int, options ...gitlab.RequestOptionFunc) (*gitlab.Job, *gitlab.Response, error)
	ListProjectJobsFunc func(pid interface{}, opts *gitlab.ListJobsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.Job, *gitlab.Response, error)

	GetJobCalls          []GetJobCall
	ListProjectJobsCalls []ListProjectJobsCall
}

type GetJobCall struct {
	PID   interface{}
	JobID int
}

type ListProjectJobsCall struct {
	PID  interface{}
	Opts *gitlab.ListJobsOptions
}

func (m *MockJobsService) GetJob(pid interface{}, jobID int, options ...gitlab.RequestOptionFunc) (*gitlab.Job, *gitlab.Response, error) {
	m.GetJobCalls = append(m.GetJobCalls, GetJobCall{PID: pid, JobID: jobID})
	if m.GetJobFunc != nil {
		return m.GetJobFunc(pid, jobID, options...)
	}
	return nil, NewMockResponse(0), nil
}

func (m *MockJobsService) ListProjectJobs(pid interface{}, opts *gitlab.ListJobsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.Job, *gitlab.Response, error) {
	m.ListProjectJobsCalls = append(m.ListProjectJobsCalls, ListProjectJobsCall{PID: pid, Opts: opts})
	if m.ListProjectJobsFunc != nil {
		return m.ListProjectJobsFunc(pid, opts, options...)
	}
	return nil, NewMockResponse(0), nil
}
