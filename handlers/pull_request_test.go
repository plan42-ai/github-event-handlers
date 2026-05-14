package handlers_test

import (
	"context"
	"testing"
	"time"

	"github.com/plan42-ai/github-event-handlers/handlers"
	"github.com/plan42-ai/sdk-go/p42"
	"github.com/stretchr/testify/require"
)

const (
	testRepoKey       = "octo/demo"
	testOtherRepoKey  = "octo/lib"
	testWorkstreamID  = "ws-1"
	testTaskID        = "task"
	testFeatureBranch = "feature"
	testTargetBranch  = "main"
)

// recordingPlan42Client wraps fakeTaskClient and records UpdateTask/UpdateWorkstreamTask calls.
type recordingPlan42Client struct {
	*fakeTaskClient
	updateReqs               []*p42.UpdateTaskRequest
	updateWorkstreamTaskReqs []*p42.UpdateWorkstreamTaskRequest
}

func (r *recordingPlan42Client) ListGithubOrgs(context.Context, *p42.ListGithubOrgsRequest) (*p42.ListGithubOrgsResponse, error) {
	return nil, nil
}

func (r *recordingPlan42Client) UpdateGithubOrg(context.Context, *p42.UpdateGithubOrgRequest) (*p42.GithubOrg, error) {
	return nil, nil
}

func (r *recordingPlan42Client) DeleteGithubOrg(context.Context, *p42.DeleteGithubOrgRequest) error {
	return nil
}

func (r *recordingPlan42Client) AddGithubOrg(context.Context, *p42.AddGithubOrgRequest) (*p42.GithubOrg, error) {
	return nil, nil
}

func (r *recordingPlan42Client) UpdateTask(_ context.Context, req *p42.UpdateTaskRequest) (*p42.Task, error) {
	r.updateReqs = append(r.updateReqs, req)
	return &p42.Task{TaskID: req.TaskID}, nil
}

func (r *recordingPlan42Client) UpdateWorkstreamTask(_ context.Context, req *p42.UpdateWorkstreamTaskRequest) (*p42.Task, error) {
	r.updateWorkstreamTaskReqs = append(r.updateWorkstreamTaskReqs, req)
	return &p42.Task{TaskID: req.TaskID}, nil
}

func TestPullRequestHandlerUpdatesTaskStatus(t *testing.T) {
	t.Parallel()

	prID := int64(123456)
	prNumber := 7
	updatedAt := time.Now().Add(-time.Minute)
	initialStatus := "open"
	repoInfo := map[string]*p42.RepoInfo{
		testRepoKey: {
			FeatureBranch: testFeatureBranch,
			TargetBranch:  testTargetBranch,
			PRStatus:      &initialStatus,
		},
	}

	task := p42.Task{TenantID: testTenantID, TaskID: testTaskID, Version: 3, RepoInfo: repoInfo}
	fake := &recordingPlan42Client{fakeTaskClient: &fakeTaskClient{searchResp: &p42.List[p42.Task]{Items: []p42.Task{task}}}}
	registry := handlers.NewHandlerRegistry(handlers.Config{Plan42Client: fake})
	evt := samplePullRequestEvent("delivery-pr", prID, prNumber, "closed", true, &updatedAt, "https://github.com/octo/demo/pull/7")

	require.NoError(t, registry.Handle(context.Background(), evt, nil))
	require.Len(t, fake.updateReqs, 1)

	req := fake.updateReqs[0]
	require.Equal(t, task.Version, req.Version)
	require.NotNil(t, req.RepoInfo)

	repo := (*req.RepoInfo)[testRepoKey]
	require.NotNil(t, repo)
	require.Equal(t, "merged", deref(repo.PRStatus))
	require.NotNil(t, repo.PRStatusUpdatedAt)
	require.InDelta(t, updatedAt.UnixMilli(), repo.PRStatusUpdatedAt.UnixMilli(), 1000)
	require.Equal(t, prNumber, deref(repo.PRNumber))
	require.Equal(t, "https://github.com/octo/demo/pull/7", deref(repo.PRLink))
	require.Equal(t, "123456", deref(repo.PRID))
}

func TestPullRequestHandlerSkipsOlderStatus(t *testing.T) {
	t.Parallel()

	prID := int64(999)
	prIDStr := "999"
	currentStatus := "merged"
	currentUpdatedAt := time.Now()
	prNumber := 2
	repoInfo := map[string]*p42.RepoInfo{
		testRepoKey: {
			FeatureBranch:     testFeatureBranch,
			TargetBranch:      testTargetBranch,
			PRStatus:          &currentStatus,
			PRStatusUpdatedAt: &currentUpdatedAt,
			PRID:              &prIDStr,
			PRNumber:          &prNumber,
		},
	}

	task := p42.Task{TenantID: testTenantID, TaskID: testTaskID, Version: 5, RepoInfo: repoInfo}
	fake := &recordingPlan42Client{fakeTaskClient: &fakeTaskClient{searchResp: &p42.List[p42.Task]{Items: []p42.Task{task}}}}
	registry := handlers.NewHandlerRegistry(handlers.Config{Plan42Client: fake})

	older := currentUpdatedAt.Add(-time.Hour)
	evt := samplePullRequestEvent("delivery-old", prID, 2, "open", false, &older, "")

	require.NoError(t, registry.Handle(context.Background(), evt, nil))
	require.Empty(t, fake.updateReqs)
}

