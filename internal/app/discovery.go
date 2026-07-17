package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/discovery"
	"github.com/morluto/gitcontribute/internal/github"
)

type searchSourceDefinition struct {
	Query string `json:"query"`
}

var sourceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func (s *Service) AddSearchSource(ctx context.Context, name, query string) (*cli.SourceResult, error) {
	name = strings.TrimSpace(name)
	query = strings.TrimSpace(query)
	if !sourceNamePattern.MatchString(name) {
		return nil, errors.New("source name must be 1-64 letters, numbers, dots, underscores, or hyphens")
	}
	if query == "" {
		return nil, errors.New("source query is required")
	}
	definition, err := json.Marshal(searchSourceDefinition{Query: query})
	if err != nil {
		return nil, err
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	stored, err := c.SaveDiscoverySource(ctx, corpus.DiscoverySource{
		Name: name, Kind: "search", Definition: string(definition), Enabled: true,
	})
	if err != nil {
		return nil, err
	}
	return sourceResult(stored), nil
}

func (s *Service) ListSources(ctx context.Context) (*cli.SourceListResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	sources, err := c.ListDiscoverySources(ctx)
	if err != nil {
		return nil, err
	}
	result := &cli.SourceListResult{Sources: make([]cli.SourceResult, len(sources))}
	for i := range sources {
		result.Sources[i] = *sourceResult(&sources[i])
	}
	return result, nil
}

func sourceResult(source *corpus.DiscoverySource) *cli.SourceResult {
	return &cli.SourceResult{
		Name: source.Name, Kind: source.Kind, Definition: source.Definition, Enabled: source.Enabled,
	}
}

func (s *Service) Crawl(ctx context.Context, name string, opts cli.CrawlOptions) (_ *cli.CrawlResult, resultErr error) {
	if opts.Since <= 0 || opts.Budget <= 0 || opts.Budget > 5000 {
		return nil, errors.New("invalid crawl since or budget")
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	source, err := c.GetDiscoverySource(ctx, name)
	if err != nil {
		return nil, err
	}
	if source == nil || !source.Enabled {
		return nil, fmt.Errorf("discovery source %q not found or disabled", name)
	}
	if source.Kind != "search" {
		return nil, fmt.Errorf("source %q has unsupported kind %q", name, source.Kind)
	}
	var definition searchSourceDefinition
	if err := json.Unmarshal([]byte(source.Definition), &definition); err != nil {
		return nil, fmt.Errorf("decode source %q: %w", name, err)
	}
	reader, err := s.githubReader()
	if err != nil {
		return nil, err
	}
	searcher, ok := reader.(github.RepositorySearcher)
	if !ok {
		return nil, errors.New("configured GitHub reader does not support repository search")
	}

	run, err := c.StartRun(ctx, "crawl")
	if err != nil {
		return nil, err
	}
	defer func() {
		if resultErr == nil {
			return
		}
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = c.FailRun(cleanup, run.ID, resultErr.Error())
	}()

	now := time.Now().UTC().Truncate(time.Second)
	start := now.Add(-opts.Since)
	qualifier := discovery.Created
	checkpointKey := "source:" + source.Name
	if checkpoint, exists, err := c.GetTime(ctx, checkpointKey); err != nil {
		return nil, err
	} else if exists {
		start = checkpoint.Add(-6 * time.Hour)
		qualifier = discovery.Updated
	}
	budgeted := &budgetedRepositorySearch{searcher: searcher, budget: opts.Budget}
	partitioner := discovery.SearchPartitioner{Searcher: budgeted}
	windows, err := partitioner.Partition(ctx, definition.Query, start, now, qualifier)
	if err != nil {
		return nil, err
	}
	discovered := 0
	seen := make(map[string]struct{})
	for _, window := range windows {
		partition := corpus.SourcePartition{
			SourceID: source.ID, Key: fmt.Sprintf("%s:%d:%d", qualifier, window.Start.Unix(), window.End.Unix()),
			Query: window.Query, Qualifier: string(qualifier), Start: window.Start, End: window.End,
			Total: window.Total, Incomplete: window.Incomplete, Unsplittable: window.Unsplittable, ObservedAt: now,
		}
		if err := c.RecordSourcePartition(ctx, partition); err != nil {
			return nil, err
		}
		if window.Unsplittable || window.Incomplete {
			return nil, fmt.Errorf("search window %s is incomplete or exceeds GitHub's result ceiling", partition.Key)
		}
		pages := (window.Total + 99) / 100
		for page := 1; page <= pages; page++ {
			response, err := budgeted.page(ctx, window.Query, page, 100)
			if err != nil {
				return nil, err
			}
			for _, repo := range response.Items {
				identity := strings.ToLower(repo.Owner + "/" + repo.Name)
				if repo.NodeID != "" {
					identity = repo.NodeID
				}
				if _, duplicate := seen[identity]; duplicate {
					continue
				}
				seen[identity] = struct{}{}
				payload, err := json.Marshal(repo)
				if err != nil {
					return nil, err
				}
				_, err = c.UpsertRepository(ctx, corpusRepoFromGitHub(repo), string(payload))
				if err != nil {
					return nil, err
				}
				_, _, err = c.EnqueueFrontierItem(ctx, corpus.FrontierItem{
					WorkKey:     fmt.Sprintf("repository:%s/%s:threads", repo.Owner, repo.Name),
					SubjectKind: "repository", Owner: repo.Owner, Repo: repo.Name, Facet: "threads",
					Priority: 10, Reason: "discovered by " + source.Name, Source: source.Name,
				})
				if err != nil {
					return nil, err
				}
				discovered++
			}
		}
	}
	if err := c.SetTime(ctx, checkpointKey, now); err != nil {
		return nil, err
	}
	stats, _ := json.Marshal(map[string]int{"windows": len(windows), "repositories": discovered, "requests": budgeted.used})
	if err := c.FinishRun(ctx, run.ID, string(stats)); err != nil {
		return nil, err
	}
	return &cli.CrawlResult{
		Source: source.Name, Windows: len(windows), Repositories: discovered,
		Requests: budgeted.used, Checkpoint: now.Format(time.RFC3339),
	}, nil
}

type budgetedRepositorySearch struct {
	searcher github.RepositorySearcher
	budget   int
	used     int
}

func (s *budgetedRepositorySearch) Search(ctx context.Context, query string) (discovery.SearchResponse, error) {
	result, err := s.page(ctx, query, 1, 1)
	if err != nil {
		return discovery.SearchResponse{}, err
	}
	return discovery.SearchResponse{Total: result.Total, Incomplete: result.Incomplete}, nil
}

func (s *budgetedRepositorySearch) page(ctx context.Context, query string, page, perPage int) (github.RepositorySearchResult, error) {
	if s.used >= s.budget {
		return github.RepositorySearchResult{}, fmt.Errorf("crawl API budget of %d requests exhausted", s.budget)
	}
	s.used++
	return s.searcher.SearchRepositories(ctx, github.RepositorySearchOptions{
		Query: query, PageOptions: github.PageOptions{Page: page, PerPage: perPage},
	})
}
