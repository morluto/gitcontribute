package clustering

import (
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

// MemberRef identifies a thread across repositories and kinds.
type MemberRef struct {
	Owner  string
	Repo   string
	Kind   domain.ThreadKind
	Number int
}

func (m MemberRef) String() string {
	return fmt.Sprintf("%s/%s:%s#%d", m.Owner, m.Repo, m.Kind, m.Number)
}

// Less defines a deterministic total order for canonical selection.
func (m MemberRef) Less(other MemberRef) bool {
	if m.Kind != other.Kind {
		return m.Kind < other.Kind
	}
	if m.Owner != other.Owner {
		return m.Owner < other.Owner
	}
	if m.Repo != other.Repo {
		return m.Repo < other.Repo
	}
	return m.Number < other.Number
}

// Candidate is a thread considered for duplicate-candidate clustering.
type Candidate struct {
	ThreadID  int64
	Repo      domain.RepoRef
	Kind      domain.ThreadKind
	Number    int
	State     domain.ThreadState
	Title     string
	Body      string
	Author    string
	Labels    []string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Ref returns the member identity for the candidate.
func (c Candidate) Ref() MemberRef {
	return MemberRef{
		Owner:  c.Repo.Owner(),
		Repo:   c.Repo.Repo(),
		Kind:   c.Kind,
		Number: c.Number,
	}
}

// Member is one thread inside a cluster.
type Member struct {
	ThreadID int64
	Ref      MemberRef
	Title    string
	State    domain.ThreadState
	Score    float64
	Reason   string
	Included bool
}

// ClusterState is the local lifecycle of a cluster.
type ClusterState string

const (
	ClusterOpen   ClusterState = "open"
	ClusterClosed ClusterState = "closed"
	// ClusterRetired preserves governance history for a cluster that is no
	// longer present in the latest computation.
	ClusterRetired ClusterState = "retired"
)

// ParseClusterState converts durable text into a supported cluster lifecycle
// state.
func ParseClusterState(value string) (ClusterState, error) {
	switch ClusterState(strings.TrimSpace(value)) {
	case ClusterOpen:
		return ClusterOpen, nil
	case ClusterClosed:
		return ClusterClosed, nil
	case ClusterRetired:
		return ClusterRetired, nil
	default:
		return "", fmt.Errorf("unsupported cluster state %q", value)
	}
}

// Cluster is a group of duplicate-candidate threads.
type Cluster struct {
	ID          int64
	StableID    string
	State       ClusterState
	Repo        domain.RepoRef
	Canonical   MemberRef
	Revision    string
	WindowStart time.Time
	WindowEnd   time.Time
	Members     []Member
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// OverrideAction is a local governance instruction for a cluster member.
type OverrideAction string

const (
	OverrideInclude      OverrideAction = "include"
	OverrideExclude      OverrideAction = "exclude"
	OverrideSetCanonical OverrideAction = "set_canonical"
)

// ParseOverrideAction converts durable text into a supported local governance
// decision.
func ParseOverrideAction(value string) (OverrideAction, error) {
	switch OverrideAction(strings.TrimSpace(value)) {
	case OverrideInclude:
		return OverrideInclude, nil
	case OverrideExclude:
		return OverrideExclude, nil
	case OverrideSetCanonical:
		return OverrideSetCanonical, nil
	default:
		return "", fmt.Errorf("unsupported cluster override action %q", value)
	}
}

// MembershipOverride records an explicit local include/exclude/canonical decision.
type MembershipOverride struct {
	ID        int64
	ClusterID int64
	Ref       MemberRef
	Action    OverrideAction
	Reason    string
	CreatedAt time.Time
}
