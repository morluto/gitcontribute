package app

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func jobArtifactsAndFollowUp(job *contracts.JobResult, total int) ([]mcpcontract.JobArtifactReference, *mcpcontract.JobFollowUp) {
	switch job.Kind {
	case "mine_repository_fix_patterns":
		return fixPatternJobArtifact(job)
	case "build_repository_dossier":
		return dossierJobArtifact(job)
	case "create_workspace":
		return workspaceJobArtifact(job)
	case "run_validation":
		return validationJobArtifact(job, "validation_run")
	case "run_validation_group":
		return validationJobArtifact(job, "validation_group")
	case "sync_repository_context":
		return repositoryBatchJobArtifact(job, total)
	case "sync_threads":
		return threadBatchJobArtifact(job, total)
	case jobKindSyncThreadFacets:
		return threadFacetJobArtifact(job)
	case jobKindSyncPullRequestPortfolio:
		return portfolioJobArtifact(job)
	case "sync_pull_request_feedback", "sync_ci_failures":
		return pullRequestWorkflowJobArtifact(job)
	case jobKindIndexPullRequestFeedback:
		return pullRequestFeedbackIndexJobArtifact(job)
	case "index_repositories":
		return indexRepositoriesJobArtifact(job)
	case jobKindEnsureCoverage:
		return ensureCoverageJobArtifact(job)
	default:
		return nil, nil
	}
}

func ensureCoverageJobArtifact(job *contracts.JobResult) ([]mcpcontract.JobArtifactReference, *mcpcontract.JobFollowUp) {
	var result mcpcontract.EnsureCoverageJobResult
	if json.Unmarshal([]byte(job.Result), &result) != nil || result.SnapshotToken == "" || result.ArtifactDigest == "" {
		return nil, nil
	}
	uri := "gitcontribute://snapshot/" + result.SnapshotToken
	return []mcpcontract.JobArtifactReference{{Kind: "corpus_snapshot", ID: result.ArtifactDigest, URI: uri}}, resourceFollowUp(uri, "Read the immutable snapshot produced by the coverage workflow.")
}

func resourceFollowUp(uri, reason string) *mcpcontract.JobFollowUp {
	return &mcpcontract.JobFollowUp{Action: mcpcontract.FollowUpAction{Type: "read_resource", ReadResource: &mcpcontract.ResourceReadAction{URI: uri}}, Reason: reason}
}

func fixPatternJobArtifact(job *contracts.JobResult) ([]mcpcontract.JobArtifactReference, *mcpcontract.JobFollowUp) {
	uri := "gitcontribute://fix-pattern-report/" + job.ID
	return []mcpcontract.JobArtifactReference{{Kind: "fix_pattern_report", ID: job.ID, URI: uri}},
		resourceFollowUp(uri, "Read the persisted typed fix-pattern report.")
}

func dossierJobArtifact(job *contracts.JobResult) ([]mcpcontract.JobArtifactReference, *mcpcontract.JobFollowUp) {
	var request struct {
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
	}
	if json.Unmarshal([]byte(job.Request), &request) != nil || request.Owner == "" || request.Repo == "" {
		return nil, nil
	}
	uri := fmt.Sprintf("gitcontribute://dossier/%s/%s", request.Owner, request.Repo)
	return []mcpcontract.JobArtifactReference{{Kind: "dossier", ID: request.Owner + "/" + request.Repo, URI: uri}},
		resourceFollowUp(uri, "Read the persisted typed dossier resource.")
}

func workspaceJobArtifact(job *contracts.JobResult) ([]mcpcontract.JobArtifactReference, *mcpcontract.JobFollowUp) {
	var result struct {
		ID string `json:"id"`
	}
	if json.Unmarshal([]byte(job.Result), &result) != nil || result.ID == "" {
		return nil, nil
	}
	return []mcpcontract.JobArtifactReference{{Kind: "workspace", ID: result.ID}},
		&mcpcontract.JobFollowUp{
			Action: mcpcontract.FollowUpAction{Type: "inspect_commit_changes", InspectCommitChanges: &mcpcontract.InspectCommitChangesInput{WorkspaceID: result.ID}},
			Reason: "Inspect the managed workspace before planning commits.",
		}
}

