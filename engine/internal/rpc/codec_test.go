package rpc

import (
	"bytes"
	"strings"
	"testing"
)

func TestReadMessage_singleLine(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")
	c := NewCodec(in, &bytes.Buffer{})

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if msg.Method != "ping" {
		t.Errorf("method = %q, want %q", msg.Method, "ping")
	}
	if msg.ID == nil || string(msg.ID) != "1" {
		t.Errorf("id raw = %q, want %q", string(msg.ID), "1")
	}
}

func TestReadMessage_eof(t *testing.T) {
	in := strings.NewReader("")
	c := NewCodec(in, &bytes.Buffer{})

	_, err := c.Read()
	if err == nil {
		t.Fatal("Read on empty stream should return io.EOF, got nil")
	}
}

func TestWriteMessage_appendsNewline(t *testing.T) {
	out := &bytes.Buffer{}
	c := NewCodec(strings.NewReader(""), out)

	if err := c.Write(Message{
		JSONRPC: "2.0",
		ID:      RawID(`7`),
		Result:  RawJSON(`"pong"`),
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got := out.String()
	want := `{"jsonrpc":"2.0","id":7,"result":"pong"}` + "\n"
	if got != want {
		t.Errorf("write: got %q, want %q", got, want)
	}
}

func TestWriteMessage_notification(t *testing.T) {
	out := &bytes.Buffer{}
	c := NewCodec(strings.NewReader(""), out)

	if err := c.Write(Message{
		JSONRPC: "2.0",
		Method:  "ai.delta",
		Params:  RawJSON(`{"text":"hi"}`),
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got := out.String()
	want := `{"jsonrpc":"2.0","method":"ai.delta","params":{"text":"hi"}}` + "\n"
	if got != want {
		t.Errorf("write: got %q, want %q", got, want)
	}
}
