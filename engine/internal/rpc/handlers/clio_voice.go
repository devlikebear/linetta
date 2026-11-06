package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/clio/post"
	"github.com/devlikebear/linetta/engine/internal/clio/voice"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// ClioVoiceProfile returns a handler for voice.profile. platform and kind pick
// the context ("*" for all); the reply says which context it actually used in
// "fallback" when the one asked for had too few posts. The profile is computed
// on demand from every stored post.
func ClioVoiceProfile(repo *post.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			Platform string `json:"platform"`
			Kind     string `json:"kind"`
		}
		if len(params) > 0 && string(params) != "null" {
			if err := json.Unmarshal(params, &in); err != nil {
				return nil, clioInvalid("invalid params")
			}
		}
		if in.Platform == "" {
			in.Platform = post.PlatformX
		}
		if in.Kind == "" {
			in.Kind = "*"
		}
		if in.Platform != "*" && !clioPlatform(in.Platform) || in.Kind != "*" && !clioKind(in.Kind) {
			return nil, clioInvalid("unknown platform or kind")
		}
		posts, err := repo.List(ctx, post.ListParams{})
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(voice.Build(posts).For(in.Platform, in.Kind))
	}
}