func validationJobArtifact(job *contracts.JobResult, kind string) ([]mcpcontract.JobArtifactReference, *mcpcontract.JobFollowUp) {
	var result struct {
		ID string `json:"id"`
	}
	if json.Unmarshal([]byte(job.Result), &result) != nil || result.ID == "" {
		return nil, nil
	}
	return []mcpcontract.JobArtifactReference{{Kind: kind, ID: result.ID}}, nil
}

type syncBatchItem struct {
	Key        string                  `json:"key"`
	Status     string                  `json:"status"`
	Reason     string                  `json:"reason"`
	Message    string                  `json:"message"`
	RetryAfter int                     `json:"retry_after_ms"`
	Threads    []mcpcontract.ThreadRef `json:"threads"`
}

type syncBatchResult struct {
	Items []syncBatchItem `json:"items"`
}

func decodeSyncBatchResult(job *contracts.JobResult, total int) (syncBatchResult, int) {
	var result syncBatchResult
	count := total
	if json.Unmarshal([]byte(job.Result), &result) == nil && result.Items != nil {
		count = len(result.Items)
	}
	return result, count
}

func syncBatchReferences(result syncBatchResult, includeThreads bool) ([]string, []mcpcontract.ThreadRef, []mcpcontract.JobArtifactFailure) {
	references := make([]string, 0, min(len(result.Items), 100))
	threadRefs := make([]mcpcontract.ThreadRef, 0, min(len(result.Items), 100))
	failures := make([]mcpcontract.JobArtifactFailure, 0, min(len(result.Items), 100))
	for _, item := range result.Items {
		partialThreadBatch := includeThreads && item.Status == "partial"
		if item.Status != "complete" && !partialThreadBatch {
			if len(failures) < 100 {
				failures = append(failures, mcpcontract.JobArtifactFailure{
					Reference: item.Key, Status: mcpcontract.BatchItemStatus(item.Status), Reason: item.Reason,
					Message: item.Message, RetryAfterMS: mcpcontract.NonNegativeInt(item.RetryAfter),
				})
			}
			continue
		}
		if includeThreads && len(item.Threads) > 0 {
			for _, ref := range item.Threads {
				if len(threadRefs) >= 100 {
					break
				}
				threadRefs = append(threadRefs, ref)
				references = append(references, threadRefKey(ref))
			}
			continue
		}
		if item.Key != "" && len(references) < 100 {
			references = append(references, item.Key)
		}
	}
	return references, threadRefs, failures
}

func repositoryBatchJobArtifact(job *contracts.JobResult, total int) ([]mcpcontract.JobArtifactReference, *mcpcontract.JobFollowUp) {
	result, count := decodeSyncBatchResult(job, total)
	references, _, failures := syncBatchReferences(result, false)
	value := mcpcontract.NonNegativeInt(count)
	follow := &mcpcontract.JobFollowUp{
		Action: mcpcontract.FollowUpAction{Type: "get_repositories", GetRepositories: &mcpcontract.GetRepositoriesInput{}},
		Reason: "Read synchronized repository facts and coverage from the offline corpus.",
	}
	var request mcpcontract.SyncRepositoryContextInput
	if json.Unmarshal([]byte(job.Request), &request) == nil {
		follow.Action.GetRepositories.Repositories = append([]mcpcontract.RepositoryRef(nil), request.Repositories...)
	}
	return []mcpcontract.JobArtifactReference{{
		Kind: "repository_batch", Count: &value, References: references,
		ReferencesTruncated: len(result.Items) > len(references), Failures: failures,
	}}, follow
}

func threadBatchJobArtifact(job *contracts.JobResult, total int) ([]mcpcontract.JobArtifactReference, *mcpcontract.JobFollowUp) {
	result, count := decodeSyncBatchResult(job, total)
	references, threadRefs, failures := syncBatchReferences(result, true)
	value := mcpcontract.NonNegativeInt(count)
	var follow *mcpcontract.JobFollowUp
	if len(threadRefs) > 0 {
		follow = &mcpcontract.JobFollowUp{
			Action: mcpcontract.FollowUpAction{Type: "get_threads", GetThreads: &mcpcontract.GetThreadsInput{Threads: threadRefs}},
			Reason: "Read synchronized thread facts and coverage from the offline corpus.",
		}
	}
	return []mcpcontract.JobArtifactReference{{
		Kind: "thread_batch", Count: &value, References: references,
		ReferencesTruncated: len(result.Items) > len(references), Failures: failures,
	}}, follow
}

