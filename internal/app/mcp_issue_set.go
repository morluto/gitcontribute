package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
	"github.com/morluto/gitcontribute/internal/radar"
	"github.com/morluto/gitcontribute/internal/relatedwork"
)

const (
	defaultIssueSetPrecedentLimit = 3
	conciseIssueSetRelatedLimit   = 5
)

type prepareIssueSetRequest struct {
	repository     domain.RepoRef
	issueNumbers   []int
	precedentLimit int
	format         responseFormat
	snapshotToken  string
}

// PrepareIssueSet composes contribution-facing evidence for exact stored
// issues. It opens only the read-only corpus and never creates workflow state.
func (r *MCPReader) PrepareIssueSet(ctx context.Context, in mcpcontract.PrepareIssueSetInput) (mcpcontract.PrepareIssueSetOutput, error) {
	request, err := parsePrepareIssueSetInput(in)
	if err != nil {
		return mcpcontract.PrepareIssueSetOutput{}, err
	}
	ref := request.repository
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.PrepareIssueSetOutput{}, err
	}
	revision, err := beginCorpusRead(ctx, c, request.snapshotToken)
	if err != nil {
		return mcpcontract.PrepareIssueSetOutput{}, err
	}
	out := mcpcontract.PrepareIssueSetOutput{
		Status: "complete", Owner: ref.Owner(), Repo: ref.Repo(), ResponseFormat: request.format.String(),
		Items:         make([]mcpcontract.BatchItem[mcpcontract.PreparedIssueEvidence], len(request.issueNumbers)),
		Coverage:      []mcpcontract.FacetCoverageOutput{},
		SnapshotToken: snapshotIdentity(request.snapshotToken, revision),
	}
	stored, err := c.GetRepository(ctx, ref.Owner(), ref.Repo())
	if err != nil {
		return mcpcontract.PrepareIssueSetOutput{}, err
	}
	if stored == nil {
		if err := finishCorpusRead(ctx, c, revision); err != nil {
			return mcpcontract.PrepareIssueSetOutput{}, err
		}
		return unavailableIssueSet(request, out), nil
	}
	threadsCoverage, err := c.GetCoverage(ctx, stored.ID, nil, "threads")
	if err != nil {
		return mcpcontract.PrepareIssueSetOutput{}, err
	}
	if threadsCoverage == nil {
		action := repositoryPullRequestSyncAction(ref)
		out.Coverage = []mcpcontract.FacetCoverageOutput{{Facet: "threads", Status: "unknown"}}
		out.Gaps = append(out.Gaps, mcpcontract.IssueSetGap{
			Code: "relationship_population_unknown", Facet: "threads", Status: "unknown",
			Message: "repository thread coverage is not recorded, so the stored pull-request population may be incomplete", Recovery: recoveryPlan("coverage_stale", "Complete the stored pull-request population used for related-work analysis.", action),
		})
		out.RecoveryPlans = append(out.RecoveryPlans, *recoveryPlan("coverage_stale", "Complete the stored pull-request population used for related-work analysis.", action))
		out.Status = "partial"
	} else {
		status := "complete"
		if !threadsCoverage.Complete {
			status = "partial"
			action := repositoryPullRequestSyncAction(ref)
			out.Gaps = append(out.Gaps, mcpcontract.IssueSetGap{
				Code: "relationship_population_incomplete", Facet: "threads", Status: "partial",
				Message: "repository thread coverage is incomplete, so the stored pull-request population is not exhaustive", Recovery: recoveryPlan("facet_incomplete", "Complete the stored pull-request population used for related-work analysis.", action),
			})
			out.RecoveryPlans = append(out.RecoveryPlans, *recoveryPlan("facet_incomplete", "Complete the stored pull-request population used for related-work analysis.", action))
			out.Status = "partial"
		}
		out.Coverage = []mcpcontract.FacetCoverageOutput{{
			Facet: "threads", Complete: threadsCoverage.Complete, Status: status, UpdatedAt: formatTime(threadsCoverage.SourceUpdatedAt),
		}}
		out.SourceAsOf = formatTime(threadsCoverage.SourceUpdatedAt)
	}

	issues := make([]corpus.Thread, 0, len(request.issueNumbers))
	issuesByNumber := make(map[int]corpus.Thread, len(request.issueNumbers))
	for _, number := range request.issueNumbers {
		issue, err := c.GetThread(ctx, stored.ID, domain.IssueKind, number)
		if err != nil {
			return mcpcontract.PrepareIssueSetOutput{}, err
		}
		if issue != nil {
			issues = append(issues, *issue)
			issuesByNumber[number] = *issue
		}
	}

	pullRequests, err := c.ListThreadsFiltered(ctx, stored.ID, corpus.PullRequestThreadKind(), corpus.AnyThreadState(), radarPullRequestPopulation)
	if err != nil {
		return mcpcontract.PrepareIssueSetOutput{}, err
	}
	pullRequestTotal, err := c.CountThreadsFiltered(ctx, stored.ID, corpus.PullRequestThreadKind(), corpus.AnyThreadState())
	if err != nil {
		return mcpcontract.PrepareIssueSetOutput{}, err
	}
	out.RelationshipPopulation = pullRequestTotal
	out.RelationshipConsidered = len(pullRequests)
	relationshipScanCapped := pullRequestTotal > len(pullRequests)
	out.Truncated = relationshipScanCapped
	if out.Truncated {
		out.Status = "partial"
	}
	relatedByIssue, projectedCapped, err := radarPullRequestRelatedWork(ctx, c, stored, ref, issues, pullRequests, corpus.AnyThreadState())
	if err != nil {
		return mcpcontract.PrepareIssueSetOutput{}, err
	}
	if projectedCapped {
		relationshipScanCapped = true
		out.Status, out.Truncated = "partial", true
	}
	duplicatesByIssue, duplicateCapped, err := radarDuplicateClusters(ctx, c, ref)
	if err != nil {
		return mcpcontract.PrepareIssueSetOutput{}, err
	}
	if duplicateCapped {
		out.Status, out.Truncated = "partial", true
	}

	precedentInput := issueSetPrecedentInput(request)
	// Reuse only a caller-supplied durable token. The response token for an
	// unpinned read is an ephemeral result identity and cannot be resolved by a
	// nested read.
	precedentInput.SnapshotToken = request.snapshotToken
	precedents, err := r.FindPrecedents(ctx, precedentInput)
	if err != nil {
		return mcpcontract.PrepareIssueSetOutput{}, err
	}
	pullRequestsByNumber := make(map[int]corpus.Thread, len(pullRequests))
	pullRequestBodiesAvailable := true
	for _, pullRequest := range pullRequests {
		pullRequestsByNumber[pullRequest.Number] = pullRequest
		if pullRequest.Body == "" {
			pullRequestBodiesAvailable = false
		}
	}

	evaluatedAt := r.now()
	for i, number := range request.issueNumbers {
		key := threadRefKey(mcpcontract.ThreadRef{Owner: ref.Owner(), Repo: ref.Repo(), Kind: string(domain.IssueKind), Number: number})
		item := mcpcontract.BatchItem[mcpcontract.PreparedIssueEvidence]{Key: key, Status: "complete"}
		issue, ok := issuesByNumber[number]
		if !ok {
			item.Status, item.Reason = "unavailable", "thread_not_indexed"
			item.Message = "issue is not present in the local corpus"
			item.Recovery = recoveryPlan("thread_not_indexed", item.Message, issueSyncAction(ref, number))
			out.Items[i] = item
			out.Status = "partial"
			out.RecoveryPlans = append(out.RecoveryPlans, *recoveryPlan("thread_not_indexed", item.Message, issueSyncAction(ref, number)))
			continue
		}
		value, actions, partial, err := prepareOneIssue(
			ctx, c, stored, ref, issue, relatedByIssue[number], pullRequestsByNumber, duplicatesByIssue[number],
			precedents.Items[i], request.format, request.precedentLimit,
			threadsCoverage != nil && threadsCoverage.Complete && !relationshipScanCapped && pullRequestBodiesAvailable,
			threadsCoverage == nil || !threadsCoverage.SourceUpdatedAt.Before(issue.SourceUpdatedAt), evaluatedAt,
		)
		if err != nil {
			return mcpcontract.PrepareIssueSetOutput{}, err
		}
		if partial {
			out.Status = "partial"
		}
		if sourceAsOf := preparedIssueSourceAsOf(value); sourceAsOf > out.SourceAsOf {
			out.SourceAsOf = sourceAsOf
		}
		item.Value = &value
		out.Items[i] = item
		out.RecoveryPlans = append(out.RecoveryPlans, actions...)
		if value.RelatedWorkTruncated {
			out.Truncated = true
		}
		if item := precedents.Items[i].Value; item != nil && item.Truncated {
			out.Status, out.Truncated = "partial", true
		}
	}
	if err := finishCorpusRead(ctx, c, revision); err != nil {
		return mcpcontract.PrepareIssueSetOutput{}, err
	}
	return out, nil
}

