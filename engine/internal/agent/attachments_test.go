//go:build !mobile

package agent

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/tars/pkg/llm"
)

var pngBytes = []byte("\x89PNG\r\n\x1a\nface")

func TestRun_sendsAttachedImagesWithThisMessageOnly(t *testing.T) {
	rec := &recorder{}
	c := &scriptedClient{responses: []llm.ChatResponse{textReply("봤어요")}}
	svc := newService(t, c, rec)

	runID, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "이 얼굴로 그려줘",
		Images: []Image{{MIME: "image/jpeg", Data: pngBytes}, {Data: pngBytes}}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "agent.done", func() bool { return rec.hasTerminalFor(runID) })

	msgs := c.messages()
	last := msgs[len(msgs)-1]
	if len(last.ContentBlocks) != 3 || last.ContentBlocks[0].Type != "text" || !strings.Contains(last.ContentBlocks[0].Text, "이 얼굴로 그려줘") {
		t.Fatalf("blocks = %+v, want the text then the two images", last.ContentBlocks)
	}
	for _, b := range last.ContentBlocks[1:] {
		// The type comes from the bytes, not from what the panel said.
		if b.Type != "image" || b.MediaType != "image/png" || b.Data != base64.StdEncoding.EncodeToString(pngBytes) {
			t.Fatalf("image block = %+v", b)
		}
	}
	if !strings.HasSuffix(last.ContentBlocks[0].Text, `<images count="2"/>`) {
		t.Fatalf("text %q does not say images came with it", last.ContentBlocks[0].Text)
	}

	history, err := svc.History(context.Background(), "p1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if history[0].Role != "user" || history[0].Content != "이 얼굴로 그려줘\n\n<images count=\"2\"/>" {
		t.Fatalf("transcript row = %q", history[0].Content)
	}
	// The pictures are the turn's alone: once it ends, tools cannot reach them.
	if _, _, ok := svc.Attachment(runID, 1); ok {
		t.Fatal("an attachment outlived its turn")
	}

	// A later turn replays the marker, not the pictures.
	c2 := &scriptedClient{responses: []llm.ChatResponse{textReply("네")}}
	svc.deps.Providers = fakeProviders{client: c2}
	second, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "다시"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "second turn", func() bool { return rec.hasTerminalFor(second) })
	for _, m := range c2.messages() {
		if len(m.ContentBlocks) > 0 {
			t.Fatalf("a later turn re-sent pictures: %+v", m)
		}
	}
}

func TestRun_refusesBadAttachmentsBeforeAnythingIsSent(t *testing.T) {
	big := append(append([]byte{}, pngBytes...), make([]byte, maxImageBytes)...)
	five := []Image{{Data: pngBytes}, {Data: pngBytes}, {Data: pngBytes}, {Data: pngBytes}, {Data: pngBytes}}
	for name, images := range map[string][]Image{
		"not a picture": {{MIME: "image/png", Data: []byte("hello")}},
		"empty":         {{MIME: "image/png"}},
		"too large":     {{Data: big}},
		"too many":      five,
	} {
		c := &scriptedClient{responses: []llm.ChatResponse{textReply("x")}}
		svc := newService(t, c, &recorder{})
		_, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "x", Images: images})
		var re *rpc.ReasonError
		if !errors.As(err, &re) || re.Reason != rpc.ReasonAgentAttachmentInvalid {
			t.Fatalf("%s: err = %v", name, err)
		}
		if h, _ := svc.History(context.Background(), "p1", 10); len(h) != 0 || c.count() != 0 {
			t.Fatalf("%s: a refused message reached the transcript or the model", name)
		}
	}
}

func TestRun_theCLIProviderCannotTakeImages(t *testing.T) {
	st := openStoreForAgentTests(t)
	svc := New(Deps{
		Providers: cliProviders{client: &scriptedClient{responses: []llm.ChatResponse{textReply("x")}}},
		History:   companion.NewHistoryRepo(st.DB()),
		Register:  stubTools(nil),
		Notify:    (&recorder{}).notify,
		Clock:     func() int64 { return 1 },
	})
	t.Cleanup(func() { _ = svc.Close() })
	_, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "x", Images: []Image{{Data: pngBytes}}})
	var re *rpc.ReasonError
	if !errors.As(err, &re) || re.Reason != rpc.ReasonAgentAttachmentUnsupported {
		t.Fatalf("err = %v", err)
	}
}

func TestAttachment_numbersFromOneWhileTheTurnRuns(t *testing.T) {
	r := newRunRegistry()
	if err := r.start("p1", "run-1", func() {}); err != nil {
		t.Fatal(err)
	}
	r.attach("run-1", []Image{{MIME: "image/png", Data: []byte("a")}, {MIME: "image/gif", Data: []byte("b")}})
	if img, ok := r.image("run-1", 2); !ok || img.MIME != "image/gif" {
		t.Fatalf("image 2 = %+v %v", img, ok)
	}
	for _, n := range []int{0, 3} {
		if _, ok := r.image("run-1", n); ok {
			t.Fatalf("image %d found", n)
		}
	}
	r.finish("p1", "run-1")
	if _, ok := r.image("run-1", 1); ok {
		t.Fatal("image kept after the turn")
	}
}
