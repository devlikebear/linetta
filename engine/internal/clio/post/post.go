// Package post stores the writer's own posts, the raw material of a voice profile.
package post

// Platforms, kinds and sources a post may carry. The schema CHECKs the same sets.
const (
	PlatformX       = "x"
	PlatformThreads = "threads"
	PlatformBlog    = "blog"

	KindPost  = "post"
	KindReply = "reply"

	SourceImport  = "import"
	SourceWritten = "written"
)

// NewPost is a post not yet stored. ID and CreatedAt are assigned on insert.
type NewPost struct {
	Body       string `json:"body"`
	Platform   string `json:"platform"`
	Kind       string `json:"kind"`
	Source     string `json:"source"`
	ExternalID string `json:"external_id"`
	PostedAt   int64  `json:"posted_at"`
}

// Post is a stored post.
type Post struct {
	ID         string `json:"id"`
	Body       string `json:"body"`
	Platform   string `json:"platform"`
	Kind       string `json:"kind"`
	Source     string `json:"source"`
	ExternalID string `json:"external_id"`
	PostedAt   int64  `json:"posted_at"`
	CreatedAt  int64  `json:"created_at"`
}

// ContextCount is how many posts one platform/kind context holds.
type ContextCount struct {
	Platform string `json:"platform"`
	Kind     string `json:"kind"`
	Count    int    `json:"count"`
}

// ListParams filters List. Empty Platform or Kind match any; Limit <= 0 means no limit.
type ListParams struct {
	Platform string
	Kind     string
	Limit    int
	Offset   int
}
