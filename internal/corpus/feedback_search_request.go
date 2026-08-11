package corpus

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// FeedbackSearchInput is the transient loose representation accepted at an
// untrusted search boundary. ParseFeedbackSearchQuery is its only path into
// the query layer.
type FeedbackSearchInput struct {
	FeedbackAuthor    string
	PullRequestAuthor string
	State             string
	Merged            string
	ThreadState       string
	Channel           string
	Text              string
	CreatedAfter      string
	CreatedBefore     string
	UpdatedAfter      string
	UpdatedBefore     string
	Sort              string
	Order             string
	Limit             int
	Cursor            string
}

// FeedbackSearchQuery is a parsed repository-independent feedback query.
// Bind it to one stored repository before execution.
type FeedbackSearchQuery struct {
	feedbackAuthor    string
	pullRequestAuthor string
	state             ThreadStateFilter
	merge             MergeFilter
	threadState       feedbackThreadState
	channel           FeedbackChannel
	text              string
	createdAfter      time.Time
	createdBefore     time.Time
	updatedAfter      time.Time
	updatedBefore     time.Time
	sort              feedbackSort
	order             feedbackOrder
	page              SearchPage
}

// FeedbackSearchRequest is an executable query bound to exactly one stored
// repository. Its fields are private so repository-wide coverage cannot be
// accidentally combined with a cross-repository result set.
type FeedbackSearchRequest struct {
	repositoryID int64
	query        FeedbackSearchQuery
}

// ParseFeedbackSearchQuery parses all loose modes, dates, and paging once.
func ParseFeedbackSearchQuery(input FeedbackSearchInput) (FeedbackSearchQuery, error) {
	page, err := ParseSearchPage(input.Limit, input.Cursor)
	if err != nil {
		return FeedbackSearchQuery{}, fmt.Errorf("feedback search page: %w", err)
	}
	state, err := ParseThreadStateFilter(input.State)
	if err != nil {
		return FeedbackSearchQuery{}, fmt.Errorf("feedback search state: %w", err)
	}
	merge, err := ParseMergeFilter(input.Merged)
	if err != nil {
		return FeedbackSearchQuery{}, fmt.Errorf("feedback search merged: %w", err)
	}
	threadState, err := parseFeedbackThreadState(input.ThreadState)
	if err != nil {
		return FeedbackSearchQuery{}, err
	}
	channel, err := parseFeedbackChannelFilter(input.Channel)
	if err != nil {
		return FeedbackSearchQuery{}, err
	}
	sortMode, err := parseFeedbackSort(input.Sort)
	if err != nil {
		return FeedbackSearchQuery{}, err
	}
	order, err := parseFeedbackOrder(input.Order)
	if err != nil {
		return FeedbackSearchQuery{}, err
	}
	createdAfter, err := parseFeedbackSearchTime("created_after", input.CreatedAfter)
	if err != nil {
		return FeedbackSearchQuery{}, err
	}
	createdBefore, err := parseFeedbackSearchTime("created_before", input.CreatedBefore)
	if err != nil {
		return FeedbackSearchQuery{}, err
	}
	updatedAfter, err := parseFeedbackSearchTime("updated_after", input.UpdatedAfter)
	if err != nil {
		return FeedbackSearchQuery{}, err
	}
	updatedBefore, err := parseFeedbackSearchTime("updated_before", input.UpdatedBefore)
	if err != nil {
		return FeedbackSearchQuery{}, err
	}
	if !createdAfter.IsZero() && !createdBefore.IsZero() && createdAfter.After(createdBefore) {
		return FeedbackSearchQuery{}, errors.New("feedback search created_after must not be after created_before")
	}
	if !updatedAfter.IsZero() && !updatedBefore.IsZero() && updatedAfter.After(updatedBefore) {
		return FeedbackSearchQuery{}, errors.New("feedback search updated_after must not be after updated_before")
	}
	if state.IsOpen() && merge.IsMerged() {
		return FeedbackSearchQuery{}, errors.New("feedback search cannot select merged open pull requests")
	}
	if !threadState.isAny() && channel != 0 && channel != FeedbackReviewThreads {
		return FeedbackSearchQuery{}, errors.New("feedback search thread_state requires the review_threads channel")
	}
	return FeedbackSearchQuery{
		feedbackAuthor: strings.TrimSpace(input.FeedbackAuthor), pullRequestAuthor: strings.TrimSpace(input.PullRequestAuthor),
		state: state, merge: merge, threadState: threadState, channel: channel, text: strings.TrimSpace(input.Text),
		createdAfter: createdAfter, createdBefore: createdBefore, updatedAfter: updatedAfter, updatedBefore: updatedBefore,
		sort: sortMode, order: order, page: page,
	}, nil
}

