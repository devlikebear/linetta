package companion

import "testing"

func block(body string) string {
	return "어쩌고\n```linetta-proposal\n" + body + "\n```\n저쩌고"
}

func TestParseProposal_NoBlock(t *testing.T) {
	_, present, err := ParseProposal("그냥 대화입니다. 블록 없음.")
	if present || err != nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
}

func TestParseProposal_Valid(t *testing.T) {
	body := `{"summary":"복수극 추가","ops":[
	  {"op":"create_thread","ref":"t1","name":"복수극"},
	  {"op":"add_beat","thread_ref":"t1","node_id":"n1","label":"결심"},
	  {"op":"set_outline","outline":"전체 개요"}
	]}`
	p, present, err := ParseProposal(block(body))
	if !present || err != nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
	if p.Summary != "복수극 추가" || len(p.Ops) != 3 {
		t.Fatalf("p=%+v", p)
	}
}

func TestParseProposal_UnknownOp(t *testing.T) {
	_, present, err := ParseProposal(block(`{"ops":[{"op":"frobnicate"}]}`))
	if !present || err == nil {
		t.Fatalf("expected invalid-op error, present=%v err=%v", present, err)
	}
}

func TestParseProposal_AddBeatXorThread(t *testing.T) {
	_, present, err := ParseProposal(block(`{"ops":[{"op":"add_beat","thread_id":"x","thread_ref":"y","node_id":"n","label":"l"}]}`))
	if !present {
		t.Fatal("block should be present")
	}
	if err == nil {
		t.Fatal("expected XOR error (both)")
	}
	_, present, err = ParseProposal(block(`{"ops":[{"op":"add_beat","node_id":"n","label":"l"}]}`))
	if !present {
		t.Fatal("block should be present")
	}
	if err == nil {
		t.Fatal("expected XOR error (neither)")
	}
}

func TestParseProposal_DanglingThreadRef(t *testing.T) {
	// An undeclared thread_ref now parses (it is often a real thread id);
	// resolution and any error happen at apply time.
	_, present, err := ParseProposal(block(`{"ops":[{"op":"add_beat","thread_ref":"nope","node_id":"n","label":"l"}]}`))
	if !present {
		t.Fatal("block should be present")
	}
	if err != nil {
		t.Fatalf("dangling thread_ref should parse (resolved at apply), got %v", err)
	}
}

func TestParseProposal_BadJSON(t *testing.T) {
	_, present, err := ParseProposal(block(`{not json`))
	if !present || err == nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
}

func TestParseProposal_MultipleBlocks(t *testing.T) {
	two := block(`{"ops":[{"op":"set_outline","outline":"a"}]}`) + "\n" + block(`{"ops":[{"op":"set_outline","outline":"b"}]}`)
	_, present, err := ParseProposal(two)
	if !present || err == nil {
		t.Fatalf("expected multi-block error, present=%v err=%v", present, err)
	}
}

func TestParseProposal_CRLF(t *testing.T) {
	body := "{\"summary\":\"s\",\"ops\":[{\"op\":\"set_outline\",\"outline\":\"o\"}]}"
	full := "intro\r\n```linetta-proposal\r\n" + body + "\r\n```\r\nafter"
	p, present, err := ParseProposal(full)
	if !present || err != nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
	if len(p.Ops) != 1 || p.Ops[0].Outline != "o" {
		t.Fatalf("p=%+v", p)
	}
}

func TestParseProposal_Remember(t *testing.T) {
	p, present, err := ParseProposal(block(`{"ops":[{"op":"remember","text":"작가는 단문을 선호","category":"preference"}]}`))
	if !present || err != nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
	if len(p.Ops) != 1 || p.Ops[0].Text != "작가는 단문을 선호" {
		t.Fatalf("p=%+v", p)
	}
}

func TestParseProposal_RememberRequiresText(t *testing.T) {
	_, present, err := ParseProposal(block(`{"ops":[{"op":"remember","category":"x"}]}`))
	if !present || err == nil {
		t.Fatalf("expected text-required error, present=%v err=%v", present, err)
	}
}

func TestParseProposal_AddBeatNodeIDOptional(t *testing.T) {
	_, present, err := ParseProposal(block(`{"ops":[{"op":"create_thread","ref":"t1","name":"X"},{"op":"add_beat","thread_ref":"t1","label":"비트"}]}`))
	if !present || err != nil {
		t.Fatalf("add_beat without node_id should be valid now: present=%v err=%v", present, err)
	}
}

func TestParseProposal_EntityAndRelationship(t *testing.T) {
	body := `{"ops":[
	  {"op":"create_entity","ref":"e1","kind":"character","name":"하나","role":"주인공"},
	  {"op":"create_entity","ref":"e2","kind":"character","name":"도윤"},
	  {"op":"create_relationship","from_ref":"e1","to_ref":"e2","label":"라이벌"}
	]}`
	p, present, err := ParseProposal(block(body))
	if !present || err != nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
	if len(p.Ops) != 3 || p.Ops[0].Name != "하나" || p.Ops[2].Label != "라이벌" {
		t.Fatalf("p=%+v", p)
	}
}

