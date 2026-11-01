package vault

import "context"

// Store is where sealed revisions live. Everything above it, the bundle, the
// sealing, the conflict rules, is the same for every target. A store reports a
// conflict and missing blobs with the same *APIError the site does, so resuming
// and conflict handling have one path.
type Store interface {
	Account(ctx context.Context) (Account, error)
	Items(ctx context.Context) ([]RemoteItem, error)
	// Revisions lists an item's restore points, newest first.
	Revisions(ctx context.Context, itemID string) ([]Revision, error)
	PutBlob(ctx context.Context, id string, sealed []byte) error
	GetBlob(ctx context.Context, id string) ([]byte, error)
	Commit(ctx context.Context, itemID string, base int, manifest string, blobs []string) (int, error)
	DeleteItem(ctx context.Context, itemID string) error
}

// Round is a store that has to be brought up to date around a run: Git pulls
// before it and pushes after. Finish returning ErrRoundAgain means another
// device got in first, and the whole run goes again on top of it, once.
type Round interface {
	Begin(ctx context.Context) error
	Finish(ctx context.Context) error
}

// worksStore is Linetta Works: the site's client, with the token read per call
// so a device that disconnects stops at once.
type worksStore struct {
	client *Client
	token  string
}

func (w worksStore) Account(ctx context.Context) (Account, error) {
	return w.client.Account(ctx, w.token)
}

func (w worksStore) Items(ctx context.Context) ([]RemoteItem, error) {
	return w.client.Items(ctx, w.token)
}

func (w worksStore) Revisions(ctx context.Context, itemID string) ([]Revision, error) {
	return w.client.Revisions(ctx, w.token, itemID)
}

func (w worksStore) PutBlob(ctx context.Context, id string, sealed []byte) error {
	return w.client.PutBlob(ctx, w.token, id, sealed)
}

func (w worksStore) GetBlob(ctx context.Context, id string) ([]byte, error) {
	return w.client.GetBlob(ctx, w.token, id)
}

func (w worksStore) Commit(ctx context.Context, itemID string, base int, manifest string, blobs []string) (int, error) {
	return w.client.Commit(ctx, w.token, itemID, base, manifest, blobs)
}

func (w worksStore) DeleteItem(ctx context.Context, itemID string) error {
	return w.client.DeleteItem(ctx, w.token, itemID)
}
