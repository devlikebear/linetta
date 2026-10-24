package publish

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// fakeGallery is the work's gallery: illustration id → bytes.
type fakeGallery map[string][]byte

func (g fakeGallery) IDsInProject(context.Context, string) (map[string]bool, error) {
	out := map[string]bool{}
	for id := range g {
		out[id] = true
	}
	return out, nil
}

func (g fakeGallery) IllustrationImage(_ context.Context, id string) (string, []byte, error) {
	data, ok := g[id]
	if !ok {
		return "", nil, fmt.Errorf("no picture %s", id)
	}
	return "image/png", data, nil
}

// pictureDoc is a scene: text, then each picture id with the caption "그림 <id>".
func pictureDoc(text string, ids ...string) string {
	blocks := []string{`{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}`}
	for _, id := range ids {
		blocks = append(blocks, fmt.Sprintf(`{"type":"illustration","attrs":{"id":%q,"caption":"그림 %s"}}`, id, id))
	}
	return `{"type":"doc","content":[` + strings.Join(blocks, ",") + `]}`
}

func pictureFixture(t *testing.T, gallery fakeGallery, ids ...string) *serviceFixture {
	t.Helper()
	f := newServiceFixture(t, 1)
	f.svc.WithIllustrations(gallery)
	if err := f.nodes.UpdateContent(context.Background(), f.episodes[0], pictureDoc("비가 내렸다.", ids...), 500); err != nil {
		t.Fatal(err)
	}
	f.register(t)
	return f
}

func imagePuts(requests []string) []string {
	var out []string
	for _, r := range requests {
		if strings.HasPrefix(r, "PUT ") && strings.Contains(r, "/images/") {
			out = append(out, r[strings.LastIndex(r, "/")+1:])
		}
	}
	return out
}

func TestService_publishUploadsPicturesBeforeTheEpisode(t *testing.T) {
	gallery := fakeGallery{"pic-a": []byte("PNG-A"), "pic-b": []byte("PNG-B")}
	f := pictureFixture(t, gallery, "pic-b", "pic-a", "pic-b", "gone")
	before := len(f.site.seen())

	res := f.publish(t, f.episodes[0])
	sent := f.site.seen()[before:]
	if got := strings.Join(imagePuts(sent), ","); got != "pic-b,pic-a" {
		t.Fatalf("uploads = %s (requests %v)", got, sent)
	}
	lastImage, episodePut := -1, -1
	for i, r := range sent {
		if strings.Contains(r, "/images/") {
			lastImage = i
		} else if strings.HasPrefix(r, "PUT ") && strings.Contains(r, "/episodes/") {
			episodePut = i
		}
	}
	if episodePut < lastImage {
		t.Fatalf("the episode went before its pictures: %v", sent)
	}
	if res.Episode.Illustrations != 2 {
		t.Fatalf("illustrations = %d", res.Episode.Illustrations)
	}
	work := f.site.works[f.project.ID]
	if work.imageTypes[f.episodes[0]+"/pic-a"] != "image/png" {
		t.Fatalf("content type = %q", work.imageTypes[f.episodes[0]+"/pic-a"])
	}
	if eps := f.site.episodes(f.project.ID); len(eps) != 1 || len(eps[0].Images) != 2 {
		t.Fatalf("site episodes = %+v", eps)
	}
}

func TestService_republishSendsOnlyNewPictures(t *testing.T) {
	gallery := fakeGallery{"pic-a": []byte("PNG-A"), "pic-b": []byte("PNG-B")}
	f := pictureFixture(t, gallery, "pic-a")
	f.publish(t, f.episodes[0])

	before := len(f.site.seen())
	f.publish(t, f.episodes[0])
	if got := imagePuts(f.site.seen()[before:]); len(got) != 0 {
		t.Fatalf("unchanged pictures were sent again: %v", got)
	}

	if err := f.nodes.UpdateContent(context.Background(), f.episodes[0], pictureDoc("고쳤다.", "pic-a", "pic-b"), 600); err != nil {
		t.Fatal(err)
	}
	before = len(f.site.seen())
	f.publish(t, f.episodes[0])
	if got := strings.Join(imagePuts(f.site.seen()[before:]), ","); got != "pic-b" {
		t.Fatalf("uploads after adding a picture = %s", got)
	}
}

func TestService_tooManyPicturesStopsBeforeSending(t *testing.T) {
	gallery := fakeGallery{}
	var ids []string
	for i := 0; i <= MaxImagesPerEpisode; i++ {
		id := fmt.Sprintf("pic-%02d", i)
		gallery[id] = []byte(id)
		ids = append(ids, id)
	}
	f := pictureFixture(t, gallery, ids...)
	before := len(f.site.seen())
	_, err := f.svc.PublishEpisode(context.Background(), f.episodes[0])
	if reasonOf(err) != rpc.ReasonPublishTooManyImages {
		t.Fatalf("err = %v", err)
	}
	if sent := f.site.seen()[before:]; len(sent) != 0 {
		t.Fatalf("requests sent: %v", sent)
	}
}

func TestService_failedPictureUploadPublishesNothing(t *testing.T) {
	f := pictureFixture(t, fakeGallery{"pic-a": []byte("PNG-A")}, "pic-a")
	f.site.mu.Lock()
	f.site.fail = append(f.site.fail, fakeFailure{match: "PUT /api/artworks/works/" + f.project.ID + "/episodes/" + f.episodes[0] + "/images/", status: 413, code: CodeTooLarge})
	f.site.mu.Unlock()

	_, err := f.svc.PublishEpisode(context.Background(), f.episodes[0])
	if reasonOf(err) != rpc.ReasonPublishImageTooLarge {
		t.Fatalf("err = %v", err)
	}
	if eps := f.site.episodes(f.project.ID); len(eps) != 0 {
		t.Fatalf("the episode went up without its picture: %+v", eps)
	}
	if n, _ := f.nodes.Get(context.Background(), f.episodes[0]); n.Status == node.StatusPublished {
		t.Fatal("the episode was marked published")
	}
}

func TestService_workStatusCountsThePictures(t *testing.T) {
	f := pictureFixture(t, fakeGallery{"pic-a": []byte("A")}, "pic-a", "gone")
	ws, err := f.svc.WorkStatus(context.Background(), f.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Episodes[0].Illustrations != 1 {
		t.Fatalf("illustrations = %d", ws.Episodes[0].Illustrations)
	}
}