func parsePrepareIssueSetInput(in mcpcontract.PrepareIssueSetInput) (prepareIssueSetRequest, error) {
	if len(in.IssueNumbers) < 1 || len(in.IssueNumbers) > 20 {
		return prepareIssueSetRequest{}, errors.New("issue_numbers must contain 1 to 20 items")
	}
	seen := make(map[int]struct{}, len(in.IssueNumbers))
	for _, number := range in.IssueNumbers {
		if number < 1 {
			return prepareIssueSetRequest{}, errors.New("issue_numbers must contain only positive numbers")
		}
		if _, ok := seen[number]; ok {
			return prepareIssueSetRequest{}, fmt.Errorf("issue_numbers contains duplicate #%d", number)
		}
		seen[number] = struct{}{}
	}
	if in.PrecedentLimit == 0 {
		in.PrecedentLimit = defaultIssueSetPrecedentLimit
	}
	if in.PrecedentLimit < 1 || in.PrecedentLimit > 10 {
		return prepareIssueSetRequest{}, errors.New("precedent_limit must be between 1 and 10")
	}
	format, err := parseResponseFormat(in.ResponseFormat)
	if err != nil {
		return prepareIssueSetRequest{}, err
	}
	repository, err := domain.NewRepoRef(in.Owner, in.Repo)
	if err != nil {
		return prepareIssueSetRequest{}, err
	}
	return prepareIssueSetRequest{
		repository: repository, issueNumbers: append([]int(nil), in.IssueNumbers...),
		precedentLimit: in.PrecedentLimit, format: format, snapshotToken: in.SnapshotToken,
	}, nil
}

