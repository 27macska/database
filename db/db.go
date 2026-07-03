// Package db is the command engine: it wires the in-memory index
// (package index) to the durable append-only log (package storage) and
// implements the store's command set and its BEGIN/COMMIT/ABORT
// transactions. Execute is the single entry point the CLI (or tests) call
// with a tokenized command line.
package db

import (
	"errors"
	"fmt"
	"time"

	"kvstore/index"
	"kvstore/storage"
)

// ErrExit is returned by Execute when the client issues EXIT; the CLI loop
// treats it as a signal to stop reading, not as a failure.
var ErrExit = errors.New("exit requested")

const (
	nilStr   = "(nil)"
	emptyStr = "(empty)"
)

// DB is the top-level store: an in-memory index plus the log that makes it
// durable, along with any in-progress transaction state.
type DB struct {
	idx *index.Index
	log *storage.Log

	// Clock returns the current unix time in seconds. It is a field
	// rather than a direct time.Now() call so tests can inject a fixed
	// or stepping clock to exercise EXPIRE/TTL deterministically.
	Clock func() int64

	inTx     bool
	snapshot *index.Index
	pending  []storage.Record
}

// Open replays the log at path (if any) to rebuild the index, then opens
// the same file for appending further commands. A path that doesn't exist
// yet simply starts as an empty store.
func Open(path string) (*DB, error) {
	d := &DB{
		idx:   index.New(),
		Clock: func() int64 { return time.Now().Unix() },
	}
	if err := storage.Replay(path, d.applyReplayed); err != nil {
		return nil, fmt.Errorf("replay %s: %w", path, err)
	}
	l, err := storage.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	d.log = l
	return d, nil
}

// Close releases the underlying log file.
func (d *DB) Close() error {
	return d.log.Close()
}

// logMutation durably records a committed mutation. Inside a transaction
// the record is buffered until COMMIT instead of being written straight
// away, so ABORT can discard it without touching disk.
func (d *DB) logMutation(cmd string, args []string) error {
	if d.inTx {
		d.pending = append(d.pending, storage.Record{Cmd: cmd, Args: args})
		return nil
	}
	return d.log.Append(cmd, args)
}

// applyReplayed re-applies one previously-logged mutation while rebuilding
// the index from disk. It is intentionally the only place that maps a
// logged command name back to an index mutation, shared with Execute's own
// doXXX helpers so live execution and replay can never drift apart.
func (d *DB) applyReplayed(cmd string, args []string) error {
	switch cmd {
	case "SET":
		return d.doSET(args)
	case "DEL":
		_, err := d.doDEL(args)
		return err
	case "MSET":
		return d.doMSET(args)
	case "EXPIREAT":
		return d.doEXPIREAT(args)
	case "HSET":
		return d.doHSET(args)
	case "LPUSH":
		_, err := d.doLPUSH(args)
		return err
	case "RPUSH":
		_, err := d.doRPUSH(args)
		return err
	case "INCR":
		_, err := d.doINCR(args)
		return err
	case "DECR":
		_, err := d.doDECR(args)
		return err
	case "FLUSHDB":
		return d.doFLUSHDB(args)
	default:
		return fmt.Errorf("replay: unknown logged command %q", cmd)
	}
}
