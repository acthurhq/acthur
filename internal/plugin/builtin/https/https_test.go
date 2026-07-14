package https_test

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/acthurhq/acthur/internal/plugin"
	"github.com/acthurhq/acthur/internal/plugin/builtin/https"
)

func TestEnsureDevCert_GeneratesCAAndLeaf(t *testing.T) {
	dir := t.TempDir()

	paths, err := https.EnsureDevCert(dir, "acme.test")
	if err != nil {
		t.Fatalf("EnsureDevCert: %v", err)
	}
	for _, p := range []string{paths.CAFile, paths.CAKey, paths.CertFile, paths.KeyFile} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s to exist: %v", p, err)
		}
	}

	// The leaf cert + key must load as a valid TLS key pair.
	if _, err := tls.LoadX509KeyPair(paths.CertFile, paths.KeyFile); err != nil {
		t.Fatalf("LoadX509KeyPair: %v", err)
	}
}

func TestEnsureDevCert_IdempotentWhenStillValid(t *testing.T) {
	dir := t.TempDir()

	first, err := https.EnsureDevCert(dir, "acme.test")
	if err != nil {
		t.Fatalf("first EnsureDevCert: %v", err)
	}
	firstBytes, err := os.ReadFile(first.CertFile)
	if err != nil {
		t.Fatal(err)
	}

	second, err := https.EnsureDevCert(dir, "acme.test")
	if err != nil {
		t.Fatalf("second EnsureDevCert: %v", err)
	}
	secondBytes, err := os.ReadFile(second.CertFile)
	if err != nil {
		t.Fatal(err)
	}

	if string(firstBytes) != string(secondBytes) {
		t.Error("expected EnsureDevCert to reuse the existing cert, got a regenerated one")
	}
}

func TestEnsureDevCert_RegeneratesForNewDomain(t *testing.T) {
	dir := t.TempDir()
	if _, err := https.EnsureDevCert(dir, "acme.test"); err != nil {
		t.Fatal(err)
	}
	second, err := https.EnsureDevCert(dir, "other.test")
	if err != nil {
		t.Fatalf("EnsureDevCert for new domain: %v", err)
	}
	cert, err := tls.LoadX509KeyPair(second.CertFile, second.KeyFile)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := leaf.VerifyHostname("other.test"); err != nil {
		t.Errorf("expected regenerated cert to cover other.test: %v", err)
	}
}

// TestServeTLS_RealHandshake proves the generated cert/key pair actually
// serves a working TLS listener a Go client can complete a handshake
// against (trusting the generated CA) — not just "parses as a cert".
func TestServeTLS_RealHandshake(t *testing.T) {
	dir := t.TempDir()
	paths, err := https.EnsureDevCert(dir, "localhost")
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	cert, err := tls.LoadX509KeyPair(paths.CertFile, paths.KeyFile)
	if err != nil {
		t.Fatal(err)
	}
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	srv.Listener, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv.StartTLS()
	defer srv.Close()

	caPEM, err := os.ReadFile(paths.CAFile)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("failed to add CA to pool")
	}

	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}},
		Timeout:   5 * time.Second,
	}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("HTTPS request against generated cert failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestGenerator_WritesReadmeAndCert(t *testing.T) {
	dir := t.TempDir()
	p, err := plugin.Resolve("https")
	if err != nil {
		// Ensure the plugin is loaded (blank import side effect) — in this
		// package's own test binary the import below registers it.
		t.Fatalf("resolve https plugin: %v", err)
	}
	k := plugin.NewKernelAPI(plugin.NewBus(), nil, func(plugin.CLICommand) {}, func(plugin.LogLevel, string, ...any) {})
	if err := p.Register(k); err != nil {
		t.Fatalf("register: %v", err)
	}
	gen, ok := k.Generator("https")
	if !ok {
		t.Fatal("expected https generator to be registered")
	}

	files, err := gen.Generate("*", plugin.GeneratorContext{
		RootDir: dir,
		Extra:   map[string]any{"dev_domain": "acme.test"},
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(files) != 1 || files[0].Path != ".acthur/certs/README.md" {
		t.Fatalf("unexpected files: %+v", files)
	}
	if _, err := os.Stat(filepath.Join(dir, ".acthur", "certs", "dev.pem")); err != nil {
		t.Errorf("expected cert generated as a side effect: %v", err)
	}
}
