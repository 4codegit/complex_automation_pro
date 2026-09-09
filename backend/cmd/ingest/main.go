// Command ingest is the push-ingest microservice: gateways (and the simulator
// in hybrid setups) deliver canonical telemetry here. Accepted readings are
// forwarded to EVENT_SINKS so the live service keeps its WebSocket feed.
package main

import (
	"log"

	"cap/internal/service"
)

func main() {
	if err := service.Run(service.Options{
		Name:     "ingest",
		Sections: []string{"ingest"},
	}); err != nil {
		log.Fatalf("%v", err)
	}
}
