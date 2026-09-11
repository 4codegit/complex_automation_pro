package plantsim

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Modbus function codes served by the stand.
const (
	fcReadCoils       = 1
	fcReadDiscrete    = 2
	fcReadHolding     = 3
	fcReadInput       = 4
	fcWriteSingleReg  = 6
	exceptionFlag     = 0x80
	excIllegalFunc    = 1
	excIllegalAddress = 2
	excIllegalValue   = 3
	excServerFailure  = 4
)

// MBAP header size + PDU limits (Modbus TCP spec V1.1b3).
const (
	mbapHeaderLen = 7
	maxPDUSize    = 253
	maxReadQty    = 125
)

// ModbusServer serves the register image of a Model over Modbus TCP
// (unit id 1). Implementation is hand-rolled per TZ §8.7: no external
// dependencies beyond the standard library.
type ModbusServer struct {
	model  *Model
	unitID uint8

	ln      net.Listener
	closed  atomic.Bool
	conns   sync.WaitGroup
	connCnt atomic.Int64
}

// NewModbusServer binds addr and starts serving. Call Close to shut down.
func NewModbusServer(addr string, model *Model, unitID uint8) (*ModbusServer, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", addr, err)
	}
	s := &ModbusServer{model: model, unitID: unitID, ln: ln}
	go s.acceptLoop()
	return s, nil
}

// Addr returns the bound address (useful with :0 in tests).
func (s *ModbusServer) Addr() string { return s.ln.Addr().String() }

// Connections returns the current client connection count.
func (s *ModbusServer) Connections() int64 { return s.connCnt.Load() }

func (s *ModbusServer) acceptLoop() {
	for !s.closed.Load() {
		conn, err := s.ln.Accept()
		if err != nil {
			if s.closed.Load() {
				return
			}
			continue
		}
		s.conns.Add(1)
		s.connCnt.Add(1)
		go func() {
			defer s.conns.Done()
			defer s.connCnt.Add(-1)
			s.serveConn(conn)
		}()
	}
}

// Close shuts the server down and waits for connections to drain.
func (s *ModbusServer) Close() {
	s.closed.Store(true)
	_ = s.ln.Close()
	s.conns.Wait()
}

func (s *ModbusServer) serveConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Time{}) // long-lived connection
	for {
		head := make([]byte, mbapHeaderLen)
		if _, err := io.ReadFull(conn, head); err != nil {
			return
		}
		txnID := binary.BigEndian.Uint16(head[0:2])
		protoID := binary.BigEndian.Uint16(head[2:4])
		length := binary.BigEndian.Uint16(head[4:6])
		unitID := head[6]
		if protoID != 0 || length < 2 || length > maxPDUSize+2 {
			return // malformed framing: drop the connection
		}
		pdu := make([]byte, length-1)
		if _, err := io.ReadFull(conn, pdu); err != nil {
			return
		}
		// Scenario 4 (communication loss): the request is drained but never
		// answered — and once the outage is over the stale request stays
		// discarded, exactly like a dead device. Clients must recover by
		// timing out and retrying.
		if s.model.CommLoss() {
			for s.model.CommLoss() {
				time.Sleep(50 * time.Millisecond)
			}
			continue
		}
		if unitID != s.unitID && unitID != 0xFF {
			resp := buildException(txnID, unitID, pdu[0], excServerFailure)
			s.writeResp(conn, resp)
			continue
		}

		resp := s.handle(txnID, unitID, pdu)
		if err := s.writeResp(conn, resp); err != nil {
			return
		}
	}
}

func (s *ModbusServer) writeResp(conn net.Conn, resp []byte) error {
	_, err := conn.Write(resp)
	return err
}

