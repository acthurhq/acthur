package migrations

import (
	"errors"
	"fmt"
)

// ResetSteps are the three steps Reset composes — down-all migrations,
// up-all migrations, then seed — injectable so orchestration order and
// error handling are unit-testable without a live postgres connection. Any
// nil field falls back to this package's real DownAll/Migrate/Seed.
type ResetSteps struct {
	DownAll func(root, databaseURL string) error
	UpAll   func(root, databaseURL string) error
	Seed    func(root, databaseURL string) error
}

// Reset re-applies a project's database from scratch and seeds it — the
// "drop + migrate + seed" cycle docs/acthur-prd.md §19.4 describes for
// `acthur db reset`, implemented as down-all + up-all rather than an actual
// DROP DATABASE so it needs no privileges beyond what migrate already
// requires. A project with no seeds/ directory still resets cleanly: Reset
// treats ErrNoSeedsDir as "nothing to seed", not a failure.
func Reset(root, databaseURL string, steps ResetSteps) error {
	downAll, upAll, seed := steps.DownAll, steps.UpAll, steps.Seed
	if downAll == nil {
		downAll = DownAll
	}
	if upAll == nil {
		upAll = Migrate
	}
	if seed == nil {
		seed = Seed
	}

	if err := downAll(root, databaseURL); err != nil {
		return fmt.Errorf("reset: rolling back all migrations: %w", err)
	}
	if err := upAll(root, databaseURL); err != nil {
		return fmt.Errorf("reset: re-applying migrations: %w", err)
	}
	if err := seed(root, databaseURL); err != nil {
		if errors.Is(err, ErrNoSeedsDir) {
			return nil
		}
		return fmt.Errorf("reset: seeding: %w", err)
	}
	return nil
}
