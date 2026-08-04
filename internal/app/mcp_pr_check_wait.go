package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
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

// WaitPullRequestChecks owns the unchanged-state interval inside a durable
// job. It watches one exact head and only replaces the stored health snapshot
// after a complete terminal observation, so timeout/cancellation cannot erase
// the last usable corpus projection.
func (r *MCPReader) WaitPullRequestChecks(ctx context.Context, in mcpcontract.WaitPullRequestChecksInput) (mcpcontract.JobReference, error) {
	if err := validatePullRequestCheckWaitInput(&in); err != nil {
		return mcpcontract.JobReference{}, err
	}
	id, err := r.submitJob(ctx, "wait_pull_request_checks", in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.waitPullRequestChecks(ctx, in, report)
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, "wait_pull_request_checks", "pull-request check watch started"), nil
}

func validatePullRequestCheckWaitInput(in *mcpcontract.WaitPullRequestChecksInput) error {
	if err := (domain.RepoRef{Owner: in.Owner, Repo: in.Repo}).Validate(); err != nil {
		return err
	}
	if in.Number < 1 {
		return errors.New("number must be positive")
	}
	if len(strings.TrimSpace(in.ExpectedHeadSHA)) != 40 {
		return errors.New("expected_head_sha must be a full 40-character hexadecimal commit SHA")
	}
	if _, err := hex.DecodeString(strings.TrimSpace(in.ExpectedHeadSHA)); err != nil {
		return errors.New("expected_head_sha must be a full 40-character hexadecimal commit SHA")
	}
	in.ExpectedHeadSHA = strings.ToLower(strings.TrimSpace(in.ExpectedHeadSHA))
	if in.Timeout == "" {
		in.Timeout = defaultPullRequestCheckWaitTimeout.String()
	}
	timeout, err := time.ParseDuration(in.Timeout)
	if err != nil || timeout <= 0 || timeout > maxPullRequestCheckWaitTimeout {
		return fmt.Errorf("timeout must be between 1s and %s", maxPullRequestCheckWaitTimeout)
	}
	if in.PollInterval == "" {
		in.PollInterval = defaultPullRequestCheckPoll.String()
	}
	interval, err := time.ParseDuration(in.PollInterval)
	if err != nil || interval < time.Second || interval > 5*time.Minute {
		return errors.New("poll_interval must be between 1s and 5m")
	}
	if timeout/interval > maxPullRequestCheckPolls {
		return fmt.Errorf("timeout and poll_interval allow at most %d status polls", maxPullRequestCheckPolls)
	}
	if in.MaxPages == 0 {
		in.MaxPages = 10
	}
	if in.MaxPages < 1 || in.MaxPages > 100 {
		return errors.New("max_pages must be between 1 and 100")
	}
	return nil
}

