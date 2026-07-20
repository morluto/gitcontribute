package cli

// SyncResult reports the outcome of syncing a repository.
type SyncResult struct {
	Repo            RepoRef `json:"repo"`
	Updated         int     `json:"updated"`
	Requests        int     `json:"requests"`
	PlannedRequests int     `json:"planned_requests"`
	RequestBudget   int     `json:"request_budget"`
	Capped          bool    `json:"request_capped"`
	Message         string  `json:"message"`
}

// SyncPlanResult is the conservative request ceiling computed before a sync
// obtains a GitHub reader or writes the corpus.
type SyncPlanResult struct {
	Repo                 RepoRef `json:"repo"`
	FixedRequests        int     `json:"fixed_requests"`
	ThreadRequestCeiling int     `json:"thread_request_ceiling"`
	PlannedRequests      int     `json:"planned_requests"`
	RequestBudget        int     `json:"request_budget"`
	MaxPages             int     `json:"max_pages"`
	ExactThreads         int     `json:"exact_threads"`
}
