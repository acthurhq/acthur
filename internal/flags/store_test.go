package flags

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateName(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"new-booking-flow", true},
		{"new_booking_flow", true},
		{"a", true},
		{"New-Booking", false},
		{"1flag", false},
		{"has space", false},
		{"", false},
	}
	for _, c := range cases {
		err := ValidateName(c.name)
		if c.ok && err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", c.name, err)
		}
		if !c.ok && err == nil {
			t.Errorf("ValidateName(%q) = nil, want error", c.name)
		}
	}
}

func TestEnvKey(t *testing.T) {
	cases := map[string]string{
		"new-booking-flow": "ACTHUR_FLAG_NEW_BOOKING_FLOW",
		"simple":           "ACTHUR_FLAG_SIMPLE",
		"a_b-c":            "ACTHUR_FLAG_A_B_C",
	}
	for in, want := range cases {
		if got := EnvKey(in); got != want {
			t.Errorf("EnvKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStore_CreateList(t *testing.T) {
	root := t.TempDir()
	s := New(root)

	list, err := s.List()
	if err != nil {
		t.Fatalf("List on empty store: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("List on empty store = %v, want empty", list)
	}

	if err := s.Create("new-booking-flow", "rolls out the new flow"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.Create("second-flag", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}

	list, err = s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List() = %v, want 2 entries", list)
	}
	if list[0].Name != "new-booking-flow" || list[1].Name != "second-flag" {
		t.Errorf("List() not sorted by name: %v", list)
	}
	if list[0].Enabled {
		t.Error("new flag should default to disabled")
	}
	if list[0].Description != "rolls out the new flow" {
		t.Errorf("description = %q", list[0].Description)
	}
}

func TestStore_Create_Duplicate(t *testing.T) {
	s := New(t.TempDir())
	_ = s.Create("flag", "")
	if err := s.Create("flag", ""); err == nil {
		t.Fatal("expected error creating duplicate flag")
	}
}

func TestStore_EnableDisableToggle(t *testing.T) {
	s := New(t.TempDir())
	_ = s.Create("flag", "")

	if err := s.Enable("flag"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	f, _ := s.Get("flag")
	if !f.Enabled {
		t.Error("flag should be enabled")
	}

	if err := s.Disable("flag"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	f, _ = s.Get("flag")
	if f.Enabled {
		t.Error("flag should be disabled")
	}

	newState, err := s.Toggle("flag")
	if err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	if !newState {
		t.Error("Toggle from disabled should return true")
	}
	newState, err = s.Toggle("flag")
	if err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	if newState {
		t.Error("Toggle from enabled should return false")
	}
}

func TestStore_Enable_NotFound(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Enable("missing"); err == nil {
		t.Fatal("expected error enabling nonexistent flag")
	}
}

func TestStore_Remove(t *testing.T) {
	s := New(t.TempDir())
	_ = s.Create("flag", "")
	if err := s.Remove("flag"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := s.Get("flag"); err == nil {
		t.Fatal("expected error getting removed flag")
	}
	if err := s.Remove("flag"); err == nil {
		t.Fatal("expected error removing already-removed flag")
	}
}

func TestStore_PersistsAcrossInstances(t *testing.T) {
	root := t.TempDir()
	s1 := New(root)
	_ = s1.Create("flag", "desc")
	_ = s1.Enable("flag")

	// A second Store instance (simulating a second CLI invocation, or the
	// dev engine's hot-reload) must see the same persisted state.
	s2 := New(root)
	f, err := s2.Get("flag")
	if err != nil {
		t.Fatalf("Get from second instance: %v", err)
	}
	if !f.Enabled {
		t.Error("second instance should see enabled state written by first")
	}

	raw, err := os.ReadFile(filepath.Join(root, ".acthur", "flags.json"))
	if err != nil {
		t.Fatalf("read flags.json: %v", err)
	}
	var onDisk []Flag
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("flags.json is not valid JSON: %v", err)
	}
	if len(onDisk) != 1 || onDisk[0].Name != "flag" {
		t.Errorf("on-disk content = %v", onDisk)
	}
}
