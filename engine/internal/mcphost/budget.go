//go:build !mobile

package mcphost

import (
	"context"
	"encoding/json"
	"slices"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/settings"
)

// ToolGroups says which optional tool groups a server registers (#99).
//
// Two groups, because the writer's two switches are what they answer to.
// Everything not in a group is unconditional: the manuscript tools are what
// an agent exists for, and there is no budget worth the writer losing them.
type ToolGroups struct {
	// Memory is linetta_edit_memory.
	Memory bool
	// Skills is linetta_read_skill and linetta_edit_skill together. They are
	// one group and not two on purpose: a write tool with no read tool
	// invites blind overwrites of a document, and a read tool alone is the
	// smaller half of a 3 kB pair. The writer's question is whether the agent
	// keeps skills at all.
	Skills bool
}

// AllToolGroups is the default — every group on, which is the tool set a
// writer who never opens the pane keeps.
func AllToolGroups() ToolGroups { return ToolGroups{Memory: true, Skills: true} }

// MemoryToolNames and SkillToolNames are each group's members. They are
// subsets of ReadToolNames and WriteToolNames rather than a separate
// inventory, and TestToolGroupNamesAreAllRealTools holds them to that: a
// group naming a tool that does not exist would silently discount a budget by
// bytes nothing was ever going to send.
var (
	MemoryToolNames = []string{"linetta_edit_memory"}
	SkillToolNames  = []string{"linetta_read_skill", "linetta_edit_skill"}
)

// inGroup reports whether name belongs to a group that groups has switched
// off, i.e. whether Register would leave it out.
func excludedByGroups(name string, groups ToolGroups) bool {
	if !groups.Memory && slices.Contains(MemoryToolNames, name) {
		return true
	}
	if !groups.Skills && slices.Contains(SkillToolNames, name) {
		return true
	}
	return false
}

// ToolNames returns the tools Register installs for a mode and a set of
// groups, in registration order.
//
// This is the one place the tool set is described in Go rather than
// discovered over the wire, and TestToolNamesMatchesWhatTheServerActuallyServes
// pins it to a real tools/list for every mode-and-group combination — so the
// Settings pane's count is the count the agent gets, and cannot drift from it
// the way a number typed into a marketing page does.
func ToolNames(mode string, groups ToolGroups) []string {
	names := make([]string, 0, len(ReadToolNames)+len(WriteToolNames))
	for _, n := range ReadToolNames {
		if !excludedByGroups(n, groups) {
			names = append(names, n)
		}
	}
	if mode == settings.MCPModeFull {
		for _, n := range WriteToolNames {
			if !excludedByGroups(n, groups) {
				names = append(names, n)
			}
		}
	}
	return names
}

// toolSchemaBytes is what one tool costs in a tools/list response, by name,
// measured once from a real server.
//
// Measured, not counted from the Go source: what travels is the marshalled
// mcp.Tool — name, description and the JSON schema the SDK derives from the
// input struct's fields and jsonschema tags — and no amount of reading
// tools_read.go tells you how many bytes that is. A schema the SDK renders
// differently after an upgrade shows up here immediately.
//
// Once, because the tool set is static for the life of the process and the
// measurement stands up a client/server pair. A failure yields an empty map,
// which surfaces as a zero budget the pane simply does not draw — a number
// nobody can measure must not become a number somebody made up.
var toolSchemaBytes = sync.OnceValue(measureToolSchemaBytes)

func measureToolSchemaBytes() map[string]int {
	ctx := context.Background()
	srv := mcp.NewServer(&mcp.Implementation{Name: ServerName, Version: ServerVersion}, nil)
	// Every group on and full mode: this measures the whole inventory once,
	// and any subset is a sum over ToolNames. ToolDeps carries only Settings,
	// nil here so the groups fall back to AllToolGroups — registration reads
	// no other collaborator and no handler runs.
	ToolDeps{}.Register(srv, settings.MCPModeFull)

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, serverTransport, nil)
	if err != nil {
		logf("tool budget: connect server: %v", err)
		return map[string]int{}
	}
	defer func() { _ = ss.Close() }()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "linetta-budget", Version: ServerVersion}, nil).
		Connect(ctx, clientTransport, nil)
	if err != nil {
		logf("tool budget: connect client: %v", err)
		return map[string]int{}
	}
	defer func() { _ = cs.Close() }()

	sizes := map[string]int{}
	for tool, err := range cs.Tools(ctx, nil) {
		if err != nil {
			logf("tool budget: list tools: %v", err)
			return map[string]int{}
		}
		body, err := json.Marshal(tool)
		if err != nil {
			logf("tool budget: marshal %s: %v", tool.Name, err)
			return map[string]int{}
		}
		sizes[tool.Name] = len(body)
	}
	return sizes
}

// MeasureToolBudget reports what the built-in agent's tool set costs with the
// writer's switches as they stand, plus what each group costs on its own.
//
// Full mode always: the agent registers the full set regardless of
// settings.MCPMode (that setting governs external clients), so this is the
// budget the pane is about — and the ceiling for any external client, which
// can only ever be offered fewer.
//
// It satisfies settings.ToolBudgetFunc, which is how it reaches settings.get
// without settings importing this //go:build !mobile package.
func MeasureToolBudget(memoryTools, skillTools bool) settings.ToolBudget {
	sizes := toolSchemaBytes()
	groups := ToolGroups{Memory: memoryTools, Skills: skillTools}

	sum := func(names []string) settings.ToolBudgetGroup {
		g := settings.ToolBudgetGroup{}
		for _, n := range names {
			if b, ok := sizes[n]; ok {
				g.Tools++
				g.Bytes += b
			}
		}
		return g
	}

	current := sum(ToolNames(settings.MCPModeFull, groups))
	return settings.ToolBudget{
		Tools:  current.Tools,
		Bytes:  current.Bytes,
		Memory: sum(MemoryToolNames),
		Skills: sum(SkillToolNames),
	}
}
