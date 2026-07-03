package index

// ListValue is a slice-backed ordered list supporting push at either end
// and Redis-style ranged reads.
type ListValue struct {
	items []string
}

// NewListValue creates an empty list.
func NewListValue() *ListValue {
	return &ListValue{}
}

// LPush inserts value at the head of the list.
func (l *ListValue) LPush(value string) {
	l.items = append(l.items, "")
	copy(l.items[1:], l.items)
	l.items[0] = value
}

// RPush appends value at the tail of the list.
func (l *ListValue) RPush(value string) {
	l.items = append(l.items, value)
}

// Len returns the number of elements in the list.
func (l *ListValue) Len() int {
	return len(l.items)
}

// Range returns the elements between start and stop, inclusive. Negative
// indices count from the end of the list (-1 is the last element), same
// convention as Redis's LRANGE. Out-of-bounds indices are clamped rather
// than treated as errors.
func (l *ListValue) Range(start, stop int) []string {
	n := len(l.items)
	if n == 0 {
		return nil
	}
	start = normalizeIndex(start, n)
	stop = normalizeIndex(stop, n)
	if start < 0 {
		start = 0
	}
	if stop >= n {
		stop = n - 1
	}
	if start > stop || start >= n {
		return nil
	}
	out := make([]string, stop-start+1)
	copy(out, l.items[start:stop+1])
	return out
}

// normalizeIndex converts a possibly-negative Redis-style index into a
// zero-based position within a slice of length n.
func normalizeIndex(i, n int) int {
	if i < 0 {
		i = n + i
	}
	return i
}

// Clone returns a deep copy of the list.
func (l *ListValue) Clone() *ListValue {
	clone := &ListValue{items: make([]string, len(l.items))}
	copy(clone.items, l.items)
	return clone
}
