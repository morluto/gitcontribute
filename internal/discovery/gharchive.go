package discovery

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

// ErrAlreadyImported is returned by ArchiveReader when a GH Archive hour has
// already been recorded as imported.
var ErrAlreadyImported = errors.New("hour already imported")

const defaultMaxEventBytes = 4 << 20

// ErrDecompressedTooLarge is returned when an hour's decompressed payload
// exceeds ArchiveReader.MaxTotalBytes.
var ErrDecompressedTooLarge = errors.New("decompressed archive exceeds size limit")

// ArchiveHourRange returns the inclusive hourly bounds for a --since crawl.
// The latest complete hour is the hour before the current hour, because the
// current hour's file may not yet be published.
func ArchiveHourRange(since time.Duration, now time.Time) (start, end time.Time) {
	now = now.UTC()
	end = now.Truncate(time.Hour).Add(-time.Hour)
	start = now.Add(-since).Truncate(time.Hour)
	if start.After(end) {
		start = end
	}
	return start, end
}

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

// ParseEventType converts a raw GH Archive discriminator into one supported
// event kind.
func ParseEventType(value string) (EventType, error) {
	switch EventType(value) {
	case PushEvent:
		return PushEvent, nil
	case IssuesEvent:
		return IssuesEvent, nil
	case PullRequestEvent:
		return PullRequestEvent, nil
	case IssueCommentEvent:
		return IssueCommentEvent, nil
	case PullRequestReviewEvent:
		return PullRequestReviewEvent, nil
	case PullRequestReviewCommentEvent:
		return PullRequestReviewCommentEvent, nil
	case ReleaseEvent:
		return ReleaseEvent, nil
	case WatchEvent:
		return WatchEvent, nil
	case ForkEvent:
		return ForkEvent, nil
	case DiscussionEvent:
		return DiscussionEvent, nil
	case DiscussionCommentEvent:
		return DiscussionCommentEvent, nil
	default:
		return "", fmt.Errorf("unsupported GH Archive event type %q", value)
	}
}

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
	include       map[EventType]struct{}
	Store         CheckpointStore
	MaxEventBytes int
	// MaxTotalBytes bounds the total decompressed bytes for an hour. Zero
	// disables the limit.
	MaxTotalBytes int
}

// NewArchiveReader creates a reader that retains the given event types. An
// empty include list retains all events.
func NewArchiveReader(include []string, store CheckpointStore) (*ArchiveReader, error) {
	parsed := make(map[EventType]struct{}, len(include))
	for _, raw := range include {
		eventType, err := ParseEventType(raw)
		if err != nil {
			return nil, err
		}
		parsed[eventType] = struct{}{}
	}
	return &ArchiveReader{include: parsed, Store: store, MaxEventBytes: defaultMaxEventBytes}, nil
}

// Read decompresses the hourly gzip stream, parses JSON lines, and emits a
// Signal for each retained event. It checks the checkpoint store for hour
// idempotency and marks the hour imported on successful completion. Malformed
// lines are skipped. If ctx is cancelled, Read returns the cancellation error.
func (r *ArchiveReader) Read(ctx context.Context, hour time.Time, in io.Reader, emit func(Signal) error) error {
	if in == nil {
		return errors.New("archive input is required")
	}
	if emit == nil {
		return errors.New("archive emitter is required")
	}
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
	stream := io.Reader(gzr)
	closeFn := func() error { return gzr.Close() }
	if r.MaxTotalBytes > 0 {
		lr := NewLimitedReader(gzr, int64(r.MaxTotalBytes), gzr.Close, ErrDecompressedTooLarge)
		stream = lr
		closeFn = lr.Close
	}
	defer func() { _ = closeFn() }()

	maxEventBytes := r.MaxEventBytes
	if maxEventBytes <= 0 {
		maxEventBytes = defaultMaxEventBytes
	}
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, min(64<<10, maxEventBytes)), maxEventBytes)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var ev rawEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		eventType, err := ParseEventType(ev.Type)
		if err != nil || !r.shouldInclude(eventType) {
			continue
		}

		sig, ok := normalizeEvent(ev, hour, eventType)
		if !ok {
			continue
		}

		if err := emit(sig); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read archive event: %w", err)
	}

	if r.Store != nil {
		if err := r.Store.MarkImported(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

func (r *ArchiveReader) shouldInclude(eventType EventType) bool {
	if len(r.include) == 0 {
		return true
	}
	_, ok := r.include[eventType]
	return ok
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

type discussionPayload struct {
	Action string `json:"action"`
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

func normalizeEvent(ev rawEvent, hour time.Time, eventType EventType) (Signal, bool) {
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
		EventType:  eventType,
		Repo:       ref,
		RepoID:     ev.Repo.ID,
		Actor:      ev.Actor.Login,
	}

	switch eventType {
	case PushEvent:
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

	case IssuesEvent:
		var p issuePayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return Signal{}, false
		}
		fillIssueSignal(&sig, p.Issue, domain.IssueKind)
		sig.Action = p.Action
		return sig, true

	case PullRequestEvent:
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

	case IssueCommentEvent:
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

	case PullRequestReviewEvent:
		var p reviewPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return Signal{}, false
		}
		fillIssueSignal(&sig, p.PullRequest, domain.PullRequestKind)
		sig.Action = p.Action
		return sig, true

	case PullRequestReviewCommentEvent:
		var p reviewCommentPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return Signal{}, false
		}
		fillIssueSignal(&sig, p.PullRequest, domain.PullRequestKind)
		sig.Action = p.Action
		return sig, true

	case WatchEvent:
		var p watchPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return Signal{}, false
		}
		sig.Action = p.Action
		return sig, true

	case ForkEvent:
		var p forkPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return Signal{}, false
		}
		sig.Action = "forked"
		_ = p.Forkee.ID
		return sig, true

	case ReleaseEvent:
		var p releasePayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return Signal{}, false
		}
		sig.Action = p.Action
		sig.TagName = p.Release.TagName
		return sig, true

	case DiscussionEvent, DiscussionCommentEvent:
		var p discussionPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return Signal{}, false
		}
		sig.Action = p.Action
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
	if state, err := domain.ParseThreadState(issue.State); err == nil {
		sig.ThreadState = state
	}
	if kind == domain.PullRequestKind {
		sig.Merged = issue.Merged
	}
}

func parseRepoRef(name string) (domain.RepoRef, bool) {
	ref, err := domain.ParseRepoRef(name)
	if err != nil {
		return domain.RepoRef{}, false
	}
	return ref, true
}