func unavailableIssueSet(request prepareIssueSetRequest, out mcpcontract.PrepareIssueSetOutput) mcpcontract.PrepareIssueSetOutput {
	out.Status = "partial"
	for i, number := range request.issueNumbers {
		out.Items[i] = mcpcontract.BatchItem[mcpcontract.PreparedIssueEvidence]{
			Key: threadRefKey(mcpcontract.ThreadRef{Owner: request.repository.Owner(), Repo: request.repository.Repo(), Kind: string(domain.IssueKind), Number: number}), Status: "unavailable",
			Reason: "repository_not_indexed", Message: "repository is not present in the local corpus",
			Recovery: recoveryPlan("repository_not_indexed", "Synchronize the repository, then retry this exact issue.", syncRepositoryContextCall(request.repository.Owner(), request.repository.Repo()), issueSyncAction(request.repository, number)),
		}
		out.RecoveryPlans = append(out.RecoveryPlans, *recoveryPlan("repository_not_indexed", "Synchronize the repository, then retry this exact issue.", syncRepositoryContextCall(request.repository.Owner(), request.repository.Repo()), issueSyncAction(request.repository, number)))
	}
	return out
}

func issueSetPrecedentInput(request prepareIssueSetRequest) mcpcontract.FindPrecedentsInput {
	threads := make([]mcpcontract.ThreadRef, len(request.issueNumbers))
	for i, number := range request.issueNumbers {
		threads[i] = mcpcontract.ThreadRef{Owner: request.repository.Owner(), Repo: request.repository.Repo(), Kind: string(domain.IssueKind), Number: number}
	}
	return mcpcontract.FindPrecedentsInput{Threads: threads, Limit: 100, SnapshotToken: request.snapshotToken}
}

