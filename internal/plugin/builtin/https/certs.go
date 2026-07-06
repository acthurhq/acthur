// Package https is the built-in "https" plugin (Phase 9, tracker #65). It
// gives `acthur dev` browser-trusted-shaped local TLS without depending on
// an external tool: EnsureDevCert generates a self-signed CA and a leaf
// certificate (SANs: the project's dev domain, `*.<domain>`, and
// localhost/127.0.0.1) directly with Go's crypto/x509, persists them under
// <root>/.acthur/certs/, and reuses them on every subsequent call as long
// as they remain valid and cover the requested domain.
//
// Honest scope: the PRD's dev-mode description names mkcert specifically
// (generate a local CA via mkcert, install it in the system trust store so
// browsers never warn). This plugin deliberately does NOT shell out to
// mkcert (an external binary this repo cannot assume is installed) and does
// NOT install the generated CA into the OS/browser trust store (that step
// needs elevated permissions/`security`/`certutil` calls that are
// platform-specific and were judged out of scope for a first slice — see
// the status note in docs/implementation/active/0007-prd-completion.md).
// The result: `acthur dev --https` serves real TLS 1.2+ with a real
// self-signed cert, but a browser will show a one-time trust warning until
// a developer manually imports .acthur/certs/ca.pem — an explicit, honest
// gap, not a silent stub. Production TLS (Caddy/Nginx/ACME) is likewise not
// implemented here; that belongs to the deploy targets (Phase 8), not dev
// mode.
package https

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// CertPaths are the on-disk locations of the generated dev CA and leaf
// certificate/key, all under <root>/.acthur/certs/.
type CertPaths struct {
	CAFile   string
	CAKey    string
	CertFile string
	KeyFile  string
}

// certsDir returns <root>/.acthur/certs.
func certsDir(root string) string {
	return filepath.Join(root, ".acthur", "certs")
}

// PathsFor returns the well-known cert file locations for root, without
// touching disk.
func PathsFor(root string) CertPaths {
	dir := certsDir(root)
	return CertPaths{
		CAFile:   filepath.Join(dir, "ca.pem"),
		CAKey:    filepath.Join(dir, "ca.key"),
		CertFile: filepath.Join(dir, "dev.pem"),
		KeyFile:  filepath.Join(dir, "dev.key"),
	}
}

// EnsureDevCert returns a CA + leaf certificate/key pair covering domain
// (and "*.<domain>", "localhost", "127.0.0.1"), generating and persisting a
// fresh pair under <root>/.acthur/certs/ if none exists yet, or if the
// existing leaf cert doesn't cover domain or expires within 30 days.
func EnsureDevCert(root, domain string) (CertPaths, error) {
	paths := PathsFor(root)

	if certCoversDomain(paths.CertFile, domain) {
		return paths, nil
	}

	if err := os.MkdirAll(certsDir(root), 0o755); err != nil {
		return CertPaths{}, fmt.Errorf("create %s: %w", certsDir(root), err)
	}

	caCert, caKey, err := generateCA()
	if err != nil {
		return CertPaths{}, fmt.Errorf("generate dev CA: %w", err)
	}
	if err := writeCertKeyPair(paths.CAFile, paths.CAKey, caCert, caKey); err != nil {
		return CertPaths{}, err
	}

	leafDER, leafKey, err := generateLeaf(domain, caCert, caKey)
	if err != nil {
		return CertPaths{}, fmt.Errorf("generate dev leaf cert: %w", err)
	}
	if err := writeCertKeyPair(paths.CertFile, paths.KeyFile, leafDER, leafKey); err != nil {
		return CertPaths{}, err
	}

	return paths, nil
}

// certCoversDomain reports whether an existing cert at certFile is present,
// not expiring within 30 days, and covers domain via SAN (exact match or a
// wildcard parent).
func certCoversDomain(certFile, domain string) bool {
	b, err := os.ReadFile(certFile)
	if err != nil {
		return false
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}
	if time.Until(cert.NotAfter) < 30*24*time.Hour {
		return false
	}
	if err := cert.VerifyHostname(domain); err == nil {
		return true
	}
	return false
}

// generateCA creates a self-signed CA certificate (DER-encoded) and its key.
func generateCA() ([]byte, *ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{Organization: []string{"Acthur Dev CA"}, CommonName: "Acthur Local Development CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(2, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	return der, key, nil
}

// generateLeaf creates a leaf certificate for domain (plus *.domain,
// localhost, 127.0.0.1, ::1), signed by the given CA.
func generateLeaf(domain string, caDER []byte, caKey *ecdsa.PrivateKey) ([]byte, *ecdsa.PrivateKey, error) {
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{Organization: []string{"Acthur Dev"}, CommonName: domain},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(0, 3, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{domain, "*." + domain, "localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}
	return der, key, nil
}

func writeCertKeyPair(certFile, keyFile string, der []byte, key *ecdsa.PrivateKey) error {
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(certFile, certPEM, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", certFile, err)
	}
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", keyFile, err)
	}
	return nil
}
