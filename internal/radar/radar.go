// Package radar ranks locally stored contribution candidates with transparent,
// deterministic signals. It performs no I/O and owns no GitHub capability.
package radar

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

const (
	// ScoreVersion changes whenever ranking semantics change.
	ScoreVersion = "radar.v3"
	// DefaultLimit is the default number of returned candidates.
	DefaultLimit = 20
	// MaxLimit bounds one radar response.
	MaxLimit  = 100
	baseScore = 40
)

// Eligibility separates objective state from a candidate's numeric score.
type Eligibility string

const (
	// EligibilityReadyToCode means source-backed policy, ownership, discussion,
	// and issue evidence contain no reason to delay implementation.
	EligibilityReadyToCode Eligibility = "ready_to_code"
	// EligibilityNeedsDiagnosis means the problem or requested behavior still
	// needs reproduction, triage, or acceptance clarification.
	EligibilityNeedsDiagnosis Eligibility = "needs_diagnosis"
	// EligibilityNeedsCoordination means a contributor should coordinate with
	// maintainers or another participant before starting implementation.
	EligibilityNeedsCoordination Eligibility = "needs_coordination"
	// EligibilityBlocked means stored evidence contains an objective blocker.
	EligibilityBlocked Eligibility = "blocked"
)

// Signal is one explainable contribution to a candidate assessment.
type Signal struct {
	Code      string `json:"code"`
	Summary   string `json:"summary"`
	Weight    int    `json:"weight"`
	SourceURL string `json:"source_url,omitempty"`
}

// Unknown records missing or incomplete evidence without treating it as a
// negative ranking signal.
type Unknown struct {
	Code        string `json:"code"`
	Summary     string `json:"summary"`
	Remediation string `json:"remediation,omitempty"`
}

// Coverage is the bounded facet coverage relevant to a recommendation.
type Coverage struct {
	Facet    string    `json:"facet"`
	Scope    string    `json:"scope"`
	Present  bool      `json:"present"`
	Complete bool      `json:"complete"`
	AsOf     time.Time `json:"as_of,omitempty"`
}

// LinkedPullRequest is an open PR that explicitly references an issue.
type LinkedPullRequest struct {
	Number          int       `json:"number"`
	Title           string    `json:"title"`
	URL             string    `json:"url"`
	Closing         bool      `json:"closing"`
	SourceUpdatedAt time.Time `json:"source_updated_at,omitempty"`
}

// DuplicateCluster is a stored, explainable duplicate-candidate fact.
type DuplicateCluster struct {
	StableID       string    `json:"stable_id"`
	CanonicalRef   string    `json:"canonical_ref"`
	CandidateCount int       `json:"candidate_count"`
	SourceAsOf     time.Time `json:"source_as_of,omitempty"`
}

// RepositorySnapshot contains only product-owned facts required by Rank.
type RepositorySnapshot struct {
	Repo           domain.RepoRef
	Archived       bool
	SourceUpdated  time.Time
	Coverage       []Coverage
	GuidanceStatus string
	Guidance       []GuidanceDocument
}

// GuidanceDocument is one source-backed repository policy document.
type GuidanceDocument struct {
	Path    string
	Content string
	URL     string
}

// DiscussionComment is the product-owned discussion evidence consumed by the
// pure eligibility classifier.
type DiscussionComment struct {
	Author            string
	AuthorAssociation string
	Body              string
	URL               string
	CreatedAt         time.Time
}

// DiscussionSummary is the bounded, source-backed eligibility evidence
// extracted from a complete stored comment snapshot.
type DiscussionSummary struct {
	MaintainerResponseURL  string
	MaintainerDirection    string
	MaintainerDirectionURL string
	ActiveClaimAuthors     []string
	ActiveClaimURL         string
}

// IssueSnapshot contains one locally stored issue and its derived local facts.
type IssueSnapshot struct {
	Number             int
	State              string
	Title              string
	Body               string
	Labels             []string
	Assignees          []string
	Locked             bool
	SourceUpdated      time.Time
	URL                string
	Coverage           []Coverage
	Discussion         DiscussionSummary
	LinkedPullRequests []LinkedPullRequest
	RelatedWork        []RelatedWork
	RelatedWorkCapped  bool
	DuplicateCluster   *DuplicateCluster
}

