package app

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// syncThreadsRequest is the parsed form of SyncThreadsInput. The wire input is
// intentionally a tagged field bag for JSON compatibility; this type prevents
// repository-only filters from leaking into exact-thread execution.
type syncThreadsRequest struct {
	selection   syncThreadsSelection
	maxRequests int
}

type syncThreadsSelection interface {
	isSyncThreadsSelection()
}

type repositoryThreadSelection struct {
	repositories       []domain.RepoRef
	kind               syncThreadKind
	state              syncThreadState
	updatedAfter       time.Time
	limitPerRepository int
}

func (repositoryThreadSelection) isSyncThreadsSelection() {}

type exactThreadSelection struct {
	threads []exactThreadTarget
}

func (exactThreadSelection) isSyncThreadsSelection() {}

type exactThreadTarget struct {
	repository domain.RepoRef
	kind       syncThreadKind
	number     int
}

func (t exactThreadTarget) wire() mcpcontract.ThreadRef {
	kind := ""
	if !t.kind.includesAll() {
		kind = t.kind.String()
	}
	return mcpcontract.ThreadRef{Owner: t.repository.Owner(), Repo: t.repository.Repo(), Kind: kind, Number: t.number}
}

func parseSyncThreadsInput(in mcpcontract.SyncThreadsInput) (syncThreadsRequest, mcpcontract.SyncThreadsInput, error) {
	if in.MaxRequests == 0 {
		in.MaxRequests = defaultSyncBatchMaxRequests
	}
	if in.MaxRequests < 1 || in.MaxRequests > defaultSyncBatchMaxRequests {
		return syncThreadsRequest{}, mcpcontract.SyncThreadsInput{}, fmt.Errorf("max requests must be between 1 and %d", defaultSyncBatchMaxRequests)
	}
	switch in.Selection {
	case "repositories":
		selection, normalized, err := parseRepositoryThreadSelection(in)
		if err != nil {
			return syncThreadsRequest{}, mcpcontract.SyncThreadsInput{}, err
		}
		return syncThreadsRequest{selection: selection, maxRequests: in.MaxRequests}, normalized, nil
	case "threads":
		selection, normalized, err := parseExactThreadSelection(in)
		if err != nil {
			return syncThreadsRequest{}, mcpcontract.SyncThreadsInput{}, err
		}
		return syncThreadsRequest{selection: selection, maxRequests: in.MaxRequests}, normalized, nil
	default:
		return syncThreadsRequest{}, mcpcontract.SyncThreadsInput{}, errors.New("selection must be repositories or threads")
	}
}

func parseRepositoryThreadSelection(in mcpcontract.SyncThreadsInput) (repositoryThreadSelection, mcpcontract.SyncThreadsInput, error) {
	if len(in.Threads) != 0 {
		return repositoryThreadSelection{}, mcpcontract.SyncThreadsInput{}, errors.New("threads is only valid in thread selection mode")
	}
	if len(in.Repositories) < 1 || len(in.Repositories) > 50 {
		return repositoryThreadSelection{}, mcpcontract.SyncThreadsInput{}, errors.New("repositories must contain 1 to 50 items")
	}
	in.Repositories = append([]mcpcontract.RepositoryRef(nil), in.Repositories...)
	repositories := make([]domain.RepoRef, len(in.Repositories))
	for i := range in.Repositories {
		ref, err := domain.NewRepoRef(in.Repositories[i].Owner, in.Repositories[i].Repo)
		if err != nil {
			return repositoryThreadSelection{}, mcpcontract.SyncThreadsInput{}, err
		}
		in.Repositories[i] = mcpcontract.RepositoryRef{Owner: ref.Owner(), Repo: ref.Repo()}
		repositories[i] = ref
	}
	if err := rejectDuplicateRepositoryRefs(in.Repositories); err != nil {
		return repositoryThreadSelection{}, mcpcontract.SyncThreadsInput{}, err
	}
	kind, err := parseSyncThreadKind(in.Kind)
	if err != nil {
		return repositoryThreadSelection{}, mcpcontract.SyncThreadsInput{}, err
	}
	in.Kind = kind.String()
	if in.State == "" {
		in.State = "open"
	}
	state, err := parseSyncThreadState(in.State)
	if err != nil {
		return repositoryThreadSelection{}, mcpcontract.SyncThreadsInput{}, err
	}
	in.State = state.String()
	var updatedAfter time.Time
	if in.UpdatedAfter != "" {
		parsed, err := time.Parse(time.RFC3339, in.UpdatedAfter)
		if err != nil {
			return repositoryThreadSelection{}, mcpcontract.SyncThreadsInput{}, errors.New("updated_after must be RFC 3339")
		}
		updatedAfter = parsed
	}
	if in.LimitPerRepository == 0 {
		in.LimitPerRepository = 100
	}
	if in.LimitPerRepository < 1 || in.LimitPerRepository > 1000 {
		return repositoryThreadSelection{}, mcpcontract.SyncThreadsInput{}, errors.New("limit_per_repository must be between 1 and 1000")
	}
	selection := repositoryThreadSelection{
		repositories:       repositories,
		kind:               kind,
		state:              state,
		updatedAfter:       updatedAfter,
		limitPerRepository: in.LimitPerRepository,
	}
	return selection, in, nil
}

