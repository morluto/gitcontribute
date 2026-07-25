package corpus

import (
	"context"
	"errors"
	"fmt"
)

const (
	schemaApplicationID = 1195594818 // "GCTB"
	schemaLineage       = "gitcontribute-canonical-v1"
)

// SupportedSchemaVersion returns the newest embedded corpus schema without
// opening a database or inspecting the filesystem.
func SupportedSchemaVersion() (int64, error) {
	return latestSchemaVersion()
}

// SupportedSchemaLineage returns the durable identity required on every
// corpus supported by this binary.
func SupportedSchemaLineage() string {
	return schemaLineage
}

func (c *Corpus) establishSchemaIdentity(ctx context.Context) error {
	var applicationID int
	if err := c.db.QueryRowContext(ctx, `PRAGMA application_id`).Scan(&applicationID); err != nil {
		return fmt.Errorf("inspect corpus identity before initialization: %w", err)
	}
	if applicationID == schemaApplicationID {
		return c.resetEmptyMigrationBootstrap(ctx)
	}
	target, err := latestSchemaVersion()
	if err != nil {
		return err
	}
	if applicationID != 0 {
		return &IncompatibleSchemaError{Target: target}
	}
	var objectCount int
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' AND name <> 'goose_db_version'`).Scan(&objectCount); err != nil {
		return fmt.Errorf("inspect corpus objects before initialization: %w", err)
	}
	if objectCount != 0 {
		return &IncompatibleSchemaError{Target: target}
	}
	if _, err := c.db.ExecContext(ctx, fmt.Sprintf(`PRAGMA application_id = %d`, schemaApplicationID)); err != nil {
		return fmt.Errorf("establish corpus identity: %w", err)
	}
	return nil
}

func (c *Corpus) resetEmptyMigrationBootstrap(ctx context.Context) error {
	var tableCount int
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'goose_db_version'`).Scan(&tableCount); err != nil {
		return fmt.Errorf("inspect migration bootstrap: %w", err)
	}
	if tableCount == 0 {
		return nil
	}
	var versionCount int
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM goose_db_version`).Scan(&versionCount); err != nil {
		return fmt.Errorf("inspect migration bootstrap versions: %w", err)
	}
	if versionCount != 0 {
		return nil
	}
	var objectCount int
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' AND name <> 'goose_db_version'`).Scan(&objectCount); err != nil {
		return fmt.Errorf("inspect interrupted initialization objects: %w", err)
	}
	if objectCount != 0 {
		return errors.New("interrupted corpus initialization contains schema objects without a migration version")
	}
	if _, err := c.db.ExecContext(ctx, `DROP TABLE goose_db_version`); err != nil {
		return fmt.Errorf("reset interrupted migration bootstrap: %w", err)
	}
	return nil
}
