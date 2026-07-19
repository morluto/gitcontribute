package corpus

import (
	"database/sql"
	"errors"
)

type sqlCloser interface {
	Close() error
}

func closeSQLOnReturn(closer sqlCloser, returnErr *error) {
	if err := closer.Close(); err != nil && *returnErr == nil {
		*returnErr = err
	}
}

func rollbackSQLOnReturn(tx *sql.Tx, returnErr *error) {
	err := tx.Rollback()
	if err != nil && !errors.Is(err, sql.ErrTxDone) && *returnErr == nil {
		*returnErr = err
	}
}
