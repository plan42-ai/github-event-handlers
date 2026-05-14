package handlers_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/plan42-ai/github-event-handlers/github"
	"github.com/plan42-ai/github-event-handlers/handlers"
	"github.com/plan42-ai/sdk-go/p42"
	"github.com/stretchr/testify/require"
)

const (
	defaultRepoOwner = "octo"
	defaultRepoName  = "demo"
	defaultCommand   = "/plan42"
	defaultPRState   = "open"
	defaultTaskID    = "task-1"
	testTenantID     = "tenant-1"
)

func TestCommentsHandlerCreatesTurnFromIssueComment(t *testing.T) {
	gh := &fakeGithubAPI{pullRequestID: 1234, pullRequestState: defaultPRState, pullRequestAuthor: "commenter"}
	fakeTasks := &fakeTaskClient{
		searchResp: &p42.List[p42.Task]{
			Items: []p42.Task{{
				TenantID:     testTenantID,
				TaskID:       defaultTaskID,
				Version:      7,
				AssignedToAI: true,
			}},
		},
		lastTurnResp: &p42.Turn{TurnIndex: 2},
	}
	registry := newRegistryWithTasks(fakeTasks)
	issueEvt := issueCommentEvent("delivery-1", defaultCommand+" rerun", "commenter", 17, true)

	require.NoError(t, registry.Handle(context.Background(), issueEvt, gh))
	require.NotNil(t, fakeTasks.createReq)
	require.Equal(t, 3, fakeTasks.createReq.TurnIndex)
	require.True(t, gh.getPRCalled)
}

func TestCommentsHandlerUsesInstallationTokenForIssueComment(t *testing.T) {
	installID := int64(9001)
	gh := &fakeGithubAPI{pullRequestID: 1234, pullRequestState: defaultPRState, pullRequestAuthor: "commenter"}
	fakeTasks := &fakeTaskClient{
		searchResp: &p42.List[p42.Task]{
			Items: []p42.Task{{
				TenantID:     testTenantID,
				TaskID:       defaultTaskID,
				Version:      7,
				AssignedToAI: true,
			}},
		},
		lastTurnResp: &p42.Turn{TurnIndex: 2},
	}
	planClient := newFakePlan42Client(fakeTasks)
	tokenFetcher := &fakeTokenFetcher{token: "install-token"}
	registry := handlers.NewHandlerRegistry(handlers.Config{
		Plan42Client:      planClient,
		CommentTriggerStr: defaultCommand,
		TokenFetcher:      tokenFetcher,
		UseGithubApp:      true,
	})

	issueEvt := issueCommentEvent("delivery-install", defaultCommand, "commenter", 17, true)
	issueEvt.InstallationID = ptr(installID)

	require.NoError(t, registry.Handle(context.Background(), issueEvt, gh))
	require.Equal(t, []int64{installID}, tokenFetcher.installationIDs)
	require.Equal(t, "token install-token", gh.lastPRAuthHeader)
}

func TestCommentsHandlerLooksUpInstallationWhenIssueEventMissingID(t *testing.T) {
	gh := &fakeGithubAPI{pullRequestID: 1234, pullRequestState: defaultPRState, pullRequestAuthor: "commenter"}
	fakeTasks := &fakeTaskClient{
		searchResp: &p42.List[p42.Task]{
			Items: []p42.Task{{
				TenantID:     testTenantID,
				TaskID:       defaultTaskID,
				Version:      7,
				AssignedToAI: true,
			}},
		},
		lastTurnResp: &p42.Turn{TurnIndex: 2},
	}
	planClient := &fakePlan42TaskClient{
		fakeTaskClient: fakeTasks,
		listOrgsResp: &p42.ListGithubOrgsResponse{
			Orgs: []p42.GithubOrg{
				{
					OrgName:        defaultRepoOwner,
					InstallationID: 77,
				},
			},
		},
	}
	tokenFetcher := &fakeTokenFetcher{token: "lookup-token"}
	registry := handlers.NewHandlerRegistry(handlers.Config{
		Plan42Client:      planClient,
		CommentTriggerStr: defaultCommand,
		TokenFetcher:      tokenFetcher,
		UseGithubApp:      true,
	})

	issueEvt := issueCommentEvent("delivery-lookup", defaultCommand, "commenter", 99, true)

	require.NoError(t, registry.Handle(context.Background(), issueEvt, gh))
	require.Equal(t, []int64{77}, tokenFetcher.installationIDs)
	require.Equal(t, "token lookup-token", gh.lastPRAuthHeader)
	require.NotNil(t, planClient.listOrgsReq)
	require.Equal(t, defaultRepoOwner, *planClient.listOrgsReq.Name)
}

