package githubevents

import "time"

// Repository identifies a repository. FullName is "owner/name", Name is the short name,
// Org is the owner login (a GitHub user or organization login). All three fields are
// populated for every event.
type Repository struct {
	FullName string
	Name     string
	Org      string
}

// Comment carries the body and author of an issue comment or PR review comment.
// Login is the commenter's GitHub login.
type Comment struct {
	Body  string
	Login string
}

// Issue carries the issue-level fields the handlers read. IsPullRequest is true when the
// underlying issue is the issue-side projection of a pull request.
type Issue struct {
	Number        int
	State         string // "open" or "closed"
	IsPullRequest bool
}

// PullRequest carries the PR-level fields the handlers read.
// Login is the PR author's GitHub login.
// UpdatedAt is *time.Time so the handler can distinguish "GitHub did not provide a timestamp"
// (nil) from a real timestamp. The Go zero value of time.Time does not mean "not set";
// translators that receive a missing or zero timestamp from go-github must store nil here.
type PullRequest struct {
	ID        int64
	Number    int
	State     string // "open" or "closed"
	Merged    bool
	Draft     bool
	HTMLURL   string
	UpdatedAt *time.Time
	Login     string
}

// Review carries a PR review's body and author.
// Body is *string because GitHub allows reviews with no body (e.g. an "approve" review with
// no comment); a nil Body causes the review handler to short-circuit before evaluating
// trigger commands.
// Login is the reviewer's GitHub login.
type Review struct {
	Body  *string
	Login string
}

// Installation carries GitHub App installation context. Used only by InstallationEvent.
// OrgLogin and OrgID identify the org or user account this installation targets - the
// "account" the App is installed onto.
type Installation struct {
	ID       int64
	AppID    int64
	AppSlug  string
	OrgLogin string
	OrgID    int64
}
