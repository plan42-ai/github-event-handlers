package handlers_test

import (
	"context"
	"errors"
	"testing"

	"github.com/plan42-ai/github-event-handlers/handlers"
	"github.com/plan42-ai/sdk-go/p42"
	"github.com/stretchr/testify/require"
)

const testAllowedAppID int64 = 1234

const testInstallationLogin = "kirillgolo"

const testOrgID = "org-123"

type fakePlan42Client struct {
	listCalled   bool
	updateCalled bool
	deleteCalled bool
	addCalled    bool
	listReq      *p42.ListGithubOrgsRequest
	updateReq    *p42.UpdateGithubOrgRequest
	deleteReq    *p42.DeleteGithubOrgRequest
	addReq       *p42.AddGithubOrgRequest
	listResp     *p42.ListGithubOrgsResponse
	listErr      error
	updateResp   *p42.GithubOrg
	updateErr    error
	deleteErr    error
	addResp      *p42.GithubOrg
	addErr       error
}

func (f *fakePlan42Client) ListGithubOrgs(_ context.Context, req *p42.ListGithubOrgsRequest) (*p42.ListGithubOrgsResponse, error) {
	f.listCalled = true
	f.listReq = req
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.listResp, nil
}

func (f *fakePlan42Client) UpdateGithubOrg(_ context.Context, req *p42.UpdateGithubOrgRequest) (*p42.GithubOrg, error) {
	f.updateCalled = true
	f.updateReq = req
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	if f.updateResp != nil {
		return f.updateResp, nil
	}
	return &p42.GithubOrg{}, nil
}

func (f *fakePlan42Client) DeleteGithubOrg(_ context.Context, req *p42.DeleteGithubOrgRequest) error {
	f.deleteCalled = true
	f.deleteReq = req
	if f.deleteErr != nil {
		return f.deleteErr
	}
	return nil
}

func (f *fakePlan42Client) AddGithubOrg(_ context.Context, req *p42.AddGithubOrgRequest) (*p42.GithubOrg, error) {
	f.addCalled = true
	f.addReq = req
	if f.addErr != nil {
		return nil, f.addErr
	}
	if f.addResp != nil {
		return f.addResp, nil
	}
	return &p42.GithubOrg{}, nil
}

func (f *fakePlan42Client) SearchTasks(_ context.Context, _ *p42.SearchTasksRequest) (*p42.List[p42.Task], error) {
	return nil, nil
}

func (f *fakePlan42Client) CreateTurn(_ context.Context, _ *p42.CreateTurnRequest) (*p42.Turn, error) {
	return nil, nil
}

func (f *fakePlan42Client) GetLastTurn(_ context.Context, _ *p42.GetLastTurnRequest) (*p42.Turn, error) {
	return nil, nil
}

func (f *fakePlan42Client) UpdateTask(_ context.Context, _ *p42.UpdateTaskRequest) (*p42.Task, error) {
	return nil, nil
}

func (f *fakePlan42Client) UpdateWorkstreamTask(_ context.Context, _ *p42.UpdateWorkstreamTaskRequest) (*p42.Task, error) {
	return nil, nil
}

//nolint:unparam // helper keeps signature aligned with event fields
func installationEvent(appSlug string, appID int64, login string, externalOrgID, installationID int64, action string) *handlers.InstallationEvent {
	return &handlers.InstallationEvent{
		EventBase: handlers.EventBase{DeliveryID: "delivery-1"},
		Action:    action,
		Installation: handlers.Installation{
			ID:       installationID,
			AppID:    appID,
			AppSlug:  appSlug,
			OrgLogin: login,
			OrgID:    externalOrgID,
		},
	}
}

func newTestRegistry(appName string, appID int64, client handlers.Plan42Client) *handlers.HandlerRegistry {
	return handlers.NewHandlerRegistry(handlers.Config{
		GithubAppName: appName,
		GithubAppID:   appID,
		Plan42Client:  client,
	})
}