func TestParseProposal_CreateEntityKindNormalization(t *testing.T) {
	// Missing kind defaults to "character" (the dominant entity type) so the
	// common "캐릭터 만들어줘" flow applies instead of failing.
	p, _, err := ParseProposal(block(`{"ops":[{"op":"create_entity","name":"X"}]}`))
	if err != nil {
		t.Fatalf("missing kind should default, got err=%v", err)
	}
	if p.Ops[0].Kind != "character" {
		t.Fatalf("missing kind: want character, got %q", p.Ops[0].Kind)
	}

	// Synonyms (Korean / mixed case) normalize to the canonical value.
	cases := map[string]string{
		"캐릭터":       "character",
		"인물":        "character",
		"Character": "character",
		"장소":        "place",
		"LOCATION":  "place",
		"아이템":       "item",
		"개념":        "concept",
	}
	for raw, want := range cases {
		p, _, err := ParseProposal(block(`{"ops":[{"op":"create_entity","name":"X","kind":"` + raw + `"}]}`))
		if err != nil {
			t.Fatalf("kind %q: unexpected err %v", raw, err)
		}
		if p.Ops[0].Kind != want {
			t.Fatalf("kind %q: want %q, got %q", raw, want, p.Ops[0].Kind)
		}
	}

	// A truly unknown kind is still rejected.
	if _, _, err := ParseProposal(block(`{"ops":[{"op":"create_entity","name":"X","kind":"bogus"}]}`)); err == nil {
		t.Fatal("expected invalid-kind error")
	}
}

func TestParseProposal_RelationshipXorAndDanglingRef(t *testing.T) {
	if _, _, err := ParseProposal(block(`{"ops":[{"op":"create_relationship","from":"a","from_ref":"x","to":"b","label":"L"}]}`)); err == nil {
		t.Fatal("expected from XOR error")
	}
	// An undeclared to_ref now parses (it is often a real entity id/name);
	// resolution and any error happen at apply time.
	if _, _, err := ParseProposal(block(`{"ops":[{"op":"create_relationship","from":"a","to_ref":"nope","label":"L"}]}`)); err != nil {
		t.Fatalf("dangling to_ref should parse (resolved at apply), got %v", err)
	}
}

func TestParseProposal_UpdateEntityRequiresID(t *testing.T) {
	if _, _, err := ParseProposal(block(`{"ops":[{"op":"update_entity","name":"X"}]}`)); err == nil {
		t.Fatal("expected entity_id error")
	}
}

func TestParseProposal_CreateSceneAndNodeRef(t *testing.T) {
	body := `{"ops":[
	  {"op":"create_thread","ref":"t1","name":"메인"},
	  {"op":"create_scene","ref":"s1","label":"재회"},
	  {"op":"add_beat","thread_ref":"t1","node_ref":"s1","label":"첫 만남"}
	]}`
	p, present, err := ParseProposal(block(body))
	if !present || err != nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
	if len(p.Ops) != 3 || p.Ops[1].Label != "재회" || p.Ops[2].NodeRef != "s1" {
		t.Fatalf("p=%+v", p)
	}
}

func TestParseProposal_CreateOutlineNode(t *testing.T) {
	body := `{"ops":[
	  {"op":"create_outline_node","ref":"p1","kind":"container","label":"1부"},
	  {"op":"create_outline_node","ref":"c1","kind":"container","parent_node_ref":"p1","label":"1장"},
	  {"op":"create_outline_node","ref":"s1","kind":"leaf","parent_node_ref":"c1","label":"씬 1"}
	]}`
	p, present, err := ParseProposal(block(body))
	if !present || err != nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
	if len(p.Ops) != 3 || p.Ops[1].ParentNodeRef != "p1" || p.Ops[2].Kind != "leaf" {
		t.Fatalf("p=%+v", p)
	}
}

func TestParseProposal_CreateOutlineNodeValidation(t *testing.T) {
	if _, _, err := ParseProposal(block(`{"ops":[{"op":"create_outline_node","kind":"leaf"}]}`)); err == nil {
		t.Fatal("expected label error")
	}
	if _, _, err := ParseProposal(block(`{"ops":[{"op":"create_outline_node","kind":"folder","label":"x"}]}`)); err == nil {
		t.Fatal("expected kind error")
	}
	if _, _, err := ParseProposal(block(`{"ops":[{"op":"create_outline_node","kind":"leaf","label":"x","parent_node_id":"p","parent_node_ref":"pr"}]}`)); err == nil {
		t.Fatal("expected parent id/ref mutual-exclusion error")
	}
	if _, _, err := ParseProposal(block(`{"ops":[{"op":"create_outline_node","kind":"leaf","label":"x","after_node_id":"a","after_node_ref":"ar"}]}`)); err == nil {
		t.Fatal("expected after id/ref mutual-exclusion error")
	}
}

