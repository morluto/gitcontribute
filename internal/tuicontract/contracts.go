// Package tuicontract defines the product-owned offline workbench boundary.
package tuicontract

import "context"

// Reader is a narrow local data source. Implementations must not perform
// network I/O.
type Reader interface {
	Load(ctx context.Context) (Data, error)
}

// Capability describes the side-effect boundary crossed by an action.
type Capability string

const (
	CapabilityOfflineRead Capability = "offline_read"
	CapabilityLocalWrite  Capability = "local_write"
)

// Action is one contextual, currently executable application operation.
type Action struct {
	ID                   string     `json:"id"`
	Label                string     `json:"label"`
	Description          string     `json:"description,omitempty"`
	Capability           Capability `json:"capability"`
	RequiresConfirmation bool       `json:"requires_confirmation"`
}

// ActionRequest binds an action to the item from which it was offered.
type ActionRequest struct {
	ActionID string `json:"action_id"`
	Item     Item   `json:"item"`
}

// ActionResult is the concise outcome presented by the workbench.
type ActionResult struct {
	Title          string             `json:"title,omitempty"`
	Message        string             `json:"message"`
	Facts          []ActionResultFact `json:"facts,omitempty"`
	Items          []ActionResultItem `json:"items,omitempty"`
	SourceRevision string             `json:"source_revision,omitempty"`
	Target         *ActionTarget      `json:"target,omitempty"`
	Reload         bool               `json:"reload"`
}

// ActionResultFact is one labelled result value.
type ActionResultFact struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// ActionResultItem is one inspectable match or check returned by an action.
type ActionResultItem struct {
	Ref    string `json:"ref,omitempty"`
	Title  string `json:"title"`
	Status string `json:"status,omitempty"`
	Source string `json:"source,omitempty"`
}

// ActionTarget identifies a workflow item created or selected by an action.
type ActionTarget struct {
	Stage string `json:"stage"`
	ID    string `json:"id"`
}

// ActionProvider owns discovery and execution of contextual operations.
// Actions must truthfully declare their capability and must never perform
// GitHub mutation.
type ActionProvider interface {
	Actions(ctx context.Context, item Item) ([]Action, error)
	ExecuteAction(ctx context.Context, request ActionRequest) (ActionResult, error)
}

// BriefProvider builds a source-backed research brief from the local corpus.
// It must not perform network access, local writes, or process execution.
type BriefProvider interface {
	ResearchBrief(ctx context.Context, item Item) (ResearchBrief, error)
}

// ResearchBrief is the terminal-oriented projection of stored research facts.
type ResearchBrief struct {
	Ref                string      `json:"ref"`
	Title              string      `json:"title"`
	SourceAsOf         string      `json:"source_as_of,omitempty"`
	Problem            string      `json:"problem,omitempty"`
	ExpectedBehavior   []BriefFact `json:"expected_behavior,omitempty"`
	Discussion         []BriefFact `json:"discussion,omitempty"`
	ReproductionStatus BriefFact   `json:"reproduction_status"`
	RelatedWork        []BriefFact `json:"related_work,omitempty"`
	MissingEvidence    []string    `json:"missing_evidence,omitempty"`
	SuggestedNext      []string    `json:"suggested_next,omitempty"`
}

// BriefFact is one concise statement with optional source provenance.
type BriefFact struct {
	Summary string `json:"summary"`
	Source  string `json:"source,omitempty"`
}

// Data is the offline dataset loaded by the reader.
type Data struct {
	Candidates     []Item            `json:"candidates,omitempty"`
	Hypotheses     []Item            `json:"hypotheses,omitempty"`
	SyncStatuses   []Item            `json:"sync_statuses,omitempty"`
	Contributions  []Item            `json:"contributions,omitempty"`
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
	Kind       string      `json:"kind"`
	ID         string      `json:"id"`
	Ref        string      `json:"ref"`
	Title      string      `json:"title"`
	Subtitle   string      `json:"subtitle,omitempty"`
	Detail     string      `json:"detail,omitempty"`
	Status     string      `json:"status,omitempty"`
	Stage      string      `json:"stage,omitempty"`
	Score      int         `json:"score,omitempty"`
	Confidence string      `json:"confidence,omitempty"`
	Source     string      `json:"source,omitempty"`
	AsOf       string      `json:"as_of,omitempty"`
	Coverage   []Facet     `json:"coverage,omitempty"`
	Assessment *Assessment `json:"assessment,omitempty"`
	Actions    []Action    `json:"actions,omitempty"`
	Commands   []string    `json:"commands,omitempty"`
}

// Assessment carries concise, source-backed reasons for a workbench item.
type Assessment struct {
	Positive []Fact `json:"positive,omitempty"`
	Risks    []Fact `json:"risks,omitempty"`
	Blockers []Fact `json:"blockers,omitempty"`
	Unknowns []Fact `json:"unknowns,omitempty"`
	Related  []Fact `json:"related,omitempty"`
}

// Fact is one explainable assessment statement.
type Fact struct {
	Code    string `json:"code,omitempty"`
	Summary string `json:"summary"`
	Source  string `json:"source,omitempty"`
}

// Facet describes coverage for one data facet.
type Facet struct {
	Name     string `json:"name"`
	Present  bool   `json:"present"`
	Complete bool   `json:"complete"`
	AsOf     string `json:"as_of,omitempty"`
}
