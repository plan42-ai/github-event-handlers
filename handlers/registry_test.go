package handlers_test

import (
	"context"
	"errors"
	"testing"

	"github.com/plan42-ai/github-event-handlers/github"
	"github.com/plan42-ai/github-event-handlers/handlers"
)

func TestHandlerRegistry_NilRegistry(t *testing.T) {
	t.Parallel()
	var r *handlers.HandlerRegistry
	evt := &handlers.IssueCommentEvent{
		EventBase: handlers.EventBase{DeliveryID: testDeliveryID},
		Action:    testActionCreated,
	}
	err := r.Handle(context.Background(), evt, nil)
	if err != nil {
		t.Errorf("Handle on nil registry returned error: %v", err)
	}
}

func TestHandlerRegistry_UnknownEvent(t *testing.T) {
	t.Parallel()
	r := handlers.NewHandlerRegistry(handlers.Config{})
	evt := &handlers.IssueCommentEvent{
		EventBase: handlers.EventBase{DeliveryID: testDeliveryID},
		Action:    testActionCreated,
	}
	err := r.Handle(context.Background(), evt, nil)
	if !errors.Is(err, handlers.ErrUnknownEvent) {
		t.Errorf("expected ErrUnknownEvent, got %v", err)
	}
}

func TestHandlerRegistry_Dispatch(t *testing.T) {
	t.Parallel()

	cfg := handlers.Config{GithubAppName: "test-app"}
	r := handlers.NewHandlerRegistry(cfg)

	var called bool
	var receivedEvt handlers.Event
	var receivedGH github.API
	r.Register("issue_comment", func(_ context.Context, evt handlers.Event, gh github.API) {
		called = true
		receivedEvt = evt
		receivedGH = gh
	})

	evt := &handlers.IssueCommentEvent{
		EventBase: handlers.EventBase{DeliveryID: "d-test"},
		Action:    testActionCreated,
		Comment:   handlers.Comment{Body: "/Plan42", Login: "alice"},
	}

	mockGH := &mockAPI{}

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

	r := handlers.NewHandlerRegistry(handlers.Config{})

	var commentCalled, prCalled bool
	r.Register("issue_comment", func(_ context.Context, _ handlers.Event, _ github.API) {
		commentCalled = true
	})
	r.Register("pull_request", func(_ context.Context, _ handlers.Event, _ github.API) {
		prCalled = true
	})

	evt := &handlers.PullRequestEvent{
		EventBase: handlers.EventBase{DeliveryID: "d-pr"},
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

// mockAPI is a minimal mock for testing dispatch.
type mockAPI struct{}

func (m *mockAPI) FindIssueCommentWithMarker(_ context.Context, _, _ string, _ int, _ string) (*github.IssueComment, error) {
	return nil, nil
}

func (m *mockAPI) CreateIssueComment(_ context.Context, _, _ string, _ int, _ string) (*github.IssueComment, error) {
	return nil, nil
}

func (m *mockAPI) UpdateIssueComment(_ context.Context, _, _ string, _ int64, _ string) (*github.IssueComment, error) {
	return nil, nil
}

func (m *mockAPI) GetPullRequest(_ context.Context, _, _ string, _ int) (*github.PullRequest, error) {
	return nil, nil
}

func (m *mockAPI) GetInstallationToken(_ context.Context, _ int64) (string, error) {
	return "", nil
}
