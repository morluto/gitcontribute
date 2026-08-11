package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

const (
	defaultPullRequestCheckWaitTimeout = 30 * time.Minute
	defaultPullRequestCheckPoll        = 10 * time.Second
	maxPullRequestCheckWaitTimeout     = 24 * time.Hour
	maxPullRequestCheckPolls           = 10000
	maxPullRequestCheckTransitions     = 64
)

type gitCommitSHA [20]byte

func parseGitCommitSHA(value string) (gitCommitSHA, error) {
	decoded, err := hex.DecodeString(strings.TrimSpace(value))
	if err != nil || len(decoded) != len(gitCommitSHA{}) {
		return gitCommitSHA{}, errors.New("expected_head_sha must be a full 40-character hexadecimal commit SHA")
	}
	var sha gitCommitSHA
	copy(sha[:], decoded)
	return sha, nil
}

func (s gitCommitSHA) String() string {
	return hex.EncodeToString(s[:])
}

func (s gitCommitSHA) matches(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), s.String())
}

type pullRequestCheckCompletion uint8

const (
	pullRequestCheckCompletionAll pullRequestCheckCompletion = iota + 1
	pullRequestCheckCompletionFailFast
)

func (c pullRequestCheckCompletion) failFast() bool {
	return c == pullRequestCheckCompletionFailFast
}

type pullRequestCheckWaitRequest struct {
	repository   domain.RepoRef
	number       int
	expectedHead gitCommitSHA
	timeout      time.Duration
	pollInterval time.Duration
	maxPages     int
	completion   pullRequestCheckCompletion
}

func (r pullRequestCheckWaitRequest) canonical() mcpcontract.WaitPullRequestChecksInput {
	return mcpcontract.WaitPullRequestChecksInput{
		Owner:           r.repository.Owner(),
		Repo:            r.repository.Repo(),
		Number:          r.number,
		ExpectedHeadSHA: r.expectedHead.String(),
		Timeout:         r.timeout.String(),
		PollInterval:    r.pollInterval.String(),
		MaxPages:        r.maxPages,
		FailFast:        r.completion.failFast(),
	}
}

// WaitPullRequestChecks owns the unchanged-state interval inside a durable
// job. It watches one exact head and only replaces the stored health snapshot
// after a complete terminal observation, so timeout/cancellation cannot erase
// the last usable corpus projection.
func (r *MCPReader) WaitPullRequestChecks(ctx context.Context, in mcpcontract.WaitPullRequestChecksInput) (mcpcontract.JobReference, error) {
	request, err := parsePullRequestCheckWaitInput(in)
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	canonical := request.canonical()
	id, err := r.submitJob(ctx, "wait_pull_request_checks", canonical, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.waitPullRequestChecks(ctx, request, report)
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, "wait_pull_request_checks", "pull-request check watch started"), nil
}

func parsePullRequestCheckWaitInput(in mcpcontract.WaitPullRequestChecksInput) (pullRequestCheckWaitRequest, error) {
	repository, err := domain.NewRepoRef(in.Owner, in.Repo)
	if err != nil {
		return pullRequestCheckWaitRequest{}, err
	}
	if in.Number < 1 {
		return pullRequestCheckWaitRequest{}, errors.New("number must be positive")
	}
	expectedHead, err := parseGitCommitSHA(in.ExpectedHeadSHA)
	if err != nil {
		return pullRequestCheckWaitRequest{}, err
	}
	if in.Timeout == "" {
		in.Timeout = defaultPullRequestCheckWaitTimeout.String()
	}
	timeout, err := time.ParseDuration(in.Timeout)
	if err != nil || timeout <= 0 || timeout > maxPullRequestCheckWaitTimeout {
		return pullRequestCheckWaitRequest{}, fmt.Errorf("timeout must be between 1s and %s", maxPullRequestCheckWaitTimeout)
	}
	if in.PollInterval == "" {
		in.PollInterval = defaultPullRequestCheckPoll.String()
	}
	interval, err := time.ParseDuration(in.PollInterval)
	if err != nil || interval < time.Second || interval > 5*time.Minute {
		return pullRequestCheckWaitRequest{}, errors.New("poll_interval must be between 1s and 5m")
	}
	if timeout/interval > maxPullRequestCheckPolls {
		return pullRequestCheckWaitRequest{}, fmt.Errorf("timeout and poll_interval allow at most %d status polls", maxPullRequestCheckPolls)
	}
	if in.MaxPages == 0 {
		in.MaxPages = 10
	}
	if in.MaxPages < 1 || in.MaxPages > 100 {
		return pullRequestCheckWaitRequest{}, errors.New("max_pages must be between 1 and 100")
	}
	completion := pullRequestCheckCompletionAll
	if in.FailFast {
		completion = pullRequestCheckCompletionFailFast
	}
	return pullRequestCheckWaitRequest{
		repository: repository, number: in.Number, expectedHead: expectedHead,
		timeout: timeout, pollInterval: interval, maxPages: in.MaxPages, completion: completion,
	}, nil
}

