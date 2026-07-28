package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
	"github.com/morluto/gitcontribute/internal/radar"
)

const (
	defaultIssueSetPrecedentLimit = 3
	conciseIssueSetRelatedLimit   = 5
)

// PrepareIssueSet composes contribution-facing evidence for exact stored
// issues. It opens only the read-only corpus and never creates workflow state.
func (r *MCPReader) PrepareIssueSet(ctx context.Context, in mcpcontract.PrepareIssueSetInput) (mcpcontract.PrepareIssueSetOutput, error) {
	if err := normalizePrepareIssueSetInput(&in); err != nil {
		return mcpcontract.PrepareIssueSetOutput{}, err
	}
	ref := domain.RepoRef{Owner: in.Owner, Repo: in.Repo}
	if err := ref.Validate(); err != nil {
		return mcpcontract.PrepareIssueSetOutput{}, err
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.PrepareIssueSetOutput{}, err
	}
	out := mcpcontract.PrepareIssueSetOutput{
		Status: "complete", Owner: ref.Owner, Repo: ref.Repo, ResponseFormat: in.ResponseFormat,
		Items:    make([]mcpcontract.BatchItem[mcpcontract.PreparedIssueEvidence], len(in.IssueNumbers)),
		Coverage: []mcpcontract.FacetCoverageOutput{},
	}
	stored, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return mcpcontract.PrepareIssueSetOutput{}, err
	}
	if stored == nil {
		return unavailableIssueSet(in, out), nil
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
			Message: "repository thread coverage is not recorded, so the stored pull-request population may be incomplete", NextAction: action,
		})
		out.SuggestedActions = append(out.SuggestedActions, action)
		out.Status = "partial"
	} else {
		status := "complete"
		if !threadsCoverage.Complete {
			status = "partial"
			action := repositoryPullRequestSyncAction(ref)
			out.Gaps = append(out.Gaps, mcpcontract.IssueSetGap{
				Code: "relationship_population_incomplete", Facet: "threads", Status: "partial",
				Message: "repository thread coverage is incomplete, so the stored pull-request population is not exhaustive", NextAction: action,
			})
			out.SuggestedActions = append(out.SuggestedActions, action)
			out.Status = "partial"
		}
		out.Coverage = []mcpcontract.FacetCoverageOutput{{
			Facet: "threads", Complete: threadsCoverage.Complete, Status: status, UpdatedAt: formatTime(threadsCoverage.SourceUpdatedAt),
		}}
		out.SourceAsOf = formatTime(threadsCoverage.SourceUpdatedAt)
	}

	issues := make([]corpus.Thread, 0, len(in.IssueNumbers))
	issuesByNumber := make(map[int]corpus.Thread, len(in.IssueNumbers))
	for _, number := range in.IssueNumbers {
		issue, err := c.GetThread(ctx, stored.ID, corpus.ThreadKindIssue, number)
		if err != nil {
			return mcpcontract.PrepareIssueSetOutput{}, err
		}
		if issue != nil {
			issues = append(issues, *issue)
			issuesByNumber[number] = *issue
		}
	}

	pullRequests, err := c.ListThreadsFiltered(ctx, stored.ID, corpus.ThreadKindPullRequest, "all", radarPullRequestPopulation)
	if err != nil {
		return mcpcontract.PrepareIssueSetOutput{}, err
	}
	pullRequestTotal, err := c.CountThreadsFiltered(ctx, stored.ID, corpus.ThreadKindPullRequest, "all")
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
	relatedByIssue, projectedCapped, err := radarPullRequestRelatedWork(ctx, c, stored, ref, issues, pullRequests, "all")
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

	precedents, err := r.FindPrecedents(ctx, issueSetPrecedentInput(in))
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
	for i, number := range in.IssueNumbers {
		key := fmt.Sprintf("%s/%s#%d", ref.Owner, ref.Repo, number)
		item := mcpcontract.BatchItem[mcpcontract.PreparedIssueEvidence]{Key: key, Status: "complete"}
		issue, ok := issuesByNumber[number]
		if !ok {
			item.Status, item.Reason = "unavailable", "issue_not_indexed"
			item.Message = "issue is not present in the local corpus"
			item.NextAction = "Call github.sync_threads for this exact issue."
			out.Items[i] = item
			out.Status = "partial"
			out.SuggestedActions = append(out.SuggestedActions, issueSyncAction(ref, number))
			continue
		}
		value, actions, partial, err := prepareOneIssue(
			ctx, c, stored, ref, issue, relatedByIssue[number], pullRequestsByNumber, duplicatesByIssue[number],
			precedents.Items[i], in.ResponseFormat, in.PrecedentLimit,
			threadsCoverage != nil && threadsCoverage.Complete && !relationshipScanCapped && pullRequestBodiesAvailable, evaluatedAt,
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
		out.SuggestedActions = append(out.SuggestedActions, actions...)
		if value.RelatedWorkTruncated {
			out.Truncated = true
		}
		if item := precedents.Items[i].Value; item != nil && item.Truncated {
			out.Status, out.Truncated = "partial", true
		}
	}
	return out, nil
}

