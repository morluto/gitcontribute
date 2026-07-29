package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/research"
	"github.com/morluto/gitcontribute/internal/tuicontract"
)

const (
	tuiActionStartInvestigation = "start_investigation"
	tuiActionCheckDuplicates    = "check_duplicates"
	tuiActionCheckCollisions    = "check_collisions"
	tuiActionCheckReadiness     = "check_readiness"
	tuiActionRefreshClusters    = "refresh_clusters"
	tuiActionResultLimit        = 10
)

var _ tuicontract.ActionProvider = (*Service)(nil)

// Actions returns only parameter-free operations that the application can
// execute for the selected item today.
func (s *Service) Actions(_ context.Context, item tuicontract.Item) ([]tuicontract.Action, error) {
	switch item.Kind {
	case "candidate":
		return []tuicontract.Action{{
			ID: tuiActionStartInvestigation, Label: "Start investigation",
			Description: "Create or reopen the local investigation and seed hypothesis.",
			Capability:  tuicontract.CapabilityLocalWrite, RequiresConfirmation: true,
		}}, nil
	case "hypothesis":
		return researchActions(), nil
	case "opportunity":
		return append([]tuicontract.Action{{
			ID: tuiActionCheckReadiness, Label: "Check readiness",
			Description: "Evaluate contribution gates from the local corpus.",
			Capability:  tuicontract.CapabilityOfflineRead,
		}}, researchActions()...), nil
	case "repository":
		return []tuicontract.Action{{
			ID: tuiActionRefreshClusters, Label: "Refresh related-work clusters",
			Description: "Recompute and persist the local duplicate projection.",
			Capability:  tuicontract.CapabilityLocalWrite, RequiresConfirmation: true,
		}}, nil
	default:
		return nil, nil
	}
}

func researchActions() []tuicontract.Action {
	return []tuicontract.Action{
		{
			ID: tuiActionCheckDuplicates, Label: "Find similar threads",
			Description: "Search the local corpus for possible duplicates.",
			Capability:  tuicontract.CapabilityOfflineRead,
		},
		{
			ID: tuiActionCheckCollisions, Label: "Find competing pull requests",
			Description: "Search the local corpus for open pull requests with overlapping work.",
			Capability:  tuicontract.CapabilityOfflineRead,
		},
	}
}

