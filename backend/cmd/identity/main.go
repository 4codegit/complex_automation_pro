// Command identity is the access-control microservice: RBAC roles,
// assignments, whoami and the immutable audit trail.
package main

import (
	"log"

	"cap/internal/service"
)

func main() {
	if err := service.Run(service.Options{
		Name:     "identity",
		Sections: []string{"identity"},
	}); err != nil {
		log.Fatalf("%v", err)
	}
}
