package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

type fakeProviderService struct {
	list   json.RawMessage
	models []string
	err    error
	seenID string
}

func (f *fakeProviderService) List(context.Context) (json.RawMessage, error) { return f.list, f.err }
func (f *fakeProviderService) ListModels(_ context.Context, id string) ([]string, error) {
	f.seenID = id
	return f.models, f.err
}
func (f *fakeProviderService) Test(_ context.Context, id string) error {
	f.seenID = id
	return f.err
}

func TestProvidersList_returnsTheServicePayload(t *testing.T) {
	svc := &fakeProviderService{list: json.RawMessage(`[{"id":"anthropic","auth":"api_key"}]`)}
	res, err := ProvidersList(svc)(context.Background(), nil)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if string(res) != `[{"id":"anthropic","auth":"api_key"}]` {
		t.Errorf("payload = %s", res)
	}
}

func TestProvidersListModels_passesTheIdAndWrapsModels(t *testing.T) {
	svc := &fakeProviderService{models: []string{"a", "b"}}
	res, err := ProvidersListModels(svc)(context.Background(), json.RawMessage(`{"provider":"openai"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if svc.seenID != "openai" {
		t.Errorf("provider id = %q", svc.seenID)
	}
	var out struct {
		Models []string `json:"models"`
	}
	_ = json.Unmarshal(res, &out)
	if len(out.Models) != 2 {
		t.Errorf("models = %v", out.Models)
	}
}

func TestProvidersListModels_reasonErrorKeepsItsCode(t *testing.T) {
	svc := &fakeProviderService{err: &rpc.ReasonError{Reason: rpc.ReasonProviderAuthFailed, Err: errors.New("401")}}
	_, err := ProvidersListModels(svc)(context.Background(), json.RawMessage(`{"provider":"anthropic"}`))
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
	if data.Reason != "provider_auth_failed" {
		t.Errorf("reason = %q", data.Reason)
	}
}

func TestProvidersTest_okPayload(t *testing.T) {
	svc := &fakeProviderService{}
	res, err := ProvidersTest(svc)(context.Background(), json.RawMessage(`{"provider":"gemini-native"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if svc.seenID != "gemini-native" || string(res) != `{"ok":true}` {
		t.Errorf("id=%q payload=%s", svc.seenID, res)
	}
}
