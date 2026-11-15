//go:build !mobile

package semantic

import (
	"context"
	"os"
	"testing"
)

// TestRealModelFindsParaphrase runs the real model when LINETTA_SEMANTIC_MODEL
// names its folder (model_quantized.onnx, tokenizer.json, config.json); CI
// has none and skips it.
func TestRealModelFindsParaphrase(t *testing.T) {
	dir := os.Getenv("LINETTA_SEMANTIC_MODEL")
	if dir == "" {
		t.Skip("LINETTA_SEMANTIC_MODEL not set")
	}
	ctx := context.Background()
	e, err := OpenEmbedder(ctx, dir, Model.ONNXFile)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	paras := []string{
		"유나의 칼끝이 하린의 목에서 한 치 앞에 멈췄다. \"넌 이제 내 적이야.\"",
		"도현은 처음 받은 품삯으로 어머니께 비녀를 사 드렸다.",
		"밤새 내린 눈이 발자국을 모두 지웠다. 추격대는 그들을 찾지 못했다.",
	}
	texts := make([]string, len(paras))
	for i, p := range paras {
		texts[i] = passage(p)
	}
	pv, err := e.Embed(ctx, texts)
	if err != nil {
		t.Fatal(err)
	}
	// The model places the right paragraph first about two times in three
	// on the spike's set: this checks it runs and means something, not that
	// it is never wrong.
	right := 0
	for want, q := range []string{"동료가 적으로 돌아서는 배신", "첫 월급으로 부모님께 선물한 일", "흔적이 사라져 쫓는 사람들을 따돌림"} {
		qv, err := e.Embed(ctx, []string{query(q)})
		if err != nil {
			t.Fatal(err)
		}
		best, bestScore := -1, float32(-2)
		for i, v := range pv {
			if s := dot(qv[0], quantize(v)); s > bestScore {
				best, bestScore = i, s
			}
		}
		if best == want {
			right++
		} else {
			t.Logf("%q found paragraph %d, want %d", q, best, want)
		}
	}
	if right < 2 {
		t.Fatalf("%d of 3 paraphrases found their paragraph", right)
	}
}
