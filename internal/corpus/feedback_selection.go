package corpus

import (
	"errors"
	"fmt"
	"strings"
)

// FeedbackChannel identifies one product-owned pull-request feedback facet.
type FeedbackChannel uint8

const (
	FeedbackIssueComments FeedbackChannel = iota + 1
	FeedbackSubmittedReviews
	FeedbackInlineComments
	FeedbackReviewThreads
)

// ParseFeedbackChannel parses one canonical feedback channel.
func ParseFeedbackChannel(value string) (FeedbackChannel, error) {
	switch strings.TrimSpace(value) {
	case "issue_comments":
		return FeedbackIssueComments, nil
	case "submitted_reviews":
		return FeedbackSubmittedReviews, nil
	case "inline_comments":
		return FeedbackInlineComments, nil
	case "review_threads":
		return FeedbackReviewThreads, nil
	default:
		return 0, fmt.Errorf("unsupported feedback channel %q", value)
	}
}

// String returns the provider and storage spelling.
func (c FeedbackChannel) String() string {
	switch c {
	case FeedbackIssueComments:
		return "issue_comments"
	case FeedbackSubmittedReviews:
		return "submitted_reviews"
	case FeedbackInlineComments:
		return "inline_comments"
	case FeedbackReviewThreads:
		return "review_threads"
	default:
		return ""
	}
}

// Facet returns the corpus facet that stores this feedback channel.
func (c FeedbackChannel) Facet() string { return feedbackFacet(c) }

// FeedbackThreadSelection is the review-thread population fetched from
// GitHub. The zero value is invalid so persisted discovery always records the
// scope that established its coverage.
type FeedbackThreadSelection uint8

const (
	AllFeedbackThreads FeedbackThreadSelection = iota + 1
	UnresolvedFeedbackThreads
)

// ParseFeedbackThreadSelection parses one explicit acquisition scope.
func ParseFeedbackThreadSelection(value string) (FeedbackThreadSelection, error) {
	switch strings.TrimSpace(value) {
	case "all":
		return AllFeedbackThreads, nil
	case "unresolved":
		return UnresolvedFeedbackThreads, nil
	default:
		return 0, errors.New("thread_state must be unresolved or all")
	}
}

// String returns the provider and storage spelling.
func (s FeedbackThreadSelection) String() string {
	switch s {
	case AllFeedbackThreads:
		return "all"
	case UnresolvedFeedbackThreads:
		return "unresolved"
	default:
		return ""
	}
}

// FeedbackSelection binds a non-empty unique channel set to the review-thread
// acquisition scope used to establish coverage.
type FeedbackSelection struct {
	channels    uint8
	threadState FeedbackThreadSelection
}

// ParseFeedbackSelection parses and canonicalizes a feedback acquisition
// selection. Channel order is not semantically meaningful.
func ParseFeedbackSelection(channels []string, threadState string) (FeedbackSelection, error) {
	if len(channels) < 1 || len(channels) > 4 {
		return FeedbackSelection{}, errors.New("channels must contain 1 to 4 items")
	}
	var bits uint8
	for _, value := range channels {
		channel, err := ParseFeedbackChannel(value)
		if err != nil {
			return FeedbackSelection{}, err
		}
		bit := uint8(1 << (channel - 1))
		if bits&bit != 0 {
			return FeedbackSelection{}, fmt.Errorf("duplicate feedback channel %q", value)
		}
		bits |= bit
	}
	selection, err := ParseFeedbackThreadSelection(threadState)
	if err != nil {
		return FeedbackSelection{}, err
	}
	return FeedbackSelection{channels: bits, threadState: selection}, nil
}

// AllFeedbackSelection returns complete-channel, all-thread acquisition.
func AllFeedbackSelection() FeedbackSelection {
	return FeedbackSelection{channels: 0b1111, threadState: AllFeedbackThreads}
}

// Valid reports whether the selection can establish a coverage fact.
func (s FeedbackSelection) Valid() bool {
	return s.channels != 0 && s.channels&^uint8(0b1111) == 0 && s.threadState.String() != ""
}

// Channels returns the selected channels in stable canonical order.
func (s FeedbackSelection) Channels() []string {
	values := s.ChannelValues()
	out := make([]string, 0, len(values))
	for _, channel := range values {
		out = append(out, channel.String())
	}
	return out
}

// ChannelValues returns the selected product-owned channels in stable order.
func (s FeedbackSelection) ChannelValues() []FeedbackChannel {
	out := make([]FeedbackChannel, 0, 4)
	for _, channel := range []FeedbackChannel{FeedbackIssueComments, FeedbackSubmittedReviews, FeedbackInlineComments, FeedbackReviewThreads} {
		if s.Includes(channel) {
			out = append(out, channel)
		}
	}
	return out
}

// ThreadState returns the selected review-thread population.
func (s FeedbackSelection) ThreadState() string { return s.threadState.String() }

// Includes reports whether one channel belongs to the selection.
func (s FeedbackSelection) Includes(channel FeedbackChannel) bool {
	if channel < FeedbackIssueComments || channel > FeedbackReviewThreads {
		return false
	}
	return s.channels&uint8(1<<(channel-1)) != 0
}

// Equal reports semantic selection equality independent of input order.
func (s FeedbackSelection) Equal(other FeedbackSelection) bool { return s == other }