// Options bounds a deterministic ranking operation.
type Options struct {
	Limit                       int
	Now                         time.Time
	TotalOpenIssues             int
	PopulationCapped            bool
	LinkedPullRequestScanCapped bool
	DuplicateClusterScanCapped  bool
}

// Candidate is one ranked, fully explained issue.
type Candidate struct {
	Rank               int                 `json:"rank"`
	Ref                string              `json:"ref"`
	Repo               string              `json:"repo"`
	Number             int                 `json:"number"`
	Title              string              `json:"title"`
	URL                string              `json:"url"`
	Labels             []string            `json:"labels"`
	Eligibility        Eligibility         `json:"eligibility"`
	BaseScore          int                 `json:"base_score"`
	Score              int                 `json:"score"`
	ScoreVersion       string              `json:"score_version"`
	Confidence         string              `json:"confidence"`
	PositiveSignals    []Signal            `json:"positive_signals"`
	Risks              []Signal            `json:"risks"`
	Blockers           []Signal            `json:"blockers"`
	Unknowns           []Unknown           `json:"unknowns"`
	Coverage           []Coverage          `json:"coverage"`
	LinkedPullRequests []LinkedPullRequest `json:"linked_pull_requests"`
	RelatedWork        []RelatedWork       `json:"related_work"`
	DuplicateCluster   *DuplicateCluster   `json:"duplicate_cluster,omitempty"`
	SourceUpdatedAt    time.Time           `json:"source_updated_at,omitempty"`
	SourceAsOf         time.Time           `json:"source_as_of,omitempty"`
}

// Report is the offline contribution radar result for one repository.
type Report struct {
	Repo                string      `json:"repo"`
	ScoreVersion        string      `json:"score_version"`
	GeneratedAt         time.Time   `json:"generated_at"`
	SourceAsOf          time.Time   `json:"source_as_of,omitempty"`
	Limit               int         `json:"limit"`
	TotalOpenIssues     int         `json:"total_open_issues"`
	CandidatePopulation int         `json:"candidate_population"`
	PopulationCapped    bool        `json:"population_capped"`
	Unknowns            []Unknown   `json:"unknowns"`
	Candidates          []Candidate `json:"candidates"`
}