func TestCommentsHandlerPostsConflictCommentToTriggeringPRWhenRepoInfoMissing(t *testing.T) {
	fakeTasks := &fakeTaskClient{
		searchResp: &p42.List[p42.Task]{
			Items: []p42.Task{{
				TenantID: testTenantID,
				TaskID:   defaultTaskID,
				Version:  3,
			}},
		},
		lastTurnResp: &p42.Turn{TurnIndex: 1},
		createErr:    &p42.ConflictError{Message: "conflict"},
	}
	gh := &fakeGithubAPI{}
	registry := handlers.NewHandlerRegistry(handlers.Config{
		Plan42Client:      newFakePlan42Client(fakeTasks),
		CommentTriggerStr: defaultCommand,
		UseGithubApp:      true,
	})
	reviewEvt := reviewEvent("delivery-3", ptr(defaultCommand), "commenter", "commenter", 55, 321)

	require.NoError(t, registry.Handle(context.Background(), reviewEvt, gh))
	require.Len(t, gh.createdComments, 1)
	comment := gh.createdComments[0]
	require.Equal(t, defaultRepoOwner, comment.owner)
	require.Equal(t, defaultRepoName, comment.repo)
	require.Equal(t, 55, comment.issueNumber)
}

func TestCommentsHandlerUsesTokenFetcherForMismatchComment(t *testing.T) {
	fakeTasks := &fakeTaskClient{
		searchResp: &p42.List[p42.Task]{
			Items: []p42.Task{{
				TenantID: testTenantID,
				TaskID:   defaultTaskID,
				Version:  1,
			}},
		},
		lastTurnResp: &p42.Turn{TurnIndex: 1},
	}
	planClient := &fakePlan42TaskClient{
		fakeTaskClient: fakeTasks,
		listOrgsResp: &p42.ListGithubOrgsResponse{Orgs: []p42.GithubOrg{{
			OrgName:        defaultRepoOwner,
			InstallationID: 77,
		}}},
	}
	tokenFetcher := &fakeTokenFetcher{token: "mismatch-token"}
	gh := &fakeGithubAPI{}
	gh.pullRequestAuthor = defaultRepoOwner
	registry := handlers.NewHandlerRegistry(handlers.Config{
		Plan42Client:      planClient,
		CommentTriggerStr: defaultCommand,
		TokenFetcher:      tokenFetcher,
		UseGithubApp:      true,
	})

	reviewEvt := reviewCommentEvent("delivery-token", defaultCommand, "other-user", defaultRepoOwner, 21, 456)

	require.NoError(t, registry.Handle(context.Background(), reviewEvt, gh))
	require.Len(t, gh.createdComments, 1)
	require.Equal(t, "token mismatch-token", gh.lastAuthHeader)
	require.Equal(t, []int64{77}, tokenFetcher.installationIDs)
	require.Nil(t, fakeTasks.createReq, "turn should not be created for mismatched author")
}

