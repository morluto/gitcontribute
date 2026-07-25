package corpus

import (
	"errors"
	"strings"
)

const (
	maxBatchReadItems = 100
	maxBatchFacets    = 32
)

type rowScanner interface {
	Scan(dest ...any) error
}

func validateThreadFacetBatch(threadIDs []int64, facets []string) error {
	return validateFacetBatch(threadIDs, facets, "thread")
}

func validateFacetBatch(entityIDs []int64, facets []string, entity string) error {
	if len(entityIDs) > maxBatchReadItems {
		return errors.New(entity + " batch cannot exceed 100 items")
	}
	if len(facets) > maxBatchFacets {
		return errors.New("facet batch cannot exceed 32 items")
	}
	return nil
}

func sqlPlaceholders(count int) string {
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}