// Rank scores a bounded set of local issue snapshots. Missing coverage is
// represented as unknown and never silently converted into a penalty.
func Rank(repo RepositorySnapshot, issues []IssueSnapshot, opts Options) (*Report, error) {
	if err := repo.Repo.Validate(); err != nil {
		return nil, err
	}
	if opts.Limit == 0 {
		opts.Limit = DefaultLimit
	}
	if opts.Limit < 1 || opts.Limit > MaxLimit {
		return nil, fmt.Errorf("radar limit must be between 1 and %d", MaxLimit)
	}
	if opts.Now.IsZero() {
		return nil, errors.New("radar evaluation time is required")
	}
	opts.Now = opts.Now.UTC()

	report := &Report{
		Repo:                repo.Repo.String(),
		ScoreVersion:        ScoreVersion,
		GeneratedAt:         opts.Now,
		SourceAsOf:          repo.SourceUpdated.UTC(),
		Limit:               opts.Limit,
		TotalOpenIssues:     opts.TotalOpenIssues,
		CandidatePopulation: len(issues),
		PopulationCapped:    opts.PopulationCapped,
		Unknowns:            repositoryUnknowns(repo),
		Candidates:          make([]Candidate, 0, len(issues)),
	}
	if opts.PopulationCapped {
		report.Unknowns = append(report.Unknowns, Unknown{
			Code:        "candidate_population_capped",
			Summary:     "ranking considered only the newest bounded open-issue population",
			Remediation: "narrow the repository or inspect the full local issue list",
		})
	}
	if opts.LinkedPullRequestScanCapped {
		report.Unknowns = append(report.Unknowns, Unknown{
			Code:        "linked_pull_request_scan_capped",
			Summary:     "active-implementation checks considered only the newest bounded open-PR population",
			Remediation: "inspect older open pull requests before starting work",
		})
	}
	if opts.DuplicateClusterScanCapped {
		report.Unknowns = append(report.Unknowns, Unknown{
			Code:        "duplicate_cluster_scan_may_be_capped",
			Summary:     "duplicate checks reached the bounded stored-cluster scan limit",
			Remediation: "inspect stored clusters before treating absence as proof of uniqueness",
		})
	}
	if report.TotalOpenIssues == 0 {
		report.TotalOpenIssues = len(issues)
	}

	for _, issue := range issues {
		candidate := assess(repo, issue, opts.Now)
		if opts.LinkedPullRequestScanCapped {
			addCandidateScanUnknown(&candidate, Unknown{
				Code:        "linked_pull_request_scan_capped",
				Summary:     "active-implementation evidence may omit older open pull requests",
				Remediation: "inspect older open pull requests before starting work",
			})
		}
		if opts.DuplicateClusterScanCapped {
			addCandidateScanUnknown(&candidate, Unknown{
				Code:        "duplicate_cluster_scan_may_be_capped",
				Summary:     "duplicate evidence may omit stored clusters beyond the scan bound",
				Remediation: "inspect stored clusters before treating absence as proof of uniqueness",
			})
		}
		report.Candidates = append(report.Candidates, candidate)
		if candidate.SourceAsOf.After(report.SourceAsOf) {
			report.SourceAsOf = candidate.SourceAsOf
		}
	}

	sort.SliceStable(report.Candidates, func(i, j int) bool {
		left, right := report.Candidates[i], report.Candidates[j]
		if eligibilityOrder(left.Eligibility) != eligibilityOrder(right.Eligibility) {
			return eligibilityOrder(left.Eligibility) < eligibilityOrder(right.Eligibility)
		}
		if left.Score != right.Score {
			return left.Score > right.Score
		}
		if !left.SourceUpdatedAt.Equal(right.SourceUpdatedAt) {
			return left.SourceUpdatedAt.After(right.SourceUpdatedAt)
		}
		return left.Number < right.Number
	})
	if len(report.Candidates) > opts.Limit {
		report.Candidates = report.Candidates[:opts.Limit]
	}
	for i := range report.Candidates {
		report.Candidates[i].Rank = i + 1
	}
	return report, nil
}

type candidateAssessment struct {
	repo      RepositorySnapshot
	issue     IssueSnapshot
	now       time.Time
	candidate Candidate
}

func assess(repo RepositorySnapshot, issue IssueSnapshot, now time.Time) Candidate {
	assessment := newCandidateAssessment(repo, issue, now)
	assessment.addRepositoryAndLabelSignals()
	assessment.addPolicySignals()
	assessment.addContentSignals()
	assessment.addFreshnessSignal()
	assessment.addOwnershipSignals()
	assessment.addDiscussionSignals()
	assessment.addCollisionSignals()
	assessment.addCoverageUnknowns()
	return assessment.finish()
}