func TestCommentsHandlerUsesFallbackInstallationForConflictComment(t *testing.T) {
	fakeTasks := &fakeTaskClient{
		searchResp: &p42.List[p42.Task]{
			Items: []p42.Task{{
				TenantID: testTenantID,
				TaskID:   defaultTaskID,
				Version:  5,
			}},
		},
		lastTurnResp: &p42.Turn{TurnIndex: 2},
		createErr:    &p42.ConflictError{Message: "conflict"},
	}
	planClient := &fakePlan42TaskClient{
		fakeTaskClient: fakeTasks,
		listOrgsResp: &p42.ListGithubOrgsResponse{Orgs: []p42.GithubOrg{{
			OrgName:        defaultRepoOwner,
			InstallationID: 55,
		}}},
	}
	tokenFetcher := &fakeTokenFetcher{token: "conflict-token"}
	gh := &fakeGithubAPI{}
	gh.pullRequestAuthor = defaultRepoOwner
	registry := handlers.NewHandlerRegistry(handlers.Config{
		Plan42Client:      planClient,
		CommentTriggerStr: defaultCommand,
		TokenFetcher:      tokenFetcher,
		UseGithubApp:      true,
	})

	reviewEvt := reviewEvent("delivery-conflict", ptr(defaultCommand), defaultRepoOwner, defaultRepoOwner, 33, 789)

	require.NoError(t, registry.Handle(context.Background(), reviewEvt, gh))
	require.Len(t, gh.createdComments, 1)
	require.Equal(t, "token conflict-token", gh.lastAuthHeader)
	require.Equal(t, []int64{55}, tokenFetcher.installationIDs)
}

func TestCommentsHandlerSkipsConflictCommentWhenGithubAppDisabled(t *testing.T) {
	fakeTasks := &fakeTaskClient{
		searchResp: &p42.List[p42.Task]{
			Items: []p42.Task{{
				TenantID: testTenantID,
				TaskID:   defaultTaskID,
				Version:  3,
			}},
		},
		lastTurnResp: &p42.Turn{TurnIndex: 1},
		createErr:    &p42.ConflictError{Message: "conflict"},
	}
	planClient := &fakePlan42TaskClient{
		fakeTaskClient: fakeTasks,
		listOrgsResp: &p42.ListGithubOrgsResponse{Orgs: []p42.GithubOrg{{
			OrgName:        defaultRepoOwner,
			InstallationID: 55,
		}}},
	}
	tokenFetcher := &fakeTokenFetcher{token: "should-not-be-used"}
	gh := &fakeGithubAPI{}
	registry := handlers.NewHandlerRegistry(handlers.Config{
		Plan42Client:      planClient,
		CommentTriggerStr: defaultCommand,
		TokenFetcher:      tokenFetcher,
	})

	reviewEvt := reviewEvent("delivery-no-app", ptr(defaultCommand), "commenter", "commenter", 55, 321)

	require.NoError(t, registry.Handle(context.Background(), reviewEvt, gh))
	require.Empty(t, gh.createdComments)
	require.Empty(t, tokenFetcher.installationIDs)
}

func TestCommentsHandlerSkipsAuthorMismatchCommentWhenGithubAppDisabled(t *testing.T) {
	fakeTasks := &fakeTaskClient{
		searchResp: &p42.List[p42.Task]{
			Items: []p42.Task{{
				TenantID: testTenantID,
				TaskID:   defaultTaskID,
				Version:  1,
			}},
		},
		lastTurnResp: &p42.Turn{TurnIndex: 1},
	}
	planClient := &fakePlan42TaskClient{
		fakeTaskClient: fakeTasks,
		listOrgsResp: &p42.ListGithubOrgsResponse{Orgs: []p42.GithubOrg{{
			OrgName:        defaultRepoOwner,
			InstallationID: 77,
		}}},
	}
	tokenFetcher := &fakeTokenFetcher{token: "should-not-be-used"}
	gh := &fakeGithubAPI{}
	gh.pullRequestAuthor = defaultRepoOwner
	registry := handlers.NewHandlerRegistry(handlers.Config{
		Plan42Client:      planClient,
		CommentTriggerStr: defaultCommand,
		TokenFetcher:      tokenFetcher,
	})

	reviewEvt := reviewCommentEvent("delivery-no-app-mismatch", defaultCommand, "other-user", defaultRepoOwner, 21, 456)

	require.NoError(t, registry.Handle(context.Background(), reviewEvt, gh))
	require.Empty(t, gh.createdComments)
	require.Empty(t, tokenFetcher.installationIDs)
	require.Nil(t, fakeTasks.createReq, "turn should not be created for mismatched author")
}

