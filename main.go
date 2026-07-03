// Command kvstore is a persistent, single-binary key-value store. It reads
// one command per line from stdin and writes each command's reply to
// stdout, so it can be driven interactively or by piping a script of
// commands into it for automated (black-box) testing. All state is kept
// durable by appending every mutating command to an on-disk log (data.db
// by default), which is replayed to rebuild the in-memory index on
// startup.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"kvstore/db"
)

func main() {
	path := flag.String("db", "data.db", "path to the append-only log file")
	flag.Parse()

	store, err := db.Open(*path)
	if err != nil {
		log.Fatalf("kvstore: failed to open %s: %v", *path, err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("kvstore: error closing %s: %v", *path, err)
		}
	}()

	run(store, os.Stdin, os.Stdout)
}

// run drives the read-eval-print loop against an already-open store. It is
// separated from main so tests can exercise the full command loop against
// an in-memory reader/writer instead of real stdin/stdout.
func run(store *db.DB, in io.Reader, out io.Writer) {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	w := bufio.NewWriter(out)

	for scanner.Scan() {
		tokens := db.Tokenize(scanner.Text())
		if len(tokens) == 0 {
			continue
		}

		result, err := store.Execute(tokens)
		if err != nil {
			if errors.Is(err, db.ErrExit) {
				break
			}
			// A non-ErrExit error means the durable log write itself
			// failed, so we can no longer guarantee persistence.
			// Report it and stop rather than silently continuing.
			fmt.Fprintf(w, "ERR fatal: %v\n", err) //nolint:errcheck // best-effort diagnostic before exiting
			_ = w.Flush()
			return
		}
		fmt.Fprintln(w, result) //nolint:errcheck // write failures to stdout are unrecoverable here
		_ = w.Flush()
	}
}
