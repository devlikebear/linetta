package evidence

import (
	"errors"
	"testing"
)

func TestVerifyQuote(t *testing.T) {
	paragraphs := []string{"하린은 오른손으로 검을 뽑았다.", "두 번째 문단이다."}

	tests := []struct {
		name     string
		from, to int
		quote    string
		wantErr  bool
		wantKind ErrorKind
	}{
		{name: "valid range and quote", from: 1, to: 1, quote: "오른손으로"},
		{name: "valid range, empty quote is not an error", from: 1, to: 1, quote: ""},
		{name: "valid range, whitespace-only quote is not an error", from: 1, to: 1, quote: "   "},
		{name: "from too small", from: 0, to: 1, quote: "", wantErr: true, wantKind: FromTooSmall},
		{name: "to before from", from: 2, to: 1, quote: "", wantErr: true, wantKind: ToBeforeFrom},
		{name: "to out of range", from: 1, to: 3, quote: "", wantErr: true, wantKind: OutOfRange},
		{name: "quote not found", from: 1, to: 1, quote: "존재하지 않는 문장", wantErr: true, wantKind: QuoteNotFound},
		{name: "quote spans the range joined by newline", from: 1, to: 2, quote: "뽑았다.\n두 번째"},
		{
			name: "composed vs decomposed Hangul and extra whitespace still match",
			from: 1, to: 1, quote: "오른손으로   검을",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := VerifyQuote(paragraphs, tc.from, tc.to, tc.quote)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("VerifyQuote() = nil, want error")
				}
				var ee *Error
				if !errors.As(err, &ee) {
					t.Fatalf("VerifyQuote() error is not *evidence.Error: %v", err)
				}
				if ee.Kind != tc.wantKind {
					t.Errorf("Kind = %v, want %v", ee.Kind, tc.wantKind)
				}
				return
			}
			if err != nil {
				t.Fatalf("VerifyQuote() = %v, want nil", err)
			}
		})
	}
}

func TestContainsNormalized(t *testing.T) {
	tests := []struct {
		name            string
		haystack, quote string
		want            bool
	}{
		{name: "exact match", haystack: "하린은 검을 들었다.", quote: "검을 들었다", want: true},
		{name: "extra inner whitespace folds", haystack: "하린은  검을   들었다.", quote: "검을 들었다", want: true},
		{name: "different words do not match", haystack: "하린은 검을 들었다.", quote: "창을 들었다", want: false},
		{name: "quote longer than haystack", haystack: "짧다", quote: "이것은 훨씬 더 긴 문장이다", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ContainsNormalized(tc.haystack, tc.quote); got != tc.want {
				t.Errorf("ContainsNormalized(%q, %q) = %v, want %v", tc.haystack, tc.quote, got, tc.want)
			}
		})
	}
}

func TestError_Error(t *testing.T) {
	// Every Kind must produce a non-empty message; the exact wording is a
	// generic fallback — callers that need specific phrasing (continuity,
	// proposals) switch on Kind instead of parsing this string.
	kinds := []ErrorKind{FromTooSmall, ToBeforeFrom, OutOfRange, QuoteNotFound, ErrorKind(99)}
	for _, k := range kinds {
		e := &Error{Kind: k, From: 1, To: 2, N: 3, Quote: "x"}
		if e.Error() == "" {
			t.Errorf("Error() for kind %v is empty", k)
		}
	}
}
