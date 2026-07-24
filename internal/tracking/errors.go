package tracking

import "errors"

// ErrImportConflict reports two different immutable or equal-timestamp
// revisions carrying the same durable ID.
var ErrImportConflict = errors.New("tracking import conflict")

// ErrExportTruncated indicates that a requested bundle limit cannot contain a
// complete portable snapshot.
var ErrExportTruncated = errors.New("tracking export would be incomplete")
