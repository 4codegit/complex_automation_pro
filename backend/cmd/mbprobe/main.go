// mbprobe читает пару входных регистров (IR0, IR10) по Modbus TCP и печатает
// сырые и инженерные значения — быстрая проверка «отвечает ли телефон».
// Использование: go run ./cmd/mbprobe [<host>[:port]]   (по умолчанию 127.0.0.1:5020)
package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"strings"

	modbus "github.com/goburrow/modbus"
)

func main() {
	addr := "127.0.0.1:5020"
	if len(os.Args) > 1 {
		addr = os.Args[1]
		if !strings.Contains(addr, ":") {
			addr += ":5020"
		}
	}
	h := modbus.NewTCPClientHandler(addr)
	h.SlaveId = 1
	h.Timeout = 2e9
	if err := h.Connect(); err != nil {
		log.Fatalf("%s: %v", addr, err)
	}
	defer h.Close()
	c := modbus.NewClient(h)
	ir, err := c.ReadInputRegisters(0, 28)
	if err != nil {
		log.Fatalf("чтение IR0..27: %v", err)
	}
	fmt.Printf("%s отвечает, unit 1. IR0..IR27 (сырое): %v\n", addr, ir[:28])
	fmt.Printf("  IR0  x0.1  -> %.1f t/h (fi101)\n", float64(binary.BigEndian.Uint16(ir[0:2]))*0.1)
	fmt.Printf("  IR10 x0.01 -> %.2f pH  (ai301)\n", float64(binary.BigEndian.Uint16(ir[20:22]))*0.01)
	hr, err := c.ReadHoldingRegisters(0, 7)
	if err != nil {
		log.Fatalf("чтение HR0..6: %v", err)
	}
	fmt.Printf("  HR0..HR6 (сырое): %v\n", hr)
}
