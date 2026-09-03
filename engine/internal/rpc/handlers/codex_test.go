package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

type fakeCodexService struct {
	start     json.RawMessage
	status    json.RawMessage
	err       error
	loggedOut bool
}

func (f *fakeCodexService) LoginStart(context.Context) (json.RawMessage, error) {
	return f.start, f.err
}
func (f *fakeCodexService) LoginStatus(context.Context) (json.RawMessage, error) {
	return f.status, f.err
}
func (f *fakeCodexService) Logout(context.Context) error {
	f.loggedOut = true
	return f.err
}

func TestCodexLoginStart_returnsTheAuthURL(t *testing.T) {
	svc := &fakeCodexService{start: json.RawMessage(`{"auth_url":"https://auth.openai.com/oauth/authorize?x=1"}`)}
	res, err := CodexLoginStart(svc)(context.Background(), nil)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if string(res) != `{"auth_url":"https://auth.openai.com/oauth/authorize?x=1"}` {
		t.Errorf("payload = %s", res)
	}
}

func TestCodexLoginStart_portInUseKeepsItsReasonCode(t *testing.T) {
	svc := &fakeCodexService{err: &rpc.ReasonError{
		Reason: rpc.ReasonCodexPortInUse, Err: errors.New("1455, 1457 busy"),
	}}
	_, err := CodexLoginStart(svc)(context.Background(), nil)
	var me *rpc.MethodError
	if !errors.As(err, &me) {
		t.Fatalf("want MethodError, got %v", err)
	}
	if me.Code != rpc.CodeInvalidParams {
		t.Errorf("code = %d", me.Code)
	}
	var data struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(me.Data, &data)
	if data.Reason != "codex_port_in_use" {
		t.Errorf("reason = %q", data.Reason)
	}
}

func TestCodexLoginStatus_passesTheServicePayload(t *testing.T) {
	svc := &fakeCodexService{status: json.RawMessage(`{"logged_in":true,"email":"w@example.com"}`)}
	res, err := CodexLoginStatus(svc)(context.Background(), nil)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if string(res) != `{"logged_in":true,"email":"w@example.com"}` {
		t.Errorf("payload = %s", res)
	}
}

func TestCodexLogout_callsTheServiceAndReportsOK(t *testing.T) {
	svc := &fakeCodexService{}
	res, err := CodexLogout(svc)(context.Background(), nil)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !svc.loggedOut {
		t.Error("the service was never asked to log out")
	}
	if string(res) != `{"ok":true}` {
		t.Errorf("payload = %s", res)
	}
}
