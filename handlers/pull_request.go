package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/plan42-ai/concurrency"
	"github.com/plan42-ai/github-event-handlers/github"
	"github.com/plan42-ai/sdk-go/p42"
)

const (
	prStatusMerged  = "merged"
	prStatusClosed  = "closed"
	prStatusDraft   = "draft"
	prStatusOpen    = "open"
	prStatusUnknown = "unknown"
)

func newPullRequestHandler(client Plan42Client) func(ctx context.Context, evt Event, gh github.API) {
	h := &pullRequestHandler{tasks: client}
	return h.handle
}

type pullRequestHandler struct {
	tasks Plan42Client
}

func (h *pullRequestHandler) handle(ctx context.Context, evt Event, _ github.API) {
	if h == nil || h.tasks == nil {
		return
	}

	prEvt, ok := evt.(*PullRequestEvent)
	if !ok || prEvt == nil {
		return
	}

	deliveryID := prEvt.GetDeliveryID()
	prID := prEvt.PullRequest.ID
	repoKey := pullRequestRepoKey(prEvt.Repository)

	status := prStatus(prEvt.PullRequest)
	statusUpdatedAt := prStatusUpdatedAt(prEvt.PullRequest.UpdatedAt)

	resp, err := h.tasks.SearchTasks(ctx, &p42.SearchTasksRequest{PullRequestID: &prID})
	if err != nil {
		slog.ErrorContext(ctx, "search tasks failed for pull request",
			"delivery_id", deliveryID,
			"pull_request_id", prID,
			"error", err,
		)
		return
	}
	if resp == nil || len(resp.Items) == 0 {
		slog.InfoContext(ctx, "no tasks found for pull request",
			"delivery_id", deliveryID,
			"pull_request_id", prID,
		)
		return
	}

	if len(resp.Items) > 1 {
		slog.WarnContext(ctx, "multiple tasks matched pull request status update; using first result",
			"delivery_id", deliveryID,
			"pull_request_id", prID,
		)
	}

	h.updateTaskRepoStatus(
		ctx,
		deliveryID,
		resp.Items[0],
		repoKey,
		status,
		statusUpdatedAt,
		prID,
		prEvt.PullRequest.Number,
		prEvt.PullRequest.HTMLURL,
	)
}

func prStatus(pr PullRequest) string {
	if pr.Merged {
		return prStatusMerged
	}
	if strings.EqualFold(pr.State, prStatusClosed) {
		return prStatusClosed
	}
	if pr.Draft {
		return prStatusDraft
	}
	if strings.EqualFold(pr.State, prStatusOpen) {
		return prStatusOpen
	}
	return prStatusUnknown
}

func prStatusUpdatedAt(updatedAt *time.Time) time.Time {
	if updatedAt != nil && !updatedAt.IsZero() {
		return *updatedAt
	}
	return time.Now()
}

func pullRequestRepoKey(repo Repository) string {
	if strings.Contains(repo.FullName, "/") {
		return repo.FullName
	}
	if repo.Org != "" && repo.Name != "" {
		return fmt.Sprintf("%s/%s", repo.Org, repo.Name)
	}
	return ""
}

func (h *pullRequestHandler) updateTaskRepoStatus(
	ctx context.Context,
	deliveryID string,
	task p42.Task,
	repoKey, status string,
	statusUpdatedAt time.Time,
	prID int64,
	prNumber int,
	prLink string,
) {
	if task.RepoInfo == nil {
		return
	}
	repoInfo, ok := task.RepoInfo[repoKey]
	if !ok || repoInfo == nil {
		slog.WarnContext(ctx, "task missing repo info for pull request",
			"delivery_id", deliveryID,
			"task_id", task.TaskID,
			"repo", repoKey,
		)
		return
	}

	resolvedStatus := normalizeStatus(status)
	resolvedStatusAt := statusUpdatedAt
	if repoInfo.PRStatusUpdatedAt != nil && !repoInfo.PRStatusUpdatedAt.IsZero() && repoInfo.PRStatusUpdatedAt.After(resolvedStatusAt) {
		resolvedStatusAt = *repoInfo.PRStatusUpdatedAt
		resolvedStatus = normalizeStatus(deref(repoInfo.PRStatus))
	}

	info := repoInfo

	needsUpdate := false
	currentStatus := normalizeStatus(deref(info.PRStatus))
	currentStatusAt := time.Time{}
	if info.PRStatusUpdatedAt != nil {
		currentStatusAt = *info.PRStatusUpdatedAt
	}

	if resolvedStatus != currentStatus || currentStatusAt.Before(resolvedStatusAt) || currentStatusAt.IsZero() {
		statusCopy := resolvedStatus
		info.PRStatus = &statusCopy
		statusTime := resolvedStatusAt
		info.PRStatusUpdatedAt = &statusTime
		needsUpdate = true
	}

	id := fmt.Sprintf("%d", prID)
	if info.PRID == nil || *info.PRID != id {
		info.PRID = &id
		needsUpdate = true
	}
	if prNumber > 0 {
		if info.PRNumber == nil || *info.PRNumber != prNumber {
			number := prNumber
			info.PRNumber = &number
			needsUpdate = true
		}
	}
	if prLink != "" {
		if info.PRLink == nil || *info.PRLink != prLink {
			link := prLink
			info.PRLink = &link
			needsUpdate = true
		}
	}

	if !needsUpdate {
		if task.WorkstreamID == nil || nextWorkstreamTaskState(task) == task.State {
			return
		}
	}

	err := h.updateTask(ctx, task, repoKey, resolvedStatus, resolvedStatusAt, prID, prNumber, prLink)
	if err != nil {
		slog.ErrorContext(ctx, "failed to update task with pull request status",
			"delivery_id", deliveryID,
			"task_id", task.TaskID,
			"pull_request_id", prID,
			"error", err,
		)
	}
}