func threadFacetJobArtifact(job *contracts.JobResult) ([]mcpcontract.JobArtifactReference, *mcpcontract.JobFollowUp) {
	var request mcpcontract.HydrateThreadsInput
	_ = json.Unmarshal([]byte(job.Request), &request)
	return facetBatchArtifact(append([]mcpcontract.ThreadRef(nil), request.Threads...), request.Facets)
}

func portfolioJobArtifact(job *contracts.JobResult) ([]mcpcontract.JobArtifactReference, *mcpcontract.JobFollowUp) {
	var result struct {
		Status           string   `json:"status"`
		Login            string   `json:"login"`
		PullRequests     []string `json:"pull_requests"`
		Refreshed        int      `json:"refreshed"`
		DiscoveryStatus  string   `json:"discovery_status"`
		SearchIncomplete bool     `json:"search_incomplete"`
		RequestCapped    bool     `json:"request_capped"`
		Failures         []struct {
			Reference    string `json:"reference"`
			Status       string `json:"status"`
			Reason       string `json:"reason"`
			Message      string `json:"message"`
			RetryAfterMS int    `json:"retry_after_ms"`
		} `json:"failures"`
	}
	if json.Unmarshal([]byte(job.Result), &result) != nil {
		return nil, nil
	}
	value := mcpcontract.NonNegativeInt(result.Refreshed)
	failures := make([]mcpcontract.JobArtifactFailure, len(result.Failures))
	for i, failure := range result.Failures {
		failures[i] = mcpcontract.JobArtifactFailure{
			Reference: failure.Reference, Status: mcpcontract.BatchItemStatus(failure.Status), Reason: failure.Reason,
			Message: failure.Message, RetryAfterMS: mcpcontract.NonNegativeInt(failure.RetryAfterMS),
		}
	}
	var request mcpcontract.SyncPortfolioInput
	_ = json.Unmarshal([]byte(job.Request), &request)
	var recovery *mcpcontract.RecoveryPlan
	if result.Status != "complete" || result.SearchIncomplete || result.RequestCapped || result.DiscoveryStatus != "complete" {
		next := request
		// Older authored jobs predate the required discriminator. Infer that
		// legacy shape only when the stored result proves it was an authored
		// discovery. When that proof is absent, recover the exact references
		// already present in the result rather than selecting the current user.
		if next.Selection == "" && result.Login != "" {
			next.Selection = "authored"
		}
		if next.Selection == "" {
			next.PullRequests = portfolioResultRefs(result.PullRequests)
			if len(next.PullRequests) > 0 {
				next.Selection = "explicit"
			}
		}
		if next.Selection != "" && next.Selection == "authored" {
			next.DiscoveryMaxRequests = min(1000, max(next.DiscoveryMaxRequests*2, max(next.DiscoveryMaxRequests+1, 2)))
			next.Limit = min(100, max(next.Limit*2, max(next.Limit+1, 20)))
		}
		if next.Selection != "" {
			recovery = recoveryPlan("portfolio_discovery_incomplete", "Portfolio discovery was incomplete or bounded. Repeat synchronization with the returned larger discovery bound, then reread the portfolio.", mcpcontract.RecoveryAction(next))
		}
	}
	follow := &mcpcontract.JobFollowUp{
		Action: mcpcontract.FollowUpAction{Type: "list_pull_request_portfolio", ListPortfolio: portfolioReadFollowUpArguments(request, result.Login, result.PullRequests)},
		Reason: "Read these refreshed pull requests from the offline portfolio.",
	}
	return []mcpcontract.JobArtifactReference{{
		Kind: "pull_request_batch", Count: &value, References: append([]string(nil), result.PullRequests...), Failures: failures,
		Status: result.Status, DiscoveryStatus: result.DiscoveryStatus, SearchIncomplete: result.SearchIncomplete, RequestCapped: result.RequestCapped, Recovery: recovery,
	}}, follow
}