func prepareOneIssue(
	ctx context.Context,
	c *corpus.Corpus,
	stored *corpus.Repository,
	ref domain.RepoRef,
	issue corpus.Thread,
	inbound []radar.RelatedWork,
	pullRequests map[int]corpus.Thread,
	duplicate *radar.DuplicateCluster,
	precedents mcpcontract.BatchItem[mcpcontract.PrecedentSet],
	format responseFormat,
	precedentLimit int,
	relationshipPopulationComplete bool,
	relationshipPopulationFresh bool,
	evaluatedAt time.Time,
) (mcpcontract.PreparedIssueEvidence, []mcpcontract.RecoveryPlan, bool, error) {
	value := mcpcontract.PreparedIssueEvidence{
		Number: issue.Number, Title: issue.Title, State: string(issue.State), StateReason: issue.StateReason,
		Labels: append([]string(nil), issue.Labels...), BodyStatus: "unknown",
		SourceUpdatedAt: formatTime(issue.SourceUpdatedAt), Coverage: []mcpcontract.FacetCoverageOutput{},
		RelatedWork: []mcpcontract.IssueSetRelatedWork{}, AcceptedExamples: []mcpcontract.PrecedentOutput{},
		Linkage: mcpcontract.IssueSetLinkageCandidate{
			IssueNumber: issue.Number, Relation: "related", AllowedRelations: []string{"closes", "advances", "related"},
			RequiresConfirmation: true, Basis: "The caller explicitly included this issue; the implementation relationship has not been validated.",
		},
	}
	actions := []mcpcontract.RecoveryPlan{}
	partial := false
	if !relationshipPopulationFresh {
		action := repositoryPullRequestSyncAction(ref)
		value.Gaps = append(value.Gaps, mcpcontract.IssueSetGap{
			Code: "relationship_coverage_stale", Facet: "threads", Status: "unknown",
			Message: "repository relationship coverage predates the issue observation", Recovery: recoveryPlan("coverage_stale", "Repository relationship coverage predates the issue observation.", action),
		})
		actions, partial = append(actions, *recoveryPlan("coverage_stale", "Repository relationship coverage predates the issue observation.", action)), true
		relationshipPopulationComplete = false
	}
	relationshipEvidenceComplete := issue.Body != ""
	if issue.Body != "" {
		value.BodyStatus = "available"
		if format.includesDetails() {
			value.Body = issue.Body
		}
	} else {
		action := issueSyncAction(ref, issue.Number)
		value.Gaps = append(value.Gaps, mcpcontract.IssueSetGap{
			Code: "body_unknown", Facet: "body", Status: "unknown",
			Message: "the corpus does not distinguish a known-empty body from a body that was not captured", Recovery: recoveryPlan("facet_not_observed", "Fetch the exact issue header and body into the local corpus.", action),
		})
		actions, partial = append(actions, *recoveryPlan("facet_not_observed", "Fetch the exact issue header and body into the local corpus.", action)), true
	}

	for _, facet := range []string{FacetIssueComments, FacetIssueTimeline} {
		coverage, err := c.GetCoverage(ctx, stored.ID, &issue.ID, facet)
		if err != nil {
			return mcpcontract.PreparedIssueEvidence{}, nil, false, err
		}
		if coverage == nil {
			relationshipEvidenceComplete = false
			value.Coverage = append(value.Coverage, mcpcontract.FacetCoverageOutput{Facet: facet, Status: "unknown"})
			action := issueHydrateAction(ref, issue.Number, facet)
			value.Gaps = append(value.Gaps, mcpcontract.IssueSetGap{
				Code: "facet_missing", Facet: facet, Status: "unknown",
				Message: "no coverage observation is stored for this facet", Recovery: recoveryPlan("facet_not_observed", "Complete the missing issue evidence facet.", action),
			})
			actions, partial = append(actions, *recoveryPlan("facet_not_observed", "Complete the missing issue evidence facet.", action)), true
			continue
		}
		status := "complete"
		if !coverage.Complete {
			status = "partial"
		}
		value.Coverage = append(value.Coverage, mcpcontract.FacetCoverageOutput{
			Facet: facet, Complete: coverage.Complete, Status: status, UpdatedAt: formatTime(coverage.SourceUpdatedAt),
		})
		if !coverage.Complete {
			relationshipEvidenceComplete = false
			action := issueHydrateAction(ref, issue.Number, facet)
			value.Gaps = append(value.Gaps, mcpcontract.IssueSetGap{
				Code: "facet_incomplete", Facet: facet, Status: "partial",
				Message: "stored facet coverage is incomplete", Recovery: recoveryPlan("facet_incomplete", "Complete the missing issue evidence facet.", action),
			})
			actions, partial = append(actions, *recoveryPlan("facet_incomplete", "Complete the missing issue evidence facet.", action)), true
		}
	}

	_, outbound, relatedCapped, err := radarIssueDiscussionAndRelatedWork(ctx, c, stored, issue, ref, evaluatedAt)
	if err != nil {
		return mcpcontract.PreparedIssueEvidence{}, nil, false, err
	}
	combined, relatedTotal, recordsCapped, evidenceCapped := normalizeRadarRelatedWorkDetails(
		append(append([]radar.RelatedWork(nil), inbound...), outbound...), maxRadarRelatedWork,
	)
	value.RelatedWorkTotal = relatedTotal
	value.RelatedWorkTotalKnown = relationshipPopulationComplete && relationshipEvidenceComplete && !relatedCapped
	limit := len(combined)
	if !format.includesDetails() && limit > conciseIssueSetRelatedLimit {
		limit = conciseIssueSetRelatedLimit
	}
	conciseEvidenceOmitted := false
	for _, work := range combined[:limit] {
		if !format.includesDetails() && len(work.Evidence) > 0 {
			conciseEvidenceOmitted = true
		}
		value.RelatedWork = append(value.RelatedWork, issueSetRelatedWork(work, ref, pullRequests, format))
	}
	value.RelatedWorkTruncated = relatedCapped || recordsCapped || evidenceCapped || conciseEvidenceOmitted || limit < len(combined)
	if relatedCapped || recordsCapped || evidenceCapped {
		partial = true
	}
	if duplicate != nil {
		value.DuplicateCluster = &mcpcontract.IssueSetDuplicateCluster{
			StableID: duplicate.StableID, CanonicalRef: duplicate.CanonicalRef, CandidateCount: duplicate.CandidateCount,
		}
	}
	if precedents.Status != "complete" || precedents.Value == nil {
		action := repositoryHistorySyncAction(ref)
		value.Gaps = append(value.Gaps, mcpcontract.IssueSetGap{
			Code: "precedent_evidence_unavailable", Facet: "precedents", Status: "unknown",
			Message: "historical precedent evidence is unavailable for this issue", Recovery: recoveryPlan("coverage_stale", "Fetch closed issue and pull-request headers used for historical precedent analysis.", action),
		})
		actions, partial = append(actions, *recoveryPlan("coverage_stale", "Fetch closed issue and pull-request headers used for historical precedent analysis.", action)), true
	} else {
		for _, match := range precedents.Value.Matches {
			if match.Kind == string(domain.PullRequestKind) && match.MergedAt != "" {
				value.AcceptedExamples = append(value.AcceptedExamples, match)
				if len(value.AcceptedExamples) == precedentLimit {
					break
				}
			}
		}
	}
	value.ContributionDisposition = issueContributionDisposition(value)
	return value, actions, partial, nil
}

