package cli

// SearchMatch is one local search result.
type SearchMatch struct {
	Kind           string   `json:"kind"`
	Repo           RepoRef  `json:"repo"`
	Title          string   `json:"title"`
	Number         int      `json:"number,omitempty"`
	State          string   `json:"state,omitempty"`
	Author         string   `json:"author,omitempty"`
	Labels         []string `json:"labels,omitempty"`
	URL            string   `json:"url,omitempty"`
	Score          float64  `json:"score"`
	Body           string   `json:"-"`
	Freshness      string   `json:"freshness,omitempty"`
	Coverage       []string `json:"coverage,omitempty"`
	MatchSource    string   `json:"match_source,omitempty"`
	MatchExcerpt   string   `json:"match_excerpt,omitempty"`
	MatchTruncated bool     `json:"match_truncated,omitempty"`
}

// SearchResult is the result of a local corpus search.
type SearchResult struct {
	Query      string        `json:"query"`
	Kind       string        `json:"kind"`
	Repo       string        `json:"repo,omitempty"`
	Limit      int           `json:"limit"`
	Total      int           `json:"total"`
	Matches    []SearchMatch `json:"matches"`
	NextCursor string        `json:"next_cursor,omitempty"`
}
