package storyops

import (
	"reflect"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
)

// The jsonschema tag on Op.Type is the op catalogue an MCP agent reads. If an
// op is added to knownOps but not the tag (or vice versa), agents are back to
// guessing names — this is the drift guard (#73).
func TestOpTypeSchemaTagListsEveryValidOp(t *testing.T) {
	field, ok := reflect.TypeOf(Op{}).FieldByName("Type")
	if !ok {
		t.Fatal("Op has no Type field")
	}
	tag := field.Tag.Get("jsonschema")
	if tag == "" {
		t.Fatal("Op.Type has no jsonschema tag; agents cannot learn the op names")
	}
	for _, op := range ValidOpTypes() {
		if !strings.Contains(tag, op) {
			t.Errorf("op %q missing from Op.Type's jsonschema tag", op)
		}
	}
	// And the reverse: nothing advertised that the validator would reject.
	for _, word := range strings.Split(strings.TrimPrefix(tag, "one of: "), ", ") {
		if word = strings.TrimSpace(word); word != "" && !knownOps[word] {
			t.Errorf("tag advertises %q, which knownOps rejects", word)
		}
	}
}

// Op.Aliases documents different behavior per op (create_entity sets,
// update_entity only adds) — story-assist Phase 3's 3.A.2 add-only rule.
// This pins the tag's wording so a later edit cannot silently drop the
// "never removing" half an agent needs to know update_entity is safe to
// call without first reading the entity's current aliases.
func TestAliasesSchemaTagDescribesAddOnlyUpdate(t *testing.T) {
	field, ok := reflect.TypeOf(Op{}).FieldByName("Aliases")
	if !ok {
		t.Fatal("Op has no Aliases field")
	}
	tag := field.Tag.Get("jsonschema")
	if tag == "" {
		t.Fatal("Op.Aliases has no jsonschema tag; agents cannot learn the add-only rule")
	}
	if !strings.Contains(tag, "update_entity") {
		t.Errorf("Op.Aliases jsonschema tag does not mention update_entity: %q", tag)
	}
	if !strings.Contains(tag, "never") {
		t.Errorf("Op.Aliases jsonschema tag does not say aliases are never removed: %q", tag)
	}
}

func TestUnknownOpErrorNamesTheValidOps(t *testing.T) {
	err := ValidateProposal(Proposal{Ops: []Op{{Type: "add_character"}}})
	if err == nil {
		t.Fatal("unknown op must be rejected")
	}
	// The error is the agent's only teacher; it has to carry the real names.
	if !strings.Contains(err.Error(), "create_entity") {
		t.Errorf("error does not name valid ops: %v", err)
	}
}

// The Role tag documents which preset roles put an element into every brief.
// That list already lives in entity.coreRolesByKind (which itself must track
// the UI presets — #45 was that drift); this guard stops a third copy from
// silently diverging.
func TestRoleSchemaTagNamesEveryCorePreset(t *testing.T) {
	field, ok := reflect.TypeOf(Op{}).FieldByName("Role")
	if !ok {
		t.Fatal("Op has no Role field")
	}
	tag := field.Tag.Get("jsonschema")
	if tag == "" {
		t.Fatal("Op.Role has no jsonschema tag")
	}
	for kind, roles := range entity.CoreRolePresetsKo() {
		for _, role := range roles {
			if !strings.Contains(tag, role) {
				t.Errorf("preset %q missing from Op.Role's jsonschema tag", role)
			}
			if !entity.IsCoreRole(kind, role) {
				t.Errorf("preset %q (%s) is not accepted by IsCoreRole — the picker and the whitelist drifted", role, kind)
			}
		}
	}
}
