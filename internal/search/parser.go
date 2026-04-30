package search

// QueryParser parses a Kibana-style query string into structured filters and keywords.
type QueryParser interface {
	Parse(raw string) (*ParsedQuery, error)
}

type ParsedQuery struct {
	Filters  []Filter
	Keywords []string
}

// defaultParser implements QueryParser.
type defaultParser struct{}

// NewQueryParser returns the built-in query parser.
func NewQueryParser() QueryParser {
	return &defaultParser{}
}

// Parse parses queries like: level:error tag:api "some phrase" timeout
// Rules:
//   - key:value → filter (Field=key, Operator=eq)
//   - key:"quoted value" → filter with quoted value
//   - bare word → keyword (message ILIKE search)
//   - empty string → no filters, no keywords
func (p *defaultParser) Parse(raw string) (*ParsedQuery, error) {
	result := &ParsedQuery{}
	raw = trimSpace(raw)
	if raw == "" {
		return result, nil
	}

	tokens := tokenize(raw)
	for _, tok := range tokens {
		if tok.isKV {
			result.Filters = append(result.Filters, Filter{
				Field:    tok.key,
				Operator: "eq",
				Value:    tok.value,
			})
		} else {
			result.Keywords = append(result.Keywords, tok.text)
		}
	}

	return result, nil
}

type token struct {
	isKV  bool
	key   string
	value string
	text  string
}

func tokenize(raw string) []token {
	var tokens []token
	i := 0
	n := len(raw)

	for i < n {
		// skip whitespace
		for i < n && raw[i] == ' ' {
			i++
		}
		if i >= n {
			break
		}

		// try to find key:value pattern
		// scan until space or colon
		j := i
		hasColon := false
		colonPos := -1
		for j < n && raw[j] != ' ' {
			if raw[j] == ':' && !hasColon {
				hasColon = true
				colonPos = j
			}
			j++
		}

		if hasColon && colonPos > i {
			key := raw[i:colonPos]
			valStart := colonPos + 1

			if valStart < n && raw[valStart] == '"' {
				// quoted value
				endQuote := findClosingQuote(raw, valStart+1)
				if endQuote != -1 {
					tokens = append(tokens, token{isKV: true, key: key, value: raw[valStart+1 : endQuote]})
					i = endQuote + 1
					// skip trailing space
					for i < n && raw[i] == ' ' {
						i++
					}
					continue
				}
			}

			if valStart < j {
				tokens = append(tokens, token{isKV: true, key: key, value: raw[valStart:j]})
				i = j
				continue
			}
		}

		// Check if current segment starts with a quote
		if raw[i] == '"' {
			endQuote := findClosingQuote(raw, i+1)
			if endQuote != -1 {
				tokens = append(tokens, token{text: raw[i+1 : endQuote]})
				i = endQuote + 1
				continue
			}
		}

		// bare word
		tokens = append(tokens, token{text: raw[i:j]})
		i = j
	}

	return tokens
}

func findClosingQuote(s string, start int) int {
	for i := start; i < len(s); i++ {
		if s[i] == '"' {
			return i
		}
		if s[i] == '\\' && i+1 < len(s) {
			i++ // skip escaped char
		}
	}
	return -1
}

func trimSpace(s string) string {
	for len(s) > 0 && s[0] == ' ' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == ' ' {
		s = s[:len(s)-1]
	}
	return s
}