func normalizePrepareIssueSetInput(in *mcpcontract.PrepareIssueSetInput) error {
	if len(in.IssueNumbers) < 1 || len(in.IssueNumbers) > 20 {
		return errors.New("issue_numbers must contain 1 to 20 items")
	}
	seen := make(map[int]struct{}, len(in.IssueNumbers))
	for _, number := range in.IssueNumbers {
		if number < 1 {
			return errors.New("issue_numbers must contain only positive numbers")
		}
		if _, ok := seen[number]; ok {
			return fmt.Errorf("issue_numbers contains duplicate #%d", number)
		}
		seen[number] = struct{}{}
	}
	if in.PrecedentLimit == 0 {
		in.PrecedentLimit = defaultIssueSetPrecedentLimit
	}
	if in.PrecedentLimit < 1 || in.PrecedentLimit > 10 {
		return errors.New("precedent_limit must be between 1 and 10")
	}
	if in.ResponseFormat == "" {
		in.ResponseFormat = "concise"
	}
	if in.ResponseFormat != "concise" && in.ResponseFormat != "detailed" {
		return errors.New("response_format must be concise or detailed")
	}
	return nil
}

func unavailableIssueSet(in mcpcontract.PrepareIssueSetInput, out mcpcontract.PrepareIssueSetOutput) mcpcontract.PrepareIssueSetOutput {
	out.Status = "partial"
	for i, number := range in.IssueNumbers {
		out.Items[i] = mcpcontract.BatchItem[mcpcontract.PreparedIssueEvidence]{
			Key: fmt.Sprintf("%s/%s#%d", in.Owner, in.Repo, number), Status: "unavailable",
			Reason: "repository_not_indexed", Message: "repository is not present in the local corpus",
			NextAction: "Call github.sync_threads for this exact issue.",
		}
		out.SuggestedActions = append(out.SuggestedActions, issueSyncAction(domain.RepoRef{Owner: in.Owner, Repo: in.Repo}, number))
	}
	return out
}

func issueSetPrecedentInput(in mcpcontract.PrepareIssueSetInput) mcpcontract.FindPrecedentsInput {
	threads := make([]mcpcontract.ThreadRef, len(in.IssueNumbers))
	for i, number := range in.IssueNumbers {
		threads[i] = mcpcontract.ThreadRef{Owner: in.Owner, Repo: in.Repo, Kind: corpus.ThreadKindIssue, Number: number}
	}
	return mcpcontract.FindPrecedentsInput{Threads: threads, Limit: 100}
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
	responseFormat string,
	precedentLimit int,
	relationshipPopulationComplete bool,
	evaluatedAt time.Time,
) (mcpcontract.PreparedIssueEvidence, []mcpcontract.SuggestedAction, bool, error) {
	value := mcpcontract.PreparedIssueEvidence{
		Number: issue.Number, Title: issue.Title, State: issue.State, StateReason: issue.StateReason,
		Labels: append([]string(nil), issue.Labels...), BodyStatus: "unknown",
		SourceUpdatedAt: formatTime(issue.SourceUpdatedAt), Coverage: []mcpcontract.FacetCoverageOutput{},
		RelatedWork: []mcpcontract.IssueSetRelatedWork{}, AcceptedExamples: []mcpcontract.PrecedentOutput{},
		Linkage: mcpcontract.IssueSetLinkageCandidate{
			IssueNumber: issue.Number, Relation: "related", AllowedRelations: []string{"closes", "advances", "related"},
			RequiresConfirmation: true, Basis: "The caller explicitly included this issue; the implementation relationship has not been validated.",
		},
	}
	actions := []mcpcontract.SuggestedAction{}
	partial := false
	relationshipEvidenceComplete := issue.Body != ""
	if issue.Body != "" {
		value.BodyStatus = "available"
		if responseFormat == "detailed" {
			value.Body = issue.Body
		}
	} else {
		action := issueSyncAction(ref, issue.Number)
		value.Gaps = append(value.Gaps, mcpcontract.IssueSetGap{
			Code: "body_unknown", Facet: "body", Status: "unknown",
			Message: "the corpus does not distinguish a known-empty body from a body that was not captured", NextAction: action,
		})
		actions, partial = append(actions, action), true
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
				Message: "no coverage observation is stored for this facet", NextAction: action,
			})
			actions, partial = append(actions, action), true
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
				Message: "stored facet coverage is incomplete", NextAction: action,
			})
			actions, partial = append(actions, action), true
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
	if responseFormat == "concise" && limit > conciseIssueSetRelatedLimit {
		limit = conciseIssueSetRelatedLimit
	}
	conciseEvidenceOmitted := false
	for _, work := range combined[:limit] {
		if responseFormat == "concise" && len(work.Evidence) > 0 {
			conciseEvidenceOmitted = true
		}
		value.RelatedWork = append(value.RelatedWork, issueSetRelatedWork(work, ref, pullRequests, responseFormat))
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
			Message: "historical precedent evidence is unavailable for this issue", NextAction: action,
		})
		actions, partial = append(actions, action), true
	} else {
		for _, match := range precedents.Value.Matches {
			if match.Kind == corpus.ThreadKindPullRequest && match.MergedAt != "" {
				value.AcceptedExamples = append(value.AcceptedExamples, match)
				if len(value.AcceptedExamples) == precedentLimit {
					break
				}
			}
		}
	}
	return value, actions, partial, nil
}

