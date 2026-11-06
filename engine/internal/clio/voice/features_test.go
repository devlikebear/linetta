package voice

import "testing"

func TestFeaturesTable(t *testing.T) {
	tests := []struct {
		name string
		post string
		get  func(Feature) float64
		want float64
	}{
		{"emoji", "좋은 하루 😀🚀 보냄", func(f Feature) float64 { return f.Emoji }, 2},
		{"laugh", "이거 웃기다 ㅋㅋㅋ 그리고 ㅎㅎ", func(f Feature) float64 { return f.Laugh }, 2},
		{"cry", "너무 슬프다 ㅠㅠ", func(f Feature) float64 { return f.Cry }, 1},
		{"hashtag", "오늘 기록 #일상 #메모", func(f Feature) float64 { return f.Hashtag }, 2},
		{"ellipsis", "음... 글쎄… 모르겠다", func(f Feature) float64 { return f.Ellipsis }, 2},
		{"exclaim", "좋다! 진짜 좋다!!", func(f Feature) float64 { return f.Exclaim }, 3},
		{"lines", "한 줄이다\n두 줄이다\n세 줄이다", func(f Feature) float64 { return f.Lines }, 3},
		{"polite all", "저는 괜찮아요. 감사합니다", func(f Feature) float64 { return f.Polite }, 1},
		{"polite half", "저는 괜찮아요. 근데 별로임", func(f Feature) float64 { return f.Polite }, 0.5},
		{"no punctuation sentences", "마침표가 없다\n그래도 센다", func(f Feature) float64 { return f.Punctuated }, 0},
		{"punctuated", "끝났다. 시작이다", func(f Feature) float64 { return f.Punctuated }, 0.5},
		{"ai tells", "결론적으로 다양한 인사이트", func(f Feature) float64 { return f.AITells }, 3},
		{"length counts runes", "가나다", func(f Feature) float64 { return f.Length }, 3},
	}
	for _, tc := range tests {
		if got := tc.get(Features(tc.post)); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestEnding(t *testing.T) {
	if Ending("안녕하세요!") != "세요" || Ending("가") != "" || Ending("abc") != "" {
		t.Fatal("ending")
	}
	if !IsPolite("합니다") || !IsPolite("요") || IsPolite("했다") {
		t.Fatal("polite")
	}
}

func TestSegmentsNeedHangul(t *testing.T) {
	got := Segments("hello world. 안녕. ok! 반가워")
	if len(got) != 2 || got[0] != "안녕." || got[1] != "반가워" {
		t.Fatalf("got %q", got)
	}
}
