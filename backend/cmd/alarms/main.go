// Command alarms is the alarm microservice: active alarms, acknowledgement
// and plant-approved per-tag limits (ISA-18.2 rationalisation).
package main

import (
	"log"

	"cap/internal/service"
)

func main() {
	if err := service.Run(service.Options{
		Name:     "alarms",
		Sections: []string{"alarms"},
	}); err != nil {
		log.Fatalf("%v", err)
	}
}