func (r *MCPReader) waitPullRequestChecks(ctx context.Context, request pullRequestCheckWaitRequest, report func(string, string) error) (mcpcontract.WaitPullRequestChecksOutput, error) { //nolint:gocognit // The bounded watcher owns polling, coalescing, exact-head checks, and terminal persistence.
	watchCtx, cancel := context.WithTimeout(ctx, request.timeout)
	defer cancel()
	reader, err := r.githubReader() //nolint:contextcheck // Client construction performs no request; operations below receive ctx.
	if err != nil {
		return mcpcontract.WaitPullRequestChecksOutput{}, err
	}
	statusReader, ok := reader.(github.PullRequestStatusReader)
	if !ok {
		return mcpcontract.WaitPullRequestChecksOutput{}, errors.New("pull-request check status support is unavailable")
	}

	result := mcpcontract.WaitPullRequestChecksOutput{
		Status: mcpcontract.PullRequestCheckWaiting, Owner: request.repository.Owner(), Repo: request.repository.Repo(), Number: request.number,
		ExpectedHeadSHA: request.expectedHead.String(), Transitions: make([]mcpcontract.PullRequestCheckTransition, 0, 8),
	}
	var lastSignature string
	for poll := 1; poll <= maxPullRequestCheckPolls; poll++ {
		if err := watchCtx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				result.Status, result.Reason = mcpcontract.PullRequestCheckTimedOut, "watch timeout elapsed before a complete terminal check set was observed"
				return result, nil
			}
			return result, err
		}
		remote, err := statusReader.GetPullRequestStatus(watchCtx, request.repository.Owner(), request.repository.Repo(), request.number, github.PullRequestStatusOptions{PageSize: 100, MaxPages: request.maxPages})
		if err != nil {
			if errors.Is(watchCtx.Err(), context.DeadlineExceeded) {
				result.Status, result.Reason = mcpcontract.PullRequestCheckTimedOut, "watch timeout elapsed before a complete terminal check set was observed"
				return result, nil
			}
			return mcpcontract.WaitPullRequestChecksOutput{}, err
		}
		result.Polls = poll
		result.ObservedHeadSHA = remote.HeadSHA
		result.ObservedAt = remote.SourceUpdatedAt.UTC().Format(time.RFC3339Nano)
		if !request.expectedHead.matches(remote.HeadSHA) {
			result.Status, result.Reason = mcpcontract.PullRequestCheckSuperseded, "pull-request head changed while waiting; start a new watch for the new revision"
			return result, nil
		}
		if request.completion.failFast() && pullRequestChecksHasFailed(remote.Checks.Items) && (!remote.Checks.Coverage.Complete || !pullRequestChecksAllTerminal(remote.Checks.Items)) {
			result.Checks = pullRequestChecksToOutput(remote.Checks.Items)
			result.Status, result.Reason = mcpcontract.PullRequestCheckFailed, "one or more observed checks concluded unsuccessfully; fail-fast returned without replacing incomplete coverage"
			return result, nil
		}
		if !remote.Checks.Coverage.Complete {
			// A truncated rollup cannot prove terminality. Keep the prior complete
			// snapshot intact and let the bounded wait return timed_out if needed.
			result.Status = mcpcontract.PullRequestCheckIncomplete
		} else {
			signature := pullRequestCheckSignature(remote.Checks.Items)
			if signature != lastSignature && len(result.Transitions) < maxPullRequestCheckTransitions {
				result.Transitions = append(result.Transitions, mcpcontract.PullRequestCheckTransition{
					ObservedAt: result.ObservedAt, Signature: signature, CheckCount: len(remote.Checks.Items),
				})
				lastSignature = signature
			} else if signature != lastSignature {
				result.TransitionsTruncated = true
				lastSignature = signature
			}
			terminal, failed := pullRequestChecksTerminal(remote.Checks.Items, request.completion)
			if terminal {
				result.Checks = pullRequestChecksToOutput(remote.Checks.Items)
				if failed && request.completion.failFast() && !pullRequestChecksAllTerminal(remote.Checks.Items) {
					result.Status, result.Reason = mcpcontract.PullRequestCheckFailed, "one or more checks concluded unsuccessfully; fail-fast returned before other checks completed"
					return result, nil
				}
				latest, latestErr := statusReader.GetPullRequestStatus(watchCtx, request.repository.Owner(), request.repository.Repo(), request.number, github.PullRequestStatusOptions{PageSize: 100, MaxPages: request.maxPages})
				if latestErr != nil {
					if errors.Is(watchCtx.Err(), context.DeadlineExceeded) {
						result.Status, result.Reason = mcpcontract.PullRequestCheckTimedOut, "watch timeout elapsed before the terminal health projection could be replaced"
						return result, nil
					}
					return result, latestErr
				}
				if !request.expectedHead.matches(latest.HeadSHA) {
					result.Status, result.Reason = mcpcontract.PullRequestCheckSuperseded, "pull-request head changed before the terminal health projection could be replaced"
					return result, nil
				}
				if !latest.Checks.Coverage.Complete {
					result.Status, result.Reason = mcpcontract.PullRequestCheckIncomplete, "terminal observation became incomplete before local replacement"
					return result, nil
				}
				latestTerminal, _ := pullRequestChecksTerminal(latest.Checks.Items, pullRequestCheckCompletionAll)
				if !latestTerminal || !pullRequestChecksAllTerminal(latest.Checks.Items) {
					result.Status, result.Reason = mcpcontract.PullRequestCheckIncomplete, "check state changed before local replacement and is no longer a complete terminal set"
					return result, nil
				}
				remote = latest
				result.Checks = pullRequestChecksToOutput(remote.Checks.Items)
				baselines, baselineErr := r.pullRequestHealthBaselines(watchCtx, mcpcontract.ThreadRef{Owner: request.repository.Owner(), Repo: request.repository.Repo(), Kind: string(domain.PullRequestKind), Number: request.number})
				if baselineErr == nil {
					final, finalErr := statusReader.GetPullRequestStatus(watchCtx, request.repository.Owner(), request.repository.Repo(), request.number, github.PullRequestStatusOptions{PageSize: 100, MaxPages: request.maxPages})
					if finalErr != nil {
						if errors.Is(watchCtx.Err(), context.DeadlineExceeded) {
							result.Status, result.Reason = mcpcontract.PullRequestCheckTimedOut, "watch timeout elapsed before the terminal health projection could be replaced"
							return result, nil
						}
						return result, finalErr
					}
					if !request.expectedHead.matches(final.HeadSHA) {
						result.Status, result.Reason = mcpcontract.PullRequestCheckSuperseded, "pull-request head changed immediately before the local health projection could be replaced"
						return result, nil
					}
					if !final.Checks.Coverage.Complete || !pullRequestChecksAllTerminal(final.Checks.Items) {
						result.Status, result.Reason = mcpcontract.PullRequestCheckIncomplete, "check state changed immediately before the local health projection could be replaced"
						return result, nil
					}
					remote = final
					failed = pullRequestChecksHasFailed(final.Checks.Items)
					result.Checks = pullRequestChecksToOutput(final.Checks.Items)
					_, persistErr := r.persistPullRequestHealth(watchCtx, mcpcontract.ThreadRef{Owner: request.repository.Owner(), Repo: request.repository.Repo(), Kind: string(domain.PullRequestKind), Number: request.number}, remote, nil, baselines)
					result.Persisted = persistErr == nil
					if persistErr != nil {
						result.Status, result.Reason = mcpcontract.PullRequestCheckIncomplete, "terminal checks observed but the local health projection could not be replaced"
						return result, nil
					}
				} else {
					result.Status, result.Reason = mcpcontract.PullRequestCheckIncomplete, "terminal checks observed but the local health baseline is unavailable"
					return result, nil
				}
				if failed {
					result.Status, result.Reason = mcpcontract.PullRequestCheckFailed, "one or more checks concluded unsuccessfully"
				} else {
					result.Status = mcpcontract.PullRequestCheckSucceeded
				}
				return result, nil
			}
		}
		if err := report("waiting_for_checks", fmt.Sprintf(`{"polls":%d,"transitions":%d}`, result.Polls, len(result.Transitions))); err != nil {
			return result, err
		}
		timer := time.NewTimer(request.pollInterval)
		select {
		case <-watchCtx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-timer.C:
		}
	}
	result.Status, result.Reason = mcpcontract.PullRequestCheckTimedOut, "maximum poll count reached before a complete terminal check set was observed"
	return result, nil
}

