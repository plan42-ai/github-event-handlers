package githubevents

// InstallationEvent is delivered only via webhook. The Events API does not include this
// event type, and the runner never constructs one.
type InstallationEvent struct {
	EventBase
	Action       string // "created", "deleted", or one of GitHub's other installation actions
	Installation Installation
}

// EventType returns "installation".
func (*InstallationEvent) EventType() string { return "installation" }

// IssueCommentEvent is fired when a comment is created on an issue or pull request. The
// runner only forwards "created" actions; the handler additionally filters on
// Action == "created".
type IssueCommentEvent struct {
	EventBase
	Action     string
	Issue      Issue
	Comment    Comment
	Repository Repository
}

// EventType returns "issue_comment".
func (*IssueCommentEvent) EventType() string { return "issue_comment" }

// PullRequestReviewCommentEvent is fired when an inline review comment is created on a PR.
type PullRequestReviewCommentEvent struct {
	EventBase
	Action      string
	Comment     Comment
	PullRequest PullRequest
	Repository  Repository
}

// EventType returns "pull_request_review_comment".
func (*PullRequestReviewCommentEvent) EventType() string { return "pull_request_review_comment" }

// PullRequestReviewEvent is fired when a PR review is submitted, edited, or dismissed. The
// handler only acts on Action == "submitted".
type PullRequestReviewEvent struct {
	EventBase
	Action      string
	Review      Review
	PullRequest PullRequest
	Repository  Repository
}

// EventType returns "pull_request_review".
func (*PullRequestReviewEvent) EventType() string { return "pull_request_review" }

// PullRequestEvent is fired on PR state changes. The handler does not filter on Action; it
// reads PullRequest.State, PullRequest.Merged, and PullRequest.Draft to decide what to do.
type PullRequestEvent struct {
	EventBase
	Action      string
	Number      int
	PullRequest PullRequest
	Repository  Repository
}

// EventType returns "pull_request".
func (*PullRequestEvent) EventType() string { return "pull_request" }
