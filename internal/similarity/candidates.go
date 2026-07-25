package similarity

import (
	"context"
	"strings"

	"github.com/morluto/gitcontribute/internal/domain"
)

type duplicateCandidateIndex struct {
	prepared   []PreparedDuplicate
	byTitle    map[string][]int
	byExplicit map[int][]int
}

type duplicateCandidateCursor struct {
	index      duplicateCandidateIndex
	marked     []uint32
	generation uint32
	candidates []int
}

// VisitCandidatePairs visits every pair that can reach the duplicate-v1
// threshold. A pair without an explicit reference or shared title token can
// score at most 0.15 from body, labels, and author, below the 0.30 threshold.
func (r DuplicateRule) VisitCandidatePairs(ctx context.Context, prepared []PreparedDuplicate, visit func(left, right int)) error {
	index, err := r.candidateIndex(ctx, prepared)
	if err != nil {
		return err
	}
	cursor := index.cursor()
	var visited uint64
	for source := range prepared {
		if err := ctx.Err(); err != nil {
			return err
		}
		for _, target := range cursor.after(source) {
			if visited%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			visit(source, target)
			visited++
		}
	}
	return nil
}

func (DuplicateRule) candidateIndex(ctx context.Context, prepared []PreparedDuplicate) (duplicateCandidateIndex, error) {
	index := duplicateCandidateIndex{
		prepared:   prepared,
		byTitle:    make(map[string][]int),
		byExplicit: make(map[int][]int),
	}

	exactRefs := make(map[threadRefKey][]int, len(prepared))
	untypedRefs := make(map[threadRefKey][]int, len(prepared))
	for i, candidate := range prepared {
		if err := ctx.Err(); err != nil {
			return duplicateCandidateIndex{}, err
		}
		for _, token := range candidate.title {
			index.byTitle[token] = append(index.byTitle[token], i)
		}
		exact := duplicateThreadRefKey(candidate.ref)
		exactRefs[exact] = append(exactRefs[exact], i)
		exact.kind = ""
		untypedRefs[exact] = append(untypedRefs[exact], i)
	}

	for source, candidate := range prepared {
		if err := ctx.Err(); err != nil {
			return duplicateCandidateIndex{}, err
		}
		for refIndex, ref := range candidate.references {
			if refIndex%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return duplicateCandidateIndex{}, err
				}
			}
			key := duplicateThreadRefKey(ref)
			targets := exactRefs[key]
			if ref.Kind == "" {
				targets = untypedRefs[key]
			}
			for _, target := range targets {
				if source == target {
					continue
				}
				index.byExplicit[source] = append(index.byExplicit[source], target)
				index.byExplicit[target] = append(index.byExplicit[target], source)
			}
		}
	}
	return index, nil
}

func (index duplicateCandidateIndex) cursor() duplicateCandidateCursor {
	return duplicateCandidateCursor{index: index, marked: make([]uint32, len(index.prepared))}
}

func (cursor *duplicateCandidateCursor) after(source int) []int {
	if source < 0 || source >= len(cursor.index.prepared)-1 {
		return nil
	}
	cursor.generation++
	if cursor.generation == 0 {
		clear(cursor.marked)
		cursor.generation = 1
	}
	cursor.candidates = cursor.candidates[:0]
	add := func(target int) {
		if target <= source || cursor.marked[target] == cursor.generation {
			return
		}
		cursor.marked[target] = cursor.generation
		cursor.candidates = append(cursor.candidates, target)
	}

	for _, token := range cursor.index.prepared[source].title {
		for _, target := range cursor.index.byTitle[token] {
			add(target)
		}
	}
	for _, target := range cursor.index.byExplicit[source] {
		add(target)
	}
	return cursor.candidates
}

type threadRefKey struct {
	owner  string
	repo   string
	kind   domain.ThreadKind
	number int
}

func duplicateThreadRefKey(ref ThreadRef) threadRefKey {
	return threadRefKey{
		owner: strings.ToLower(ref.Repo.Owner),
		repo:  strings.ToLower(ref.Repo.Repo),
		kind:  ref.Kind, number: ref.Number,
	}
}
