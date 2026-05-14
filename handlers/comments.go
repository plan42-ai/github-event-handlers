package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/plan42-ai/github-event-handlers/github"
	"github.com/plan42-ai/github-event-handlers/tokens"
	"github.com/plan42-ai/sdk-go/p42"
)

func newCommentsHandler(cfg Config) func(ctx context.Context, evt Event, gh github.API) {
	return (&commentsHandler{
		command:      strings.TrimSpace(cfg.CommentTriggerStr),
		tasks:        cfg.Plan42Client,
		UIURL:        strings.TrimSpace(cfg.UIURL),
		logPayloads:  cfg.LogPayloads,
		tokens:       cfg.TokenFetcher,
		useGithubApp: cfg.UseGithubApp,
	}).handle
}

type commentsHandler struct {
	command      string
	tasks        Plan42Client
	UIURL        string
	logPayloads  bool
	tokens       tokens.Fetcher
	useGithubApp bool
}

func (h *commentsHandler) handle(ctx context.Context, evt Event, gh github.API) { //nolint:cyclop
	if evt == nil {
		return
	}
	if h.tasks == nil {
		slog.WarnContext(ctx, "plan42 client not configured; skipping comment trigger",
			"delivery_id", evt.GetDeliveryID(),
			"event", evt.EventType(),
		)
		return
	}

	deliveryID := evt.GetDeliveryID()
	attrs := logPayload(h.logPayloads, evt, "delivery_id", deliveryID, "event", evt.EventType())
	slog.InfoContext(ctx, "received comments event", attrs...)

	var data *commentEventData

	switch e := evt.(type) {
	case *IssueCommentEvent:
		data = h.issueCommentData(ctx, deliveryID, e, gh)
	case *PullRequestReviewCommentEvent:
		data = h.reviewCommentData(ctx, deliveryID, e)
	case *PullRequestReviewEvent:
		data = h.pullRequestReviewData(ctx, deliveryID, e)
	default:
		slog.ErrorContext(ctx, "comments handler received unexpected event type",
			"delivery_id", deliveryID,
			"event_type", evt.EventType(),
		)
		return
	}

	if data == nil {
		return
	}

	task := h.lookupTask(ctx, deliveryID, data.PullRequestID)
	if task == nil {
		return
	}
	if task.Deleted {
		slog.InfoContext(ctx, "task associated with pull request is deleted; skipping",
			"delivery_id", deliveryID,
			"task_id", task.TaskID,
		)
		return
	}

	if !h.isCommentByPRAuthor(ctx, deliveryID, data, gh) {
		return
	}

	lastTurn, err := h.tasks.GetLastTurn(ctx, &p42.GetLastTurnRequest{
		TenantID:     task.TenantID,
		TaskID:       task.TaskID,
		WorkstreamID: task.WorkstreamID,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to load last turn",
			"delivery_id", deliveryID,
			"task_id", task.TaskID,
			"error", err,
		)
		return
	}
	if lastTurn == nil {
		slog.WarnContext(ctx, "no existing turns for task; cannot create new turn",
			"delivery_id", deliveryID,
			"task_id", task.TaskID,
		)
		return
	}

	req := &p42.CreateTurnRequest{
		TenantID:     task.TenantID,
		TaskID:       task.TaskID,
		TurnIndex:    lastTurn.TurnIndex + 1,
		TaskVersion:  task.Version,
		WorkstreamID: task.WorkstreamID,
		Prompt:       "User requested changes via Github comments.",
	}

	if _, err := h.tasks.CreateTurn(ctx, req); err != nil {
		var conflict *p42.ConflictError
		if errors.As(err, &conflict) {
			slog.InfoContext(ctx, "create turn conflict",
				"delivery_id", deliveryID,
				"task_id", task.TaskID,
				"error", err,
			)

			if !h.useGithubApp {
				return
			}

			fallback := conflictTarget{owner: data.Owner, repo: data.Repo, issueNumber: data.IssueNumber, installationID: data.InstallationID}
			for _, target := range h.conflictTargets(ctx, deliveryID, task, fallback) {
				h.postConflictComment(ctx, gh, deliveryID, target.owner, target.repo, target.issueNumber, task.TaskID, target.installationID)
			}
			return
		}
		slog.ErrorContext(ctx, "failed to create new turn",
			"delivery_id", deliveryID,
			"task_id", task.TaskID,
			"error", err,
		)
		return
	}

	slog.InfoContext(ctx, "created new turn from GitHub comment",
		"delivery_id", deliveryID,
		"task_id", task.TaskID,
		"turn_index", req.TurnIndex,
	)
}