// Helpers --------------------------------------------------------------------

func newRegistryWithTasks(tasks *fakeTaskClient) *handlers.HandlerRegistry {
	return handlers.NewHandlerRegistry(handlers.Config{
		Plan42Client:      newFakePlan42Client(tasks),
		CommentTriggerStr: defaultCommand,
	})
}

func issueCommentEvent(deliveryID, body, author string, issueNumber int, isPR bool) *handlers.IssueCommentEvent {
	return &handlers.IssueCommentEvent{
		EventBase: handlers.EventBase{DeliveryID: deliveryID},
		Action:    testActionCreated,
		Comment: handlers.Comment{
			Body:  body,
			Login: author,
		},
		Issue: handlers.Issue{
			Number:        issueNumber,
			State:         defaultPRState,
			IsPullRequest: isPR,
		},
		Repository: defaultRepository(),
	}
}

func reviewCommentEvent(deliveryID, body, commenter, author string, issueNumber int, prID int64) *handlers.PullRequestReviewCommentEvent {
	return &handlers.PullRequestReviewCommentEvent{
		EventBase: handlers.EventBase{DeliveryID: deliveryID},
		Action:    testActionCreated,
		Comment: handlers.Comment{
			Body:  body,
			Login: commenter,
		},
		PullRequest: handlers.PullRequest{
			ID:     prID,
			Number: issueNumber,
			State:  defaultPRState,
			Login:  author,
		},
		Repository: defaultRepository(),
	}
}

func reviewEvent(deliveryID string, body *string, reviewer, author string, issueNumber int, prID int64) *handlers.PullRequestReviewEvent {
	return &handlers.PullRequestReviewEvent{
		EventBase: handlers.EventBase{DeliveryID: deliveryID},
		Action:    testActionSubmitted,
		Review: handlers.Review{
			Body:  body,
			Login: reviewer,
		},
		PullRequest: handlers.PullRequest{
			ID:     prID,
			Number: issueNumber,
			State:  defaultPRState,
			Login:  author,
		},
		Repository: defaultRepository(),
	}
}

func defaultRepository() handlers.Repository {
	return handlers.Repository{Org: defaultRepoOwner, Name: defaultRepoName, FullName: defaultRepoOwner + "/" + defaultRepoName}
}

func ptr[T any](v T) *T { return &v }

// Fake clients ----------------------------------------------------------------

type fakeTaskClient struct {
	searchReq    *p42.SearchTasksRequest
	searchResp   *p42.List[p42.Task]
	lastTurnReq  *p42.GetLastTurnRequest
	lastTurnResp *p42.Turn
	createReq    *p42.CreateTurnRequest
	createErr    error
}

func (f *fakeTaskClient) SearchTasks(_ context.Context, req *p42.SearchTasksRequest) (*p42.List[p42.Task], error) {
	f.searchReq = req
	return f.searchResp, nil
}

func (f *fakeTaskClient) CreateTurn(_ context.Context, req *p42.CreateTurnRequest) (*p42.Turn, error) {
	f.createReq = req
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &p42.Turn{TurnIndex: req.TurnIndex}, nil
}

func (f *fakeTaskClient) GetLastTurn(_ context.Context, req *p42.GetLastTurnRequest) (*p42.Turn, error) {
	f.lastTurnReq = req
	return f.lastTurnResp, nil
}