func TestInstallationHandler_CreatedUpdatesInstallationID(t *testing.T) {
	client := &fakePlan42Client{
		listResp: &p42.ListGithubOrgsResponse{
			Orgs: []p42.GithubOrg{
				{OrgID: testOrgID, OrgName: testInstallationLogin, ExternalOrgID: 17693182, InstallationID: 0, Version: 7},
			},
		},
	}

	registry := newTestRegistry("event-horizon-dev-kg", testAllowedAppID, client)
	evt := installationEvent("event-horizon-dev-kg", testAllowedAppID, testInstallationLogin, 17693182, 94050746, "created")
	err := registry.Handle(context.Background(), evt, nil)
	require.NoError(t, err)

	require.True(t, client.listCalled, "expected ListGithubOrgs to be called")
	require.NotNil(t, client.listReq)
	require.NotNil(t, client.listReq.Name)
	require.Equal(t, testInstallationLogin, *client.listReq.Name)
	require.True(t, client.updateCalled, "expected UpdateGithubOrg to be called")
	require.NotNil(t, client.updateReq)
	require.NotNil(t, client.updateReq.InstallationID)
	require.Equal(t, 94050746, *client.updateReq.InstallationID)
	require.Equal(t, testOrgID, client.updateReq.OrgID)
	require.Equal(t, 7, client.updateReq.Version)
	require.False(t, client.deleteCalled, "did not expect DeleteGithubOrg to be called")
}

func TestInstallationHandler_SkipsWhenSlugMismatch(t *testing.T) {
	client := &fakePlan42Client{listResp: &p42.ListGithubOrgsResponse{}}
	registry := newTestRegistry("expected-app", testAllowedAppID, client)
	evt := installationEvent("other-app", testAllowedAppID, testInstallationLogin, 17693182, 94050746, "created")
	require.NoError(t, registry.Handle(context.Background(), evt, nil))
	require.False(t, client.listCalled || client.updateCalled || client.deleteCalled, "expected no Plan42 calls on slug mismatch")
}

func TestInstallationHandler_SkipsWhenAppIDNotConfigured(t *testing.T) {
	client := &fakePlan42Client{}
	registry := newTestRegistry("event-horizon-dev-kg", 0, client)
	evt := installationEvent("event-horizon-dev-kg", testAllowedAppID, testInstallationLogin, 17693182, 94050746, "created")
	require.NoError(t, registry.Handle(context.Background(), evt, nil))
	require.False(t, client.listCalled || client.updateCalled || client.deleteCalled || client.addCalled, "expected no Plan42 calls when app id missing")
}

func TestInstallationHandler_SkipsWhenAppIDMismatch(t *testing.T) {
	client := &fakePlan42Client{}
	registry := newTestRegistry("event-horizon-dev-kg", testAllowedAppID, client)
	evt := installationEvent("event-horizon-dev-kg", testAllowedAppID+1, testInstallationLogin, 17693182, 94050746, "created")
	require.NoError(t, registry.Handle(context.Background(), evt, nil))
	require.False(t, client.listCalled || client.updateCalled || client.deleteCalled || client.addCalled, "expected no Plan42 calls when app id mismatched")
}

func TestInstallationHandler_SkipsWhenInstallationUpToDate(t *testing.T) {
	client := &fakePlan42Client{
		listResp: &p42.ListGithubOrgsResponse{
			Orgs: []p42.GithubOrg{
				{OrgID: testOrgID, OrgName: testInstallationLogin, ExternalOrgID: 17693182, InstallationID: 94050746, Version: 3},
			},
		},
	}
	registry := newTestRegistry("event-horizon-dev-kg", testAllowedAppID, client)
	evt := installationEvent("event-horizon-dev-kg", testAllowedAppID, testInstallationLogin, 17693182, 94050746, "created")
	require.NoError(t, registry.Handle(context.Background(), evt, nil))
	require.True(t, client.listCalled, "expected ListGithubOrgs to be called")
	require.False(t, client.updateCalled || client.deleteCalled, "expected no update/delete when installation id unchanged")
}