func TestPullRequestHandlerUpdatesWorkstreamTaskStatus(t *testing.T) {
	t.Parallel()

	prID := int64(123456)
	prNumber := 7
	updatedAt := time.Now().Add(-time.Minute)
	initialStatus := "open"
	repoInfo := map[string]*p42.RepoInfo{
		testRepoKey: {
			FeatureBranch: testFeatureBranch,
			TargetBranch:  testTargetBranch,
			PRStatus:      &initialStatus,
		},
	}

	wsID := testWorkstreamID
	task := p42.Task{
		TenantID:     testTenantID,
		TaskID:       testTaskID,
		Version:      3,
		RepoInfo:     repoInfo,
		WorkstreamID: &wsID,
		State:        p42.TaskStateAwaitingCodeReview,
	}
	fake := &recordingPlan42Client{fakeTaskClient: &fakeTaskClient{searchResp: &p42.List[p42.Task]{Items: []p42.Task{task}}}}
	registry := handlers.NewHandlerRegistry(handlers.Config{Plan42Client: fake})
	evt := samplePullRequestEvent("delivery-pr", prID, prNumber, "closed", true, &updatedAt, "https://github.com/octo/demo/pull/7")

	require.NoError(t, registry.Handle(context.Background(), evt, nil))
	require.Empty(t, fake.updateReqs)
	require.Len(t, fake.updateWorkstreamTaskReqs, 1)

	req := fake.updateWorkstreamTaskReqs[0]
	require.Equal(t, task.Version, req.Version)
	require.Equal(t, testWorkstreamID, req.WorkstreamID)
	require.NotNil(t, req.RepoInfo)

	repo := (*req.RepoInfo)[testRepoKey]
	require.NotNil(t, repo)
	require.Equal(t, "merged", deref(repo.PRStatus))
	require.NotNil(t, req.State)
	require.Equal(t, p42.TaskStateCompleted, *req.State)
	require.Equal(t, prNumber, deref(repo.PRNumber))
	require.Equal(t, "https://github.com/octo/demo/pull/7", deref(repo.PRLink))
	require.Equal(t, "123456", deref(repo.PRID))
}

func TestPullRequestHandlerCompletesWorkstreamTaskWhenAllPRsTerminal(t *testing.T) {
	t.Parallel()
	req := runWorkstreamTransitionTest(t, p42.TaskStateAwaitingCodeReview, "open", "closed")
	require.Equal(t, p42.TaskStateCompleted, *req.State)
}

func TestPullRequestHandlerDoesNotCompleteExecutingWorkstreamTaskWhenAllPRsTerminal(t *testing.T) {
	t.Parallel()
	req := runWorkstreamTransitionTest(t, p42.TaskStateExecuting, "open", "closed")
	require.Equal(t, p42.TaskStateExecuting, *req.State)
}

func TestPullRequestHandlerReopensCompletedWorkstreamTaskWhenAnyPRBecomesNonTerminal(t *testing.T) {
	t.Parallel()
	req := runWorkstreamTransitionTest(t, p42.TaskStateCompleted, "closed", "open")
	require.Equal(t, p42.TaskStateAwaitingCodeReview, *req.State)
}

func TestPullRequestHandlerDoesNotMoveExecutingWorkstreamTaskBackToAwaitingCodeReview(t *testing.T) {
	t.Parallel()
	req := runWorkstreamTransitionTest(t, p42.TaskStateExecuting, "closed", "open")
	require.Equal(t, p42.TaskStateExecuting, *req.State)
}

