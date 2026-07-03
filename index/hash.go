package index

// FieldEntry is one field/value pair inside a HashValue.
type FieldEntry struct {
	Field string
	Value string
}

// HashValue is a linear-scan, slice-backed hash (the same "no built-in
// map" rule used for the top-level Index applies to nested field lookups
// too, so HSET/HGET/HGETALL never rely on Go's map type either).
type HashValue struct {
	fields []FieldEntry
}

// NewHashValue creates an empty hash.
func NewHashValue() *HashValue {
	return &HashValue{}
}

// Set assigns field to value, last-write-wins.
func (h *HashValue) Set(field, value string) {
	for i := range h.fields {
		if h.fields[i].Field == field {
			h.fields[i].Value = value
			return
		}
	}
	h.fields = append(h.fields, FieldEntry{Field: field, Value: value})
}

// Get returns the value stored for field, if any.
func (h *HashValue) Get(field string) (string, bool) {
	for _, fe := range h.fields {
		if fe.Field == field {
			return fe.Value, true
		}
	}
	return "", false
}

// All returns a copy of every field/value pair in the hash.
func (h *HashValue) All() []FieldEntry {
	out := make([]FieldEntry, len(h.fields))
	copy(out, h.fields)
	return out
}

// Clone returns a deep copy of the hash.
func (h *HashValue) Clone() *HashValue {
	clone := &HashValue{fields: make([]FieldEntry, len(h.fields))}
	copy(clone.fields, h.fields)
	return clone
}
