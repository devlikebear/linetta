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