// ExecuteAction dispatches a previously offered operation. Every case uses
// local application services; none performs network access or GitHub mutation.
func (s *Service) ExecuteAction(ctx context.Context, request tuicontract.ActionRequest) (tuicontract.ActionResult, error) {
	item := request.Item
	switch request.ActionID {
	case tuiActionStartInvestigation:
		if item.Kind != "candidate" {
			return tuicontract.ActionResult{}, invalidTUIAction(request)
		}
		ref, err := research.ParseThreadRef(item.Ref)
		if err != nil {
			return tuicontract.ActionResult{}, err
		}
		result, err := s.StartInvestigationFromThread(ctx, ref)
		if err != nil {
			return tuicontract.ActionResult{}, err
		}
		disposition := "Started"
		if !result.Created {
			disposition = "Using existing"
		}
		return tuicontract.ActionResult{
			Title:   "Investigation " + strings.ToLower(disposition),
			Message: fmt.Sprintf("%s investigation for %s", disposition, displayActionItem(item)),
			Facts: []tuicontract.ActionResultFact{
				{Label: "Investigation", Value: result.Investigation.ID},
				{Label: "Seed hypothesis", Value: result.Hypothesis.ID},
			},
			Target: &tuicontract.ActionTarget{Stage: "research", ID: result.Hypothesis.ID},
			Reload: true,
		}, nil

	case tuiActionCheckDuplicates:
		var duplicateResult *contracts.DuplicateCheckResult
		var err error
		switch item.Kind {
		case "hypothesis":
			duplicateResult, err = s.CheckHypothesisDuplicates(ctx, item.ID, tuiActionResultLimit)
		case "opportunity":
			duplicateResult, err = s.CheckOpportunityDuplicates(ctx, item.ID, tuiActionResultLimit)
		default:
			return tuicontract.ActionResult{}, invalidTUIAction(request)
		}
		if err != nil {
			return tuicontract.ActionResult{}, err
		}
		return tuicontract.ActionResult{
			Title:          "Duplicate check complete",
			Message:        fmt.Sprintf("Found %d similar local threads for %s", duplicateResult.Total, displayActionItem(item)),
			Facts:          checkResultFacts(duplicateResult.Total, duplicateResult.Limit),
			Items:          actionEvidenceItems(duplicateResult.Findings),
			SourceRevision: duplicateResult.SourceRevision,
		}, nil

	case tuiActionCheckCollisions:
		var collisionResult *contracts.CollisionCheckResult
		var err error
		switch item.Kind {
		case "hypothesis":
			collisionResult, err = s.CheckHypothesisCollisions(ctx, item.ID, tuiActionResultLimit)
		case "opportunity":
			collisionResult, err = s.CheckOpportunityCollisions(ctx, item.ID, tuiActionResultLimit)
		default:
			return tuicontract.ActionResult{}, invalidTUIAction(request)
		}
		if err != nil {
			return tuicontract.ActionResult{}, err
		}
		return tuicontract.ActionResult{
			Title:          "Competing-work check complete",
			Message:        fmt.Sprintf("Found %d competing local pull requests for %s", collisionResult.Total, displayActionItem(item)),
			Facts:          checkResultFacts(collisionResult.Total, collisionResult.Limit),
			Items:          actionEvidenceItems(collisionResult.Findings),
			SourceRevision: collisionResult.SourceRevision,
		}, nil

	case tuiActionCheckReadiness:
		if item.Kind != "opportunity" {
			return tuicontract.ActionResult{}, invalidTUIAction(request)
		}
		result, err := s.OpportunityReadiness(ctx, item.ID)
		if err != nil {
			return tuicontract.ActionResult{}, err
		}
		return tuicontract.ActionResult{
			Title:   "Readiness check complete",
			Message: fmt.Sprintf("Readiness is %s for %s", result.Status, displayActionItem(item)),
			Facts: []tuicontract.ActionResultFact{
				{Label: "Status", Value: result.Status},
				{Label: "Checks", Value: strconv.Itoa(len(result.Checks))},
			},
			Items:          readinessActionItems(result.Checks),
			SourceRevision: result.RuleSetVersion,
		}, nil

	case tuiActionRefreshClusters:
		if item.Kind != "repository" {
			return tuicontract.ActionResult{}, invalidTUIAction(request)
		}
		repo, err := parseTUIRepo(item.Ref)
		if err != nil {
			return tuicontract.ActionResult{}, err
		}
		result, err := s.RefreshClusters(ctx, repo)
		if err != nil {
			return tuicontract.ActionResult{}, err
		}
		return tuicontract.ActionResult{
			Title:   "Related-work refresh complete",
			Message: fmt.Sprintf("Cluster projection %s for %s", result.Disposition, item.Ref),
			Facts: []tuicontract.ActionResultFact{
				{Label: "Disposition", Value: result.Disposition},
				{Label: "Candidates", Value: strconv.Itoa(result.Stats.CandidateCount)},
				{Label: "Clusters", Value: strconv.Itoa(result.Stats.ClusterCount)},
			},
			SourceRevision: result.Projection.SourceRevision,
			Reload:         true,
		}, nil
	default:
		return tuicontract.ActionResult{}, invalidTUIAction(request)
	}
}

func checkResultFacts(total, limit int) []tuicontract.ActionResultFact {
	return []tuicontract.ActionResultFact{
		{Label: "Possible matches", Value: strconv.Itoa(total)},
		{Label: "Result limit", Value: strconv.Itoa(limit)},
	}
}

func actionEvidenceItems(findings []evidence.Evidence) []tuicontract.ActionResultItem {
	items := make([]tuicontract.ActionResultItem, 0, len(findings))
	for _, finding := range findings {
		item := tuicontract.ActionResultItem{
			Ref: finding.ID, Title: finding.Description, Status: string(finding.Relation),
		}
		if item.Title == "" {
			item.Title = "Potential overlapping work"
		}
		if len(finding.SourceRefs) > 0 {
			item.Source = finding.SourceRefs[0].URL
			if item.Ref == "" {
				item.Ref = finding.SourceRefs[0].URL
			}
		}
		items = append(items, item)
	}
	return items
}

func readinessActionItems(checks []contracts.ReadinessCheck) []tuicontract.ActionResultItem {
	items := make([]tuicontract.ActionResultItem, 0, len(checks))
	for _, check := range checks {
		items = append(items, tuicontract.ActionResultItem{
			Ref: check.CheckID, Title: check.Summary, Status: check.Status,
			Source: strings.Join(check.EvidenceRefs, ", "),
		})
	}
	return items
}

func parseTUIRepo(value string) (contracts.RepoRef, error) {
	parts := strings.Split(value, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return contracts.RepoRef{}, fmt.Errorf("invalid repository reference %q", value)
	}
	return contracts.RepoRef{Owner: parts[0], Repo: parts[1]}, nil
}

func displayActionItem(item tuicontract.Item) string {
	if strings.TrimSpace(item.Title) != "" {
		return item.Title
	}
	return item.Ref
}

func invalidTUIAction(request tuicontract.ActionRequest) error {
	return fmt.Errorf("action %q is not available for %s", request.ActionID, request.Item.Kind)
}
