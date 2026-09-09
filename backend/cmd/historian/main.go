// Command historian is the read-side microservice: telemetry history, latest
// values, aggregates and CSV reports.
package main

import (
	"log"

	"cap/internal/service"
)

func main() {
	if err := service.Run(service.Options{
		Name:     "historian",
		Sections: []string{"historian"},
	}); err != nil {
		log.Fatalf("%v", err)
	}
}
