package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"kvstore/db"
)

func TestRunEndToEnd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = store.Close() }()

	script := strings.Join([]string{
		`SET name gopher`,
		`GET name`,
		`SET greeting "hello world"`,
		`GET greeting`,
		`INCR visits`,
		`INCR visits`,
		`EXISTS name missing`,
		`EXIT`,
	}, "\n")

	var out bytes.Buffer
	run(store, strings.NewReader(script), &out)

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	want := []string{"OK", "gopher", "OK", "hello world", "1", "2", "1"}
	if len(lines) != len(want) {
		t.Fatalf("got %d output lines %v, want %d %v", len(lines), lines, len(want), want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("line %d = %q, want %q (full output: %q)", i, lines[i], want[i], out.String())
		}
	}
}

func TestRunStopsOnExit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = store.Close() }()

	script := "SET a 1\nEXIT\nSET b 2\n"
	var out bytes.Buffer
	run(store, strings.NewReader(script), &out)

	got := strings.TrimRight(out.String(), "\n")
	if got != "OK" {
		t.Fatalf("expected only the pre-EXIT command to run, got %q", got)
	}
}

// buildBinary compiles the kvstore binary from the current package so the
// restart test below exercises the real process-boundary durability
// contract instead of just the in-process db.DB type.
func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "kvstore-under-test")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, ".")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func runBinary(t *testing.T, bin, dbPath, stdin string) string {
	t.Helper()
	cmd := exec.Command(bin, "-db", dbPath)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("running binary: %v\n%s", err, out)
	}
	return string(out)
}

// TestBinaryPersistsAcrossRestart is the black-box test for the
// assignment's core requirement: data must survive restarts. It runs the
// compiled binary as two entirely separate OS processes against the same
// data.db file and checks that a value written in the first process is
// visible to the second.
func TestBinaryPersistsAcrossRestart(t *testing.T) {
	bin := buildBinary(t)
	dbPath := filepath.Join(t.TempDir(), "data.db")

	out1 := runBinary(t, bin, dbPath, "SET name gopher\nGET name\nEXIT\n")
	if strings.TrimSpace(out1) != "OK\ngopher" {
		t.Fatalf("unexpected first-run output: %q", out1)
	}

	out2 := runBinary(t, bin, dbPath, "GET name\nEXIT\n")
	if strings.TrimSpace(out2) != "gopher" {
		t.Fatalf("expected value to survive process restart, got %q", out2)
	}
}

func TestBinaryPersistsTransactionAcrossRestart(t *testing.T) {
	bin := buildBinary(t)
	dbPath := filepath.Join(t.TempDir(), "data.db")

	runBinary(t, bin, dbPath, "BEGIN\nSET a 1\nSET b 2\nCOMMIT\nEXIT\n")
	runBinary(t, bin, dbPath, "BEGIN\nSET a 999\nABORT\nEXIT\n")

	out := runBinary(t, bin, dbPath, "GET a\nGET b\nEXIT\n")
	if strings.TrimSpace(out) != "1\n2" {
		t.Fatalf("expected committed values to survive restarts and aborted ones to not, got %q", out)
	}
}