func TestInstallationHandler_AddsGithubOrgWhenNotFound(t *testing.T) {
	client := &fakePlan42Client{listResp: &p42.ListGithubOrgsResponse{Orgs: []p42.GithubOrg{}}}
	registry := newTestRegistry("event-horizon-dev-kg", testAllowedAppID, client)
	evt := installationEvent("event-horizon-dev-kg", testAllowedAppID, testInstallationLogin, 17693182, 94050746, "created")
	require.NoError(t, registry.Handle(context.Background(), evt, nil))
	require.True(t, client.listCalled, "expected list to be called")
	require.True(t, client.addCalled, "expected add to be called")
	require.Equal(t, testInstallationLogin, client.addReq.OrgName)
	require.Equal(t, 17693182, client.addReq.ExternalOrgID)
	require.Equal(t, 94050746, client.addReq.InstallationID)
}

func TestInstallationHandler_DeletesGithubOrg(t *testing.T) {
	client := &fakePlan42Client{
		listResp: &p42.ListGithubOrgsResponse{
			Orgs: []p42.GithubOrg{
				{OrgID: testOrgID, OrgName: testInstallationLogin, ExternalOrgID: 17693182, InstallationID: 94050746, Version: 11},
			},
		},
	}
	registry := newTestRegistry("event-horizon-dev-kg", testAllowedAppID, client)
	evt := installationEvent("event-horizon-dev-kg", testAllowedAppID, testInstallationLogin, 17693182, 94050746, "deleted")
	require.NoError(t, registry.Handle(context.Background(), evt, nil))
	require.True(t, client.listCalled, "expected list to be called")
	require.True(t, client.deleteCalled, "expected delete to be called")
	require.Equal(t, testOrgID, client.deleteReq.OrgID)
	require.Equal(t, 11, client.deleteReq.Version)
}

func TestInstallationHandler_DeleteSkipsWhenInstallationIDMismatch(t *testing.T) {
	client := &fakePlan42Client{
		listResp: &p42.ListGithubOrgsResponse{
			Orgs: []p42.GithubOrg{
				{OrgID: testOrgID, OrgName: testInstallationLogin, ExternalOrgID: 17693182, InstallationID: 99999, Version: 12},
			},
		},
	}
	registry := newTestRegistry("event-horizon-dev-kg", testAllowedAppID, client)
	evt := installationEvent("event-horizon-dev-kg", testAllowedAppID, testInstallationLogin, 17693182, 94050746, "deleted")
	require.NoError(t, registry.Handle(context.Background(), evt, nil))
	require.True(t, client.listCalled, "expected ListGithubOrgs to be called")
	require.False(t, client.deleteCalled || client.updateCalled, "expected no delete/update when installation ID does not match")
}

func TestInstallationHandler_EmptyAccountLogin(t *testing.T) {
	client := &fakePlan42Client{}
	registry := newTestRegistry("my-app", testAllowedAppID, client)
	evt := installationEvent("my-app", testAllowedAppID, "", 17693182, 94050746, "created")
	require.NoError(t, registry.Handle(context.Background(), evt, nil))
	require.False(t, client.listCalled || client.addCalled, "expected no calls when account login is empty")
}

func TestInstallationHandler_NilClient(t *testing.T) {
	registry := newTestRegistry("my-app", testAllowedAppID, nil)
	evt := installationEvent("my-app", testAllowedAppID, testInstallationLogin, 17693182, 94050746, "created")
	require.NoError(t, registry.Handle(context.Background(), evt, nil))
}

func TestInstallationHandler_HandlesLookupError(t *testing.T) {
	client := &fakePlan42Client{listErr: errors.New("boom")}
	registry := newTestRegistry("event-horizon", testAllowedAppID, client)
	evt := installationEvent("event-horizon", testAllowedAppID, testInstallationLogin, 17693182, 94050746, "created")
	require.NoError(t, registry.Handle(context.Background(), evt, nil))
	require.True(t, client.listCalled, "expected ListGithubOrgs to be called when lookup fails")
}
