package githubevents

import (
	"context"

	"github.com/plan42-ai/sdk-go/p42"

	"github.com/plan42-ai/github-event-handlers/githubclient"
)

// Plan42Client is the subset of the Plan42 API used by event handlers.
// Renamed from the webhook's "GithubClient" to avoid collision with the new GithubClient type.
type Plan42Client interface {
	ListGithubOrgs(ctx context.Context, req *p42.ListGithubOrgsRequest) (*p42.ListGithubOrgsResponse, error)
	UpdateGithubOrg(ctx context.Context, req *p42.UpdateGithubOrgRequest) (*p42.GithubOrg, error)
	DeleteGithubOrg(ctx context.Context, req *p42.DeleteGithubOrgRequest) error
	AddGithubOrg(ctx context.Context, req *p42.AddGithubOrgRequest) (*p42.GithubOrg, error)
	SearchTasks(ctx context.Context, req *p42.SearchTasksRequest) (*p42.List[p42.Task], error)
	CreateTurn(ctx context.Context, req *p42.CreateTurnRequest) (*p42.Turn, error)
	GetLastTurn(ctx context.Context, req *p42.GetLastTurnRequest) (*p42.Turn, error)
	UpdateTask(ctx context.Context, req *p42.UpdateTaskRequest) (*p42.Task, error)
	UpdateWorkstreamTask(ctx context.Context, req *p42.UpdateWorkstreamTaskRequest) (*p42.Task, error)
}

// Config contains options for configuring the handler registry.
//
// Compared to the webhook's current Config:
//   - GithubClient is renamed to Plan42Client (it is a Plan42 API client, not GitHub).
//   - GithubAPI is removed (now passed per-call to Handle).
//   - GithubJWTSigner is removed (webhook-internal concern).
type Config struct {
	GithubAppName     string
	GithubAppID       int64
	Plan42Client      Plan42Client
	LogPayloads       bool
	CommentTriggerStr string
	UIURL             string
}

// HandlerRegistry holds one handler function per supported EventType and dispatches
// events to the matching handler via Handle.
type HandlerRegistry struct {
	handlers map[string]func(ctx context.Context, evt Event, gh githubclient.GithubAPI)
	cfg      Config
}

// NewHandlerRegistry constructs a registry with the supplied configuration.
func NewHandlerRegistry(cfg Config) *HandlerRegistry {
	r := &HandlerRegistry{
		handlers: make(map[string]func(ctx context.Context, evt Event, gh githubclient.GithubAPI)),
		cfg:      cfg,
	}
	r.handlers["installation"] = newInstallationHandler(cfg)
	return r
}

// Register adds a handler for the given event type. Subsequent tasks use this to wire
// up concrete handler implementations.
func (r *HandlerRegistry) Register(eventType string, handler func(ctx context.Context, evt Event, gh githubclient.GithubAPI)) {
	r.handlers[eventType] = handler
}

// Handle dispatches evt to the registered handler for evt.EventType(). Returns
// ErrUnknownEvent if no handler is registered for that type. Returns nil on
// successful dispatch or when the registry is nil. Individual handler implementations
// log their own internal errors rather than surfacing them through this return value.
func (r *HandlerRegistry) Handle(ctx context.Context, evt Event, gh githubclient.GithubAPI) error {
	if r == nil {
		return nil
	}
	handler, ok := r.handlers[evt.EventType()]
	if !ok {
		return ErrUnknownEvent
	}
	handler(ctx, evt, gh)
	return nil
}