func TestParseProposal_OutlineMaintenanceOps(t *testing.T) {
	body := `{"ops":[
	  {"op":"rename_outline_node","node_id":"n1","label":"1부","title":"새 제목"},
	  {"op":"move_outline_node","node_id":"n2","direction":"up"},
	  {"op":"delete_outline_node","node_ref":"old"}
	]}`
	p, present, err := ParseProposal(block(body))
	if !present || err != nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
	if p.Ops[0].Type != "rename_outline_node" || p.Ops[1].Direction != "up" || p.Ops[2].NodeRef != "old" {
		t.Fatalf("p=%+v", p)
	}
}

func TestParseProposal_OutlineMaintenanceValidation(t *testing.T) {
	if _, _, err := ParseProposal(block(`{"ops":[{"op":"rename_outline_node","label":"1부"}]}`)); err == nil {
		t.Fatal("expected rename target error")
	}
	if _, _, err := ParseProposal(block(`{"ops":[{"op":"move_outline_node","node_id":"n1","direction":"sideways"}]}`)); err == nil {
		t.Fatal("expected direction error")
	}
	if _, _, err := ParseProposal(block(`{"ops":[{"op":"delete_outline_node","node_id":"n1","node_ref":"r1"}]}`)); err == nil {
		t.Fatal("expected node id/ref mutual-exclusion error")
	}
}

func TestParseProposal_CreateSceneRequiresLabel(t *testing.T) {
	if _, _, err := ParseProposal(block(`{"ops":[{"op":"create_scene"}]}`)); err == nil {
		t.Fatal("expected label error")
	}
}

func TestParseProposal_AddBeatNodeIDXorNodeRef(t *testing.T) {
	if _, _, err := ParseProposal(block(`{"ops":[{"op":"add_beat","thread_id":"x","node_id":"n","node_ref":"s","label":"L"}]}`)); err == nil {
		t.Fatal("expected node_id/node_ref mutual-exclusion error")
	}
	// A node_ref that is not declared by a create_scene in the same proposal is
	// NO LONGER a parse error: the model often places a real node id there, so
	// resolution (and any clear error) is deferred to apply time.
	if _, _, err := ParseProposal(block(`{"ops":[{"op":"add_beat","thread_id":"x","node_ref":"nope","label":"L"}]}`)); err != nil {
		t.Fatalf("dangling node_ref should parse (resolved at apply), got %v", err)
	}
}

func TestParseProposal_RelationshipInverseLabel(t *testing.T) {
	_, present, err := ParseProposal(block(`{"ops":[{"op":"create_relationship","from":"a","to":"b","label":"라이벌","inverse_label":"라이벌"}]}`))
	if !present || err != nil {
		t.Fatalf("inverse_label should be valid: present=%v err=%v", present, err)
	}
}

func TestParseProposal_CreateFactCard(t *testing.T) {
	body := `{"ops":[{"op":"create_fact_card","claim":"런던 일반 경찰은 항상 총기를 휴대한다","result":"일반 경찰은 통상 비무장이다.","status":"verified","category":"police","sources":[{"url":"https://www.met.police.uk/","title":"Met Police","snippet":"official reference","accessed_at":100}]}]}`
	p, present, err := ParseProposal(block(body))
	if !present || err != nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
	if len(p.Ops) != 1 || p.Ops[0].Claim == "" || len(p.Ops[0].Sources) != 1 {
		t.Fatalf("p=%+v", p)
	}
}

func TestParseProposal_CreateFactCardAllowsStringAccessedAt(t *testing.T) {
	body := `{"ops":[{"op":"create_fact_card","claim":"비 온 뒤 흙냄새","result":"페트리코어와 지오스민이 관련된다.","status":"verified","sources":[{"url":"https://example.com/petrichor","title":"Example","accessed_at":""},{"url":"https://example.com/geosmin","accessed_at":"123"}]}]}`
	p, present, err := ParseProposal(block(body))
	if !present || err != nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
	if got := p.Ops[0].Sources[0].AccessedAt; got != 0 {
		t.Fatalf("empty accessed_at string should be ignored, got %d", got)
	}
	if got := p.Ops[0].Sources[1].AccessedAt; got != 123 {
		t.Fatalf("numeric accessed_at string should be parsed, got %d", got)
	}
}

func TestParseProposal_CreateFactCardRequiresSource(t *testing.T) {
	_, present, err := ParseProposal(block(`{"ops":[{"op":"create_fact_card","claim":"X","result":"Y","status":"verified"}]}`))
	if !present || err == nil {
		t.Fatalf("expected source-required error, present=%v err=%v", present, err)
	}
}
