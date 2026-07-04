package companion

import "testing"

func TestFriendlyToolLabel(t *testing.T) {
	cases := map[string]string{
		"web_search":        "웹 검색 중…",
		"web_fetch":         "웹 페이지 읽는 중…",
		"linetta_apply_ops": "작품 설정 반영 중…",
		"something_else":    "도구 실행 중: something_else",
	}
	for name, want := range cases {
		if got := friendlyToolLabel(name, ""); got != want {
			t.Errorf("friendlyToolLabel(%q) = %q, want %q", name, got, want)
		}
	}
}
