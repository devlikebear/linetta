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

// logSink holds only the path, never an open file. The file is opened, written
// and closed on every line: lines are rare (failures, mostly), and a handle
// held for the life of the process is one Windows will not let anything
// delete or rename — the data directory, a test's temp home, the rotation
// below.
type logSink struct {
	mu   sync.Mutex
	path string
}

var agentLog logSink

func (l *logSink) write(line string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.path == "" {
		_, _ = io.WriteString(os.Stdout, line)
		return
	}
	if info, err := os.Stat(l.path); err == nil && info.Size() >= logFileMax {
		_ = os.Rename(l.path, l.path+".1")
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		_, _ = io.WriteString(os.Stdout, line)
		return
	}
	_, _ = io.WriteString(f, time.Now().Format(time.RFC3339)+" "+line)
	_ = f.Close()
}

// setLogFile points logf at path, creating its directory. A failure leaves
// logging on stdout and says so there; it is not a reason to refuse to start
// the agent.
func setLogFile(path string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		fmt.Printf("agent: log file: %v\n", err)
		return
	}
	agentLog.mu.Lock()
	agentLog.path = path
	agentLog.mu.Unlock()
}
