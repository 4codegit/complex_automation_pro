// Command capd is the single server binary of the platform (TZ §4): ingestion,
// historian, alarms, profiles, registry, identity, supervisory control,
// metallurgical balance and the embedded dashboard SPA, all in one process
// behind one HTTP port.
package main

import (
	"log"

	"cap/internal/service"
)

func main() {
	if err := service.Run(); err != nil {
		log.Fatalf("%v", err)
	}
}
