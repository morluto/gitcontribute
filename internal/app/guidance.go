package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
)

const (
	// FacetContributionGuidance stores bounded repository contribution and AI policy documents.
	FacetContributionGuidance = "contribution_guidance"
	maxGuidanceDocumentBytes  = 256 * 1024
)

var contributionGuidancePaths = []string{
	".github/CONTRIBUTING.md",
	"CONTRIBUTING.md",
	"docs/CONTRIBUTING.md",
	".github/AI_POLICY.md",
	".github/AI-CONTRIBUTION-POLICY.md",
	".github/GENERATIVE_AI_POLICY.md",
	"AI_POLICY.md",
	"AI.md",
}

type storedGuidanceDocument struct {
	File       github.RepositoryFile
	ObservedAt time.Time
	AsOf       time.Time
}

// syncRepositoryGuidance fetches only a fixed set of policy paths. Repository
// text is persisted as untrusted source data and is never executed.
func syncRepositoryGuidance(
	ctx context.Context,
	c *corpus.Corpus,
	reader github.Reader,
	repo corpus.Repository,
	ref domain.RepoRef,
	sourceUpdatedAt time.Time,
	runID int64,
	budget *syncRequestBudget,
) error {
	fileReader, ok := reader.(github.RepositoryFileReader)
	if !ok {
		return nil
	}

	pages := make([]corpus.FacetObservationInput, 0, len(contributionGuidancePaths))
	for _, path := range contributionGuidancePaths {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := budget.take(); err != nil {
			return err
		}
		file, _, err := fileReader.GetRepositoryFile(ctx, ref.Owner, ref.Repo, path)
		if err != nil {
			var notFound *github.NotFoundError
			if errors.As(err, &notFound) {
				continue
			}
			return fmt.Errorf("read contribution guidance %q: %w", path, err)
		}
		if len(file.Content) > maxGuidanceDocumentBytes {
			return fmt.Errorf("contribution guidance %q exceeds %d bytes", path, maxGuidanceDocumentBytes)
		}
		payload, err := json.Marshal(file)
		if err != nil {
			return fmt.Errorf("marshal contribution guidance %q: %w", path, err)
		}
		pages = append(pages, corpus.FacetObservationInput{SourceUpdatedAt: sourceUpdatedAt, Payload: string(payload)})
	}

	if err := c.ApplyFacetObservationSet(ctx, repo.ID, nil, FacetContributionGuidance, sourceUpdatedAt, pages, true, runID); err != nil {
		return fmt.Errorf("store contribution guidance: %w", err)
	}
	return nil
}

func readContributionGuidanceDocuments(ctx context.Context, c *corpus.Corpus, repoID int64) ([]storedGuidanceDocument, error) {
	observations, capped, err := c.ListFacetObservationsBounded(ctx, repoID, nil, FacetContributionGuidance, len(contributionGuidancePaths))
	if err != nil {
		return nil, fmt.Errorf("list contribution guidance: %w", err)
	}
	if capped {
		return nil, errors.New("stored contribution guidance exceeds the fixed document bound")
	}
	documents := make([]storedGuidanceDocument, 0, len(observations))
	for _, observation := range observations {
		var file github.RepositoryFile
		if err := json.Unmarshal([]byte(observation.Payload), &file); err != nil {
			return nil, fmt.Errorf("decode contribution guidance observation %d: %w", observation.ID, err)
		}
		if strings.TrimSpace(file.Content) == "" {
			continue
		}
		documents = append(documents, storedGuidanceDocument{
			File: file, ObservedAt: observation.ObservedAt, AsOf: observation.SourceUpdatedAt,
		})
	}
	sort.Slice(documents, func(i, j int) bool { return documents[i].File.Path < documents[j].File.Path })
	return documents, nil
}

func renderContributionGuidance(documents []storedGuidanceDocument) (string, []domain.SourceRef) {
	sections := make([]string, 0, len(documents))
	refs := make([]domain.SourceRef, 0, len(documents))
	for _, document := range documents {
		sections = append(sections, fmt.Sprintf("## %s\n\n%s", document.File.Path, strings.TrimSpace(document.File.Content)))
		refs = append(refs, domain.SourceRef{
			Source: "github:rest", URL: document.File.HTMLURL, CommitSHA: document.File.SHA,
			ObservedAt: document.ObservedAt, AsOf: document.AsOf,
		})
	}
	return strings.Join(sections, "\n\n"), refs
}
