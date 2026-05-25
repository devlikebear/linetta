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

// Notifier sends JSONRPC notifications on the active stdio connection.
// Returns an error if the server is not currently serving.
type Notifier interface {
	Notify(method string, params any) error
}

type serverNotifier struct{ s *Server }

func (n *serverNotifier) Notify(method string, params any) error {
	var raw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		raw = b
	}
	n.s.writeMu.Lock()
	defer n.s.writeMu.Unlock()
	if n.s.codec == nil {
		return errors.New("rpc: server is not serving")
	}
	return n.s.codec.Write(Message{Method: method, Params: raw})
}

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
	writeMu  sync.Mutex
	codec    *Codec // set during Serve; nil otherwise
}

// NewServer returns a Server with no handlers registered.
func NewServer() *Server { return &Server{handlers: map[string]Handler{}} }

// Notifier returns a handle for sending notifications on the active connection.
// The same handle is valid across multiple Serve calls.
func (s *Server) Notifier() Notifier { return &serverNotifier{s: s} }

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
	s.writeMu.Lock()
	s.codec = codec
	s.writeMu.Unlock()
	defer func() {
		s.writeMu.Lock()
		s.codec = nil
		s.writeMu.Unlock()
	}()

	write := func(m Message) {
		s.writeMu.Lock()
		defer s.writeMu.Unlock()
		_ = codec.Write(m)
	}

	for {
		msg, err := codec.Read()
		if errors.Is(err, io.EOF) {
			return io.EOF
		}
		if err != nil {
			write(Message{ID: json.RawMessage(`null`), Error: &Error{Code: CodeParseError, Message: err.Error()}})
			continue
		}

		isNotification := len(msg.ID) == 0
		s.mu.RLock()
		h, ok := s.handlers[msg.Method]
		s.mu.RUnlock()

		if !ok {
			if !isNotification {
				write(Message{ID: msg.ID, Error: &Error{Code: CodeMethodNotFound, Message: "method not found: " + msg.Method}})
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
				write(Message{ID: msg.ID, Error: &Error{Code: me.Code, Message: me.Message, Data: me.Data}})
			} else {
				write(Message{ID: msg.ID, Error: &Error{Code: CodeInternalError, Message: herr.Error()}})
			}
			continue
		}
		write(Message{ID: msg.ID, Result: result})
	}
}
