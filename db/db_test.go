package db

import (
	"path/filepath"
	"testing"
)

// openTestDB opens a store backed by a fresh temp file and pins its clock
// to a mutable variable so EXPIRE/TTL tests don't need to sleep.
func openTestDB(t *testing.T) (*DB, string, *int64) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "data.db")
	d, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	clock := int64(1_000_000)
	d.Clock = func() int64 { return clock }
	return d, path, &clock
}

func exec(t *testing.T, d *DB, tokens ...string) string {
	t.Helper()
	out, err := d.Execute(tokens)
	if err != nil {
		t.Fatalf("Execute(%v): unexpected error %v", tokens, err)
	}
	return out
}

func TestSetGet(t *testing.T) {
	d, _, _ := openTestDB(t)

	if got := exec(t, d, "SET", "a", "1"); got != "OK" {
		t.Fatalf("SET = %q, want OK", got)
	}
	if got := exec(t, d, "GET", "a"); got != "1" {
		t.Fatalf("GET = %q, want 1", got)
	}
	if got := exec(t, d, "GET", "missing"); got != "(nil)" {
		t.Fatalf("GET missing = %q, want (nil)", got)
	}
}

func TestSetLastWriteWins(t *testing.T) {
	d, _, _ := openTestDB(t)
	exec(t, d, "SET", "a", "1")
	exec(t, d, "SET", "a", "2")
	if got := exec(t, d, "GET", "a"); got != "2" {
		t.Fatalf("GET = %q, want 2", got)
	}
}

func TestDelExists(t *testing.T) {
	d, _, _ := openTestDB(t)
	exec(t, d, "SET", "a", "1")

	if got := exec(t, d, "EXISTS", "a"); got != "1" {
		t.Fatalf("EXISTS = %q, want 1", got)
	}
	if got := exec(t, d, "DEL", "a"); got != "1" {
		t.Fatalf("DEL = %q, want 1", got)
	}
	if got := exec(t, d, "EXISTS", "a"); got != "0" {
		t.Fatalf("EXISTS after delete = %q, want 0", got)
	}
	if got := exec(t, d, "DEL", "a"); got != "0" {
		t.Fatalf("DEL again = %q, want 0", got)
	}
}

func TestMSetMGet(t *testing.T) {
	d, _, _ := openTestDB(t)
	if got := exec(t, d, "MSET", "a", "1", "b", "2"); got != "OK" {
		t.Fatalf("MSET = %q, want OK", got)
	}
	got := exec(t, d, "MGET", "a", "b", "missing")
	want := "1\n2\n(nil)"
	if got != want {
		t.Fatalf("MGET = %q, want %q", got, want)
	}
}

func TestExpireTTL(t *testing.T) {
	d, _, clock := openTestDB(t)
	exec(t, d, "SET", "a", "1")

	if got := exec(t, d, "TTL", "a"); got != "-1" {
		t.Fatalf("TTL with no expiry = %q, want -1", got)
	}
	if got := exec(t, d, "EXPIRE", "a", "10"); got != "1" {
		t.Fatalf("EXPIRE = %q, want 1", got)
	}
	if got := exec(t, d, "TTL", "a"); got != "10" {
		t.Fatalf("TTL = %q, want 10", got)
	}

	*clock += 10
	if got := exec(t, d, "GET", "a"); got != "(nil)" {
		t.Fatalf("GET after expiry = %q, want (nil)", got)
	}
	if got := exec(t, d, "TTL", "a"); got != "-2" {
		t.Fatalf("TTL on missing key = %q, want -2", got)
	}
	if got := exec(t, d, "EXPIRE", "nosuchkey", "10"); got != "0" {
		t.Fatalf("EXPIRE on missing key = %q, want 0", got)
	}
}

func TestRange(t *testing.T) {
	d, _, _ := openTestDB(t)
	exec(t, d, "MSET", "banana", "1", "apple", "2", "cherry", "3", "date", "4")

	got := exec(t, d, "RANGE", "apple", "cherry")
	want := "apple 2\nbanana 1\ncherry 3"
	if got != want {
		t.Fatalf("RANGE = %q, want %q", got, want)
	}

	if got := exec(t, d, "RANGE", "zzz", "zzzz"); got != "(empty)" {
		t.Fatalf("RANGE with no matches = %q, want (empty)", got)
	}
}

