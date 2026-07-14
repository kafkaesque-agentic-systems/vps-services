package dbs

import (
	"fmt"
	"regexp"
	"strings"
)

var re = regexp.MustCompile(`[\W_]`)

// CreateTag : creates a unique string from the author's surname and
// and the quote text used to prevent duplicate entries in the database
func CreateTag(author, quote string) string {
	surname := strings.Split(author, " ")
	return re.ReplaceAllString(strings.ToLower(surname[len(surname)-1]+quote), "")
}

// CreateRegexQueryString : generates regex based on user input
// used for search quote by authors name
//
// SECURITY (Audit C-5): user input previously flowed UNESCAPED into a MongoDB
// $regex evaluation. A request such as /authors/(a+)+$ let callers execute
// arbitrary — including catastrophically backtracking — patterns against the
// database: an unauthenticated CPU-exhaustion DoS, plus the ability to match
// arbitrary records with wildcard patterns.
//
// Every user-supplied segment is now neutralized with regexp.QuoteMeta before
// being spliced into the pattern, so only OUR anchors and separators
// (`^`, `.*`, `\s`, `$`) carry regex semantics. FAIL CLOSED: input can no
// longer alter the shape of the query, only the literal text matched.
func CreateRegexQueryString(author string) string {
	ns := strings.Split(author, "-")

	// Escape ALL regex metacharacters in every user-controlled segment.
	for i, v := range ns {
		ns[i] = regexp.QuoteMeta(v)
	}

	switch len(ns) {
	case 1:
		return ns[0]

	case 2:
		return fmt.Sprintf(`^%s.*%s$`, ns[0], ns[1])

	default:
		// e.g. "john-fitzgerald-kennedy" -> `^john\sfitzgerald\skennedy$`
		// strings.Join replaces the previous O(N²) string-concatenation loop.
		return `^` + strings.Join(ns[:len(ns)-1], `\s`) + `\s` + ns[len(ns)-1] + `$`
	}
}

// CreateTextQueryString : create a string formatted for multiple text queries
func CreateTextQueryString(words string) (string, []string) {
	split := strings.Split(words, ",")

	query := ""
	var phrases []string

	for _, v := range split {
		if strings.Contains(v, "-") {
			phrases = append(phrases, fmt.Sprintf("\"%s\" ", strings.Replace(v, "-", " ", -1)))

		} else {
			query += fmt.Sprintf("%s ", v)
		}
	}

	return strings.TrimSpace(query), phrases

}