func issueContributionDisposition(issue mcpcontract.PreparedIssueEvidence) mcpcontract.ContributionDisposition {
	unknown := func(reasons ...string) mcpcontract.ContributionDisposition {
		return mcpcontract.ContributionDisposition{
			Status: "unknown", Confidence: "low", Unknowns: reasons,
			Recovery: recoveryPlan("blocked", "Complete the listed evidence gaps, then prepare this exact issue set again."),
		}
	}
	issueRef := fmt.Sprintf("issue:#%d", issue.Number)
	if strings.EqualFold(issue.StateReason, "not_planned") || slices.ContainsFunc(issue.Labels, func(label string) bool {
		label = strings.ToLower(strings.TrimSpace(label))
		return label == "duplicate" || label == "wontfix" || label == "wont-fix"
	}) {
		return mcpcontract.ContributionDisposition{
			Status: "blocked_by_repository_policy", Confidence: "high", EvidenceRefs: []string{issueRef},
			Recovery: recoveryPlan("blocked", "Do not create an implementation workspace unless a maintainer reopens or redirects the issue."),
		}
	}
	var mergedClosing, openClosing, closedUnmerged []mcpcontract.IssueSetRelatedWork
	var missingMerge []string
	for _, work := range issue.RelatedWork {
		if work.Kind != string(domain.PullRequestKind) || work.Relation != string(relatedwork.RelationClaimsToClose) || work.Direction != string(radar.RelatedWorkInbound) {
			continue
		}
		switch {
		case work.Merged != nil && *work.Merged:
			mergedClosing = append(mergedClosing, work)
		case strings.EqualFold(work.State, "open"):
			openClosing = append(openClosing, work)
		case work.Merged != nil && !*work.Merged:
			closedUnmerged = append(closedUnmerged, work)
		default:
			missingMerge = append(missingMerge, work.Ref)
		}
	}
	if len(mergedClosing) > 0 {
		return mcpcontract.ContributionDisposition{
			Status: "already_resolved_upstream", Confidence: "high", EvidenceRefs: relatedWorkRefs(mergedClosing),
			Recovery: recoveryPlan("blocked", "Verify the released behavior before considering any follow-up contribution."),
		}
	}
	if len(missingMerge) > 0 {
		return unknown("the merge outcome is unknown for " + strings.Join(missingMerge, ", "))
	}
	if !issue.RelatedWorkTotalKnown {
		return unknown("related-work population or relationship evidence is incomplete")
	}
	if len(openClosing) > 0 {
		return mcpcontract.ContributionDisposition{
			Status: "active_competing_work", Confidence: "high", EvidenceRefs: relatedWorkRefs(openClosing),
			Recovery: recoveryPlan("blocked", "Coordinate with the active pull request before creating another implementation workspace."),
		}
	}
	if len(closedUnmerged) > 0 {
		if !hasCompleteIssueFacet(issue.Coverage, FacetIssueComments) {
			return unknown("a closing pull request was closed unmerged, but maintainer discussion is incomplete")
		}
		return mcpcontract.ContributionDisposition{
			Status: "needs_maintainer_alignment", Confidence: "medium", EvidenceRefs: relatedWorkRefs(closedUnmerged),
			Recovery: recoveryPlan("blocked", "Confirm the desired semantics and acceptable approach with maintainers before coding."),
		}
	}
	if !strings.EqualFold(issue.State, "open") {
		return unknown("the issue is not open and no deterministic resolution or policy reason is stored")
	}
	return mcpcontract.ContributionDisposition{
		Status: "ready_to_investigate", Confidence: "medium", EvidenceRefs: []string{issueRef},
		Recovery: recoveryPlan("blocked", "Investigate the current behavior and contribution fit before creating an implementation workspace."),
	}
}

