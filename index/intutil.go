package index

import (
	"fmt"
	"strconv"
)

// parseInt parses s as a base-10 int64, returning a descriptive error on
// failure (used by INCR/DECR when the stored value isn't numeric).
func parseInt(s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("value is not an integer: %q", s)
	}
	return n, nil
}

// formatInt renders n as a base-10 string.
func formatInt(n int64) string {
	return strconv.FormatInt(n, 10)
}