// handle dispatches one request PDU and builds the response PDU (with MBAP).
// A nil response means "do not answer" (communication-loss scenario).
func (s *ModbusServer) handle(txnID uint16, unitID uint8, pdu []byte) []byte {
	if len(pdu) < 1 {
		return buildException(txnID, unitID, 0, excIllegalFunc)
	}
	fc := pdu[0]
	switch fc {
	case fcReadHolding, fcReadInput:
		if len(pdu) != 5 {
			return buildException(txnID, unitID, fc, excServerFailure)
		}
		addr := binary.BigEndian.Uint16(pdu[1:3])
		qty := binary.BigEndian.Uint16(pdu[3:5])
		if qty < 1 || qty > maxReadQty {
			return buildException(txnID, unitID, fc, excIllegalValue)
		}
		regs, err := s.readRegs(fc, addr, qty)
		if err != nil {
			return buildException(txnID, unitID, fc, excIllegalAddress)
		}
		return buildReadResponse(txnID, unitID, fc, regs)

	case fcWriteSingleReg:
		if len(pdu) != 5 {
			return buildException(txnID, unitID, fc, excServerFailure)
		}
		addr := binary.BigEndian.Uint16(pdu[1:3])
		value := binary.BigEndian.Uint16(pdu[3:5])
		if err := s.model.WriteHolding(addr, value); err != nil {
			return buildException(txnID, unitID, fc, excIllegalValue)
		}
		return buildWriteResponse(txnID, unitID, addr, value)

	case fcReadCoils, fcReadDiscrete:
		// The stand has no coil/discrete area (read-only sensors, HR writes):
		// answer with an illegal-function exception per the spec.
		return buildException(txnID, unitID, fc, excIllegalFunc)

	default:
		return buildException(txnID, unitID, fc, excIllegalFunc)
	}
}

func (s *ModbusServer) readRegs(fc byte, addr, qty uint16) ([]uint16, error) {
	img := s.model.Input()
	if fc == fcReadHolding {
		h := s.model.Holding()
		if int(addr)+int(qty) > NumHoldingRegisters {
			return nil, errors.New("out of range")
		}
		out := make([]uint16, qty)
		for i := 0; i < int(qty); i++ {
			out[i] = h[int(addr)+i]
		}
		return out, nil
	}
	if int(addr)+int(qty) > NumInputRegisters {
		return nil, errors.New("out of range")
	}
	out := make([]uint16, qty)
	for i := 0; i < int(qty); i++ {
		out[i] = img[int(addr)+i]
	}
	return out, nil
}

// buildReadResponse assembles MBAP+PDU for FC3/FC4 responses.
func buildReadResponse(txnID uint16, unitID, fc byte, regs []uint16) []byte {
	byteCount := len(regs) * 2
	pdu := make([]byte, 2+byteCount) // fc + byte count + data
	pdu[0] = fc
	pdu[1] = byte(byteCount)
	for i, r := range regs {
		binary.BigEndian.PutUint16(pdu[2+i*2:], r)
	}
	return frame(txnID, unitID, pdu)
}

func buildWriteResponse(txnID uint16, unitID byte, addr, value uint16) []byte {
	pdu := []byte{fcWriteSingleReg, 0, 0, 0, 0}
	binary.BigEndian.PutUint16(pdu[1:3], addr)
	binary.BigEndian.PutUint16(pdu[3:5], value)
	return frame(txnID, unitID, pdu)
}

func buildException(txnID uint16, unitID, fc, code byte) []byte {
	return frame(txnID, unitID, []byte{fc | exceptionFlag, code})
}

// frame wraps a PDU in the MBAP header.
func frame(txnID uint16, unitID byte, pdu []byte) []byte {
	out := make([]byte, mbapHeaderLen+len(pdu))
	binary.BigEndian.PutUint16(out[0:2], txnID)
	binary.BigEndian.PutUint16(out[2:4], 0) // protocol id
	binary.BigEndian.PutUint16(out[4:6], uint16(len(pdu)+1))
	out[6] = unitID
	copy(out[7:], pdu)
	return out
}