func (h *pullRequestHandler) updateTask(
	ctx context.Context, task p42.Task, repoKey string,
	status string, statusAt time.Time, prID int64, prNumber int, prLink string,
) error {
	const maxAttempts = 5
	backoff := concurrency.NewBackoff(10*time.Millisecond, time.Second)
	defer backoff.StopTimer()

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		var err error
		if task.WorkstreamID != nil {
			state := nextWorkstreamTaskState(task)
			_, err = h.tasks.UpdateWorkstreamTask(ctx, &p42.UpdateWorkstreamTaskRequest{
				TenantID:     task.TenantID,
				TaskID:       task.TaskID,
				Version:      task.Version,
				WorkstreamID: *task.WorkstreamID,
				RepoInfo:     &task.RepoInfo,
				State:        &state,
			})
		} else {
			_, err = h.tasks.UpdateTask(ctx, &p42.UpdateTaskRequest{
				TenantID: task.TenantID,
				TaskID:   task.TaskID,
				Version:  task.Version,
				RepoInfo: &task.RepoInfo,
			})
		}
		if err == nil {
			return nil
		}
		var confErr *p42.ConflictError
		if !errors.As(err, &confErr) {
			return err
		}
		// Extract the current task from the conflict response.
		var curTask *p42.Task
		switch c := confErr.Current.(type) {
		case *p42.WorkstreamTaskConflict:
			curTask = c.Task
		case *p42.Task:
			curTask = c
		}
		if curTask == nil {
			return err
		}
		// Re-merge our data against the server's current state
		// to avoid overwriting newer concurrent updates.
		task = *curTask
		if !applyRepoUpdate(&task, repoKey, status, statusAt, prID, prNumber, prLink) {
			return nil
		}
		if attempt < maxAttempts {
			backoff.Backoff()
			if werr := backoff.WaitContext(ctx); werr != nil {
				return werr
			}
		}
	}
	return fmt.Errorf("failed to update task after %d attempts", maxAttempts)
}

// applyRepoUpdate merges PR data into the task's repo entry, respecting
// timestamp ordering so a newer concurrent update wins. Returns false if
// the repo entry no longer exists.
func applyRepoUpdate(task *p42.Task, repoKey, status string, statusAt time.Time, prID int64, prNumber int, prLink string) bool {
	if task.RepoInfo == nil {
		return false
	}
	info := task.RepoInfo[repoKey]
	if info == nil {
		return false
	}
	id := fmt.Sprintf("%d", prID)
	if info.PRID != nil && *info.PRID != "" && *info.PRID != id {
		return false
	}
	if info.PRStatusUpdatedAt == nil || info.PRStatusUpdatedAt.IsZero() || !info.PRStatusUpdatedAt.After(statusAt) {
		s := status
		info.PRStatus = &s
		t := statusAt
		info.PRStatusUpdatedAt = &t
	}
	info.PRID = &id
	if prNumber > 0 {
		info.PRNumber = &prNumber
	}
	if prLink != "" {
		info.PRLink = &prLink
	}
	return true
}

func nextWorkstreamTaskState(task p42.Task) p42.TaskState {
	allTerminal, anyNonTerminal := summarizePRStates(task.RepoInfo)
	if allTerminal && isReviewState(task.State) {
		return p42.TaskStateCompleted
	}
	if anyNonTerminal && task.State == p42.TaskStateCompleted {
		return p42.TaskStateAwaitingCodeReview
	}
	return task.State
}

func isReviewState(state p42.TaskState) bool {
	switch state {
	case p42.TaskStateAwaitingCodeReview, p42.TaskStateCompleted:
		return true
	default:
		return false
	}
}

func summarizePRStates(repoInfo map[string]*p42.RepoInfo) (allTerminal bool, anyNonTerminal bool) {
	hasPR := false
	allTerminal = true
	for _, info := range repoInfo {
		if info == nil || info.PRID == nil || strings.TrimSpace(*info.PRID) == "" {
			continue
		}
		hasPR = true
		if !isTerminalPRStatus(deref(info.PRStatus)) {
			allTerminal = false
			anyNonTerminal = true
		}
	}
	if !hasPR {
		return false, false
	}
	return allTerminal, anyNonTerminal
}

func isTerminalPRStatus(status string) bool {
	switch normalizeStatus(status) {
	case prStatusClosed, prStatusMerged:
		return true
	default:
		return false
	}
}

func normalizeStatus(status string) string {
	s := strings.ToLower(strings.TrimSpace(status))
	if s == "" {
		return prStatusUnknown
	}
	return s
}

// deref safely dereferences a pointer, returning the zero value if nil.
func deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}
