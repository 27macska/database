// Package index implements the in-memory key index for the store.
//
// The assignment requires that the main key index NOT be built on top of
// Go's built-in map type. Instead, Index keeps its entries in a plain slice
// and finds keys by scanning the slice linearly. This is obviously O(n) per
// lookup rather than the O(1) a map would give, but it satisfies the
// "custom index" constraint and keeps the scanning logic in one place
// (find) so the rest of the package stays simple.
package index

import (
	"errors"
	"sort"
)

// ErrWrongType is returned when a command expects one value type (e.g. a
// hash) but the key already holds a different type (e.g. a string).
var ErrWrongType = errors.New("WRONGTYPE key holds the wrong kind of value")

// ValueType identifies which kind of value an Entry stores.
type ValueType int

const (
	// TypeString marks an Entry whose value is a plain string.
	TypeString ValueType = iota
	// TypeHash marks an Entry whose value is a field/value hash.
	TypeHash
	// TypeList marks an Entry whose value is an ordered list of strings.
	TypeList
)

// Entry is a single record held by the index.
type Entry struct {
	Key      string
	Type     ValueType
	Str      string     // valid when Type == TypeString
	Hash     *HashValue // valid when Type == TypeHash
	List     *ListValue // valid when Type == TypeList
	ExpireAt int64      // unix seconds; 0 means "no expiry"
}

// Expired reports whether the entry has an expiry set that is at or before
// now (a unix timestamp in seconds).
func (e *Entry) Expired(now int64) bool {
	return e.ExpireAt != 0 && now >= e.ExpireAt
}

// Clone returns a deep copy of the entry, used for transaction snapshots.
func (e *Entry) Clone() *Entry {
	clone := &Entry{
		Key:      e.Key,
		Type:     e.Type,
		Str:      e.Str,
		ExpireAt: e.ExpireAt,
	}
	if e.Hash != nil {
		clone.Hash = e.Hash.Clone()
	}
	if e.List != nil {
		clone.List = e.List.Clone()
	}
	return clone
}

// Index is the custom, map-free key index. Entries are kept in a slice and
// located with a linear scan (see find). Writes follow "last write wins":
// setting an existing key overwrites its entry in place instead of
// appending a duplicate.
type Index struct {
	entries []*Entry
}

// New creates an empty Index.
func New() *Index {
	return &Index{}
}

// find performs the linear scan that backs every lookup in the index. It
// returns nil if key is not present.
func (idx *Index) find(key string) *Entry {
	for _, e := range idx.entries {
		if e.Key == key {
			return e
		}
	}
	return nil
}

// findPos is like find but also reports the slice position, used by
// Delete.
func (idx *Index) findPos(key string) (int, *Entry) {
	for i, e := range idx.entries {
		if e.Key == key {
			return i, e
		}
	}
	return -1, nil
}

// Get returns the live entry for key, honoring lazy expiry: an entry whose
// TTL has passed is treated as absent and is purged from the index.
func (idx *Index) Get(key string, now int64) (*Entry, bool) {
	i, e := idx.findPos(key)
	if e == nil {
		return nil, false
	}
	if e.Expired(now) {
		idx.removeAt(i)
		return nil, false
	}
	return e, true
}

// SetString stores key with a plain string value, last-write-wins, and
// clears any prior expiry or hash/list contents (matching typical SET
// semantics: a fresh SET replaces the whole entry).
func (idx *Index) SetString(key, value string) {
	if e := idx.find(key); e != nil {
		e.Type = TypeString
		e.Str = value
		e.Hash = nil
		e.List = nil
		e.ExpireAt = 0
		return
	}
	idx.entries = append(idx.entries, &Entry{Key: key, Type: TypeString, Str: value})
}

// Delete removes key from the index. It reports whether the key was
// present.
func (idx *Index) Delete(key string) bool {
	i, e := idx.findPos(key)
	if e == nil {
		return false
	}
	idx.removeAt(i)
	return true
}

// removeAt deletes the entry at slice position i without preserving order
// (swap with the last element, then truncate) since the index is unordered
// anyway.
func (idx *Index) removeAt(i int) {
	last := len(idx.entries) - 1
	idx.entries[i] = idx.entries[last]
	idx.entries[last] = nil
	idx.entries = idx.entries[:last]
}

// Exists reports whether key is present and unexpired.
func (idx *Index) Exists(key string, now int64) bool {
	_, ok := idx.Get(key, now)
	return ok
}

// SetExpireAt sets an absolute unix-second expiry timestamp on an existing,
// unexpired key. It reports whether the key existed.
func (idx *Index) SetExpireAt(key string, at int64, now int64) bool {
	e, ok := idx.Get(key, now)
	if !ok {
		return false
	}
	e.ExpireAt = at
	return true
}

// TTL returns the number of whole seconds remaining before key expires.
// It returns (-1, true) for a key with no expiry, and (0, false) if the key
// does not exist (or has already expired).
func (idx *Index) TTL(key string, now int64) (int64, bool) {
	e, ok := idx.Get(key, now)
	if !ok {
		return 0, false
	}
	if e.ExpireAt == 0 {
		return -1, true
	}
	return e.ExpireAt - now, true
}

// Clear removes every entry from the index (used by FLUSHDB).
func (idx *Index) Clear() {
	idx.entries = nil
}