func TestPullRequestHandlerStatusDerivation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		merged bool
		state  string
		draft  bool
		want   string
	}{
		{"merged", true, "closed", false, "merged"},
		{"closed", false, "closed", false, "closed"},
		{"draft", false, "open", true, "draft"},
		{"open", false, "open", false, "open"},
		{"unknown", false, "pending", false, "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Status derivation is internal; test via the handler output.
			initialStatus := "open"
			repoInfo := map[string]*p42.RepoInfo{
				testRepoKey: {PRStatus: &initialStatus},
			}
			task := p42.Task{TenantID: testTenantID, TaskID: testTaskID, Version: 1, RepoInfo: repoInfo}
			fake := &recordingPlan42Client{fakeTaskClient: &fakeTaskClient{searchResp: &p42.List[p42.Task]{Items: []p42.Task{task}}}}
			registry := handlers.NewHandlerRegistry(handlers.Config{Plan42Client: fake})

			now := time.Now()
			evt := samplePullRequestEvent("delivery-status", 1, 1, tt.state, tt.merged, &now, "")
			evt.PullRequest.Draft = tt.draft

			require.NoError(t, registry.Handle(context.Background(), evt, nil))
			require.Len(t, fake.updateReqs, 1)
			repo := (*fake.updateReqs[0].RepoInfo)[testRepoKey]
			require.NotNil(t, repo)
			require.Equal(t, tt.want, deref(repo.PRStatus))
		})
	}
}

func TestPullRequestHandlerIdempotentDuplicate(t *testing.T) {
	t.Parallel()

	prID := int64(42)
	prIDStr := "42"
	now := time.Now()
	status := "merged"
	prNumber := 5
	prLink := "https://github.com/octo/demo/pull/5"

	repoInfo := map[string]*p42.RepoInfo{
		testRepoKey: {
			FeatureBranch:     testFeatureBranch,
			TargetBranch:      testTargetBranch,
			PRStatus:          &status,
			PRStatusUpdatedAt: &now,
			PRID:              &prIDStr,
			PRNumber:          &prNumber,
			PRLink:            &prLink,
		},
	}

	task := p42.Task{TenantID: testTenantID, TaskID: testTaskID, Version: 10, RepoInfo: repoInfo}
	fake := &recordingPlan42Client{fakeTaskClient: &fakeTaskClient{searchResp: &p42.List[p42.Task]{Items: []p42.Task{task}}}}
	registry := handlers.NewHandlerRegistry(handlers.Config{Plan42Client: fake})
	evt := samplePullRequestEvent("delivery-dup", prID, prNumber, "closed", true, &now, prLink)

	require.NoError(t, registry.Handle(context.Background(), evt, nil))
	require.Empty(t, fake.updateReqs, "expected no update for duplicate event")
	require.Empty(t, fake.updateWorkstreamTaskReqs, "expected no workstream update for duplicate event")
}

func TestPullRequestHandlerConflictRetry(t *testing.T) {
	t.Parallel()

	prID := int64(77)
	updatedAt := time.Now()
	initialStatus := "open"
	repoInfo := map[string]*p42.RepoInfo{
		testRepoKey: {
			FeatureBranch: testFeatureBranch,
			TargetBranch:  testTargetBranch,
			PRStatus:      &initialStatus,
		},
	}

	task := p42.Task{TenantID: testTenantID, TaskID: testTaskID, Version: 1, RepoInfo: repoInfo}
	updatedTask := task
	updatedTask.Version = 2

	fake := &conflictThenSuccessPlan42Client{
		searchResp:  &p42.List[p42.Task]{Items: []p42.Task{task}},
		conflictErr: &p42.ConflictError{Message: "conflict", Current: &updatedTask},
	}
	registry := handlers.NewHandlerRegistry(handlers.Config{Plan42Client: fake})
	evt := samplePullRequestEvent("delivery-retry", prID, 10, "closed", true, &updatedAt, "")

	require.NoError(t, registry.Handle(context.Background(), evt, nil))
	require.GreaterOrEqual(t, fake.updateCalls, 2)
}

func TestPullRequestHandlerNoSearchResults(t *testing.T) {
	t.Parallel()

	fake := &recordingPlan42Client{fakeTaskClient: &fakeTaskClient{searchResp: &p42.List[p42.Task]{}}}
	registry := handlers.NewHandlerRegistry(handlers.Config{Plan42Client: fake})

	updatedAt := time.Now()
	evt := samplePullRequestEvent("delivery-none", 999, 1, "open", false, &updatedAt, "")

	require.NoError(t, registry.Handle(context.Background(), evt, nil))
	require.Empty(t, fake.updateReqs)
}

func TestPullRequestHandlerNilUpdatedAtFallsBackToNow(t *testing.T) {
	t.Parallel()

	prID := int64(55)
	initialStatus := "open"
	repoInfo := map[string]*p42.RepoInfo{
		testRepoKey: {
			FeatureBranch: testFeatureBranch,
			TargetBranch:  testTargetBranch,
			PRStatus:      &initialStatus,
		},
	}

	task := p42.Task{TenantID: testTenantID, TaskID: testTaskID, Version: 1, RepoInfo: repoInfo}
	fake := &recordingPlan42Client{fakeTaskClient: &fakeTaskClient{searchResp: &p42.List[p42.Task]{Items: []p42.Task{task}}}}
	registry := handlers.NewHandlerRegistry(handlers.Config{Plan42Client: fake})

	before := time.Now()
	evt := samplePullRequestEvent("delivery-nil-ts", prID, 3, "closed", true, nil, "")

	require.NoError(t, registry.Handle(context.Background(), evt, nil))
	require.Len(t, fake.updateReqs, 1)

	repo := (*fake.updateReqs[0].RepoInfo)[testRepoKey]
	require.NotNil(t, repo)
	require.NotNil(t, repo.PRStatusUpdatedAt)
	require.False(t, repo.PRStatusUpdatedAt.Before(before), "expected fallback to time.Now()")
}

