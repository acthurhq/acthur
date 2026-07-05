package contract

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadDir loads every "contracts/*.contract.yml" file found directly under
// root into a fresh Registry. root is the project root — the same directory
// that holds acthur.yml — and "contracts/" is resolved relative to it per
// the Phase 4 convention (contracts/<name>.contract.yml).
//
// A project with no contracts/ directory yields an empty, non-nil Registry
// and a nil error — contracts are optional. A file that fails to parse or
// register produces an error naming that file; loading stops at the first
// such failure.
func LoadDir(root string) (*Registry, error) {
	reg := NewRegistry()

	dir := filepath.Join(root, "contracts")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return reg, nil
		}
		return nil, fmt.Errorf("cannot read contracts directory %q: %w", dir, err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".contract.yml") && !strings.HasSuffix(e.Name(), ".contract.yaml") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		path := filepath.Join(dir, name)
		if _, err := reg.RegisterFile(path); err != nil {
			return nil, fmt.Errorf("failed to load contract file %q: %w", path, err)
		}
	}

	return reg, nil
}