// InRepository binds a parsed query to one stored repository row.
func (q FeedbackSearchQuery) InRepository(repositoryID int64) (FeedbackSearchRequest, error) {
	if repositoryID <= 0 {
		return FeedbackSearchRequest{}, fmt.Errorf("feedback search repository id must be positive, got %d", repositoryID)
	}
	return FeedbackSearchRequest{repositoryID: repositoryID, query: q}, nil
}

// Limit returns the normalized result bound for adapter provenance.
func (q FeedbackSearchQuery) Limit() int { return q.page.Limit() }

func parseFeedbackSearchTime(field, value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be RFC3339: %w", field, err)
	}
	return parsed, nil
}

type feedbackThreadState uint8

func parseFeedbackThreadState(value string) (feedbackThreadState, error) {
	switch strings.TrimSpace(value) {
	case "", "all":
		return 0, nil
	case "resolved":
		return 1, nil
	case "unresolved":
		return 2, nil
	default:
		return 0, errors.New("feedback search thread_state must be resolved, unresolved, or all")
	}
}

func (s feedbackThreadState) isAny() bool      { return s == 0 }
func (s feedbackThreadState) isResolved() bool { return s == 1 }

func (s feedbackThreadState) String() string {
	switch s {
	case 1:
		return "resolved"
	case 2:
		return "unresolved"
	default:
		return "all"
	}
}

func parseFeedbackChannelFilter(value string) (FeedbackChannel, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	return ParseFeedbackChannel(value)
}

type feedbackSort uint8

func parseFeedbackSort(value string) (feedbackSort, error) {
	switch strings.TrimSpace(value) {
	case "", "updated":
		return 0, nil
	case "feedback_author":
		return 1, nil
	case "pull_request_state":
		return 2, nil
	case "merge_state":
		return 3, nil
	case "created":
		return 4, nil
	case "pull_request_number":
		return 5, nil
	default:
		return 0, errors.New("feedback search sort is unsupported")
	}
}

func (s feedbackSort) String() string {
	switch s {
	case 1:
		return "feedback_author"
	case 2:
		return "pull_request_state"
	case 3:
		return "merge_state"
	case 4:
		return "created"
	case 5:
		return "pull_request_number"
	default:
		return "updated"
	}
}

func (s feedbackSort) expression() string {
	switch s {
	case 1:
		return "lower(p.author)"
	case 2:
		return "t.state"
	case 3:
		return "CASE WHEN t.merged_known = 0 THEN 0 WHEN t.merged = 0 THEN 1 ELSE 2 END"
	case 4:
		return "p.created_at"
	case 5:
		return "t.number"
	default:
		return "p.updated_at"
	}
}

type feedbackOrder bool

func parseFeedbackOrder(value string) (feedbackOrder, error) {
	switch strings.TrimSpace(value) {
	case "", "desc":
		return false, nil
	case "asc":
		return true, nil
	default:
		return false, errors.New("feedback search order must be asc or desc")
	}
}

func (o feedbackOrder) String() string {
	if o {
		return "asc"
	}
	return "desc"
}

func (o feedbackOrder) sqlDirection() string {
	if o {
		return "ASC"
	}
	return "DESC"
}
