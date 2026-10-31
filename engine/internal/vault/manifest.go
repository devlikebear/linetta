package vault

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/devlikebear/linetta/engine/internal/vaultbundle"
)

// manifestFormat is the layout of a revision's manifest blob. Like the sealed
// format it is permanent once shipped, and a change takes a new number.
const manifestFormat = 1

// revisionManifest is the sealed first blob of a revision: everything the server
// must not see about it. Title is here so a device that has never held the work
// can name it before downloading the rest.
type revisionManifest struct {
	Format int                  `json:"format"`
	Title  string               `json:"title"`
	Bundle vaultbundle.Manifest `json:"bundle"`
	// Blobs maps each part to its blobs in order. A part of no bytes has none.
	Blobs map[string][]string `json:"blobs"`
}

func openManifest(keys *Keys, id string, sealed []byte) (*revisionManifest, error) {
	plain, err := keys.Open(id, sealed)
	if err != nil {
		return nil, err
	}
	var m revisionManifest
	if err := json.Unmarshal(plain, &m); err != nil {
		return nil, fmt.Errorf("%w: manifest: %v", ErrCorrupt, err)
	}
	if m.Format > manifestFormat {
		return nil, ErrNewerFormat
	}
	return &m, nil
}

// outgoing is a revision ready to send. Blobs stay plain until the site asks for
// them, so a retry that is missing two blobs does not seal four hundred.
type outgoing struct {
	blobIDs        []string
	plain          map[string][]byte
	manifestID     string
	sealedManifest []byte
}

// prepare splits a bundle into blobs and seals its manifest.
func prepare(keys *Keys, title string, bundle *vaultbundle.Bundle) (*outgoing, error) {
	out := &outgoing{plain: map[string][]byte{}}
	manifest := revisionManifest{Format: manifestFormat, Title: title, Bundle: bundle.Manifest, Blobs: map[string][]string{}}
	for _, part := range bundle.Manifest.Parts {
		for _, chunk := range split(bundle.Parts[part.Name]) {
			id := keys.BlobID(chunk)
			manifest.Blobs[part.Name] = append(manifest.Blobs[part.Name], id)
			if _, seen := out.plain[id]; !seen {
				out.plain[id] = chunk
				out.blobIDs = append(out.blobIDs, id)
			}
		}
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	if out.manifestID, out.sealedManifest, err = keys.Seal(raw); err != nil {
		return nil, err
	}
	return out, nil
}

func split(data []byte) [][]byte {
	var chunks [][]byte
	for len(data) > 0 {
		n := min(len(data), MaxPlaintext)
		chunks = append(chunks, data[:n])
		data = data[n:]
	}
	return chunks
}

// IDs from the server name files in the download cache, so they are checked
// before they get near a path.
var (
	blobIDPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	itemIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,64}$`)
)