func issueSetRelatedWork(work radar.RelatedWork, ref domain.RepoRef, pullRequests map[int]corpus.Thread, responseFormat string) mcpcontract.IssueSetRelatedWork {
	out := mcpcontract.IssueSetRelatedWork{
		Ref: work.Ref, Kind: work.Kind, Number: work.Number, Title: work.Title, State: work.State,
		Relation: work.Relation, Direction: work.Direction, URL: work.URL,
		SourceUpdatedAt: formatTime(work.SourceUpdatedAt),
	}
	localPullRequestRef := fmt.Sprintf("pull_request:%s#%d", ref, work.Number)
	if pullRequest, ok := pullRequests[work.Number]; ok && work.Kind == corpus.ThreadKindPullRequest && work.Ref == localPullRequestRef {
		if pullRequest.MergedKnown {
			merged := pullRequest.Merged
			out.Merged = &merged
		}
		out.MergedAt = formatTime(pullRequest.MergedAt)
	}
	if responseFormat == "detailed" {
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

func issueSyncAction(ref domain.RepoRef, number int) mcpcontract.SuggestedAction {
	return mcpcontract.SuggestedAction{
		Tool: mcpcontract.ToolSyncThreads, Reason: "Fetch the exact issue header and body into the local corpus.",
		Arguments: &mcpcontract.SuggestedActionArguments{
			Selection: "threads",
			Threads:   []mcpcontract.ThreadRef{{Owner: ref.Owner, Repo: ref.Repo, Kind: corpus.ThreadKindIssue, Number: number}},
		},
	}
}

func issueHydrateAction(ref domain.RepoRef, number int, facet string) mcpcontract.SuggestedAction {
	return mcpcontract.SuggestedAction{
		Tool: mcpcontract.ToolHydrateThreads, Reason: "Complete the missing issue evidence facet.",
		Arguments: &mcpcontract.SuggestedActionArguments{
			Threads: []mcpcontract.ThreadRef{{Owner: ref.Owner, Repo: ref.Repo, Kind: corpus.ThreadKindIssue, Number: number}},
			Facets:  []string{facet},
		},
	}
}

func repositoryPullRequestSyncAction(ref domain.RepoRef) mcpcontract.SuggestedAction {
	return mcpcontract.SuggestedAction{
		Tool: mcpcontract.ToolSyncThreads, Reason: "Complete the stored pull-request population used for related-work analysis.",
		Arguments: &mcpcontract.SuggestedActionArguments{
			Selection:    "repositories",
			Repositories: []mcpcontract.RepositoryRef{{Owner: ref.Owner, Repo: ref.Repo}},
			Kind:         corpus.ThreadKindPullRequest,
			State:        "all",
		},
	}
}

func repositoryHistorySyncAction(ref domain.RepoRef) mcpcontract.SuggestedAction {
	return mcpcontract.SuggestedAction{
		Tool: mcpcontract.ToolSyncThreads, Reason: "Fetch closed issue and pull-request headers used for historical precedent analysis.",
		Arguments: &mcpcontract.SuggestedActionArguments{
			Selection:    "repositories",
			Repositories: []mcpcontract.RepositoryRef{{Owner: ref.Owner, Repo: ref.Repo}},
			Kind:         "both",
			State:        "closed",
		},
	}
}