func newCandidateAssessment(repo RepositorySnapshot, issue IssueSnapshot, now time.Time) *candidateAssessment {
	labels := append([]string{}, issue.Labels...)
	sort.Slice(labels, func(i, j int) bool {
		left, right := strings.ToLower(labels[i]), strings.ToLower(labels[j])
		if left != right {
			return left < right
		}
		return labels[i] < labels[j]
	})
	coverage := make([]Coverage, 0, len(repo.Coverage)+len(issue.Coverage))
	coverage = append(coverage, repo.Coverage...)
	coverage = append(coverage, issue.Coverage...)
	for i := range coverage {
		coverage[i].AsOf = coverage[i].AsOf.UTC()
	}
	sort.Slice(coverage, func(i, j int) bool {
		if coverage[i].Scope != coverage[j].Scope {
			return coverage[i].Scope < coverage[j].Scope
		}
		return coverage[i].Facet < coverage[j].Facet
	})
	linked := append([]LinkedPullRequest{}, issue.LinkedPullRequests...)
	for i := range linked {
		linked[i].SourceUpdatedAt = linked[i].SourceUpdatedAt.UTC()
	}
	sort.Slice(linked, func(i, j int) bool { return linked[i].Number < linked[j].Number })
	related := cloneRelatedWork(issue.RelatedWork)
	var duplicate *DuplicateCluster
	if issue.DuplicateCluster != nil {
		cluster := *issue.DuplicateCluster
		cluster.SourceAsOf = cluster.SourceAsOf.UTC()
		duplicate = &cluster
	}

	issueSourceUpdated := issue.SourceUpdated.UTC()
	sourceAsOf := issueSourceUpdated
	if repo.SourceUpdated.After(sourceAsOf) {
		sourceAsOf = repo.SourceUpdated.UTC()
	}
	candidate := Candidate{
		Ref:                fmt.Sprintf("issue:%s#%d", repo.Repo, issue.Number),
		Repo:               repo.Repo.String(),
		Number:             issue.Number,
		Title:              issue.Title,
		URL:                issue.URL,
		Labels:             labels,
		Eligibility:        EligibilityReadyToCode,
		BaseScore:          baseScore,
		Score:              baseScore,
		ScoreVersion:       ScoreVersion,
		PositiveSignals:    []Signal{},
		Risks:              []Signal{},
		Blockers:           []Signal{},
		Unknowns:           []Unknown{},
		Coverage:           coverage,
		LinkedPullRequests: linked,
		RelatedWork:        related,
		DuplicateCluster:   duplicate,
		SourceUpdatedAt:    issueSourceUpdated,
		SourceAsOf:         sourceAsOf,
	}
	for _, item := range coverage {
		if item.AsOf.After(candidate.SourceAsOf) {
			candidate.SourceAsOf = item.AsOf
		}
	}
	for _, pullRequest := range linked {
		if pullRequest.SourceUpdatedAt.After(candidate.SourceAsOf) {
			candidate.SourceAsOf = pullRequest.SourceUpdatedAt
		}
	}
	for _, work := range related {
		if work.SourceUpdatedAt.After(candidate.SourceAsOf) {
			candidate.SourceAsOf = work.SourceUpdatedAt
		}
		for _, item := range work.Evidence {
			if item.SourceAsOf.After(candidate.SourceAsOf) {
				candidate.SourceAsOf = item.SourceAsOf
			}
		}
	}
	return &candidateAssessment{repo: repo, issue: issue, now: now, candidate: candidate}
}

func (a *candidateAssessment) positive(code, summary string, weight int, source string) {
	a.candidate.PositiveSignals = append(a.candidate.PositiveSignals, Signal{Code: code, Summary: summary, Weight: weight, SourceURL: source})
	a.candidate.Score += weight
}

func (a *candidateAssessment) risk(code, summary string, weight int, source string) {
	a.candidate.Risks = append(a.candidate.Risks, Signal{Code: code, Summary: summary, Weight: weight, SourceURL: source})
	a.candidate.Score += weight
}

func (a *candidateAssessment) blocker(code, summary, source string) {
	a.candidate.Blockers = append(a.candidate.Blockers, Signal{Code: code, Summary: summary, SourceURL: source})
}

func (a *candidateAssessment) addRepositoryAndLabelSignals() {
	if !strings.EqualFold(strings.TrimSpace(a.issue.State), "open") {
		a.blocker("issue_not_open", "issue is not open in the local corpus", a.issue.URL)
	}
	if a.repo.Archived {
		a.blocker("repository_archived", "repository is archived", "https://github.com/"+a.repo.Repo.String())
	}
	if a.repo.GuidanceStatus == "available" {
		a.positive("contribution_guidance_available", "stored contribution guidance is available for review", 5, "")
	}

	labelSet := normalizedLabels(a.issue.Labels)
	if hasAny(labelSet, "duplicate", "invalid", "wontfix", "won't fix", "not planned") {
		a.blocker("terminal_label", "issue has a label indicating it should not be implemented", a.issue.URL)
	}
	if hasAny(labelSet, "good first issue", "good-first-issue", "first timers only", "beginner") {
		a.positive("beginner_label", "maintainers marked this as beginner-oriented", 15, a.issue.URL)
	}
	if hasAny(labelSet, "help wanted", "help-wanted", "up for grabs", "up-for-grabs") {
		a.positive("help_wanted", "maintainers marked this as open to outside help", 12, a.issue.URL)
	}
}

