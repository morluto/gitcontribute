package mcpcontract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// RecoveryPlan is the only model-visible recovery shape. Versioning keeps the
// contract explicit while Then preserves the order in which calls are made.
type RecoveryPlan struct {
	Version string     `json:"version"`
	Reason  string     `json:"reason"`
	Message string     `json:"message"`
	Then    []ToolCall `json:"then,omitempty"`
}

func (p *RecoveryPlan) UnmarshalJSON(data []byte) error {
	var wire struct {
		Version string            `json:"version"`
		Reason  string            `json:"reason"`
		Message string            `json:"message"`
		Then    []json.RawMessage `json:"then"`
	}
	if err := decodeStrictJSON(data, &wire); err != nil {
		return fmt.Errorf("decode recovery plan: %w", err)
	}
	if wire.Version != RecoveryPlanVersion {
		return fmt.Errorf("decode recovery plan: unsupported version %q", wire.Version)
	}
	var actions []ToolCall
	if wire.Then != nil {
		actions = make([]ToolCall, len(wire.Then))
	}
	for i, raw := range wire.Then {
		action, err := parseRecoveryAction(raw)
		if err != nil {
			return fmt.Errorf("decode recovery plan action %d: %w", i, err)
		}
		actions[i] = action
	}
	*p = RecoveryPlan{Version: wire.Version, Reason: wire.Reason, Message: wire.Message, Then: actions}
	return nil
}

// ToolCall is one discriminated, replayable MCP action in a recovery plan.
// The unexported method seals the variant set to this package. Callers can
// inspect the discriminator and concrete input, but cannot construct a non-nil
// empty, mismatched, or multi-payload action.
type ToolCall interface {
	json.Marshaler
	Type() string
	Input() any
	isToolCall()
}

// RecoveryActionPrototypes is the canonical recovery-action catalog used by
// schema generation and round-trip tests. Values carry zero inputs only; use
// RecoveryAction to construct an executable call.
func RecoveryActionPrototypes() []ToolCall {
	return []ToolCall{
		RecoveryAction(GetJobsInput{}),
		RecoveryAction(ResourceReadAction{}),
		RecoveryAction(SnapshotReadAction{}),
		RecoveryAction(GetThreadsInput{}),
		RecoveryAction(GetThreadFacetsInput{}),
		RecoveryAction(SearchPullRequestFeedbackInput{}),
		RecoveryAction(GetRepositoriesInput{}),
		RecoveryAction(EnsureCoverageInput{}),
		RecoveryAction(SyncRepositoryContextInput{}),
		RecoveryAction(SyncThreadsInput{}),
		RecoveryAction(HydrateThreadsInput{}),
		RecoveryAction(SyncPortfolioInput{}),
		RecoveryAction(SyncPullRequestFeedbackInput{}),
		RecoveryAction(IndexPullRequestFeedbackInput{}),
		RecoveryAction(SyncCIFailuresInput{}),
		RecoveryAction(DeepWikiInput{}),
		RecoveryAction(IndexRepositoriesInput{}),
		RecoveryAction(FindClustersInput{}),
		RecoveryAction(FindNeighborsInput{}),
		RecoveryAction(RankOpportunitiesInput{}),
		RecoveryAction(MineRepositoryFixPatternsInput{}),
		RecoveryAction(PreviewRepositoryFixPatternsInput{}),
		RecoveryAction(SearchGitHubRepositoriesInput{}),
		RecoveryAction(SearchGitHubThreadsInput{}),
		RecoveryAction(SearchCodeInput{}),
		RecoveryAction(ReadSourceFilesInput{}),
		RecoveryAction(InspectCommitChangesInput{}),
		RecoveryAction(CheckMergeConflictsInput{}),
		RecoveryAction(FindRelatedWorkInput{}),
		RecoveryAction(ListConcernsInput{}),
		RecoveryAction(ListPullRequestPortfolioInput{}),
		RecoveryAction(ExportManifestInput{}),
	}
}

type recoveryActionInput interface {
	GetJobsInput | GetRepositoriesInput | EnsureCoverageInput | SyncRepositoryContextInput | SyncThreadsInput | HydrateThreadsInput | SyncPortfolioInput | SyncPullRequestFeedbackInput | IndexPullRequestFeedbackInput | SyncCIFailuresInput | DeepWikiInput | IndexRepositoriesInput | FindClustersInput | FindNeighborsInput | RankOpportunitiesInput | MineRepositoryFixPatternsInput | PreviewRepositoryFixPatternsInput | SearchGitHubRepositoriesInput | SearchGitHubThreadsInput | SearchCodeInput | ReadSourceFilesInput | InspectCommitChangesInput | CheckMergeConflictsInput | FindRelatedWorkInput | ListConcernsInput | ListPullRequestPortfolioInput | ExportManifestInput | ResourceReadAction | SnapshotReadAction | GetThreadsInput | GetThreadFacetsInput | SearchPullRequestFeedbackInput
}

