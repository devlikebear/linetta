package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/devlikebear/linetta/engine/internal/engineapp"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

func main() {
	stdio := flag.Bool("stdio", false, "serve JSONRPC over stdin/stdout")
	flag.Parse()

	if !*stdio {
		fmt.Fprintln(os.Stderr, "linetta-engine: --stdio required (other modes land in later plans)")
		os.Exit(2)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := serveStdio(ctx, os.Stdin, os.Stdout, ""); err != nil {
		fail("serve: %v", err)
	}
}

func serveStdio(ctx context.Context, in io.Reader, out io.Writer, home string) error {
	app, err := engineapp.Open(ctx, engineapp.Options{Home: home})
	if err != nil {
		return err
	}
	defer app.Close()

	var mu sync.Mutex
	write := func(b []byte) error {
		mu.Lock()
		defer mu.Unlock()
		_, err := out.Write(append(b, '\n'))
		return err
	}

	app.SetNotifier(func(method string, params json.RawMessage) {
		if len(params) == 0 {
			params = json.RawMessage("null")
		}
		msg, err := json.Marshal(rpc.Message{Method: method, Params: params})
		if err != nil {
			return
		}
		_ = write(msg)
	})

	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		resp, err := app.Handle(ctx, scanner.Bytes())
		if err != nil {
			return err
		}
		if resp != nil {
			if err := write(resp); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "linetta-engine: "+format+"\n", args...)
	os.Exit(1)
}
