// Package rpc implements a minimal newline-delimited JSONRPC 2.0 codec
// over arbitrary io.Reader/Writer pairs (typically os.Stdin/os.Stdout).
package rpc

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
)

// RawJSON is a json.RawMessage alias for clarity in this package.
type RawJSON = json.RawMessage

// RawID is the raw bytes of a JSONRPC id, which may be a number or a string.
// Using json.RawMessage avoids forcing a type choice at the codec layer.
type RawID = json.RawMessage

// Message is a JSONRPC 2.0 frame — request, response, or notification.
// Only fields with non-zero values are emitted on the wire.
type Message struct {
	JSONRPC string  `json:"jsonrpc"`
	ID      RawID   `json:"id,omitempty"`
	Method  string  `json:"method,omitempty"`
	Params  RawJSON `json:"params,omitempty"`
	Result  RawJSON `json:"result,omitempty"`
	Error   *Error  `json:"error,omitempty"`
}

// Error is the JSONRPC error object.
type Error struct {
	Code    int     `json:"code"`
	Message string  `json:"message"`
	Data    RawJSON `json:"data,omitempty"`
}

// Codec reads and writes newline-delimited JSONRPC messages.
type Codec struct {
	r *bufio.Reader
	w io.Writer
}

// NewCodec returns a Codec backed by the given streams.
func NewCodec(r io.Reader, w io.Writer) *Codec {
	return &Codec{
		r: bufio.NewReaderSize(r, 1<<20), // 1 MiB; Tiptap docs fit comfortably
		w: w,
	}
}

// Read reads one message. Returns io.EOF when the stream is closed.
func (c *Codec) Read() (Message, error) {
	line, err := c.r.ReadBytes('\n')
	if len(line) == 0 && err != nil {
		return Message{}, err
	}
	// Strip the trailing newline (and possibly a stray CR on Windows pipes).
	for len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r') {
		line = line[:len(line)-1]
	}
	if len(line) == 0 {
		return Message{}, errors.New("rpc: empty message line")
	}
	var msg Message
	if err := json.Unmarshal(line, &msg); err != nil {
		return Message{}, err
	}
	return msg, nil
}

// Write writes one message followed by a newline.
func (c *Codec) Write(m Message) error {
	if m.JSONRPC == "" {
		m.JSONRPC = "2.0"
	}
	buf, err := json.Marshal(m)
	if err != nil {
		return err
	}
	buf = append(buf, '\n')
	_, err = c.w.Write(buf)
	return err
}
