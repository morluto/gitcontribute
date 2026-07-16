package discovery

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

// ErrAlreadyImported is returned by ArchiveReader when a GH Archive hour has
// already been recorded as imported.
var ErrAlreadyImported = errors.New("hour already imported")

// EventType identifies a GH Archive event type.
type EventType string

// Recognized GH Archive event types.
const (
	PushEvent                     EventType = "PushEvent"
	IssuesEvent                   EventType = "IssuesEvent"
	PullRequestEvent              EventType = "PullRequestEvent"
	IssueCommentEvent             EventType = "IssueCommentEvent"
	PullRequestReviewEvent        EventType = "PullRequestReviewEvent"
	PullRequestReviewCommentEvent EventType = "PullRequestReviewCommentEvent"
	ReleaseEvent                  EventType = "ReleaseEvent"
	WatchEvent                    EventType = "WatchEvent"
	ForkEvent                     EventType = "ForkEvent"
	DiscussionEvent               EventType = "DiscussionEvent"
	DiscussionCommentEvent        EventType = "DiscussionCommentEvent"
)

// Signal is a normalized, product-owned discovery signal emitted from GH Archive
// events. Not all fields are populated for every event kind.
type Signal struct {
	Source       string
	Hour         time.Time
	ObservedAt   time.Time
	EventType    EventType
	Action       string
	Repo         domain.RepoRef
	RepoID       int64
	Actor        string
	ThreadKind   domain.ThreadKind
	ThreadNumber int
	ThreadTitle  string
	ThreadAuthor string
	ThreadState  domain.ThreadState
	Merged       bool
	Ref          string
	SHA          string
	Size         int
	TagName      string
}

// ArchiveReader streams an hourly GH Archive gzip file line by line, retains
// only configured event types, and emits normalized repository/thread signals.
type ArchiveReader struct {
	Include map[string]bool
	Store   CheckpointStore
}

// NewArchiveReader creates a reader that retains the given event types. An
// empty include list retains all events.
func NewArchiveReader(include []string, store CheckpointStore) *ArchiveReader {
	m := make(map[string]bool, len(include))
	for _, t := range include {
		m[t] = true
	}
	return &ArchiveReader{Include: m, Store: store}
}

