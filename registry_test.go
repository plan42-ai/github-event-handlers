package githubevents_test

import (
	"context"
	"errors"
	"testing"

	githubevents "github.com/plan42-ai/github-event-handlers"
	"github.com/plan42-ai/github-event-handlers/githubclient"
)

func TestHandlerRegistry_NilRegistry(t *testing.T) {
	t.Parallel()
	var r *githubevents.HandlerRegistry
	evt := &githubevents.IssueCommentEvent{
		EventBase: githubevents.EventBase{DeliveryID: testDeliveryID},
		Action:    testActionCreated,
	}
	err := r.Handle(context.Background(), evt, nil)
	if err != nil {
		t.Errorf("Handle on nil registry returned error: %v", err)
	}
}

func TestHandlerRegistry_UnknownEvent(t *testing.T) {
	t.Parallel()
	r := githubevents.NewHandlerRegistry(githubevents.Config{})
	evt := &githubevents.IssueCommentEvent{
		EventBase: githubevents.EventBase{DeliveryID: testDeliveryID},
		Action:    testActionCreated,
	}
	err := r.Handle(context.Background(), evt, nil)
	if !errors.Is(err, githubevents.ErrUnknownEvent) {
		t.Errorf("expected ErrUnknownEvent, got %v", err)
	}
}

func TestHandlerRegistry_Dispatch(t *testing.T) {
	t.Parallel()

	cfg := githubevents.Config{GithubAppName: "test-app"}
	r := githubevents.NewHandlerRegistry(cfg)

	// Register a test handler that records what it received.
	var called bool
	var receivedEvt githubevents.Event
	var receivedGH githubclient.GithubAPI
	r.Register("issue_comment", func(_ context.Context, evt githubevents.Event, gh githubclient.GithubAPI) {
		called = true
		receivedEvt = evt
		receivedGH = gh
	})

	evt := &githubevents.IssueCommentEvent{
		EventBase: githubevents.EventBase{DeliveryID: "d-test"},
		Action:    testActionCreated,
		Comment:   githubevents.Comment{Body: "/Plan42", Login: "alice"},
	}

	mockGH := &mockGithubAPI{}

	err := r.Handle(context.Background(), evt, mockGH)
	if err != nil {
		t.Fatalf("Handle returned unexpected error: %v", err)
	}
	if !called {
		t.Fatal("handler was not called")
	}
	if receivedEvt != evt {
		t.Error("handler received wrong event")
	}
	if receivedGH != mockGH {
		t.Error("handler received wrong GithubAPI")
	}
}

func TestHandlerRegistry_DispatchCorrectHandler(t *testing.T) {
	t.Parallel()

	r := githubevents.NewHandlerRegistry(githubevents.Config{})

	var commentCalled, prCalled bool
	r.Register("issue_comment", func(_ context.Context, _ githubevents.Event, _ githubclient.GithubAPI) {
		commentCalled = true
	})
	r.Register("pull_request", func(_ context.Context, _ githubevents.Event, _ githubclient.GithubAPI) {
		prCalled = true
	})

	// Dispatch a pull_request event; only the PR handler should fire.
	evt := &githubevents.PullRequestEvent{
		EventBase: githubevents.EventBase{DeliveryID: "d-pr"},
		Action:    "opened",
	}
	err := r.Handle(context.Background(), evt, nil)
	if err != nil {
		t.Fatalf("Handle returned unexpected error: %v", err)
	}
	if commentCalled {
		t.Error("issue_comment handler should not have been called")
	}
	if !prCalled {
		t.Error("pull_request handler should have been called")
	}
}

// mockGithubAPI is a minimal mock for testing dispatch.
type mockGithubAPI struct{}

func (m *mockGithubAPI) FindIssueCommentWithMarker(_ context.Context, _, _ string, _ int, _ string) (*githubclient.IssueComment, error) {
	return nil, nil
}

func (m *mockGithubAPI) CreateIssueComment(_ context.Context, _, _ string, _ int, _ string) (*githubclient.IssueComment, error) {
	return nil, nil
}

func (m *mockGithubAPI) UpdateIssueComment(_ context.Context, _, _ string, _ int64, _ string) (*githubclient.IssueComment, error) {
	return nil, nil
}

func (m *mockGithubAPI) GetPullRequest(_ context.Context, _, _ string, _ int) (*githubclient.PullRequest, error) {
	return nil, nil
}
