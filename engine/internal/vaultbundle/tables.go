package vaultbundle

import (
	"fmt"
	"sort"
)

// Table says how one table travels with a work. Scope selects the work's rows
// with one bound parameter, the project id, and is the same clause for reading
// them out and clearing them before a pull.
type Table struct {
	Name  string
	Scope string
}

const (
	byProject  = "project_id = ?"
	byNode     = "node_id IN (SELECT id FROM nodes WHERE project_id = ?)"
	byEntity   = "entity_id IN (SELECT id FROM entities WHERE project_id = ?)"
	byThread   = "thread_id IN (SELECT id FROM threads WHERE project_id = ?)"
	byFactCard = "card_id IN (SELECT id FROM fact_cards WHERE project_id = ?)"
)

// Parents are written in place, never deleted and re-inserted: tables that stay
// on this device point at them, and a delete would cascade into publish_works
// and publish_entities or null the node_id of ai_runs and companion_messages.
var parents = []Table{
	{"projects", "id = ?"},
	{"entities", byProject},
	{"nodes", byProject},
}

// Dependents are replaced wholesale, in this order so that every foreign key
// target is already there. A pull clears them in the reverse order.
var dependents = []Table{
	{"entity_visuals", byEntity},
	{"entity_reference_images", byEntity},
	{"project_art_style", byProject},
	{"project_style_rules", byProject},
	{"project_style_drafts", byProject},
	{"project_focus", byProject},
	{"illustrations", byProject},
	{"node_representative_illustrations", byProject},
	{"illustration_requests", byProject},
	{"mentions", byNode},
	{"relationships", byProject},
	{"threads", byProject},
	{"beats", byThread},
	{"notes", byNode},
	{"scene_intents", byProject},
	{"node_snapshots", byNode},
	{"fact_cards", byProject},
	{"fact_sources", byFactCard},
	{"writing_stats", byProject},
	{"agent_memory", "scope = 'work_notes' AND project_id = ?"},
	{"continuity_checks", byProject},
	{"continuity_observations", byProject},
	{"forecasts", byProject},
	{"story_proposals", byProject},
	{"story_proposal_items", byProject},
}

// excluded names every table that stays behind, and why. A table in neither
// list fails TestEveryTableIsClassified, so a migration that adds a table to a
// work cannot silently leave it out of the vault.
var excluded = map[string]string{
	"ai_runs":                   "device history of model calls",
	"companion_messages":        "device conversation history",
	"companion_references":      "device conversation history",
	"companion_history_imports": "device conversation history",
	"mcp_activity":              "device history of tool calls",
	"skill_snapshots":           "device history of skill edits",
	"vault_items":               "this device's bookmark into the vault itself",
	"publish_works":             "reconciled with the site after a pull",
	"publish_episodes":          "reconciled with the site after a pull",
	"publish_entities":          "reconciled with the site after a pull",
	"publish_covers":            "reconciled with the site after a pull",
	"ops_status":                "library-wide background job state",
	"schema_migrations":         "library-wide schema state",
	"manuscript_fts":            "search index, rebuilt from nodes",
	"manuscript_fts_config":     "search index, rebuilt from nodes",
	"manuscript_fts_content":    "search index, rebuilt from nodes",
	"manuscript_fts_data":       "search index, rebuilt from nodes",
	"manuscript_fts_docsize":    "search index, rebuilt from nodes",
	"manuscript_fts_idx":        "search index, rebuilt from nodes",
}

// imageColumns are the BLOB columns. Each value leaves the row as a part of its
// own, so re-uploading a work does not re-upload its pictures.
var imageColumns = map[string]string{
	"illustrations":           "data",
	"entity_reference_images": "data",
}

// deviceColumns stay out of the rows, so merely opening a work does not read as
// a change: node.SetLastOpened writes both of these. updated_at still travels,
// in the manifest, outside what the content hash covers.
var deviceColumns = map[string]map[string]bool{
	"projects": {"last_opened_node_id": true, "updated_at": true},
}

func allTables() []Table {
	return append(append([]Table{}, parents...), dependents...)
}

// checkClassified reports tables the bundle has no decision about, and
// decisions about tables that no longer exist.
func checkClassified(schemaTables []string) error {
	known := map[string]bool{}
	for _, table := range allTables() {
		known[table.Name] = true
	}
	for name := range excluded {
		known[name] = true
	}
	var unclassified []string
	present := map[string]bool{}
	for _, name := range schemaTables {
		present[name] = true
		if !known[name] {
			unclassified = append(unclassified, name)
		}
	}
	var stale []string
	for name := range known {
		if !present[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(unclassified)
	sort.Strings(stale)
	if len(unclassified) > 0 || len(stale) > 0 {
		return fmt.Errorf("vaultbundle: unclassified tables %v, classified but missing %v", unclassified, stale)
	}
	return nil
}
