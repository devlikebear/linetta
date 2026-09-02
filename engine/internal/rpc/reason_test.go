package rpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func reasonIn(t *testing.T, me *MethodError) string {
	t.Helper()
	var data struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(me.Data, &data); err != nil {
		t.Fatalf("data is not a reason payload: %s", me.Data)
	}
	return data.Reason
}

func TestMethodErrorFrom_reasonErrorBecomesInvalidParamsWithReason(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", &ReasonError{Reason: ReasonProviderAuthFailed, Err: errors.New("401")})
	me := MethodErrorFrom(err)
	if me.Code != CodeInvalidParams {
		t.Errorf("code = %d, want %d", me.Code, CodeInvalidParams)
	}
	if got := reasonIn(t, me); got != "provider_auth_failed" {
		t.Errorf("reason = %q", got)
	}
	if me.Message == "" {
		t.Error("message must survive for logs")
	}
}

func TestMethodErrorFrom_plainErrorIsInternal(t *testing.T) {
	me := MethodErrorFrom(errors.New("boom"))
	if me.Code != CodeInternalError || me.Data != nil {
		t.Errorf("got %+v, want internal error without reason data", me)
	}
}

func TestReasonError_unwrapsItsCause(t *testing.T) {
	cause := errors.New("cause")
	err := &ReasonError{Reason: ReasonProviderUnreachable, Err: cause}
	if !errors.Is(err, cause) {
		t.Error("ReasonError must unwrap to its cause")
	}
	if err.Error() != "provider_unreachable: cause" {
		t.Errorf("Error() = %q", err.Error())
	}
}
