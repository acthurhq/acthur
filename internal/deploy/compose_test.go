package deploy_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/acthur/acthur/internal/deploy"
)

type fakeRunner struct {
	calls   [][]string
	fail    map[string]error  // command word → error
	outputs map[string]string // command word → stdout
}

func (f *fakeRunner) Run(args ...string) (string, error) {
	f.calls = append(f.calls, args)
	key := strings.Join(args, " ")
	for word, err := range f.fail {
		if strings.Contains(key, word) {
			return "", err
		}
	}
	for word, out := range f.outputs {
		if strings.Contains(key, word) {
			return out, nil
		}
	}
	return "", nil
}

// TestComposeTarget_Up_BuildsAndStarts: Up runs docker compose with the
// project's prod file, detached, building images.
func TestComposeTarget_Up_BuildsAndStarts(t *testing.T) {
	r := &fakeRunner{outputs: map[string]string{"ps": `[{"Service":"api","Health":"healthy"}]`}}
	target := deploy.NewComposeTarget("deploy/docker-compose.prod.yml", r.Run)

	if err := target.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}

	joined := ""
	for _, c := range r.calls {
		joined += strings.Join(c, " ") + "\n"
	}
	for _, want := range []string{"compose", "-f deploy/docker-compose.prod.yml", "up", "-d", "--build"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected docker compose invocation to include %q, got:\n%s", want, joined)
		}
	}
}

// TestComposeTarget_Up_SurfacesFailure: a failing compose up is a pointed
// error, not a silent success.
func TestComposeTarget_Up_SurfacesFailure(t *testing.T) {
	r := &fakeRunner{fail: map[string]error{"up": fmt.Errorf("boom: port already allocated")}}
	target := deploy.NewComposeTarget("deploy/docker-compose.prod.yml", r.Run)
	err := target.Up()
	if err == nil || !strings.Contains(err.Error(), "port already allocated") {
		t.Fatalf("expected surfaced compose failure, got: %v", err)
	}
}

// TestComposeTarget_Status_ReportsServices: Status parses `compose ps` JSON.
func TestComposeTarget_Status_ReportsServices(t *testing.T) {
	r := &fakeRunner{outputs: map[string]string{"ps": `{"Service":"api","State":"running","Health":"healthy"}
{"Service":"db","State":"running","Health":"healthy"}`}}
	target := deploy.NewComposeTarget("deploy/docker-compose.prod.yml", r.Run)

	services, err := target.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(services) != 2 {
		t.Fatalf("expected 2 services, got %+v", services)
	}
	if services[0].Service != "api" || services[0].Health != "healthy" {
		t.Errorf("unexpected first service: %+v", services[0])
	}
}

// TestComposeTarget_Down: teardown delegates to compose down.
func TestComposeTarget_Down(t *testing.T) {
	r := &fakeRunner{}
	target := deploy.NewComposeTarget("deploy/docker-compose.prod.yml", r.Run)
	if err := target.Down(); err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, c := range r.calls {
		joined += strings.Join(c, " ")
	}
	if !strings.Contains(joined, "down") {
		t.Errorf("expected compose down, got: %s", joined)
	}
}