func portfolioResultRefs(values []string) []mcpcontract.ThreadRef {
	refs := make([]mcpcontract.ThreadRef, 0, len(values))
	for _, value := range values {
		marker := strings.LastIndex(value, "#")
		if marker <= 0 || marker == len(value)-1 {
			continue
		}
		number, err := strconv.Atoi(value[marker+1:])
		if err != nil || number <= 0 {
			continue
		}
		fullName := value[:marker]
		slash := strings.IndexByte(fullName, '/')
		if slash <= 0 || slash == len(fullName)-1 {
			continue
		}
		refs = append(refs, mcpcontract.ThreadRef{Owner: fullName[:slash], Repo: fullName[slash+1:], Kind: "pull_request", Number: number})
	}
	return refs
}

func pullRequestWorkflowJobArtifact(job *contracts.JobResult) ([]mcpcontract.JobArtifactReference, *mcpcontract.JobFollowUp) {
	var result pullRequestWorkflowResult
	if json.Unmarshal([]byte(job.Result), &result) != nil {
		return nil, nil
	}
	kind, reason, resourceKind := "pull_request_feedback", "Read the persisted feedback snapshots through their resource links.", "pull-request-feedback"
	if job.Kind == "sync_ci_failures" {
		kind, reason, resourceKind = "ci_failure_report", "Read the persisted CI reports and bounded job logs through their resource links.", "ci-failure-report"
	}
	artifact := mcpcontract.JobArtifactReference{Kind: kind}
	if len(result.Items) == 0 {
		var request struct {
			PullRequests []mcpcontract.ThreadRef `json:"pull_requests"`
		}
		if json.Unmarshal([]byte(job.Request), &request) == nil {
			for _, ref := range request.PullRequests {
				artifact.References = append(artifact.References, fmt.Sprintf(
					"gitcontribute://%s/%s/%s/%d", resourceKind, ref.Owner, ref.Repo, ref.Number,
				))
			}
		}
	}
	for _, item := range result.Items {
		if item.Status == "complete" {
			artifact.References = append(artifact.References, item.ResourceURI)
			continue
		}
		artifact.Failures = append(artifact.Failures, mcpcontract.JobArtifactFailure{
			Reference: item.Key, Status: item.Status, Reason: item.Code, Message: item.Message,
			RetryAfterMS: mcpcontract.NonNegativeInt(item.RetryAfterMS),
		})
	}
	count := mcpcontract.NonNegativeInt(len(artifact.References))
	artifact.Count = &count
	var follow *mcpcontract.JobFollowUp
	if len(artifact.References) > 0 {
		follow = resourceFollowUp(artifact.References[0], reason)
	}
	return []mcpcontract.JobArtifactReference{artifact}, follow
}

func pullRequestFeedbackIndexJobArtifact(job *contracts.JobResult) ([]mcpcontract.JobArtifactReference, *mcpcontract.JobFollowUp) {
	var result pullRequestFeedbackIndexResult
	if json.Unmarshal([]byte(job.Result), &result) != nil {
		return nil, nil
	}
	var request mcpcontract.IndexPullRequestFeedbackInput
	if json.Unmarshal([]byte(job.Request), &request) != nil {
		return nil, nil
	}
	refs := make([]string, 0, len(result.Items))
	failures := make([]mcpcontract.JobArtifactFailure, 0, len(result.Items))
	for _, item := range result.Items {
		if item.Status == "complete" {
			refs = append(refs, item.Key)
			continue
		}
		failures = append(failures, mcpcontract.JobArtifactFailure{Reference: item.Key, Status: item.Status, Reason: item.Code, Message: item.Message, RetryAfterMS: mcpcontract.NonNegativeInt(item.RetryAfterMS)})
	}
	artifact := mcpcontract.JobArtifactReference{Kind: "pull_request_feedback_index", Count: ptrNonNegative(len(refs)), References: refs, ReferencesTruncated: len(result.Items) > len(refs), Failures: failures, Status: result.Status, DiscoveryStatus: result.DiscoveryStatus, Recovery: result.Recovery}
	follow := &mcpcontract.JobFollowUp{Action: mcpcontract.FollowUpAction{Type: "search_pull_request_feedback", SearchFeedback: &mcpcontract.SearchPullRequestFeedbackInput{Repository: request.Repository}}, Reason: "Search the indexed pull-request feedback through the offline corpus."}
	return []mcpcontract.JobArtifactReference{artifact}, follow
}

