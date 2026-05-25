package main

import (
	"flag"
	"fmt"
	"os"

	_ "github.com/devlikebear/tars/pkg/llm" // pin import to validate module path
)

func main() {
	stdio := flag.Bool("stdio", false, "serve JSONRPC over stdin/stdout")
	flag.Parse()

	if !*stdio {
		fmt.Fprintln(os.Stderr, "linetta-engine: --stdio required (other modes land in later plans)")
		os.Exit(2)
	}

	// Wired up in Task 6.
	fmt.Fprintln(os.Stderr, "linetta-engine: stdio mode placeholder")
}
