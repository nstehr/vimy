package ipc

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
)

// Envelope is the wire format shared with the OpenRA mod.
// Data is kept as RawMessage so handlers can defer deserialization to the concrete type.
type Envelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func NewEnvelope(msgType string, data any) (Envelope, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return Envelope{}, fmt.Errorf("marshal data: %w", err)
	}
	return Envelope{Type: msgType, Data: raw}, nil
}

// ReadEnvelope reads a single length-prefixed JSON envelope from the connection.
// The 4-byte LE prefix matches the C# BinaryWriter on the OpenRA side.
func ReadEnvelope(conn net.Conn) (Envelope, error) {
	var length uint32
	if err := binary.Read(conn, binary.LittleEndian, &length); err != nil {
		return Envelope{}, fmt.Errorf("read length: %w", err)
	}

	// Guard against corrupted frames or malicious payloads.
	if length == 0 || length > 1<<20 {
		return Envelope{}, fmt.Errorf("invalid message length: %d", length)
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return Envelope{}, fmt.Errorf("read payload: %w", err)
	}

	var env Envelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return Envelope{}, fmt.Errorf("unmarshal envelope: %w", err)
	}

	return env, nil
}

func WriteEnvelope(conn net.Conn, env Envelope) error {
	payload, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	if err := binary.Write(conn, binary.LittleEndian, uint32(len(payload))); err != nil {
		return fmt.Errorf("write length: %w", err)
	}

	if _, err := conn.Write(payload); err != nil {
		return fmt.Errorf("write payload: %w", err)
	}

	return nil
}
