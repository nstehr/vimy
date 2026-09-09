package ipc

import (
	"log/slog"
	"net"
	"sync/atomic"
)

// Handler processes a received envelope. Return nil to send no reply.
type Handler func(env Envelope) (*Envelope, error)

// Connection is one OpenRA mod instance talking to the sidecar — one per player,
// identified after the hello handshake.
type Connection struct {
	conn     net.Conn
	handlers map[string]Handler
	// Envelopes written, ever. Engine.Evaluate samples it around an action to
	// tell one that ordered something from one that returned early.
	sent atomic.Uint64
}

func NewConnection(conn net.Conn, handlers map[string]Handler) *Connection {
	if handlers == nil {
		handlers = make(map[string]Handler)
	}
	return &Connection{
		conn:     conn,
		handlers: handlers,
	}
}

func (c *Connection) RegisterHandler(msgType string, handler Handler) {
	c.handlers[msgType] = handler
}

func (c *Connection) Send(msgType string, data any) error {
	env, err := NewEnvelope(msgType, data)
	if err != nil {
		return err
	}
	if err := WriteEnvelope(c.conn, env); err != nil {
		return err
	}
	c.sent.Add(1)
	return nil
}

// Sent is monotonic; only the difference between two readings means anything.
func (c *Connection) Sent() uint64 {
	return c.sent.Load()
}

// ReadLoop owns the connection's lifetime and blocks until it closes or errors.
func (c *Connection) ReadLoop() {
	defer c.conn.Close()

	for {
		env, err := ReadEnvelope(c.conn)
		if err != nil {
			slog.Info("connection read ended", "error", err)
			return
		}

		handler, ok := c.handlers[env.Type]
		if !ok {
			slog.Warn("no handler for message type", "type", env.Type)
			continue
		}

		resp, err := handler(env)
		if err != nil {
			slog.Error("handler error", "type", env.Type, "error", err)
			continue
		}

		if resp != nil {
			if err := WriteEnvelope(c.conn, *resp); err != nil {
				slog.Error("failed to send response", "type", resp.Type, "error", err)
				return
			}
			slog.Debug("sent response", "type", resp.Type)
		}
	}
}
