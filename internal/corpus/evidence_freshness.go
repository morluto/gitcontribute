package corpus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/morluto/gitcontribute/internal/evidence"
)

var _ evidence.RevisionReader = (*Corpus)(nil)

// CurrentSourceRevision returns the winning local revision for one evidence
// subject. It performs only SQLite reads.
func (c *Corpus) CurrentSourceRevision(ctx context.Context, subject evidence.SourceSubject) (*evidence.SourceRevision, error) {
	if err := subject.Validate(); err != nil {
		return nil, err
	}
	switch subject.Kind {
	case evidence.SourceSubjectRepository:
		return c.currentRepositorySourceRevision(ctx, subject)
	case evidence.SourceSubjectThread:
		return c.currentThreadSourceRevision(ctx, subject)
	case evidence.SourceSubjectFacet, evidence.SourceSubjectGuidance:
		return c.currentFacetSourceRevision(ctx, subject)
	default:
		return nil, fmt.Errorf("unsupported source subject kind %q", subject.Kind)
	}
}

func (c *Corpus) currentRepositorySourceRevision(ctx context.Context, subject evidence.SourceSubject) (*evidence.SourceRevision, error) {
	var sourceUpdatedAt, sequence, observedAt int64
	err := c.db.QueryRowContext(ctx, `
		SELECT r.source_updated_at, r.observation_sequence,
		       COALESCE((SELECT o.observed_at FROM repository_observations o
		                 WHERE o.repository_id=r.id AND o.source_updated_at=r.source_updated_at
		                   AND o.observation_sequence=r.observation_sequence LIMIT 1), r.updated_at)
		FROM repositories r
		WHERE r.owner=? COLLATE NOCASE AND r.name=? COLLATE NOCASE
	`, subject.Owner, subject.Repo).Scan(&sourceUpdatedAt, &sequence, &observedAt)
	return scannedSourceRevision(subject, sourceUpdatedAt, sequence, observedAt, err)
}

func (c *Corpus) currentThreadSourceRevision(ctx context.Context, subject evidence.SourceSubject) (*evidence.SourceRevision, error) {
	var sourceUpdatedAt, sequence, observedAt int64
	err := c.db.QueryRowContext(ctx, `
		SELECT t.source_updated_at, t.observation_sequence,
		       COALESCE((SELECT o.observed_at FROM thread_observations o
		                 WHERE o.thread_id=t.id AND o.source_updated_at=t.source_updated_at
		                   AND o.observation_sequence=t.observation_sequence LIMIT 1), t.updated_at)
		FROM threads t
		JOIN repositories r ON r.id=t.repository_id
		WHERE r.owner=? COLLATE NOCASE AND r.name=? COLLATE NOCASE
		  AND t.kind=? AND t.number=?
	`, subject.Owner, subject.Repo, subject.ThreadKind, subject.Number).Scan(&sourceUpdatedAt, &sequence, &observedAt)
	return scannedSourceRevision(subject, sourceUpdatedAt, sequence, observedAt, err)
}

func (c *Corpus) currentFacetSourceRevision(ctx context.Context, subject evidence.SourceSubject) (*evidence.SourceRevision, error) {
	var repoID int64
	err := c.db.QueryRowContext(ctx, `
		SELECT id FROM repositories WHERE owner=? COLLATE NOCASE AND name=? COLLATE NOCASE
	`, subject.Owner, subject.Repo).Scan(&repoID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve source repository: %w", err)
	}

	var threadID sql.NullInt64
	if subject.ThreadKind != "" {
		threadID.Valid = true
		err = c.db.QueryRowContext(ctx, `
			SELECT id FROM threads WHERE repository_id=? AND kind=? AND number=?
		`, repoID, subject.ThreadKind, subject.Number).Scan(&threadID.Int64)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("resolve source thread: %w", err)
		}
	}
	facet := subject.Facet
	if subject.Kind == evidence.SourceSubjectGuidance {
		facet = evidence.GuidanceFacet
	}
	var sourceUpdatedAt, sequence, observedAt int64
	err = c.db.QueryRowContext(ctx, `
		SELECT source_updated_at, observation_sequence, updated_at
		FROM facet_coverage
		WHERE repository_id=? AND COALESCE(thread_id, -1)=COALESCE(?, -1) AND facet=?
	`, repoID, threadID, facet).Scan(&sourceUpdatedAt, &sequence, &observedAt)
	return scannedSourceRevision(subject, sourceUpdatedAt, sequence, observedAt, err)
}

func scannedSourceRevision(subject evidence.SourceSubject, sourceUpdatedAt, sequence, observedAt int64, err error) (*evidence.SourceRevision, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &evidence.SourceRevision{
		Subject: subject, SourceUpdatedAt: scanTime(sourceUpdatedAt),
		ObservationSequence: sequence, ObservedAt: scanTime(observedAt),
	}, nil
}
