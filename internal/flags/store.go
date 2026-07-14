// Package flags is the project's feature flag store backing `acthur flag
// create/list/enable/disable/toggle`. It implements the PRD's "local"
// provider: a single JSON file at <root>/.acthur/flags.json, hot-reloaded —
// every read re-parses the file from disk, so a flag toggled from a second
// CLI invocation is picked up by a running `acthur dev` on its next lookup
// without a restart.
package flags

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// namePattern restricts flag names to characters that round-trip cleanly
// into an env var name and a generated Go constant (PRD §37.3).
var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// ValidateName reports whether name is a legal flag name.
func ValidateName(name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("invalid flag name %q — use lowercase letters, digits, - or _, starting with a letter (e.g. new-booking-flow)", name)
	}
	return nil
}

// Flag is one feature flag's persisted state.
type Flag struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
}

// EnvKey returns the env var name a flag is exposed as to service processes:
// ACTHUR_FLAG_<NAME>, with the flag's name upper-cased and non-alnum runs
// collapsed to a single underscore (PRD: "exposed to services as env
// (ACTHUR_FLAG_<NAME>)").
func EnvKey(name string) string {
	upper := strings.ToUpper(name)
	var b strings.Builder
	lastUnderscore := false
	for _, r := range upper {
		switch {
		case r >= 'A' && r <= 'Z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	return "ACTHUR_FLAG_" + strings.Trim(b.String(), "_")
}

// EnvValue returns the string a flag's env var carries: "true" or "false".
func EnvValue(enabled bool) string {
	if enabled {
		return "true"
	}
	return "false"
}

// Store is the flags store for one project root.
type Store struct {
	path string
}

// New creates a Store rooted at rootDir's .acthur/flags.json file.
func New(rootDir string) *Store {
	return &Store{path: filepath.Join(rootDir, ".acthur", "flags.json")}
}

// load re-reads the flags file from disk, never caching — every operation
// (including List) must see concurrent edits from other `acthur flag`
// invocations or a running `acthur dev`.
func (s *Store) load() (map[string]*Flag, error) {
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]*Flag{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", s.path, err)
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return map[string]*Flag{}, nil
	}
	var list []*Flag
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, fmt.Errorf("parse %s: %w", s.path, err)
	}
	out := make(map[string]*Flag, len(list))
	for _, f := range list {
		out[f.Name] = f
	}
	return out, nil
}

func (s *Store) save(flags map[string]*Flag) error {
	list := make([]*Flag, 0, len(flags))
	for _, f := range flags {
		list = append(list, f)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })

	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("encode flags: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(s.path), err)
	}
	return os.WriteFile(s.path, append(b, '\n'), 0o644)
}

// Create adds a new flag, disabled by default. Returns a pointed error if a
// flag with that name already exists.
func (s *Store) Create(name, description string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	all, err := s.load()
	if err != nil {
		return err
	}
	if _, exists := all[name]; exists {
		return fmt.Errorf("flag %q already exists", name)
	}
	all[name] = &Flag{Name: name, Description: description, Enabled: false}
	return s.save(all)
}

// List returns every flag, sorted by name.
func (s *Store) List() ([]Flag, error) {
	all, err := s.load()
	if err != nil {
		return nil, err
	}
	out := make([]Flag, 0, len(all))
	for _, f := range all {
		out = append(out, *f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Get returns a single flag by name, or a pointed error if it does not exist.
func (s *Store) Get(name string) (Flag, error) {
	all, err := s.load()
	if err != nil {
		return Flag{}, err
	}
	f, ok := all[name]
	if !ok {
		return Flag{}, fmt.Errorf("flag %q does not exist — run 'acthur flag create %s' first", name, name)
	}
	return *f, nil
}

// setEnabled is the shared implementation for Enable/Disable/Toggle.
func (s *Store) setEnabled(name string, enabled func(current bool) bool) (bool, error) {
	all, err := s.load()
	if err != nil {
		return false, err
	}
	f, ok := all[name]
	if !ok {
		return false, fmt.Errorf("flag %q does not exist — run 'acthur flag create %s' first", name, name)
	}
	f.Enabled = enabled(f.Enabled)
	if err := s.save(all); err != nil {
		return false, err
	}
	return f.Enabled, nil
}

// Enable turns a flag on.
func (s *Store) Enable(name string) error {
	_, err := s.setEnabled(name, func(bool) bool { return true })
	return err
}

// Disable turns a flag off.
func (s *Store) Disable(name string) error {
	_, err := s.setEnabled(name, func(bool) bool { return false })
	return err
}

// Toggle flips a flag's enabled state and returns the new state.
func (s *Store) Toggle(name string) (bool, error) {
	return s.setEnabled(name, func(current bool) bool { return !current })
}

// Remove deletes a flag entirely ("retire" in PRD terms).
func (s *Store) Remove(name string) error {
	all, err := s.load()
	if err != nil {
		return err
	}
	if _, ok := all[name]; !ok {
		return fmt.Errorf("flag %q does not exist", name)
	}
	delete(all, name)
	return s.save(all)
}
