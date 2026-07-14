package main

import (
	"strings"
	"testing"

	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/doctor"
)

// TestDevDoctorPreflight_AbortsOnBlockingFailure asserts `acthur dev` refuses
// to start when a required tool is missing — the caller (devCmd.RunE) must
// never proceed to build the graph or start the engine in that case.
func TestDevDoctorPreflight_AbortsOnBlockingFailure(t *testing.T) {
	cfg := &config.Config{Project: "test"}
	run := func(*config.Config) *doctor.Result {
		return &doctor.Result{Checks: []*doctor.Check{
			{Name: "docker", Required: true, Status: doctor.StatusFailed},
		}}
	}

	err := devDoctorPreflight(cfg, run)
	if err == nil {
		t.Fatal("expected an error aborting dev")
	}
	if !strings.Contains(err.Error(), "--skip-doctor") {
		t.Fatalf("expected error to mention --skip-doctor bypass, got %q", err.Error())
	}
}

// TestDevDoctorPreflight_PassesWhenHealthy asserts a clean environment
// report never blocks dev.
func TestDevDoctorPreflight_PassesWhenHealthy(t *testing.T) {
	cfg := &config.Config{Project: "test"}
	run := func(*config.Config) *doctor.Result {
		return &doctor.Result{Checks: []*doctor.Check{
			{Name: "git", Required: true, Status: doctor.StatusOK},
			{Name: "docker", Required: true, Status: doctor.StatusOK},
		}}
	}

	if err := devDoctorPreflight(cfg, run); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// TestDevDoctorPreflight_WarningOnlyPasses: a warning-status required check
// (e.g. a port already in use) must not abort dev — only missing/failed
// required tooling does.
func TestDevDoctorPreflight_WarningOnlyPasses(t *testing.T) {
	cfg := &config.Config{Project: "test"}
	run := func(*config.Config) *doctor.Result {
		return &doctor.Result{Checks: []*doctor.Check{
			{Name: "port 8080", Required: true, Status: doctor.StatusWarning},
		}}
	}

	if err := devDoctorPreflight(cfg, run); err != nil {
		t.Fatalf("expected no error for warning-only result, got %v", err)
	}
}