// helpers

func deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

func samplePullRequestEvent(
	deliveryID string,
	prID int64,
	prNumber int,
	state string,
	merged bool,
	updatedAt *time.Time,
	htmlURL string,
) *handlers.PullRequestEvent {
	return &handlers.PullRequestEvent{
		EventBase: handlers.EventBase{DeliveryID: deliveryID},
		Action:    "closed",
		Number:    prNumber,
		PullRequest: handlers.PullRequest{
			ID:        prID,
			Number:    prNumber,
			State:     state,
			Merged:    merged,
			HTMLURL:   htmlURL,
			UpdatedAt: updatedAt,
		},
		Repository: defaultRepository(),
	}
}

func runWorkstreamTransitionTest(
	t *testing.T,
	initialState p42.TaskState,
	repoOneStatus string,
	incomingState string,
) *p42.UpdateWorkstreamTaskRequest {
	t.Helper()

	wsID := testWorkstreamID
	repoInfo := map[string]*p42.RepoInfo{
		testRepoKey:      {PRID: ptr("1"), PRStatus: &repoOneStatus},
		testOtherRepoKey: {PRID: ptr("2"), PRStatus: ptr("merged")},
	}
	task := p42.Task{
		TenantID:     testTenantID,
		TaskID:       testTaskID,
		Version:      1,
		RepoInfo:     repoInfo,
		WorkstreamID: &wsID,
		State:        initialState,
	}
	fake := &recordingPlan42Client{fakeTaskClient: &fakeTaskClient{searchResp: &p42.List[p42.Task]{Items: []p42.Task{task}}}}
	registry := handlers.NewHandlerRegistry(handlers.Config{Plan42Client: fake})

	now := time.Now()
	evt := samplePullRequestEvent("delivery-workstream", 1, 10, incomingState, false, &now, "")

	require.NoError(t, registry.Handle(context.Background(), evt, nil))
	require.Len(t, fake.updateWorkstreamTaskReqs, 1)
	return fake.updateWorkstreamTaskReqs[0]
}

// conflictThenSuccessPlan42Client returns a conflict on the first UpdateTask call, then succeeds.
type conflictThenSuccessPlan42Client struct {
	searchResp  *p42.List[p42.Task]
	conflictErr *p42.ConflictError
	updateCalls int
}

func (c *conflictThenSuccessPlan42Client) SearchTasks(_ context.Context, _ *p42.SearchTasksRequest) (*p42.List[p42.Task], error) {
	return c.searchResp, nil
}

func (c *conflictThenSuccessPlan42Client) CreateTurn(_ context.Context, _ *p42.CreateTurnRequest) (*p42.Turn, error) {
	return nil, nil
}

func (c *conflictThenSuccessPlan42Client) GetLastTurn(_ context.Context, _ *p42.GetLastTurnRequest) (*p42.Turn, error) {
	return nil, nil
}

func (c *conflictThenSuccessPlan42Client) ListGithubOrgs(context.Context, *p42.ListGithubOrgsRequest) (*p42.ListGithubOrgsResponse, error) {
	return nil, nil
}

func (c *conflictThenSuccessPlan42Client) UpdateGithubOrg(context.Context, *p42.UpdateGithubOrgRequest) (*p42.GithubOrg, error) {
	return nil, nil
}

func (c *conflictThenSuccessPlan42Client) DeleteGithubOrg(context.Context, *p42.DeleteGithubOrgRequest) error {
	return nil
}

func (c *conflictThenSuccessPlan42Client) AddGithubOrg(context.Context, *p42.AddGithubOrgRequest) (*p42.GithubOrg, error) {
	return nil, nil
}

func (c *conflictThenSuccessPlan42Client) UpdateTask(_ context.Context, _ *p42.UpdateTaskRequest) (*p42.Task, error) {
	c.updateCalls++
	if c.updateCalls == 1 && c.conflictErr != nil {
		return nil, c.conflictErr
	}
	return &p42.Task{}, nil
}

func (c *conflictThenSuccessPlan42Client) UpdateWorkstreamTask(_ context.Context, _ *p42.UpdateWorkstreamTaskRequest) (*p42.Task, error) {
	c.updateCalls++
	if c.updateCalls == 1 && c.conflictErr != nil {
		return nil, c.conflictErr
	}
	return &p42.Task{}, nil
}