func (a *candidateAssessment) addContentSignals() {
	body := strings.TrimSpace(a.issue.Body)
	switch {
	case len(body) >= 200:
		a.positive("detailed_description", "issue has a detailed description", 8, a.issue.URL)
	case body != "":
		a.positive("description_present", "issue has a description", 4, a.issue.URL)
	default:
		a.risk("description_missing", "issue has no stored description", -6, a.issue.URL)
	}
	lowerBody := strings.ToLower(body)
	if strings.Contains(lowerBody, "- [ ]") {
		a.positive("acceptance_checklist", "issue includes an unchecked task or acceptance checklist", 8, a.issue.URL)
	}
	if containsAny(lowerBody, "steps to reproduce", "expected behavior", "actual behavior", "acceptance criteria") {
		a.positive("structured_problem", "issue contains structured problem or acceptance sections", 6, a.issue.URL)
	}
}

func (a *candidateAssessment) addFreshnessSignal() {
	if a.issue.SourceUpdated.IsZero() {
		a.candidate.Unknowns = append(a.candidate.Unknowns, Unknown{Code: "source_update_unknown", Summary: "issue source update time is unavailable"})
		return
	}
	age := max(a.now.Sub(a.issue.SourceUpdated), 0)
	switch {
	case age <= 30*24*time.Hour:
		a.positive("recently_updated", "issue was updated within 30 days", 12, a.issue.URL)
	case age <= 90*24*time.Hour:
		a.positive("recently_updated", "issue was updated within 90 days", 7, a.issue.URL)
	case age <= 180*24*time.Hour:
		a.positive("recently_updated", "issue was updated within 180 days", 3, a.issue.URL)
	case age > 365*24*time.Hour:
		a.risk("stale_issue", "issue has not been updated for over a year", -10, a.issue.URL)
	default:
		a.risk("aging_issue", "issue has not been updated for over six months", -5, a.issue.URL)
	}
}

func (a *candidateAssessment) addOwnershipSignals() {
	assignees := cleanSorted(a.issue.Assignees)
	if len(assignees) == 0 {
		a.positive("unassigned", "issue has no stored assignee", 5, a.issue.URL)
	} else {
		a.risk("assigned", "issue is assigned to "+strings.Join(assignees, ", "), -15, a.issue.URL)
		a.escalate(EligibilityNeedsCoordination)
	}
	if a.issue.Locked {
		a.risk("locked_conversation", "issue conversation is locked", -10, a.issue.URL)
		a.blocker("locked_conversation", "issue discussion is locked, so contribution coordination is unavailable", a.issue.URL)
	}
}

func (a *candidateAssessment) addCollisionSignals() {
	if a.candidate.DuplicateCluster != nil {
		cluster := a.candidate.DuplicateCluster
		summary := fmt.Sprintf("stored duplicate cluster %s has %d other candidate(s)", cluster.StableID, cluster.CandidateCount)
		a.risk("duplicate_candidates", summary, -18, "")
		a.escalate(EligibilityNeedsCoordination)
		if cluster.SourceAsOf.After(a.candidate.SourceAsOf) {
			a.candidate.SourceAsOf = cluster.SourceAsOf.UTC()
		}
	}

	closing, inbound, dependencies, outboundPRs := relatedWorkCollisions(a.candidate.RelatedWork)
	// Preserve the pre-v3 product input contract for direct Rank callers while
	// all application-produced candidates use RelatedWork as the source.
	if len(a.candidate.RelatedWork) == 0 {
		for _, pullRequest := range a.candidate.LinkedPullRequests {
			if pullRequest.Closing {
				closing = append(closing, RelatedWork{Kind: "pull_request", Number: pullRequest.Number, URL: pullRequest.URL})
			} else {
				inbound = append(inbound, RelatedWork{Kind: "pull_request", Number: pullRequest.Number, URL: pullRequest.URL})
			}
		}
	}
	if len(closing) > 0 {
		work := closing[0]
		a.risk("active_closing_pr", fmt.Sprintf("open PR #%d declares that it closes this issue", work.Number), -30, work.URL)
		a.blocker("active_implementation", fmt.Sprintf("open PR #%d already declares an implementation", work.Number), work.URL)
		return
	}
	if len(inbound) > 0 {
		a.risk("linked_open_pr", fmt.Sprintf("%d open PR(s) explicitly reference this issue", len(inbound)), -20, inbound[0].URL)
		a.escalate(EligibilityNeedsCoordination)
	}
	if len(dependencies) > 0 {
		a.risk("open_dependency", fmt.Sprintf("candidate explicitly depends on %d open related thread(s)", len(dependencies)), -15, dependencies[0].URL)
		a.escalate(EligibilityNeedsCoordination)
	}
	if len(outboundPRs) > 0 {
		a.risk("related_open_pr", fmt.Sprintf("candidate discussion explicitly references %d open related PR(s)", len(outboundPRs)), -12, outboundPRs[0].URL)
		a.escalate(EligibilityNeedsCoordination)
	}
}

