package db

import "testing"

func TestTokenize(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"SET a 1", []string{"SET", "a", "1"}},
		{"  SET   a    1  ", []string{"SET", "a", "1"}},
		{`SET greeting "hello world"`, []string{"SET", "greeting", "hello world"}},
		{"", nil},
		{"   ", nil},
		{`SET q "she said \"hi\""`, []string{"SET", "q", `she said "hi"`}},
		{"EXIT", []string{"EXIT"}},
	}

	for _, c := range cases {
		got := Tokenize(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("Tokenize(%q) = %v, want %v", c.in, got, c.want)
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Fatalf("Tokenize(%q) = %v, want %v", c.in, got, c.want)
			}
		}
	}
}
