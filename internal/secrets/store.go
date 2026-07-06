// Package secrets is the project-scoped secret store backing `acthur
// secrets set/get/list/rm/rotate`. It persists user-managed secrets under
// <root>/.acthur/secrets/kv/<KEY> as plaintext files with 0600 permissions.
//
// This is deliberately the same "local .env, dev-only" model the PRD
// documents for the built-in secrets provider (§ "secrets" plugin: "local
// .env (dev only)") — there is no encryption-at-rest here. Production
// deploys never read from this store; the pre-deploy gate resolves
// production secrets from the real deploy environment (see
// internal/deploy.RunGate / checkEnv), falling back to this store only to
// make local `acthur deploy --dry-run` dev-loop testing convenient.
//
// It lives in its own package (not internal/engine, which already owns a
// *different* concern — per-node auto-generated adapter secrets like a
// scaffolded APP_SECRET, keyed by (nodeID, key) under .acthur/secrets/) so
// the two never collide on disk: this store's directory is
// .acthur/secrets/kv/, one level below the per-node files.
package secrets

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// keyPattern restricts secret keys to the shape every downstream consumer
// (shell env, Docker Compose env, Go os.Getenv) can use unambiguously:
// upper-snake-case, starting with a letter.
var keyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// ValidateKey reports whether key is a legal secret key.
func ValidateKey(key string) error {
	if !keyPattern.MatchString(key) {
		return fmt.Errorf("invalid secret key %q — must be UPPER_SNAKE_CASE (e.g. STRIPE_API_KEY)", key)
	}
	return nil
}

// Store is the project-scoped secret store for one project root.
type Store struct {
	dir string
}

// New creates a Store rooted at rootDir's .acthur/secrets/kv directory.
func New(rootDir string) *Store {
	return &Store{dir: filepath.Join(rootDir, ".acthur", "secrets", "kv")}
}

func (s *Store) path(key string) string { return filepath.Join(s.dir, key) }

// Set writes value for key, creating the store directory if needed.
func (s *Store) Set(key, value string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create secrets store: %w", err)
	}
	if err := os.WriteFile(s.path(key), []byte(value), 0o600); err != nil {
		return fmt.Errorf("persist secret %q: %w", key, err)
	}
	return nil
}

// Get returns the value stored for key. Returns a pointed error naming the
// key if it has never been set.
func (s *Store) Get(key string) (string, error) {
	if err := ValidateKey(key); err != nil {
		return "", err
	}
	b, err := os.ReadFile(s.path(key))
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("secret %q is not set — run 'acthur secrets set %s <value>'", key, key)
		}
		return "", fmt.Errorf("read secret %q: %w", key, err)
	}
	return string(b), nil
}

// Has reports whether key is currently set, without erroring when it is not.
func (s *Store) Has(key string) bool {
	_, err := os.Stat(s.path(key))
	return err == nil
}

// List returns every secret key currently set, sorted, without their values.
func (s *Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		keys = append(keys, e.Name())
	}
	sort.Strings(keys)
	return keys, nil
}

// Remove deletes key from the store. Returns a pointed error if key was
// never set.
func (s *Store) Remove(key string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	if err := os.Remove(s.path(key)); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("secret %q is not set", key)
		}
		return fmt.Errorf("remove secret %q: %w", key, err)
	}
	return nil
}

// All returns every stored secret as a key/value map, for injection into a
// process environment (the dev engine's use case). An empty/absent store
// returns an empty map, never an error.
func (s *Store) All() (map[string]string, error) {
	keys, err := s.List()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		v, err := s.Get(k)
		if err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}

// RandomValue generates cryptographically random hex value suitable for a
// rotated secret. Exported so callers (the `acthur secrets rotate` command)
// don't need their own crypto/rand plumbing, and so tests can inject a
// deterministic generator via a function of this same shape.
func RandomValue() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// Rotate replaces key's value with a freshly generated one via generate,
// returning the new value. Returns a pointed error if key was never set —
// rotate is for refreshing an existing secret, not creating one (use Set).
func (s *Store) Rotate(key string, generate func() (string, error)) (string, error) {
	if !s.Has(key) {
		return "", fmt.Errorf("secret %q is not set — run 'acthur secrets set %s <value>' before rotating", key, key)
	}
	if generate == nil {
		generate = RandomValue
	}
	value, err := generate()
	if err != nil {
		return "", err
	}
	if err := s.Set(key, value); err != nil {
		return "", err
	}
	return value, nil
}
