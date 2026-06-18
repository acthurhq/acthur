// Acthur — Runtime Graph Operating System with Pluggable Infrastructure Nodes
//
// Usage:
//
//	acthur new my-project    create a new project
//	acthur init              adopt an existing project
//	acthur dev               start the development environment
//	acthur deploy            deploy to production
//	acthur --help            show all commands
//
// Build with version info:
//
//	go build -ldflags "-X github.com/acthur/acthur/cmd/acthur.Version=0.1.0" ./cmd/acthur
package main

func main() {
	Execute()
}
