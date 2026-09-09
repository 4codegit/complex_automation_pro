// Command server is the all-in-one development bundle: every route section
// (ingestion, historian, alarms, profiles, registry, identity, live dashboard
// and simulator) in a single process. Split deployments run the individual
// services behind cmd/gateway-api instead; scripts/services.sh orchestrates
// them.
package main

import (
	"log"

	"cap/internal/service"
)

func main() {
	if err := service.Run(service.Options{Name: "server", Seed: true}); err != nil {
		log.Fatalf("%v", err)
	}
}
