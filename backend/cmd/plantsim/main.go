// Command plantsim is the demonstration process stand (TZ §7/§8): a physics
// model of the flotation plant behind a real Modbus TCP server, plus a small
// HTTP interface for scenario control. It exists only in demo deployments.
package main

import (
	"cap/internal/plantsim"
)

func main() {
	plantsim.Main()
}
