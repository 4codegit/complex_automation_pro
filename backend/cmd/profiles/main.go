// Command profiles is the ore-profile microservice: draft/approve/activate
// change control over process profiles and thresholds.
package main

import (
	"log"

	"cap/internal/service"
)

func main() {
	if err := service.Run(service.Options{
		Name:     "profiles",
		Sections: []string{"profiles"},
	}); err != nil {
		log.Fatalf("%v", err)
	}
}