func (a *candidateAssessment) addCoverageUnknowns() {
	metadataComplete := coverageComplete(a.repo.Coverage, "metadata")
	threadsComplete := coverageComplete(a.repo.Coverage, "threads")
	commentsPresent, commentsComplete := coverageState(a.issue.Coverage, "issue_comments")
	if !metadataComplete {
		a.candidate.Unknowns = append(a.candidate.Unknowns, Unknown{
			Code:        "metadata_coverage_incomplete",
			Summary:     "repository metadata coverage is missing or incomplete, so archived state may be unknown",
			Remediation: "run an explicit sync before treating repository state as complete",
		})
		a.escalate(EligibilityNeedsDiagnosis)
	}
	if !threadsComplete {
		a.candidate.Unknowns = append(a.candidate.Unknowns, Unknown{
			Code:        "threads_coverage_incomplete",
			Summary:     "repository thread coverage is missing or incomplete",
			Remediation: "run an explicit archive sync before relying on this ranking",
		})
		a.escalate(EligibilityNeedsDiagnosis)
	}
	if !commentsPresent {
		a.candidate.Unknowns = append(a.candidate.Unknowns, Unknown{
			Code:        "comments_not_hydrated",
			Summary:     "issue comments have not been hydrated",
			Remediation: fmt.Sprintf("run gitcontribute archive hydrate %s#%d --with issue_comments --max-pages 3", a.repo.Repo, a.issue.Number),
		})
		a.escalate(EligibilityNeedsDiagnosis)
	} else if !commentsComplete {
		a.candidate.Unknowns = append(a.candidate.Unknowns, Unknown{
			Code:        "comments_coverage_incomplete",
			Summary:     "issue comment coverage is incomplete",
			Remediation: fmt.Sprintf("rerun gitcontribute archive hydrate %s#%d --with issue_comments --max-pages 3", a.repo.Repo, a.issue.Number),
		})
		a.escalate(EligibilityNeedsDiagnosis)
	}
	switch a.repo.GuidanceStatus {
	case "available":
	case "missing":
		a.candidate.Unknowns = append(a.candidate.Unknowns, Unknown{
			Code: "contribution_guidance_missing", Summary: "repository contribution guidance was checked but not found",
			Remediation: "confirm contribution policy with maintainers before starting implementation",
		})
		a.escalate(EligibilityNeedsCoordination)
	default:
		a.candidate.Unknowns = append(a.candidate.Unknowns, Unknown{
			Code: "contribution_guidance_unknown", Summary: "contribution and AI policy has not been completely ingested",
			Remediation: "run an explicit repository sync before starting implementation",
		})
		a.escalate(EligibilityNeedsCoordination)
	}
	a.candidate.Confidence = confidence(metadataComplete, threadsComplete, commentsComplete, a.repo.GuidanceStatus == "available")
	unknownRelatedPullRequests := 0
	for _, work := range a.candidate.RelatedWork {
		if work.Kind == "pull_request" && strings.TrimSpace(work.State) == "" {
			unknownRelatedPullRequests++
		}
	}
	if unknownRelatedPullRequests > 0 {
		a.candidate.Unknowns = append(a.candidate.Unknowns, Unknown{
			Code:        "related_pull_request_state_unknown",
			Summary:     fmt.Sprintf("%d related pull request(s) lack stored open or closed state", unknownRelatedPullRequests),
			Remediation: "sync the referenced pull-request headers before treating them as inactive",
		})
		a.escalate(EligibilityNeedsCoordination)
		if a.candidate.Confidence == "high" {
			a.candidate.Confidence = "medium"
		}
	}
	if a.issue.RelatedWorkCapped {
		a.candidate.Unknowns = append(a.candidate.Unknowns, Unknown{
			Code: "related_work_scan_capped", Summary: "related-work evidence exceeded the per-candidate bound",
			Remediation: "inspect the thread research brief and stored relationships before starting work",
		})
		a.escalate(EligibilityNeedsCoordination)
		if a.candidate.Confidence == "high" {
			a.candidate.Confidence = "medium"
		}
	}
}