// Read decompresses the hourly gzip stream, parses JSON lines, and emits a
// Signal for each retained event. It checks the checkpoint store for hour
// idempotency and marks the hour imported on successful completion. Malformed
// lines are skipped. If ctx is cancelled, Read returns the cancellation error.
func (r *ArchiveReader) Read(ctx context.Context, hour time.Time, in io.Reader, emit func(Signal) error) error {
	key := HourKey(hour)
	if r.Store != nil {
		imported, err := r.Store.IsImported(ctx, key)
		if err != nil {
			return err
		}
		if imported {
			return ErrAlreadyImported
		}
	}

	gzr, err := gzip.NewReader(in)
	if err != nil {
		return fmt.Errorf("open gzip: %w", err)
	}
	defer gzr.Close()

	br := bufio.NewReader(gzr)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		line, err := br.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(line) == 0 {
					break
				}
			} else {
				return err
			}
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		var ev rawEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if !r.shouldInclude(ev.Type) {
			continue
		}

		sig, ok := normalizeEvent(ev, hour)
		if !ok {
			continue
		}

		if err := emit(sig); err != nil {
			return err
		}
	}

	if r.Store != nil {
		if err := r.Store.MarkImported(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

func (r *ArchiveReader) shouldInclude(t string) bool {
	if len(r.Include) == 0 {
		return true
	}
	return r.Include[t]
}

// HourKey returns a stable, UTC hour identifier for checkpoint storage.
func HourKey(hour time.Time) string {
	return hour.UTC().Truncate(time.Hour).Format("2006010215")
}

type rawEvent struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Actor     rawActor        `json:"actor"`
	Repo      rawRepo         `json:"repo"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt string          `json:"created_at"`
}

type rawActor struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

type rawRepo struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type pushPayload struct {
	Ref     string `json:"ref"`
	Head    string `json:"head"`
	Before  string `json:"before"`
	Size    int    `json:"size"`
	Commits []struct {
		SHA string `json:"sha"`
	} `json:"commits"`
}

type issueObj struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	State       string    `json:"state"`
	Merged      bool      `json:"merged"`
	PullRequest *struct{} `json:"pull_request"`
	User        struct {
		Login string `json:"login"`
	} `json:"user"`
}

type issuePayload struct {
	Action string   `json:"action"`
	Issue  issueObj `json:"issue"`
}

type pullRequestPayload struct {
	Action      string   `json:"action"`
	Number      int      `json:"number"`
	PullRequest issueObj `json:"pull_request"`
}

type watchPayload struct {
	Action string `json:"action"`
}

type forkPayload struct {
	Forkee struct {
		FullName string `json:"full_name"`
		ID       int64  `json:"id"`
	} `json:"forkee"`
}

type releasePayload struct {
	Action  string `json:"action"`
	Release struct {
		TagName string `json:"tag_name"`
	} `json:"release"`
}

type commentPayload struct {
	Action  string   `json:"action"`
	Issue   issueObj `json:"issue"`
	Comment struct {
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	} `json:"comment"`
}

type reviewPayload struct {
	Action      string   `json:"action"`
	PullRequest issueObj `json:"pull_request"`
	Review      struct {
		User struct {
			Login string `json:"login"`
		} `json:"user"`
		State string `json:"state"`
	} `json:"review"`
}

type reviewCommentPayload struct {
	Action      string   `json:"action"`
	PullRequest issueObj `json:"pull_request"`
	Comment     struct {
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	} `json:"comment"`
}

func normalizeEvent(ev rawEvent, hour time.Time) (Signal, bool) {
	observed, err := time.Parse(time.RFC3339, ev.CreatedAt)
	if err != nil {
		return Signal{}, false
	}

	ref, ok := parseRepoRef(ev.Repo.Name)
	if !ok {
		return Signal{}, false
	}

	sig := Signal{
		Source:     "gharchive",
		Hour:       hour.UTC().Truncate(time.Hour),
		ObservedAt: observed,
		EventType:  EventType(ev.Type),
		Repo:       ref,
		RepoID:     ev.Repo.ID,
		Actor:      ev.Actor.Login,
	}

	switch ev.Type {
	case string(PushEvent):
		var p pushPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return Signal{}, false
		}
		sig.Action = "pushed"
		sig.Ref = p.Ref
		sig.SHA = p.Head
		sig.Size = p.Size
		if sig.Size == 0 {
			sig.Size = len(p.Commits)
		}
		return sig, true

	case string(IssuesEvent):
		var p issuePayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return Signal{}, false
		}
		fillIssueSignal(&sig, p.Issue, domain.IssueKind)
		sig.Action = p.Action
		return sig, true

	case string(PullRequestEvent):
		var p pullRequestPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return Signal{}, false
		}
		pr := p.PullRequest
		if pr.Number == 0 {
			pr.Number = p.Number
		}
		fillIssueSignal(&sig, pr, domain.PullRequestKind)
		sig.Action = p.Action
		return sig, true

	case string(IssueCommentEvent):
		var p commentPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return Signal{}, false
		}
		kind := domain.IssueKind
		if p.Issue.PullRequest != nil {
			kind = domain.PullRequestKind
		}
		fillIssueSignal(&sig, p.Issue, kind)
		sig.Action = p.Action
		return sig, true

	case string(PullRequestReviewEvent):
		var p reviewPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return Signal{}, false
		}
		fillIssueSignal(&sig, p.PullRequest, domain.PullRequestKind)
		sig.Action = p.Action
		return sig, true

	case string(PullRequestReviewCommentEvent):
		var p reviewCommentPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return Signal{}, false
		}
		fillIssueSignal(&sig, p.PullRequest, domain.PullRequestKind)
		sig.Action = p.Action
		return sig, true

	case string(WatchEvent):
		var p watchPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return Signal{}, false
		}
		sig.Action = p.Action
		return sig, true

	case string(ForkEvent):
		var p forkPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return Signal{}, false
		}
		sig.Action = "forked"
		_ = p.Forkee.ID
		return sig, true

	case string(ReleaseEvent):
		var p releasePayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return Signal{}, false
		}
		sig.Action = p.Action
		sig.TagName = p.Release.TagName
		return sig, true

	case string(DiscussionEvent), string(DiscussionCommentEvent):
		var m map[string]any
		if err := json.Unmarshal(ev.Payload, &m); err != nil {
			return Signal{}, false
		}
		if a, ok := m["action"].(string); ok {
			sig.Action = a
		}
		return sig, true

	default:
		return Signal{}, false
	}
}

func fillIssueSignal(sig *Signal, issue issueObj, kind domain.ThreadKind) {
	sig.ThreadKind = kind
	sig.ThreadNumber = issue.Number
	sig.ThreadTitle = issue.Title
	sig.ThreadAuthor = issue.User.Login
	sig.ThreadState = mapState(issue.State)
	if kind == domain.PullRequestKind {
		sig.Merged = issue.Merged
	}
}

func mapState(state string) domain.ThreadState {
	if state == "open" {
		return domain.OpenState
	}
	return domain.ClosedState
}

func parseRepoRef(name string) (domain.RepoRef, bool) {
	parts := strings.Split(name, "/")
	if len(parts) != 2 {
		return domain.RepoRef{}, false
	}
	ref := domain.RepoRef{Owner: parts[0], Repo: parts[1]}
	if err := ref.Validate(); err != nil {
		return domain.RepoRef{}, false
	}
	return ref, true
}
