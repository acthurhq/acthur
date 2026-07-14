package artifacts_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/deploy/artifacts"
	"github.com/acthurhq/acthur/internal/graph"
	"github.com/acthurhq/acthur/internal/plugin"

	_ "github.com/acthurhq/acthur/internal/adapter/backend/gofiber" // register go:fiber
	_ "github.com/acthurhq/acthur/internal/adapter/infra/postgres"  // register db:postgres

	"gopkg.in/yaml.v3"
)

// loadVetangle builds the deploy-supported portion of the vetangle fixture.
// Tests that exercise rejection of an incomplete production projection build
// their own graph explicitly; successful projection tests must start from a
// graph whose every declared node has production capability.
var vetangleRoot = filepath.Join("..", "..", "..", "testdata", "vetangle")

func loadVetangle(t *testing.T) (*config.Config, *graph.Graph) {
	t.Helper()
	cfg, err := config.LoadFile(filepath.Join(vetangleRoot, "acthur.yml"))
	if err != nil {
		t.Fatalf("config.LoadFile: %v", err)
	}
	omitted := map[string]bool{
		"web": true, "backoffice": true, "cache": true, "storage": true, "queue": true,
	}
	for nodeID := range omitted {
		delete(cfg.Graph.Nodes, nodeID)
	}
	edges := cfg.Graph.Edges[:0]
	for _, edge := range cfg.Graph.Edges {
		if !omitted[edge.From] && !omitted[edge.To] {
			edges = append(edges, edge)
		}
	}
	cfg.Graph.Edges = edges
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("graph.Build: %v", err)
	}
	return cfg, g
}

func fileByPath(t *testing.T, files []plugin.GeneratedFile, path string) plugin.GeneratedFile {
	t.Helper()
	for _, f := range files {
		if f.Path == path {
			return f
		}
	}
	t.Fatalf("expected generated file %q, got paths: %v", path, paths(files))
	return plugin.GeneratedFile{}
}

func paths(files []plugin.GeneratedFile) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Path
	}
	return out
}

// composeService mirrors the subset of docker-compose fields Project emits,
// used to unmarshal and assert structurally rather than string-match.
type composeService struct {
	Build *struct {
		Context    string `yaml:"context"`
		Dockerfile string `yaml:"dockerfile"`
	} `yaml:"build"`
	Image       string            `yaml:"image"`
	Volumes     []string          `yaml:"volumes"`
	Environment map[string]string `yaml:"environment"`
	Ports       []string          `yaml:"ports"`
	DependsOn   map[string]struct {
		Condition string `yaml:"condition"`
	} `yaml:"depends_on"`
	Healthcheck *struct {
		Test     []string `yaml:"test"`
		Interval string   `yaml:"interval"`
		Timeout  string   `yaml:"timeout"`
		Retries  int      `yaml:"retries"`
	} `yaml:"healthcheck"`
	Restart  string   `yaml:"restart"`
	Networks []string `yaml:"networks"`
}

type composeFile struct {
	Services map[string]composeService `yaml:"services"`
	Volumes  map[string]any            `yaml:"volumes"`
	Networks map[string]any            `yaml:"networks"`
}

func parseCompose(t *testing.T, content []byte) composeFile {
	t.Helper()
	var cf composeFile
	if err := yaml.Unmarshal(content, &cf); err != nil {
		t.Fatalf("compose file did not parse as YAML: %v\n%s", err, content)
	}
	return cf
}

