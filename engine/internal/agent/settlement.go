//go:build !mobile

package agent

import (
	"fmt"
	"strings"
)

// A settlement may read each scene and the ledger, and write what changed. Each
// fact is one call, so it gets a larger budget than the review.
const settlementCallsPerScene = 14

// settlementTools can record in the ledger and report what does not fit. It has
// no way to touch the prose or the story world: what a scene established is the
// only thing it is here to write down.
var settlementTools = map[string]bool{
	"linetta_read_scene":        true,
	"linetta_list_ledger":       true,
	"linetta_edit_ledger":       true,
	"linetta_report_continuity": true,
}

// settlementSystemPrompt asks for changes only. A fact that the story leans on
// later is worth a row; the prose is the record of everything else, and a ledger
// that copies it grows with the book and stops being readable.
const settlementSystemPrompt = `You are settling a finished chapter of fiction into the work's ledger, for its author. You did not write it. Your reply goes nowhere: what you find reaches the author only through the ledger and linetta_report_continuity.

For each scene you are given:
1. Read it with linetta_read_scene numbered=true.
2. Look up what the ledger already holds with linetta_list_ledger, using query for the subject.
3. For each thing this scene changed that later scenes must honour, call linetta_edit_ledger with kind (number, timeline, knowledge or decision), subject, attribute, body, planted_node_id and planted_quote copied exactly from the scene. Use the attributes 위치, 소지금, 경지, 부상, 소속, 호칭, 작중 날짜, 앎. Set value_num when the value is a number, and strength soft when it comes from a character's mouth or a rumour rather than the narrator.
4. When a value replaces one already recorded, pass its id as supersedes.
5. When the result lists conflicts and the scene is not plainly changing the value, the prose and the record disagree: call linetta_report_continuity with the quote, and leave the fact as it is.

Record only what changed and will matter. Do not retell the plot, and do not record what is already there. Nothing to record is a good result.`

func settlementRequest(projectID string, scenes []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Work: %s\nSettle these scenes, in order:\n", projectID)
	for _, id := range scenes {
		b.WriteString("- " + id + "\n")
	}
	return b.String()
}
