//go:build mobile

package semantic

import "context"

// OpenEmbedder: the mobile engine does not carry the runtime yet (#384 Phase
// 4 decides after measuring memory on a phone).
func OpenEmbedder(context.Context, string, string) (Embedder, error) {
	return nil, ErrUnsupported
}

// Supported: this build cannot run the model.
const Supported = false