func (a *candidateAssessment) finish() Candidate {
	if len(a.candidate.Blockers) > 0 {
		a.candidate.Eligibility = EligibilityBlocked
	}
	a.candidate.Score = min(100, max(0, a.candidate.Score))
	return a.candidate
}

func addCandidateScanUnknown(candidate *Candidate, unknown Unknown) {
	candidate.Unknowns = append(candidate.Unknowns, unknown)
	if eligibilitySeverity(candidate.Eligibility) < eligibilitySeverity(EligibilityNeedsCoordination) {
		candidate.Eligibility = EligibilityNeedsCoordination
	}
	if candidate.Confidence == "high" {
		candidate.Confidence = "medium"
	}
}

func repositoryUnknowns(repo RepositorySnapshot) []Unknown {
	out := []Unknown{}
	present, complete := coverageState(repo.Coverage, "metadata")
	if !present {
		out = append(out, Unknown{
			Code:        "metadata_not_synced",
			Summary:     "repository metadata coverage has not been recorded",
			Remediation: "run an explicit sync",
		})
	} else if !complete {
		out = append(out, Unknown{
			Code:        "metadata_coverage_incomplete",
			Summary:     "repository metadata coverage is incomplete",
			Remediation: "complete an explicit sync",
		})
	}
	present, complete = coverageState(repo.Coverage, "threads")
	if !present {
		out = append(out, Unknown{
			Code:        "threads_not_synced",
			Summary:     "repository thread coverage has not been recorded",
			Remediation: "run an explicit archive sync",
		})
	} else if !complete {
		out = append(out, Unknown{
			Code:        "threads_coverage_incomplete",
			Summary:     "repository thread coverage is incomplete",
			Remediation: "complete an explicit archive sync",
		})
	}
	switch repo.GuidanceStatus {
	case "available":
	case "missing":
		out = append(out, Unknown{Code: "contribution_guidance_missing", Summary: "repository contribution guidance was checked but not found"})
	default:
		out = append(out, Unknown{Code: "contribution_guidance_unknown", Summary: "contribution guidance has not been ingested into the local corpus"})
	}
	return out
}

func normalizedLabels(labels []string) map[string]struct{} {
	out := make(map[string]struct{}, len(labels))
	for _, label := range labels {
		label = strings.ToLower(strings.TrimSpace(label))
		if label != "" {
			out[label] = struct{}{}
		}
	}
	return out
}

func hasAny(values map[string]struct{}, candidates ...string) bool {
	for _, candidate := range candidates {
		if _, ok := values[candidate]; ok {
			return true
		}
	}
	return false
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func cleanSorted(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func coverageState(coverage []Coverage, facet string) (bool, bool) {
	for _, item := range coverage {
		if item.Facet == facet {
			return item.Present, item.Present && item.Complete
		}
	}
	return false, false
}

func coverageComplete(coverage []Coverage, facet string) bool {
	present, complete := coverageState(coverage, facet)
	return present && complete
}

func confidence(metadataComplete, threadsComplete, commentsComplete, guidanceAvailable bool) string {
	complete := 0
	for _, value := range []bool{metadataComplete, threadsComplete, commentsComplete, guidanceAvailable} {
		if value {
			complete++
		}
	}
	switch complete {
	case 4:
		return "high"
	case 2, 3:
		return "medium"
	default:
		return "low"
	}
}

func eligibilityOrder(value Eligibility) int {
	switch value {
	case EligibilityReadyToCode:
		return 0
	case EligibilityNeedsDiagnosis:
		return 1
	case EligibilityNeedsCoordination:
		return 2
	default:
		return 3
	}
}
