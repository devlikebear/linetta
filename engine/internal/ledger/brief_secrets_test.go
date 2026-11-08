package ledger

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestBriefKeepsTheSecretUntilTheStoryTellsIt(t *testing.T) {
	f := setup(t, 4)
	yes, no := KnowsYes, KnowsNo
	q, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindQuestion, Body: "하람은 누구인가", Answer: "폐위된 왕자다", PlantedNodeID: f.scenes[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []struct {
		subject string
		scene   int
		v       *float64
	}{{"서진", 0, &no}, {"도윤", 0, &yes}, {"무명", 0, &yes}} {
		if _, err := f.repo.Add(f.ctx, 2, Entry{ProjectID: f.pid, Kind: KindKnowledge, Subject: k.subject, Attribute: AttributeKnows,
			Body: "하람의 정체", About: q.ID, ValueNum: k.v, PlantedNodeID: f.scenes[k.scene].ID}); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.nodes.SetPlan(f.ctx, f.scenes[1].ID, "서진과 도윤이 객잔에서 하람을 기다린다"); err != nil {
		t.Fatal(err)
	}

	b, err := f.repo.ContextLedger(f.ctx, f.pid, f.scenes[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Secrets) != 1 || !strings.Contains(b.Secrets[0], "폐위된 왕자다") || !strings.Contains(b.Secrets[0], "(작가만 앎)") {
		t.Fatalf("secrets = %q", b.Secrets)
	}
	knows := strings.Join(b.Knows, "\n")
	if !strings.Contains(knows, "서진: 모름=") || !strings.Contains(knows, "도윤: 앎=") {
		t.Fatalf("knows = %q", b.Knows)
	}
	if strings.Contains(knows, "무명") {
		t.Fatalf("무명 is not in the scene: %q", b.Knows)
	}
	for _, line := range append(append([]string{}, b.Open...), b.Due...) {
		if strings.Contains(line, "폐위된 왕자다") {
			t.Fatalf("the answer belongs under the secrets alone: %q", line)
		}
	}

	status, paid := StatusPaid, f.scenes[2].ID
	if _, err := f.repo.Update(f.ctx, 3, q.ID, Patch{Status: &status, PaidNodeID: &paid}); err != nil {
		t.Fatal(err)
	}
	b, err = f.repo.ContextLedger(f.ctx, f.pid, f.scenes[2].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Secrets) != 0 || len(b.Knows) != 0 {
		t.Fatalf("the scene that tells it keeps no secret: %q %q", b.Secrets, b.Knows)
	}
	// Going back to an earlier scene, it is a secret again.
	b, err = f.repo.ContextLedger(f.ctx, f.pid, f.scenes[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Secrets) != 1 {
		t.Fatalf("before the scene that tells it: %q", b.Secrets)
	}
}

// A pinned question is told in full under the work's rules and named, with its
// answer, under the secrets: the answer once, the question in full once.
func TestPinnedSecretIsToldOnce(t *testing.T) {
	f := setup(t, 2)
	body := "북쪽 성문을 연 사람은 누구이며 왜 그날 밤 아무도 모르게 성문을 열고 다시 닫았는가"
	q, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindQuestion, Body: body, Answer: "서진이 동생을 살리려고 열었다", PlantedNodeID: f.scenes[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.SetPinned(f.ctx, 2, q.ID, true); err != nil {
		t.Fatal(err)
	}
	b, err := f.repo.ContextLedger(f.ctx, f.pid, f.scenes[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Pinned) != 1 || !strings.Contains(b.Pinned[0], body) || strings.Contains(b.Pinned[0], "동생을 살리려고") {
		t.Fatalf("pinned = %q", b.Pinned)
	}
	if len(b.Secrets) != 1 || strings.Contains(b.Secrets[0], body) || !strings.Contains(b.Secrets[0], "동생을 살리려고") {
		t.Fatalf("secrets = %q", b.Secrets)
	}
}

func TestBriefSecretsStayInTheirBudget(t *testing.T) {
	f := setup(t, 2)
	for i := range 40 {
		if _, err := f.repo.Add(f.ctx, int64(i+1), Entry{ProjectID: f.pid, Kind: KindQuestion, Body: fmt.Sprintf("질문 %d: %s", i, strings.Repeat("긴 질문 ", 10)),
			Answer: strings.Repeat("작가만 아는 긴 답 ", 6), PlantedNodeID: f.scenes[0].ID}); err != nil {
			t.Fatal(err)
		}
	}
	b, err := f.repo.ContextLedger(f.ctx, f.pid, f.scenes[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, line := range append(append([]string{}, b.Secrets...), b.Knows...) {
		n += utf8.RuneCountInString(line)
	}
	if n > briefSecretsRunes || len(b.Secrets) == 0 {
		t.Fatalf("%d secrets ran to %d runes", len(b.Secrets), n)
	}
	if b.Omitted < 40-len(b.Secrets) {
		t.Fatalf("omitted = %d with %d shown of 40", b.Omitted, len(b.Secrets))
	}
}