func (r *MCPReader) waitPullRequestChecks(ctx context.Context, in mcpcontract.WaitPullRequestChecksInput, report func(string, string) error) (mcpcontract.WaitPullRequestChecksOutput, error) { //nolint:gocognit // The bounded watcher owns polling, coalescing, exact-head checks, and terminal persistence.
	timeout, _ := time.ParseDuration(in.Timeout)
	interval, _ := time.ParseDuration(in.PollInterval)
	watchCtx, cancel := context.WithTimeout(ctx, timeout)
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
		Status: "waiting", Owner: in.Owner, Repo: in.Repo, Number: in.Number,
		ExpectedHeadSHA: in.ExpectedHeadSHA, Transitions: make([]mcpcontract.PullRequestCheckTransition, 0, 8),
	}
	var lastSignature string
	for poll := 1; poll <= maxPullRequestCheckPolls; poll++ {
		if err := watchCtx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				result.Status, result.Reason = "timed_out", "watch timeout elapsed before a complete terminal check set was observed"
				return result, nil
			}
			return result, err
		}
		remote, err := statusReader.GetPullRequestStatus(watchCtx, in.Owner, in.Repo, in.Number, github.PullRequestStatusOptions{PageSize: 100, MaxPages: in.MaxPages})
		if err != nil {
			if errors.Is(watchCtx.Err(), context.DeadlineExceeded) {
				result.Status, result.Reason = "timed_out", "watch timeout elapsed before a complete terminal check set was observed"
				return result, nil
			}
			return mcpcontract.WaitPullRequestChecksOutput{}, err
		}
		result.Polls = poll
		result.ObservedHeadSHA = remote.HeadSHA
		result.ObservedAt = remote.SourceUpdatedAt.UTC().Format(time.RFC3339Nano)
		if !strings.EqualFold(remote.HeadSHA, in.ExpectedHeadSHA) {
			result.Status, result.Reason = "superseded", "pull-request head changed while waiting; start a new watch for the new revision"
			return result, nil
		}
		if in.FailFast && pullRequestChecksHasFailed(remote.Checks.Items) && (!remote.Checks.Coverage.Complete || !pullRequestChecksAllTerminal(remote.Checks.Items)) {
			result.Checks = pullRequestChecksToOutput(remote.Checks.Items)
			result.Status, result.Reason = "failed", "one or more observed checks concluded unsuccessfully; fail-fast returned without replacing incomplete coverage"
			return result, nil
		}
		if !remote.Checks.Coverage.Complete {
			// A truncated rollup cannot prove terminality. Keep the prior complete
			// snapshot intact and let the bounded wait return timed_out if needed.
			result.Status = "incomplete"
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
			terminal, failed := pullRequestChecksTerminal(remote.Checks.Items, in.FailFast)
			if terminal {
				result.Checks = pullRequestChecksToOutput(remote.Checks.Items)
				if failed && in.FailFast && !pullRequestChecksAllTerminal(remote.Checks.Items) {
					result.Status, result.Reason = "failed", "one or more checks concluded unsuccessfully; fail-fast returned before other checks completed"
					return result, nil
				}
				latest, latestErr := statusReader.GetPullRequestStatus(watchCtx, in.Owner, in.Repo, in.Number, github.PullRequestStatusOptions{PageSize: 100, MaxPages: in.MaxPages})
				if latestErr != nil {
					if errors.Is(watchCtx.Err(), context.DeadlineExceeded) {
						result.Status, result.Reason = "timed_out", "watch timeout elapsed before the terminal health projection could be replaced"
						return result, nil
					}
					return result, latestErr
				}
				if !strings.EqualFold(latest.HeadSHA, in.ExpectedHeadSHA) {
					result.Status, result.Reason = "superseded", "pull-request head changed before the terminal health projection could be replaced"
					return result, nil
				}
				if !latest.Checks.Coverage.Complete {
					result.Status, result.Reason = "incomplete", "terminal observation became incomplete before local replacement"
					return result, nil
				}
				latestTerminal, _ := pullRequestChecksTerminal(latest.Checks.Items, false)
				if !latestTerminal || !pullRequestChecksAllTerminal(latest.Checks.Items) {
					result.Status, result.Reason = "incomplete", "check state changed before local replacement and is no longer a complete terminal set"
					return result, nil
				}
				remote = latest
				result.Checks = pullRequestChecksToOutput(remote.Checks.Items)
				baselines, baselineErr := r.pullRequestHealthBaselines(watchCtx, mcpcontract.ThreadRef{Owner: in.Owner, Repo: in.Repo, Kind: corpus.ThreadKindPullRequest, Number: in.Number})
				if baselineErr == nil {
					final, finalErr := statusReader.GetPullRequestStatus(watchCtx, in.Owner, in.Repo, in.Number, github.PullRequestStatusOptions{PageSize: 100, MaxPages: in.MaxPages})
					if finalErr != nil {
						if errors.Is(watchCtx.Err(), context.DeadlineExceeded) {
							result.Status, result.Reason = "timed_out", "watch timeout elapsed before the terminal health projection could be replaced"
							return result, nil
						}
						return result, finalErr
					}
					if !strings.EqualFold(final.HeadSHA, in.ExpectedHeadSHA) {
						result.Status, result.Reason = "superseded", "pull-request head changed immediately before the local health projection could be replaced"
						return result, nil
					}
					if !final.Checks.Coverage.Complete || !pullRequestChecksAllTerminal(final.Checks.Items) {
						result.Status, result.Reason = "incomplete", "check state changed immediately before the local health projection could be replaced"
						return result, nil
					}
					remote = final
					failed = pullRequestChecksHasFailed(final.Checks.Items)
					result.Checks = pullRequestChecksToOutput(final.Checks.Items)
					_, persistErr := r.persistPullRequestHealth(watchCtx, mcpcontract.ThreadRef{Owner: in.Owner, Repo: in.Repo, Kind: corpus.ThreadKindPullRequest, Number: in.Number}, remote, nil, baselines)
					result.Persisted = persistErr == nil
					if persistErr != nil {
						result.Status, result.Reason = "incomplete", "terminal checks observed but the local health projection could not be replaced"
						return result, nil
					}
				} else {
					result.Status, result.Reason = "incomplete", "terminal checks observed but the local health baseline is unavailable"
					return result, nil
				}
				if failed {
					result.Status, result.Reason = "failed", "one or more checks concluded unsuccessfully"
				} else {
					result.Status = "succeeded"
				}
				return result, nil
			}
		}
		if err := report("waiting_for_checks", fmt.Sprintf(`{"polls":%d,"transitions":%d}`, result.Polls, len(result.Transitions))); err != nil {
			return result, err
		}
		timer := time.NewTimer(interval)
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
	result.Status, result.Reason = "timed_out", "maximum poll count reached before a complete terminal check set was observed"
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

func pullRequestChecksTerminal(items []github.PullRequestCheck, failFast bool) (terminal, failed bool) {
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
	if failFast && failed {
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
