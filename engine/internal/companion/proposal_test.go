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
	_, present, err := ParseProposal(block(`{"ops":[{"op":"add_beat","thread_ref":"nope","node_id":"n","label":"l"}]}`))
	if !present {
		t.Fatal("block should be present")
	}
	if err == nil {
		t.Fatal("expected dangling thread_ref error")
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
