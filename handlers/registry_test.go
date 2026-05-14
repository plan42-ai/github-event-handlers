package handlers_test

import (
	"context"
	"testing"

	"github.com/plan42-ai/github-event-handlers/github"
	"github.com/plan42-ai/github-event-handlers/handlers"
	"github.com/stretchr/testify/require"
)

func TestHandlerRegistry_NilRegistry(t *testing.T) {
	t.Parallel()
	var r *handlers.HandlerRegistry
	evt := &handlers.IssueCommentEvent{
		EventBase: handlers.EventBase{DeliveryID: testDeliveryID},
		Action:    testActionCreated,
	}
	err := r.Handle(context.Background(), evt, nil)
	require.NoError(t, err)
}

func TestHandlerRegistry_UnknownEvent(t *testing.T) {
	t.Parallel()
	r := handlers.NewHandlerRegistry(handlers.Config{})
	evt := &unknownTestEvent{EventBase: handlers.EventBase{DeliveryID: testDeliveryID}}
	err := r.Handle(context.Background(), evt, nil)
	require.ErrorIs(t, err, handlers.ErrUnknownEvent)
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
	require.NoError(t, err)
	require.True(t, called, "handler was not called")
	require.Equal(t, evt, receivedEvt, "handler received wrong event")
	require.Equal(t, mockGH, receivedGH, "handler received wrong GithubAPI")
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
	require.NoError(t, err)
	require.False(t, commentCalled, "issue_comment handler should not have been called")
	require.True(t, prCalled, "pull_request handler should have been called")
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

// unknownTestEvent is an event type not registered in the handler registry.
type unknownTestEvent struct {
	handlers.EventBase
}

func (*unknownTestEvent) EventType() string { return "unknown_test_event" }
