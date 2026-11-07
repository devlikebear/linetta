package agent

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/devlikebear/tars/pkg/llm"
)

// A new episode written with nothing to say what it is for is written from the
// agent's guess, and the guess is what the writer then corrects by hand. The
// work already has three places that say what an episode is for: its plan
// (nodes.plan, written with linetta_write_summary as_plan=true), the writer's
// intent for the scene, and a beat of the forecast branch the writer chose.
// So the first prose written into an empty scene waits until one of them says
// something. Nothing about the plan's form is checked: a new writing contract
// is not what this is for, and one line is a plan.
//
// Like the brief gate, the refusal is an ordinary tool error the model reads
// and acts on: one call to write the plan, and the write goes through in the
// same turn. A scene that already has prose is never held back here — the
// brief gate already guards a revision. And it is the built-in agent's rule
// only: an external MCP client's write gets a warning in its result instead
// (mcphost writeSceneOutput.Warnings).

const planTool = "linetta_write_summary"

// PlanLookup answers, for one scene, the two questions the gate asks. An
// interface so this package reads no store: engineapp answers it from the
// node, the scene intent and the selected forecast branch.
type PlanLookup interface {
	// PlanState reports whether nodeID is a scene with no prose yet, and
	// whether its plan, its scene intent or a beat of the chosen branch says
	// what it is for. A node that is not a scene is not empty.
	PlanState(ctx context.Context, nodeID string) (empty, planned bool, err error)
}

func planFirstRefusal(nodeID string) string {
	return "Not written. This episode has no plan yet: call " + planTool + " with node_id " + nodeID +
		" and as_plan=true, a few lines on what this episode has to do, then write it. " +
		"The plan stays with the scene after the prose is written."
}

// planGate holds the first prose of an empty scene back until something says
// what the scene is for. It keeps no state of its own: every check asks the
// lookup again, so a plan written earlier in the same turn opens it.
type planGate struct {
	lookup PlanLookup
}

// newPlanGate arms the gate only when the turn can satisfy it: without the
// summary tool the agent has no way to write the plan the refusal asks for.
func newPlanGate(lookup PlanLookup, schemas []llm.ToolSchema) *planGate {
	if lookup == nil || !offers(schemas, planTool) {
		return nil
	}
	return &planGate{lookup: lookup}
}

// refusal is the error to return instead of running name with args, and the
// scene it is about, or "" to run it.
func (g *planGate) refusal(ctx context.Context, name, args string) (string, string) {
	if g == nil || !proseWriteTools[name] {
		return "", ""
	}
	for _, id := range proseTargets(name, args) {
		empty, planned, err := g.lookup.PlanState(ctx, id)
		if err != nil {
			// A lookup that failed is not a reason to stop the writer's
			// turn; the write itself will say if the scene is gone.
			logf("plan gate: %s: %v", id, err)
			continue
		}
		if empty && !planned {
			return planFirstRefusal(id), id
		}
	}
	return "", ""
}

// proseTargets names the scenes a prose write would put text into. A revise
// with no node_ids sweeps the work for existing text, which no empty scene
// has, so it names none.
func proseTargets(name, args string) []string {
	var in struct {
		NodeID  string   `json:"node_id"`
		NodeIDs []string `json:"node_ids"`
	}
	if json.Unmarshal([]byte(args), &in) != nil {
		return nil
	}
	var out []string
	if name == "linetta_write_scene" {
		if id := strings.TrimSpace(in.NodeID); id != "" {
			out = append(out, id)
		}
		return out
	}
	for _, id := range in.NodeIDs {
		if id = strings.TrimSpace(id); id != "" {
			out = append(out, id)
		}
	}
	return out
}
