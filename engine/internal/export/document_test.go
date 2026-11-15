package export

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"testing"
)

func para(text string) string {
	return `{"type":"doc","content":[{"type":"paragraph","content":[` +
		`{"type":"mention","attrs":{"id":"entity-secret","label":"수아"}},` +
		`{"type":"noteMarker","attrs":{"noteId":"note-secret"}},` +
		`{"type":"text","text":"` + text + `"}]}]}`
}

// A work whose first scene stays at the root, then 1화 with two scenes.
func documentFixture(t *testing.T) (context.Context, DocumentRequest, func(DocumentRequest) (DocumentPayload, error), string) {
	t.Helper()
	_, pr, nr, _, _ := newExportFixture(t)
	ctx := context.Background()
	p, err := pr.Create(ctx, 1, newProjectInput())
	if err != nil {
		t.Fatal(err)
	}
	if err := nr.UpdateContent(ctx, *p.LastOpenedNodeID, para("는 첫 장면"), 5); err != nil {
		t.Fatal(err)
	}
	episode, err := nr.CreateSibling(ctx, *p.LastOpenedNodeID, "container", "1화", "비", 10)
	if err != nil {
		t.Fatal(err)
	}
	for i, text := range []string{"는 둘째 장면", "는 셋째 장면"} {
		s, err := nr.CreateChild(ctx, episode.ID, "leaf", "씬", "", int64(20+i))
		if err != nil {
			t.Fatal(err)
		}
		if err := nr.UpdateContent(ctx, s.ID, para(text), int64(30+i)); err != nil {
			t.Fatal(err)
		}
	}
	run := func(req DocumentRequest) (DocumentPayload, error) {
		return ExportDocument(ctx, pr, nr, req, "ko")
	}
	return ctx, DocumentRequest{ProjectID: p.ID, Format: FormatTXT}, run, episode.ID
}

func decode(t *testing.T, out DocumentPayload) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(out.BytesBase64)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestExportDocumentWritesTheWorkAsText(t *testing.T) {
	_, req, run, _ := documentFixture(t)
	out, err := run(req)
	if err != nil {
		t.Fatal(err)
	}
	text := string(decode(t, out))
	want := "수아는 첫 장면\n\n1화 비\n\n수아는 둘째 장면\n\n* * *\n\n수아는 셋째 장면\n"
	if text != want {
		t.Fatalf("text:\n%q\nwant\n%q", text, want)
	}
	if out.Scenes != 3 || out.Chars == 0 || !strings.HasSuffix(out.SuggestedFilename, ".txt") || !strings.HasPrefix(out.MIME, "text/plain") {
		t.Fatalf("payload = %+v", out)
	}
}

func TestExportDocumentWritesOneEpisodeAsDOCX(t *testing.T) {
	_, req, run, episodeID := documentFixture(t)
	req.Format, req.NodeID = FormatDOCX, episodeID
	out, err := run(req)
	if err != nil {
		t.Fatal(err)
	}
	data := decode(t, out)
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("not a docx: %v", err)
	}
	var document string
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		if strings.Contains(string(b), "secret") {
			t.Errorf("%s carries an editing id", f.Name)
		}
		if f.Name == "word/document.xml" {
			document = string(b)
		}
	}
	if !strings.Contains(document, "수아는 둘째 장면") || strings.Contains(document, "첫 장면") {
		t.Fatalf("the episode export holds the wrong scenes:\n%s", document)
	}
	if out.Scenes != 2 || !strings.HasSuffix(out.SuggestedFilename, "-1화.docx") {
		t.Fatalf("payload = %+v", out)
	}
}

func TestExportDocumentWritesHWPX(t *testing.T) {
	_, req, run, _ := documentFixture(t)
	req.Format = FormatHWPX
	out, err := run(req)
	if err != nil {
		t.Fatal(err)
	}
	data := decode(t, out)
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || zr.File[0].Name != "mimetype" || zr.File[0].Method != zip.Store {
		t.Fatalf("not a Hangul package: err=%v", err)
	}
	if out.MIME != "application/hwp+zip" || !strings.HasSuffix(out.SuggestedFilename, ".hwpx") || out.Scenes != 3 {
		t.Fatalf("payload = %+v", out)
	}
}

func TestExportDocumentPreviewCountsWithoutWriting(t *testing.T) {
	_, req, run, episodeID := documentFixture(t)
	req.Preview, req.NodeID = true, episodeID
	out, err := run(req)
	if err != nil {
		t.Fatal(err)
	}
	if out.BytesBase64 != "" || out.Scenes != 2 {
		t.Fatalf("preview = %+v", out)
	}
}

func TestExportDocumentRefusesWhatTheDialogShouldNotSend(t *testing.T) {
	_, req, run, _ := documentFixture(t)
	tooBig, longBreak, unknown := req, req, req
	tooBig.Options.SizePt = 200
	long := strings.Repeat("*", 21)
	longBreak.Options.SceneBreak = &long
	unknown.Format = "pdf"
	elsewhere := req
	elsewhere.NodeID = "node-from-another-work"
	for name, r := range map[string]DocumentRequest{"size": tooBig, "scene break": longBreak, "format": unknown, "node": elsewhere} {
		_, err := run(r)
		var badReq *ErrBadDocumentRequest
		if !errors.As(err, &badReq) {
			t.Errorf("%s: err = %v, want a bad request", name, err)
		}
	}
}