type commentEventData struct {
	CommentBody    string
	CommentAuthor  string
	PRAuthor       string
	Owner          string
	Repo           string
	IssueNumber    int
	PullRequestID  int64
	InstallationID int64
}

type conflictTarget struct {
	owner          string
	repo           string
	issueNumber    int
	installationID int64
}

func (h *commentsHandler) isCommentByPRAuthor(
	ctx context.Context,
	deliveryID string,
	data *commentEventData,
	gh github.API,
) bool {
	if strings.EqualFold(data.CommentAuthor, data.PRAuthor) {
		return true
	}

	slog.InfoContext(ctx, "comment author does not match pull request author",
		"delivery_id", deliveryID,
		"comment_author", data.CommentAuthor,
		"pr_author", data.PRAuthor,
	)

	h.postAuthorMismatchComment(ctx, deliveryID, data, gh)

	return false
}

func (h *commentsHandler) postAuthorMismatchComment(
	ctx context.Context,
	deliveryID string,
	data *commentEventData,
	gh github.API,
) {
	if !h.useGithubApp {
		return
	}

	ctx, ok := h.ctxWithInstallationToken(ctx, gh, deliveryID, data.InstallationID)
	if !ok {
		return
	}

	body := fmt.Sprintf(
		"@%s I am unable to process your request because you do not own the Plan 42 task associated with this pr. Please coordinate with @%s if you would like me to make changes.",
		data.CommentAuthor,
		data.PRAuthor,
	)
	if _, err := gh.CreateIssueComment(ctx, data.Owner, data.Repo, data.IssueNumber, body); err != nil {
		slog.ErrorContext(ctx, "failed to post mismatch comment",
			"delivery_id", deliveryID,
			"owner", data.Owner,
			"repo", data.Repo,
			"issue_number", data.IssueNumber,
			"error", err,
		)
	}
}

func (h *commentsHandler) shouldHandleComment(commentBody string) bool {
	if commentBody == "" || h.command == "" {
		return false
	}
	if strings.HasPrefix(commentBody, "<!-- event-horizon") {
		return false
	}
	return slices.Contains(strings.Fields(commentBody), h.command)
}

func (h *commentsHandler) issueCommentData(
	ctx context.Context,
	deliveryID string,
	evt *IssueCommentEvent,
	gh github.API,
) *commentEventData {
	if evt == nil {
		return nil
	}
	if !evt.Issue.IsPullRequest {
		slog.InfoContext(ctx, "comment is not associated with a pull request; skipping",
			"delivery_id", deliveryID,
		)
		return nil
	}
	if !strings.EqualFold(evt.Action, "created") {
		return nil
	}
	if !h.shouldHandleComment(evt.Comment.Body) {
		return nil
	}

	owner, repo := repoOwnerAndName(evt.Repository)
	issueNumber := evt.Issue.Number

	installationID := int64(0)
	if evt.InstallationID != nil {
		installationID = *evt.InstallationID
	} else if h.useGithubApp {
		installationID = h.lookupInstallationID(ctx, deliveryID, owner)
	}

	if h.useGithubApp {
		var ok bool
		ctx, ok = h.ctxWithInstallationToken(ctx, gh, deliveryID, installationID)
		if !ok {
			return nil
		}
	}

	pr := h.fetchPullRequest(ctx, deliveryID, gh, owner, repo, issueNumber, evt.Issue.State)
	if pr == nil {
		return nil
	}

	data := &commentEventData{
		CommentBody:    evt.Comment.Body,
		CommentAuthor:  evt.Comment.Login,
		PRAuthor:       pr.User.Login,
		Owner:          owner,
		Repo:           repo,
		IssueNumber:    issueNumber,
		PullRequestID:  pr.ID,
		InstallationID: installationID,
	}
	h.populateInstallationID(ctx, deliveryID, data)
	return data
}

func (h *commentsHandler) reviewCommentData(
	ctx context.Context,
	deliveryID string,
	evt *PullRequestReviewCommentEvent,
) *commentEventData {
	if evt == nil {
		return nil
	}
	if !strings.EqualFold(evt.Action, "created") {
		return nil
	}
	if !h.shouldHandleComment(evt.Comment.Body) {
		return nil
	}

	owner, repo := repoOwnerAndName(evt.Repository)
	issueNumber := evt.PullRequest.Number
	prID := evt.PullRequest.ID
	if owner == "" || repo == "" || issueNumber <= 0 || prID <= 0 {
		slog.WarnContext(ctx, "repository details missing from pull_request_review_comment payload",
			"delivery_id", deliveryID,
			"owner", owner,
			"repo", repo,
			"issue_number", issueNumber,
			"pull_request_id", prID,
		)
		return nil
	}
	if evt.PullRequest.State != "" && !strings.EqualFold(evt.PullRequest.State, "open") {
		slog.InfoContext(ctx, "pull request is not open; skipping comment command",
			"delivery_id", deliveryID,
			"state", evt.PullRequest.State,
		)
		return nil
	}

	data := &commentEventData{
		CommentBody:   evt.Comment.Body,
		CommentAuthor: evt.Comment.Login,
		PRAuthor:      evt.PullRequest.Login,
		Owner:         owner,
		Repo:          repo,
		IssueNumber:   issueNumber,
		PullRequestID: prID,
	}
	h.populateInstallationID(ctx, deliveryID, data)
	return data
}

