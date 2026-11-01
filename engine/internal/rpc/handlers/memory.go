package handlers

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/agentmemory"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// MemoryStore is the slice of agentmemory.Repo these handlers need. An
// interface, not the repo: handlers must never link tars/pkg/llm, and keeping
// the dependency abstract is how that stays true by construction.
type MemoryStore interface {
	Load(ctx context.Context, scope agentmemory.Scope, projectID string) (agentmemory.Document, error)
	Save(ctx context.Context, scope agentmemory.Scope, projectID, body string, now int64) (agentmemory.Document, error)
	ListArchived(ctx context.Context, projectID string, limit int) ([]agentmemory.Archived, error)
	DeleteArchived(ctx context.Context, projectID, id string) error
	RestoreArchived(ctx context.Context, projectID, id string, now int64) (agentmemory.Document, error)
}

// memoryChangedPayload mirrors mcphost's memoryChangedPayload
// (mcphost/tools_write.go): a listener must not have to handle two shapes for
// the same notification method.
type memoryChangedPayload struct {
	Scope     string `json:"scope"`
	ProjectID string `json:"project_id,omitempty"`
	Source    string `json:"source"`
}

type getMemoryParams struct {
	ProjectID string `json:"project_id"`
}

type getMemoryResult struct {
	WriterProfile agentmemory.Document `json:"writer_profile"`
	WorkNotes     agentmemory.Document `json:"work_notes"`
}

// GetMemory returns a handler for memory.get: both documents in one call, so
// the Settings panel and the story brief never have to round-trip twice for
// what is really one screen's worth of state.
func GetMemory(store MemoryStore) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p getMemoryParams
		if len(params) > 0 {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
			}
		}
		projectID := strings.TrimSpace(p.ProjectID)

		profile, err := store.Load(ctx, agentmemory.ScopeWriterProfile, "")
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		// No work picked yet is a legitimate UI state, not an error: the
		// Settings pane opens before the writer has opened a work, and this
		// is the first call it makes. The store is right to refuse
		// ScopeWorkNotes with no work id — a row whose project_id is NULL is
		// the *profile*, so asking it for work notes without a work is a
		// programming error down there, and agentmemory's own tests pin that
		// refusal. This layer is the one that knows the difference, so it
		// answers with the empty document itself rather than asking. The
		// budget still comes back, because the pane draws its capacity line
		// from it before anything is typed.
		notes := agentmemory.Document{
			Scope:       agentmemory.ScopeWorkNotes,
			CharsBudget: agentmemory.ScopeWorkNotes.Budget(),
		}
		if projectID != "" {
			notes, err = store.Load(ctx, agentmemory.ScopeWorkNotes, projectID)
			if err != nil {
				return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
			}
		}
		return json.Marshal(getMemoryResult{WriterProfile: profile, WorkNotes: notes})
	}
}

type setMemoryParams struct {
	Scope     string `json:"scope"`
	ProjectID string `json:"project_id"`
	Body      string `json:"body"`
}

// SetMemory returns a handler for memory.set: a whole-document replace, which
// is what the Settings textarea needs. It emits memory.changed with source
// "writer": the vocabulary in mcphost/activity.go:105-108 is external/agent,
// and a save made by the person at the keyboard is neither. A nil notify is
// tolerated so callers that do not care about the event (tests, callers with
// no live connection) do not have to fake one.
func SetMemory(store MemoryStore, clock func() int64, notify func(method string, params any)) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p setMemoryParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		scope, err := agentmemory.ParseScope(p.Scope)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		projectID := strings.TrimSpace(p.ProjectID)

		saved, err := store.Save(ctx, scope, projectID, p.Body, clock())
		if err != nil {
			// A save fails for reasons the writer can act on: an unscreenable
			// character, a markdown heading, or the document is over budget.
			// None of those are a server problem, so this is InvalidParams
			// with the underlying message (which names what to fix), not an
			// opaque 500.
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if notify != nil {
			notify("memory.changed", memoryChangedPayload{
				Scope: string(scope), ProjectID: projectID, Source: "writer",
			})
		}
		return json.Marshal(saved)
	}
}

type archiveListParams struct {
	ProjectID string `json:"project_id"`
}

type archiveItemParams struct {
	ProjectID string `json:"project_id"`
	ID        string `json:"id"`
}

// ListMemoryArchive returns a handler for memory.archive.list. No work picked
// is the pane's ordinary first state, as in GetMemory, so it answers empty.
func ListMemoryArchive(store MemoryStore) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p archiveListParams
		if len(params) > 0 {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
			}
		}
		projectID := strings.TrimSpace(p.ProjectID)
		if projectID == "" {
			return json.Marshal([]agentmemory.Archived{})
		}
		list, err := store.ListArchived(ctx, projectID, 0)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(list)
	}
}

// DeleteMemoryArchive returns a handler for memory.archive.delete.
func DeleteMemoryArchive(store MemoryStore) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p archiveItemParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if err := store.DeleteArchived(ctx, p.ProjectID, p.ID); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		return json.Marshal(map[string]bool{"ok": true})
	}
}

// RestoreMemoryArchive returns a handler for memory.archive.restore. It emits
// memory.changed like memory.set does: the notes moved under an open pane. A
// full document is InvalidParams for memory.set's reason: the message names
// what the writer can do about it.
func RestoreMemoryArchive(store MemoryStore, clock func() int64, notify func(method string, params any)) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p archiveItemParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		doc, err := store.RestoreArchived(ctx, p.ProjectID, p.ID, clock())
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if notify != nil {
			notify("memory.changed", memoryChangedPayload{
				Scope: string(agentmemory.ScopeWorkNotes), ProjectID: strings.TrimSpace(p.ProjectID), Source: "writer",
			})
		}
		return json.Marshal(doc)
	}
}
