package gofiber

import (
	"fmt"

	"github.com/acthurhq/acthur/internal/contract"
	"github.com/acthurhq/acthur/internal/plugin"
)

// Generate turns a parsed contract into the full go:fiber file set for the
// target node: DTOs, handlers, service + repository skeletons, route
// mounting, a contract-derived migration pair, and a test beside every
// source file. Paths are relative to the node dir except migrations/,
// which the write engine routes to the project root.
func Generate(c *contract.Contract, ctx plugin.GeneratorContext) ([]plugin.GeneratedFile, error) {
	if c.Name == "" {
		return nil, fmt.Errorf("contract has no name")
	}
	modulePath, _ := ctx.Extra["module_path"].(string)
	if modulePath == "" {
		return nil, fmt.Errorf("GeneratorContext.Extra[\"module_path\"] is required to derive import paths")
	}

	m, err := buildModel(c, modulePath)
	if err != nil {
		return nil, err
	}

	dir := "internal/" + m.Package + "/"
	src := func(path string, content []byte) plugin.GeneratedFile {
		return plugin.GeneratedFile{Path: dir + path, Content: content, Overwrite: true}
	}

	up, down := emitMigrations(m)
	files := []plugin.GeneratedFile{
		src("dto.go", emitDTO(m)),
		src("dto_test.go", emitDTOTest(m)),
		src("handler.go", emitHandler(m)),
		src("handler_test.go", emitHandlerTest(m)),
		src("service.go", emitService(m)),
		src("service_test.go", emitServiceTest(m)),
		src("repository.go", emitRepository(m)),
		src("repository_test.go", emitRepositoryTest(m)),
		src("routes.go", emitRoutes(m)),
		src("routes_test.go", emitRoutesTest(m)),
		{Path: "migrations/0400_" + m.Package + ".up.sql", Content: up, Overwrite: false},
		{Path: "migrations/0400_" + m.Package + ".down.sql", Content: down, Overwrite: false},
	}
	return files, nil
}
