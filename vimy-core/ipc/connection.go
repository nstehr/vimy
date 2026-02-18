package ipc

import (
	"log/slog"
	"net"
)

// Handler processes a received envelope. Return nil to send no reply.
type Handler func(env Envelope) (*Envelope, error)

// Connection represents a single OpenRA mod instance talking to the sidecar.
// Each game player gets its own connection, identified after the hello handshake.
type Connection struct {
	conn     net.Conn
	handlers map[string]Handler
	Player   string
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
	return WriteEnvelope(c.conn, env)
}

// ReadLoop blocks until the connection closes or errors. It owns the conn lifetime
// so callers don't need to track cleanup.
func (c *Connection) ReadLoop() {
	defer c.conn.Close()

	for {
		env, err := ReadEnvelope(c.conn)
		if err != nil {
			slog.Info("connection read ended", "player", c.Player, "error", err)
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
			slog.Info("sent response", "type", resp.Type, "player", c.Player)
		}
	}
}
