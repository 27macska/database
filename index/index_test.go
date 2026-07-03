package index

import "testing"

func TestSetGetLastWriteWins(t *testing.T) {
	idx := New()
	idx.SetString("a", "1")
	idx.SetString("a", "2")

	e, ok := idx.Get("a", 0)
	if !ok {
		t.Fatalf("expected key a to exist")
	}
	if e.Str != "2" {
		t.Fatalf("expected last write to win, got %q", e.Str)
	}
	if idx.Len(0) != 1 {
		t.Fatalf("expected exactly one entry, got %d", idx.Len(0))
	}
}

func TestDelete(t *testing.T) {
	idx := New()
	idx.SetString("a", "1")
	idx.SetString("b", "2")

	if !idx.Delete("a") {
		t.Fatalf("expected delete of existing key to report true")
	}
	if idx.Delete("a") {
		t.Fatalf("expected delete of already-removed key to report false")
	}
	if _, ok := idx.Get("a", 0); ok {
		t.Fatalf("expected a to be gone")
	}
	if _, ok := idx.Get("b", 0); !ok {
		t.Fatalf("expected b to still be present")
	}
}

func TestExpiry(t *testing.T) {
	idx := New()
	idx.SetString("a", "1")
	if !idx.SetExpireAt("a", 100, 0) {
		t.Fatalf("expected expire to apply to existing key")
	}

	if !idx.Exists("a", 50) {
		t.Fatalf("expected key to still exist before expiry")
	}
	if idx.Exists("a", 100) {
		t.Fatalf("expected key to be expired at/after its expiry timestamp")
	}
	if _, ok := idx.Get("a", 150); ok {
		t.Fatalf("expected expired key to be purged and absent")
	}
}

func TestTTL(t *testing.T) {
	idx := New()
	idx.SetString("a", "1")

	ttl, ok := idx.TTL("a", 0)
	if !ok || ttl != -1 {
		t.Fatalf("expected -1 TTL for key with no expiry, got %d, %v", ttl, ok)
	}

	idx.SetExpireAt("a", 110, 100)
	ttl, ok = idx.TTL("a", 100)
	if !ok || ttl != 10 {
		t.Fatalf("expected TTL 10, got %d, %v", ttl, ok)
	}

	if _, ok := idx.TTL("missing", 0); ok {
		t.Fatalf("expected TTL on missing key to report not-found")
	}
}

func TestRangeLexicographical(t *testing.T) {
	idx := New()
	for _, k := range []string{"banana", "apple", "cherry", "date"} {
		idx.SetString(k, "v-"+k)
	}

	got := idx.Range("apple", "cherry", 0)
	want := []string{"apple", "banana", "cherry"}
	if len(got) != len(want) {
		t.Fatalf("expected %d entries, got %d (%v)", len(want), len(got), got)
	}
	for i, k := range want {
		if got[i].Key != k {
			t.Fatalf("position %d: expected key %q, got %q", i, k, got[i].Key)
		}
	}
}

func TestHashOperations(t *testing.T) {
	idx := New()
	if err := idx.HSet("h", "f1", "v1", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := idx.HSet("h", "f2", "v2", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := idx.HSet("h", "f1", "v1-updated", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	v, ok, err := idx.HGet("h", "f1", 0)
	if err != nil || !ok || v != "v1-updated" {
		t.Fatalf("expected updated value, got %q, %v, %v", v, ok, err)
	}

	all, err := idx.HGetAll("h", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(all))
	}
}

func TestHashWrongType(t *testing.T) {
	idx := New()
	idx.SetString("s", "value")
	if err := idx.HSet("s", "f", "v", 0); err != ErrWrongType {
		t.Fatalf("expected ErrWrongType, got %v", err)
	}
}

func TestListOperations(t *testing.T) {
	idx := New()
	if _, err := idx.RPush("l", []string{"a", "b"}, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := idx.LPush("l", []string{"z"}, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// list should now be [z, a, b]
	got, err := idx.LRange("l", 0, -1, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"z", "a", "b"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestIncrDecr(t *testing.T) {
	idx := New()
	n, err := idx.Incr("counter", 1, 0)
	if err != nil || n != 1 {
		t.Fatalf("expected 1, got %d, %v", n, err)
	}
	n, err = idx.Incr("counter", 1, 0)
	if err != nil || n != 2 {
		t.Fatalf("expected 2, got %d, %v", n, err)
	}
	n, err = idx.Incr("counter", -5, 0)
	if err != nil || n != -3 {
		t.Fatalf("expected -3, got %d, %v", n, err)
	}
}

func TestIncrNonInteger(t *testing.T) {
	idx := New()
	idx.SetString("s", "not-a-number")
	if _, err := idx.Incr("s", 1, 0); err == nil {
		t.Fatalf("expected error incrementing a non-integer value")
	}
}

func TestCloneAndRestore(t *testing.T) {
	idx := New()
	idx.SetString("a", "1")
	if err := idx.HSet("h", "f", "v", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := idx.RPush("l", []string{"x"}, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	snapshot := idx.Clone()

	idx.SetString("a", "2")
	idx.Delete("h")
	if _, err := idx.RPush("l", []string{"y"}, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	idx.SetString("new", "created-after-snapshot")

	idx.Restore(snapshot)

	e, ok := idx.Get("a", 0)
	if !ok || e.Str != "1" {
		t.Fatalf("expected restore to bring back a=1, got %v, %v", e, ok)
	}
	if _, ok := idx.Get("h", 0); !ok {
		t.Fatalf("expected restore to bring back deleted hash h")
	}
	if _, ok := idx.Get("new", 0); ok {
		t.Fatalf("expected restore to remove keys created after the snapshot")
	}
	list, _ := idx.LRange("l", 0, -1, 0)
	if len(list) != 1 || list[0] != "x" {
		t.Fatalf("expected restored list to be [x], got %v", list)
	}
}

func TestCloneIsIndependent(t *testing.T) {
	idx := New()
	idx.SetString("a", "1")
	clone := idx.Clone()
	idx.SetString("a", "2")

	e, _ := clone.Get("a", 0)
	if e.Str != "1" {
		t.Fatalf("expected clone to be unaffected by later mutation of original, got %q", e.Str)
	}
}
