// Package storage implements the durable, append-only command log that
// backs the key-value store. Every mutating command is appended to a file
// (by default data.db) as one JSON record per line and fsynced before the
// write is acknowledged, so a crash never loses an acknowledged write. On
// startup the log is replayed from the beginning to rebuild in-memory
// state.
package storage

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"strings"
)

// Record is one logged command: a name (e.g. "SET") plus its arguments.
type Record struct {
	Cmd  string   `json:"cmd"`
	Args []string `json:"args"`
}

// Log is an append-only, fsync-on-write command log.
type Log struct {
	file *os.File
}

// Open opens (creating if necessary) the log file at path for appending.
func Open(path string) (*Log, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &Log{file: f}, nil
}

// Append writes one record to the log and fsyncs it to disk before
// returning, so the caller can treat the write as durable once Append
// returns without error.
func (l *Log) Append(cmd string, args []string) error {
	data, err := json.Marshal(Record{Cmd: cmd, Args: args})
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if _, err := l.file.Write(data); err != nil {
		return err
	}
	return l.file.Sync()
}

// AppendBatch writes multiple records with a single write syscall and a
// single fsync, rather than one of each per record. It is used to commit a
// transaction's buffered writes together, so the records making up one
// COMMIT land on disk as one unit instead of interleaving fsyncs with any
// concurrent writer.
func (l *Log) AppendBatch(records []Record) error {
	if len(records) == 0 {
		return nil
	}
	var buf bytes.Buffer
	for _, r := range records {
		data, err := json.Marshal(r)
		if err != nil {
			return err
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}
	if _, err := l.file.Write(buf.Bytes()); err != nil {
		return err
	}
	return l.file.Sync()
}

// Close closes the underlying log file.
func (l *Log) Close() error {
	return l.file.Close()
}

// Replay reads every record from the log file at path, in order, invoking
// apply for each. If the file does not exist, Replay treats that as an
// empty log and returns nil (a fresh store simply has no history yet).
func Replay(path string, apply func(cmd string, args []string) error) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return err
		}
		if err := apply(rec.Cmd, rec.Args); err != nil {
			return err
		}
	}
	return scanner.Err()
}
