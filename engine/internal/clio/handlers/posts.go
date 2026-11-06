// Package handlers holds Clio's RPC methods. It is separate from the Linetta
// handlers so the Clio engine links none of the story domain packages.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/clio/post"
	"github.com/devlikebear/linetta/engine/internal/clio/voiceimport"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// posts.* serve Clio's import and library screens. The engine never reads the
// disk: the frontend reads the file and passes its content.

const (
	clioPreviewCount = 5
	clioListMax      = 500
	clioListDefault  = 50
)

func clioInvalid(msg string) error {
	return &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: msg}
}

type clioImportParams struct {
	FileName    string `json:"file_name"`
	Content     string `json:"content"`
	DefaultKind string `json:"default_kind"`
	Platform    string `json:"platform"`
	OwnUserID   string `json:"own_user_id"`
}

func parseImport(params json.RawMessage) (voiceimport.Result, error) {
	var in clioImportParams
	if err := json.Unmarshal(params, &in); err != nil {
		return voiceimport.Result{}, clioInvalid("invalid params")
	}
	if strings.TrimSpace(in.Content) == "" {
		return voiceimport.Result{}, clioInvalid("content is required")
	}
	res, err := voiceimport.Parse(in.FileName, []byte(in.Content), voiceimport.Options{
		DefaultKind: in.DefaultKind, Platform: in.Platform, OwnUserID: in.OwnUserID,
	})
	if err != nil {
		if errors.Is(err, voiceimport.ErrInvalidOptions) || errors.Is(err, voiceimport.ErrUnreadable) {
			return voiceimport.Result{}, clioInvalid(err.Error())
		}
		return voiceimport.Result{}, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
	}
	return res, nil
}

type clioPreviewResult struct {
	Total    int                 `json:"total"`
	Contexts []post.ContextCount `json:"contexts"`
	Skipped  map[string]int      `json:"skipped"`
	Preview  []post.NewPost      `json:"preview"`
}

// ImportPreview returns a handler for posts.import_preview: what an import
// would keep, without storing anything.
func ImportPreview() rpc.Handler {
	return func(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
		res, err := parseImport(params)
		if err != nil {
			return nil, err
		}
		out := clioPreviewResult{Total: len(res.Posts), Contexts: []post.ContextCount{}, Skipped: res.Skipped, Preview: res.Posts}
		if len(out.Preview) > clioPreviewCount {
			out.Preview = out.Preview[:clioPreviewCount]
		}
		idx := map[[2]string]int{}
		for _, p := range res.Posts {
			k := [2]string{p.Platform, p.Kind}
			i, ok := idx[k]
			if !ok {
				i = len(out.Contexts)
				idx[k] = i
				out.Contexts = append(out.Contexts, post.ContextCount{Platform: p.Platform, Kind: p.Kind})
			}
			out.Contexts[i].Count++
		}
		return json.Marshal(out)
	}
}

// ClioImport returns a handler for posts.import.
func Import(repo *post.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		res, err := parseImport(params)
		if err != nil {
			return nil, err
		}
		inserted, skipped, err := repo.InsertMany(ctx, res.Posts)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(struct {
			Inserted int `json:"inserted"`
			Skipped  int `json:"skipped"`
		}{inserted, skipped})
	}
}

// ListPosts returns a handler for posts.list.
func ListPosts(repo *post.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			Platform string `json:"platform"`
			Kind     string `json:"kind"`
			Limit    int    `json:"limit"`
			Offset   int    `json:"offset"`
		}
		if len(params) > 0 && string(params) != "null" {
			if err := json.Unmarshal(params, &in); err != nil {
				return nil, clioInvalid("invalid params")
			}
		}
		if in.Limit < 0 || in.Offset < 0 || in.Limit > clioListMax {
			return nil, clioInvalid("limit must be 0..500 and offset non-negative")
		}
		if in.Limit == 0 {
			in.Limit = clioListDefault
		}
		if in.Platform != "" && !clioPlatform(in.Platform) || in.Kind != "" && !clioKind(in.Kind) {
			return nil, clioInvalid("unknown platform or kind")
		}
		list, err := repo.List(ctx, post.ListParams{Platform: in.Platform, Kind: in.Kind, Limit: in.Limit, Offset: in.Offset})
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(list)
	}
}

// CountPosts returns a handler for posts.counts.
func CountPosts(repo *post.Repo) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		counts, err := repo.Counts(ctx)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(counts)
	}
}

func clioPlatform(p string) bool {
	return p == post.PlatformX || p == post.PlatformThreads || p == post.PlatformBlog
}

func clioKind(k string) bool { return k == post.KindPost || k == post.KindReply }
