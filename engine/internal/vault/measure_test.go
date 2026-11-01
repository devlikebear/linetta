package vault

import (
	"context"
	"crypto/rand"
	"fmt"
	mrand "math/rand/v2"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/vaultbundle"
)

// TestMeasureLargeWork is T3.1, kept so the numbers can be taken again:
// LINETTA_MEASURE=1 go test ./internal/vault -run TestMeasureLargeWork -v
func TestMeasureLargeWork(t *testing.T) {
	if os.Getenv("LINETTA_MEASURE") == "" {
		t.Skip("set LINETTA_MEASURE=1 to take the numbers")
	}
	ctx := context.Background()
	site := newFakeSite(t)
	a := newDevice(t, site)
	a.makeKey(t)
	const (
		scenes      = 200
		sceneChars  = 4000
		snapshots   = 10
		pictures    = 60
		pictureSize = 320 << 10
	)
	exec(t, a.db, `INSERT INTO projects (id, title, genres, length_target, default_pov, created_at, updated_at) VALUES (?, '큰 작품', '[]', 'long', '3rd', 1, 2)`, workID)
	// Random syllables, because repeated text compresses far better than prose
	// and hides what an upload really costs.
	body := func(i int) string {
		rng := mrand.New(mrand.NewPCG(uint64(i), 7))
		var text strings.Builder
		for range sceneChars {
			text.WriteRune(rune(0xAC00 + rng.IntN(400)))
		}
		return doc(text.String())
	}
	for i := range scenes {
		id := fmt.Sprintf("%s-n%d", workID, i+1)
		exec(t, a.db, `INSERT INTO nodes (id, project_id, ordinal, kind, label, content_doc, created_at, updated_at) VALUES (?, ?, ?, 'leaf', '씬', ?, 1, 2)`, id, workID, i, body(i))
		for j := range snapshots {
			exec(t, a.db, `INSERT INTO node_snapshots (id, node_id, content_doc, reason, created_at) VALUES (?, ?, ?, 'auto', ?)`, fmt.Sprintf("%s-s%d", id, j), id, body(i+j), j)
		}
	}
	for i := range pictures {
		picture := make([]byte, pictureSize)
		_, _ = rand.Read(picture)
		exec(t, a.db, `INSERT INTO illustrations (id, project_id, prompt, provider, model, mime, byte_size, data, created_at) VALUES (?, ?, 'p', 'p', 'm', 'image/png', ?, ?, 1)`, fmt.Sprintf("%s-i%d", workID, i), workID, pictureSize, picture)
	}
	measure := func(name string, fn func()) time.Duration {
		start := time.Now()
		fn()
		d := time.Since(start)
		t.Logf("%-28s %v", name, d.Round(time.Millisecond))
		return d
	}
	for range 3 {
		measure("export", func() {
			if _, err := vaultbundle.Export(ctx, a.db, a.home, workID, time.Now(), nil); err != nil {
				t.Fatal(err)
			}
		})
	}
	bundle, err := vaultbundle.Export(ctx, a.db, a.home, workID, time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var total, tables, history, images int
	for _, part := range bundle.Manifest.Parts {
		total += part.Size
		switch vaultbundle.PartCategory(part.Name) {
		case vaultbundle.CategoryImages:
			images += part.Size
		case vaultbundle.CategoryHistory:
			history += part.Size
		default:
			tables += part.Size
		}
	}
	t.Logf("plain total %d MB: images %d, history %d, rest %d", total>>20, images>>20, history>>20, tables>>20)

	bytesSent := func() int {
		site.mu.Lock()
		defer site.mu.Unlock()
		n := 0
		for _, blob := range site.blobs {
			n += len(blob)
		}
		return n
	}
	measure("first push", func() { a.push(t, workID) })
	first := bytesSent()
	t.Logf("first push sealed %d KB in %d blobs", first>>10, len(site.blobs))

	exec(t, a.db, `UPDATE nodes SET content_doc = ? WHERE id = ?`, body(9999), workID+"-n1")
	exec(t, a.db, `INSERT INTO node_snapshots (id, node_id, content_doc, reason, created_at) VALUES (?, ?, ?, 'auto', 99)`, "new-snap", workID+"-n1", body(9999))
	measure("second push (one scene)", func() { a.push(t, workID) })
	t.Logf("one edited scene sent %d KB", (bytesSent()-first)>>10)
}
