package githubevents_test

import (
	"context"
	"errors"
	"testing"

	githubevents "github.com/plan42-ai/github-event-handlers"
	"github.com/plan42-ai/sdk-go/p42"
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

func (f *fakePlan42Client) ListGithubOrgs(_ context.Context, req *p42.ListGithubOrgsRequest) (
	*p42.ListGithubOrgsResponse,
	error,
) {
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

// Task-related methods required by the Plan42Client interface but unused by the
// installation handler. Included here to satisfy the interface.

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
func installationEvent(appSlug string, appID int64, login string, externalOrgID, installationID int64, action string) *githubevents.InstallationEvent {
	return &githubevents.InstallationEvent{
		EventBase: githubevents.EventBase{DeliveryID: "delivery-1"},
		Action:    action,
		Installation: githubevents.Installation{
			ID:       installationID,
			AppID:    appID,
			AppSlug:  appSlug,
			OrgLogin: login,
			OrgID:    externalOrgID,
		},
	}
}

func newTestRegistry(appName string, appID int64, client githubevents.Plan42Client) *githubevents.HandlerRegistry {
	return githubevents.NewHandlerRegistry(githubevents.Config{
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
	if err != nil {
		t.Fatalf("Handle returned unexpected error: %v", err)
	}

	if !client.listCalled {
		t.Fatalf("expected ListGithubOrgs to be called")
	}
	if client.listReq == nil || client.listReq.Name == nil || *client.listReq.Name != testInstallationLogin {
		t.Fatalf("expected list request to filter by login, got %#v", client.listReq)
	}
	if !client.updateCalled {
		t.Fatalf("expected UpdateGithubOrg to be called")
	}
	if client.updateReq == nil || client.updateReq.InstallationID == nil {
		t.Fatalf("expected installation ID to be set on update request")
	}
	if got := *client.updateReq.InstallationID; got != 94050746 {
		t.Fatalf("expected installation id 94050746, got %d", got)
	}
	if client.updateReq.OrgID != testOrgID {
		t.Fatalf("expected org id org-123, got %s", client.updateReq.OrgID)
	}
	if client.updateReq.Version != 7 {
		t.Fatalf("expected version 7, got %d", client.updateReq.Version)
	}
	if client.deleteCalled {
		t.Fatalf("did not expect DeleteGithubOrg to be called")
	}
}

func TestInstallationHandler_SkipsWhenSlugMismatch(t *testing.T) {
	client := &fakePlan42Client{
		listResp: &p42.ListGithubOrgsResponse{},
	}
	registry := newTestRegistry("expected-app", testAllowedAppID, client)
	evt := installationEvent("other-app", testAllowedAppID, testInstallationLogin, 17693182, 94050746, "created")
	err := registry.Handle(context.Background(), evt, nil)
	if err != nil {
		t.Fatalf("Handle returned unexpected error: %v", err)
	}

	if client.listCalled {
		t.Fatalf("expected ListGithubOrgs not to be called on slug mismatch")
	}
	if client.updateCalled {
		t.Fatalf("expected UpdateGithubOrg not to be called on slug mismatch")
	}
	if client.deleteCalled {
		t.Fatalf("expected DeleteGithubOrg not to be called on slug mismatch")
	}
}

func TestInstallationHandler_SkipsWhenAppIDNotConfigured(t *testing.T) {
	client := &fakePlan42Client{}
	registry := newTestRegistry("event-horizon-dev-kg", 0, client)
	evt := installationEvent("event-horizon-dev-kg", testAllowedAppID, testInstallationLogin, 17693182, 94050746, "created")
	err := registry.Handle(context.Background(), evt, nil)
	if err != nil {
		t.Fatalf("Handle returned unexpected error: %v", err)
	}

	if client.listCalled {
		t.Fatalf("expected ListGithubOrgs not to be called when app id missing")
	}
	if client.updateCalled {
		t.Fatalf("expected UpdateGithubOrg not to be called when app id missing")
	}
	if client.deleteCalled {
		t.Fatalf("expected DeleteGithubOrg not to be called when app id missing")
	}
	if client.addCalled {
		t.Fatalf("expected AddGithubOrg not to be called when app id missing")
	}
}

func TestInstallationHandler_SkipsWhenAppIDMismatch(t *testing.T) {
	client := &fakePlan42Client{}
	registry := newTestRegistry("event-horizon-dev-kg", testAllowedAppID, client)
	evt := installationEvent("event-horizon-dev-kg", testAllowedAppID+1, testInstallationLogin, 17693182, 94050746, "created")
	err := registry.Handle(context.Background(), evt, nil)
	if err != nil {
		t.Fatalf("Handle returned unexpected error: %v", err)
	}

	if client.listCalled {
		t.Fatalf("expected ListGithubOrgs not to be called when app id mismatched")
	}
	if client.updateCalled {
		t.Fatalf("expected UpdateGithubOrg not to be called when app id mismatched")
	}
	if client.deleteCalled {
		t.Fatalf("expected DeleteGithubOrg not to be called when app id mismatched")
	}
	if client.addCalled {
		t.Fatalf("expected AddGithubOrg not to be called when app id mismatched")
	}
}

func TestInstallationHandler_SkipsWhenInstallationUpToDate(t *testing.T) {
	client := &fakePlan42Client{
		listResp: &p42.ListGithubOrgsResponse{
			Orgs: []p42.GithubOrg{
				{
					OrgID:          testOrgID,
					OrgName:        testInstallationLogin,
					ExternalOrgID:  17693182,
					InstallationID: 94050746,
					Version:        3,
				},
			},
		},
	}
	registry := newTestRegistry("event-horizon-dev-kg", testAllowedAppID, client)
	evt := installationEvent("event-horizon-dev-kg", testAllowedAppID, testInstallationLogin, 17693182, 94050746, "created")
	err := registry.Handle(context.Background(), evt, nil)
	if err != nil {
		t.Fatalf("Handle returned unexpected error: %v", err)
	}

	if !client.listCalled {
		t.Fatalf("expected ListGithubOrgs to be called")
	}
	if client.updateCalled {
		t.Fatalf("expected UpdateGithubOrg not to be called when installation id unchanged")
	}
	if client.deleteCalled {
		t.Fatalf("expected DeleteGithubOrg not to be called when installation id unchanged")
	}
}

func TestInstallationHandler_AddsGithubOrgWhenNotFound(t *testing.T) {
	client := &fakePlan42Client{
		listResp: &p42.ListGithubOrgsResponse{
			Orgs: []p42.GithubOrg{},
		},
	}

	registry := newTestRegistry("event-horizon-dev-kg", testAllowedAppID, client)
	evt := installationEvent("event-horizon-dev-kg", testAllowedAppID, testInstallationLogin, 17693182, 94050746, "created")
	err := registry.Handle(context.Background(), evt, nil)
	if err != nil {
		t.Fatalf("Handle returned unexpected error: %v", err)
	}

	if !client.listCalled {
		t.Fatalf("expected ListGithubOrgs to be called before adding org")
	}
	if !client.addCalled {
		t.Fatalf("expected AddGithubOrg to be called when org not found")
	}
	if client.addReq == nil {
		t.Fatalf("expected add request to be captured")
	}
	if client.addReq.OrgName != testInstallationLogin {
		t.Fatalf("expected org name %s, got %s", testInstallationLogin, client.addReq.OrgName)
	}
	if client.addReq.ExternalOrgID != 17693182 {
		t.Fatalf("expected external org id 17693182, got %d", client.addReq.ExternalOrgID)
	}
	if client.addReq.InstallationID != 94050746 {
		t.Fatalf("expected installation id 94050746, got %d", client.addReq.InstallationID)
	}
}

func TestInstallationHandler_DeletesGithubOrg(t *testing.T) {
	client := &fakePlan42Client{
		listResp: &p42.ListGithubOrgsResponse{
			Orgs: []p42.GithubOrg{
				{
					OrgID:          testOrgID,
					OrgName:        testInstallationLogin,
					ExternalOrgID:  17693182,
					InstallationID: 94050746,
					Version:        11,
				},
			},
		},
	}
	registry := newTestRegistry("event-horizon-dev-kg", testAllowedAppID, client)
	evt := installationEvent("event-horizon-dev-kg", testAllowedAppID, testInstallationLogin, 17693182, 94050746, "deleted")
	err := registry.Handle(context.Background(), evt, nil)
	if err != nil {
		t.Fatalf("Handle returned unexpected error: %v", err)
	}

	if !client.listCalled {
		t.Fatalf("expected ListGithubOrgs to be called for deletion")
	}
	if client.updateCalled {
		t.Fatalf("did not expect UpdateGithubOrg when handling deletion")
	}
	if !client.deleteCalled {
		t.Fatalf("expected DeleteGithubOrg to be called")
	}
	if client.deleteReq == nil {
		t.Fatalf("expected delete request to be captured")
	}
	if client.deleteReq.OrgID != testOrgID {
		t.Fatalf("expected delete org id org-123, got %s", client.deleteReq.OrgID)
	}
	if client.deleteReq.Version != 11 {
		t.Fatalf("expected delete version 11, got %d", client.deleteReq.Version)
	}
}

func TestInstallationHandler_EmptyAccountLogin(t *testing.T) {
	client := &fakePlan42Client{}
	registry := newTestRegistry("my-app", testAllowedAppID, client)
	evt := installationEvent("my-app", testAllowedAppID, "", 17693182, 94050746, "created")
	err := registry.Handle(context.Background(), evt, nil)
	if err != nil {
		t.Fatalf("Handle returned unexpected error: %v", err)
	}

	if client.listCalled {
		t.Fatalf("expected ListGithubOrgs not to be called when account login is empty")
	}
	if client.addCalled {
		t.Fatalf("expected AddGithubOrg not to be called when account login is empty")
	}
}

func TestInstallationHandler_NilClient(t *testing.T) {
	registry := newTestRegistry("my-app", testAllowedAppID, nil)
	evt := installationEvent("my-app", testAllowedAppID, testInstallationLogin, 17693182, 94050746, "created")
	err := registry.Handle(context.Background(), evt, nil)
	if err != nil {
		t.Fatalf("Handle returned unexpected error: %v", err)
	}
	// Should not panic; the handler logs and returns when client is nil.
}

func TestInstallationHandler_HandlesLookupError(t *testing.T) {
	client := &fakePlan42Client{
		listErr: errors.New("boom"),
	}
	registry := newTestRegistry("event-horizon", testAllowedAppID, client)
	evt := installationEvent("event-horizon", testAllowedAppID, testInstallationLogin, 17693182, 94050746, "created")
	err := registry.Handle(context.Background(), evt, nil)
	if err != nil {
		t.Fatalf("Handle returned unexpected error: %v", err)
	}

	if !client.listCalled {
		t.Fatalf("expected ListGithubOrgs to be called when lookup fails")
	}
}