type fakePlan42TaskClient struct {
	*fakeTaskClient
	listOrgsResp *p42.ListGithubOrgsResponse
	listOrgsErr  error
	listOrgsReq  *p42.ListGithubOrgsRequest
}

func newFakePlan42Client(tasks *fakeTaskClient) handlers.Plan42Client {
	return &fakePlan42TaskClient{fakeTaskClient: tasks}
}

func (f *fakePlan42TaskClient) ListGithubOrgs(_ context.Context, req *p42.ListGithubOrgsRequest) (*p42.ListGithubOrgsResponse, error) {
	f.listOrgsReq = req
	if f.listOrgsErr != nil {
		return nil, f.listOrgsErr
	}
	return f.listOrgsResp, nil
}

func (f *fakePlan42TaskClient) UpdateGithubOrg(context.Context, *p42.UpdateGithubOrgRequest) (*p42.GithubOrg, error) {
	return nil, nil
}

func (f *fakePlan42TaskClient) DeleteGithubOrg(context.Context, *p42.DeleteGithubOrgRequest) error {
	return nil
}

func (f *fakePlan42TaskClient) AddGithubOrg(context.Context, *p42.AddGithubOrgRequest) (*p42.GithubOrg, error) {
	return nil, nil
}

func (f *fakePlan42TaskClient) UpdateTask(context.Context, *p42.UpdateTaskRequest) (*p42.Task, error) {
	return nil, nil
}

func (f *fakePlan42TaskClient) UpdateWorkstreamTask(context.Context, *p42.UpdateWorkstreamTaskRequest) (*p42.Task, error) {
	return nil, nil
}

type recordedComment struct {
	owner       string
	repo        string
	issueNumber int
	body        string
}

type fakeGithubAPI struct {
	pullRequestID     int64
	pullRequestState  string
	pullRequestAuthor string
	getPRCalled       bool
	createdComments   []recordedComment
	lastAuthHeader    string
	lastPRAuthHeader  string
}

func (f *fakeGithubAPI) FindIssueCommentWithMarker(context.Context, string, string, int, string) (*github.IssueComment, error) {
	return nil, nil
}

func (f *fakeGithubAPI) CreateIssueComment(ctx context.Context, owner, repo string, issueNumber int, body string) (*github.IssueComment, error) {
	f.lastAuthHeader = f.captureAuthHeader(ctx)
	f.createdComments = append(f.createdComments, recordedComment{owner: owner, repo: repo, issueNumber: issueNumber, body: body})
	return &github.IssueComment{ID: int64(len(f.createdComments)), Body: body}, nil
}

func (f *fakeGithubAPI) UpdateIssueComment(context.Context, string, string, int64, string) (*github.IssueComment, error) {
	return nil, nil
}

func (f *fakeGithubAPI) GetPullRequest(ctx context.Context, _ string, _ string, _ int) (*github.PullRequest, error) {
	f.getPRCalled = true
	f.lastPRAuthHeader = f.captureAuthHeader(ctx)
	return &github.PullRequest{
		ID:    f.pullRequestID,
		State: f.pullRequestState,
		User: github.PullRequestUser{
			Login: f.pullRequestAuthor,
		},
	}, nil
}

func (f *fakeGithubAPI) GetInstallationToken(context.Context, int64) (string, error) {
	return "", nil
}

func (f *fakeGithubAPI) captureAuthHeader(ctx context.Context) string {
	ap := github.GetAuthProvider(ctx)
	if ap == nil {
		return ""
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com", nil)
	if err != nil {
		return ""
	}
	req, err = ap.AddAuth(req)
	if err != nil {
		return ""
	}
	return req.Header.Get("Authorization")
}

type fakeTokenFetcher struct {
	token           string
	installationIDs []int64
}

func (f *fakeTokenFetcher) InstallationToken(_ context.Context, _ github.API, installationID int64) (string, error) {
	f.installationIDs = append(f.installationIDs, installationID)
	return f.token, nil
}
