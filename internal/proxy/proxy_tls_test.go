package proxy_test

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/acthurhq/acthur/internal/plugin/builtin/https"
	"github.com/acthurhq/acthur/internal/proxy"
)

// TestProxy_WithTLS_ServesRealHTTPS proves the WithTLS seam end-to-end: a
// proxy configured with a real cert (generated the same way the https
// plugin generates one for `acthur dev`) actually serves TLS 1.2+ and a Go
// client trusting the generated CA can complete requests against it.
func TestProxy_WithTLS_ServesRealHTTPS(t *testing.T) {
	apiBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "api")
		w.WriteHeader(http.StatusOK)
	}))
	defer apiBackend.Close()

	apiPort := extractPort(t, apiBackend.URL)
	g := buildGraphWithPorts(t, apiPort, apiPort)

	dir := t.TempDir()
	paths, err := https.EnsureDevCert(dir, "localhost")
	if err != nil {
		t.Fatalf("EnsureDevCert: %v", err)
	}

	p, err := proxy.New(g, 14443, proxy.WithTLS(paths.CertFile, paths.KeyFile))
	if err != nil {
		t.Fatalf("proxy.New: %v", err)
	}
	if err := p.Start(); err != nil {
		t.Fatalf("proxy start: %v", err)
	}
	defer func() { _ = p.Stop() }()

	time.Sleep(150 * time.Millisecond)

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

	resp, err := client.Get("https://localhost:14443/api/users")
	if err != nil {
		t.Fatalf("HTTPS request through proxy failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.Header.Get("X-Backend") != "api" {
		t.Errorf("expected X-Backend=api, got %q", resp.Header.Get("X-Backend"))
	}
}
