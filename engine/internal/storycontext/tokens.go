package storycontext

// EstimateChars returns the visible rune count used for approximate context
// budgeting. It intentionally avoids byte length so Korean text is not
// overcounted compared with ASCII.
func EstimateChars(text string) int {
	return len([]rune(text))
}

// EstimateTokens is a cheap, provider-neutral token estimate for UI budgeting.
// It is not a tokenizer; it only helps writers see whether a request is light
// or heavy before sending it.
func EstimateTokens(text string) int {
	chars := EstimateChars(text)
	if chars == 0 {
		return 0
	}
	return (chars + 2) / 3
}