func pullRequestChecksAllTerminal(items []github.PullRequestCheck) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if !pullRequestCheckTerminal(item) {
			return false
		}
	}
	return true
}

func pullRequestChecksHasFailed(items []github.PullRequestCheck) bool {
	for _, item := range items {
		if pullRequestCheckTerminal(item) && pullRequestCheckFailed(item) {
			return true
		}
	}
	return false
}

func pullRequestCheckSignature(items []github.PullRequestCheck) string {
	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "%s\x00%s\x00%s\x00%s\n", item.Kind, item.Name, item.Status, item.Conclusion)
	}
	digest := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(digest[:])
}

func pullRequestChecksTerminal(items []github.PullRequestCheck, completion pullRequestCheckCompletion) (terminal, failed bool) {
	if len(items) == 0 {
		return false, false
	}
	pending := false
	for _, item := range items {
		if !pullRequestCheckTerminal(item) {
			pending = true
			continue
		}
		if pullRequestCheckFailed(item) {
			failed = true
		}
	}
	if completion.failFast() && failed {
		return true, true
	}
	if pending {
		return false, false
	}
	return true, failed
}

func pullRequestCheckTerminal(item github.PullRequestCheck) bool {
	if item.Conclusion != "" {
		switch strings.ToUpper(item.Conclusion) {
		case "SUCCESS", "NEUTRAL", "SKIPPED", "FAILURE", "ERROR", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED", "STALE", "STARTUP_FAILURE":
			return true
		default:
			return false
		}
	}
	switch strings.ToUpper(item.Status) {
	case "SUCCESS", "FAILURE", "ERROR", "NEUTRAL", "CANCELLED", "SKIPPED", "STALE", "ACTION_REQUIRED", "TIMED_OUT":
		return true
	default:
		return false
	}
}

func pullRequestCheckFailed(item github.PullRequestCheck) bool {
	switch strings.ToUpper(item.Conclusion) {
	case "FAILURE", "ERROR", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED", "STALE", "STARTUP_FAILURE":
		return true
	}
	switch strings.ToUpper(item.Status) {
	case "FAILURE", "ERROR", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED", "STALE":
		return true
	default:
		return false
	}
}

func pullRequestChecksToOutput(items []github.PullRequestCheck) []mcpcontract.PullRequestCheckOutput {
	out := make([]mcpcontract.PullRequestCheckOutput, 0, len(items))
	for _, item := range items {
		out = append(out, mcpcontract.PullRequestCheckOutput{Kind: item.Kind, Name: item.Name, Status: item.Status, Conclusion: item.Conclusion, DetailsURL: item.DetailsURL, StartedAt: item.StartedAt, CompletedAt: item.CompletedAt})
	}
	return out
}
