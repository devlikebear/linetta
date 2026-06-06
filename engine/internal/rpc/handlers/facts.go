package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	tarstools "github.com/devlikebear/tars/pkg/tools"
)

const directFactSnippetRunes = 240

type createFactFromURLParams struct {
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id,omitempty"`
	Claim     string `json:"claim"`
	Result    string `json:"result,omitempty"`
	Status    string `json:"status,omitempty"`
	Category  string `json:"category,omitempty"`
	URL       string `json:"url"`
}

type fetchedFactSource struct {
	URL     string
	Title   string
	Content string
}

type factURLFetcher func(context.Context, string) (fetchedFactSource, error)

func CreateFact(repo *fact.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in fact.NewInput
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		card, err := repo.Create(ctx, now(), in)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		return json.Marshal(card)
	}
}

func CreateFactFromURL(repo *fact.Repo, now Clock, fetch factURLFetcher) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in createFactFromURLParams
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if strings.TrimSpace(in.ProjectID) == "" || strings.TrimSpace(in.Claim) == "" || strings.TrimSpace(in.URL) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id, claim and url required"}
		}
		if fetch == nil {
			fetch = defaultFactURLFetcher
		}
		fetched, err := fetch(ctx, in.URL)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		sourceURL := strings.TrimSpace(fetched.URL)
		if sourceURL == "" {
			sourceURL = strings.TrimSpace(in.URL)
		}
		snippet := trimRunes(strings.Join(strings.Fields(fetched.Content), " "), directFactSnippetRunes)
		result := strings.TrimSpace(in.Result)
		if result == "" {
			result = "직접 입력한 출처 URL에서 확인했습니다."
			if snippet != "" {
				result += " " + snippet
			}
		}
		status := strings.TrimSpace(in.Status)
		if status == "" {
			status = fact.StatusUncertain
		}
		var nodeID *string
		if strings.TrimSpace(in.NodeID) != "" {
			v := strings.TrimSpace(in.NodeID)
			nodeID = &v
		}
		card, err := repo.Create(ctx, now(), fact.NewInput{
			ProjectID: strings.TrimSpace(in.ProjectID),
			NodeID:    nodeID,
			Claim:     strings.TrimSpace(in.Claim),
			Result:    result,
			Status:    status,
			Category:  strings.TrimSpace(in.Category),
			Sources: []fact.SourceInput{{
				URL:     sourceURL,
				Title:   strings.TrimSpace(fetched.Title),
				Snippet: snippet,
			}},
		})
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		return json.Marshal(card)
	}
}

func defaultFactURLFetcher(ctx context.Context, rawURL string) (fetchedFactSource, error) {
	tool := tarstools.NewWebFetchTool(true)
	payload, err := json.Marshal(map[string]any{"url": strings.TrimSpace(rawURL), "max_chars": 4000})
	if err != nil {
		return fetchedFactSource{}, err
	}
	result, err := tool.Execute(ctx, json.RawMessage(payload))
	if err != nil {
		return fetchedFactSource{}, err
	}
	var body struct {
		URL     string `json:"url"`
		Content string `json:"content"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(result.Text()), &body); err != nil {
		return fetchedFactSource{}, fmt.Errorf("decode web_fetch response failed: %w", err)
	}
	if result.IsError {
		if strings.TrimSpace(body.Message) != "" {
			return fetchedFactSource{}, errors.New(body.Message)
		}
		return fetchedFactSource{}, errors.New("web_fetch failed")
	}
	u := strings.TrimSpace(body.URL)
	if u == "" {
		u = strings.TrimSpace(rawURL)
	}
	return fetchedFactSource{URL: u, Title: titleFromURL(u), Content: body.Content}, nil
}

func titleFromURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" {
		return strings.TrimSpace(raw)
	}
	return u.Hostname()
}

func trimRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	count := 0
	for idx := range value {
		if count == limit {
			return strings.TrimSpace(value[:idx])
		}
		count++
	}
	return strings.TrimSpace(value)
}

func ListFacts(repo *fact.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var f fact.ListFilter
		if err := json.Unmarshal(params, &f); err != nil || f.ProjectID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		list, err := repo.List(ctx, f)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if list == nil {
			list = []fact.Card{}
		}
		return json.Marshal(list)
	}
}

func UpdateFact(repo *fact.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in fact.UpdateInput
		if err := json.Unmarshal(params, &in); err != nil || in.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		card, err := repo.Update(ctx, now(), in)
		if errors.Is(err, fact.ErrNotFound) {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "fact card not found"}
		}
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		return json.Marshal(card)
	}
}

func DeleteFact(repo *fact.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := repo.Delete(ctx, p.ID); err != nil {
			if errors.Is(err, fact.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "fact card not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}
