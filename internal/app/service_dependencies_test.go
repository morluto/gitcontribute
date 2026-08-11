package app

import (
	"time"

	"github.com/morluto/gitcontribute/internal/deepwiki"
	"github.com/morluto/gitcontribute/internal/discovery"
	"github.com/morluto/gitcontribute/internal/github"
)

func (s *Service) SetClock(clock func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clock = clock
}

func (s *Service) SetGitHubReader(reader github.Reader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ghReader = reader
}

func (s *Service) SetDeepWikiReader(reader deepwiki.Reader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deepWikiReader = reader
}

func (s *Service) SetArchiveFetcher(fetcher discovery.ArchiveFetcher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.archiveFetcher = fetcher
}