func (h *commentsHandler) pullRequestReviewData(
	ctx context.Context,
	deliveryID string,
	evt *PullRequestReviewEvent,
) *commentEventData {
	if evt == nil {
		return nil
	}
	if !strings.EqualFold(evt.Action, "submitted") {
		return nil
	}
	if evt.Review.Body == nil {
		return nil
	}
	if !h.shouldHandleComment(*evt.Review.Body) {
		return nil
	}

	owner, repo := repoOwnerAndName(evt.Repository)
	issueNumber := evt.PullRequest.Number
	prID := evt.PullRequest.ID
	if owner == "" || repo == "" || issueNumber <= 0 || prID <= 0 {
		slog.WarnContext(ctx, "repository details missing from pull_request_review payload",
			"delivery_id", deliveryID,
			"owner", owner,
			"repo", repo,
			"issue_number", issueNumber,
			"pull_request_id", prID,
		)
		return nil
	}
	if evt.PullRequest.State != "" && !strings.EqualFold(evt.PullRequest.State, "open") {
		slog.InfoContext(ctx, "pull request is not open; skipping comment command",
			"delivery_id", deliveryID,
			"state", evt.PullRequest.State,
		)
		return nil
	}

	data := &commentEventData{
		CommentBody:   *evt.Review.Body,
		CommentAuthor: evt.Review.Login,
		PRAuthor:      evt.PullRequest.Login,
		Owner:         owner,
		Repo:          repo,
		IssueNumber:   issueNumber,
		PullRequestID: prID,
	}
	h.populateInstallationID(ctx, deliveryID, data)
	return data
}

func (h *commentsHandler) populateInstallationID(ctx context.Context, deliveryID string, data *commentEventData) {
	if data == nil || data.Owner == "" || data.InstallationID > 0 || h.tokens == nil {
		return
	}
	data.InstallationID = h.lookupInstallationID(ctx, deliveryID, data.Owner)
}

func (h *commentsHandler) lookupTask(ctx context.Context, deliveryID string, prID int64) *p42.Task {
	resp, err := h.tasks.SearchTasks(ctx, &p42.SearchTasksRequest{PullRequestID: &prID})
	if err != nil {
		slog.ErrorContext(ctx, "search tasks failed",
			"delivery_id", deliveryID,
			"pull_request_id", prID,
			"error", err,
		)
		return nil
	}
	if resp == nil {
		return nil
	}
	if len(resp.Items) > 1 {
		slog.ErrorContext(ctx, "multiple tasks matched pull request",
			"delivery_id", deliveryID,
			"pull_request_id", prID,
			"task_count", len(resp.Items),
		)
		return nil
	}
	if len(resp.Items) == 1 {
		return &resp.Items[0]
	}
	slog.InfoContext(ctx, "no tasks matched pull request",
		"delivery_id", deliveryID,
		"pull_request_id", prID,
	)
	return nil
}

func (h *commentsHandler) lookupInstallationID(ctx context.Context, deliveryID, owner string) int64 {
	if !h.useGithubApp || h.tokens == nil || h.tasks == nil {
		return 0
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return 0
	}
	nameFilter := owner
	resp, err := h.tasks.ListGithubOrgs(ctx, &p42.ListGithubOrgsRequest{Name: &nameFilter})
	if err != nil {
		slog.ErrorContext(ctx, "failed to list github orgs",
			"delivery_id", deliveryID,
			"owner", owner,
			"error", err,
		)
		return 0
	}
	if resp == nil {
		return 0
	}
	for i := range resp.Orgs {
		org := resp.Orgs[i]
		if org.Deleted {
			continue
		}
		if strings.EqualFold(org.OrgName, owner) && org.InstallationID > 0 {
			return int64(org.InstallationID)
		}
	}
	return 0
}

