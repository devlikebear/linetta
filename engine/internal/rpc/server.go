package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
)

// Handler is the signature for a JSONRPC method implementation. It receives a
// context (cancelled when Serve returns) and the raw params; it must return
// either a JSON-encoded result or an error.
type Handler func(ctx context.Context, params json.RawMessage) (json.RawMessage, error)

// Standard JSONRPC error codes (subset).
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// MethodError lets a handler return a typed JSONRPC error.
type MethodError struct {
	Code    int
	Message string
	Data    json.RawMessage
}

func (e *MethodError) Error() string { return e.Message }

// Server is a tiny JSONRPC 2.0 server dispatching to in-process handlers.
// One Server instance is bound to one stdio pair via Serve.
type Server struct {
	mu       sync.RWMutex
	handlers map[string]Handler
}

// NewServer returns a Server with no handlers registered.
func NewServer() *Server { return &Server{handlers: map[string]Handler{}} }

// Handle registers a handler for a method. Overwrites previous registrations
// silently — registration happens once at boot.
func (s *Server) Handle(method string, h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[method] = h
}

// Serve reads one message at a time from r and writes responses to w. It
// returns io.EOF when r is exhausted. Notifications (id == nil) never get a
// response. Each request is dispatched on the calling goroutine — concurrency
// is the writer's responsibility to add later if needed.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	codec := NewCodec(r, w)
	for {
		msg, err := codec.Read()
		if errors.Is(err, io.EOF) {
			return io.EOF
		}
		if err != nil {
			// Best-effort parse-error response with null id.
			_ = codec.Write(Message{
				ID:    json.RawMessage(`null`),
				Error: &Error{Code: CodeParseError, Message: err.Error()},
			})
			continue
		}

		isNotification := len(msg.ID) == 0
		s.mu.RLock()
		h, ok := s.handlers[msg.Method]
		s.mu.RUnlock()

		if !ok {
			if !isNotification {
				_ = codec.Write(Message{
					ID:    msg.ID,
					Error: &Error{Code: CodeMethodNotFound, Message: "method not found: " + msg.Method},
				})
			}
			continue
		}

		result, herr := h(ctx, msg.Params)
		if isNotification {
			continue
		}
		if herr != nil {
			var me *MethodError
			if errors.As(herr, &me) {
				_ = codec.Write(Message{ID: msg.ID, Error: &Error{Code: me.Code, Message: me.Message, Data: me.Data}})
			} else {
				_ = codec.Write(Message{ID: msg.ID, Error: &Error{Code: CodeInternalError, Message: herr.Error()}})
			}
			continue
		}
		_ = codec.Write(Message{ID: msg.ID, Result: result})
	}
}
