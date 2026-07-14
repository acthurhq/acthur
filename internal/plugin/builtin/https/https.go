package https

import (
	"fmt"

	"github.com/acthurhq/acthur/internal/plugin"
)

type httpsPlugin struct{}

func (httpsPlugin) Name() string        { return "https" }
func (httpsPlugin) Version() string     { return "0.1.0" }
func (httpsPlugin) DependsOn() []string { return nil }

// Register installs the "https:setup" command (also reachable as
// `acthur add https` since RegisterGenerator makes this plugin generator-
// bearing) and a lifecycle hook that reminds a developer to trust the
// generated CA. The actual dev-mode TLS wiring lives in cmd/acthur's dev
// command + internal/proxy's WithTLS option — this plugin's job is
// certificate management, not owning the proxy lifecycle.
func (httpsPlugin) Register(k plugin.KernelAPI) error {
	k.RegisterGenerator("https", generator{})

	k.OnEvent(plugin.EventBeforeNodeStart, func(p plugin.EventPayload) {
		rootDir, _ := p.Data["root_dir"].(string)
		if rootDir == "" {
			return
		}
		paths := PathsFor(rootDir)
		k.Log(plugin.LogInfo, "https: dev cert at %s (import into your browser/OS trust store to avoid warnings — see docs/adr and plugin package docs for why this isn't automated)", paths.CAFile)
	})

	return nil
}

func init() {
	plugin.Register(httpsPlugin{})
}

// ---------------------------------------------------------------------------
// Generator — not per-service code (TLS is a dev-proxy concern, not
// something each service node needs its own package for). Generate instead
// ensures the dev cert exists for the project's dev domain and reports the
// resulting paths as a README so `acthur add https` leaves a durable,
// visible trail of what it did, matching the shape every other plugin's
// generator has (a real, inspectable file), rather than a silent side
// effect.
// ---------------------------------------------------------------------------

type generator struct{}

func (generator) SupportedAdapters() []string { return []string{"*"} }

func (generator) Generate(adapterName string, ctx plugin.GeneratorContext) ([]plugin.GeneratedFile, error) {
	domain, _ := ctx.Extra["dev_domain"].(string)
	if domain == "" {
		domain = "localhost"
	}
	if ctx.RootDir == "" {
		return nil, fmt.Errorf("https generator requires ctx.RootDir to generate a dev certificate")
	}
	paths, err := EnsureDevCert(ctx.RootDir, domain)
	if err != nil {
		return nil, fmt.Errorf("https: %w", err)
	}

	readme := fmt.Sprintf(`# https plugin — local dev TLS

Generated a self-signed CA and a leaf certificate for %q under
.acthur/certs/:

  - %s (CA — import into your OS/browser trust store to silence warnings)
  - %s (CA private key — never commit)
  - %s (leaf cert covering %s, *.%s, localhost, 127.0.0.1)
  - %s (leaf private key — never commit)

Set dev.https: true in acthur.yml and run 'acthur dev' to serve the proxy
over TLS using this certificate. Regenerate at any time with
'acthur add https' (a no-op if the current cert still covers the domain
and has more than 30 days left before expiry).

Not implemented (see internal/plugin/builtin/https/certs.go doc comment):
automatic trust-store installation (mkcert-style) and production TLS
(Caddy/Nginx/ACME — that's owned by the deploy targets, not this plugin).
`, domain, paths.CAFile, paths.CAKey, paths.CertFile, domain, domain, paths.KeyFile)

	return []plugin.GeneratedFile{{
		Path:      ".acthur/certs/README.md",
		Content:   []byte(readme),
		Mode:      0o644,
		Overwrite: true,
	}}, nil
}
