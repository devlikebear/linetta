//go:build !mobile

package agent

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// Limits on the images a writer attaches to one message. They ride in the
// request body of every model call the turn makes, so the bound is on what a
// turn of up to maxIterations round-trips re-sends, not on what one upload
// can carry.
const (
	MaxImages          = 4
	maxImageBytes      = 5 * 1024 * 1024
	maxTotalImageBytes = 12 * 1024 * 1024
)

// Image is one picture the writer attached to a message.
type Image struct {
	MIME string
	Data []byte
}

// checkImages confirms the attachments are pictures every provider reads and
// within the limits, and returns them with the type their bytes say they
// are: a file's name or the browser's guess is not trusted.
func checkImages(images []Image) ([]Image, error) {
	if len(images) > MaxImages {
		return nil, attachmentErr("at most %d images can be attached to a message", MaxImages)
	}
	out := make([]Image, 0, len(images))
	total := 0
	for i, img := range images {
		if len(img.Data) == 0 {
			return nil, attachmentErr("image %d is empty", i+1)
		}
		if len(img.Data) > maxImageBytes {
			return nil, attachmentErr("image %d is larger than 5 MiB", i+1)
		}
		mime := http.DetectContentType(img.Data)
		switch mime {
		case "image/jpeg", "image/png", "image/webp", "image/gif":
		default:
			return nil, attachmentErr("image %d is not a JPEG, PNG, WebP or GIF image", i+1)
		}
		total += len(img.Data)
		out = append(out, Image{MIME: mime, Data: img.Data})
	}
	if total > maxTotalImageBytes {
		return nil, attachmentErr("the attached images come to more than 12 MiB")
	}
	return out, nil
}

func attachmentErr(format string, args ...any) error {
	return &rpc.ReasonError{Reason: rpc.ReasonAgentAttachmentInvalid, Err: fmt.Errorf("agent: "+format, args...)}
}

// errImagesUnsupported: the Claude Code CLI provider is handed a prompt as
// text, so it has no way to receive a picture.
var errImagesUnsupported = &rpc.ReasonError{
	Reason: rpc.ReasonAgentAttachmentUnsupported,
	Err:    errors.New("agent: the selected provider cannot receive attached images"),
}

// withImages marks a message that came with pictures, so the transcript,
// the panel and later turns all know there were some: only the turn they
// came with sends the pictures themselves.
func withImages(n int, prompt string) string {
	if n == 0 {
		return prompt
	}
	return fmt.Sprintf("%s\n\n<images count=\"%d\"/>", prompt, n)
}

// Attachment returns image n (1-based) of the message run runID is
// answering, while that turn runs.
func (s *Service) Attachment(runID string, n int) (string, []byte, bool) {
	img, ok := s.runs.image(runID, n)
	return img.MIME, img.Data, ok
}
