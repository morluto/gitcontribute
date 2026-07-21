package corpus

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// BusyError reports that another GitContribute process holds an
// incompatible corpus lease. Operations fail fast instead of appearing hung.
type BusyError struct {
	Path      string
	Operation string
}

func (e *BusyError) Error() string {
	return fmt.Sprintf("corpus is in use by another process; cannot %s: %s", e.Operation, e.Path)
}

type corpusLease struct {
	lock *flock.Flock
}

func acquireCorpusLease(path string, exclusive bool, operation string) (*corpusLease, error) {
	lockPath, ok := corpusLockPath(path)
	if !ok {
		return &corpusLease{}, nil
	}
	if err := ensureCorpusLeaseFile(path); err != nil {
		return nil, err
	}
	lock := flock.New(lockPath)
	var (
		acquired bool
		err      error
	)
	if exclusive {
		acquired, err = lock.TryLock()
	} else {
		acquired, err = lock.TryRLock()
	}
	if err != nil {
		return nil, fmt.Errorf("acquire corpus lease for %s: %w", operation, err)
	}
	if !acquired {
		return nil, &BusyError{Path: path, Operation: operation}
	}
	return &corpusLease{lock: lock}, nil
}

func ensureCorpusLeaseFile(path string) error {
	lockPath, ok := corpusLockPath(path)
	if !ok {
		return nil
	}
	directory, base := filepath.Split(lockPath)
	root, err := os.OpenRoot(directory)
	if err != nil {
		return fmt.Errorf("open corpus lease directory: %w", err)
	}
	file, err := root.OpenFile(base, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return errors.Join(fmt.Errorf("prepare corpus lease: %w", err), root.Close())
	}
	return errors.Join(file.Close(), root.Close())
}

// CheckExclusiveAccess fails fast when another cooperating process holds a
// corpus lease. It makes no database changes and does not reserve the lease for
// later work; the mutating operation must acquire it again.
func CheckExclusiveAccess(path, operation string) error {
	lease, err := acquireCorpusLease(path, true, operation)
	if err != nil {
		return err
	}
	return lease.release()
}

func corpusLockPath(path string) (string, bool) {
	filePath, _, inspectable, err := schemaInspectionTarget(path)
	if err != nil || !inspectable {
		return "", false
	}
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return filePath + ".lock", true
	}
	return abs + ".lock", true
}

// acquireSharedCorpusLeaseIfPresent acquires a shared flock when the lock sidecar
// already exists, without creating one. It lets InspectSchema coordinate with
// active readers/writers while remaining side-effect free for missing or
// damaged files that have no lock.
func acquireSharedCorpusLeaseIfPresent(path, operation string) (*corpusLease, error) {
	lockPath, ok := corpusLockPath(path)
	if !ok {
		return &corpusLease{}, nil
	}
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		return &corpusLease{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("inspect corpus lease for %s: %w", operation, err)
	}
	lock := flock.New(lockPath)
	acquired, err := lock.TryRLock()
	if err != nil {
		return nil, fmt.Errorf("acquire corpus lease for %s: %w", operation, err)
	}
	if !acquired {
		return nil, &BusyError{Path: path, Operation: operation}
	}
	return &corpusLease{lock: lock}, nil
}

func (l *corpusLease) release() error {
	if l == nil || l.lock == nil {
		return nil
	}
	return l.lock.Unlock()
}

func releaseLeaseOnReturn(lease *corpusLease, returnErr *error) {
	*returnErr = errors.Join(*returnErr, lease.release())
}
