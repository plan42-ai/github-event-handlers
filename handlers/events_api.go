package handlers

import (
	"context"
	"log/slog"
	"strings"

	"github.com/google/go-github/v81/github"
	"github.com/google/uuid"
	ghapi "github.com/plan42-ai/github-event-handlers/github"
)

// ParseEventsAPI translates a go-github Events API envelope into a shared
// library Event. It calls env.ParsePayload() internally, type-switches on
// the result, and returns the matching concrete Event value. Unsupported
// event types (including InstallationEvent, which the Events API does not
// deliver) return nil, nil so callers can skip them.
//
// A fresh random UUID is generated for the delivery ID, matching the format
// webhooks use for X-GitHub-Delivery.
func ParseEventsAPI(ctx context.Context, env *github.Event, gh ghapi.API) (Event, error) {
	payload, err := env.ParsePayload()
	if err != nil {
		return nil, err
	}

	deliveryID := uuid.NewString()
	repo := eventsAPIRepository(env)

	switch p := payload.(type) {
	case *github.IssueCommentEvent:
		return eventsAPIToIssueComment(deliveryID, p, repo), nil
	case *github.PullRequestReviewCommentEvent:
		return eventsAPIToReviewComment(deliveryID, p, repo), nil
	case *github.PullRequestReviewEvent:
		return eventsAPIToReview(deliveryID, p, repo), nil
	case *github.PullRequestEvent:
		return eventsAPIToPullRequest(ctx, deliveryID, p, repo, gh), nil
	default:
		// Unsupported event type; caller should skip.
		return nil, nil
	}
}

func eventsAPIToIssueComment(deliveryID string, p *github.IssueCommentEvent, repo Repository) *IssueCommentEvent {
	return &IssueCommentEvent{
		EventBase: EventBase{DeliveryID: deliveryID},
		Action:    p.GetAction(),
		Comment: Comment{
			Body:  p.GetComment().GetBody(),
			Login: p.GetComment().GetUser().GetLogin(),
		},
		Issue: Issue{
			Number:        p.GetIssue().GetNumber(),
			State:         p.GetIssue().GetState(),
			IsPullRequest: p.GetIssue().IsPullRequest(),
		},
		Repository: repo,
	}
}

func eventsAPIToReviewComment(deliveryID string, p *github.PullRequestReviewCommentEvent, repo Repository) *PullRequestReviewCommentEvent {
	return &PullRequestReviewCommentEvent{
		EventBase: EventBase{DeliveryID: deliveryID},
		Action:    p.GetAction(),
		Comment: Comment{
			Body:  p.GetComment().GetBody(),
			Login: p.GetComment().GetUser().GetLogin(),
		},
		PullRequest: eventsAPIPullRequest(p.GetPullRequest()),
		Repository:  repo,
	}
}

func eventsAPIToReview(deliveryID string, p *github.PullRequestReviewEvent, repo Repository) *PullRequestReviewEvent {
	var body *string
	if p.GetReview().Body != nil {
		v := *p.GetReview().Body
		body = &v
	}

	// The Events API delivers new reviews with action "created", but the
	// shared handler expects the webhook-style action "submitted". Normalize
	// so the comments handler processes review-body trigger commands.
	action := p.GetAction()
	if strings.EqualFold(action, "created") {
		action = "submitted"
	}

	return &PullRequestReviewEvent{
		EventBase: EventBase{DeliveryID: deliveryID},
		Action:    action,
		Review: Review{
			Body:  body,
			Login: p.GetReview().GetUser().GetLogin(),
		},
		PullRequest: eventsAPIPullRequest(p.GetPullRequest()),
		Repository:  repo,
	}
}

func eventsAPIToPullRequest(ctx context.Context, deliveryID string, p *github.PullRequestEvent, repo Repository, gh ghapi.API) *PullRequestEvent {
	apiPR := p.GetPullRequest()
	number := p.GetNumber()

	pr := PullRequest{
		ID:     apiPR.GetID(),
		Number: apiPR.GetNumber(),
		State:  apiPR.GetState(),
		Login:  apiPR.GetUser().GetLogin(),
	}

	full, err := gh.GetPullRequest(ctx, repo.Org, repo.Name, number)
	if err != nil {
		slog.ErrorContext(ctx, "events api: failed to fetch pull request; using partial payload",
			"deliveryID", deliveryID, "owner", repo.Org, "repo", repo.Name, "number", number, "error", err)
	} else {
		pr.ID = full.ID
		pr.Number = full.Number
		pr.State = full.State
		pr.Merged = full.Merged
		pr.Draft = full.Draft
		pr.HTMLURL = full.HTMLURL
		pr.UpdatedAt = full.UpdatedAt
		pr.Login = full.User.Login
	}

	return &PullRequestEvent{
		EventBase:   EventBase{DeliveryID: deliveryID},
		Action:      p.GetAction(),
		Number:      number,
		PullRequest: pr,
		Repository:  repo,
	}
}

// eventsAPIRepository extracts Repository from the Events API envelope.
// The envelope's Repo.Name carries "owner/name"; FullName and Owner are nil.
func eventsAPIRepository(env *github.Event) Repository {
	name := env.GetRepo().GetName()
	org, repoName, _ := strings.Cut(name, "/")
	return Repository{
		FullName: name,
		Org:      org,
		Name:     repoName,
	}
}

// eventsAPIPullRequest builds a PullRequest from the Events API payload's PR.
func eventsAPIPullRequest(pr *github.PullRequest) PullRequest {
	return PullRequest{
		ID:     pr.GetID(),
		Number: pr.GetNumber(),
		State:  pr.GetState(),
		Login:  pr.GetUser().GetLogin(),
	}
}
