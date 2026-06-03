package companion

import "testing"

func choiceBlock(body string) string {
	return "맥락 설명\n```linetta-choices\n" + body + "\n```\n마무리"
}

func TestParseChoices_NoBlock(t *testing.T) {
	_, present, err := ParseChoices("그냥 대화입니다. 블록 없음.")
	if present || err != nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
}

func TestParseChoices_Valid(t *testing.T) {
	body := `{"prompt":"새 제목?","options":["「부엌」","「온기」","「밥」"],"allow_custom":true}`
	c, present, err := ParseChoices(choiceBlock(body))
	if !present || err != nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
	if c.Prompt != "새 제목?" || len(c.Options) != 3 || !c.AllowCustom {
		t.Fatalf("c=%+v", c)
	}
}

func TestParseChoices_TooFewOptions(t *testing.T) {
	_, present, err := ParseChoices(choiceBlock(`{"options":["하나만"]}`))
	if !present || err == nil {
		t.Fatalf("expected too-few-options error, present=%v err=%v", present, err)
	}
}

func TestParseChoices_BlankOptionsDropped(t *testing.T) {
	// Whitespace-only options don't count toward the 2-option minimum.
	_, present, err := ParseChoices(choiceBlock(`{"options":["진짜","  ",""]}`))
	if !present || err == nil {
		t.Fatalf("expected too-few-options after dropping blanks, present=%v err=%v", present, err)
	}
}

func TestParseChoices_Malformed(t *testing.T) {
	_, present, err := ParseChoices(choiceBlock(`{"options": not json}`))
	if !present || err == nil {
		t.Fatalf("expected JSON error, present=%v err=%v", present, err)
	}
}

func TestParseChoices_MultipleBlocks(t *testing.T) {
	two := choiceBlock(`{"options":["a","b"]}`) + "\n" + choiceBlock(`{"options":["c","d"]}`)
	c, present, err := ParseChoices(two)
	if !present || err == nil {
		t.Fatalf("expected multiple-blocks error, present=%v err=%v", present, err)
	}
	// best-effort returns the first block's decoded content
	if len(c.Options) != 2 || c.Options[0] != "a" {
		t.Fatalf("expected first block, got %+v", c)
	}
}
