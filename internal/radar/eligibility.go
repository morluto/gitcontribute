package radar

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const activeClaimWindow = 90 * 24 * time.Hour

var (
	helpWantedPolicyPatterns = compilePatterns(
		`accept(?:s|ed)? pull requests? (?:only )?for issues? (?:that are )?label(?:l)?ed help wanted`,
		`do not (?:open|submit) (?:a )?pull requests? for issues? without (?:the )?help wanted label`,
		`pull requests? (?:are )?(?:only )?accepted for issues?.{0,80}help wanted`,
		`only accept pull requests?.{0,80}help wanted`,
	)
	aiProhibitedPatterns = compilePatterns(
		`(?:ai|artificial intelligence|llm|generative ai)[ -]generated contributions? (?:are )?(?:not accepted|prohibited|forbidden)`,
		`(?:do not|don't|must not) (?:use|submit).{0,80}(?:ai|artificial intelligence|llm|generative ai)`,
		`(?:do not|don't) accept.{0,80}(?:ai|artificial intelligence|llm|generative ai)`,
	)
	aiDisclosurePatterns = compilePatterns(
		`(?:disclose|declare|identify).{0,80}(?:ai|artificial intelligence|llm|generative ai)`,
		`(?:ai|artificial intelligence|llm|generative ai).{0,80}(?:disclosure|must be disclosed|must be declared)`,
	)
	maintainerDeclinePatterns = compilePatterns(
		`not accepting (?:any )?(?:implementations?|pull requests?|prs?|contributions?)`,
		`(?:do not|don't|please do not|please don't) (?:open|submit|implement|work on)`,
		`(?:will not|won't) accept (?:a )?(?:pull request|pr|implementation|contribution)`,
		`(?:implementation|pull request|pr|contribution) (?:is|are) not (?:wanted|accepted|planned)`,
	)
	maintainerDiagnosisPatterns = compilePatterns(
		`(?:can|could|would) you (?:please )?(?:test|reproduce|provide|share|confirm|check)`,
		`please (?:test|reproduce|provide|share|confirm|check)`,
		`need(?:s|ed)? (?:more )?(?:information|details|a reproduction|reproduction|diagnosis)`,
		`does .{0,80} show anything`,
	)
	maintainerApprovalPatterns = compilePatterns(
		`(?:contributions?|pull requests?|prs?) (?:are )?welcome`,
		`(?:please|feel free to|go ahead and) (?:open|send|submit|work on|implement)`,
		`happy to accept (?:a )?(?:pull request|pr|implementation|contribution)`,
		`help wanted`,
	)
	claimPatterns = compilePatterns(
		`(?:i|we) (?:am|are|'m|'re) (?:currently )?working on (?:this|it)`,
		`(?:i|we) (?:am|'m) working on (?:a fix|an implementation|a pull request|a pr)`,
		`(?:i|we) (?:have|'ve) started working on (?:this|it)`,
		`(?:i|we) (?:can|will|'ll|would like to|'d like to|want to) (?:work on|take|pick up|handle|implement|fix) (?:this|it)`,
		`(?:i|we) (?:can|will|'ll|would like to|'d like to|want to) pick (?:this|it) up`,
		`(?:i|we) (?:will|'ll) (?:open|submit|send) (?:a )?(?:pull request|pr)`,
		`can i (?:work on|take|pick up|handle|implement|fix) (?:this|it)`,
		`can i pick (?:this|it) up`,
		`let me (?:work on|take|pick up|handle|implement|fix) (?:this|it)`,
	)
	releaseClaimPatterns = compilePatterns(
		`(?:i|we) (?:am|are|'m|'re) no longer working on (?:this|it)`,
		`(?:i|we) (?:cannot|can't|won't|will not) (?:continue|work on|finish|handle) (?:this|it)`,
		`feel free to (?:take|pick up|work on|handle) (?:this|it)`,
		`unassign me`,
	)
)

func compilePatterns(values ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(values))
	for i, value := range values {
		out[i] = regexp.MustCompile(`(?i)\b` + value + `\b`)
	}
	return out
}

func (a *candidateAssessment) escalate(value Eligibility) {
	if eligibilitySeverity(value) > eligibilitySeverity(a.candidate.Eligibility) {
		a.candidate.Eligibility = value
	}
}