func relatedWorkRefs(values []mcpcontract.IssueSetRelatedWork) []string {
	refs := make([]string, 0, len(values))
	for _, value := range values {
		refs = append(refs, value.Ref)
	}
	slices.Sort(refs)
	return slices.Compact(refs)
}

func hasCompleteIssueFacet(values []mcpcontract.FacetCoverageOutput, facet string) bool {
	return slices.ContainsFunc(values, func(value mcpcontract.FacetCoverageOutput) bool {
		return value.Facet == facet && value.Complete
	})
}

func issueSetRelatedWork(work radar.RelatedWork, ref domain.RepoRef, pullRequests map[int]corpus.Thread, format responseFormat) mcpcontract.IssueSetRelatedWork {
	out := mcpcontract.IssueSetRelatedWork{
		Ref: work.Ref, Kind: work.Kind, Number: work.Number, Title: work.Title, State: work.State,
		Relation: string(work.Relation), Direction: string(work.Direction), URL: work.URL,
		SourceUpdatedAt: formatTime(work.SourceUpdatedAt),
	}
	localPullRequestRef := fmt.Sprintf("pull_request:%s#%d", ref, work.Number)
	if pullRequest, ok := pullRequests[work.Number]; ok && work.Kind == string(domain.PullRequestKind) && work.Ref == localPullRequestRef {
		if pullRequest.Merge.Known() {
			merged := pullRequest.Merge.IsMerged()
			out.Merged = &merged
		}
		out.MergedAt = formatTime(pullRequest.Merge.MergedAt())
	}
	if format.includesDetails() {
		seen := map[string]struct{}{}
		for _, evidence := range work.Evidence {
			if _, ok := seen[evidence.Kind]; ok {
				continue
			}
			seen[evidence.Kind] = struct{}{}
			out.EvidenceKinds = append(out.EvidenceKinds, evidence.Kind)
		}
	}
	return out
}