func TestHash(t *testing.T) {
	d, _, _ := openTestDB(t)
	exec(t, d, "HSET", "h", "f1", "v1")
	exec(t, d, "HSET", "h", "f2", "v2")

	if got := exec(t, d, "HGET", "h", "f1"); got != "v1" {
		t.Fatalf("HGET = %q, want v1", got)
	}
	if got := exec(t, d, "HGET", "h", "missing"); got != "(nil)" {
		t.Fatalf("HGET missing field = %q, want (nil)", got)
	}

	got := exec(t, d, "HGETALL", "h")
	want := "f1 v1\nf2 v2"
	if got != want {
		t.Fatalf("HGETALL = %q, want %q", got, want)
	}
}

func TestList(t *testing.T) {
	d, _, _ := openTestDB(t)
	if got := exec(t, d, "RPUSH", "l", "a", "b"); got != "2" {
		t.Fatalf("RPUSH = %q, want 2", got)
	}
	if got := exec(t, d, "LPUSH", "l", "z"); got != "3" {
		t.Fatalf("LPUSH = %q, want 3", got)
	}
	got := exec(t, d, "LRANGE", "l", "0", "-1")
	want := "z\na\nb"
	if got != want {
		t.Fatalf("LRANGE = %q, want %q", got, want)
	}
	if got := exec(t, d, "LRANGE", "missing", "0", "-1"); got != "(empty)" {
		t.Fatalf("LRANGE missing key = %q, want (empty)", got)
	}
}

func TestIncrDecr(t *testing.T) {
	d, _, _ := openTestDB(t)
	if got := exec(t, d, "INCR", "c"); got != "1" {
		t.Fatalf("INCR = %q, want 1", got)
	}
	if got := exec(t, d, "INCR", "c"); got != "2" {
		t.Fatalf("INCR = %q, want 2", got)
	}
	if got := exec(t, d, "DECR", "c"); got != "1" {
		t.Fatalf("DECR = %q, want 1", got)
	}
}

func TestFlushDB(t *testing.T) {
	d, _, _ := openTestDB(t)
	exec(t, d, "SET", "a", "1")
	exec(t, d, "SET", "b", "2")
	if got := exec(t, d, "FLUSHDB"); got != "OK" {
		t.Fatalf("FLUSHDB = %q, want OK", got)
	}
	if got := exec(t, d, "EXISTS", "a"); got != "0" {
		t.Fatalf("EXISTS after FLUSHDB = %q, want 0", got)
	}
	if got := exec(t, d, "EXISTS", "b"); got != "0" {
		t.Fatalf("EXISTS after FLUSHDB = %q, want 0", got)
	}
}

func TestWrongType(t *testing.T) {
	d, _, _ := openTestDB(t)
	exec(t, d, "SET", "s", "value")

	if got := exec(t, d, "HGET", "s", "field"); got[:4] != "ERR " {
		t.Fatalf("HGET on string key = %q, want ERR prefix", got)
	}
	if got := exec(t, d, "LRANGE", "s", "0", "-1"); got[:4] != "ERR " {
		t.Fatalf("LRANGE on string key = %q, want ERR prefix", got)
	}

	exec(t, d, "RPUSH", "l", "x")
	if got := exec(t, d, "GET", "l"); got[:4] != "ERR " {
		t.Fatalf("GET on list key = %q, want ERR prefix", got)
	}
}

func TestUnknownCommand(t *testing.T) {
	d, _, _ := openTestDB(t)
	got := exec(t, d, "FROBNICATE", "a")
	if got != `ERR unknown command "FROBNICATE"` {
		t.Fatalf("unexpected error text: %q", got)
	}
}

func TestExitSentinel(t *testing.T) {
	d, _, _ := openTestDB(t)
	_, err := d.Execute([]string{"EXIT"})
	if err != ErrExit {
		t.Fatalf("expected ErrExit, got %v", err)
	}
}

