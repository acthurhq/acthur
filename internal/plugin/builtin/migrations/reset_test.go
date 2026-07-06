package migrations_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/acthur/acthur/internal/plugin/builtin/migrations"
)

// TestReset_RunsStepsInOrder: down-all, then up-all, then seed — and each
// step receives the same root/databaseURL.
func TestReset_RunsStepsInOrder(t *testing.T) {
	var order []string
	steps := migrations.ResetSteps{
		DownAll: func(root, url string) error { order = append(order, "down:"+root+":"+url); return nil },
		UpAll:   func(root, url string) error { order = append(order, "up:"+root+":"+url); return nil },
		Seed:    func(root, url string) error { order = append(order, "seed:"+root+":"+url); return nil },
	}

	if err := migrations.Reset("/proj", "postgres://x", steps); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	want := []string{"down:/proj:postgres://x", "up:/proj:postgres://x", "seed:/proj:postgres://x"}
	if len(order) != len(want) {
		t.Fatalf("expected %v, got %v", want, order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("step %d: expected %q, got %q", i, want[i], order[i])
		}
	}
}

// TestReset_DownAllFails_StopsBeforeUpAllAndSeed: a failure in the first
// step must not run the later steps.
func TestReset_DownAllFails_StopsBeforeUpAllAndSeed(t *testing.T) {
	var ranUp, ranSeed bool
	steps := migrations.ResetSteps{
		DownAll: func(root, url string) error { return fmt.Errorf("down failed") },
		UpAll:   func(root, url string) error { ranUp = true; return nil },
		Seed:    func(root, url string) error { ranSeed = true; return nil },
	}

	err := migrations.Reset("/proj", "postgres://x", steps)
	if err == nil {
		t.Fatal("expected an error when down-all fails")
	}
	if !strings.Contains(err.Error(), "down failed") {
		t.Errorf("expected underlying error wrapped, got: %v", err)
	}
	if ranUp || ranSeed {
		t.Error("expected up-all and seed to be skipped after down-all failure")
	}
}

// TestReset_UpAllFails_StopsBeforeSeed
func TestReset_UpAllFails_StopsBeforeSeed(t *testing.T) {
	var ranSeed bool
	steps := migrations.ResetSteps{
		DownAll: func(root, url string) error { return nil },
		UpAll:   func(root, url string) error { return fmt.Errorf("up failed") },
		Seed:    func(root, url string) error { ranSeed = true; return nil },
	}

	err := migrations.Reset("/proj", "postgres://x", steps)
	if err == nil {
		t.Fatal("expected an error when up-all fails")
	}
	if ranSeed {
		t.Error("expected seed to be skipped after up-all failure")
	}
}

// TestReset_NoSeedsDir_StillSucceeds: a project with no seeds/ directory
// resets cleanly rather than failing the whole reset.
func TestReset_NoSeedsDir_StillSucceeds(t *testing.T) {
	steps := migrations.ResetSteps{
		DownAll: func(root, url string) error { return nil },
		UpAll:   func(root, url string) error { return nil },
		Seed:    func(root, url string) error { return migrations.ErrNoSeedsDir },
	}

	if err := migrations.Reset("/proj", "postgres://x", steps); err != nil {
		t.Errorf("expected Reset to succeed when there is nothing to seed, got: %v", err)
	}
}

// TestReset_SeedFailure_PropagatesNonNoSeedsDirErrors
func TestReset_SeedFailure_PropagatesNonNoSeedsDirErrors(t *testing.T) {
	seedErr := errors.New("seed sql error")
	steps := migrations.ResetSteps{
		DownAll: func(root, url string) error { return nil },
		UpAll:   func(root, url string) error { return nil },
		Seed:    func(root, url string) error { return seedErr },
	}

	err := migrations.Reset("/proj", "postgres://x", steps)
	if err == nil {
		t.Fatal("expected a real seed failure to propagate")
	}
	if !strings.Contains(err.Error(), "seed sql error") {
		t.Errorf("expected underlying seed error wrapped, got: %v", err)
	}
}