func TestProject_RefusesToEmitPartialGraphWhenNodeCannotBeProjected(t *testing.T) {
	tests := []struct {
		name     string
		nodeType config.NodeType
		adapter  string
	}{
		{
			name:     "adapter cannot be resolved",
			nodeType: config.NodeTypeInfra,
			adapter:  "cache:not-installed",
		},
		{
			name:     "adapter lacks the required production capability",
			nodeType: config.NodeTypeInfra,
			adapter:  "go:fiber",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Graph: config.GraphConfig{Nodes: map[string]config.NodeConfig{
				"api": {
					Type:    config.NodeTypeService,
					Adapter: "go:fiber",
					Port:    8080,
				},
				"backing-store": {
					Type:    tt.nodeType,
					Adapter: tt.adapter,
				},
			}}}
			g, err := graph.Build(cfg)
			if err != nil {
				t.Fatalf("graph.Build: %v", err)
			}

			files, err := artifacts.Project(cfg, g, t.TempDir())
			if err == nil {
				t.Fatal("Project succeeded; want a pointed error instead of a partial production graph")
			}
			for _, want := range []string{"backing-store", tt.adapter} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name %q", err, want)
				}
			}
			if len(files) != 0 {
				t.Errorf("Project returned partial artifacts %v; want none", paths(files))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Dockerfile generation
// ---------------------------------------------------------------------------

func TestProject_EmitsDockerfilePerDockerizableServiceNode(t *testing.T) {
	cfg, g := loadVetangle(t)
	files, err := artifacts.Project(cfg, g, vetangleRoot)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	// api and worker both use go:fiber (Dockerizable).
	fileByPath(t, files, "deploy/Dockerfile.api")
	fileByPath(t, files, "deploy/Dockerfile.worker")
}

// TestProject_DockerfileBuildsTheRootMainPackage: the scaffold's main
// package lives at the module root; `go build -o <file> ./...` fails with
// "cannot write multiple packages to non-directory" the moment the module
// has more than one package (caught live by the Phase 8 witness).
func TestProject_DockerfileBuildsTheRootMainPackage(t *testing.T) {
	cfg, g := loadVetangle(t)
	files, err := artifacts.Project(cfg, g, vetangleRoot)
	if err != nil {
		t.Fatal(err)
	}
	df := fileByPath(t, files, "deploy/Dockerfile.api")
	content := string(df.Content)
	if strings.Contains(content, "./...") {
		t.Error("Dockerfile must build the root main package (.), not ./... — multiple packages cannot share one -o output")
	}
	if !strings.Contains(content, "go build") {
		t.Error("expected a go build line")
	}
}

// TestProject_HealthcheckUsesIPv4Loopback: inside alpine, `localhost`
// resolves to ::1 first while the scaffolded app listens on IPv4 only —
// the healthcheck must probe 127.0.0.1 (caught live by the Phase 8
// witness: running app, permanently unhealthy container).
func TestProject_HealthcheckUsesIPv4Loopback(t *testing.T) {
	cfg, g := loadVetangle(t)
	files, err := artifacts.Project(cfg, g, vetangleRoot)
	if err != nil {
		t.Fatal(err)
	}
	df := fileByPath(t, files, "deploy/Dockerfile.api")
	content := string(df.Content)
	if strings.Contains(content, "://localhost") {
		t.Error("healthcheck must use 127.0.0.1, not localhost (alpine resolves localhost to ::1)")
	}
	if !strings.Contains(content, "127.0.0.1") {
		t.Error("expected healthcheck against 127.0.0.1")
	}
	compose := fileByPath(t, files, "deploy/docker-compose.prod.yml")
	if strings.Contains(string(compose.Content), "://localhost") {
		t.Error("compose healthchecks must use 127.0.0.1, not localhost")
	}
}

func TestProject_NoDockerfileForInfraNodes(t *testing.T) {
	cfg, g := loadVetangle(t)
	files, err := artifacts.Project(cfg, g, vetangleRoot)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	for _, f := range files {
		if f.Path == "deploy/Dockerfile.db" {
			t.Error("db:postgres is an infra node using an official image — it must not get a Dockerfile")
		}
	}
}

func TestProject_DockerfileContent_MatchesDockerizableCapability(t *testing.T) {
	cfg, g := loadVetangle(t)
	files, err := artifacts.Project(cfg, g, vetangleRoot)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	f := fileByPath(t, files, "deploy/Dockerfile.api")
	content := string(f.Content)
	if !strings.Contains(content, "FROM golang:1.22-alpine AS builder") {
		t.Error("expected multi-stage build matching go.mod's go 1.22")
	}
	if !strings.Contains(content, "EXPOSE 8080") {
		t.Error("expected EXPOSE derived from the api node's port (8080)")
	}
	if !strings.Contains(content, "HEALTHCHECK") {
		t.Error("expected a HEALTHCHECK instruction")
	}
	if !f.Overwrite {
		t.Error("expected Overwrite=true — deploy artifacts are regenerated fresh every deploy")
	}
}

func TestProject_WorkerDockerfile_NoExposeOrHealthcheck(t *testing.T) {
	cfg, g := loadVetangle(t)
	files, err := artifacts.Project(cfg, g, vetangleRoot)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	f := fileByPath(t, files, "deploy/Dockerfile.worker")
	content := string(f.Content)
	if strings.Contains(content, "EXPOSE") {
		t.Error("worker is a queue-worker with no port — expected no EXPOSE")
	}
	if strings.Contains(content, "HEALTHCHECK") {
		t.Error("worker is a queue-worker with no port — expected no HEALTHCHECK")
	}
}

// ---------------------------------------------------------------------------
// docker-compose.prod.yml structure
// ---------------------------------------------------------------------------

func TestProject_EmitsComposeFile(t *testing.T) {
	cfg, g := loadVetangle(t)
	files, err := artifacts.Project(cfg, g, vetangleRoot)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	f := fileByPath(t, files, "deploy/docker-compose.prod.yml")
	if !f.Overwrite {
		t.Error("expected Overwrite=true for the compose file")
	}
}

func TestProject_Compose_ContainsExpectedServices(t *testing.T) {
	cfg, g := loadVetangle(t)
	files, err := artifacts.Project(cfg, g, vetangleRoot)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	cf := parseCompose(t, fileByPath(t, files, "deploy/docker-compose.prod.yml").Content)

	for _, want := range []string{"api", "worker", "db"} {
		if _, ok := cf.Services[want]; !ok {
			t.Errorf("expected service %q in compose file, got: %v", want, serviceNames(cf))
		}
	}
	for _, notWant := range []string{"web", "backoffice", "cache", "storage", "queue", "proxy"} {
		if _, ok := cf.Services[notWant]; ok {
			t.Errorf("did not expect service %q (excluded from this supported fixture or kernel-materialized)", notWant)
		}
	}
}

func serviceNames(cf composeFile) []string {
	names := make([]string, 0, len(cf.Services))
	for k := range cf.Services {
		names = append(names, k)
	}
	return names
}

func TestProject_Compose_ServiceUsesBuildDirective(t *testing.T) {
	cfg, g := loadVetangle(t)
	files, err := artifacts.Project(cfg, g, vetangleRoot)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	cf := parseCompose(t, fileByPath(t, files, "deploy/docker-compose.prod.yml").Content)
	api := cf.Services["api"]
	if api.Build == nil {
		t.Fatal("expected api service to use a build directive")
	}
	// The compose file lives at deploy/docker-compose.prod.yml and compose
	// resolves build.context relative to the file's directory — ../api, not
	// ./api (caught live by the Phase 8 witness: "path deploy/api not found").
	if api.Build.Context != "../api" {
		t.Errorf("expected build.context=../api (relative to deploy/), got %q", api.Build.Context)
	}
	if api.Build.Dockerfile != "../deploy/Dockerfile.api" {
		t.Errorf("expected build.dockerfile=../deploy/Dockerfile.api, got %q", api.Build.Dockerfile)
	}
}

func TestProject_Compose_PostgresUsesOfficialImage(t *testing.T) {
	cfg, g := loadVetangle(t)
	files, err := artifacts.Project(cfg, g, vetangleRoot)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	cf := parseCompose(t, fileByPath(t, files, "deploy/docker-compose.prod.yml").Content)
	db := cf.Services["db"]
	if db.Build != nil {
		t.Error("db:postgres must use an official image, not a build directive")
	}
	if db.Image != "postgres:16" {
		t.Errorf("expected image postgres:16 (vetangle fixture pins version 16), got %q", db.Image)
	}
}

func TestProject_Compose_PostgresHasNamedVolume(t *testing.T) {
	cfg, g := loadVetangle(t)
	files, err := artifacts.Project(cfg, g, vetangleRoot)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	cf := parseCompose(t, fileByPath(t, files, "deploy/docker-compose.prod.yml").Content)
	db := cf.Services["db"]
	if len(db.Volumes) != 1 {
		t.Fatalf("expected exactly one volume mount on db, got %v", db.Volumes)
	}
	if !strings.Contains(db.Volumes[0], "/var/lib/postgresql/data") {
		t.Errorf("expected volume mounted at postgres data dir, got %q", db.Volumes[0])
	}
	if len(cf.Volumes) == 0 {
		t.Error("expected the named volume declared at the top-level volumes: key")
	}
}

func TestProject_Compose_PostgresHealthcheckUsesPgIsready(t *testing.T) {
	cfg, g := loadVetangle(t)
	files, err := artifacts.Project(cfg, g, vetangleRoot)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	cf := parseCompose(t, fileByPath(t, files, "deploy/docker-compose.prod.yml").Content)
	db := cf.Services["db"]
	if db.Healthcheck == nil {
		t.Fatal("expected db healthcheck")
	}
	found := false
	for _, s := range db.Healthcheck.Test {
		if strings.Contains(s, "pg_isready") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected pg_isready in db healthcheck test, got %v", db.Healthcheck.Test)
	}
}

func TestProject_Compose_DependsOnServiceHealthyFromGraphEdges(t *testing.T) {
	cfg, g := loadVetangle(t)
	files, err := artifacts.Project(cfg, g, vetangleRoot)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	cf := parseCompose(t, fileByPath(t, files, "deploy/docker-compose.prod.yml").Content)

	api := cf.Services["api"]
	dep, ok := api.DependsOn["db"]
	if !ok {
		t.Fatal("expected api to depend_on db (graph has an api->db depends_on edge)")
	}
	if dep.Condition != "service_healthy" {
		t.Errorf("expected condition service_healthy, got %q", dep.Condition)
	}

	worker := cf.Services["worker"]
	if _, ok := worker.DependsOn["db"]; !ok {
		t.Error("expected worker to depend_on db (graph has a worker->db depends_on edge)")
	}

}

func TestProject_Compose_PortsPublishedOnlyForProxiedNodes(t *testing.T) {
	cfg, g := loadVetangle(t)
	files, err := artifacts.Project(cfg, g, vetangleRoot)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	cf := parseCompose(t, fileByPath(t, files, "deploy/docker-compose.prod.yml").Content)

	api := cf.Services["api"]
	if len(api.Ports) != 1 || api.Ports[0] != "8080:8080" {
		t.Errorf("expected api to publish 8080:8080 (it has a proxied_through edge), got %v", api.Ports)
	}

	worker := cf.Services["worker"]
	if len(worker.Ports) != 0 {
		t.Errorf("worker has no proxied_through edge and no port — expected no published ports, got %v", worker.Ports)
	}

	db := cf.Services["db"]
	if len(db.Ports) != 0 {
		t.Errorf("infra nodes are not proxied — expected no published ports for db, got %v", db.Ports)
	}
}

func TestProject_Compose_RestartUnlessStopped(t *testing.T) {
	cfg, g := loadVetangle(t)
	files, err := artifacts.Project(cfg, g, vetangleRoot)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	cf := parseCompose(t, fileByPath(t, files, "deploy/docker-compose.prod.yml").Content)
	for name, svc := range cf.Services {
		if svc.Restart != "unless-stopped" {
			t.Errorf("service %q: expected restart=unless-stopped, got %q", name, svc.Restart)
		}
	}
}

func TestProject_Compose_OneInternalNetworkSharedByAllServices(t *testing.T) {
	cfg, g := loadVetangle(t)
	files, err := artifacts.Project(cfg, g, vetangleRoot)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	cf := parseCompose(t, fileByPath(t, files, "deploy/docker-compose.prod.yml").Content)
	if len(cf.Networks) != 1 {
		t.Fatalf("expected exactly one top-level network, got %v", cf.Networks)
	}
	var networkName string
	for name := range cf.Networks {
		networkName = name
	}
	for name, svc := range cf.Services {
		if len(svc.Networks) != 1 || svc.Networks[0] != networkName {
			t.Errorf("service %q: expected networks=[%s], got %v", name, networkName, svc.Networks)
		}
	}
}

func TestProject_Compose_EnvUsesVarRefsNeverLiteralSecrets(t *testing.T) {
	cfg, g := loadVetangle(t)
	files, err := artifacts.Project(cfg, g, vetangleRoot)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	compose := fileByPath(t, files, "deploy/docker-compose.prod.yml")
	cf := parseCompose(t, compose.Content)

	api := cf.Services["api"]
	dbURL, ok := api.Environment["DATABASE_URL"]
	if !ok {
		t.Fatal("expected api environment to declare DATABASE_URL")
	}
	if dbURL != "${DATABASE_URL}" {
		t.Errorf("expected DATABASE_URL to be a ${VAR} reference, got %q", dbURL)
	}

	db := cf.Services["db"]
	for key, val := range db.Environment {
		if val != "${"+key+"}" {
			t.Errorf("expected db env %q to be a ${VAR} reference, got %q", key, val)
		}
	}

	// No file emitted by Project may contain a literal secret value the
	// dev-time postgres adapter uses (its hardcoded default password).
	for _, f := range files {
		if strings.Contains(string(f.Content), "POSTGRES_PASSWORD=postgres") ||
			strings.Contains(string(f.Content), "POSTGRES_PASSWORD: postgres\n") {
			t.Errorf("file %q contains a literal secret value", f.Path)
		}
	}
}

// ---------------------------------------------------------------------------
// Determinism
// ---------------------------------------------------------------------------

// TestProject_DockerfileUsesNodesActualGoModVersion: a plugin's dependency
// (e.g. observability's prometheus/client_golang) can bump a scaffolded
// node's go.mod `go` directive well past whatever version was true at
// scaffold time — caught live running a real `acthur deploy` against a
// project with the observability plugin added: `go mod tidy` bumped go.mod
// to "go 1.25.0" but the rendered Dockerfile still pinned
// "golang:1.22-alpine", so `go mod download` failed with "go.mod requires
// go >= 1.25.0". The Dockerfile must reflect the node's actual go.mod, not
// a version fixed at codegen time.
func TestProject_DockerfileUsesNodesActualGoModVersion(t *testing.T) {
	cfg, g := loadVetangle(t)

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "api"), 0o755); err != nil {
		t.Fatalf("mkdir api: %v", err)
	}
	goMod := "module example.com/api\n\ngo 1.25.0\n\nrequire github.com/gofiber/fiber/v2 v2.52.4\n"
	if err := os.WriteFile(filepath.Join(root, "api", "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	files, err := artifacts.Project(cfg, g, root)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	f := fileByPath(t, files, "deploy/Dockerfile.api")
	content := string(f.Content)
	if !strings.Contains(content, "FROM golang:1.25-alpine AS builder") {
		t.Errorf("expected builder image to track go.mod's go 1.25 directive, got:\n%s", content)
	}
}

func TestProject_DeterministicAcrossRuns(t *testing.T) {
	cfg, g := loadVetangle(t)
	files1, err := artifacts.Project(cfg, g, vetangleRoot)
	if err != nil {
		t.Fatalf("Project (run 1): %v", err)
	}
	files2, err := artifacts.Project(cfg, g, vetangleRoot)
	if err != nil {
		t.Fatalf("Project (run 2): %v", err)
	}
	if len(files1) != len(files2) {
		t.Fatalf("expected same file count across runs, got %d vs %d", len(files1), len(files2))
	}
	for _, f1 := range files1 {
		f2 := fileByPath(t, files2, f1.Path)
		if string(f1.Content) != string(f2.Content) {
			t.Errorf("file %q is not byte-identical across two runs", f1.Path)
		}
	}
}
