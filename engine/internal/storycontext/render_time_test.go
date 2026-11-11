package storycontext

import (
	"strings"
	"testing"
)

func TestLedgerStoryTimeFollowsTheCurrentStateInEveryLanguage(t *testing.T) {
	l := LedgerBrief{
		State:   []string{"서진: 위치=객잔"},
		Time:    []string{"등대지기 실종(1화): 3화에서 석 달(약 90일)"},
		Secrets: []string{"하람은 누구인가 — 폐위된 왕자다 (작가만 앎)"},
	}
	for _, c := range []struct{ lang, state, time, rule, secrets string }{
		{"ko", "## 현재 상태", "## 작중 시간", "같은 사건으로부터의 경과는 줄지 않고", "## 비밀과 앎"},
		{"en", "## Current State", "## Story Time", "Time since an event never shrinks", "## Secrets and Who Knows"},
		{"ja", "## 現在の状態", "## 作中の時間", "同じ出来事からの経過は減らず", "## 秘密と知っていること"},
	} {
		out := renderLedger(l, c.lang)
		state, tm, secrets := strings.Index(out, c.state), strings.Index(out, c.time), strings.Index(out, c.secrets)
		if state < 0 || tm < state || secrets < tm {
			t.Fatalf("%s: sections out of place:\n%s", c.lang, out)
		}
		if !strings.Contains(out, c.rule) || !strings.Contains(out, "- 등대지기 실종(1화): 3화에서 석 달(약 90일)") {
			t.Fatalf("%s:\n%s", c.lang, out)
		}
	}
	if out := renderLedger(LedgerBrief{State: []string{"서진: 위치=객잔"}}, "ko"); strings.Contains(out, "## 작중 시간") {
		t.Fatalf("no story time, no section:\n%s", out)
	}
	if (LedgerBrief{Time: []string{"x"}}).Empty() {
		t.Fatal("a brief with only story time is not empty")
	}
}