func eligibilitySeverity(value Eligibility) int {
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

func (a *candidateAssessment) addPolicySignals() {
	labels := canonicalLabels(a.issue.Labels)
	if hasCanonicalLabel(labels, "internal", "internal only", "core", "staff only") {
		a.risk("restricted_label", "issue is labelled for internal or core-team work", -30, a.issue.URL)
		a.blocker("external_contribution_restricted", "stored labels reserve this issue for internal or core-team work", a.issue.URL)
	}
	if hasCanonicalLabel(labels, "needs info", "needs reproduction", "needs triage", "question") {
		a.risk("diagnosis_label", "issue labels indicate that more diagnosis or triage is required", -12, a.issue.URL)
		a.escalate(EligibilityNeedsDiagnosis)
	}
	if hasCanonicalLabel(labels, "stale", "needs decision", "needs design", "tracking", "tracking issue", "gsoc", "proposal", "discussion") {
		a.risk("coordination_label", "issue labels require maintainer coordination before implementation", -12, a.issue.URL)
		a.escalate(EligibilityNeedsCoordination)
	}

	if a.repo.GuidanceStatus != "available" {
		return
	}
	helpWantedURL := ""
	aiProhibitedURL := ""
	aiDisclosureURL := ""
	for _, document := range a.repo.Guidance {
		policy := normalizePolicyText(document.Content)
		if helpWantedURL == "" && matchesAny(policy, helpWantedPolicyPatterns) {
			helpWantedURL = document.URL
		}
		if aiProhibitedURL == "" && matchesAny(policy, aiProhibitedPatterns) {
			aiProhibitedURL = document.URL
		}
		if aiDisclosureURL == "" && matchesAny(policy, aiDisclosurePatterns) {
			aiDisclosureURL = document.URL
		}
	}
	labelSet := normalizedLabels(a.issue.Labels)
	if helpWantedURL != "" {
		if hasAny(labelSet, "help wanted", "help-wanted") {
			a.positive("policy_allows_issue", "stored contribution policy allows pull requests for help-wanted issues", 8, helpWantedURL)
		} else {
			a.risk("policy_label_required", "stored contribution policy requires the help-wanted label", -30, helpWantedURL)
			a.blocker("contribution_policy_mismatch", "repository policy does not accept pull requests for issues without the help-wanted label", helpWantedURL)
		}
	}
	if aiProhibitedURL != "" {
		a.risk("ai_contributions_prohibited", "stored repository policy prohibits AI-assisted or AI-generated contributions", -30, aiProhibitedURL)
		a.blocker("ai_policy_block", "repository policy prohibits AI-assisted or AI-generated contributions", aiProhibitedURL)
	} else if aiDisclosureURL != "" {
		a.risk("ai_disclosure_required", "stored repository policy requires AI-use disclosure", -8, aiDisclosureURL)
		a.escalate(EligibilityNeedsCoordination)
	}
}

func (a *candidateAssessment) addDiscussionSignals() {
	discussion := a.issue.Discussion
	if discussion.MaintainerResponseURL != "" {
		a.positive("maintainer_response", "stored comments include a maintainer response", 4, discussion.MaintainerResponseURL)
		switch discussion.MaintainerDirection {
		case "declined":
			a.risk("maintainer_declined", "latest stored maintainer direction declines this implementation", -30, discussion.MaintainerDirectionURL)
			a.blocker("maintainer_declined", "a maintainer explicitly declined this implementation direction", discussion.MaintainerDirectionURL)
		case "diagnosis":
			a.risk("maintainer_requested_diagnosis", "latest stored maintainer direction requests diagnosis or reproduction evidence", -12, discussion.MaintainerDirectionURL)
			a.escalate(EligibilityNeedsDiagnosis)
		case "approved":
			a.positive("maintainer_invitation", "latest stored maintainer direction welcomes implementation", 8, discussion.MaintainerDirectionURL)
		}
	}
	if len(discussion.ActiveClaimAuthors) > 0 {
		a.risk("active_claim", fmt.Sprintf("recent discussion indicates that %s has claimed this work", strings.Join(discussion.ActiveClaimAuthors, ", ")), -20, discussion.ActiveClaimURL)
		a.escalate(EligibilityNeedsCoordination)
	}
}

// SummarizeDiscussion classifies a complete stored comment snapshot into the
// compact facts needed by contribution eligibility. It performs no I/O.
func SummarizeDiscussion(comments []DiscussionComment, now time.Time) DiscussionSummary {
	comments = append([]DiscussionComment(nil), comments...)
	sort.SliceStable(comments, func(i, j int) bool { return comments[i].CreatedAt.Before(comments[j].CreatedAt) })

	var latestMaintainer *DiscussionComment
	var decisionComment *DiscussionComment
	decision := ""
	activeClaims := map[string]DiscussionComment{}
	for i := range comments {
		comment := comments[i]
		body := normalizeDiscussionText(comment.Body)
		if isMaintainerAssociation(comment.AuthorAssociation) {
			currentComment := comment
			latestMaintainer = &currentComment
			switch {
			case matchesAny(body, maintainerDeclinePatterns):
				decision = "declined"
				decisionComment = &currentComment
			case matchesAny(body, maintainerDiagnosisPatterns):
				decision = "diagnosis"
				decisionComment = &currentComment
			case matchesAny(body, maintainerApprovalPatterns):
				decision = "approved"
				decisionComment = &currentComment
			}
		}

		author := strings.ToLower(strings.TrimSpace(comment.Author))
		if author == "" || isBotAuthor(author) {
			continue
		}
		if matchesAny(body, releaseClaimPatterns) {
			delete(activeClaims, author)
			continue
		}
		claimAge := now.Sub(comment.CreatedAt)
		if matchesAny(body, claimPatterns) && !comment.CreatedAt.IsZero() && claimAge >= 0 && claimAge <= activeClaimWindow {
			activeClaims[author] = comment
		}
	}

	summary := DiscussionSummary{MaintainerDirection: decision}
	if latestMaintainer != nil {
		summary.MaintainerResponseURL = latestMaintainer.URL
		summary.MaintainerDirectionURL = latestMaintainer.URL
		if decisionComment != nil {
			summary.MaintainerDirectionURL = decisionComment.URL
		}
	}

	if len(activeClaims) > 0 {
		authors := make([]string, 0, len(activeClaims))
		for author := range activeClaims {
			authors = append(authors, author)
		}
		sort.Strings(authors)
		claim := activeClaims[authors[0]]
		summary.ActiveClaimAuthors = authors
		summary.ActiveClaimURL = claim.URL
	}
	return summary
}

func normalizePolicyText(value string) string {
	value = strings.NewReplacer("`", "", "*", " ", "_", " ", "\n", " ", "\r", " ", "\t", " ").Replace(strings.ToLower(value))
	return strings.Join(strings.Fields(value), " ")
}

func normalizeDiscussionText(value string) string {
	lines := strings.Split(value, "\n")
	kept := make([]string, 0, len(lines))
	inFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence || strings.HasPrefix(trimmed, ">") {
			continue
		}
		kept = append(kept, trimmed)
	}
	return normalizePolicyText(strings.Join(kept, " "))
}

func matchesAny(value string, patterns []*regexp.Regexp) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

func canonicalLabels(labels []string) map[string]struct{} {
	out := make(map[string]struct{}, len(labels))
	replacer := strings.NewReplacer("-", " ", "_", " ", ":", " ", "/", " ", ".", " ")
	for _, label := range labels {
		canonical := strings.Join(strings.Fields(replacer.Replace(strings.ToLower(strings.TrimSpace(label)))), " ")
		if canonical != "" {
			out[canonical] = struct{}{}
		}
	}
	return out
}

func hasCanonicalLabel(labels map[string]struct{}, candidates ...string) bool {
	for label := range labels {
		for _, candidate := range candidates {
			if label == candidate || strings.HasSuffix(label, " "+candidate) {
				return true
			}
		}
	}
	return false
}

func isMaintainerAssociation(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "OWNER", "MEMBER", "COLLABORATOR":
		return true
	default:
		return false
	}
}

func isBotAuthor(value string) bool {
	return strings.HasSuffix(value, "[bot]") || strings.HasSuffix(value, "-bot") || value == "renovate"
}
