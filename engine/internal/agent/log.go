//go:build !mobile

package agent

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// logf writes agent diagnostics. Failures here are never fatal to a turn: a
// transcript row that did not save must not cost the writer the reply it
// belonged to.
//
// Until a log file is set they go to stdout, the way mcphost's do. The app
// sets one (Deps.LogPath) because stdout goes nowhere when the engine runs
// inside a desktop app opened from the Finder — and a turn that failed with
// nothing on record is how a Gemini timeout passed for a dead network: the
// panel can only show the reason code, and the cause was gone.
func logf(format string, args ...any) {
	agentLog.write(fmt.Sprintf("agent: "+format+"\n", args...))
}

// logFileMax is where the log rolls over to a single ".1" backup: enough for
// weeks of failures, small enough that nobody has to think about it.
const logFileMax = 1 << 20

type logSink struct {
	mu   sync.Mutex
	path string
	file *os.File
	size int64
}

var agentLog logSink

func (l *logSink) write(line string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		_, _ = io.WriteString(os.Stdout, line)
		return
	}
	if l.size >= logFileMax {
		l.rotate()
	}
	n, _ := io.WriteString(l.file, time.Now().Format(time.RFC3339)+" "+line)
	l.size += int64(n)
}

// setLogFile points logf at path, creating its directory. A failure leaves
// logging on stdout and says so there; it is not a reason to refuse to start
// the agent.
func setLogFile(path string) {
	agentLog.mu.Lock()
	defer agentLog.mu.Unlock()
	if agentLog.file != nil && agentLog.path == path {
		return
	}
	if err := agentLog.open(path); err != nil {
		fmt.Printf("agent: log file: %v\n", err)
	}
}

// open replaces the current file with path. Called with mu held.
func (l *logSink) open(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if l.file != nil {
		_ = l.file.Close()
	}
	l.path, l.file, l.size = path, f, 0
	if info, err := f.Stat(); err == nil {
		l.size = info.Size()
	}
	return nil
}

// rotate moves a full log aside and starts a fresh one. Called with mu held.
// If the fresh one cannot be opened, logging falls back to stdout rather than
// growing the full file without bound.
func (l *logSink) rotate() {
	path := l.path
	_ = l.file.Close()
	l.file = nil
	_ = os.Rename(path, path+".1")
	if err := l.open(path); err != nil {
		fmt.Printf("agent: log file: %v\n", err)
	}
}
