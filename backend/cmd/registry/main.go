// Command registry is the asset/tag microservice: the equipment hierarchy,
// registered signals and the gateway registry.
package main

import (
	"log"

	"cap/internal/service"
)

func main() {
	if err := service.Run(service.Options{
		Name:     "registry",
		Sections: []string{"registry"},
	}); err != nil {
		log.Fatalf("%v", err)
	}
}
