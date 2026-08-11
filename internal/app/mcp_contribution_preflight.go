package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/gitremote"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
	"github.com/morluto/gitcontribute/internal/workspace"
)

const (
	defaultContributionPreflightLimit       = 20
	defaultContributionPreflightMaxRequests = 100
)

type preflightCandidate struct {
	title            string
	query            string
	body             string
	issueNumber      int
	headRef          string
	headSHA          string
	changedFileCount int
}

func parsePreflightCandidate(input mcpcontract.ContributionPreflightCandidate) (preflightCandidate, error) {
	if input.IssueNumber < 0 {
		return preflightCandidate{}, errors.New("candidate issue_number must be positive when provided")
	}
	return preflightCandidate{
		title: strings.TrimSpace(input.Title), query: strings.TrimSpace(input.Query), body: strings.TrimSpace(input.Body),
		issueNumber: input.IssueNumber, headRef: strings.TrimSpace(input.HeadRef), headSHA: strings.TrimSpace(input.HeadSHA),
		changedFileCount: len(input.ChangedFiles),
	}, nil
}

func (c preflightCandidate) hasInputContext(workspacePathCount int) bool {
	return c.title != "" || c.query != "" || c.body != "" || c.issueNumber > 0 ||
		c.headRef != "" || c.headSHA != "" || c.changedFileCount > 0 || workspacePathCount > 0
}

func (c preflightCandidate) hasComparableIdentity(worktrees []workspace.LocalWorktree) bool {
	return c.title != "" || c.query != "" || c.body != "" || c.issueNumber > 0 ||
		c.headRef != "" || c.headSHA != "" || len(worktrees) > 0
}

func (c preflightCandidate) searchQuery() string {
	switch {
	case c.query != "":
		return c.query
	case c.title != "":
		return c.title
	case c.headRef != "":
		return c.headRef
	case len(c.body) > 200:
		return c.body[:200]
	default:
		return c.body
	}
}