func (h *commentsHandler) postConflictComment(
	ctx context.Context,
	gh github.API,
	deliveryID string,
	owner, repo string,
	issueNumber int,
	taskID string,
	installationID int64,
) {
	if !h.useGithubApp {
		return
	}

	ctx, ok := h.ctxWithInstallationToken(ctx, gh, deliveryID, installationID)
	if !ok {
		return
	}

	body := h.conflictCommentBody(taskID)
	if _, err := gh.CreateIssueComment(ctx, owner, repo, issueNumber, body); err != nil {
		slog.ErrorContext(ctx, "failed to post conflict comment",
			"delivery_id", deliveryID,
			"owner", owner,
			"repo", repo,
			"issue_number", issueNumber,
			"error", err,
		)
	}
}

func (h *commentsHandler) ctxWithInstallationToken(
	ctx context.Context,
	gh github.API,
	deliveryID string,
	installationID int64,
) (context.Context, bool) {
	if h.tokens == nil || installationID <= 0 {
		return ctx, true
	}
	token, err := h.tokens.InstallationToken(ctx, gh, installationID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to fetch installation token",
			"delivery_id", deliveryID,
			"installation_id", installationID,
			"error", err,
		)
		return ctx, false
	}
	return github.WithGithubToken(ctx, token), true
}

func (h *commentsHandler) conflictCommentBody(taskID string) string {
	command := h.command
	if command == "" {
		command = "/Plan42"
	}
	link := h.taskLink(taskID)
	marker := ""
	if taskID != "" {
		marker = fmt.Sprintf("<!-- event-horizon-conflict:%s -->\n", taskID)
	}
	if link == "" {
		return fmt.Sprintf(
			"%sI'm currently acting on previous feedback for this task. Once that's done and you have reviewed my latest changes, just type %s again and I will implement your new feedback.",
			marker,
			command,
		)
	}
	return fmt.Sprintf(
		"%sI'm currently acting on previous feedback for this task (%s). Once that's done and you have reviewed my latest changes, just type %s again and I will implement your new feedback.",
		marker,
		link,
		command,
	)
}

func (h *commentsHandler) taskLink(taskID string) string {
	if h.UIURL == "" || taskID == "" {
		return ""
	}
	return fmt.Sprintf(`<a href="%s/tasks/%s">%s</a>`, h.UIURL, taskID, taskID)
}

func (h *commentsHandler) conflictTargets(ctx context.Context, deliveryID string, task *p42.Task, fallback conflictTarget) []conflictTarget {
	var targets []conflictTarget
	for key, repoInfo := range task.RepoInfo {
		parts := strings.SplitN(key, "/", 2)
		if len(parts) != 2 || repoInfo == nil || repoInfo.PRNumber == nil {
			continue
		}
		installationID := h.lookupInstallationID(ctx, deliveryID, parts[0])
		targets = append(targets, conflictTarget{
			owner:          parts[0],
			repo:           parts[1],
			issueNumber:    *repoInfo.PRNumber,
			installationID: installationID,
		})
	}
	if len(targets) == 0 && fallback.owner != "" && fallback.repo != "" && fallback.issueNumber > 0 {
		if fallback.installationID == 0 {
			fallback.installationID = h.lookupInstallationID(ctx, deliveryID, fallback.owner)
		}
		targets = append(targets, fallback)
	}
	return targets
}

func (h *commentsHandler) fetchPullRequest(
	ctx context.Context,
	deliveryID string,
	gh github.API,
	owner, repo string,
	issueNumber int,
	issueState string,
) *github.PullRequest {
	if owner == "" || repo == "" || issueNumber <= 0 {
		slog.WarnContext(ctx, "repository details missing from issue_comment payload",
			"delivery_id", deliveryID,
			"owner", owner,
			"repo", repo,
			"issue_number", issueNumber,
		)
		return nil
	}

	pr, err := gh.GetPullRequest(ctx, owner, repo, issueNumber)
	if err != nil {
		slog.ErrorContext(ctx, "failed to fetch pull request",
			"delivery_id", deliveryID,
			"owner", owner,
			"repo", repo,
			"number", issueNumber,
			"error", err,
		)
		return nil
	}
	if pr == nil || pr.ID <= 0 {
		slog.WarnContext(ctx, "github pull request lookup did not return an id",
			"delivery_id", deliveryID,
			"owner", owner,
			"repo", repo,
			"number", issueNumber,
		)
		return nil
	}

	state := issueState
	if pr.State != "" {
		state = pr.State
	}
	if !strings.EqualFold(state, "open") {
		slog.InfoContext(ctx, "pull request is not open; skipping comment command",
			"delivery_id", deliveryID,
			"state", state,
		)
		return nil
	}

	return pr
}

func repoOwnerAndName(repo Repository) (string, string) {
	owner := repo.Org
	name := repo.Name
	if owner == "" && repo.FullName != "" {
		parts := strings.SplitN(repo.FullName, "/", 2)
		owner = parts[0]
		if len(parts) == 2 && name == "" {
			name = parts[1]
		}
	}
	return owner, name
}
