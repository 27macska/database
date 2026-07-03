package storage

import (
	"path/filepath"
	"testing"
)

func TestAppendAndReplay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.db")

	log, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := log.Append("SET", []string{"a", "1"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := log.Append("SET", []string{"b", "hello world"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := log.Append("DEL", []string{"a"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := log.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	var got []Record
	err = Replay(path, func(cmd string, args []string) error {
		got = append(got, Record{Cmd: cmd, Args: args})
		return nil
	})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}

	want := []Record{
		{Cmd: "SET", Args: []string{"a", "1"}},
		{Cmd: "SET", Args: []string{"b", "hello world"}},
		{Cmd: "DEL", Args: []string{"a"}},
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d records, got %d (%v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i].Cmd != want[i].Cmd || len(got[i].Args) != len(want[i].Args) {
			t.Fatalf("record %d mismatch: got %+v, want %+v", i, got[i], want[i])
		}
		for j := range want[i].Args {
			if got[i].Args[j] != want[i].Args[j] {
				t.Fatalf("record %d arg %d mismatch: got %q, want %q", i, j, got[i].Args[j], want[i].Args[j])
			}
		}
	}
}

func TestReplayMissingFileIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.db")

	called := false
	err := Replay(path, func(cmd string, args []string) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error replaying a missing log, got %v", err)
	}
	if called {
		t.Fatalf("expected apply to never be called for a missing log")
	}
}

func TestAppendPersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.db")

	log, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := log.Append("SET", []string{"k", "v"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := log.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Reopen (simulating process restart) and append more.
	log2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if err := log2.Append("SET", []string{"k2", "v2"}); err != nil {
		t.Fatalf("append after reopen: %v", err)
	}
	if err := log2.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	var count int
	err = Replay(path, func(cmd string, args []string) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 records surviving reopen, got %d", count)
	}
}
