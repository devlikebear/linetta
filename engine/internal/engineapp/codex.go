package engineapp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/codexauth"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// codexService adapts *codexauth.Service to handlers.CodexService, and is the
// one place that turns a login failure into a reason code the pane can
// translate.
type codexService struct {
	svc *codexauth.Service
}

func (c codexService) LoginStart(ctx context.Context) (json.RawMessage, error) {
	url, err := c.svc.Start(ctx)
	if err != nil {
		return nil, codexReason(err)
	}
	return json.Marshal(struct {
		AuthURL string `json:"auth_url"`
	}{url})
}

func (c codexService) LoginStatus(context.Context) (json.RawMessage, error) {
	return json.Marshal(c.svc.Status())
}

func (c codexService) Logout(context.Context) error {
	if err := c.svc.Logout(); err != nil {
		return codexReason(err)
	}
	return nil
}

// codexReason maps the login package's sentinels onto reason codes. Anything
// unrecognised stays an internal error rather than being dressed up as a
// failure the writer can act on.
func codexReason(err error) error {
	switch {
	case errors.Is(err, codexauth.ErrPortInUse):
		return &rpc.ReasonError{Reason: rpc.ReasonCodexPortInUse, Err: err}
	case errors.Is(err, codexauth.ErrLoginFailed):
		return &rpc.ReasonError{Reason: rpc.ReasonCodexLoginFailed, Err: err}
	default:
		return err
	}
}
