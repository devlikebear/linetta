package note

type Note struct {
	ID        string `json:"id"`
	NodeID    string `json:"node_id"`
	Anchor    int    `json:"anchor"`
	Body      string `json:"body"`
	CreatedAt int64  `json:"created_at"`
}

type NewInput struct {
	NodeID string `json:"node_id"`
	Anchor int    `json:"anchor"`
	Body   string `json:"body"`
}

type UpdateInput struct {
	ID   string `json:"id"`
	Body string `json:"body"`
}