// PreflightContribution performs the bounded, side-effect-free routing check
// described by workflow.preflight_contribution. Unlike portfolio sync, it does
// not write observations, create jobs, create worktrees, or adopt paths.
func (r *MCPReader) PreflightContribution(ctx context.Context, in mcpcontract.ContributionPreflightInput) (mcpcontract.ContributionPreflightOutput, error) {
	repository, err := domain.NewRepoRef(in.Repository.Owner, in.Repository.Repo)
	if err != nil {
		return mcpcontract.ContributionPreflightOutput{}, err
	}
	in.Repository.Owner = repository.Owner()
	in.Repository.Repo = repository.Repo()
	if in.Fork != nil {
		fork, err := domain.NewRepoRef(in.Fork.Owner, in.Fork.Repo)
		if err != nil {
			return mcpcontract.ContributionPreflightOutput{}, fmt.Errorf("validate fork repository: %w", err)
		}
		in.Fork.Owner = fork.Owner()
		in.Fork.Repo = fork.Repo()
		if sameGitHubRepository(in.Fork.Owner, in.Fork.Repo, in.Repository) {
			return mcpcontract.ContributionPreflightOutput{}, errors.New("fork repository must differ from the upstream repository")
		}
	}
	candidate, err := parsePreflightCandidate(in.Candidate)
	if err != nil {
		return mcpcontract.ContributionPreflightOutput{}, err
	}
	if in.Limit == 0 {
		in.Limit = defaultContributionPreflightLimit
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpcontract.ContributionPreflightOutput{}, errors.New("limit must be between 1 and 100")
	}
	if in.MaxRequests == 0 {
		in.MaxRequests = defaultContributionPreflightMaxRequests
	}
	if in.MaxRequests < 2 || in.MaxRequests > 1000 {
		return mcpcontract.ContributionPreflightOutput{}, errors.New("max_requests must be between 2 and 1000")
	}
	if !candidate.hasInputContext(len(in.WorkspacePaths)) {
		return mcpcontract.ContributionPreflightOutput{}, errors.New("candidate or workspace_paths must provide contribution context")
	}

	out := mcpcontract.ContributionPreflightOutput{
		Status:     "coverage_unknown",
		Repository: in.Repository,
		Coverage:   "coverage_unknown",
		NextAction: "complete_preflight_coverage",
	}

	worktrees, worktreeReasons := inspectPreflightWorktrees(ctx, in.WorkspacePaths)
	out.CoverageReasons = append(out.CoverageReasons, worktreeReasons...)

	reader, readerErr := r.githubReader() //nolint:contextcheck // construction performs no request
	if readerErr != nil {
		out.CoverageReasons = append(out.CoverageReasons, "authenticated GitHub reader is unavailable")
		return out, nil //nolint:nilerr // unavailable optional live reads are a structured unknown result
	}
	identityReader, ok := reader.(github.IdentityReader)
	if !ok {
		out.CoverageReasons = append(out.CoverageReasons, "configured GitHub reader does not support authenticated identity lookup")
		return out, nil
	}
	identity, _, err := identityReader.GetAuthenticatedIdentity(ctx)
	if err != nil {
		if contextError(err) {
			return mcpcontract.ContributionPreflightOutput{}, err
		}
		out.CoverageReasons = append(out.CoverageReasons, "authenticated GitHub identity could not be resolved")
		return out, nil
	}
	if strings.TrimSpace(identity.Login) == "" {
		out.CoverageReasons = append(out.CoverageReasons, "authenticated GitHub identity did not include a login")
		return out, nil
	}
	out.Identity = identity.Login
	if !candidate.hasComparableIdentity(worktrees) {
		out.CoverageReasons = append(out.CoverageReasons, "candidate has no comparable title, branch, commit, issue, or inspected worktree identity")
	}

	searcher, ok := reader.(github.AuthoredPullRequestSearcher)
	if !ok {
		out.CoverageReasons = append(out.CoverageReasons, "configured GitHub reader does not support authored pull-request search")
		return out, nil
	}
	requests := 1 // identity lookup
	if requests >= in.MaxRequests {
		out.CoverageReasons = append(out.CoverageReasons, "request budget cannot fund authored pull-request discovery")
		return out, nil
	}
	requests++
	authored, err := searcher.SearchAuthoredPullRequests(ctx, github.AuthoredPullRequestSearchOptions{
		Login: identity.Login, RepositoryOwner: in.Repository.Owner, RepositoryName: in.Repository.Repo,
		State: "open", PageOptions: github.PageOptions{Page: 1, PerPage: in.Limit},
	})
	if err != nil {
		if contextError(err) {
			return mcpcontract.ContributionPreflightOutput{}, err
		}
		out.CoverageReasons = append(out.CoverageReasons, "live authored pull-request search failed")
		return out, nil
	}
	if authored.Incomplete || authored.Page.HasNext || authored.Total > len(authored.Items) {
		out.CoverageReasons = append(out.CoverageReasons, "authored pull-request search coverage is incomplete")
	}

	threadSearcher, hasThreadSearch := reader.(github.ThreadSearcher)
	query := candidate.searchQuery()
	if !hasThreadSearch {
		out.CoverageReasons = append(out.CoverageReasons, "configured GitHub reader does not support related-thread search")
	} else if requests >= in.MaxRequests {
		out.CoverageReasons = append(out.CoverageReasons, "request budget cannot fund related-thread search")
	} else {
		requests++
		related, searchErr := threadSearcher.SearchThreads(ctx, github.ThreadSearchOptions{
			Owner: in.Repository.Owner, Repo: in.Repository.Repo, Query: query, State: "open",
			Sort: "updated", Order: "desc", PageOptions: github.PageOptions{Page: 1, PerPage: in.Limit},
		})
		if searchErr != nil {
			if contextError(searchErr) {
				return mcpcontract.ContributionPreflightOutput{}, searchErr
			}
			out.CoverageReasons = append(out.CoverageReasons, "live related-thread search failed")
		} else {
			out.Related = relatedThreadOutputs(related.Items)
			if related.Incomplete || related.Page.HasNext || related.Total > len(related.Items) {
				out.CoverageReasons = append(out.CoverageReasons, "related-thread search coverage is incomplete")
			}
		}
	}

	detailsReader := reader
	existing := make([]preflightExisting, 0)
	for _, marker := range authored.Items {
		if !sameRepository(marker.RepositoryOwner, marker.RepositoryName, in.Repository) || marker.Kind != domain.PullRequestKind {
			continue
		}
		if requests >= in.MaxRequests {
			out.CoverageReasons = append(out.CoverageReasons, "request budget prevented complete pull-request detail inspection")
			break
		}
		requests++
		details, _, detailErr := detailsReader.GetPullRequestDetails(ctx, in.Repository.Owner, in.Repository.Repo, marker.Number)
		if detailErr != nil {
			if contextError(detailErr) {
				return mcpcontract.ContributionPreflightOutput{}, detailErr
			}
			out.CoverageReasons = append(out.CoverageReasons, fmt.Sprintf("pull-request #%d details could not be inspected", marker.Number))
			continue
		}
		if preflightMatchesCandidate(candidate, marker, details, worktrees) {
			existing = append(existing, preflightExisting{marker: marker, details: details})
		}
	}
	if len(existing) > 0 {
		sort.Slice(existing, func(i, j int) bool { return existing[i].marker.Number < existing[j].marker.Number })
		match := existing[0]
		issue := candidate.issueNumber
		if issue == 0 {
			issue = relatedIssueNumber(out.Related, candidate)
		}
		headOwner := match.details.HeadOwner
		if headOwner == "" {
			headOwner = identity.Login
		}
		head := strings.TrimSpace(headOwner + ":" + match.details.HeadRef)
		if strings.HasSuffix(head, ":") {
			head = ""
		}
		out.Existing = &mcpcontract.ExistingContributionOutput{Issue: issue, PullRequest: match.marker.Number, Head: head, HeadSHA: match.details.HeadSHA, URL: match.details.HTMLURL}
		out.LocalMatches = localMatchOutputs(worktrees, existing)
	}

	if out.Existing != nil {
		out.Status = "existing_pr"
		out.NextAction = "review_or_follow_through"
	}
	forkFreshness, forkChecked, forkErr := checkPreflightForkFreshness(ctx, reader, in.Repository, in.Fork, identity.Login, candidate, worktrees, existingMatch(existing), in.MaxRequests, requests)
	if forkErr != nil {
		return mcpcontract.ContributionPreflightOutput{}, forkErr
	}
	if forkChecked {
		out.ForkFreshness = &forkFreshness
		if forkFreshness.Coverage != "verified" {
			out.CoverageReasons = append(out.CoverageReasons, "fork freshness coverage is unavailable: "+forkFreshness.Reason)
		}
	}
	if len(out.CoverageReasons) == 0 {
		out.Coverage = "live_verified"
		if out.Existing == nil {
			out.Status = "new_work"
			out.NextAction = "create_local_work"
		}
	}
	return out, nil
}

