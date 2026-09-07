// Package hub implements an in-process publish/subscribe fan-out for live
// dashboard events. It removes the need for Redis in single-node deployments;
// a message broker can replace it when CAP scales to multiple nodes.
package hub

import "sync"

// Client is a registered subscriber. The send channel is buffered so a slow
// consumer never blocks the ingestion path.
type Client struct {
	send chan []byte
	hub  *Hub
	done chan struct{}
}

// Send queues a message, dropping it if the consumer is too slow.
func (c *Client) Send(msg []byte) bool {
	select {
	case c.send <- msg:
		return true
	default:
		return false
	}
}

// Messages returns the channel the WS writer goroutine reads from.
func (c *Client) Messages() <-chan []byte { return c.send }

// Done is closed when the client is removed from the hub.
func (c *Client) Done() <-chan struct{} { return c.done }

// Close unregisters the client.
func (c *Client) Close() { c.hub.unregister(c) }

// Hub broadcasts JSON messages to all registered clients.
type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
}

// New creates an empty hub.
func New() *Hub {
	return &Hub{clients: make(map[*Client]struct{})}
}

// Register adds a client and returns it.
func (h *Hub) Register() *Client {
	c := &Client{
		send: make(chan []byte, 256),
		hub:  h,
		done: make(chan struct{}),
	}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}

func (h *Hub) unregister(c *Client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.done)
		close(c.send)
	}
	h.mu.Unlock()
}

// Broadcast delivers msg to every client. It never blocks.
func (h *Hub) Broadcast(msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		c.Send(msg)
	}
}

// Count returns the number of connected clients.
func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
