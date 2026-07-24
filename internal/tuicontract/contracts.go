// Package tuicontract defines the product-owned offline browse boundary.
package tuicontract

import "context"

// Reader is a narrow local data source. Implementations must not perform
// network I/O.
type Reader interface {
	Load(ctx context.Context) (Data, error)
}

// Data is the offline dataset loaded by the reader.
type Data struct {
	Repositories   []Item            `json:"repositories"`
	Threads        []Item            `json:"threads"`
	Clusters       []Item            `json:"clusters"`
	Investigations []Item            `json:"investigations"`
	Opportunities  []Item            `json:"opportunities"`
	Windows        map[string]Window `json:"windows,omitempty"`
}

// Window distinguishes the loaded browse window from its source population.
type Window struct {
	Total     int  `json:"total"`
	Truncated bool `json:"truncated"`
}

// Item is one browsable record.
type Item struct {
	Kind     string  `json:"kind"`
	ID       string  `json:"id"`
	Ref      string  `json:"ref"`
	Title    string  `json:"title"`
	Subtitle string  `json:"subtitle,omitempty"`
	Detail   string  `json:"detail,omitempty"`
	Source   string  `json:"source,omitempty"`
	AsOf     string  `json:"as_of,omitempty"`
	Coverage []Facet `json:"coverage,omitempty"`
}

// Facet describes coverage for one data facet.
type Facet struct {
	Name     string `json:"name"`
	Present  bool   `json:"present"`
	Complete bool   `json:"complete"`
	AsOf     string `json:"as_of,omitempty"`
}