func existingMatch(existing []preflightExisting) *preflightExisting {
	if len(existing) == 0 {
		return nil
	}
	return &existing[0]
}

type preflightExisting struct {
	marker  github.Issue
	details github.PullRequestDetails
}

func contextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func sameRepository(owner, repo string, ref mcpcontract.RepositoryRef) bool {
	return strings.EqualFold(strings.TrimSpace(owner), strings.TrimSpace(ref.Owner)) && strings.EqualFold(strings.TrimSpace(repo), strings.TrimSpace(ref.Repo))
}

func inspectPreflightWorktrees(ctx context.Context, paths []string) ([]workspace.LocalWorktree, []string) {
	var inspected []workspace.LocalWorktree
	var reasons []string
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			reasons = append(reasons, "a supplied workspace path is empty")
			continue
		}
		worktree, err := workspace.InspectPath(ctx, path, nil)
		if err != nil {
			reasons = append(reasons, "local workspace inspection failed for "+path)
			continue
		}
		inspected = append(inspected, worktree)
	}
	return inspected, reasons
}

func relatedThreadOutputs(items []github.Issue) []mcpcontract.RelatedContributionThread {
	out := make([]mcpcontract.RelatedContributionThread, 0, len(items))
	for _, item := range items {
		out = append(out, mcpcontract.RelatedContributionThread{Kind: string(item.Kind), Number: item.Number, Title: item.Title, Author: item.Author, URL: item.HTMLURL})
	}
	return out
}