func TestTransactionCommit(t *testing.T) {
	d, path, _ := openTestDB(t)
	exec(t, d, "SET", "a", "1")

	if got := exec(t, d, "BEGIN"); got != "OK" {
		t.Fatalf("BEGIN = %q, want OK", got)
	}
	exec(t, d, "SET", "a", "2")
	exec(t, d, "SET", "b", "new")

	// Reads inside the transaction should see its own uncommitted writes.
	if got := exec(t, d, "GET", "a"); got != "2" {
		t.Fatalf("GET inside tx = %q, want 2 (read-your-own-writes)", got)
	}

	if got := exec(t, d, "COMMIT"); got != "OK" {
		t.Fatalf("COMMIT = %q, want OK", got)
	}
	if got := exec(t, d, "GET", "a"); got != "2" {
		t.Fatalf("GET after commit = %q, want 2", got)
	}
	if got := exec(t, d, "GET", "b"); got != "new" {
		t.Fatalf("GET after commit = %q, want new", got)
	}

	_ = d.Close()

	// Reopen (replay the log) and confirm the committed writes persisted.
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	reopened.Clock = d.Clock
	if got := exec(t, reopened, "GET", "a"); got != "2" {
		t.Fatalf("GET after reopen = %q, want 2", got)
	}
	if got := exec(t, reopened, "GET", "b"); got != "new" {
		t.Fatalf("GET after reopen = %q, want new", got)
	}
}

func TestTransactionAbort(t *testing.T) {
	d, path, _ := openTestDB(t)
	exec(t, d, "SET", "a", "1")

	exec(t, d, "BEGIN")
	exec(t, d, "SET", "a", "999")
	exec(t, d, "DEL", "a")
	exec(t, d, "SET", "brand-new", "x")

	if got := exec(t, d, "ABORT"); got != "OK" {
		t.Fatalf("ABORT = %q, want OK", got)
	}

	if got := exec(t, d, "GET", "a"); got != "1" {
		t.Fatalf("GET after abort = %q, want 1 (restored)", got)
	}
	if got := exec(t, d, "EXISTS", "brand-new"); got != "0" {
		t.Fatalf("EXISTS after abort = %q, want 0", got)
	}

	_ = d.Close()

	// Nothing from the aborted transaction should have reached the log.
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	if got := exec(t, reopened, "GET", "a"); got != "1" {
		t.Fatalf("GET after reopen = %q, want 1", got)
	}
	if got := exec(t, reopened, "EXISTS", "brand-new"); got != "0" {
		t.Fatalf("EXISTS after reopen = %q, want 0", got)
	}
}

func TestCommitWithoutBeginErrors(t *testing.T) {
	d, _, _ := openTestDB(t)
	got := exec(t, d, "COMMIT")
	if got != "ERR no transaction in progress" {
		t.Fatalf("COMMIT without BEGIN = %q", got)
	}
}

func TestAbortWithoutBeginErrors(t *testing.T) {
	d, _, _ := openTestDB(t)
	got := exec(t, d, "ABORT")
	if got != "ERR no transaction in progress" {
		t.Fatalf("ABORT without BEGIN = %q", got)
	}
}

func TestNestedBeginErrors(t *testing.T) {
	d, _, _ := openTestDB(t)
	exec(t, d, "BEGIN")
	got := exec(t, d, "BEGIN")
	if got != "ERR transaction already in progress" {
		t.Fatalf("nested BEGIN = %q", got)
	}
}

func TestPersistenceAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.db")

	d1, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	exec(t, d1, "SET", "a", "1")
	exec(t, d1, "RPUSH", "l", "x", "y")
	exec(t, d1, "HSET", "h", "f", "v")
	exec(t, d1, "INCR", "c")
	exec(t, d1, "INCR", "c")
	_ = d1.Close()

	d2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = d2.Close() }()

	if got := exec(t, d2, "GET", "a"); got != "1" {
		t.Fatalf("GET after restart = %q, want 1", got)
	}
	if got := exec(t, d2, "LRANGE", "l", "0", "-1"); got != "x\ny" {
		t.Fatalf("LRANGE after restart = %q, want x\\ny", got)
	}
	if got := exec(t, d2, "HGET", "h", "f"); got != "v" {
		t.Fatalf("HGET after restart = %q, want v", got)
	}
	if got := exec(t, d2, "GET", "c"); got != "2" {
		t.Fatalf("GET counter after restart = %q, want 2", got)
	}
}

func TestDeleteThenRestartDoesNotResurrectKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.db")

	d1, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	exec(t, d1, "SET", "a", "1")
	exec(t, d1, "DEL", "a")
	_ = d1.Close()

	d2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = d2.Close() }()
	if got := exec(t, d2, "EXISTS", "a"); got != "0" {
		t.Fatalf("EXISTS after restart = %q, want 0", got)
	}
}
