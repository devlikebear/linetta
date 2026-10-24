package engineapp

import (
	"context"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/export"
	"github.com/devlikebear/linetta/engine/internal/illustration"
)

// exportImages hands export the gallery's pictures. A picture the gallery no
// longer has is export.ErrIllustrationMissing, which the export leaves out
// rather than failing the whole file.
type exportImages struct {
	repo *illustration.Repo
}

func (e exportImages) IllustrationImage(ctx context.Context, id string) (string, []byte, error) {
	img, err := e.repo.Get(ctx, id)
	if errors.Is(err, illustration.ErrNotFound) {
		return "", nil, export.ErrIllustrationMissing
	}
	if err != nil {
		return "", nil, err
	}
	return img.MIME, img.Data, nil
}