type followUpActionInput interface {
	GetJobsInput | ResourceReadAction | SnapshotReadAction | InspectCommitChangesInput | GetRepositoriesInput | GetThreadsInput | GetThreadFacetsInput | ListPullRequestPortfolioInput | SearchPullRequestFeedbackInput
}

type recoveryToolCall[T recoveryActionInput] struct {
	input T
}

func (recoveryToolCall[T]) isToolCall() {}

func (a recoveryToolCall[T]) Type() string {
	name, ok := recoveryActionName(any(a.input))
	if !ok {
		panic("unreachable recovery action input")
	}
	return name
}

func (a recoveryToolCall[T]) Input() any { return a.input }

func (a recoveryToolCall[T]) MarshalJSON() ([]byte, error) {
	name := a.Type()
	payload, err := json.Marshal(a.input)
	if err != nil {
		return nil, fmt.Errorf("marshal recovery action %s: %w", name, err)
	}
	typeJSON, err := json.Marshal(name)
	if err != nil {
		return nil, fmt.Errorf("marshal recovery action type: %w", err)
	}
	var out bytes.Buffer
	out.WriteString(`{"type":`)
	out.Write(typeJSON)
	out.WriteString(`,"`)
	out.WriteString(name)
	out.WriteString(`":`)
	out.Write(payload)
	out.WriteByte('}')
	return out.Bytes(), nil
}

// RecoveryAction derives the action discriminator from a concrete input type,
// making incompatible tool/argument combinations unrepresentable.
func RecoveryAction[T recoveryActionInput](input T) ToolCall {
	return recoveryToolCall[T]{input: input}
}

// FollowUpActionFor constructs a typed post-job action from its concrete
// input. The discriminator is derived from the input type.
func FollowUpActionFor[T followUpActionInput](input T) FollowUpAction {
	return FollowUpAction{call: recoveryToolCall[T]{input: input}}
}

// FollowUpActionPrototypes is the closed catalog of read and poll actions that
// may follow a durable job. Recovery plans have a broader catalog because they
// may also prescribe explicit write operations.
func FollowUpActionPrototypes() []ToolCall {
	return []ToolCall{
		FollowUpActionFor(GetJobsInput{}),
		FollowUpActionFor(ResourceReadAction{}),
		FollowUpActionFor(SnapshotReadAction{}),
		FollowUpActionFor(InspectCommitChangesInput{}),
		FollowUpActionFor(GetRepositoriesInput{}),
		FollowUpActionFor(GetThreadsInput{}),
		FollowUpActionFor(GetThreadFacetsInput{}),
		FollowUpActionFor(ListPullRequestPortfolioInput{}),
		FollowUpActionFor(SearchPullRequestFeedbackInput{}),
	}
}

func isFollowUpAction(input any) bool {
	switch input.(type) {
	case GetJobsInput, ResourceReadAction, SnapshotReadAction, InspectCommitChangesInput, GetRepositoriesInput, GetThreadsInput, GetThreadFacetsInput, ListPullRequestPortfolioInput, SearchPullRequestFeedbackInput:
		return true
	default:
		return false
	}
}

// RecoveryInput narrows an action back to its concrete input type for callers
// that need to replay or inspect it. The boolean is false for a different
// variant, so the discriminator and payload cannot disagree.
func RecoveryInput[T recoveryActionInput](action ToolCall) (T, bool) {
	var zero T
	if action == nil {
		return zero, false
	}
	input, ok := action.Input().(T)
	return input, ok
}

func recoveryActionName(input any) (string, bool) {
	switch input.(type) {
	case GetJobsInput:
		return "poll_job", true
	case ResourceReadAction:
		return "read_resource", true
	case SnapshotReadAction:
		return "read_snapshot", true
	case GetThreadsInput:
		return "get_threads", true
	case GetThreadFacetsInput:
		return "get_thread_facets", true
	case SearchPullRequestFeedbackInput:
		return "search_pull_request_feedback", true
	case GetRepositoriesInput:
		return "get_repositories", true
	case EnsureCoverageInput:
		return "ensure_coverage", true
	case SyncRepositoryContextInput:
		return "sync_repository_context", true
	case SyncThreadsInput:
		return "sync_threads", true
	case HydrateThreadsInput:
		return "hydrate_threads", true
	case SyncPortfolioInput:
		return "sync_portfolio", true
	case SyncPullRequestFeedbackInput:
		return "sync_pull_request_feedback", true
	case IndexPullRequestFeedbackInput:
		return "index_pull_request_feedback", true
	case SyncCIFailuresInput:
		return "sync_ci_failures", true
	case DeepWikiInput:
		return "query_deepwiki", true
	case IndexRepositoriesInput:
		return "index_repositories", true
	case FindClustersInput:
		return "find_clusters", true
	case FindNeighborsInput:
		return "find_neighbors", true
	case RankOpportunitiesInput:
		return "rank_opportunities", true
	case MineRepositoryFixPatternsInput:
		return "mine_repository_fix_patterns", true
	case PreviewRepositoryFixPatternsInput:
		return "preview_fix_patterns", true
	case SearchGitHubRepositoriesInput:
		return "search_github_repositories", true
	case SearchGitHubThreadsInput:
		return "search_github_threads", true
	case SearchCodeInput:
		return "search_code", true
	case ReadSourceFilesInput:
		return "read_source_files", true
	case InspectCommitChangesInput:
		return "inspect_commit_changes", true
	case CheckMergeConflictsInput:
		return "check_merge_conflicts", true
	case FindRelatedWorkInput:
		return "find_related_work", true
	case ListConcernsInput:
		return "list_concerns", true
	case ListPullRequestPortfolioInput:
		return "list_pull_request_portfolio", true
	case ExportManifestInput:
		return "export_manifest", true
	default:
		return "", false
	}
}

