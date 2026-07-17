package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/discovery"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
)

type searchSourceDefinition struct {
	Query string `json:"query"`
}

type repoSourceDefinition struct {
	Repositories []domain.RepoRef `json:"repositories"`
}

type ghArchiveSourceDefinition struct {
	Events []string `json:"events,omitempty"`
}

var sourceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func validateSourceName(name string) error {
	name = strings.TrimSpace(name)
	if !sourceNamePattern.MatchString(name) {
		return errors.New("source name must be 1-64 letters, numbers, dots, underscores, or hyphens")
	}
	return nil
}

func (s *Service) AddSearchSource(ctx context.Context, name, query string) (*cli.SourceResult, error) {
	name = strings.TrimSpace(name)
	query = strings.TrimSpace(query)
	if err := validateSourceName(name); err != nil {
		return nil, err
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

func (s *Service) AddRepoSource(ctx context.Context, name string, refs []cli.RepoRef) (*cli.SourceResult, error) {
	name = strings.TrimSpace(name)
	if err := validateSourceName(name); err != nil {
		return nil, err
	}
	if len(refs) == 0 {
		return nil, errors.New("at least one repository is required")
	}
	domainRefs := make([]domain.RepoRef, len(refs))
	for i, ref := range refs {
		r := domain.RepoRef{Owner: ref.Owner, Repo: ref.Repo}
		if err := r.Validate(); err != nil {
			return nil, fmt.Errorf("invalid repository %s: %w", ref, err)
		}
		domainRefs[i] = r
	}
	definition, err := json.Marshal(repoSourceDefinition{Repositories: domainRefs})
	if err != nil {
		return nil, err
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	stored, err := c.SaveDiscoverySource(ctx, corpus.DiscoverySource{
		Name: name, Kind: "repos", Definition: string(definition), Enabled: true,
	})
	if err != nil {
		return nil, err
	}
	return sourceResult(stored), nil
}

func (s *Service) AddGHArchiveSource(ctx context.Context, name string, events []string) (*cli.SourceResult, error) {
	name = strings.TrimSpace(name)
	if err := validateSourceName(name); err != nil {
		return nil, err
	}
	for _, ev := range events {
		if !discovery.IsKnownEventType(ev) {
			return nil, fmt.Errorf("unknown GH Archive event type %q", ev)
		}
	}
	definition, err := json.Marshal(ghArchiveSourceDefinition{Events: events})
	if err != nil {
		return nil, err
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	stored, err := c.SaveDiscoverySource(ctx, corpus.DiscoverySource{
		Name: name, Kind: "gharchive", Definition: string(definition), Enabled: true,
	})
	if err != nil {
		return nil, err
	}
	return sourceResult(stored), nil
}

func (s *Service) ShowSource(ctx context.Context, name string) (*cli.SourceResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	source, err := c.GetDiscoverySource(ctx, name)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return nil, fmt.Errorf("source %q not found", name)
	}
	return sourceResult(source), nil
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

func (s *Service) Crawl(ctx context.Context, name string, opts cli.CrawlOptions) (*cli.CrawlResult, error) {
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
	switch source.Kind {
	case "search":
		return s.crawlSearchSource(ctx, c, source, opts)
	case "repos":
		return s.crawlRepoSource(ctx, c, source, opts)
	case "gharchive":
		return s.crawlGHArchiveSource(ctx, c, source, opts)
	default:
		return nil, fmt.Errorf("source %q has unsupported kind %q", name, source.Kind)
	}
}

func (s *Service) crawlSearchSource(ctx context.Context, c *corpus.Corpus, source *corpus.DiscoverySource, opts cli.CrawlOptions) (_ *cli.CrawlResult, resultErr error) {
	var definition searchSourceDefinition
	if err := json.Unmarshal([]byte(source.Definition), &definition); err != nil {
		return nil, fmt.Errorf("decode source %q: %w", source.Name, err)
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

	now := s.now().UTC().Truncate(time.Second)
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

func (s *Service) crawlRepoSource(ctx context.Context, c *corpus.Corpus, source *corpus.DiscoverySource, opts cli.CrawlOptions) (_ *cli.CrawlResult, resultErr error) {
	var definition repoSourceDefinition
	if err := json.Unmarshal([]byte(source.Definition), &definition); err != nil {
		return nil, fmt.Errorf("decode source %q: %w", source.Name, err)
	}
	if len(definition.Repositories) == 0 {
		return nil, fmt.Errorf("source %q has no repositories", source.Name)
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

	now := s.now().UTC().Truncate(time.Second)
	processed := 0
	for i, ref := range definition.Repositories {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if i >= opts.Budget {
			break
		}
		payload, err := json.Marshal(repoSourceObservation{Source: source.Name, Ref: ref})
		if err != nil {
			return nil, err
		}
		repo := corpus.Repository{
			Owner: ref.Owner,
			Name:  ref.Repo,
			// Explicit sources have no network freshness; use a zero
			// source_updated_at so canonical GitHub syncs always win.
			SourceUpdatedAt: time.Time{},
		}
		if _, err := c.UpsertRepository(ctx, repo, string(payload)); err != nil {
			return nil, err
		}
		if _, _, err := c.EnqueueFrontierItem(ctx, corpus.FrontierItem{
			WorkKey:     fmt.Sprintf("repository:%s/%s:threads", ref.Owner, ref.Repo),
			SubjectKind: "repository", Owner: ref.Owner, Repo: ref.Repo, Facet: "threads",
			Priority: 10, Reason: "explicit source " + source.Name, Source: source.Name,
		}); err != nil {
			return nil, err
		}
		processed++
	}

	stats, _ := json.Marshal(map[string]int{"repositories": processed})
	if err := c.FinishRun(ctx, run.ID, string(stats)); err != nil {
		return nil, err
	}
	return &cli.CrawlResult{
		Source: source.Name, Repositories: processed,
		Requests: 0, Checkpoint: now.Format(time.RFC3339),
	}, nil
}

type repoSourceObservation struct {
	Source string         `json:"source"`
	Ref    domain.RepoRef `json:"ref"`
}

func (s *Service) crawlGHArchiveSource(ctx context.Context, c *corpus.Corpus, source *corpus.DiscoverySource, opts cli.CrawlOptions) (_ *cli.CrawlResult, resultErr error) {
	var definition ghArchiveSourceDefinition
	if err := json.Unmarshal([]byte(source.Definition), &definition); err != nil {
		return nil, fmt.Errorf("decode source %q: %w", source.Name, err)
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

	now := s.now().UTC()
	startHour, endHour := discovery.ArchiveHourRange(opts.Since, now)
	reader := discovery.NewArchiveReader(definition.Events, nil)
	reader.MaxTotalBytes = 1 << 30 // 1 GiB decompressed per hour

	fetcher := s.getArchiveFetcher()

	stats := ghArchiveStats{}
	repoSeen := make(map[domain.RepoRef]struct{})
	threadSeen := make(map[archiveThreadKey]struct{})
	repoIDByRef := make(map[domain.RepoRef]int64)

	for hour := startHour; !hour.After(endHour); hour = hour.Add(time.Hour) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if stats.requests >= opts.Budget {
			break
		}
		stats.considered++
		hourKey := discovery.HourKey(hour)
		key := archiveImportKey(source, hour)
		imported, err := c.IsImported(ctx, key)
		if err != nil {
			return nil, err
		}
		if imported {
			stats.skipped++
			stats.advanceCheckpoint(hour)
			continue
		}

		stats.requests++
		body, err := fetcher.Fetch(ctx, hour)
		if err != nil {
			stats.recordFailure()
			_ = c.RecordRunEvent(ctx, run.ID, "warning", fmt.Sprintf("fetch %s: %v", hourKey, err))
			continue
		}

		hourSigs := archiveHourSigs{repoSigs: make(map[domain.RepoRef]discovery.Signal), threadSigs: make(map[archiveThreadKey]discovery.Signal)}
		err = reader.Read(ctx, hour, body, func(sig discovery.Signal) error {
			stats.events++
			hourSigs.repoSigs[sig.Repo] = sig
			if sig.ThreadNumber > 0 {
				k := archiveThreadKey{ref: sig.Repo, kind: string(sig.ThreadKind), number: sig.ThreadNumber}
				hourSigs.threadSigs[k] = sig
			}
			return nil
		})
		_ = body.Close()
		if err != nil {
			stats.recordFailure()
			_ = c.RecordRunEvent(ctx, run.ID, "warning", fmt.Sprintf("read %s: %v", hourKey, err))
			continue
		}

		if err := s.flushArchiveHour(ctx, c, source, hourSigs, repoSeen, threadSeen, repoIDByRef); err != nil {
			return nil, err
		}
		if err := c.MarkImported(ctx, key); err != nil {
			return nil, err
		}
		stats.imported++
		stats.advanceCheckpoint(hour)
	}

	stats.repositories = len(repoSeen)
	stats.threads = len(threadSeen)

	statsJSON, _ := json.Marshal(map[string]any{
		"hours":        stats.hours(),
		"events":       stats.events,
		"repositories": stats.repositories,
		"threads":      stats.threads,
		"requests":     stats.requests,
		"imported":     stats.imported,
		"skipped":      stats.skipped,
		"failures":     stats.failures,
	})
	if stats.failures == 0 {
		if err := c.FinishRun(ctx, run.ID, string(statsJSON)); err != nil {
			return nil, err
		}
	} else if stats.imported == 0 && stats.skipped == 0 {
		if err := c.FailRun(ctx, run.ID, fmt.Sprintf("%d archive hour(s) failed", stats.failures)); err != nil {
			return nil, err
		}
	} else if err := c.FinishRunPartial(ctx, run.ID, string(statsJSON), fmt.Sprintf("%d archive hour(s) failed", stats.failures)); err != nil {
		return nil, err
	}
	checkpoint := ""
	if !stats.checkpoint.IsZero() {
		checkpoint = stats.checkpoint.Format(time.RFC3339)
	}
	return &cli.CrawlResult{
		Source:       source.Name,
		Windows:      stats.hours(),
		Repositories: stats.repositories,
		Threads:      stats.threads,
		Events:       stats.events,
		Requests:     stats.requests,
		Imported:     stats.imported,
		Skipped:      stats.skipped,
		Failures:     stats.failures,
		Checkpoint:   checkpoint,
	}, nil
}

func archiveImportKey(source *corpus.DiscoverySource, hour time.Time) string {
	definitionHash := sha256.Sum256([]byte(source.Definition))
	return fmt.Sprintf("gharchive:%s:%x:%s", strings.ToLower(source.Name), definitionHash[:8], discovery.HourKey(hour))
}

type archiveHourSigs struct {
	repoSigs   map[domain.RepoRef]discovery.Signal
	threadSigs map[archiveThreadKey]discovery.Signal
}

type archiveThreadKey struct {
	ref    domain.RepoRef
	kind   string
	number int
}

type ghArchiveStats struct {
	considered    int
	checkpoint    time.Time
	checkpointGap bool
	events        int
	repositories  int
	threads       int
	requests      int
	imported      int
	skipped       int
	failures      int
}

func (g *ghArchiveStats) hours() int {
	return g.considered
}

func (g *ghArchiveStats) recordFailure() {
	g.failures++
	g.checkpointGap = true
}

func (g *ghArchiveStats) advanceCheckpoint(hour time.Time) {
	if !g.checkpointGap {
		g.checkpoint = hour
	}
}

func (s *Service) flushArchiveHour(ctx context.Context, c *corpus.Corpus, source *corpus.DiscoverySource, hourSigs archiveHourSigs, repoSeen map[domain.RepoRef]struct{}, threadSeen map[archiveThreadKey]struct{}, repoIDByRef map[domain.RepoRef]int64) error {
	// Upsert repositories in deterministic order so tests are stable.
	repoRefs := make([]domain.RepoRef, 0, len(hourSigs.repoSigs))
	for ref := range hourSigs.repoSigs {
		repoRefs = append(repoRefs, ref)
	}
	sort.Slice(repoRefs, func(i, j int) bool {
		if repoRefs[i].Owner != repoRefs[j].Owner {
			return repoRefs[i].Owner < repoRefs[j].Owner
		}
		return repoRefs[i].Repo < repoRefs[j].Repo
	})
	for _, ref := range repoRefs {
		sig := hourSigs.repoSigs[ref]
		existing, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
		if err != nil {
			return err
		}
		repo := mergeArchiveRepo(sig, existing)
		payload, err := json.Marshal(sig)
		if err != nil {
			return err
		}
		upserted, err := c.UpsertRepository(ctx, repo, string(payload))
		if err != nil {
			return err
		}
		repoIDByRef[ref] = upserted.ID
		repoSeen[ref] = struct{}{}
		_, _, err = c.EnqueueFrontierItem(ctx, corpus.FrontierItem{
			WorkKey:     fmt.Sprintf("repository:%s/%s:threads", ref.Owner, ref.Repo),
			SubjectKind: "repository", Owner: ref.Owner, Repo: ref.Repo, Facet: "threads",
			Priority: 10, Reason: "discovered by " + source.Name, Source: source.Name,
		})
		if err != nil {
			return err
		}
	}

	threadKeys := make([]archiveThreadKey, 0, len(hourSigs.threadSigs))
	for k := range hourSigs.threadSigs {
		threadKeys = append(threadKeys, k)
	}
	sort.Slice(threadKeys, func(i, j int) bool {
		if threadKeys[i].ref != threadKeys[j].ref {
			if threadKeys[i].ref.Owner != threadKeys[j].ref.Owner {
				return threadKeys[i].ref.Owner < threadKeys[j].ref.Owner
			}
			return threadKeys[i].ref.Repo < threadKeys[j].ref.Repo
		}
		if threadKeys[i].kind != threadKeys[j].kind {
			return threadKeys[i].kind < threadKeys[j].kind
		}
		return threadKeys[i].number < threadKeys[j].number
	})
	for _, key := range threadKeys {
		sig := hourSigs.threadSigs[key]
		repoID, ok := repoIDByRef[key.ref]
		if !ok {
			return fmt.Errorf("missing repository id for thread %s/%s#%d", key.ref.Owner, key.ref.Repo, key.number)
		}
		existing, err := c.GetThread(ctx, repoID, key.kind, key.number)
		if err != nil {
			return err
		}
		thread := mergeArchiveThread(sig, repoID, existing)
		payload, err := json.Marshal(sig)
		if err != nil {
			return err
		}
		if _, err := c.UpsertThread(ctx, thread, string(payload)); err != nil {
			return err
		}
		threadSeen[key] = struct{}{}
	}
	return nil
}

func mergeArchiveRepo(sig discovery.Signal, existing *corpus.Repository) corpus.Repository {
	r := corpus.Repository{
		Owner:      sig.Repo.Owner,
		Name:       sig.Repo.Repo,
		ExternalID: fmt.Sprintf("%d", sig.RepoID),
	}
	if existing == nil {
		return r
	}
	if !existing.SourceUpdatedAt.IsZero() {
		r = *existing
		r.SourceUpdatedAt = time.Time{}
		return r
	}
	r = *existing
	if sig.RepoID != 0 {
		r.ExternalID = fmt.Sprintf("%d", sig.RepoID)
	}
	r.SourceUpdatedAt = time.Time{}
	return r
}

func mergeArchiveThread(sig discovery.Signal, repoID int64, existing *corpus.Thread) corpus.Thread {
	t := corpus.Thread{
		RepositoryID: repoID,
		Kind:         string(sig.ThreadKind),
		Number:       sig.ThreadNumber,
	}
	if existing != nil {
		if !existing.SourceUpdatedAt.IsZero() {
			t = *existing
			t.SourceUpdatedAt = time.Time{}
			return t
		}
		t = *existing
		t.RepositoryID = repoID
		t.Kind = string(sig.ThreadKind)
		t.Number = sig.ThreadNumber
	}
	if sig.ThreadState != "" {
		t.State = string(sig.ThreadState)
	}
	if sig.ThreadTitle != "" {
		t.Title = sig.ThreadTitle
	}
	if sig.ThreadAuthor != "" {
		t.Author = sig.ThreadAuthor
	}
	if sig.ThreadKind == domain.PullRequestKind && sig.Merged {
		t.Merged = true
	}
	t.SourceUpdatedAt = time.Time{}
	return t
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

// Ensure the fetcher interface is satisfied by the injected type.
var _ discovery.ArchiveFetcher = (*discovery.ArchiveClient)(nil)
