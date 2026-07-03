package db

// Tokenize splits one raw input line into command tokens. Tokens are
// separated by runs of whitespace; wrapping a token in double quotes lets
// it contain embedded whitespace (e.g. SET greeting "hello world"), and a
// backslash inside quotes escapes the character that follows it (so a
// literal double quote can appear in a value).
func Tokenize(line string) []string {
	var tokens []string
	var cur []rune
	hasCur := false
	inQuotes := false

	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		switch {
		case inQuotes:
			switch {
			case c == '\\' && i+1 < len(runes):
				i++
				cur = append(cur, runes[i])
			case c == '"':
				inQuotes = false
			default:
				cur = append(cur, c)
			}
		case c == '"':
			inQuotes = true
			hasCur = true
		case c == ' ' || c == '\t' || c == '\r':
			if hasCur {
				tokens = append(tokens, string(cur))
				cur = nil
				hasCur = false
			}
		default:
			cur = append(cur, c)
			hasCur = true
		}
	}
	if hasCur {
		tokens = append(tokens, string(cur))
	}
	return tokens
}