func parseRecoveryAction(data []byte) (ToolCall, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, err
	}
	if len(object) != 2 {
		return nil, fmt.Errorf("recovery action must contain exactly type and one typed input")
	}
	var actionType string
	if err := json.Unmarshal(object["type"], &actionType); err != nil || actionType == "" {
		return nil, fmt.Errorf("recovery action type is required")
	}
	input, ok := object[actionType]
	if !ok {
		return nil, fmt.Errorf("recovery action %q is missing its typed input", actionType)
	}
	switch actionType {
	case "poll_job":
		return decodeRecoveryAction[GetJobsInput](input)
	case "read_resource":
		return decodeRecoveryAction[ResourceReadAction](input)
	case "read_snapshot":
		return decodeRecoveryAction[SnapshotReadAction](input)
	case "get_threads":
		return decodeRecoveryAction[GetThreadsInput](input)
	case "get_thread_facets":
		return decodeRecoveryAction[GetThreadFacetsInput](input)
	case "search_pull_request_feedback":
		return decodeRecoveryAction[SearchPullRequestFeedbackInput](input)
	case "get_repositories":
		return decodeRecoveryAction[GetRepositoriesInput](input)
	case "ensure_coverage":
		return decodeRecoveryAction[EnsureCoverageInput](input)
	case "sync_repository_context":
		return decodeRecoveryAction[SyncRepositoryContextInput](input)
	case "sync_threads":
		return decodeRecoveryAction[SyncThreadsInput](input)
	case "hydrate_threads":
		return decodeRecoveryAction[HydrateThreadsInput](input)
	case "sync_portfolio":
		return decodeRecoveryAction[SyncPortfolioInput](input)
	case "sync_pull_request_feedback":
		return decodeRecoveryAction[SyncPullRequestFeedbackInput](input)
	case "index_pull_request_feedback":
		return decodeRecoveryAction[IndexPullRequestFeedbackInput](input)
	case "sync_ci_failures":
		return decodeRecoveryAction[SyncCIFailuresInput](input)
	case "query_deepwiki":
		return decodeRecoveryAction[DeepWikiInput](input)
	case "index_repositories":
		return decodeRecoveryAction[IndexRepositoriesInput](input)
	case "find_clusters":
		return decodeRecoveryAction[FindClustersInput](input)
	case "find_neighbors":
		return decodeRecoveryAction[FindNeighborsInput](input)
	case "rank_opportunities":
		return decodeRecoveryAction[RankOpportunitiesInput](input)
	case "mine_repository_fix_patterns":
		return decodeRecoveryAction[MineRepositoryFixPatternsInput](input)
	case "preview_fix_patterns":
		return decodeRecoveryAction[PreviewRepositoryFixPatternsInput](input)
	case "search_github_repositories":
		return decodeRecoveryAction[SearchGitHubRepositoriesInput](input)
	case "search_github_threads":
		return decodeRecoveryAction[SearchGitHubThreadsInput](input)
	case "search_code":
		return decodeRecoveryAction[SearchCodeInput](input)
	case "read_source_files":
		return decodeRecoveryAction[ReadSourceFilesInput](input)
	case "inspect_commit_changes":
		return decodeRecoveryAction[InspectCommitChangesInput](input)
	case "check_merge_conflicts":
		return decodeRecoveryAction[CheckMergeConflictsInput](input)
	case "find_related_work":
		return decodeRecoveryAction[FindRelatedWorkInput](input)
	case "list_concerns":
		return decodeRecoveryAction[ListConcernsInput](input)
	case "list_pull_request_portfolio":
		return decodeRecoveryAction[ListPullRequestPortfolioInput](input)
	case "export_manifest":
		return decodeRecoveryAction[ExportManifestInput](input)
	default:
		return nil, fmt.Errorf("unknown recovery action type %q", actionType)
	}
}

func decodeRecoveryAction[T recoveryActionInput](data []byte) (ToolCall, error) {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, errors.New("recovery action input must be an object")
	}
	var input T
	if err := decodeStrictJSON(data, &input); err != nil {
		return nil, err
	}
	return RecoveryAction(input), nil
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("input contains trailing JSON")
	}
	return nil
}

const RecoveryPlanVersion = "gitcontribute.recovery.v1"
