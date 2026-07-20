package radar

import (
	"strings"
	"time"
)

// RelatedWorkEvidence identifies one stored source supporting a relationship.
type RelatedWorkEvidence struct {
	Kind       string    `json:"kind"`
	SourceURL  string    `json:"source_url"`
	SourceAsOf time.Time `json:"source_as_of,omitempty"`
}

// RelatedWork is one normalized issue, pull request, or local cluster related
// to a candidate. Relation is the strongest stored relationship; Evidence
// preserves every distinct source that contributed to that conclusion.
type RelatedWork struct {
	Ref             string                `json:"ref"`
	Kind            string                `json:"kind"`
	Number          int                   `json:"number,omitempty"`
	Title           string                `json:"title,omitempty"`
	State           string                `json:"state,omitempty"`
	Relation        string                `json:"relation"`
	Direction       string                `json:"direction,omitempty"`
	URL             string                `json:"url,omitempty"`
	Evidence        []RelatedWorkEvidence `json:"evidence"`
	SourceUpdatedAt time.Time             `json:"source_updated_at,omitempty"`
}

func cloneRelatedWork(values []RelatedWork) []RelatedWork {
	out := make([]RelatedWork, len(values))
	for i, value := range values {
		out[i] = value
		out[i].SourceUpdatedAt = value.SourceUpdatedAt.UTC()
		out[i].Evidence = append([]RelatedWorkEvidence{}, value.Evidence...)
		for j := range out[i].Evidence {
			out[i].Evidence[j].SourceAsOf = out[i].Evidence[j].SourceAsOf.UTC()
		}
	}
	return out
}

func relatedWorkCollisions(values []RelatedWork) (closing, inbound, dependencies, outboundPRs []RelatedWork) {
	for _, work := range values {
		if !strings.EqualFold(work.State, "open") {
			continue
		}
		if work.Relation == "depends_on" && work.Direction == "outbound" {
			dependencies = append(dependencies, work)
			continue
		}
		if work.Kind != "pull_request" {
			continue
		}
		switch {
		case work.Direction == "inbound" && work.Relation == "claims_to_close":
			closing = append(closing, work)
		case work.Direction == "inbound":
			inbound = append(inbound, work)
		case work.Direction == "outbound":
			outboundPRs = append(outboundPRs, work)
		}
	}
	return closing, inbound, dependencies, outboundPRs
}
