package clustering

import (
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

// MemberRef identifies a thread across repositories and kinds.
type MemberRef struct {
	Owner  string
	Repo   string
	Kind   string
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
	Kind      string
	Number    int
	State     string
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
		Owner:  c.Repo.Owner,
		Repo:   c.Repo.Repo,
		Kind:   c.Kind,
		Number: c.Number,
	}
}

// Member is one thread inside a cluster.
type Member struct {
	ThreadID int64
	Ref      MemberRef
	Title    string
	State    string
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

// MemberRefs returns the included member refs in deterministic order.
func (c Cluster) MemberRefs() []MemberRef {
	refs := make([]MemberRef, 0, len(c.Members))
	for _, m := range c.Members {
		if m.Included {
			refs = append(refs, m.Ref)
		}
	}
	sortMemberRefs(refs)
	return refs
}

// OverrideAction is a local governance instruction for a cluster member.
type OverrideAction string

const (
	OverrideInclude      OverrideAction = "include"
	OverrideExclude      OverrideAction = "exclude"
	OverrideSetCanonical OverrideAction = "set_canonical"
)

// MembershipOverride records an explicit local include/exclude/canonical decision.
type MembershipOverride struct {
	ID        int64
	ClusterID int64
	Ref       MemberRef
	Action    OverrideAction
	Reason    string
	CreatedAt time.Time
}

// ClusterRun records one clustering computation for a repository and source window.
type ClusterRun struct {
	ID             int64
	Repo           domain.RepoRef
	SourceRevision string
	WindowStart    time.Time
	WindowEnd      time.Time
	ParamsHash     string
	Status         string
	StartedAt      time.Time
	CompletedAt    *time.Time
	Stats          string
}

func sortMemberRefs(refs []MemberRef) {
	for i := 0; i < len(refs); i++ {
		for j := i + 1; j < len(refs); j++ {
			if refs[j].Less(refs[i]) {
				refs[i], refs[j] = refs[j], refs[i]
			}
		}
	}
}