func ptrNonNegative(value int) *mcpcontract.NonNegativeInt {
	out := mcpcontract.NonNegativeInt(value)
	return &out
}

type indexJobItem struct {
	Key            string             `json:"key"`
	Status         string             `json:"status"`
	Reason         string             `json:"reason"`
	Message        string             `json:"message"`
	RetryAfterMS   int                `json:"retry_after_ms"`
	CommitSHA      string             `json:"commit_sha"`
	IndexManifest  codeindex.Manifest `json:"index_manifest"`
	ArtifactDigest string             `json:"artifact_digest"`
	ManifestDigest string             `json:"manifest_digest"`
	SnapshotToken  string             `json:"snapshot_token"`
}

type indexJobResult struct {
	SnapshotToken string         `json:"snapshot_token"`
	Items         []indexJobItem `json:"items"`
}

func indexRepositoriesJobArtifact(job *contracts.JobResult) ([]mcpcontract.JobArtifactReference, *mcpcontract.JobFollowUp) {
	var result indexJobResult
	if json.Unmarshal([]byte(job.Result), &result) != nil {
		return nil, nil
	}
	artifacts := make([]mcpcontract.JobArtifactReference, 0, len(result.Items))
	completedRefs := make([]string, 0, min(len(result.Items), 100))
	failures := make([]mcpcontract.JobArtifactFailure, 0, min(len(result.Items), 100))
	for _, item := range result.Items {
		if item.Status != "complete" {
			if len(failures) < 100 {
				failures = append(failures, mcpcontract.JobArtifactFailure{
					Reference: item.Key, Status: mcpcontract.BatchItemStatus(item.Status), Reason: item.Reason,
					Message: item.Message, RetryAfterMS: mcpcontract.NonNegativeInt(item.RetryAfterMS),
				})
			}
			continue
		}
		if item.CommitSHA == "" {
			continue
		}
		owner, repo, ok := strings.Cut(item.Key, "/")
		if !ok || owner == "" || repo == "" {
			continue
		}
		if item.ArtifactDigest == "" {
			continue
		}
		artifact := mcpcontract.CodeIndexArtifact{Kind: "code_index", ID: "code-index:" + item.ArtifactDigest,
			Repository: mcpcontract.RepositoryRef{Owner: owner, Repo: repo}, CommitSHA: item.CommitSHA,
			SnapshotToken:  item.SnapshotToken,
			ManifestSHA256: item.ManifestDigest, ResourceURI: "gitcontribute://artifact/code-index/" + item.ArtifactDigest}
		artifact.FollowUp = resourceFollowUp(artifact.ResourceURI, "Read this exact digest-bound artifact through MCP resources/read.")
		if result.SnapshotToken != "" {
			artifact.SnapshotToken = result.SnapshotToken
		}
		artifacts = append(artifacts, mcpcontract.JobArtifactReference{Kind: artifact.Kind, ID: artifact.ID, URI: artifact.ResourceURI, CodeIndex: &artifact})
		if len(completedRefs) < 100 {
			completedRefs = append(completedRefs, item.Key)
		}
	}
	if len(failures) > 0 {
		count := mcpcontract.NonNegativeInt(len(completedRefs))
		artifacts = append(artifacts, mcpcontract.JobArtifactReference{
			Kind: "repository_batch", Count: &count, References: completedRefs,
			ReferencesTruncated: len(result.Items) > len(completedRefs), Failures: failures,
		})
	}
	return artifacts, firstCodeIndexFollowUp(artifacts)
}

func firstCodeIndexFollowUp(artifacts []mcpcontract.JobArtifactReference) *mcpcontract.JobFollowUp {
	for _, reference := range artifacts {
		if reference.CodeIndex == nil {
			continue
		}
		artifact := reference.CodeIndex
		return &mcpcontract.JobFollowUp{
			Action: mcpcontract.FollowUpAction{Type: "read_resource", ReadResource: &mcpcontract.ResourceReadAction{URI: artifact.ResourceURI}},
			Reason: "Read the exact indexed-commit artifact through MCP resources/read.",
		}
	}
	return nil
}