// Len returns the number of live (unexpired) keys in the index.
func (idx *Index) Len(now int64) int {
	n := 0
	for _, e := range idx.entries {
		if !e.Expired(now) {
			n++
		}
	}
	return n
}

// RangeEntry is one result row from Range.
type RangeEntry struct {
	Key   string
	Value string
}

// Range returns every live string-typed entry whose key falls within
// [start, end] inclusive, ordered lexicographically by key. Non-string
// entries are skipped since they have no single scalar value to display.
func (idx *Index) Range(start, end string, now int64) []RangeEntry {
	var out []RangeEntry
	for i := 0; i < len(idx.entries); i++ {
		e := idx.entries[i]
		if e.Expired(now) {
			idx.removeAt(i)
			i--
			continue
		}
		if e.Type != TypeString {
			continue
		}
		if e.Key >= start && e.Key <= end {
			out = append(out, RangeEntry{Key: e.Key, Value: e.Str})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// getOrCreateHash returns the HashValue for key, creating a new hash entry
// if the key is absent. It returns ErrWrongType if key already holds a
// non-hash value.
func (idx *Index) getOrCreateHash(key string, now int64) (*HashValue, error) {
	e, ok := idx.Get(key, now)
	if !ok {
		e = &Entry{Key: key, Type: TypeHash, Hash: NewHashValue()}
		idx.entries = append(idx.entries, e)
		return e.Hash, nil
	}
	if e.Type != TypeHash {
		return nil, ErrWrongType
	}
	return e.Hash, nil
}

// HSet sets field to value within the hash stored at key, creating the
// hash if necessary. Field assignment is last-write-wins, same as the
// top-level index.
func (idx *Index) HSet(key, field, value string, now int64) error {
	h, err := idx.getOrCreateHash(key, now)
	if err != nil {
		return err
	}
	h.Set(field, value)
	return nil
}

// HGet returns the value of field within the hash at key.
func (idx *Index) HGet(key, field string, now int64) (string, bool, error) {
	e, ok := idx.Get(key, now)
	if !ok {
		return "", false, nil
	}
	if e.Type != TypeHash {
		return "", false, ErrWrongType
	}
	v, found := e.Hash.Get(field)
	return v, found, nil
}

// HGetAll returns every field/value pair in the hash at key.
func (idx *Index) HGetAll(key string, now int64) ([]FieldEntry, error) {
	e, ok := idx.Get(key, now)
	if !ok {
		return nil, nil
	}
	if e.Type != TypeHash {
		return nil, ErrWrongType
	}
	return e.Hash.All(), nil
}

// getOrCreateList returns the ListValue for key, creating a new list entry
// if the key is absent. It returns ErrWrongType if key already holds a
// non-list value.
func (idx *Index) getOrCreateList(key string, now int64) (*ListValue, error) {
	e, ok := idx.Get(key, now)
	if !ok {
		e = &Entry{Key: key, Type: TypeList, List: NewListValue()}
		idx.entries = append(idx.entries, e)
		return e.List, nil
	}
	if e.Type != TypeList {
		return nil, ErrWrongType
	}
	return e.List, nil
}

// LPush prepends values (in the order given) to the list at key.
func (idx *Index) LPush(key string, values []string, now int64) (int, error) {
	l, err := idx.getOrCreateList(key, now)
	if err != nil {
		return 0, err
	}
	for _, v := range values {
		l.LPush(v)
	}
	return l.Len(), nil
}

// RPush appends values (in the order given) to the list at key.
func (idx *Index) RPush(key string, values []string, now int64) (int, error) {
	l, err := idx.getOrCreateList(key, now)
	if err != nil {
		return 0, err
	}
	for _, v := range values {
		l.RPush(v)
	}
	return l.Len(), nil
}

// LRange returns the list elements at key between start and stop
// (inclusive, Redis-style negative indices supported).
func (idx *Index) LRange(key string, start, stop int, now int64) ([]string, error) {
	e, ok := idx.Get(key, now)
	if !ok {
		return nil, nil
	}
	if e.Type != TypeList {
		return nil, ErrWrongType
	}
	return e.List.Range(start, stop), nil
}

// Incr adds delta to the integer value stored at key (treating a missing
// key as 0) and returns the new value.
func (idx *Index) Incr(key string, delta int64, now int64) (int64, error) {
	e, ok := idx.Get(key, now)
	if !ok {
		idx.SetString(key, formatInt(delta))
		return delta, nil
	}
	if e.Type != TypeString {
		return 0, ErrWrongType
	}
	n, err := parseInt(e.Str)
	if err != nil {
		return 0, err
	}
	n += delta
	e.Str = formatInt(n)
	return n, nil
}

// Clone returns a deep copy of the entire index, used to snapshot state
// before a transaction so ABORT can restore it.
func (idx *Index) Clone() *Index {
	clone := &Index{entries: make([]*Entry, len(idx.entries))}
	for i, e := range idx.entries {
		clone.entries[i] = e.Clone()
	}
	return clone
}

// Restore replaces the index's contents with the state captured by a prior
// Clone call.
func (idx *Index) Restore(snapshot *Index) {
	idx.entries = snapshot.entries
}

// Keys returns every live key in the index, in arbitrary order.
func (idx *Index) Keys(now int64) []string {
	out := make([]string, 0, len(idx.entries))
	for _, e := range idx.entries {
		if !e.Expired(now) {
			out = append(out, e.Key)
		}
	}
	return out
}
