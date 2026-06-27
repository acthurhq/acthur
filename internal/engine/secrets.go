package engine

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// secretStore supplies stable values for adapter EnvVars marked Generate.
// A generated dev secret (e.g. APP_SECRET) must survive `acthur dev` restarts,
// otherwise issued sessions and signed tokens break on every reload.
type secretStore interface {
	Secret(nodeID, key string) (string, error)
}

// fileSecretStore persists generated dev secrets under <root>/.acthur/secrets.
// Values are created on first use with crypto/rand and never overwritten, so a
// project gets one stable secret per (node, key) for the life of its checkout.
type fileSecretStore struct {
	rootDir string
}

func (s fileSecretStore) Secret(nodeID, key string) (string, error) {
	dir := filepath.Join(s.rootDir, ".acthur", "secrets")
	path := filepath.Join(dir, nodeID+"."+key)

	if b, err := os.ReadFile(path); err == nil {
		if v := strings.TrimSpace(string(b)); v != "" {
			return v, nil
		}
	}

	value, err := randomSecret()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create secrets dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(value+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("persist secret %q: %w", key, err)
	}
	return value, nil
}

func randomSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