func relatedIssueNumber(items []mcpcontract.RelatedContributionThread, candidate preflightCandidate) int {
	for _, item := range items {
		if item.Kind == string(domain.IssueKind) && textSimilarity(candidate, item.Title) >= 0.5 {
			return item.Number
		}
	}
	return 0
}

func preflightMatchesCandidate(candidate preflightCandidate, marker github.Issue, details github.PullRequestDetails, worktrees []workspace.LocalWorktree) bool {
	if candidate.issueNumber > 0 && marker.Number == candidate.issueNumber {
		return true
	}
	if candidate.headSHA != "" && strings.EqualFold(candidate.headSHA, strings.TrimSpace(details.HeadSHA)) {
		return true
	}
	if candidate.headRef != "" && strings.EqualFold(candidate.headRef, strings.TrimSpace(details.HeadRef)) {
		return true
	}
	for _, worktree := range worktrees {
		if worktree.HeadSHA != "" && strings.EqualFold(worktree.HeadSHA, details.HeadSHA) {
			return true
		}
		if worktree.Branch != "" && strings.EqualFold(worktree.Branch, details.HeadRef) {
			return true
		}
		for _, urls := range worktree.Remotes {
			for _, remote := range urls {
				identity, err := gitremote.ParseRepositoryIdentity(remote)
				if err == nil && strings.EqualFold(identity.Owner, details.HeadOwner) && strings.EqualFold(identity.Repo, details.HeadRepo) && strings.EqualFold(worktree.Branch, details.HeadRef) {
					return true
				}
			}
		}
	}
	return textSimilarity(candidate, marker.Title) >= 0.5
}

func localMatchOutputs(worktrees []workspace.LocalWorktree, existing []preflightExisting) []mcpcontract.LocalContributionMatch {
	var out []mcpcontract.LocalContributionMatch
	for _, worktree := range worktrees {
		for _, match := range existing {
			if (worktree.HeadSHA != "" && strings.EqualFold(worktree.HeadSHA, match.details.HeadSHA)) || (worktree.Branch != "" && strings.EqualFold(worktree.Branch, match.details.HeadRef)) {
				remote := ""
				if values := worktree.Remotes["origin"]; len(values) > 0 {
					remote = values[0]
				}
				out = append(out, mcpcontract.LocalContributionMatch{Path: worktree.Path, Branch: worktree.Branch, Remote: remote, PullRequest: match.marker.Number})
				break
			}
		}
	}
	return out
}

func textSimilarity(candidate preflightCandidate, other string) float64 {
	left := candidate.query
	if left == "" {
		left = candidate.title
	}
	if left == "" {
		left = candidate.body
	}
	leftTokens := preflightTokens(left)
	rightTokens := preflightTokens(other)
	if len(leftTokens) == 0 || len(rightTokens) == 0 {
		return 0
	}
	shared := 0
	for token := range leftTokens {
		if _, ok := rightTokens[token]; ok {
			shared++
		}
	}
	denominator := len(leftTokens)
	if len(rightTokens) > denominator {
		denominator = len(rightTokens)
	}
	return float64(shared) / float64(denominator)
}

func preflightTokens(value string) map[string]struct{} {
	value = strings.ToLower(value)
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			builder.WriteRune(r)
		} else {
			builder.WriteByte(' ')
		}
	}
	tokens := make(map[string]struct{})
	for _, token := range strings.Fields(builder.String()) {
		if len([]rune(token)) < 3 {
			continue
		}
		switch token {
		case "the", "and", "for", "with", "from", "this", "that", "fix", "add", "issue":
			continue
		}
		tokens[token] = struct{}{}
	}
	return tokens
}
