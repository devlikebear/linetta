package storycontext

import (
	"strings"
	"testing"
)

func TestLedgerSecretsFollowTheCurrentStateInEveryLanguage(t *testing.T) {
	l := LedgerBrief{
		State:   []string{"서진: 위치=객잔"},
		Secrets: []string{"하람은 누구인가 — 폐위된 왕자다 (작가만 앎)"},
		Knows:   []string{"서진: 모름=하람은 누구인가"},
		Open:    []string{"[독자의 질문] 하람은 누구인가"},
	}
	for _, c := range []struct{ lang, state, secrets, rule string }{
		{"ko", "## 현재 상태", "## 비밀과 앎", "서술자는 작가만 아는 답을 밝히지 않는다"},
		{"en", "## Current State", "## Secrets and Who Knows", "The narrator does not tell"},
		{"ja", "## 現在の状態", "## 秘密と知っていること", "語り手は作家だけが知る答えを明かさない"},
	} {
		out := renderLedger(l, c.lang)
		state, secrets, owed := strings.Index(out, c.state), strings.Index(out, c.secrets), strings.Index(out, "[독자의 질문]")
		if state < 0 || secrets < state || owed < secrets {
			t.Fatalf("%s: sections out of place:\n%s", c.lang, out)
		}
		if !strings.Contains(out, c.rule) || !strings.Contains(out, "- 서진: 모름=") || !strings.Contains(out, "폐위된 왕자다") {
			t.Fatalf("%s:\n%s", c.lang, out)
		}
	}
	if out := renderLedger(LedgerBrief{State: []string{"서진: 위치=객잔"}}, "ko"); strings.Contains(out, "## 비밀과 앎") {
		t.Fatalf("no secrets, no section:\n%s", out)
	}
	if (LedgerBrief{Knows: []string{"x"}}).Empty() {
		t.Fatal("a brief with only knows lines is not empty")
	}
}
