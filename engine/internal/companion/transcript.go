package companion

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/devlikebear/tars/pkg/session"
)

func readSessionMessages(path string) ([]session.Message, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open transcript %s: %w", path, err)
	}
	defer f.Close()

	reader := bufio.NewReaderSize(f, 1024*1024)
	var messages []session.Message
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			line = bytes.TrimRight(line, "\r\n")
			if len(bytes.TrimSpace(line)) > 0 {
				var msg session.Message
				if unmarshalErr := json.Unmarshal(line, &msg); unmarshalErr != nil {
					return nil, fmt.Errorf("unmarshal message: %w", unmarshalErr)
				}
				messages = append(messages, msg)
			}
		}
		if err == nil {
			continue
		}
		if err == io.EOF {
			return messages, nil
		}
		return nil, fmt.Errorf("read transcript: %w", err)
	}
}
