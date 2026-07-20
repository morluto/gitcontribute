package cli

// SyncResult reports the outcome of syncing a repository.
type SyncResult struct {
	Repo     RepoRef `json:"repo"`
	Updated  int     `json:"updated"`
	Requests int     `json:"requests"`
	Capped   bool    `json:"request_capped"`
	Message  string  `json:"message"`
}
