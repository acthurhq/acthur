// Package rbac is the built-in "rbac" plugin: role-based access control for
// The go:fiber projects. It depends on the "auth" plugin (built in a parallel
// slice) for authentication, but never imports auth's package or generated
// code — see the ASSUMPTION note on the generated internal/rbac package
// (rbac.go's Generate) for the one contract point between the two.
package rbac

import (
	"github.com/acthurhq/acthur/internal/plugin"
)

type rbacPlugin struct{}

func (rbacPlugin) Name() string    { return "rbac" }
func (rbacPlugin) Version() string { return "0.1.0" }

// DependsOn requires "auth" to load first. rbac's generated middleware reads
// an authenticated user id from fiber context locals — see the ASSUMPTION
// comment in templates/models.go.tmpl — but that is a runtime-contract
// assumption about the generated code, not a compile-time dependency on the
// auth plugin's Go package.
func (rbacPlugin) DependsOn() []string { return []string{"auth"} }

func (rbacPlugin) Register(k plugin.KernelAPI) error {
	k.RegisterGenerator("rbac", generator{})
	return nil
}

func init() {
	plugin.Register(rbacPlugin{})
}
