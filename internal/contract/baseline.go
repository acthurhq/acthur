package contract

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const deployBaselinePath = ".acthur/contracts.lock.yml"

type deployBaseline struct {
	Version   int         `yaml:"version"`
	Contracts []*Contract `yaml:"contracts"`
}

// UpdateDeployBaseline records the currently valid contracts as the explicit
// compatibility baseline used by subsequent deploy gates.
func UpdateDeployBaseline(root string) error {
	registry, err := LoadDir(root)
	if err != nil {
		return err
	}
	contracts := registry.All()
	sort.Slice(contracts, func(i, j int) bool {
		if contracts[i].Name == contracts[j].Name {
			return contracts[i].Version < contracts[j].Version
		}
		return contracts[i].Name < contracts[j].Name
	})
	data, err := yaml.Marshal(deployBaseline{Version: 1, Contracts: contracts})
	if err != nil {
		return fmt.Errorf("encode contract deploy baseline: %w", err)
	}
	path := filepath.Join(root, filepath.FromSlash(deployBaselinePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create contract deploy baseline directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write contract deploy baseline %q: %w", path, err)
	}
	return nil
}

// CheckDeployBaseline rejects breaking changes from the explicit baseline.
// Projects without .acthur/contracts.lock.yml have not opted into this check
// and pass. UpdateDeployBaseline is the only supported way to accept a new
// deployed contract surface.
func CheckDeployBaseline(root string) error {
	path := filepath.Join(root, filepath.FromSlash(deployBaselinePath))
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read contract deploy baseline %q: %w", path, err)
	}
	var baseline deployBaseline
	if err := yaml.Unmarshal(data, &baseline); err != nil {
		return fmt.Errorf("parse contract deploy baseline %q: %w", path, err)
	}
	if baseline.Version != 1 {
		return fmt.Errorf("contract deploy baseline %q has unsupported version %d", path, baseline.Version)
	}
	current, err := LoadDir(root)
	if err != nil {
		return err
	}
	var breaking []string
	for _, old := range baseline.Contracts {
		if errs := old.Validate(); len(errs) > 0 {
			return fmt.Errorf("contract deploy baseline %q contains invalid contract %q: %v", path, old.Name, errs)
		}
		next, err := current.GetLatest(old.Name)
		if err != nil {
			breaking = append(breaking, fmt.Sprintf("contract %q removed", old.Name))
			continue
		}
		result := Diff(old, next)
		for _, change := range result.Changes {
			if change.Type == ChangeBreaking {
				breaking = append(breaking, fmt.Sprintf("contract %q %s: %s", old.Name, change.Field, change.Description))
			}
		}
	}
	if len(breaking) > 0 {
		sort.Strings(breaking)
		return fmt.Errorf("breaking changes from %s:\n  - %s", deployBaselinePath, strings.Join(breaking, "\n  - "))
	}
	return nil
}