func preparedIssueSourceAsOf(value mcpcontract.PreparedIssueEvidence) string {
	latest := value.SourceUpdatedAt
	for _, coverage := range value.Coverage {
		if coverage.UpdatedAt > latest {
			latest = coverage.UpdatedAt
		}
	}
	for _, work := range value.RelatedWork {
		if work.SourceUpdatedAt > latest {
			latest = work.SourceUpdatedAt
		}
	}
	return latest
}

func issueSyncAction(ref domain.RepoRef, number int) mcpcontract.ToolCall {
	return mcpcontract.RecoveryAction(mcpcontract.SyncThreadsInput{
		Selection: "threads",
		Threads:   []mcpcontract.ThreadRef{{Owner: ref.Owner(), Repo: ref.Repo(), Kind: string(domain.IssueKind), Number: number}},
	})
}

func issueHydrateAction(ref domain.RepoRef, number int, facet string) mcpcontract.ToolCall {
	return mcpcontract.RecoveryAction(mcpcontract.HydrateThreadsInput{
		Threads: []mcpcontract.ThreadRef{{Owner: ref.Owner(), Repo: ref.Repo(), Kind: string(domain.IssueKind), Number: number}},
		Facets:  []string{facet},
	})
}

func repositoryPullRequestSyncAction(ref domain.RepoRef) mcpcontract.ToolCall {
	return mcpcontract.RecoveryAction(mcpcontract.SyncThreadsInput{
		Selection:    "repositories",
		Repositories: []mcpcontract.RepositoryRef{{Owner: ref.Owner(), Repo: ref.Repo()}},
		Kind:         string(domain.PullRequestKind),
		State:        "all",
	})
}

func repositoryHistorySyncAction(ref domain.RepoRef) mcpcontract.ToolCall {
	return mcpcontract.RecoveryAction(mcpcontract.SyncThreadsInput{
		Selection:    "repositories",
		Repositories: []mcpcontract.RepositoryRef{{Owner: ref.Owner(), Repo: ref.Repo()}},
		Kind:         "both",
		State:        "closed",
	})
}
