// Command live serves the operator-facing surface in split mode: the embedded
// dashboard, the WebSocket feed, the development simulator and the internal
// event intake that the ingest service forwards accepted readings to. It is
// started first so it also migrates and seeds a fresh database.
package main

import (
	"log"

	"cap/internal/service"
)

func main() {
	if err := service.Run(service.Options{
		Name:     "live",
		Sections: []string{"live"},
		Seed:     true,
	}); err != nil {
		log.Fatalf("%v", err)
	}
}
