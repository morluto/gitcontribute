package app

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// threadSyncInput is the transient loose boundary shape for one repository's
// thread-header synchronization.
type threadSyncInput struct {
	Kind        string
	State       string
	Since       time.Time
	Numbers     []int
	MaxItems    int
	MaxPages    int
	MaxRequests int
}

// threadSyncRequest is the only executable thread-header synchronization
// representation. Its selection is either exact or listed, never both.
type threadSyncRequest struct {
	kind        syncThreadKind
	selection   threadSyncSelection
	maxRequests int
}

type threadSyncSelection interface {
	isThreadSyncSelection()
}

type exactThreadSync struct {
	numbers []int
}

func (exactThreadSync) isThreadSyncSelection() {}

type listedThreadSync struct {
	state    syncThreadState
	since    time.Time
	maxItems int
	maxPages int
}

func (listedThreadSync) isThreadSyncSelection() {}

type syncRequestPlan struct {
	threadRequestCeiling int
	plannedRequests      int
	requestBudget        int
	maxPages             int
	exactThreads         int
}

// parseThreadSync parses modes, bounds, and the exact-versus-listed selection
// before any corpus or network I/O begins.
func parseThreadSync(input threadSyncInput) (threadSyncRequest, syncRequestPlan, error) {
	kind, err := parseSyncThreadKind(input.Kind)
	if err != nil {
		return threadSyncRequest{}, syncRequestPlan{}, err
	}
	state, err := parseSyncThreadState(input.State)
	if err != nil {
		return threadSyncRequest{}, syncRequestPlan{}, err
	}
	return newThreadSyncRequest(kind, state, input.Since, input.Numbers, input.MaxItems, input.MaxPages, input.MaxRequests)
}

func newThreadSyncRequest(
	kind syncThreadKind,
	state syncThreadState,
	since time.Time,
	numbers []int,
	maxItems, maxPages, maxRequests int,
) (threadSyncRequest, syncRequestPlan, error) {
	if maxPages <= 0 {
		maxPages = 1000
	}
	if maxPages > 1000 {
		return threadSyncRequest{}, syncRequestPlan{}, errors.New("max pages cannot exceed 1000")
	}
	if maxItems < 0 || maxItems > 1000 {
		return threadSyncRequest{}, syncRequestPlan{}, errors.New("max items must be between 0 and 1000")
	}
	if maxRequests == 0 {
		maxRequests = defaultSyncMaxRequests
	}
	if maxRequests < 1 || maxRequests > maxSyncRequests {
		return threadSyncRequest{}, syncRequestPlan{}, fmt.Errorf("max requests must be between 1 and %d", maxSyncRequests)
	}
	if len(numbers) > 100 {
		return threadSyncRequest{}, syncRequestPlan{}, errors.New("exact thread selection cannot exceed 100 numbers")
	}

	canonicalNumbers := make([]int, 0, len(numbers))
	seen := make(map[int]struct{}, len(numbers))
	for _, number := range numbers {
		if number <= 0 {
			return threadSyncRequest{}, syncRequestPlan{}, errors.New("thread numbers must be positive")
		}
		if _, duplicate := seen[number]; duplicate {
			continue
		}
		seen[number] = struct{}{}
		canonicalNumbers = append(canonicalNumbers, number)
	}
	sort.Ints(canonicalNumbers)

	request := threadSyncRequest{kind: kind, maxRequests: maxRequests}
	plan := syncRequestPlan{requestBudget: maxRequests, maxPages: maxPages}
	if len(canonicalNumbers) > 0 {
		if !state.isAll() || !since.IsZero() {
			return threadSyncRequest{}, syncRequestPlan{}, errors.New("state and since filters cannot be combined with exact thread numbers")
		}
		if len(canonicalNumbers) > maxRequests {
			return threadSyncRequest{}, syncRequestPlan{}, fmt.Errorf(
				"exact thread selection requires at least %d requests; max requests is %d",
				len(canonicalNumbers), maxRequests,
			)
		}
		request.selection = exactThreadSync{numbers: canonicalNumbers}
		plan.threadRequestCeiling = len(canonicalNumbers)
		plan.plannedRequests = len(canonicalNumbers)
		plan.exactThreads = len(canonicalNumbers)
		return request, plan, nil
	}

	request.selection = listedThreadSync{state: state, since: since, maxItems: maxItems, maxPages: maxPages}
	plan.threadRequestCeiling = min(maxPages, maxRequests)
	plan.plannedRequests = plan.threadRequestCeiling
	return request, plan, nil
}

func (r threadSyncRequest) planCapped(plan syncRequestPlan) bool {
	listed, ok := r.selection.(listedThreadSync)
	return ok && plan.threadRequestCeiling < listed.maxPages
}

type syncThreadKind uint8

const (
	syncAllThreads syncThreadKind = iota
	syncIssues
	syncPullRequests
)

func parseSyncThreadKind(value string) (syncThreadKind, error) {
	switch strings.TrimSpace(value) {
	case "", "both":
		return syncAllThreads, nil
	case "issue":
		return syncIssues, nil
	case "pull_request":
		return syncPullRequests, nil
	default:
		return 0, errors.New("kind must be issue, pull_request, or both")
	}
}

func (k syncThreadKind) String() string {
	switch k {
	case syncIssues:
		return "issue"
	case syncPullRequests:
		return "pull_request"
	default:
		return "both"
	}
}

func (k syncThreadKind) includes(kind string) bool {
	return k == syncAllThreads || k.String() == kind
}

func (k syncThreadKind) includesAll() bool { return k == syncAllThreads }

type syncThreadState uint8

const (
	syncAllStates syncThreadState = iota
	syncOpenThreads
	syncClosedThreads
)

func parseSyncThreadState(value string) (syncThreadState, error) {
	switch strings.TrimSpace(value) {
	case "", "all":
		return syncAllStates, nil
	case "open":
		return syncOpenThreads, nil
	case "closed":
		return syncClosedThreads, nil
	default:
		return 0, errors.New("state must be open, closed, or all")
	}
}

func (s syncThreadState) String() string {
	switch s {
	case syncOpenThreads:
		return "open"
	case syncClosedThreads:
		return "closed"
	default:
		return "all"
	}
}

func (s syncThreadState) isAll() bool { return s == syncAllStates }