func parseExactThreadSelection(in mcpcontract.SyncThreadsInput) (exactThreadSelection, mcpcontract.SyncThreadsInput, error) {
	if len(in.Repositories) != 0 {
		return exactThreadSelection{}, mcpcontract.SyncThreadsInput{}, errors.New("repositories is only valid in repository selection mode")
	}
	if in.Kind != "" || in.State != "" || in.UpdatedAfter != "" || in.LimitPerRepository != 0 {
		return exactThreadSelection{}, mcpcontract.SyncThreadsInput{}, errors.New("kind, state, updated_after, and limit_per_repository are only valid in repository selection mode")
	}
	if len(in.Threads) < 1 || len(in.Threads) > 100 {
		return exactThreadSelection{}, mcpcontract.SyncThreadsInput{}, errors.New("threads must contain 1 to 100 items")
	}
	in.Threads = append([]mcpcontract.ThreadRef(nil), in.Threads...)
	threads := make([]exactThreadTarget, len(in.Threads))
	for i, thread := range in.Threads {
		ref, err := domain.NewRepoRef(thread.Owner, thread.Repo)
		if err != nil {
			return exactThreadSelection{}, mcpcontract.SyncThreadsInput{}, err
		}
		if thread.Number <= 0 {
			return exactThreadSelection{}, mcpcontract.SyncThreadsInput{}, mcpcontract.InvalidArgument(fmt.Sprintf("threads[%d].number", i), "must be positive", nil)
		}
		kindValue := strings.TrimSpace(thread.Kind)
		if kindValue == "both" {
			return exactThreadSelection{}, mcpcontract.SyncThreadsInput{}, mcpcontract.InvalidArgument(fmt.Sprintf("threads[%d].kind", i), "must be issue or pull_request when provided", nil)
		}
		kind, err := parseSyncThreadKind(kindValue)
		if err != nil {
			return exactThreadSelection{}, mcpcontract.SyncThreadsInput{}, mcpcontract.InvalidArgument(fmt.Sprintf("threads[%d].kind", i), "must be issue or pull_request when provided", nil)
		}
		in.Threads[i].Owner, in.Threads[i].Repo = ref.Owner(), ref.Repo()
		in.Threads[i].Kind = kindValue
		threads[i] = exactThreadTarget{repository: ref, kind: kind, number: thread.Number}
	}
	if err := rejectDuplicateThreadRefs(in.Threads); err != nil {
		return exactThreadSelection{}, mcpcontract.SyncThreadsInput{}, err
	}
	return exactThreadSelection{threads: threads}, in, nil
}
