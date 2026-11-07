package voice

import "testing"

func TestSegmentSpansMatchSegments(t *testing.T) {
	for _, in := range []string{"  안녕하세요. 😀 반가워요!\n세 번째 줄이다", "hello. 좋다…  끝", "abc"} {
		segs, spans := Segments(in), SegmentSpans(in)
		if len(segs) != len(spans) {
			t.Fatalf("%q: %d segments, %d spans", in, len(segs), len(spans))
		}
		r := []rune(in)
		for i, sp := range spans {
			if string(r[sp[0]:sp[1]]) != segs[i] {
				t.Errorf("%q: span %d = %q, want %q", in, i, string(r[sp[0]:sp[1]]), segs[i])
			}
		}
	}
}

func TestRegister(t *testing.T) {
	for seg, want := range map[string]int{
		"써 보니 괜찮았다.":       -1,
		"다음엔 해 볼게요.":       1,
		"그런데 별로임":          -1,
		"새로 생긴 것들":         0,
		"- 생일을 넣으면 보는 페이지": 0,
		"1. 설치했다":          0,
		"ok":               0,
	} {
		if got := Register(seg); got != want {
			t.Errorf("Register(%q) = %d, want %d", seg, got, want)
		}
	}
}
