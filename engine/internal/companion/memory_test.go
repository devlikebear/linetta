package companion

import "testing"

func TestRememberAndRecall(t *testing.T) {
	svc := &Service{memBase: t.TempDir()}
	if err := svc.Remember("p1", "작가는 단문을 선호한다", "preference"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Remember("p1", "세계관: 마법은 금지되어 있다", "lore"); err != nil {
		t.Fatal(err)
	}
	hits := svc.Recall("p1", "단문", recallLimit)
	if len(hits) != 1 || hits[0] != "작가는 단문을 선호한다" {
		t.Fatalf("recall(단문) = %+v", hits)
	}
	if got := svc.Recall("p1", "존재하지않는키워드", recallLimit); len(got) != 0 {
		t.Fatalf("expected no hits, got %+v", got)
	}
	if got := svc.Recall("p2", "단문", recallLimit); len(got) != 0 {
		t.Fatalf("p2 should have no memory, got %+v", got)
	}
}

func TestRememberRequiresText(t *testing.T) {
	svc := &Service{memBase: t.TempDir()}
	if err := svc.Remember("p1", "", "x"); err == nil {
		t.Fatal("expected error for empty text")
	}
}
